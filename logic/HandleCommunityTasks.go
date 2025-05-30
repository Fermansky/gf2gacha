package logic

import (
	"fmt"
	"gf2gacha/config"
	"gf2gacha/logger"
	"gf2gacha/request"
	"gf2gacha/util"
	"github.com/pkg/errors"
	"sort"
	"time"
)

func HandleCommunityTasks() (messageList []string, err error) {
	logInfo, err := util.GetLogInfo()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if logInfo.AccessToken == "" {
		return nil, errors.New("Access Token不存在")
	}

	webToken, err := request.CommunityLogin(logInfo.AccessToken)
	if err != nil {
		var respData request.CommonResponse
		if errors.As(err, &respData) {
			if respData.Code == -1 {
				logger.Logger.Info("AccessToken失效，尝试使用保存的WebToken")
				webToken = config.GetWebToken(logInfo.Uid)
				if webToken == "" {
					return nil, errors.New("AccessToken失效且无保存的WebToken，您可能在其他设备登录过，请在本设备重新登录")
				}
			} else {
				return nil, errors.WithStack(err)
			}
		} else {
			return nil, errors.WithStack(err)
		}
	}
	err = config.SetWebToken(logInfo.Uid, webToken)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	taskListData, err := request.CommunityTaskList(webToken)
	if err != nil {
		var respData request.CommonResponse
		if errors.As(err, &respData) && respData.Code == -1 {
			return nil, errors.New("AccessToken失效且WebToken过期，您可能在其他设备登录过，请在本设备重新登录")
		} else {
			return nil, errors.WithStack(err)
		}
	}

	for _, dailyTask := range taskListData.DailyTask {
		if dailyTask.CompleteCount < dailyTask.MaxCompleteCount {
			switch dailyTask.TaskName {
			case "浏览帖子":
				viewMessageList, err := handleCommunityTaskView(webToken, dailyTask.MaxCompleteCount-dailyTask.CompleteCount)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				messageList = append(messageList, viewMessageList...)
			case "点赞帖子":
				likeMessageList, err := handleCommunityTaskLike(webToken, dailyTask.MaxCompleteCount-dailyTask.CompleteCount)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				messageList = append(messageList, likeMessageList...)
			case "分享帖子":
				shareMessageList, err := handleCommunityTaskShare(webToken, dailyTask.MaxCompleteCount-dailyTask.CompleteCount)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				messageList = append(messageList, shareMessageList...)
			default:
				logger.Logger.Errorf("未知的社区任务%s", dailyTask.TaskName)
			}
		}
	}

	exchangeMessageList, err := handleCommunityExchange(webToken)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	messageList = append(messageList, exchangeMessageList...)

	userInfo, err := request.CommunityUserInfo(webToken)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	signData, err := request.CommunitySign(webToken)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	messageList = append(messageList, fmt.Sprintf("%s(UID:%d)签到成功，获得%s*%d", userInfo.User.GameNickName, userInfo.User.GameUid, signData.GetItemName, signData.GetItemCount))

	return messageList, nil
}

func handleCommunityTaskView(webToken string, times int64) (messageList []string, err error) {
	var count int64
	topicListData, err := request.CommunityTopicList(webToken, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, topic := range topicListData.List {
		_, err = request.CommunityTopicView(webToken, topic.TopicId)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		messageList = append(messageList, fmt.Sprintf("浏览官方板块主题『%s』", topic.Title))

		count++
		if count == times {
			break
		}
	}

	return messageList, nil
}

func handleCommunityTaskLike(webToken string, times int64) (messageList []string, err error) {
	var count int64
	topicListData, err := request.CommunityTopicList(webToken, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, topic := range topicListData.List {
		if !topic.IsLike {
			//未点赞的直接点赞
			err = request.CommunityTopicLike(webToken, topic.TopicId)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			messageList = append(messageList, fmt.Sprintf("点赞官方板块主题『%s』", topic.Title))
		} else {
			//已点赞的取消点赞再点赞
			err = request.CommunityTopicLike(webToken, topic.TopicId)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			time.Sleep(50 * time.Millisecond)
			err = request.CommunityTopicLike(webToken, topic.TopicId)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			messageList = append(messageList, fmt.Sprintf("取消并再次点赞官方板块主题『%s』", topic.Title))
		}

		count++
		if count == times {
			break
		}
	}

	return messageList, nil
}

func handleCommunityTaskShare(webToken string, times int64) (messageList []string, err error) {
	var count int64
	topicListData, err := request.CommunityTopicList(webToken, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, topic := range topicListData.List {
		err = request.CommunityTopicShare(webToken, topic.TopicId)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		messageList = append(messageList, fmt.Sprintf("转发官方板块主题『%s』", topic.Title))

		count++
		if count == times {
			break
		}
	}

	return messageList, nil
}

func handleCommunityExchange(webToken string) (messageList []string, err error) {
	exchangeList := config.GetExchangeList()
	exchangeMap := make(map[int64]struct{})
	for _, itemId := range exchangeList {
		exchangeMap[itemId] = struct{}{}
	}

	exchangeListData, err := request.CommunityExchangeList(webToken)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	//按价值排序，优先兑换高价值道具
	sort.Slice(exchangeListData.List, func(i, j int) bool {
		return exchangeListData.List[i].UseScore > exchangeListData.List[j].UseScore
	})
	for _, exchangeItem := range exchangeListData.List {
		//如果不在用户设置里，那就跳过
		if _, has := exchangeMap[exchangeItem.ExchangeId]; !has {
			messageList = append(messageList, fmt.Sprintf("用户设置不兑换『%s*%d』", exchangeItem.ItemName, exchangeItem.ItemCount))
			continue
		}
		if exchangeItem.ExchangeCount < exchangeItem.MaxExchangeCount {
			for i := int64(0); i < exchangeItem.MaxExchangeCount-exchangeItem.ExchangeCount; i++ {
				info, err := request.CommunityUserInfo(webToken)
				if err != nil {
					return nil, errors.WithStack(err)
				}

				if info.User.Score >= exchangeItem.UseScore {
					err = request.CommunityExchange(webToken, exchangeItem.ExchangeId)
					if err != nil {
						return nil, errors.WithStack(err)
					}
					messageList = append(messageList, fmt.Sprintf("消耗积分%d，成功兑换『%s*%d』", exchangeItem.UseScore, exchangeItem.ItemName, exchangeItem.ItemCount))
				} else {
					messageList = append(messageList, fmt.Sprintf("积分不足%d，无法兑换『%s*%d』", exchangeItem.UseScore, exchangeItem.ItemName, exchangeItem.ItemCount))
				}
			}
		}
	}

	return messageList, nil
}
