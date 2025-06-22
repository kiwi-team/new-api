package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"os"
	"strconv"
	"strings"
	"time"

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
	p, _ := strconv.Atoi(c.Query("p"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if p < 1 {
		p = 1
	}
	if pageSize < 0 {
		pageSize = common.ItemsPerPage
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	export := c.Query("export") == "true"
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, (p-1)*pageSize, pageSize, channel, group, export)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if export {
		csvData := "ID\tUserID\tCreatedAt\tType\tContent\tUsername\tTokenName\tModelName\tQuota\tPromptTokens\tCompletionTokens\tUseTime\tIsStream\tChannelId\tChannelName\tTokenId\tGroup\tIP\tOther\tRequest\tResponse\n"
		for _, log := range logs {
			requestStr, err := parseUnicodeEscape(log.Request)
			if err != nil {
				common.LogInfo(c, "asdfasdfasf:"+err.Error())
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
				log.Id, log.UserId, time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"), log.Type, log.Content, log.Username, log.TokenName, log.ModelName,
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

func GetUserLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if p < 1 {
		p = 1
	}
	if pageSize < 0 {
		pageSize = common.ItemsPerPage
	}
	if pageSize > 100 {
		pageSize = 100
	}
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, (p-1)*pageSize, pageSize, group)
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
	return
}

func SearchAllLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	logs, err := model.SearchAllLogs(keyword)
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
		"data":    logs,
	})
	return
}

func SearchUserLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	userId := c.GetInt("id")
	logs, err := model.SearchUserLogs(userId, keyword)
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
		"data":    logs,
	})
	return
}

func GetLogByKey(c *gin.Context) {
	key := c.Query("key")
	logs, err := model.GetLogByKey(key)
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
	stat := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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
	quotaNum := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
