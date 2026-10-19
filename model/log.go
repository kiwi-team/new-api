package model

import (
	"context"
	"database/sql/driver"
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
	"gorm.io/gorm/schema"
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
	RaceResult   string `json:"race_result,omitempty" gorm:"type:varchar(10);default:''"`
	// UpstreamRequestId 记录上游返回的 request id，便于按上游工单追踪。
	UpstreamRequestId string   `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string   `json:"other"`
	Request           string   `json:"request" gorm:"type:text"`
	Response          string   `json:"response" gorm:"type:text"`
	ClientUserId      string   `json:"client_user_id" gorm:"index:idx_client_user_id,default:''"`
	ClientScenairo    string   `json:"client_scenairo" gorm:"index;size:200;default:''"`
	SessionId         string   `json:"session_id" gorm:"index:idx_logs_session_id;size:128;default:''"`
	ProjectName       string   `json:"project_name" gorm:"index;size:100;default:''"`
	PlanId            int      `json:"plan_id" gorm:"default:0;index"`
	Usage             string   `json:"usage" gorm:"type:text"`
	Extra             *LogJSON `json:"extra,omitempty"`
	Header            *LogJSON `json:"header,omitempty"`
}

// LogJSON stores validated JSON as an API string. Preserve jsonb on PostgreSQL;
// other databases use text so the same log schema can be migrated everywhere.
type LogJSON string

func (value *LogJSON) Value() (driver.Value, error) {
	if value == nil {
		return nil, nil
	}
	return string(*value), nil
}

func (value *LogJSON) Scan(src any) error {
	switch src := src.(type) {
	case nil:
		*value = ""
	case string:
		*value = LogJSON(src)
	case []byte:
		*value = LogJSON(src)
	default:
		return fmt.Errorf("unsupported log JSON database value %T", src)
	}
	return nil
}

func (LogJSON) GormDataType() string { return "string" }

func (LogJSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "jsonb"
	case "mysql":
		return "longtext"
	case "clickhouse":
		return "Nullable(String)"
	default:
		return "text"
	}
}

// normalizeJsonbString 把任意字符串规整成可写入 jsonb 列的形态：
// 空串或非法 JSON 返回 nil（落库为 NULL）——jsonb 列会拒绝空串/非法 JSON，
// 若直接写入会导致整条日志 INSERT 失败，这里宽容处理避免丢日志。
// 用于 extra（客户端 extra header 内容）和 header（完整请求头快照）等 jsonb 列。
func normalizeJsonbString(s string) *LogJSON {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var probe any
	if err := common.UnmarshalJsonStr(s, &probe); err != nil {
		return nil
	}
	value := LogJSON(s)
	return &value
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

// signedQuotaForLogType 返回按日志类型归一化符号后的额度。
//
// 退款是「还回去的额度」，与消费记在同一列上，正值会让任何 SUM(quota) 把退款
// 算成又一笔消费，统计只增不减。存成负数后聚合天然抵消，无需每个查询单独排除
// 退款类型。logs 表和 quota_data 表共用这一个符号约定。
//
// 调用方传正数（退了多少）是既有约定，这里统一取反，不要求调用方改。已经是
// 负值的按幂等处理——避免重复取反把退款翻回正数。
func signedQuotaForLogType(logType int, quota int) int {
	if logType == LogTypeRefund && quota > 0 {
		return -quota
	}
	return quota
}

func normalizeRefundQuota(log *Log) {
	if log == nil {
		return
	}
	log.Quota = signedQuotaForLogType(log.Type, log.Quota)
}

func createLog(log *Log) error {
	ensureLogRequestId(log)
	normalizeRefundQuota(log)
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
		logs[i].Other = formatLogOtherJSON(logs[i].Other, logOtherVisibilityUser)
	}
	assignDisplayLogIds(logs, startIdx)
}

// FormatAdminLogs removes root-only diagnostics while retaining operational
// admin_info. Root callers must not pass their results through this formatter.
func FormatAdminLogs(logs []*Log) {
	for i := range logs {
		logs[i].Other = formatLogOtherJSON(logs[i].Other, logOtherVisibilityAdmin)
	}
}

// FormatRootLogs normalizes legacy metadata into the current scoped shape
// without removing root-only diagnostics.
func FormatRootLogs(logs []*Log) {
	for i := range logs {
		logs[i].Other = formatLogOtherJSON(logs[i].Other, logOtherVisibilityRoot)
	}
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

func RecordLog(userId int, logType int, content *common.Message, quotas ...int) {
	quota := 0
	if len(quotas) > 0 {
		quota = quotas[0]
	}
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	other := NewLogOther()
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   other.setContent([]*common.Message{content}),
		Other:     other.JSONString(),
		Quota:     quota,
	}
	err := createLog(log)
	if err != nil {
		common.SysLog(common.LogText("failed to record log: %s", err.Error()))
	}
}

// RecordLogWithAdminInfo stores operator metadata under other.admin_info and
// an optional, user-visible operation descriptor under other.op for localization.
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo *AuditAdminInfo, operation *AuditOperation, request ...*gin.Context) {
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
	if logType == LogTypeManage {
		var c *gin.Context
		if len(request) > 0 {
			c = request[0]
		}
		actorRole := 0
		if c != nil {
			actorRole = c.GetInt("role")
		}
		RecordAuditLog(c, AuditLog{UserId: userId, Username: username, ActorRole: actorRole, Category: AuditCategoryOperation, Content: content, Other: AuditOther{AdminInfo: adminInfo, Op: operation}, Success: true})
		return
	}
	if len(request) > 0 && request[0] != nil {
		log.RequestId = request[0].GetString(common.RequestIdKey)
	}
	if adminInfo != nil || operation != nil {
		data, err := common.Marshal(AuditOther{AdminInfo: adminInfo, Op: operation})
		if err != nil {
			common.SysError(common.LogText("failed to encode log admin info: %s", err.Error()))
			return
		}
		log.Other = string(data)
	}
	if err := createLog(log); err != nil {
		common.SysLog(common.LogText("failed to record log: %s", err.Error()))
	}
}

// RecordLoginLog writes new login events to the independent audit table.
// username 由调用方传入（登录流程已持有用户对象），避免额外的数据库查询。
// content 为英文兜底文本（用于导出）；action+params 供前端本地化渲染。
// other 包含 login_method、user_agent 等结构化信息。
func RecordLoginLog(userId, actorRole int, username string, content string, ip string, action string, params map[string]any, other AuditOther, request ...*gin.Context) {
	other.Op = &AuditOperation{Action: action, Params: params}
	var c *gin.Context
	if len(request) > 0 {
		c = request[0]
	}
	RecordAuditLog(c, AuditLog{UserId: userId, Username: username, ActorRole: actorRole, Category: AuditCategoryLogin, Action: action, Content: content, Ip: ip, Other: other, Success: true})
}

// RecordOperationAuditLog writes new operation/security events to the audit table.
// logUserId 为日志归属者，管理审计日志应归属实际操作者；目标资源/用户放入
// action params。username 内部按 logUserId 查询。content 为英文兜底文本（供导出使用）。
// action+params 写入 Other.op，供前端本地化渲染（普通用户可见，不含敏感信息）。
// adminInfo 存放操作者身份（写入 Other.admin_info，普通用户查询时剥离）；
// auditInfo 存放路由/方法/结果等中间件兜底信息（写入 Other.audit_info，普通用户查询时剥离）。
func RecordOperationAuditLog(logUserId, actorRole int, content string, ip string, action string, params map[string]any, adminInfo *AuditAdminInfo, auditInfo *AuditRequestInfo, request ...*gin.Context) {
	username, _ := GetUsernameById(logUserId, false)
	other := AuditOther{
		Op:        &AuditOperation{Action: action, Params: params},
		AdminInfo: adminInfo,
		AuditInfo: auditInfo,
	}
	var c *gin.Context
	if len(request) > 0 {
		c = request[0]
	}
	category := AuditCategoryOperation
	if adminInfo == nil {
		category = AuditCategorySecurity
	}
	status, success := 200, true
	if auditInfo != nil {
		status = auditInfo.Status
		success = auditInfo.Success
	}
	RecordAuditLog(c, AuditLog{UserId: logUserId, Username: username, ActorRole: actorRole, Category: category, Action: action, Content: content, Ip: ip, Status: status, Success: success, Other: other})
}

func RecordTopupLog(userId int, content *common.Message, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	other := NewLogOther()
	other.MergeAdmin(map[string]any{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	})
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   other.setContent([]*common.Message{content}),
		Ip:        callerIp,
		Other:     other.JSONString(),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog(common.LogText("failed to record topup log: %s", err.Error()))
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, upstreamRequestId string, other *LogOther) {
	logger.LogInfo(c, common.LogText("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	sessionId := common.GetContextKeyString(c, constant.ContextKeyClaudeSessionId)
	otherStr := other.JSONString()
	// 判断是否需要记录 IP
	needRecordIp := true
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
		logger.LogError(c, common.LogText("failed to record log: %s", err.Error()))
	}
}

/*
func RecordConsumeLog(c *gin.Context, userId int, channelId int, promptTokens int, completionTokens int,

	modelName string, tokenName string, quota int, content string, tokenId int, userQuota int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}, requestStr string, responseStr string) {
	common.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, 用户调用前余额=%d, channelId=%d, promptTokens=%d, completionTokens=%d, modelName=%s, tokenName=%s, quota=%d, content=%s", userId, userQuota, channelId, promptTokens, completionTokens, modelName, tokenName, quota, content))
*/
type RecordConsumeLogParams struct {
	ChannelId                   int               `json:"channel_id"`
	PromptTokens                int               `json:"prompt_tokens"`
	CompletionTokens            int               `json:"completion_tokens"`
	CachedTokens                int               `json:"cached_tokens"`
	ClaudeCacheCreation5mTokens int               `json:"claude_cache_creation_5_m_tokens"`
	ClaudeCacheCreation1hTokens int               `json:"claude_cache_creation_1_h_tokens"`
	ModelName                   string            `json:"model_name"`
	TokenName                   string            `json:"token_name"`
	Quota                       int               `json:"quota"`
	Content                     []*common.Message `json:"content"`
	TokenId                     int               `json:"token_id"`
	UseTimeSeconds              int               `json:"use_time_seconds"`
	IsStream                    bool              `json:"is_stream"`
	Group                       string            `json:"group"`
	Other                       *LogOther         `json:"other"`
	Request                     string            `json:"request"`
	Response                    string            `json:"response"`
	ClientUserId                string            `json:"client_user_id"`
	ClientScenairo              string            `json:"client_scenairo"`
	SessionId                   string            `json:"session_id"`
	RequestId                   string            `json:"request_id"`
	ProjectName                 string            `json:"project_name"`
	PlanId                      int               `json:"plan_id"`
	Usage                       string            `json:"usage"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	username := c.GetString("username")
	requestId := params.RequestId
	if requestId == "" {
		requestId = c.GetString(common.RequestIdKey)
	}
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	createdAt := common.GetTimestamp()
	other := params.Other
	content := other.setContent(params.Content)
	otherStr := other.JSONString()
	firstTokenMs := 0
	if params.IsStream && params.Other != nil {
		if firstResponseTime, ok := params.Other.Snapshot()["frt"].(float64); ok && firstResponseTime > 0 {
			firstTokenMs = int(firstResponseTime)
		}
	}
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	// 判断是否需要记录 IP
	clientIp := ""
	if needRecordIp {
		clientIp = c.ClientIP()
	}
	//if settingMap, err := GetUserSetting(userId, false); err == nil {
	//	if settingMap.RecordIpLog {
	//	}
	//}
	log := &Log{
		UserId:            userId,
		Username:          username,
		CreatedAt:         createdAt,
		Type:              LogTypeConsume,
		Content:           content,
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
		RequestId:         requestId,
		RaceResult:        c.GetString(common.KeyChannelRaceResult),
		ProjectName:       params.ProjectName,
		PlanId:            params.PlanId,
		Usage:             params.Usage,
		Extra:             normalizeJsonbString(extra),
		Header:            normalizeJsonbString(header),
		UpstreamRequestId: upstreamRequestId,
	}
	if err := createLog(log); err != nil {
		logger.LogError(c, "failed to record consume log: "+err.Error())
	}
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
	UserId         int
	LogType        int
	Content        []*common.Message
	ChannelId      int
	ModelName      string
	Quota          int
	TokenId        int
	Group          string
	Other          *LogOther
	NodeName       string // 任务发起节点；为空时回退当前节点
	ClientUserId   string
	ClientScenairo string
	ProjectName    string
	PlanId         int
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
	other := params.Other
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: createdAt,
		Type:      params.LogType,
		Content:   other.setContent(params.Content),
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     other.JSONString(),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog(common.LogText("failed to record task billing log: %s", err.Error()))
	}
	// 消费和退款都要进 quota_data:消耗统计页(/quota-statistics)与飞书告警都按
	// SUM(quota) 聚合这张表,只写消费会让退款和差额结算的补扣永远不被冲减,
	// 总消耗只增不减。退款以负值写入,与 logs 表同一套符号约定。
	if (params.LogType == LogTypeConsume || params.LogType == LogTypeRefund) && common.DataExportEnabled {
		nodeName := params.NodeName
		if nodeName == "" {
			nodeName = common.NodeName
		}
		LogQuotaData(&LogQuotaDataCache{
			UserId:         params.UserId,
			Username:       username,
			ModelName:      params.ModelName,
			Quota:          signedQuotaForLogType(params.LogType, params.Quota),
			CreatedAt:      createdAt,
			TokenName:      tokenName,
			TokenId:        params.TokenId,
			ChannelId:      params.ChannelId,
			UseGroup:       params.Group,
			NodeName:       nodeName,
			ClientUserId:   params.ClientUserId,
			ClientScenairo: params.ClientScenairo,
			ProjectName:    params.ProjectName,
			PlanId:         params.PlanId,
			// RecordTaskBillingLog 记的都是异步任务结算后的金额修正,对应的请求在
			// 提交时已经由 LogTaskConsumption 计过一次数,这里不能重复累加。
			IsSettlementAdjustment: true,
		})
	}
}

// LogFilter is the usage-log search shared by the log list and the log export.
// Username and Channel are administrator filters.
type LogFilter struct {
	Type              int
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	Username          string
	TokenName         string
	Channel           int
	Group             string
	RequestId         string
	UpstreamRequestId string
	ClientUserId      string
	MtSessionId       string
	TraceId           string
	TrajId            string
	SessionId         string
	ScopeUIDs         []string // Server-derived authorized UID scope, never read from query parameters.
	Export            bool     // Legacy root-only raw export.
}

func (f LogFilter) apply(tx *gorm.DB) (*gorm.DB, error) {
	var err error
	if f.Type != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", f.Type)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", f.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", f.Username); err != nil {
		return nil, err
	}
	if f.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", f.TokenName)
	}
	if f.RequestId != "" {
		tx = tx.Where("logs.request_id = ?", f.RequestId)
	}
	if f.UpstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", f.UpstreamRequestId)
	}
	if f.StartTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", f.StartTimestamp)
	}
	if f.EndTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", f.EndTimestamp)
	}
	if f.Channel != 0 {
		tx = tx.Where("logs.channel_id = ?", f.Channel)
	}
	if f.Group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", f.Group)
	}
	if f.ClientUserId != "" {
		tx = tx.Where("logs.client_user_id LIKE ?", "%"+f.ClientUserId+"%")
	}
	if f.SessionId != "" {
		tx = tx.Where("logs.session_id = ?", f.SessionId)
	}
	return applyLogExtraFilters(tx, "logs.extra", f.MtSessionId, f.TraceId, f.TrajId), nil
}

// applyLogExtraFilters shares exact JSON-field matching across consume/error logs
// and their exports. The column is supplied by the model, never by the client.
func applyLogExtraFilters(tx *gorm.DB, column, mtSessionID, traceID, trajID string) *gorm.DB {
	for _, field := range []struct{ key, value string }{{"mt_session_id", mtSessionID}, {"trace_id", traceID}, {"traj_id", trajID}} {
		if field.value == "" {
			continue
		}
		var expression string
		switch common.LogDatabaseType() {
		case common.DatabaseTypePostgreSQL:
			expression = column + "->>'" + field.key + "' = ?"
		case common.DatabaseTypeMySQL:
			expression = "JSON_UNQUOTE(JSON_EXTRACT(" + column + ", '$." + field.key + "')) = ?"
		case common.DatabaseTypeClickHouse:
			expression = "JSONExtractString(" + column + ", '" + field.key + "') = ?"
		default:
			expression = "json_extract(" + column + ", '$." + field.key + "') = ?"
		}
		tx = tx.Where(expression, field.value)
	}
	return tx
}

func GetAllLogs(filter LogFilter, startIdx int, num int) (logs []*Log, total int64, err error) {
	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	if filter.Export {
		err = tx.Order("logs.id asc").Find(&logs).Error
	} else {
		err = tx.Omit("request", "response", "usage", "channel_name").Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	}
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
	filter := LogFilter{Type: logType, StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, ModelName: modelName, Username: username, TokenName: tokenName, Channel: channel, Group: group, ClientUserId: clientUserId, RequestId: requestId, MtSessionId: mtSessionId, TraceId: traceId, TrajId: trajId, SessionId: sessionId}
	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}

	err = tx.Omit("request", "response", "usage", "channel_name").Order("logs.id asc").Find(&logs).Error
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

func GetUserLogs(userId int, filter LogFilter, startIdx int, num int) (logs []*Log, total int64, err error) {
	scope := LOG_DB.Where("logs.user_id = ?", userId)
	if len(filter.ScopeUIDs) > 0 {
		scope = LOG_DB.Where("logs.client_user_id IN ?", filter.ScopeUIDs)
	}
	tx, err := filter.apply(scope)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError(common.LogText("failed to count user logs: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to query logs")
	}
	tx = tx.Omit("request", "response", "usage", "channel_name")
	order := "logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	err = tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError(common.LogText("failed to search user logs: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to query logs")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

// StreamAllLogs calls fn with each log matching filter, newest first, as an
// administrator with role sees it. Rows are read one at a time, so an export
// never holds the whole result in memory.
func StreamAllLogs(ctx context.Context, filter LogFilter, role int, fn func(*Log) error) error {
	tx, err := filter.apply(LOG_DB.WithContext(ctx))
	if err != nil {
		return err
	}
	visibility := logOtherVisibilityAdmin
	if role >= common.RoleRootUser {
		visibility = logOtherVisibilityRoot
	}
	channelNames := make(map[int]string)
	return streamLogs(tx, "logs.created_at desc, logs.id desc", func(log *Log) error {
		if log.ChannelId != 0 {
			name, ok := channelNames[log.ChannelId]
			if !ok {
				if channel, err := CacheGetChannel(log.ChannelId); err == nil {
					name = channel.Name
				}
				channelNames[log.ChannelId] = name
			}
			log.ChannelName = name
		}
		log.Other = formatLogOtherJSON(log.Other, visibility)
		return fn(log)
	})
}

// StreamUserLogs is StreamAllLogs for one user's own logs, shaped like the
// user log list.
func StreamUserLogs(ctx context.Context, userId int, filter LogFilter, fn func(*Log) error) error {
	scope := LOG_DB.WithContext(ctx).Where("logs.user_id = ?", userId)
	if len(filter.ScopeUIDs) > 0 {
		scope = LOG_DB.WithContext(ctx).Where("logs.client_user_id IN ?", filter.ScopeUIDs)
	}
	tx, err := filter.apply(scope)
	if err != nil {
		return err
	}
	count := 0
	return streamLogs(tx, "logs.id desc", func(log *Log) error {
		count++
		log.Id = count
		log.ChannelName = ""
		log.Other = formatLogOtherJSON(log.Other, logOtherVisibilityUser)
		return fn(log)
	})
}

func streamLogs(tx *gorm.DB, order string, fn func(*Log) error) error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	rows, err := tx.Model(&Log{}).Order(order).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var log Log
		if err := LOG_DB.ScanRows(rows, &log); err != nil {
			return err
		}
		if err := fn(&log); err != nil {
			return err
		}
	}
	return rows.Err()
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

	tx = tx.Where("type = ? or type=?", LogTypeConsume, LogTypeRefund)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 两个查询必须 Scan 到各自独立的结构体:GORM v2 的 Scan 走 ScanRows(ScanInitialized),
	// 每次都会先把目标 struct 整体清零再写入本次 SELECT 的列。若共用同一个 Stat,
	// 第二次 Scan 会把第一次查出的 Quota 抹成 0(表现为用量恒为 $0)。
	var quotaResult struct {
		Quota int
	}
	if err := tx.Scan(&quotaResult).Error; err != nil {
		common.SysError(common.LogText("failed to query log stat: %s", err.Error()))
		return stat, common.NewMessage("Failed to query statistics")
	}
	var rateStat struct {
		Rpm int
		Tpm int
	}
	if err := rpmTpmQuery.Scan(&rateStat).Error; err != nil {
		common.SysError(common.LogText("failed to query rpm/tpm stat: %s", err.Error()))
		return stat, common.NewMessage("Failed to query statistics")
	}
	stat.Quota = quotaResult.Quota
	stat.Rpm = rateStat.Rpm
	stat.Tpm = rateStat.Tpm

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
