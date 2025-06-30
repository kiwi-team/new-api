package controller

import (
	"context"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/model"
	"one-api/service"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	defaultTime := c.Query("default_time")
	if defaultTime == "" {
		defaultTime = "hour"
	}
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username, defaultTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	defaultTime := c.Query("default_time")
	if defaultTime == "" {
		defaultTime = "hour"
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp, defaultTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

var prevWarningTimeMap = make(map[int]int64)

func WarningUserQuota() {
	if !common.QuotaWarningEnabled {
		return
	}
	webhookUrl := common.OptionMap["FeishuRobotUrl"]
	secret := common.OptionMap["FeishuRobotSecret"]
	for {
		time.Sleep(time.Duration(common.QuotaWarningInterval) * time.Minute)
		now := time.Now().Unix()
		ctx := context.TODO()

		userIds := strings.Split(common.QuotaWarningUserIds, ",")
		for _, userIdStr := range userIds {
			userId, err := strconv.Atoi(userIdStr)
			if err != nil {
				common.LogError(ctx, "error parsing user id: "+err.Error())
				continue
			}
			prevWarningTime, ok := prevWarningTimeMap[userId]
			if !ok {
				prevWarningTimeMap[userId] = time.Now().Add(-time.Duration(common.QuotaWarningInterval) * time.Minute).Unix()
				prevWarningTime = prevWarningTimeMap[userId]
			}
			startTimeStr := time.Unix(prevWarningTime, 0).Format("2006-01-02 15:04:05")
			endTimeStr := time.Unix(now, 0).Format("2006-01-02 15:04:05")
			common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + ":消耗预警轮询开始")

			quota, err := model.GetQuotaByTime(userId, prevWarningTime, now)
			if err != nil {
				common.LogError(ctx, "error getting quota: "+err.Error())
				continue
			}
			dollerQuota := int(float64(quota) / common.QuotaPerUnit)
			if dollerQuota >= common.QuotaWarningThreshold {
				content := fmt.Sprintf("用户 %d 在%s ~ %s 内消耗了 %d 美元额度", userId, startTimeStr, endTimeStr, dollerQuota)
				err = service.SendFeishuNotify(webhookUrl, secret, dto.FeishuNotify{
					MsgType: "text",
					Content: dto.FeishuContent{
						Text: content,
					},
				})
				if err != nil {
					common.LogError(ctx, "error sending webhook notify: "+err.Error())
					continue
				}
				common.SysLog("消耗预警:" + content)
				prevWarningTimeMap[userId] = now
			} else {
				common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + strconv.Itoa(dollerQuota) + ":未超过阈值")
			}
		}
	}
}
