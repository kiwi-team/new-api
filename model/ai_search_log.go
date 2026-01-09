package model

import (
	"github.com/QuantumNous/new-api/common"

	searchCommon "github.com/QuantumNous/new-api/search/common"
)

type AiSearchLog struct {
	Id               uint   `gorm:"primaryKey" json:"id"`
	SearchType       string `json:"search_type"` // search images
	Engine           string `json:"engine"`      //serper
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
}

type SerperSearchResult struct {
	Credits         int `json:"credits"`
	KnowledgeGraph  any `json:"knowledge_graph"`
	Organic         any `json:"organic"`
	PeopleAlsoAsk   any `json:"people_also_ask"`
	RelatedSearches any `json:"related_searches"`
}

func SaveAiSearchLog(result *SerperSearchResult, info searchCommon.SearchInfo) error {
	return LOG_DB.Create(&AiSearchLog{
		Credits:          result.Credits,
		KnowledgeGraph:   common.JsonStringify(result.KnowledgeGraph),
		Organic:          common.JsonStringify(result.Organic),
		PeopleAlsoAsk:    common.JsonStringify(result.PeopleAlsoAsk),
		RelatedSearches:  common.JsonStringify(result.RelatedSearches),
		SearchParameters: common.JsonStringify(info.SearchParameters),
		UseTimeSeconds:   info.UseTimeSeconds,
		SearchType:       info.SearchType,
		Engine:           info.Engine,
		TokenId:          info.TokenId,
		UserId:           info.UserId,
		ChannelId:        info.ChannelId,
		ClientUserId:     info.ClientUserId,
	}).Error
}
