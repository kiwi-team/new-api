package jina

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	searchcommon "github.com/QuantumNous/new-api/search/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// ErrUpstreamForwarded 表示 jina 上游返回了非 2xx，并且响应已经被 DoResponse
// 直接透传写回 gin.ResponseWriter；handler 收到这个 sentinel 时应跳过
// 默认的 500 渲染与日志落库。
var ErrUpstreamForwarded = errors.New("jina_reader: upstream response forwarded")

const jinaReaderBaseURL = "https://r.jina.ai/"

// JinaReaderAdaptor 实现 search/engine.SearchAdptor，对接 https://r.jina.ai/ 抓取接口。
type JinaReaderAdaptor struct{}

func (j JinaReaderAdaptor) Init(info *searchcommon.SearchInfo) {}

// GetRequestURL 把 SearchParameters.URL 拼到 r.jina.ai 后面。
// 未带 scheme 时补 https；空值或非法 URL 返回 error。
func (j JinaReaderAdaptor) GetRequestURL(info *searchcommon.SearchInfo) (string, error) {
	if info == nil || info.SearchParameters == nil {
		return "", errors.New("jina_reader: search parameters missing")
	}
	target := strings.TrimSpace(info.SearchParameters.URL)
	if target == "" {
		// 兜底：允许从通用 Q 字段传 URL（早期客户端可能写错）
		target = strings.TrimSpace(info.SearchParameters.Q)
	}
	if target == "" {
		return "", errors.New("jina_reader: url is required")
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	if _, err := url.Parse(target); err != nil {
		return "", fmt.Errorf("jina_reader: invalid url: %w", err)
	}
	return jinaReaderBaseURL + target, nil
}

// SetupRequestHeader 设置鉴权头和所有 X-* 行为头。
func (j JinaReaderAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, info *searchcommon.SearchInfo) error {
	req.Header.Set("Authorization", "Bearer "+info.ChannelKey)
	// 强制 JSON 响应，避免上游切到 SSE
	req.Header.Set("Accept", "application/json")
	if info.SearchParameters != nil {
		applyJinaHeaders(req, info.SearchParameters.Jina)
	}
	return nil
}

func (j JinaReaderAdaptor) DoRequest(c *gin.Context, info *searchcommon.SearchInfo, _ io.Reader) (*http.Response, error) {
	fullURL, err := j.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jina_reader: new request: %w", err)
	}
	if err := j.SetupRequestHeader(c, req, info); err != nil {
		return nil, err
	}
	return service.GetHttpClient().Do(req)
}

// DoResponse 解析 2xx 响应为 JinaReaderResult；非 2xx 时把上游响应原样
// 透传给客户端（状态码 + Content-Type + body），然后返回 ErrUpstreamForwarded
// 让 handler 跳过默认错误流。
func (j JinaReaderAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *searchcommon.SearchInfo) (any, error) {
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("jina_reader: read body: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			c.Writer.Header().Set("Content-Type", ct)
		}
		c.Writer.WriteHeader(resp.StatusCode)
		_, _ = c.Writer.Write(body)
		return nil, ErrUpstreamForwarded
	}

	var out model.JinaReaderResult
	if err := common.UnmarshalJsonStr(string(body), &out); err != nil {
		return nil, fmt.Errorf("jina_reader: decode body: %w", err)
	}

	// TODO(billing): jina-reader 按 token 计费，下一期在这里基于
	//   out.Data.Usage.Tokens * 渠道单价 走 service.PostConsumeTokenQuota 扣费。
	//   同时考虑对入参 TokenBudget 做服务端 clamp，避免单次抓取打爆余额。

	return out, nil
}

// applyJinaHeaders 把 JinaReaderParams 非零字段映射成对应的 X-* 头。
// Headers 透传 map 本期暂不启用——保留字段以便后续打开白名单透传。
func applyJinaHeaders(req *http.Request, p *searchcommon.JinaReaderParams) {
	if p == nil {
		return
	}
	setStr := func(name, val string) {
		if val != "" {
			req.Header.Set(name, val)
		}
	}
	setInt := func(name string, val int) {
		if val > 0 {
			req.Header.Set(name, fmt.Sprintf("%d", val))
		}
	}
	setBool := func(name string, val *bool) {
		if val == nil {
			return
		}
		if *val {
			req.Header.Set(name, "true")
		} else {
			req.Header.Set(name, "false")
		}
	}

	setStr("X-Return-Format", p.ReturnFormat)
	setStr("X-Engine", p.Engine)
	setInt("X-Timeout", p.Timeout)
	setInt("X-Token-Budget", p.TokenBudget)
	setStr("X-Locale", p.Locale)
	setInt("X-Cache-Tolerance", p.CacheTolerance)

	setStr("X-Target-Selector", p.TargetSelector)
	setStr("X-Remove-Selector", p.RemoveSelector)
	setStr("X-Wait-For-Selector", p.WaitForSelector)
	setBool("X-With-Iframe", p.WithIframe)
	setBool("X-With-Shadow-Dom", p.WithShadowDom)

	setStr("X-Retain-Images", p.RetainImages)
	setBool("X-Keep-Img-Data-Url", p.KeepImgDataURL)
	setBool("X-With-Images-Summary", p.WithImagesSummary)
	setBool("X-With-Links-Summary", p.WithLinksSummary)

	setStr("X-Proxy", p.Proxy)
	setStr("X-Proxy-Url", p.ProxyURL)
	setStr("X-Referer", p.Referer)
	setStr("X-User-Agent", p.UserAgent)
	setStr("X-Set-Cookie", p.SetCookie)
}
