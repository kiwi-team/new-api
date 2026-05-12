package model

import (
	"github.com/QuantumNous/new-api/common"

	searchCommon "github.com/QuantumNous/new-api/search/common"
)

type AiSearchLog struct {
	Id               uint   `gorm:"primaryKey" json:"id"`
	SearchType       string `json:"search_type"` // search / images / reader
	Engine           string `json:"engine"`      // serper / jina
	TokenId          int    `json:"token_id"`
	UserId           int    `json:"user_id"`
	ChannelId        int    `json:"channel_id"`
	SearchParameters string `json:"search_parameters" gorm:"default:''"`
	KnowledgeGraph   string `json:"knowledge_graph" gorm:"default:''"`
	Organic          string `json:"organic" gorm:"default:''"`
	PeopleAlsoAsk    string `json:"people_also_ask" gorm:"default:''"`
	RelatedSearches  string `json:"related_searches" gorm:"default:''"`
	Credits          int    `json:"credits"`
	UseTimeSeconds   int64  `json:"use_time_seconds" gorm:"default:0"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_ai_search_created_at"`
	ClientUserId     string `json:"client_user_id" gorm:"default:'';index:idx_ai_search_client_user_id"`

	// Reader 类引擎使用的扩展字段。serper 等搜索类引擎留空即可。
	TargetURL     string `json:"target_url" gorm:"type:varchar(2048);default:''"`
	ContentTokens int    `json:"content_tokens" gorm:"default:0"`
	Content       string `json:"content" gorm:"type:text"`
	Extra         string `json:"extra" gorm:"type:text;default:''"`
}

// SearchResultLogger 统一不同引擎响应到 AiSearchLog 的映射。
// 新增引擎只需让自己的 result 类型实现此接口，handler 不再做类型断言。
type SearchResultLogger interface {
	ToSearchLog(info searchCommon.SearchInfo) *AiSearchLog
}

// ============ serper ============

type SerperSearchResult struct {
	Credits         int `json:"credits"`
	KnowledgeGraph  any `json:"knowledge_graph"`
	Organic         any `json:"organic"`
	PeopleAlsoAsk   any `json:"people_also_ask"`
	RelatedSearches any `json:"related_searches"`
}

func (r SerperSearchResult) ToSearchLog(info searchCommon.SearchInfo) *AiSearchLog {
	return &AiSearchLog{
		Credits:          r.Credits,
		KnowledgeGraph:   common.JsonStringify(r.KnowledgeGraph),
		Organic:          common.JsonStringify(r.Organic),
		PeopleAlsoAsk:    common.JsonStringify(r.PeopleAlsoAsk),
		RelatedSearches:  common.JsonStringify(r.RelatedSearches),
		SearchParameters: common.JsonStringify(info.SearchParameters),
		UseTimeSeconds:   info.UseTimeSeconds,
		SearchType:       info.SearchType,
		Engine:           info.Engine,
		TokenId:          info.TokenId,
		UserId:           info.UserId,
		ChannelId:        info.ChannelId,
		ClientUserId:     info.ClientUserId,
	}
}

// SaveAiSearchLog 保留为 serper 旧调用方的兼容入口；新代码请用
// SearchResultLogger.ToSearchLog + LOG_DB.Create。
func SaveAiSearchLog(result *SerperSearchResult, info searchCommon.SearchInfo) error {
	return LOG_DB.Create(result.ToSearchLog(info)).Error
}

// ============ jina_reader ============

type JinaReaderUsage struct {
	Tokens int `json:"tokens"`
}

type JinaReaderData struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	URL         string          `json:"url"`
	Content     string          `json:"content"`
	Html        string          `json:"html,omitempty"`
	Text        string          `json:"text,omitempty"`
	Usage       JinaReaderUsage `json:"usage"`
}

type JinaReaderMeta struct {
	Usage JinaReaderUsage `json:"usage"`
}

type JinaReaderResult struct {
	Code   int            `json:"code"`
	Status int            `json:"status"`
	Data   JinaReaderData `json:"data"`
	Meta   JinaReaderMeta `json:"meta"`
}

func (r JinaReaderResult) ToSearchLog(info searchCommon.SearchInfo) *AiSearchLog {
	extra, _ := common.Marshal(map[string]any{
		"title":       r.Data.Title,
		"description": r.Data.Description,
		"status":      r.Status,
		"code":        r.Code,
	})
	cnt := r.Data.Content
	if cnt == "" {
		if r.Data.Html != "" {
			cnt = r.Data.Html
		} else if r.Data.Text != "" {
			cnt = r.Data.Text
		}
	}
	return &AiSearchLog{
		SearchType:       info.SearchType,
		Engine:           info.Engine,
		TokenId:          info.TokenId,
		UserId:           info.UserId,
		ChannelId:        info.ChannelId,
		ClientUserId:     info.ClientUserId,
		SearchParameters: common.JsonStringify(info.SearchParameters),
		TargetURL:        r.Data.URL,
		ContentTokens:    r.Data.Usage.Tokens,
		Content:          cnt,
		Extra:            string(extra),
		UseTimeSeconds:   info.UseTimeSeconds,
	}
}
