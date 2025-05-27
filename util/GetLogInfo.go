package util

import (
	"gf2gacha/config"
	"gf2gacha/logger"
	"gf2gacha/model"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"regexp"
)

func GetLogInfo() (logInfo model.LogInfo, err error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}

	logPath := filepath.Join(userHome, "/AppData/LocalLow/SunBorn/少女前线2：追放/Player.log")
	logData, err := os.ReadFile(logPath)
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}

	regexpGamePath, err := regexp.Compile(`\[Subsystems] Discovering subsystems at path (.+)/UnitySubsystems`)
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}
	resultGamePath := regexpGamePath.FindSubmatch(logData)
	if len(resultGamePath) == 2 {
		logInfo.TablePath = filepath.Join(string(resultGamePath[1]), "LocalCache/Data/Table")
	} else {
		return model.LogInfo{}, errors.New("未在日志中找到游戏路径")
	}

	binLogInfo, err := GetBinLogInfo()
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}
	logInfo.GachaUrl = binLogInfo.GachaUrl
	logInfo.Uid = binLogInfo.Uid

	// 尝试从用户提交的信息中获取AccessToken
	if config.GetRuntimeLogInfo().AccessToken != "" {
		logInfo.AccessToken = config.GetRuntimeLogInfo().AccessToken
	} else {
		logger.Logger.Warnln("获取AccessToken失败，用户没有填写这一字段")
	}

	// 以下逻辑目前无效
	//regexpUserInfo, err := regexp.Compile(`"access_token":"(.+?)".+"uid":(\d+)`)
	//if err != nil {
	//	return model.LogInfo{}, errors.WithStack(err)
	//}
	//resultUserInfoList := regexpUserInfo.FindAllSubmatch(logData, -1)
	//if len(resultUserInfoList) == 0 {
	//	return logInfo, errors.New("未在日志中找到AccessToken或Uid,可能是最近一次游戏启动时未登录")
	//}
	//resultUserInfo := resultUserInfoList[len(resultUserInfoList)-1]
	//if len(resultUserInfo) == 3 {
	//	logInfo.AccessToken = string(resultUserInfo[1])
	//	logInfo.Uid = string(resultUserInfo[2])
	//} else {
	//	return logInfo, errors.New("未在日志中找到AccessToken或Uid,可能是最近一次游戏启动时未登录")
	//}
	//
	//regexpGachaUrl, err := regexp.Compile(`"gacha_record_url":"(.*?)"`)
	//if err != nil {
	//	return logInfo, errors.WithStack(err)
	//}
	//resultGachaUrlList := regexpGachaUrl.FindAllSubmatch(logData, -1)
	//if len(resultGachaUrlList) == 0 {
	//	return model.LogInfo{}, errors.New("未在日志中找到抽卡链接")
	//}
	//resultGachaUrl := resultGachaUrlList[len(resultGachaUrlList)-1]
	//if len(resultGachaUrl) == 2 {
	//	logInfo.GachaUrl = string(resultGachaUrl[1])
	//} else {
	//	return logInfo, errors.New("未在日志中找到抽卡链接")
	//}

	return logInfo, nil
}
