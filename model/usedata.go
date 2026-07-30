package model

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id                          int    `json:"id"`
	UserID                      int    `json:"user_id" gorm:"index"`
	Username                    string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName                   string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt                   int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed                   int    `json:"token_used" gorm:"default:0"`
	PromptTokens                int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens            int    `json:"completion_tokens" gorm:"default:0"`
	CachedTokens                int    `json:"cached_tokens" gorm:"default:0"`
	ClaudeCacheCreation5mTokens int    `json:"claude_cache_creation_5_m_tokens" gorm:"default:0"`
	ClaudeCacheCreation1hTokens int    `json:"claude_cache_creation_1_h_tokens" gorm:"default:0"`
	// 缓存请求次数（按请求计数，区别于上面的 token 数）：
	// 单次请求只要对应 token 数 > 0 即计一次。Claude 单次请求可能同时写 5m 与 1h 缓存，
	// 因此 5m/1h 两个计数会分别 +1；列表展示的"缓存写请求总数"= 5m + 1h。
	// 显式声明 gorm 列名，避免数字与下划线的映射歧义（参见 QuotaDataStatistics 注释）。
	CacheWrite5mRequestCount int `json:"cache_write_5m_request_count" gorm:"column:cache_write_5m_request_count;default:0"`
	CacheWrite1hRequestCount int `json:"cache_write_1h_request_count" gorm:"column:cache_write_1h_request_count;default:0"`
	CacheReadRequestCount    int `json:"cache_read_request_count" gorm:"column:cache_read_request_count;default:0"`
	// 耗时聚合埋点（写入时累加，避免读取时扫描明细 logs 表）：
	//   StreamRequestCount 有首字测量(frt>0)的流式请求次数，作为平均首字耗时的分母；
	//   FrtSum             上述请求的首字耗时累加（毫秒）；
	//   RequestTimeSum     所有请求的请求耗时累加（毫秒 = use_time 秒 × 1000）。
	// 平均首字耗时 = sum(frt_sum)/sum(stream_request_count)，平均请求耗时 = sum(request_time_sum)/sum(count)。
	StreamRequestCount int    `json:"stream_request_count" gorm:"column:stream_request_count;default:0"`
	FrtSum             int    `json:"frt_sum" gorm:"column:frt_sum;default:0"`
	RequestTimeSum     int    `json:"request_time_sum" gorm:"column:request_time_sum;default:0"`
	TokenName          string `json:"token_name" gorm:"size:64;default:''"`
	Count              int    `json:"count" gorm:"default:0"`
	Quota              int    `json:"quota" gorm:"default:0"`
	TokenId            int    `json:"token_id" gorm:"index"`
	ChannelId          int    `json:"channel_id" gorm:"index"`
	UseGroup           string `json:"use_group" gorm:"index;size:64;default:''"`
	NodeName           string `json:"node_name" gorm:"index;size:64;default:''"`
	ClientUserId       string `json:"client_user_id" gorm:"index;size:200;default:''"`
	ClientScenairo     string `json:"client_scenairo" gorm:"index;size:200;default:''"`
	ProjectName        string `json:"project_name" gorm:"index;size:200;default:''"`
	PlanId             int    `json:"plan_id" gorm:"index;default:0"`
}

type LogQuotaDataCache struct {
	UserId                      int
	Username                    string
	ModelName                   string
	CreatedAt                   int64
	TokenUsed                   int
	PromptTokens                int
	CompletionTokens            int
	CachedTokens                int
	ClaudeCacheCreation5mTokens int
	ClaudeCacheCreation1hTokens int
	Quota                       int
	TokenName                   string
	TokenId                     int
	ChannelId                   int
	UseGroup                    string
	NodeName                    string
	ClientUserId                string
	ClientScenairo              string
	ProjectName                 string
	PlanId                      int
	// 耗时埋点原始值：FirstTokenMs 首字耗时（毫秒，仅流式且测到首字时 >0）；
	// UseTimeSeconds 请求总耗时（秒）。用于累加 stream_request_count / frt_sum / request_time_sum。
	FirstTokenMs   int
	UseTimeSeconds int
}

func UpdateQuotaData() {
	// recover
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("UpdateQuotaData panic: %s\n", r))
		}
	}()
	for {
		if common.DataExportEnabled {
			fmt.Println("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(params *LogQuotaDataCache, createdAt int64) {
	key := fmt.Sprintf("%d-%s-%s-%d-%d-%d-%s-%s-%s-%s-%s-%d", params.UserId, params.Username, params.ModelName, params.TokenId, createdAt, params.ChannelId, params.UseGroup, params.NodeName, params.ClientUserId, params.ClientScenairo, params.ProjectName, params.PlanId)
	// 按请求派生缓存命中次数：本次调用即一次请求，对应 token 数 > 0 则计一次。
	cacheWrite5mReq, cacheWrite1hReq, cacheReadReq := 0, 0, 0
	if params.ClaudeCacheCreation5mTokens > 0 {
		cacheWrite5mReq = 1
	}
	if params.ClaudeCacheCreation1hTokens > 0 {
		cacheWrite1hReq = 1
	}
	if params.CachedTokens > 0 {
		cacheReadReq = 1
	}
	// 耗时埋点：仅当测到首字（firstTokenMs>0，即流式请求）才计入首字样本与首字耗时累加，
	// 保证 frt_sum 与 stream_request_count 样本集一致，平均值不被无首字请求稀释。
	// 请求耗时统计所有请求，以毫秒累加（use_time 为秒，×1000）。
	streamReq, frtSum := 0, 0
	if params.FirstTokenMs > 0 {
		streamReq = 1
		frtSum = params.FirstTokenMs
	}
	requestTimeSum := params.UseTimeSeconds * 1000
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += params.Quota
		quotaData.TokenUsed += params.TokenUsed
		quotaData.PromptTokens += params.PromptTokens
		quotaData.CompletionTokens += params.CompletionTokens
		quotaData.CachedTokens += params.CachedTokens
		quotaData.ClaudeCacheCreation5mTokens += params.ClaudeCacheCreation5mTokens
		quotaData.ClaudeCacheCreation1hTokens += params.ClaudeCacheCreation1hTokens
		quotaData.CacheWrite5mRequestCount += cacheWrite5mReq
		quotaData.CacheWrite1hRequestCount += cacheWrite1hReq
		quotaData.CacheReadRequestCount += cacheReadReq
		quotaData.StreamRequestCount += streamReq
		quotaData.FrtSum += frtSum
		quotaData.RequestTimeSum += requestTimeSum
	} else {
		quotaData = &QuotaData{
			UserID:                      params.UserId,
			Username:                    params.Username,
			ModelName:                   params.ModelName,
			CreatedAt:                   createdAt,
			Count:                       1,
			Quota:                       params.Quota,
			TokenUsed:                   params.TokenUsed,
			TokenName:                   params.TokenName,
			PromptTokens:                params.PromptTokens,
			CompletionTokens:            params.CompletionTokens,
			CachedTokens:                params.CachedTokens,
			ClaudeCacheCreation5mTokens: params.ClaudeCacheCreation5mTokens,
			ClaudeCacheCreation1hTokens: params.ClaudeCacheCreation1hTokens,
			CacheWrite5mRequestCount:    cacheWrite5mReq,
			CacheWrite1hRequestCount:    cacheWrite1hReq,
			CacheReadRequestCount:       cacheReadReq,
			StreamRequestCount:          streamReq,
			FrtSum:                      frtSum,
			RequestTimeSum:              requestTimeSum,
			TokenId:                     params.TokenId,
			ChannelId:                   params.ChannelId,
			UseGroup:                    params.UseGroup,
			NodeName:                    params.NodeName,
			ClientUserId:                params.ClientUserId,
			ClientScenairo:              params.ClientScenairo,
			ProjectName:                 params.ProjectName,
			PlanId:                      params.PlanId,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(logQuotaData *LogQuotaDataCache) {
	// 只精确到小时
	createdAt := logQuotaData.CreatedAt - (logQuotaData.CreatedAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(logQuotaData, createdAt)
}

// RefundQuotaData writes a negative quota entry to offset the original quota_data record
// written at task submission time. Called when an async task fails and quota is refunded.
func RefundQuotaData(task *Task) {
	if task == nil || task.Quota == 0 {
		return
	}
	username, _ := GetUsernameById(task.UserId, false)
	p := task.Properties
	LogQuotaData(&LogQuotaDataCache{
		UserId:         task.UserId,
		Username:       username,
		ModelName:      p.OriginModelName,
		Quota:          -task.Quota,
		CreatedAt:      common.GetTimestamp(),
		TokenId:        p.TokenId,
		TokenName:      p.TokenName,
		ChannelId:      task.ChannelId,
		ClientUserId:   p.ClientUserId,
		ClientScenairo: p.ClientScenairo,
		ProjectName:    p.ProjectName,
		PlanId:         p.PlanId,
	})
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and use_group = ? and node_name = ? and client_user_id = ? and client_scenairo = ? and project_name = ? and plan_id = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.TokenId, quotaData.ChannelId, quotaData.UseGroup, quotaData.NodeName, quotaData.ClientUserId, quotaData.ClientScenairo, quotaData.ProjectName, quotaData.PlanId).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			increaseQuotaData(quotaData)
			_ = IncreaseCliendUserUsedQuota(quotaData.ClientUserId, quotaData.Quota)
		} else {
			DB.Table("quota_data").Create(quotaData)
			_ = IncreaseCliendUserUsedQuota(quotaData.ClientUserId, quotaData.Quota)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据\n", size))
}

type QuotaDataStatistics struct {
	Date            string  `json:"date"`
	ClientUserId    string  `json:"client_user_id"`
	ModelName       string  `json:"model_name"`
	TokenName       string  `json:"token_name"`
	TokenId         int     `json:"token_id"`
	TokenKey        string  `json:"token_key"`
	TotalCount      int64   `json:"total_count"`
	TotalQuota      float64 `json:"total_quota"`
	TotalPrompt     int64   `json:"total_prompt"`
	TotalCompletion int64   `json:"total_completion"`
	// 缓存相关：读缓存 tokens 适用于所有模型，写缓存 tokens 仅 Claude 系列有值
	TotalCachedTokens        int64 `json:"total_cached_tokens"`
	TotalCacheCreationTokens int64 `json:"total_cache_creation_tokens"`
	// Claude 写缓存按 TTL 拆分：5 分钟缓存与 1 小时缓存定价不同（1h 通常更贵），
	// 这里按 tier 单独聚合方便前端分列展示；总和等于 TotalCacheCreationTokens。
	// 显式声明 gorm 列名:GORM 默认会把 `TotalCacheCreation5mTokens` 映射成
	// `total_cache_creation5m_tokens`(数字前不加下划线),但 SELECT 别名是
	// `total_cache_creation_5m_tokens`,对不上会读不到值(token 数显示 0,而费用是 Go 里算的所以正常)。
	TotalCacheCreation5mTokens int64 `json:"total_cache_creation_5m_tokens" gorm:"column:total_cache_creation_5m_tokens"`
	TotalCacheCreation1hTokens int64 `json:"total_cache_creation_1h_tokens" gorm:"column:total_cache_creation_1h_tokens"`
	// 缓存费用为按模型当前倍率估算值（忽略分组倍率、历史倍率变化、阶梯价等），仅供参考
	TotalCacheCost           float64 `json:"total_cache_cost"`
	TotalCacheCreationCost   float64 `json:"total_cache_creation_cost"`
	TotalCacheCreation5mCost float64 `json:"total_cache_creation_5m_cost"`
	TotalCacheCreation1hCost float64 `json:"total_cache_creation_1h_cost"`
	FixedQuota               int     `json:"fixed_quota"`
	TempQuota                int     `json:"temp_quota"`
}

// estimateCacheCostUSD 按模型当前倍率估算缓存费用（美元）。
// 公式与计费逻辑保持一致：cost = tokens * cacheRatio * modelRatio / QuotaPerUnit，
// 但忽略分组倍率（按 1 处理）、历史倍率变化与阶梯价，因此仅为估算值。
// 写缓存按 5m/1h 两个 TTL 拆分返回（ratio_setting 目前仅有一份 createCacheRatio，
// 因此两档使用相同倍率拆分 token 数量；后续若引入 tier 级倍率可在此扩展）。
func estimateCacheCostUSD(modelName string, cachedTokens int64, cacheCreation5mTokens int64, cacheCreation1hTokens int64) (readCostUSD, write5mCostUSD, write1hCostUSD float64) {
	if modelName == "" {
		return
	}
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	if cachedTokens > 0 {
		cacheRatio, _ := ratio_setting.GetCacheRatio(modelName)
		readCostUSD = float64(cachedTokens) * cacheRatio * modelRatio / common.QuotaPerUnit
	}
	if cacheCreation5mTokens > 0 || cacheCreation1hTokens > 0 {
		createCacheRatio, _ := ratio_setting.GetCreateCacheRatio(modelName)
		if cacheCreation5mTokens > 0 {
			write5mCostUSD = float64(cacheCreation5mTokens) * createCacheRatio * modelRatio / common.QuotaPerUnit
		}
		if cacheCreation1hTokens > 0 {
			write1hCostUSD = float64(cacheCreation1hTokens) * createCacheRatio * modelRatio / common.QuotaPerUnit
		}
	}
	return
}

// scopeUserId/scopeUids/scopeUserIds 是数据范围过滤入参，三种用法互斥但可组合:
//
//	scopeUserId > 0:   非管理员自助视图(并集语义) — WHERE (user_id = scopeUserId OR client_user_id IN scopeUids)
//	                   对应:普通用户、mt-leader/member。
//	scopeUserId <= 0 且 scopeUids 非空: 纯 client_user_id 过滤(mt-admin) —
//	                   WHERE client_user_id IN scopeUids
//	scopeUserId <= 0 且 scopeUserIds 非空: 纯 user_id 过滤(wl-admin) —
//	                   WHERE user_id IN scopeUserIds
//
// 系统 admin 三个都给 0/nil/nil,无 scope 过滤(看全局)。
// 详见 org.md 第 4 节。
func GetQuotaDataStatistics(startTime int64, endTime int64, modelName string, clientUserId string, clientScenairos string, expandModels bool, expandDates bool, expandTokens bool, userId int, projectName string, tokenIds []int, scopeUserId int, scopeUids []string, scopeUserIds []int) ([]*QuotaDataStatistics, error) {
	statistics := make([]*QuotaDataStatistics, 0)
	var err error

	// Date logic based on DB type
	dateField := ""
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		dateField = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch', '+8 hours'))"
	} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		dateField = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')"
	} else if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		dateField = "TO_CHAR(TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	} else {
		dateField = "DATE(created_at)"
	}

	// Build select fields
	datePart := "'' as date"
	if expandDates {
		datePart = dateField + " as date"
	}
	modelPart := "'' as model_name"
	if expandModels {
		modelPart = "model_name"
	}
	tokenPart := "'' as token_name, 0 as token_id"
	if expandTokens {
		tokenPart = "MAX(token_name) as token_name, token_id"
	}
	cacheTokenFields := "sum(cached_tokens) as total_cached_tokens, sum(claude_cache_creation5m_tokens + claude_cache_creation1h_tokens) as total_cache_creation_tokens, sum(claude_cache_creation5m_tokens) as total_cache_creation_5m_tokens, sum(claude_cache_creation1h_tokens) as total_cache_creation_1h_tokens"
	selectFields := datePart + ", client_user_id, " + modelPart + ", " + tokenPart + ", sum(count) as total_count, sum(quota) as total_quota, sum(prompt_tokens) as total_prompt, sum(completion_tokens) as total_completion, " + cacheTokenFields

	// 复用相同的筛选条件，主查询与缓存费用拆分查询共用
	applyFilters := func(q *gorm.DB) *gorm.DB {
		q = q.Where("created_at >= ? AND created_at <= ?", startTime, endTime)
		if modelName != "" {
			q = q.Where("model_name = ?", modelName)
		}
		if clientUserId != "" {
			q = q.Where("client_user_id LIKE ?", "%"+clientUserId+"%")
		}
		if projectName != "" {
			q = q.Where("project_name = ?", projectName)
		}
		// 处理多选scenairo筛选
		if clientScenairos != "" {
			scenairoList := strings.Split(clientScenairos, ",")
			// 已知的三个场景（不含空值）
			knownScenairos := []string{"PersonalExperiment", "ReleaseEvaluation", "DailyExternalModelEvaluation"}
			hasOther := false
			hasPersonalExperiment := false
			normalScenairos := make([]string, 0)
			for _, s := range scenairoList {
				s = strings.TrimSpace(s)
				if s == "Other" {
					hasOther = true
				} else if s == "PersonalExperiment" {
					hasPersonalExperiment = true
					normalScenairos = append(normalScenairos, s)
				} else if s != "" {
					normalScenairos = append(normalScenairos, s)
				}
			}

			// 构建查询条件
			var conditions []string
			var args []interface{}

			// 个人实验：包含 PersonalExperiment 和空值
			if hasPersonalExperiment {
				conditions = append(conditions, "(client_scenairo = ? OR client_scenairo = '')")
				args = append(args, "PersonalExperiment")
			}

			// 其他普通场景（发版评测、日常外部模型评测）
			otherNormalScenairos := make([]string, 0)
			for _, s := range normalScenairos {
				if s != "PersonalExperiment" {
					otherNormalScenairos = append(otherNormalScenairos, s)
				}
			}
			if len(otherNormalScenairos) > 0 {
				conditions = append(conditions, "client_scenairo IN ?")
				args = append(args, otherNormalScenairos)
			}

			// "其他"：不在三个已知场景中，且不为空
			if hasOther {
				conditions = append(conditions, "(client_scenairo NOT IN ? AND client_scenairo != '')")
				args = append(args, knownScenairos)
			}

			if len(conditions) > 0 {
				combinedCondition := "(" + strings.Join(conditions, " OR ") + ")"
				q = q.Where(combinedCondition, args...)
			}
		}
		if userId > 0 {
			q = q.Where("user_id = ?", userId)
		}
		// 数据范围过滤(三种互斥模式,见函数文档)
		switch {
		case scopeUserId > 0:
			// 自助视图:本账号 OR 关联 uid(并集)
			if len(scopeUids) > 0 {
				q = q.Where("(user_id = ? OR client_user_id IN ?)", scopeUserId, scopeUids)
			} else {
				q = q.Where("user_id = ?", scopeUserId)
			}
		case len(scopeUids) > 0:
			// mt-admin:纯 client_user_id IN(本 org 全员的 uid 并集)
			q = q.Where("client_user_id IN ?", scopeUids)
		case len(scopeUserIds) > 0:
			// wl-admin:纯 user_id IN(本 org 全员的 user_id 集合)
			q = q.Where("user_id IN ?", scopeUserIds)
		}
		if len(tokenIds) > 0 {
			q = q.Where("token_id IN ?", tokenIds)
		}
		return q
	}

	tx := applyFilters(DB.Model(&QuotaData{})).Select(selectFields)

	// Build group-by clause dynamically
	groupParts := []string{"client_user_id"}
	if expandDates {
		groupParts = append([]string{"date"}, groupParts...)
	}
	if expandModels {
		groupParts = append(groupParts, "model_name")
	}
	if expandTokens {
		groupParts = append(groupParts, "token_id")
	}
	groupClause := strings.Join(groupParts, ", ")

	if expandDates {
		err = tx.Group(groupClause).Order("date DESC").Scan(&statistics).Error
	} else {
		err = tx.Group(groupClause).Scan(&statistics).Error
	}
	if err != nil {
		return statistics, err
	}

	// 估算缓存费用：按模型当前倍率拆分计算。即使主视图未按模型展开，
	// 也按模型粒度聚合后再汇总，保证未展开时也能给出估算费用。
	// 缓存费用是估算值（忽略分组倍率、历史倍率变化、阶梯价等）。
	mainKey := func(date, clientUserIdVal, modelNameVal string, tokenId int) string {
		d := ""
		if expandDates {
			d = date
		}
		m := ""
		if expandModels {
			m = modelNameVal
		}
		t := 0
		if expandTokens {
			t = tokenId
		}
		return fmt.Sprintf("%s\x1f%s\x1f%s\x1f%d", d, clientUserIdVal, m, t)
	}

	type cacheCostRow struct {
		Date                  string `gorm:"column:date"`
		ClientUserId          string `gorm:"column:client_user_id"`
		ModelName             string `gorm:"column:model_name"`
		TokenId               int    `gorm:"column:token_id"`
		CachedTokens          int64  `gorm:"column:cached_tokens"`
		CacheCreation5mTokens int64  `gorm:"column:cache_creation_5m_tokens"`
		CacheCreation1hTokens int64  `gorm:"column:cache_creation_1h_tokens"`
	}
	costDatePart := "'' as date"
	if expandDates {
		costDatePart = dateField + " as date"
	}
	costTokenPart := "0 as token_id"
	if expandTokens {
		costTokenPart = "token_id"
	}
	costSelect := costDatePart + ", client_user_id, model_name, " + costTokenPart + ", sum(cached_tokens) as cached_tokens, sum(claude_cache_creation5m_tokens) as cache_creation_5m_tokens, sum(claude_cache_creation1h_tokens) as cache_creation_1h_tokens"
	// 缓存费用拆分查询始终按模型粒度分组
	costGroupParts := []string{"client_user_id", "model_name"}
	if expandDates {
		costGroupParts = append([]string{"date"}, costGroupParts...)
	}
	if expandTokens {
		costGroupParts = append(costGroupParts, "token_id")
	}
	var costRows []cacheCostRow
	if costErr := applyFilters(DB.Model(&QuotaData{})).
		Select(costSelect).
		Group(strings.Join(costGroupParts, ", ")).
		Scan(&costRows).Error; costErr == nil {
		readCostMap := make(map[string]float64)
		write5mCostMap := make(map[string]float64)
		write1hCostMap := make(map[string]float64)
		for _, r := range costRows {
			read, write5m, write1h := estimateCacheCostUSD(r.ModelName, r.CachedTokens, r.CacheCreation5mTokens, r.CacheCreation1hTokens)
			k := mainKey(r.Date, r.ClientUserId, r.ModelName, r.TokenId)
			readCostMap[k] += read
			write5mCostMap[k] += write5m
			write1hCostMap[k] += write1h
		}
		for _, s := range statistics {
			k := mainKey(s.Date, s.ClientUserId, s.ModelName, s.TokenId)
			s.TotalCacheCost = readCostMap[k]
			s.TotalCacheCreation5mCost = write5mCostMap[k]
			s.TotalCacheCreation1hCost = write1hCostMap[k]
			s.TotalCacheCreationCost = s.TotalCacheCreation5mCost + s.TotalCacheCreation1hCost
		}
	} else {
		common.SysLog("GetQuotaDataStatistics cache cost query error:" + costErr.Error())
	}

	// 批量查询token key
	if expandTokens {
		tokenIdSet := make(map[int]bool)
		for _, s := range statistics {
			if s.TokenId > 0 {
				tokenIdSet[s.TokenId] = true
			}
		}
		if len(tokenIdSet) > 0 {
			ids := make([]int, 0, len(tokenIdSet))
			for id := range tokenIdSet {
				ids = append(ids, id)
			}
			var tokens []struct {
				Id   int    `gorm:"column:id"`
				Key  string `gorm:"column:key"`
				Name string `gorm:"column:name"`
			}
			if err := DB.Table("tokens").Select("id, "+commonKeyCol+", name").Where("id IN ?", ids).Find(&tokens).Error; err == nil {
				keyMap := make(map[int]string)
				nameMap := make(map[int]string)
				for _, t := range tokens {
					keyMap[t.Id] = t.Key
					nameMap[t.Id] = t.Name
				}
				for _, s := range statistics {
					s.TokenKey = "sk-" + keyMap[s.TokenId]
					if name, ok := nameMap[s.TokenId]; ok {
						s.TokenName = name
					}
				}
			}
		}
	}

	for _, s := range statistics {
		s.TotalQuota /= common.QuotaPerUnit
		if !expandModels && s.ClientUserId != "" {
			var cu CliendUserQuota
			_ = DB.Where("client_user_id = ?", s.ClientUserId).First(&cu).Error
			s.FixedQuota = cu.FixedQuota
			s.TempQuota = cu.TempQuota
		}
	}

	if statistics == nil {
		statistics = make([]*QuotaDataStatistics, 0)
	}
	return statistics, err
}

func increaseQuotaData(quotaData *QuotaData) {
	err := DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and use_group = ? and node_name = ? and client_user_id = ? and client_scenairo = ? and project_name = ? and plan_id = ?",
		quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.TokenId, quotaData.ChannelId, quotaData.UseGroup, quotaData.NodeName, quotaData.ClientUserId, quotaData.ClientScenairo, quotaData.ProjectName, quotaData.PlanId).Updates(map[string]interface{}{
		"count":                          gorm.Expr("count + ?", quotaData.Count),
		"quota":                          gorm.Expr("quota + ?", quotaData.Quota),
		"token_used":                     gorm.Expr("token_used + ?", quotaData.TokenUsed),
		"prompt_tokens":                  gorm.Expr("prompt_tokens + ?", quotaData.PromptTokens),
		"completion_tokens":              gorm.Expr("completion_tokens + ?", quotaData.CompletionTokens),
		"cached_tokens":                  gorm.Expr("cached_tokens + ?", quotaData.CachedTokens),
		"claude_cache_creation5m_tokens": gorm.Expr("claude_cache_creation5m_tokens + ?", quotaData.ClaudeCacheCreation5mTokens),
		"claude_cache_creation1h_tokens": gorm.Expr("claude_cache_creation1h_tokens + ?", quotaData.ClaudeCacheCreation1hTokens),
		"cache_write_5m_request_count":   gorm.Expr("cache_write_5m_request_count + ?", quotaData.CacheWrite5mRequestCount),
		"cache_write_1h_request_count":   gorm.Expr("cache_write_1h_request_count + ?", quotaData.CacheWrite1hRequestCount),
		"cache_read_request_count":       gorm.Expr("cache_read_request_count + ?", quotaData.CacheReadRequestCount),
		"stream_request_count":           gorm.Expr("stream_request_count + ?", quotaData.StreamRequestCount),
		"frt_sum":                        gorm.Expr("frt_sum + ?", quotaData.FrtSum),
		"request_time_sum":               gorm.Expr("request_time_sum + ?", quotaData.RequestTimeSum),
	}).Error
	if err != nil {
		common.SysLog("increaseQuotaData error:" + err.Error())
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64, defaultTime string) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	//err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used,sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as  completion_tokens , created_at").Where("created_at >= ? and created_at <= ? and user_id = ?", startTime, endTime, userId).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string, defaultTime string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used,sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as  completion_tokens , created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaByTime(userId int, startTime int64, endTime int64) (int, error) {
	var quota int
	err := DB.Table("quota_data").Select("COALESCE(sum(quota), 0) as quota").Where("created_at >= ? and created_at <= ? and user_id = ?", startTime, endTime, userId).Find(&quota).Error
	return quota, err
}

// GetCachedQuotaByUser 获取内存缓存中尚未落库的用户消耗额度
func GetCachedQuotaByUser(userId int, startTime int64, endTime int64) int {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	var total int
	for _, qd := range CacheQuotaData {
		if qd.UserID == userId && qd.CreatedAt >= startTime && qd.CreatedAt <= endTime {
			total += qd.Quota
		}
	}
	return total
}

// ChannelQuotaStatistics 渠道消耗统计
type ChannelQuotaStatistics struct {
	ChannelId   int     `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	ModelName   string  `json:"model_name"`
	TotalCount  int64   `json:"total_count"`
	TotalQuota  float64 `json:"total_quota"`
}

// GetChannelQuotaStatistics 获取渠道消耗统计数据
func GetChannelQuotaStatistics(startTime int64, endTime int64) ([]*ChannelQuotaStatistics, error) {
	statistics := make([]*ChannelQuotaStatistics, 0)

	// 查询每个渠道下各模型的消耗数据
	err := DB.Table("quota_data").
		Select("channel_id, model_name, sum(count) as total_count, sum(quota) as total_quota").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("channel_id, model_name").
		Order("total_quota DESC").
		Scan(&statistics).Error

	if err != nil {
		return nil, err
	}

	// 获取渠道名称
	channelIds := make([]int, 0)
	channelIdMap := make(map[int]bool)
	for _, s := range statistics {
		if !channelIdMap[s.ChannelId] {
			channelIds = append(channelIds, s.ChannelId)
			channelIdMap[s.ChannelId] = true
		}
	}

	if len(channelIds) > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err == nil {
			channelNameMap := make(map[int]string)
			for _, ch := range channels {
				channelNameMap[ch.Id] = ch.Name
			}
			for _, s := range statistics {
				s.ChannelName = channelNameMap[s.ChannelId]
				// 转换为美元单位
				//s.TotalQuota = s.TotalQuota / common.QuotaPerUnit
			}
		}
	}

	if statistics == nil {
		statistics = make([]*ChannelQuotaStatistics, 0)
	}
	return statistics, nil
}

// ModelUsageAnalysisRow 用量分析页（/console/model-usage-analysis）单行数据，
// 按 日期 + Token + 模型 聚合。
type ModelUsageAnalysisRow struct {
	Date      string `json:"date" gorm:"column:date"`
	TokenId   int    `json:"token_id" gorm:"column:token_id"`
	TokenName string `json:"token_name" gorm:"column:token_name"`
	ModelName string `json:"model_name" gorm:"column:model_name"`
	// 费用（美元）= quota / QuotaPerUnit
	CostUsd float64 `json:"cost_usd" gorm:"column:cost_usd"`
	// 请求次数
	TotalRequests int64 `json:"total_requests" gorm:"column:total_requests"`
	// 缓存写请求总数 = 5m + 1h（同一请求若同时写 5m/1h 会被分别计入）
	CacheWriteRequests   int64 `json:"cache_write_requests" gorm:"-"`
	CacheWrite5mRequests int64 `json:"cache_write_5m_requests" gorm:"column:cache_write_5m_requests"`
	CacheWrite1hRequests int64 `json:"cache_write_1h_requests" gorm:"column:cache_write_1h_requests"`
	CacheReadRequests    int64 `json:"cache_read_requests" gorm:"column:cache_read_requests"`
	// token 数
	CacheWriteTokens int64 `json:"cache_write_tokens" gorm:"column:cache_write_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens" gorm:"column:cache_read_tokens"`
	InputTokens      int64 `json:"input_tokens" gorm:"column:input_tokens"`
	OutputTokens     int64 `json:"output_tokens" gorm:"column:output_tokens"`
	// 以下三列为耗时聚合的求和中间值（来源于 quota_data 的求和），仅用于计算下方均值，不下发前端。
	// frt_sum / request_time_sum 均以毫秒为单位。
	StreamRequests   int64 `json:"-" gorm:"column:stream_request_count"`
	FrtSumMs         int64 `json:"-" gorm:"column:frt_sum"`
	RequestTimeSumMs int64 `json:"-" gorm:"column:request_time_sum"`
	// 平均首字耗时（毫秒，仅统计有首字测量的流式请求）；无样本时为 0。
	AvgFirstTokenMs int64 `json:"avg_first_token_ms" gorm:"-"`
	// 平均请求耗时（毫秒，统计所有请求）。
	AvgUseTimeMs int64 `json:"avg_use_time_ms" gorm:"-"`
}

// GetModelUsageAnalysis 返回用量分析页数据，按 日期 + Token + 模型 聚合。
// userId > 0 时仅统计该用户创建的 key（token）的数据（非 root 自限范围）；
// userId == 0 表示不限用户，返回全量（root 可见）。
func GetModelUsageAnalysis(userId int, startTime int64, endTime int64) ([]*ModelUsageAnalysisRow, error) {
	rows := make([]*ModelUsageAnalysisRow, 0)

	// 日期字段按 +8 时区格式化，与 GetQuotaDataStatistics 保持一致
	dateField := ""
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		dateField = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch', '+8 hours'))"
	} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		dateField = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')"
	} else if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		dateField = "TO_CHAR(TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	} else {
		dateField = "DATE(created_at)"
	}

	selectFields := dateField + " as date, token_id, MAX(token_name) as token_name, model_name, " +
		"sum(quota) as cost_usd, sum(count) as total_requests, " +
		"sum(cache_write_5m_request_count) as cache_write_5m_requests, " +
		"sum(cache_write_1h_request_count) as cache_write_1h_requests, " +
		"sum(cache_read_request_count) as cache_read_requests, " +
		"sum(claude_cache_creation5m_tokens + claude_cache_creation1h_tokens) as cache_write_tokens, " +
		"sum(cached_tokens) as cache_read_tokens, " +
		"sum(prompt_tokens) as input_tokens, sum(completion_tokens) as output_tokens, " +
		"sum(stream_request_count) as stream_request_count, " +
		"sum(frt_sum) as frt_sum, sum(request_time_sum) as request_time_sum"

	db := DB.Model(&QuotaData{}).
		Select(selectFields).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime)
	if userId > 0 {
		// 非 root：仅限当前用户自己创建的 key 的数据
		db = db.Where("user_id = ?", userId)
	}
	err := db.
		Group("date, token_id, model_name").
		Order("date DESC").
		Scan(&rows).Error
	if err != nil {
		return rows, err
	}

	// quota 转换为美元单位；写请求总数 = 5m + 1h；耗时均值由聚合后的求和列计算。
	// 耗时口径（写入时已埋点到 quota_data，避免读取时扫描明细 logs 表）：
	//   - 平均请求耗时(ms) = sum(request_time_sum) / sum(count)（所有请求）
	//   - 平均首字耗时(ms) = sum(frt_sum) / sum(stream_request_count)（仅有首字测量的流式请求）
	// frt_sum 与 request_time_sum 均以毫秒存储，可直接相除得到毫秒均值。
	for _, r := range rows {
		r.CostUsd /= common.QuotaPerUnit
		r.CacheWriteRequests = r.CacheWrite5mRequests + r.CacheWrite1hRequests
		if r.TotalRequests > 0 {
			r.AvgUseTimeMs = int64(math.Round(float64(r.RequestTimeSumMs) / float64(r.TotalRequests)))
		}
		if r.StreamRequests > 0 {
			r.AvgFirstTokenMs = int64(math.Round(float64(r.FrtSumMs) / float64(r.StreamRequests)))
		}
	}

	return rows, nil
}

// GetDistinctProjectNames 获取所有不重复的项目名称
func GetDistinctProjectNames() ([]string, error) {
	var names []string
	err := DB.Model(&Project{}).
		Distinct("project_name").
		Where("project_name != ''").
		Order("project_name").
		Pluck("project_name", &names).Error
	return names, err
}
