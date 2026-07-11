package controller

import (
	"context"
	"encoding/csv"
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

// GetErrorLogHeader 仅 root：拉取 error_logs.header（jsonb）原始 JSON 字符串供前端按需展开。
// 返回结构与 GetLogHeader/GetLogRequest 一致：{ data: { content: "..." } }，NULL 行返回空串。
func GetErrorLogHeader(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的ID",
		})
		return
	}
	var result struct {
		Header *string `gorm:"column:header" json:"header"`
	}
	err = model.LOG_DB.Model(&model.ErrorLog{}).Select("header").Where("id = ?", id).First(&result).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	content := ""
	if result.Header != nil {
		content = *result.Header
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]any{
			"content": content,
		},
	})
}

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
	sessionId := strings.TrimSpace(c.Query("session_id"))
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
		SessionId:    sessionId,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// header 字段仅 root 可见;其他用户(包括 admin)列表里清掉。
	if c.GetInt("role") < common.RoleRootUser {
		for i := range logs {
			logs[i].Header = nil
		}
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

// ExportErrorLogsCSV 仅 root：导出当前筛选条件下的 error_logs 为 CSV。
// 时间范围强制 ≤ 24 小时；使用 encoding/csv 写出，字段内的逗号/引号/换行由其自动转义，避免破坏列结构。
func ExportErrorLogsCSV(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	requestId := c.Query("request_id")
	channel, _ := strconv.Atoi(c.Query("channel"))
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	clientUserId := c.Query("client_user_id")
	// uid 可能含 +，URL 解码后 + 会被还原成空格，这里还原回来
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))

	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "请指定导出时间范围"})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "结束时间必须晚于开始时间"})
		return
	}
	const maxRangeSeconds int64 = 24 * 3600
	if endTimestamp-startTimestamp > maxRangeSeconds {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出时间范围不能超过 24 小时"})
		return
	}

	logs, err := model.GetErrorLogsForExport(&dto.ErrorLogsRequest{
		RequestId:    requestId,
		ChannelId:    channel,
		ModelName:    modelName,
		StartTime:    startTimestamp,
		EndTime:      endTimestamp,
		TokenId:      tokenId,
		ClientUserId: clientUserId,
		MtSessionId:  mtSessionId,
		TraceId:      traceId,
		TrajId:       trajId,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	lc, _ := time.LoadLocation("Asia/Shanghai")
	startStr := time.Unix(startTimestamp, 0).In(lc).Format("20060102-150405")
	endStr := time.Unix(endTimestamp, 0).In(lc).Format("20060102-150405")
	filename := fmt.Sprintf("error-log-%s-%s.csv", startStr, endStr)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// UTF-8 BOM 让 Excel 正确识别中文
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"ID", "CreatedAt", "UserID", "ChannelId", "ChannelName", "TokenId", "TokenName",
		"ModelName", "StatusCode", "Code", "Type", "Message", "Param", "UseTimeMs",
		"RequestId", "IP", "ClientUserId", "ClientScenairo", "Body", "Extra", "Header",
	})
	for _, log := range logs {
		_ = w.Write([]string{
			strconv.Itoa(log.Id),
			time.Unix(log.CreatedAt, 0).In(lc).Format("2006-01-02 15:04:05"),
			strconv.Itoa(log.UserId),
			strconv.Itoa(log.ChannelId),
			log.ChannelName,
			strconv.Itoa(log.TokenId),
			log.TokenName,
			log.ModelName,
			strconv.Itoa(log.StatusCode),
			log.Code,
			log.Type,
			log.Message,
			log.Param,
			strconv.FormatInt(log.UseTimeMs, 10),
			log.RequestId,
			log.Ip,
			log.ClientUserId,
			log.ClientScenairo,
			log.Body,
			deref(log.Extra),
			deref(log.Header),
		})
	}
	w.Flush()
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
			// 从fileName中读取上次预警时间。
			// 文件缺失/为空/内容损坏时，用 now 自愈并立刻落盘，避免 ParseInt("") 失败后
			// 一直 continue 导致预警轮询永久卡死（prevWarningTime 始终为 0）。
			data, err1 := os.ReadFile(fileName)
			if err1 != nil {
				if !os.IsNotExist(err1) {
					logger.LogError(ctx, "error reading file: "+err1.Error())
				}
				prevWarningTime = now
			} else {
				prevWarningTime, err1 = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
				if err1 != nil {
					logger.LogError(ctx, "error parsing file data, reset to now: "+err1.Error())
					prevWarningTime = now
				}
			}
			if writeErr := os.WriteFile(fileName, []byte(strconv.FormatInt(prevWarningTime, 10)), 0644); writeErr != nil {
				logger.LogError(ctx, "error initializing warning file: "+writeErr.Error())
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
