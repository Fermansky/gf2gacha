package logic

import (
	"encoding/json"
	"fmt"
	"gf2gacha/logger"
	"gf2gacha/model"
	"gf2gacha/preload"
	"gf2gacha/util"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func UpdatePoolInfo(isFull bool) (messageList []string, err error) {
	logInfo, err := util.GetLogInfo()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if logInfo.AccessToken == "" {
		return nil, errors.New("Access Token不存在")
	}

	uid, err := getUIDByToken(logInfo.AccessToken)
	if err != nil {
		// 获取 UID 失败，谨慎起见，暂不执行更新
		return nil, errors.WithStack(err)
	}

	if logInfo.Uid != uid {
		// UID 不同，更新临时 UID，确保不写入错误账号
		logger.Logger.Warnf("UID不一致：binlog UID=%s，AccessToken解析 UID=%s，将以后者为准", logInfo.Uid, uid)
		logInfo.Uid = uid
	}

	messageList = append(messageList, logInfo.Uid)
	for _, poolTypeUnit := range preload.PoolTypeMap {
		if isFull {
			n, err := fullUpdatePoolInfo(logInfo, poolTypeUnit.Id)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			messageList = append(messageList, fmt.Sprintf("%s 全量更新%d条数据", poolTypeUnit.Name, n))
		} else {
			n, err := incrementalUpdatePoolInfo(logInfo, poolTypeUnit.Id)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			messageList = append(messageList, fmt.Sprintf("%s 增量更新%d条数据", poolTypeUnit.Name, n))
		}
	}

	return messageList, nil
}

// incrementalUpdatePoolInfo 增量更新
func incrementalUpdatePoolInfo(logInfo model.LogInfo, poolType int64) (int, error) {
	localRecordList, err := GetLocalRecord(logInfo.Uid, poolType, 0)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var lastLocalRecord model.LocalRecord

	if len(localRecordList) > 0 {
		lastLocalRecord = localRecordList[len(localRecordList)-1]
	}

	var diffRemoteRecordList []model.RemoteRecord
	respData, err := FetchRemoteData(logInfo.GachaUrl, logInfo.AccessToken, "", poolType)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	//todo 只对比一条是有可能有问题的
	var flag bool
	for i, remoteRecord := range respData.RecordList {
		if remoteRecord.ItemId == lastLocalRecord.ItemId && remoteRecord.GachaTimestamp == lastLocalRecord.GachaTimestamp {
			flag = true
			break
		} else {
			diffRemoteRecordList = append(diffRemoteRecordList, respData.RecordList[i])
		}
	}
	for respData.Next != "" && !flag {
		//time.Sleep(50 * time.Millisecond) //这个接口似乎没有限制频率
		respData, err = FetchRemoteData(logInfo.GachaUrl, logInfo.AccessToken, respData.Next, poolType)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		for i, remoteRecord := range respData.RecordList {
			if remoteRecord.ItemId == lastLocalRecord.ItemId && remoteRecord.GachaTimestamp == lastLocalRecord.GachaTimestamp {
				flag = true
				break
			} else {
				diffRemoteRecordList = append(diffRemoteRecordList, respData.RecordList[i])
			}
		}
	}

	if len(diffRemoteRecordList) > 0 {
		var diffLocalRecordList []model.LocalRecord
		for i := len(diffRemoteRecordList) - 1; i >= 0; i-- {
			diffLocalRecordList = append(diffLocalRecordList, model.LocalRecord{
				PoolType:       poolType,
				PoolId:         diffRemoteRecordList[i].PoolId,
				ItemId:         diffRemoteRecordList[i].ItemId,
				GachaTimestamp: diffRemoteRecordList[i].GachaTimestamp,
			})
		}
		err = SaveLocalRecord(logInfo.Uid, diffLocalRecordList)
		if err != nil {
			return 0, errors.WithStack(err)
		}
	}

	updateNum := len(diffRemoteRecordList)
	logger.Logger.Infof("UID:%s poolType:%d 增量更新%d条数据", logInfo.Uid, poolType, updateNum)

	return updateNum, nil
}

// fullUpdatePoolInfo 全量更新
func fullUpdatePoolInfo(logInfo model.LogInfo, poolType int64) (int, error) {
	var remoteRecordList []model.LocalRecord

	respData, err := FetchRemoteData(logInfo.GachaUrl, logInfo.AccessToken, "", poolType)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	for _, record := range respData.RecordList {
		remoteRecordList = append(remoteRecordList, model.LocalRecord{
			PoolType:       poolType,
			PoolId:         record.PoolId,
			ItemId:         record.ItemId,
			GachaTimestamp: record.GachaTimestamp,
		})
	}

	for respData.Next != "" {
		//time.Sleep(50 * time.Millisecond) //这个接口似乎没有限制频率
		respData, err = FetchRemoteData(logInfo.GachaUrl, logInfo.AccessToken, respData.Next, poolType)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		for _, record := range respData.RecordList {
			remoteRecordList = append(remoteRecordList, model.LocalRecord{
				PoolType:       poolType,
				PoolId:         record.PoolId,
				ItemId:         record.ItemId,
				GachaTimestamp: record.GachaTimestamp,
			})
		}
	}

	if len(remoteRecordList) == 0 {
		return 0, nil
	}
	//合并前先备份
	err = util.BackupDB()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	slices.Reverse(remoteRecordList)

	localRecordList, err := GetLocalRecord(logInfo.Uid, poolType, remoteRecordList[0].GachaTimestamp)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	newRecordList := MergeRecord(remoteRecordList, localRecordList)

	err = RemoveLocalRecord(logInfo.Uid, poolType)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	err = SaveLocalRecord(logInfo.Uid, newRecordList)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	updateNum := len(newRecordList)
	logger.Logger.Infof("UID:%s poolType:%d 全量更新%d条数据", logInfo.Uid, poolType, updateNum)

	return updateNum, nil
}

func getUIDByToken(accessToken string) (string, error) {
	reqBody := url.Values{}
	// 暂时写死，在binlog中可能提取到"SDK_ACCOUNT_BASE_URL":"https://gf2-zoneinfo.sunborngame.com/"
	infoUrl := "https://gf2-zoneinfo.sunborngame.com/account/info"

	request, err := http.NewRequest("POST", infoUrl, strings.NewReader(reqBody.Encode()))
	if err != nil {
		return "", errors.WithStack(err)
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Set("Authorization", accessToken)

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer resp.Body.Close()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.WithStack(err)
	}
	var respBody model.ResponseBody
	err = json.Unmarshal(respBodyBytes, &respBody)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if respBody.Code != 0 {
		return "", errors.Errorf("%s(Code %d)", respBody.Message, respBody.Code)
	}

	var raw map[string]interface{}
	err = json.Unmarshal(respBody.Data, &raw)
	if err != nil {
		return "", errors.WithStack(err)
	}

	logger.Logger.Infof("%+v\n", raw)
	uidValue, ok := raw["uid"]
	if !ok {
		return "", errors.New("未找到 uid 字段")
	}

	// 强制转换为字符串
	uid := ""
	switch v := uidValue.(type) {
	case string:
		uid = v
	case float64:
		uid = fmt.Sprintf("%.0f", v) // 去除小数点，转换为字符串
	case int:
		uid = strconv.Itoa(v)
	default:
		return "", errors.New("uid 字段格式无法识别，无法转换为字符串")
	}

	return uid, nil
}
