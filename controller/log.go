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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	// uid(client_user_id) 可能含 + 等特殊符号，URL 解码后 + 会被还原成空格，这里还原回来
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	requestId := c.Query("request_id")
	// extra 嵌套字段筛选；TrimSpace 防止前端漏掉/用户复制带空白
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))
	sessionId := strings.TrimSpace(c.Query("session_id"))
	isAdmin := isAdmin(c)
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, clientUserId, requestId, upstreamRequestId, mtSessionId, traceId, trajId, sessionId, export, isAdmin)
	//logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group)
	//requestId := c.Query("request_id")
	//logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId)
	//channel, _ := strconv.Atoi(c.Query("channel"))
	//group := c.Query("group")
	//requestId := c.Query("request_id")
	//logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 渠道信息仅 root 可见;其他用户(包括 admin / 组织 admin)无论列表/导出都清掉渠道 ID 与名称,
	// 避免普通 admin 透过响应体或 CSV 看到上游渠道归属。
	if c.GetInt("role") < common.RoleRootUser {
		for i := range logs {
			logs[i].ChannelId = 0
			logs[i].ChannelName = ""
		}
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
	// header 字段仅 root 可见;其他用户(包括 admin / 组织 admin)的列表响应里清掉,
	// 配合既有的 GET /api/log/:id/header(RootAuth) 端点保持一致。
	if c.GetInt("role") < common.RoleRootUser {
		for i := range logs {
			logs[i].Header = nil
		}
	}
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

// GetLogHeader 仅 root：拉取 logs.header（jsonb）原始 JSON 字符串供前端按需展开。
// 返回结构与 GetLogRequest/GetLogResponse 一致：{ data: { content: "..." } }，NULL 行返回空串。
func GetLogHeader(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	var result struct {
		Header *string `gorm:"column:header" json:"header"`
	}
	err := model.LOG_DB.Model(&model.Log{}).Select("header").Where("id = ?", id).First(&result).Error
	if err != nil {
		common.ApiError(c, err)
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

// isAdmin 读取鉴权中间件写入的 role。仪表盘鉴权已迁移到 JWT/PAT（见 middleware.setDashboardAuthContext），
// 不再挂 gin-contrib/sessions 中间件，读 session 会 panic。
func isAdmin(c *gin.Context) bool {
	return c.GetInt("role") >= common.RoleAdminUser
}

func limitUserLogStartTimestamp(role int, startTimestamp, now int64) int64 {
	if role >= common.RoleAdminUser {
		return startTimestamp
	}
	days := common.UserLogQueryLimitDays
	if days < 1 {
		days = common.DefaultUserLogQueryLimitDays
	}
	cutoff := now - int64(days)*24*60*60
	if startTimestamp == 0 || startTimestamp < cutoff {
		return cutoff
	}
	return startTimestamp
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	startTimestamp = limitUserLogStartTimestamp(c.GetInt("role"), startTimestamp, time.Now().Unix())
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	isAdmin := isAdmin(c)
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	// mt 业务筛选(详见 org.md 4.3):mt org 用户在使用日志页用 UID/MT Session/Trace/Traj 收窄数据
	clientUserId := c.Query("client_user_id")
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))
	sessionId := strings.TrimSpace(c.Query("session_id"))
	// 自助视图数据范围:
	//   - mt-admin: 扩展到 mt 全组织成员的 uid + related_uids 并集(详见 org.md 4.2.1)
	//   - 其他用户: 沿用 GetScopeUids(自己 uid + 自己 related_uids)
	//   - 用户没配 uid 时 scopeUids 为空,model.GetUserLogs 兜底为 WHERE user_id = self
	var scopeUids []string
	if u, e := model.GetUserById(userId, false); e == nil {
		if service.HasMtFullOrgScope(u) {
			// mt-admin / mt-leader 都拥有 mt 全员 uid 范围
			scope := service.ComputeOrgScope(u)
			scopeUids = scope.UidSet
		} else {
			scopeUids = u.GetScopeUids()
		}
	}
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, isAdmin, requestId, upstreamRequestId, scopeUids, clientUserId, mtSessionId, traceId, trajId, sessionId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// header 与渠道信息仅 root 可见;其他用户(自助视图)的列表响应里清掉。
	// ChannelName 已由 formatUserLogs 清空,这里补清 ChannelId(json:"channel"),避免渠道 ID 泄露。
	if c.GetInt("role") < common.RoleRootUser {
		for i := range logs {
			logs[i].Header = nil
			logs[i].ChannelId = 0
			logs[i].ChannelName = ""
		}
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
	// admin 端按 username 老路径,传 nil 让 model 走原 username 过滤
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, nil)
	//stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	startTimestamp = limitUserLogStartTimestamp(c.GetInt("role"), startTimestamp, time.Now().Unix())
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")

	// mt org:把统计范围从 username 切换到 client_user_id IN scope。
	// 严格语义(见 org.md 4.2.1):
	//   - mt-admin: mt 全组织成员 uid + related_uids 并集
	//   - mt-leader: 自己 uid + 自己 related_uids
	//   - mt-member: 仅自己 uid(不含 related_uids)
	// 其他 org / 系统 admin:scopeUids 留 nil,走 username 老路径,行为不变。
	var scopeUids []string
	if u, e := model.GetUserById(userId, false); e == nil && u != nil {
		switch {
		case service.HasMtFullOrgScope(u):
			// mt-admin / mt-leader 都拥有 mt 全员 uid 范围
			scopeUids = service.ComputeOrgScope(u).UidSet
		case u.OrgCode == "mt" && u.OrgRole == constant.OrgRoleMember:
			// 严格:仅自己 uid;若 uid 为空,scopeUids = [](非 nil 空切片) → model 早返 0/0/0
			if u.Uid != "" {
				scopeUids = []string{u.Uid}
			} else {
				scopeUids = []string{}
			}
		case u.OrgCode == "mt":
			// mt org 未识别角色:兜底走自己 GetScopeUids
			scopeUids = u.GetScopeUids()
		}
	}

	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, scopeUids)
	//quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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
	// uid(client_user_id) 可能含 +，URL 解码后 + 会被还原成空格，这里还原回来
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	requestId := c.Query("request_id")
	mtSessionId := strings.TrimSpace(c.Query("mt_session_id"))
	traceId := strings.TrimSpace(c.Query("trace_id"))
	trajId := strings.TrimSpace(c.Query("traj_id"))
	sessionId := strings.TrimSpace(c.Query("session_id"))

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

	logs, err := model.GetLogsForExport(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, clientUserId, requestId, mtSessionId, traceId, trajId, sessionId)
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

// DeleteHistoryLogs is the legacy synchronous log cleanup endpoint (DELETE /api/log/).
// It deletes directly instead of going through the async system task. It is kept only
// for the classic frontend; the default frontend uses POST /api/system-task/log-cleanup.
// TODO: remove this handler (and its route) once the classic frontend is removed.
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
