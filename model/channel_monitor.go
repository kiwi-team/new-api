package model

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// 模型渠道监控 (/console/model-channel-monitor) 数据。
// 按 Token(key) × 渠道 × 模型 聚合，时间窗内。
// 成功量/耗时/首字延迟来自 logs(type=Consume)，错误来自 error_logs，
// 渠道名/类型与 Token key 从主库 channels/tokens 查出后内存拼接
// （logs/error_logs 在 LOG_DB，channels/tokens 在 DB，可能不是同一连接，不能跨库 join）。

const channelMonitorBuckets = 24

type ChannelMonitorErrorTop struct {
	Count int64  `json:"count"`
	Last  string `json:"last"`
	Code  string `json:"code"`
	Text  string `json:"text"`
}

type ChannelMonitorRecord struct {
	KeyId         int    `json:"keyId"`
	KeyName       string `json:"keyName"`
	KeyHint       string `json:"keyHint"`
	ChannelId     int    `json:"channelId"`
	ChannelName   string `json:"channelName"`
	ChannelTypeId int    `json:"channelTypeId"`
	ChannelGroup  string `json:"channelGroup"`
	Group         string `json:"group"`
	Model         string `json:"model"`

	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	Samples     int64   `json:"samples"`
	SuccessRate float64 `json:"successRate"`
	Status      string  `json:"status"`

	UseMs    int64 `json:"useMs"`
	P95UseMs int64 `json:"p95UseMs"`
	// 首字延迟仅流式请求有意义；无流式样本时为 null（前端显示"待埋点"）。
	FirstMs    *int64  `json:"firstMs"`
	P95FirstMs *int64  `json:"p95FirstMs"`
	Tps        float64 `json:"tps"`

	LastSuccess string `json:"lastSuccess"`
	LastError   string `json:"lastError"`

	Trend      []int64                  `json:"trend"`
	ErrorMarks []int                    `json:"errorMarks"`
	ErrorsTop  []ChannelMonitorErrorTop `json:"errorsTop"`
}

func channelMonitorKey(tokenId, channelId int, model string) string {
	return fmt.Sprintf("%d\x1f%d\x1f%s", tokenId, channelId, model)
}

// 与前端 getAggregateStatus 阈值保持一致。
func channelMonitorStatus(successRate float64, p95UseMs int64) string {
	if successRate < 90 {
		return "down"
	}
	if successRate < 98 || p95UseMs > 120000 {
		return "degraded"
	}
	return "healthy"
}

// p95FromHist 给定 (值, 计数) 升序切片与总数，返回累计占比首次达到 95% 的值。
func p95FromHist(pairs []struct{ V, C int64 }, total int64) int64 {
	if total <= 0 || len(pairs) == 0 {
		return 0
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].V < pairs[j].V })
	target := int64(math.Ceil(0.95 * float64(total)))
	var cum int64
	for _, p := range pairs {
		cum += p.C
		if cum >= target {
			return p.V
		}
	}
	return pairs[len(pairs)-1].V
}

func maskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key
	}
	return key[:6] + "***" + key[len(key)-4:]
}

// GetChannelMonitor 返回模型渠道监控数据（全局，管理员可见）。
// 模型/状态过滤交给前端，这里返回时间窗内全部聚合组合。
// 仅统计真实业务流量：所有查询都过滤 token_id > 0，排除渠道测试
// （controller/channel-test.go 以 token_id=0、token_name="模型测试" 写入消费日志）。
func GetChannelMonitor(startTime int64, endTime int64) ([]*ChannelMonitorRecord, error) {
	cstZone := time.FixedZone("CST", 8*3600)
	hhmm := func(ts int64) string {
		if ts <= 0 {
			return ""
		}
		return time.Unix(ts, 0).In(cstZone).Format("15:04")
	}

	bucketSize := (endTime - startTime) / channelMonitorBuckets
	if bucketSize < 1 {
		bucketSize = 1
	}

	records := make(map[string]*ChannelMonitorRecord)
	getOrInit := func(tokenId, channelId int, model string) *ChannelMonitorRecord {
		key := channelMonitorKey(tokenId, channelId, model)
		r, ok := records[key]
		if !ok {
			r = &ChannelMonitorRecord{
				KeyId:      tokenId,
				ChannelId:  channelId,
				Model:      model,
				Trend:      make([]int64, channelMonitorBuckets),
				ErrorMarks: make([]int, 0),
				ErrorsTop:  make([]ChannelMonitorErrorTop, 0),
			}
			records[key] = r
		}
		return r
	}

	// 1. 成功聚合（logs, type=Consume）：请求数、总耗时、输出 token、首字延迟、最近成功时间
	type successRow struct {
		TokenId     int    `gorm:"column:token_id"`
		ChannelId   int    `gorm:"column:channel_id"`
		ModelName   string `gorm:"column:model_name"`
		TokenName   string `gorm:"column:token_name"`
		GroupVal    string `gorm:"column:group_val"`
		Requests    int64  `gorm:"column:requests"`
		UseSum      int64  `gorm:"column:use_sum"`
		CompSum     int64  `gorm:"column:comp_sum"`
		LastSuccess int64  `gorm:"column:last_success"`
		FrtSum      int64  `gorm:"column:frt_sum"`
		FrtSamples  int64  `gorm:"column:frt_samples"`
	}
	var successRows []successRow
	successSelect := "token_id, channel_id, model_name, " +
		"max(token_name) as token_name, max(" + commonGroupCol + ") as group_val, " +
		"count(*) as requests, sum(use_time) as use_sum, sum(completion_tokens) as comp_sum, " +
		"max(created_at) as last_success, " +
		"sum(CASE WHEN first_token_ms > 0 THEN first_token_ms ELSE 0 END) as frt_sum, " +
		"sum(CASE WHEN first_token_ms > 0 THEN 1 ELSE 0 END) as frt_samples"
	if err := LOG_DB.Table("logs").
		Select(successSelect).
		Where("type = ? AND token_id > 0 AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTime, endTime).
		Group("token_id, channel_id, model_name").
		Scan(&successRows).Error; err != nil {
		return nil, err
	}
	for _, s := range successRows {
		r := getOrInit(s.TokenId, s.ChannelId, s.ModelName)
		r.KeyName = s.TokenName
		r.Group = s.GroupVal
		r.Requests = s.Requests
		r.LastSuccess = hhmm(s.LastSuccess)
		if s.Requests > 0 {
			r.UseMs = int64(math.Round(float64(s.UseSum) / float64(s.Requests) * 1000))
		}
		if s.UseSum > 0 {
			r.Tps = float64(s.CompSum) / float64(s.UseSum)
		}
		if s.FrtSamples > 0 {
			avg := int64(math.Round(float64(s.FrtSum) / float64(s.FrtSamples)))
			r.FirstMs = &avg
		}
	}

	// 2. 错误聚合（error_logs）：错误数、最近错误时间
	type errorRow struct {
		TokenId   int    `gorm:"column:token_id"`
		ChannelId int    `gorm:"column:channel_id"`
		ModelName string `gorm:"column:model_name"`
		Errors    int64  `gorm:"column:errors"`
		LastError int64  `gorm:"column:last_error"`
	}
	var errorRows []errorRow
	if err := LOG_DB.Table("error_logs").
		Select("token_id, channel_id, model_name, count(*) as errors, max(created_at) as last_error").
		Where("token_id > 0 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("token_id, channel_id, model_name").
		Scan(&errorRows).Error; err != nil {
		return nil, err
	}
	for _, e := range errorRows {
		r := getOrInit(e.TokenId, e.ChannelId, e.ModelName)
		r.Errors = e.Errors
		r.LastError = hhmm(e.LastError)
	}

	// 3. 错误明细 topN（按 code 聚合，每个组合取前 3）
	type errorTopRow struct {
		TokenId    int    `gorm:"column:token_id"`
		ChannelId  int    `gorm:"column:channel_id"`
		ModelName  string `gorm:"column:model_name"`
		Code       string `gorm:"column:code"`
		Total      int64  `gorm:"column:total"`
		Message    string `gorm:"column:message"`
		StatusCode int    `gorm:"column:status_code"`
		Last       int64  `gorm:"column:last"`
	}
	var errorTopRows []errorTopRow
	if err := LOG_DB.Table("error_logs").
		Select("token_id, channel_id, model_name, code, count(*) as total, max(message) as message, max(status_code) as status_code, max(created_at) as last").
		Where("token_id > 0 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("token_id, channel_id, model_name, code").
		Order("total desc").
		Scan(&errorTopRows).Error; err != nil {
		return nil, err
	}
	const errorTopLimit = 3
	for _, e := range errorTopRows {
		key := channelMonitorKey(e.TokenId, e.ChannelId, e.ModelName)
		r, ok := records[key]
		if !ok || len(r.ErrorsTop) >= errorTopLimit {
			continue
		}
		text := e.Message
		if text == "" && e.StatusCode > 0 {
			text = fmt.Sprintf("HTTP %d", e.StatusCode)
		}
		r.ErrorsTop = append(r.ErrorsTop, ChannelMonitorErrorTop{
			Count: e.Total,
			Last:  hhmm(e.Last),
			Code:  e.Code,
			Text:  text,
		})
	}

	// 4. 趋势：成功请求按时间桶取平均耗时（毫秒）
	type trendRow struct {
		TokenId   int     `gorm:"column:token_id"`
		ChannelId int     `gorm:"column:channel_id"`
		ModelName string  `gorm:"column:model_name"`
		Bkt       int     `gorm:"column:bkt"`
		AvgUse    float64 `gorm:"column:avg_use"`
	}
	var trendRows []trendRow
	if err := LOG_DB.Table("logs").
		Select("token_id, channel_id, model_name, FLOOR((created_at - ?) / ?) as bkt, avg(use_time) as avg_use", startTime, bucketSize).
		Where("type = ? AND token_id > 0 AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTime, endTime).
		Group("token_id, channel_id, model_name, bkt").
		Scan(&trendRows).Error; err != nil {
		return nil, err
	}
	for _, t := range trendRows {
		if t.Bkt < 0 || t.Bkt >= channelMonitorBuckets {
			continue
		}
		key := channelMonitorKey(t.TokenId, t.ChannelId, t.ModelName)
		if r, ok := records[key]; ok {
			r.Trend[t.Bkt] = int64(math.Round(t.AvgUse * 1000))
		}
	}

	// 5. 错误标记：错误按时间桶计数，>0 的桶记为 errorMark
	type markRow struct {
		TokenId   int    `gorm:"column:token_id"`
		ChannelId int    `gorm:"column:channel_id"`
		ModelName string `gorm:"column:model_name"`
		Bkt       int    `gorm:"column:bkt"`
		Cnt       int64  `gorm:"column:cnt"`
	}
	var markRows []markRow
	if err := LOG_DB.Table("error_logs").
		Select("token_id, channel_id, model_name, FLOOR((created_at - ?) / ?) as bkt, count(*) as cnt", startTime, bucketSize).
		Where("token_id > 0 AND created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("token_id, channel_id, model_name, bkt").
		Scan(&markRows).Error; err != nil {
		return nil, err
	}
	markSet := make(map[string]map[int]bool)
	for _, m := range markRows {
		if m.Bkt < 0 || m.Bkt >= channelMonitorBuckets || m.Cnt <= 0 {
			continue
		}
		key := channelMonitorKey(m.TokenId, m.ChannelId, m.ModelName)
		if markSet[key] == nil {
			markSet[key] = make(map[int]bool)
		}
		markSet[key][m.Bkt] = true
	}
	for key, buckets := range markSet {
		if r, ok := records[key]; ok {
			marks := make([]int, 0, len(buckets))
			for b := range buckets {
				marks = append(marks, b)
			}
			sort.Ints(marks)
			r.ErrorMarks = marks
		}
	}

	// 6. 首字延迟 P95：按 100ms 直方图分桶（仅流式 first_token_ms>0）
	type frtHistRow struct {
		TokenId   int    `gorm:"column:token_id"`
		ChannelId int    `gorm:"column:channel_id"`
		ModelName string `gorm:"column:model_name"`
		Bkt       int64  `gorm:"column:bkt"`
		Cnt       int64  `gorm:"column:cnt"`
	}
	var frtHistRows []frtHistRow
	if err := LOG_DB.Table("logs").
		Select("token_id, channel_id, model_name, FLOOR(first_token_ms / 100) as bkt, count(*) as cnt").
		Where("type = ? AND first_token_ms > 0 AND token_id > 0 AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTime, endTime).
		Group("token_id, channel_id, model_name, bkt").
		Scan(&frtHistRows).Error; err != nil {
		return nil, err
	}
	frtHist := make(map[string][]struct{ V, C int64 })
	frtTotal := make(map[string]int64)
	for _, h := range frtHistRows {
		key := channelMonitorKey(h.TokenId, h.ChannelId, h.ModelName)
		frtHist[key] = append(frtHist[key], struct{ V, C int64 }{V: h.Bkt*100 + 50, C: h.Cnt})
		frtTotal[key] += h.Cnt
	}
	for key, pairs := range frtHist {
		if r, ok := records[key]; ok {
			p95 := p95FromHist(pairs, frtTotal[key])
			r.P95FirstMs = &p95
		}
	}

	// 7. 总耗时 P95：use_time 为整数秒，直接按秒分布求分位（秒 × 1000 = 毫秒）
	type useHistRow struct {
		TokenId   int    `gorm:"column:token_id"`
		ChannelId int    `gorm:"column:channel_id"`
		ModelName string `gorm:"column:model_name"`
		Sec       int64  `gorm:"column:sec"`
		Cnt       int64  `gorm:"column:cnt"`
	}
	var useHistRows []useHistRow
	if err := LOG_DB.Table("logs").
		Select("token_id, channel_id, model_name, use_time as sec, count(*) as cnt").
		Where("type = ? AND token_id > 0 AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTime, endTime).
		Group("token_id, channel_id, model_name, use_time").
		Scan(&useHistRows).Error; err != nil {
		return nil, err
	}
	useHist := make(map[string][]struct{ V, C int64 })
	useTotal := make(map[string]int64)
	for _, h := range useHistRows {
		key := channelMonitorKey(h.TokenId, h.ChannelId, h.ModelName)
		useHist[key] = append(useHist[key], struct{ V, C int64 }{V: h.Sec, C: h.Cnt})
		useTotal[key] += h.Cnt
	}
	for key, pairs := range useHist {
		if r, ok := records[key]; ok {
			r.P95UseMs = p95FromHist(pairs, useTotal[key]) * 1000
		}
	}

	// 收尾派生 + 补充渠道/Token 元信息
	channelIdSet := make(map[int]bool)
	tokenIdSet := make(map[int]bool)
	for _, r := range records {
		r.Samples = r.Requests + r.Errors
		if r.Samples > 0 {
			r.SuccessRate = float64(r.Requests) / float64(r.Samples) * 100
		}
		r.Status = channelMonitorStatus(r.SuccessRate, r.P95UseMs)
		if r.ChannelId > 0 {
			channelIdSet[r.ChannelId] = true
		}
		if r.KeyId > 0 {
			tokenIdSet[r.KeyId] = true
		}
	}

	// 渠道名/类型（主库 channels）
	if len(channelIdSet) > 0 {
		ids := make([]int, 0, len(channelIdSet))
		for id := range channelIdSet {
			ids = append(ids, id)
		}
		var channels []struct {
			Id    int    `gorm:"column:id"`
			Name  string `gorm:"column:name"`
			Type  int    `gorm:"column:type"`
			Group string `gorm:"column:group_val"`
		}
		if err := DB.Table("channels").Select("id, name, type, "+commonGroupCol+" as group_val").Where("id IN ?", ids).Find(&channels).Error; err == nil {
			nameMap := make(map[int]string)
			typeMap := make(map[int]int)
			groupMap := make(map[int]string)
			for _, ch := range channels {
				nameMap[ch.Id] = ch.Name
				typeMap[ch.Id] = ch.Type
				groupMap[ch.Id] = ch.Group
			}
			for _, r := range records {
				r.ChannelName = nameMap[r.ChannelId]
				r.ChannelTypeId = typeMap[r.ChannelId]
				r.ChannelGroup = groupMap[r.ChannelId]
			}
		}
	}

	// Token key 掩码 + 名称兜底（主库 tokens）
	if len(tokenIdSet) > 0 {
		ids := make([]int, 0, len(tokenIdSet))
		for id := range tokenIdSet {
			ids = append(ids, id)
		}
		var tokens []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
			Key  string `gorm:"column:token_key"`
		}
		if err := DB.Table("tokens").Select("id, name, "+commonKeyCol+" as token_key").Where("id IN ?", ids).Find(&tokens).Error; err == nil {
			nameMap := make(map[int]string)
			hintMap := make(map[int]string)
			for _, t := range tokens {
				nameMap[t.Id] = t.Name
				hintMap[t.Id] = maskTokenKey(t.Key)
			}
			for _, r := range records {
				r.KeyHint = hintMap[r.KeyId]
				if r.KeyName == "" {
					r.KeyName = nameMap[r.KeyId]
				}
			}
		}
	}

	result := make([]*ChannelMonitorRecord, 0, len(records))
	for _, r := range records {
		result = append(result, r)
	}
	// 默认按样本量倒序，便于前端默认展示高流量组合
	sort.Slice(result, func(i, j int) bool { return result[i].Samples > result[j].Samples })
	return result, nil
}
