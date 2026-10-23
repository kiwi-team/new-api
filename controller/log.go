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
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

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

// logFilterFromQuery reads the log search shared by the log list and the log
// export. Username and channel are read only for administrators.
func logFilterFromQuery(c *gin.Context, admin bool) model.LogFilter {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	filter := model.LogFilter{
		Type:              logType,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		ModelName:         c.Query("model_name"),
		TokenName:         c.Query("token_name"),
		Group:             c.Query("group"),
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
		ClientUserId:      strings.ReplaceAll(c.Query("client_user_id"), " ", "+"),
		MtSessionId:       strings.TrimSpace(c.Query("mt_session_id")),
		TraceId:           strings.TrimSpace(c.Query("trace_id")),
		TrajId:            strings.TrimSpace(c.Query("traj_id")),
		SessionId:         strings.TrimSpace(c.Query("session_id")),
	}
	if admin {
		filter.Username = c.Query("username")
		filter.Channel, _ = strconv.Atoi(c.Query("channel"))
	} else if user, err := model.GetUserById(c.GetInt("id"), false); err == nil {
		if service.HasMtFullOrgScope(user) {
			filter.ScopeUIDs = service.ComputeOrgScope(user).UidSet
		} else {
			filter.ScopeUIDs = user.GetScopeUids()
		}
	}
	return filter
}

// limitLogTimeRange returns the start of a log or dashboard search kept within
// the longest log time range. A missing start reaches back the whole range, and
// time after now does not count toward it.
func limitLogTimeRange(startTimestamp int64, endTimestamp int64) (int64, error) {
	maxRange := operation_setting.GetLogSetting().MaxRangeSeconds()
	end := common.GetTimestamp()
	if endTimestamp != 0 {
		end = min(end, endTimestamp)
	}
	if startTimestamp == 0 {
		return end - maxRange, nil
	}
	if end-startTimestamp > maxRange {
		return 0, common.NewMessage("The time range cannot exceed {{days}} days", map[string]any{"days": maxRange / (24 * 60 * 60)})
	}
	return startTimestamp, nil
}

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	export := c.Query("export") == "true"
	if export && c.GetInt("role") < common.RoleRootUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	filter := logFilterFromQuery(c, true)
	filter.Export = export
	logs, total, err := model.GetAllLogs(filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 渠道、重试链与映射后的实际模型仅 root 可见；列表和导出响应都执行同一套清理，
	// 避免普通 admin 直接从响应体或 CSV 的 Other 字段读取这些内部路由信息。
	sanitizeLogChannelInfoForRole(logs, c.GetInt("role"))
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
	if c.GetInt("role") < common.RoleRootUser {
		model.FormatAdminLogs(logs)
	} else {
		model.FormatRootLogs(logs)
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

// sanitizeLogChannelInfoForRole removes root-only upstream routing details from
// responses returned to every lower role while preserving unrelated admin_info
// diagnostics that those roles are otherwise allowed to inspect.
func sanitizeLogChannelInfoForRole(logs []*model.Log, role int) {
	if role >= common.RoleRootUser {
		return
	}

	for _, log := range logs {
		if log == nil {
			continue
		}
		log.ChannelId = 0
		log.ChannelName = ""

		other, _ := common.StrToMap(log.Other)
		if other == nil {
			continue
		}
		delete(other, "is_model_mapped")
		delete(other, "upstream_model_name")
		adminInfo, ok := other["admin_info"].(map[string]interface{})
		if !ok {
			log.Other = common.MapToJsonStr(other)
			continue
		}
		delete(adminInfo, "use_channel")
		delete(adminInfo, "use_channel_time")
		delete(adminInfo, "is_model_mapped")
		delete(adminInfo, "upstream_model_name")
		if len(adminInfo) == 0 {
			delete(other, "admin_info")
		}
		log.Other = common.MapToJsonStr(other)
	}
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
	filter := logFilterFromQuery(c, false)
	if c.GetInt("role") < common.RoleAdminUser {
		startTimestamp, err := limitLogTimeRange(filter.StartTimestamp, filter.EndTimestamp)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filter.StartTimestamp = startTimestamp
	}
	filter.StartTimestamp = limitUserLogStartTimestamp(c.GetInt("role"), filter.StartTimestamp, time.Now().Unix())
	logs, total, err := model.GetUserLogs(c.GetInt("id"), filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// header、渠道信息与映射后的实际模型仅 root 可见。
	if c.GetInt("role") < common.RoleRootUser {
		for i := range logs {
			logs[i].Header = nil
		}
		sanitizeLogChannelInfoForRole(logs, c.GetInt("role"))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// ExportAllLogs streams every user's logs matching the search.
func ExportAllLogs(c *gin.Context) {
	exportLogs(c, true)
}

// ExportUserLogs streams the caller's own logs matching the search.
func ExportUserLogs(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser && !operation_setting.GetLogSetting().UserExportEnabled {
		common.ApiErrorT(c, "Log export is disabled")
		return
	}
	exportLogs(c, false)
}

// exportLogs writes one JSON log per line, newest first, and ends with an API
// response object: success with the log count, or the failure. Each user runs
// one export at a time, and every export stays within the longest log time
// range.
func exportLogs(c *gin.Context, all bool) {
	filter := logFilterFromQuery(c, all)
	startTimestamp, err := limitLogTimeRange(filter.StartTimestamp, filter.EndTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	filter.StartTimestamp = startTimestamp
	if !all {
		filter.StartTimestamp = limitUserLogStartTimestamp(c.GetInt("role"), filter.StartTimestamp, time.Now().Unix())
	}

	userId := c.GetInt("id")
	ctx, release, ok, err := service.AcquireLogExportLock(c.Request.Context(), userId)
	if err != nil {
		common.SysError(common.LogText("failed to lock log export: %s", err.Error()))
		common.ApiErrorT(c, "Failed to export logs")
		return
	}
	if !ok {
		common.ApiErrorT(c, "Another log export is running. Try again after it finishes.")
		return
	}
	defer release()

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	count := 0
	writeLog := func(log *model.Log) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Streaming exports obey the same raw-payload and routing visibility as lists.
		if c.GetInt("role") < common.RoleRootUser {
			log.Request, log.Response, log.Usage = "", "", ""
			log.Header = nil
			sanitizeLogChannelInfoForRole([]*model.Log{log}, c.GetInt("role"))
		}
		line, err := common.Marshal(log)
		if err != nil {
			return err
		}
		if _, err := c.Writer.Write(append(line, '\n')); err != nil {
			return err
		}
		count++
		return nil
	}
	if all {
		err = model.StreamAllLogs(ctx, filter, c.GetInt("role"), writeLog)
	} else {
		err = model.StreamUserLogs(ctx, userId, filter, writeLog)
	}
	if ctx.Err() != nil {
		return
	}
	// The status is already sent, so the result object becomes the last line.
	if err != nil {
		common.SysError(common.LogText("failed to export logs: %s", err.Error()))
		common.ApiErrorT(c, "Failed to export logs")
		return
	}
	common.ApiSuccess(c, gin.H{"count": count})
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	common.ApiErrorT(c, "This API is deprecated")
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	common.ApiErrorT(c, "This API is deprecated")
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenInvalid)
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
	// Token 鉴权接口没有 root 用户身份语义，绝不返回内部实际模型或渠道路由信息。
	sanitizeLogChannelInfoForRole(logs, common.RoleCommonUser)
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
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if c.GetInt("role") < common.RoleAdminUser {
		var err error
		if startTimestamp, err = limitLogTimeRange(startTimestamp, endTimestamp); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	startTimestamp = limitUserLogStartTimestamp(c.GetInt("role"), startTimestamp, time.Now().Unix())
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
