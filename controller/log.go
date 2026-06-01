package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"

	"github.com/gin-gonic/gin"
)

// 解析 \uXXXX 转义序列
func parseUnicodeEscape(s string) (string, error) {
	var buf strings.Builder
	for i := 0; i < len(s); {
		if i+6 <= len(s) && s[i] == '\\' && s[i+1] == 'u' {
			hex := s[i+2 : i+6]
			code, err := strconv.ParseInt(hex, 16, 32)
			if err != nil {
				return "", err
			}
			buf.WriteRune(rune(code))
			i += 6
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String(), nil
}

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	export := c.Query("export") == "true"
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	clientUserId := c.Query("client_user_id")
	requestId := c.Query("request_id")
	// extra 嵌套字段筛选；TrimSpace 防止前端漏掉/用户复制带空白
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))
	isAdmin := isAdmin(c)
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, clientUserId, requestId, mtSessionId, traceId, trajId, export, isAdmin)
	//logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group)
	//requestId := c.Query("request_id")
	//logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if export {
		csvData := "ID\tUserID\tCreatedAt\tType\tContent\tUsername\tTokenName\tModelName\tQuota\tPromptTokens\tCompletionTokens\tUseTime\tIsStream\tChannelId\tChannelName\tTokenId\tGroup\tIP\tOther\tRequest\tResponse\n"
		lc, _ := time.LoadLocation("Asia/Shanghai")
		for _, log := range logs {

			requestStr, err := parseUnicodeEscape(log.Request)
			if err != nil {
				requestStr = log.Request
			}
			requestStr = strings.ReplaceAll(requestStr, "\t", "\\t")
			requestStr = strings.ReplaceAll(requestStr, "\n", "\\n")

			responseStr, err := parseUnicodeEscape(log.Response)
			if err != nil {
				responseStr = log.Response
			}
			responseStr = strings.ReplaceAll(responseStr, "\t", "\\t")
			responseStr = strings.ReplaceAll(responseStr, "\n", "\\n")
			csvData += fmt.Sprintf("%d\t%d\t%s\t%d\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%t\t%d\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				log.Id, log.UserId, time.Unix(log.CreatedAt, 0).In(lc).Format("2006-01-02 15:04:05"), log.Type, log.Content, log.Username, log.TokenName, log.ModelName,
				log.Quota, log.PromptTokens, log.CompletionTokens, log.UseTime, log.IsStream, log.ChannelId, log.ChannelName, log.TokenId, log.Group, log.Ip, log.Other, requestStr, responseStr)
		}

		now := time.Now()
		filename := fmt.Sprintf("%s_logs.csv", now.Format("2006-01-02_15-04-05"))
		filePath := fmt.Sprintf("web/dist/assets/%s", filename)

		// 写入文件
		err = os.WriteFile(filePath, []byte(csvData), 0644)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to create CSV file: " + err.Error(),
			})
			return
		}

		// 返回文件URL
		baseURL := os.Getenv("FRONTEND_BASE_URL")
		fileURL := fmt.Sprintf("%s/assets/%s", baseURL, filename)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "CSV file created successfully",
			"data": map[string]any{
				"url":      fileURL,
				"filename": filename,
			},
		})
		return
	}
	// c.JSON(http.StatusOK, gin.H{
	// 	"success": true,
	// 	"message": "",
	// 	"data": map[string]any{
	// 		"items":     logs,
	// 		"total":     total,
	// 		"page":      p,
	// 		"page_size": pageSize,
	// 	},
	// })
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetLogRequest(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	var result struct {
		Request string `gorm:"column:request" json:"request"`
	}
	err := model.LOG_DB.Model(&model.Log{}).Select("request").Where("id = ?", id).First(&result).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]any{
			"content": result.Request,
		},
	})
}

func GetLogResponse(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	var result struct {
		Response string `gorm:"column:response" json:"response"`
	}
	err := model.LOG_DB.Model(&model.Log{}).Select("response").Where("id = ?", id).First(&result).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]any{
			"content": result.Response,
		},
	})
}

func isAdmin(c *gin.Context) bool {
	session := sessions.Default(c)
	role := session.Get("role")
	return role.(int) >= common.RoleAdminUser
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	isAdmin := isAdmin(c)
	//logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, isAdmin)
	requestId := c.Query("request_id")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, isAdmin, requestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	isAdmin := isAdmin(c)
	keyword := c.Query("keyword")
	_, err := model.SearchAllLogs(keyword, isAdmin)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	userId := c.GetInt("id")
	isAdmin := isAdmin(c)
	_, err := model.SearchUserLogs(userId, keyword, isAdmin)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

// ExportLogsCSV 仅 root 可调用：按筛选条件导出日志为 CSV，时间范围最大 7 天，不包含 request/response 字段。
func ExportLogsCSV(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	clientUserId := c.Query("client_user_id")
	requestId := c.Query("request_id")
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))

	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请指定导出时间范围",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "结束时间必须晚于开始时间",
		})
		return
	}
	const maxRangeSeconds int64 = 7 * 24 * 3600
	if endTimestamp-startTimestamp > maxRangeSeconds {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "导出时间范围不能超过 7 天",
		})
		return
	}

	logs, err := model.GetLogsForExport(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, clientUserId, requestId, mtSessionId, traceId, trajId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	lc, _ := time.LoadLocation("Asia/Shanghai")
	startStr := time.Unix(startTimestamp, 0).In(lc).Format("20060102-150405")
	endStr := time.Unix(endTimestamp, 0).In(lc).Format("20060102-150405")
	filename := fmt.Sprintf("log-%s-%s.csv", startStr, endStr)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// UTF-8 BOM 让 Excel 正确识别中文
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"ID", "UserID", "CreatedAt", "Type", "Content", "Username", "TokenName",
		"ModelName", "Quota", "PromptTokens", "CompletionTokens", "UseTime",
		"IsStream", "ChannelId", "ChannelName", "TokenId", "Group", "IP",
		"RequestId", "ClientUserId", "ClientScenairo", "ProjectName", "PlanId",
		"Other",
	})
	for _, log := range logs {
		_ = w.Write([]string{
			strconv.Itoa(log.Id),
			strconv.Itoa(log.UserId),
			time.Unix(log.CreatedAt, 0).In(lc).Format("2006-01-02 15:04:05"),
			strconv.Itoa(log.Type),
			log.Content,
			log.Username,
			log.TokenName,
			log.ModelName,
			strconv.Itoa(log.Quota),
			strconv.Itoa(log.PromptTokens),
			strconv.Itoa(log.CompletionTokens),
			strconv.Itoa(log.UseTime),
			strconv.FormatBool(log.IsStream),
			strconv.Itoa(log.ChannelId),
			log.ChannelName,
			strconv.Itoa(log.TokenId),
			log.Group,
			log.Ip,
			log.RequestId,
			log.ClientUserId,
			log.ClientScenairo,
			log.ProjectName,
			strconv.Itoa(log.PlanId),
			log.Other,
		})
	}
	w.Flush()
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
