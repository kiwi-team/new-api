package controller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetErrorLogBody 获取错误日志的body字段
func GetErrorLogBody(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的ID",
		})
		return
	}
	body, err := model.GetErrorLogBody(id)
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
		"data":    body,
	})
}

func GetAllErrorLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if p < 1 {
		p = 1
	}
	if pageSize < 0 {
		pageSize = common.ItemsPerPage
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	requestId := c.Query("request_id")
	channel, _ := strconv.Atoi(c.Query("channel"))
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	clientUserId := c.Query("client_user_id")
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	// extra 嵌套字段筛选；TrimSpace 防止前端漏掉或用户复制时带空白
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))
	logs, total, err := model.GetAllErrorLog(&dto.ErrorLogsRequest{
		RequestId:    requestId,
		ChannelId:    channel,
		ModelName:    modelName,
		StartTime:    startTimestamp,
		EndTime:      endTimestamp,
		Page:         p,
		PageSize:     pageSize,
		TokenId:      tokenId,
		ClientUserId: clientUserId,
		MtSessionId:  mtSessionId,
		TraceId:      traceId,
		TrajId:       trajId,
	})
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
		"data": map[string]any{
			"items":     logs,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
		},
	})
}

var prevWarningTime int64

func WarningErrorLog() {
	if !common.ErrorWarningEnabled {
		return
	}
	webhookUrl := common.OptionMap["ErrorWarningFeishuRobotUrl"]
	secret := common.OptionMap["ErrorWarningFeishuRobotSecret"]
	envName := common.OptionMap["ErrorWarningEnvName"]
	interval := common.OptionMap["ErrorWarningInterval"]
	intervalInt, _ := strconv.Atoi(interval)
	for {
		time.Sleep(time.Duration(intervalInt) * time.Minute)
		now := time.Now().Unix()
		ctx := context.TODO()

		fileName := "error_warning.txt"
		if prevWarningTime == 0 {
			// 从fileName中读取上次预警时间
			data, err1 := os.ReadFile(fileName)
			if err1 != nil {
				logger.LogError(ctx, "error reading file: "+err1.Error())
				continue
			}
			prevWarningTime, err1 = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
			if err1 != nil {
				logger.LogError(ctx, "error parsing file data: "+err1.Error())
				continue
			}
		}
		loc, _ := time.LoadLocation("Asia/Shanghai")
		startTimeStr := time.Unix(prevWarningTime, 0).In(loc).Format("2006-01-02 15:04:05")
		endTimeStr := time.Unix(now, 0).In(loc).Format("2006-01-02 15:04:05")
		common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + ":errorlog预警轮询开始")

		statistics := model.StatisticsErrorLog(prevWarningTime, now)
		if len(statistics) > 0 {
			content := getErrorLogStatisticsContent(statistics)
			content = envName + "\n" + "[" + startTimeStr + "~" + endTimeStr + "]\n" + content
			err := service.SendFeishuNotify(webhookUrl, secret, dto.FeishuNotify{
				MsgType: "text",
				Content: dto.FeishuContent{
					Text: content,
				},
			})
			if err != nil {
				logger.LogError(ctx, "error sending webhook notify: "+err.Error())
				continue
			}
		}
		prevWarningTime = now
		// 将 now 的值写入文件
		err := os.WriteFile(fileName, []byte(strconv.FormatInt(now, 10)), 0644)
		if err != nil {
			logger.LogError(ctx, "error writing now to file: "+err.Error())
		}
		common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + ":errorlog预警轮询结束")
	}
}

func getErrorLogStatisticsContent(statistics []model.ErrorLogStatistics) string {
	var content strings.Builder
	content.WriteString("错误日志统计分析：\n")
	for _, item := range statistics {
		content.WriteString(fmt.Sprintf("渠道(%d)：%s，模型：%s，错误次数：%d, StatusCode：%d, 错误码：%s，错误信息：%s\n", item.ChannelId, item.ChannelName, item.ModelName, item.Total, item.StatusCode, item.Code, item.Message))
	}
	return content.String()
}

func DeleteErrorLogs() {
	keepDaysStr := common.OptionMap["error_log_keep_days"]
	if keepDaysStr == "" {
		keepDaysStr = "15"
	}
	keepDaysInt, _ := strconv.Atoi(keepDaysStr)
	deleteSizeStr := common.OptionMap["error_log_delete_size"]
	if deleteSizeStr == "" {
		deleteSizeStr = "10000"
	}
	deleteSizeInt, _ := strconv.Atoi(deleteSizeStr)
	deleteErrorLogInterval := common.OptionMap["error_log_delete_interval"]
	if deleteErrorLogInterval == "" {
		deleteErrorLogInterval = "10"
	}
	deleteErrorLogIntervalInt, _ := strconv.Atoi(deleteErrorLogInterval)
	for {
		err := model.DeleteErrorLog(time.Now().Unix()-int64(keepDaysInt)*24*60*60, deleteSizeInt)
		if err != nil {
			logger.LogError(context.Background(), "error deleting error log: "+err.Error())
		} else {
			common.SysLog("delete error log success,num:" + deleteSizeStr)
		}
		time.Sleep(time.Duration(deleteErrorLogIntervalInt) * time.Minute)
	}
}
