package common

import (
	"github.com/QuantumNous/new-api/constant"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// SearchParams 是搜索/抓取请求的统一入参。
// - serper 用 Q / Location / Gl / Hl / Tbs / Num / Page
// - jina_reader 用 URL + Jina（jina 特有 X-* 头映射）
// 不同引擎之间字段互不影响。
//
// {"q":"apple inc","location":"Mexico","gl":"cn","hl":"zh-cn","tbs":"qdr:h","page":2}
type SearchParams struct {
	// 通用
	Q   string `json:"q,omitempty"   form:"q"`
	URL string `json:"url,omitempty" form:"url"` // jina_reader 必填

	// serper
	Location string `json:"location,omitempty" form:"location"`
	Gl       string `json:"gl,omitempty"       form:"gl"`
	Hl       string `json:"hl,omitempty"       form:"hl"`
	Tbs      string `json:"tbs,omitempty"      form:"tbs"`
	Num      int    `json:"num,omitempty"      form:"num"`
	Page     int    `json:"page,omitempty"     form:"page"`

	// 引擎特定：GET 时由 GetSearchInfo 手动 ShouldBindQuery 二次绑定
	Jina *JinaReaderParams `json:"jina,omitempty" form:"-"`
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
	ChannelBaseUrl   string        `json:"channel_base_url"` // 渠道自定义 base_url，未配置时为空字符串
	RequestURLPath   string        `json:"request_url_path"`
	SearchParameters *SearchParams `json:"search_parameters"`
	UseTimeSeconds   int64         `json:"use_time_seconds"`
	ClientUserId     string        `json:"client_user_id,omitempty"`
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
	switch c.Request.Method {
	case "GET":
		if err := c.ShouldBindQuery(searchParams); err != nil {
			return nil
		}
		// gin 的 form binding 不支持嵌套结构体（`?jina.xxx=...` 不会被绑定），
		// 对 /v1/jina/reader 来说所有 X-* 参数都直接平铺在 query 里。
		// 这里对 JinaReaderParams 再做一次 ShouldBindQuery，按它自己的 form tag 绑定。
		if engine == "jina" && action == "reader" {
			jp := &JinaReaderParams{}
			if err := c.ShouldBindQuery(jp); err == nil {
				searchParams.Jina = jp
			}
		}
	case "POST":
		if err := c.ShouldBindJSON(searchParams); err != nil {
			return nil
		}
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
		ChannelBaseUrl:   common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		SearchParameters: searchParams,
		RequestURLPath:   c.Request.URL.String(),
		ClientUserId:     common.GetContextKeyString(c, constant.ContextKeyClientUserId),
	}
}
