package common

import (
	"github.com/QuantumNous/new-api/constant"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// {"q":"apple inc","location":"Mexico","gl":"cn","hl":"zh-cn","tbs":"qdr:h","page":2}
type SearchParams struct {
	Q        string `json:"q"`
	Location string `json:"location,omitempty"`
	Gl       string `json:"gl,omitempty"`
	Hl       string `json:"hl,omitempty"`
	Tbs      string `json:"tbs,omitempty"`
	Num      int    `json:"num,omitempty"`
	Page     int    `json:"page,omitempty"`
}

type SearchInfo struct {
	ChannelType      int           `json:"channel_type"`
	Engine           string        `json:"engine"`      // serper
	SearchType       string        `json:"search_type"` // search images
	TokenKey         string        `json:"token_key"`   // key
	TokenId          int           `json:"token_id"`
	UserId           int           `json:"user_id"`
	ChannelId        int           `json:"channel_id"`
	ChannelKey       string        `json:"channel_key"`
	RequestURLPath   string        `json:"request_url_path"`
	SearchParameters *SearchParams `json:"search_parameters"`
	UseTimeSeconds   int64         `json:"use_time_seconds"`
}

func GetSearchInfo(c *gin.Context) *SearchInfo {
	engine := c.Param("engine")
	action := c.Param("action")
	if engine == "" {
		engine = "serper"
	}
	if action == "" {
		action = "search"
	}
	searchParams := &SearchParams{}
	if err := c.ShouldBindJSON(searchParams); err != nil {
		return nil
	}
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	tokenKey := common.GetContextKeyString(c, constant.ContextKeyTokenKey)
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	return &SearchInfo{
		Engine:           engine,
		SearchType:       action,
		ChannelType:      channelType,
		TokenKey:         tokenKey,
		TokenId:          tokenId,
		UserId:           userId,
		ChannelId:        channelId,
		ChannelKey:       common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		SearchParameters: searchParams,
		RequestURLPath:   c.Request.URL.String(),
	}
}
