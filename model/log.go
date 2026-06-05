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
	Id               int     `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId           int     `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt        int64   `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int     `json:"type" gorm:"index:idx_created_at_type"`
	Content          string  `json:"content"`
	Username         string  `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string  `json:"token_name" gorm:"index;default:''"`
	ModelName        string  `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int     `json:"quota" gorm:"default:0"`
	PromptTokens     int     `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int     `json:"completion_tokens" gorm:"default:0"`
	UseTime          int     `json:"use_time" gorm:"default:0"`
	IsStream         bool    `json:"is_stream"`
	ChannelId        int     `json:"channel" gorm:"index"`
	ChannelName      string  `json:"channel_name" gorm:"->"`
	TokenId          int     `json:"token_id" gorm:"default:0;index"`
	Group            string  `json:"group" gorm:"index"`
	Ip               string  `json:"ip" gorm:"index;default:''"`
	RequestId        string  `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	Other            string  `json:"other"`
	Request          string  `json:"request" gorm:"type:text"`
	Response         string  `json:"response" gorm:"type:text"`
	ClientUserId     string  `json:"client_user_id" gorm:"index:idx_client_user_id,default:''"`
	ClientScenairo   string  `json:"client_scenairo" gorm:"index;size:200;default:''"`
	ProjectName      string  `json:"project_name" gorm:"index;size:100;default:''"`
	PlanId           int     `json:"plan_id" gorm:"default:0;index"`
	Usage            string  `json:"usage" gorm:"type:text"`
	Extra            *string `json:"extra,omitempty" gorm:"type:jsonb"`
	Header           *string `json:"header,omitempty" gorm:"type:jsonb"`
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

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			delete(otherMap, "reject_reason")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
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
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := true
	// if settingMap, err := GetUserSetting(userId, false); err == nil {
	// 	if settingMap.RecordIpLog {
	// 		needRecordIp = true
	// 	}
	// }
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
		RequestId: requestId,
		Other:     otherStr,
		Extra:     normalizeJsonbString(extra),
		Header:    normalizeJsonbString(header),
	}
	err := LOG_DB.Create(log).Error
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
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	// 判断是否需要记录 IP
	clientIp := c.ClientIP()
	//if settingMap, err := GetUserSetting(userId, false); err == nil {
	//	if settingMap.RecordIpLog {
	//	}
	//}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip:               clientIp,
		Other:            otherStr,
		Request:          strings.TrimSpace(params.Request),
		Response:         strings.TrimSpace(params.Response),
		ClientUserId:     params.ClientUserId,
		ClientScenairo:   params.ClientScenairo,
		RequestId:        params.RequestId,
		ProjectName:      params.ProjectName,
		PlanId:           params.PlanId,
		Usage:            params.Usage,
		Extra:            normalizeJsonbString(extra),
		Header:           normalizeJsonbString(header),
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
				ClientUserId:                params.ClientUserId,
				ClientScenairo:              params.ClientScenairo,
				ProjectName:                 params.ProjectName,
				PlanId:                      params.PlanId,
			})
			//LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, clientUserId string, requestId string, mtSessionId string, traceId string, trajId string, export bool, isAdmin bool) (logs []*Log, total int64, err error) {
	//func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string) (logs []*Log, total int64, err error) {
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
			return logs, total, err
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
func GetLogsForExport(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, clientUserId string, requestId string, mtSessionId string, traceId string, trajId string) (logs []*Log, err error) {
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

// scopeUids 为当前用户关联的 uid(client_user_id) 集合（自身 uid + related_uids，去重去空白）。
// 行为：
//   - scopeUids 为空（用户未配置 uid）：按账号自身过滤，WHERE logs.user_id = userId。
//   - scopeUids 非空（用户配置了 uid）：切换为按 client_user_id 过滤，
//     WHERE logs.client_user_id IN scopeUids。本账号 client_user_id 不在该集合
//     的日志（包括未带 uid header 的请求）将不可见，这是预期语义。
func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, isAdmin bool, requestId string, scopeUids []string) (logs []*Log, total int64, err error) {
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

	if modelName != "" {
		modelNamePattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return nil, 0, err
		}
		tx = tx.Where("logs.model_name LIKE ? ESCAPE '!'", modelNamePattern)
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
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	tx = tx.Omit("request", "response", "usage")
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
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

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	if username != "" {
		tx = tx.Where("username = ?", username)
		rpmTpmQuery = rpmTpmQuery.Where("username = ?", username)
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
	if modelName != "" {
		modelNamePattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return stat, err
		}
		tx = tx.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
		rpmTpmQuery = rpmTpmQuery.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
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
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
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

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
