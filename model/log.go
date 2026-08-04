package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId           int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	// FirstTokenMs 首字延迟（毫秒），来源于 other.frt，仅流式请求有意义（非流式为 0）。
	// 单独成列以便监控面板做跨库聚合/分位数，避免解析 other JSON。
	FirstTokenMs int    `json:"first_token_ms" gorm:"default:0"`
	IsStream     bool   `json:"is_stream"`
	ChannelId    int    `json:"channel" gorm:"index"`
	ChannelName  string `json:"channel_name" gorm:"->"`
	TokenId      int    `json:"token_id" gorm:"default:0;index"`
	Group        string `json:"group" gorm:"index"`
	Ip           string `json:"ip" gorm:"index;default:''"`
	RequestId    string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	// UpstreamRequestId 记录上游返回的 request id，便于按上游工单追踪。
	UpstreamRequestId string  `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string  `json:"other"`
	Request           string  `json:"request" gorm:"type:text"`
	Response          string  `json:"response" gorm:"type:text"`
	ClientUserId      string  `json:"client_user_id" gorm:"index:idx_client_user_id,default:''"`
	ClientScenairo    string  `json:"client_scenairo" gorm:"index;size:200;default:''"`
	SessionId         string  `json:"session_id" gorm:"index:idx_logs_session_id;size:128;default:''"`
	ProjectName       string  `json:"project_name" gorm:"index;size:100;default:''"`
	PlanId            int     `json:"plan_id" gorm:"default:0;index"`
	Usage             string  `json:"usage" gorm:"type:text"`
	Extra             *string `json:"extra,omitempty" gorm:"type:jsonb"`
	Header            *string `json:"header,omitempty" gorm:"type:jsonb"`
}

// normalizeJsonbString 把任意字符串规整成可写入 jsonb 列的形态：
// 空串或非法 JSON 返回 nil（落库为 NULL）——jsonb 列会拒绝空串/非法 JSON，
// 若直接写入会导致整条日志 INSERT 失败，这里宽容处理避免丢日志。
// 用于 extra（客户端 extra header 内容）和 header（完整请求头快照）等 jsonb 列。
func normalizeJsonbString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var probe any
	if err := common.UnmarshalJsonStr(s, &probe); err != nil {
		return nil
	}
	return &s
}

// sanitizeLogBody 规整 Request/Response 等直接透传上游原始字节的大字段：
//  1. 去除首尾空白；
//  2. 替换非法 UTF-8 字节序列、剔除 NUL 字节（复用 sanitizeForPGText）。
//
// 流式响应由 StreamResponseRecorder 原样抄录上游字节（不做任何编码校验），
// 一旦流在多字节字符（如汉字）中途被切断（客户端取消、上游断连、超时），
// 录制到的字符串就会残留孤立的续字节序列；NUL 字节虽是合法 UTF-8 但 PG 的
// TEXT 列同样拒绝。二者都会触发 PostgreSQL 22021、让整条日志写入失败
// （MySQL/SQLite 宽容处理，故该问题仅在 PG 环境暴露）。这里统一兜底，
// 保证三库都能落库、且不因个别非法字节丢弃整条日志。
// 字符串本就合法时 strings.ToValidUTF8 原样返回、零分配，正常日志无额外开销。
func sanitizeLogBody(s string) string {
	return sanitizeForPGText(strings.TrimSpace(s))
}

func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		condition, pattern, err := buildLogLikeCondition(column, value)
		if err != nil {
			return nil, err
		}
		return tx.Where(condition, pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

func buildLogLikeCondition(column string, value string) (string, string, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		pattern, err := sanitizeClickHouseLikePattern(value)
		if err != nil {
			return "", "", err
		}
		return column + " LIKE ?", pattern, nil
	}

	pattern, err := sanitizeLikePattern(value)
	if err != nil {
		return "", "", err
	}
	return column + " LIKE ? ESCAPE '!'", pattern, nil
}

func sanitizeClickHouseLikePattern(input string) (string, error) {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `_`, `\_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}
	return input, nil
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
	LogTypeLogin   = 7
)

func ensureLogRequestId(log *Log) {
	if log != nil && log.RequestId == "" {
		log.RequestId = common.NewRequestId()
	}
}

func createLog(log *Log) error {
	ensureLogRequestId(log)
	return LOG_DB.Create(log).Error
}

func clickHouseLogOrder(prefix string) string {
	return prefix + "created_at desc, " + prefix + "request_id desc"
}

func assignDisplayLogIds(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].Id = startIdx + i + 1
	}
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// Remove operation-audit details (operator/route info), admin-only.
			delete(otherMap, "audit_info")
			// delete(otherMap, "reject_reason")
			// delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
	}
	assignDisplayLogIds(logs, startIdx)
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	order := "id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order(order).Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string, quota int) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
		Quota:     quota,
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// buildOpField 构建语言无关的操作描述（写入 Other.op）。
// 前端依据 action(稳定操作标识) + params(结构化参数) 在渲染期用 i18n 本地化展示，
// 因此不在数据库中存储自然语言句子。
func buildOpField(action string, params map[string]interface{}) map[string]interface{} {
	op := map[string]interface{}{
		"action": action,
	}
	if len(params) > 0 {
		op["params"] = params
	}
	return op
}

// RecordLoginLog 记录用户登录成功的审计日志（type=LogTypeLogin）。
// username 由调用方传入（登录流程已持有用户对象），避免额外的数据库查询。
// content 为英文兜底文本（用于导出）；action+params 供前端本地化渲染。
// extra 可携带 login_method、user_agent 等附加信息（普通用户可见）。
func RecordLoginLog(userId int, username string, content string, ip string, action string, params map[string]interface{}, extra map[string]interface{}) {
	other := map[string]interface{}{}
	for k, v := range extra {
		other[k] = v
	}
	other["op"] = buildOpField(action, params)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeLogin,
		Content:   content,
		Ip:        ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record login log: " + err.Error())
	}
}

// RecordOperationAuditLog 记录管理/高危操作审计日志（type=LogTypeManage）。
// logUserId 为日志归属者，管理审计日志应归属实际操作者；目标资源/用户放入
// action params。username 内部按 logUserId 查询。content 为英文兜底文本（供导出使用）。
// action+params 写入 Other.op，供前端本地化渲染（普通用户可见，不含敏感信息）。
// adminInfo 存放操作者身份（写入 Other.admin_info，普通用户查询时剥离）；
// auditInfo 存放路由/方法/结果等中间件兜底信息（写入 Other.audit_info，普通用户查询时剥离）。
func RecordOperationAuditLog(logUserId int, content string, ip string, action string, params map[string]interface{}, adminInfo map[string]interface{}, auditInfo map[string]interface{}) {
	username, _ := GetUsernameById(logUserId, false)
	other := map[string]interface{}{
		"op": buildOpField(action, params),
	}
	if len(adminInfo) > 0 {
		other["admin_info"] = adminInfo
	}
	if len(auditInfo) > 0 {
		other["audit_info"] = auditInfo
	}
	log := &Log{
		UserId:    logUserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeManage,
		Content:   content,
		Ip:        ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record operation audit log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	sessionId := common.GetContextKeyString(c, constant.ContextKeyClaudeSessionId)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := true
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		Other:             otherStr,
		SessionId:         sessionId,
		UpstreamRequestId: upstreamRequestId,
		Extra:             normalizeJsonbString(extra),
		Header:            normalizeJsonbString(header),
	}
	err := createLog(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

/*
func RecordConsumeLog(c *gin.Context, userId int, channelId int, promptTokens int, completionTokens int,

	modelName string, tokenName string, quota int, content string, tokenId int, userQuota int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}, requestStr string, responseStr string) {
	common.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, 用户调用前余额=%d, channelId=%d, promptTokens=%d, completionTokens=%d, modelName=%s, tokenName=%s, quota=%d, content=%s", userId, userQuota, channelId, promptTokens, completionTokens, modelName, tokenName, quota, content))
*/
type RecordConsumeLogParams struct {
	ChannelId                   int                    `json:"channel_id"`
	PromptTokens                int                    `json:"prompt_tokens"`
	CompletionTokens            int                    `json:"completion_tokens"`
	CachedTokens                int                    `json:"cached_tokens"`
	ClaudeCacheCreation5mTokens int                    `json:"claude_cache_creation_5_m_tokens"`
	ClaudeCacheCreation1hTokens int                    `json:"claude_cache_creation_1_h_tokens"`
	ModelName                   string                 `json:"model_name"`
	TokenName                   string                 `json:"token_name"`
	Quota                       int                    `json:"quota"`
	Content                     string                 `json:"content"`
	TokenId                     int                    `json:"token_id"`
	UseTimeSeconds              int                    `json:"use_time_seconds"`
	IsStream                    bool                   `json:"is_stream"`
	Group                       string                 `json:"group"`
	Other                       map[string]interface{} `json:"other"`
	Request                     string                 `json:"request"`
	Response                    string                 `json:"response"`
	ClientUserId                string                 `json:"client_user_id"`
	ClientScenairo              string                 `json:"client_scenairo"`
	SessionId                   string                 `json:"session_id"`
	RequestId                   string                 `json:"request_id"`
	ProjectName                 string                 `json:"project_name"`
	PlanId                      int                    `json:"plan_id"`
	Usage                       string                 `json:"usage"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(params.Other)
	// 首字延迟单独落列：frt 已在 other 中算好（毫秒），仅流式请求有意义。
	firstTokenMs := 0
	if params.IsStream && params.Other != nil {
		if frt, ok := params.Other["frt"].(float64); ok && frt > 0 {
			firstTokenMs = int(frt)
		}
	}
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	// 判断是否需要记录 IP
	clientIp := c.ClientIP()
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	//if settingMap, err := GetUserSetting(userId, false); err == nil {
	//	if settingMap.RecordIpLog {
	//	}
	//}
	log := &Log{
		UserId:            userId,
		Username:          username,
		CreatedAt:         common.GetTimestamp(),
		Type:              LogTypeConsume,
		Content:           params.Content,
		PromptTokens:      params.PromptTokens,
		CompletionTokens:  params.CompletionTokens,
		TokenName:         params.TokenName,
		ModelName:         params.ModelName,
		Quota:             params.Quota,
		ChannelId:         params.ChannelId,
		TokenId:           params.TokenId,
		UseTime:           params.UseTimeSeconds,
		FirstTokenMs:      firstTokenMs,
		IsStream:          params.IsStream,
		Group:             params.Group,
		Ip:                clientIp,
		Other:             otherStr,
		Request:           sanitizeLogBody(params.Request),
		Response:          sanitizeLogBody(params.Response),
		ClientUserId:      params.ClientUserId,
		ClientScenairo:    params.ClientScenairo,
		SessionId:         params.SessionId,
		RequestId:         params.RequestId,
		ProjectName:       params.ProjectName,
		PlanId:            params.PlanId,
		Usage:             params.Usage,
		Extra:             normalizeJsonbString(extra),
		Header:            normalizeJsonbString(header),
		UpstreamRequestId: upstreamRequestId,
	}
	// 异步写入日志，避免大请求体（如 base64 图片）阻塞请求响应
	gopool.Go(func() {
		err := LOG_DB.Create(log).Error
		if err != nil {
			common.SysError("failed to record consume log: " + err.Error())
		}
	})
	if !common.DataExportEnabled {
		common.SysLog(fmt.Sprintf("[DIAG] DataExportEnabled=false, skipping LogQuotaData for model=%s", params.ModelName))
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(&LogQuotaDataCache{
				UserId:                      userId,
				Username:                    username,
				ModelName:                   params.ModelName,
				Quota:                       params.Quota,
				CreatedAt:                   common.GetTimestamp(),
				TokenUsed:                   params.PromptTokens + params.CompletionTokens,
				TokenName:                   params.TokenName,
				PromptTokens:                params.PromptTokens,
				CompletionTokens:            params.CompletionTokens,
				CachedTokens:                params.CachedTokens,
				ClaudeCacheCreation5mTokens: params.ClaudeCacheCreation5mTokens,
				ClaudeCacheCreation1hTokens: params.ClaudeCacheCreation1hTokens,
				ChannelId:                   params.ChannelId,
				TokenId:                     params.TokenId,
				UseGroup:                    params.Group,
				NodeName:                    common.NodeName,
				ClientUserId:                params.ClientUserId,
				ClientScenairo:              params.ClientScenairo,
				ProjectName:                 params.ProjectName,
				PlanId:                      params.PlanId,
				FirstTokenMs:                firstTokenMs,
				UseTimeSeconds:              params.UseTimeSeconds,
			})
			//LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
	NodeName  string // 任务发起节点；为空时回退当前节点
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	createdAt := common.GetTimestamp()
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: createdAt,
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
	if params.LogType == LogTypeConsume && common.DataExportEnabled {
		nodeName := params.NodeName
		if nodeName == "" {
			nodeName = common.NodeName
		}
		LogQuotaData(&LogQuotaDataCache{
			UserId:    params.UserId,
			Username:  username,
			ModelName: params.ModelName,
			Quota:     params.Quota,
			CreatedAt: createdAt,
			TokenName: tokenName,
			TokenId:   params.TokenId,
			ChannelId: params.ChannelId,
			UseGroup:  params.Group,
			NodeName:  nodeName,
		})
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, clientUserId string, requestId string, upstreamRequestId string, mtSessionId string, traceId string, trajId string, sessionId string, export bool, isAdmin bool) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	if clientUserId != "" {
		tx = tx.Where("logs.client_user_id LIKE ?", "%"+clientUserId+"%")
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	// extra 是 PG jsonb 列，按嵌套字段精确匹配（NULL 行天然不命中，符合预期）
	if mtSessionId != "" {
		tx = tx.Where("logs.extra->>'mt_session_id' = ?", mtSessionId)
	}
	if traceId != "" {
		tx = tx.Where("logs.extra->>'trace_id' = ?", traceId)
	}
	if trajId != "" {
		tx = tx.Where("logs.extra->>'traj_id' = ?", trajId)
	}
	if sessionId != "" {
		tx = tx.Where("logs.session_id = ?", sessionId)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	if !export {
		tx = tx.Omit("request", "response", "usage")
	}
	if export {
		//err = tx.Order("logs.id asc").Omit("request", "response").Find(&logs).Error
		err = tx.Order("logs.id asc").Find(&logs).Error
	} else {
		err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	}
	if err != nil {
		return nil, 0, err
	}
	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	err = tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		assignDisplayLogIds(logs, startIdx)
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

// GetLogsForExport 导出日志专用查询：按筛选条件返回日志（不分页），且不携带 request/response/usage 大字段。
func GetLogsForExport(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, clientUserId string, requestId string, mtSessionId string, traceId string, trajId string, sessionId string) (logs []*Log, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}
	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if username != "" {
		tx = tx.Where("logs.username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	if clientUserId != "" {
		tx = tx.Where("logs.client_user_id LIKE ?", "%"+clientUserId+"%")
	}
	if mtSessionId != "" {
		tx = tx.Where("logs.extra->>'mt_session_id' = ?", mtSessionId)
	}
	if traceId != "" {
		tx = tx.Where("logs.extra->>'trace_id' = ?", traceId)
	}
	if trajId != "" {
		tx = tx.Where("logs.extra->>'traj_id' = ?", trajId)
	}
	if sessionId != "" {
		tx = tx.Where("logs.session_id = ?", sessionId)
	}

	err = tx.Omit("request", "response", "usage").Order("logs.id asc").Find(&logs).Error
	if err != nil {
		return nil, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}
	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, ch := range channels {
			channelMap[ch.Id] = ch.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, nil
}

const logSearchCountLimit = 10000

// scopeUids 为当前用户关联的 uid(client_user_id) 集合（自身 uid + related_uids，去重去空白）。
// 行为：
//   - scopeUids 为空（用户未配置 uid）：按账号自身过滤，WHERE logs.user_id = userId。
//   - scopeUids 非空（用户配置了 uid）：切换为按 client_user_id 过滤，
//     WHERE logs.client_user_id IN scopeUids。本账号 client_user_id 不在该集合
//     的日志（包括未带 uid header 的请求）将不可见，这是预期语义。
//
// 额外 4 个 mt 业务筛选(在 scopeUids 命中范围内进一步收窄)：
//   - clientUserId  — LIKE 模糊匹配 client_user_id (UID 模糊)
//   - mtSessionId / traceId / trajId — 按 extra jsonb 嵌套字段精确匹配
//
// 这些是空串时不施加额外 WHERE,无 org 上下文用户传空也不影响。
func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, isAdmin bool, requestId string, upstreamRequestId string, scopeUids []string, clientUserId string, mtSessionId string, traceId string, trajId string, sessionId string) (logs []*Log, total int64, err error) {
	const logSearchCountLimit = 10000

	var tx *gorm.DB
	if len(scopeUids) > 0 {
		tx = LOG_DB.Where("logs.client_user_id IN ?", scopeUids)
	} else {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	}
	if logType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	// mt 业务筛选:在 scopeUids 之内进一步收窄
	if clientUserId != "" {
		tx = tx.Where("logs.client_user_id LIKE ?", "%"+clientUserId+"%")
	}
	if mtSessionId != "" {
		tx = tx.Where("logs.extra->>'mt_session_id' = ?", mtSessionId)
	}
	if traceId != "" {
		tx = tx.Where("logs.extra->>'trace_id' = ?", traceId)
	}
	if trajId != "" {
		tx = tx.Where("logs.extra->>'traj_id' = ?", trajId)
	}
	if sessionId != "" {
		tx = tx.Where("logs.session_id = ?", sessionId)
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	tx = tx.Omit("request", "response", "usage")
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	order := "logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	err = tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

func SearchAllLogs(keyword string, isAdmin bool) (logs []*Log, err error) {
	var tx *gorm.DB
	tx = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%")
	if !isAdmin {
		tx = tx.Omit("request", "response", "usage")
	}
	err = tx.Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string, isAdmin bool) (logs []*Log, err error) {
	var tx *gorm.DB
	tx = LOG_DB.Where("user_id = ? and type = ?", userId, keyword)
	if !isAdmin {
		tx = tx.Omit("request", "response", "usage")
	}
	err = tx.Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

// SumUsedQuota 汇总消费 quota / rpm / tpm。
//
// scopeUids 控制范围(详见 org.md 4.2.1):
//   - nil         → 用 username 过滤(其他 org / 系统 admin 走老路径,与现有行为完全一致)
//   - 空切片(非 nil 但 len=0) → mt org 用户但 scope 为空(比如 mt-admin 但 org 内无人配 uid,
//     或 mt-member 自己 uid 是空串),直接返回 Stat{} 不查 DB
//   - 非空切片  → mt org 用户,改为 WHERE client_user_id IN scopeUids(取代 username 过滤)
func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, scopeUids []string) (stat Stat, err error) {
	// 空 scope 早返 0/0/0,避免 IN () 语法错误也避免误命中 NULL/空 client_user_id
	if scopeUids != nil && len(scopeUids) == 0 {
		return Stat{}, nil
	}

	tx := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	if scopeUids != nil {
		// mt org:按 client_user_id 聚合(与使用日志列表的过滤维度一致)
		tx = tx.Where("client_user_id IN ?", scopeUids)
		rpmTpmQuery = rpmTpmQuery.Where("client_user_id IN ?", scopeUids)
	} else if username != "" {
		// 其他 org / 系统 admin:沿用 username 过滤
		if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
			return stat, err
		}
		if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "username", username); err != nil {
			return stat, err
		}
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

// func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
func CountOldLog(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	if err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("created_at < ?", targetTimestamp).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func DeleteOldLogBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if nil != ctx.Err() {
		return 0, ctx.Err()
	}

	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse DELETE is a heavy mutation that rewrites data parts, so
		// per-batch mutations would be pathologically slow. Remove all matching
		// rows in a single synchronous mutation regardless of limit; the reported
		// count lets the caller's progress loop complete in one pass.
		total, err := CountOldLog(ctx, targetTimestamp)
		if err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE logs DELETE WHERE created_at < ? SETTINGS mutations_sync = 1",
			targetTimestamp,
		).Error; err != nil {
			return 0, err
		}
		return total, nil
	}

	result := LOG_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
	if nil != result.Error {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
