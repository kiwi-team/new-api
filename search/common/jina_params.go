package common

// JinaReaderParams 描述 /v1/jina/reader 请求支持的 X-* 头映射。
// 字段全部 omitempty；bool 用指针区分「未传」与「显式 false」，
// 避免覆盖 jina 端的默认行为。
type JinaReaderParams struct {
	ReturnFormat   string `json:"return_format,omitempty"    form:"return_format"`    // X-Return-Format
	Engine         string `json:"engine,omitempty"           form:"jina_engine"`      // X-Engine
	Timeout        int    `json:"timeout,omitempty"          form:"timeout"`          // X-Timeout
	TokenBudget    int    `json:"token_budget,omitempty"     form:"token_budget"`     // X-Token-Budget
	Locale         string `json:"locale,omitempty"           form:"locale"`           // X-Locale
	CacheTolerance int    `json:"cache_tolerance,omitempty"  form:"cache_tolerance"`  // X-Cache-Tolerance

	TargetSelector  string `json:"target_selector,omitempty"   form:"target_selector"`   // X-Target-Selector
	RemoveSelector  string `json:"remove_selector,omitempty"   form:"remove_selector"`   // X-Remove-Selector
	WaitForSelector string `json:"wait_for_selector,omitempty" form:"wait_for_selector"` // X-Wait-For-Selector
	WithIframe      *bool  `json:"with_iframe,omitempty"       form:"with_iframe"`       // X-With-Iframe
	WithShadowDom   *bool  `json:"with_shadow_dom,omitempty"   form:"with_shadow_dom"`   // X-With-Shadow-Dom

	RetainImages      string `json:"retain_images,omitempty"       form:"retain_images"`       // X-Retain-Images: none/all
	KeepImgDataURL    *bool  `json:"keep_img_data_url,omitempty"   form:"keep_img_data_url"`   // X-Keep-Img-Data-Url
	WithImagesSummary *bool  `json:"with_images_summary,omitempty" form:"with_images_summary"` // X-With-Images-Summary
	WithLinksSummary  *bool  `json:"with_links_summary,omitempty"  form:"with_links_summary"`  // X-With-Links-Summary

	Proxy     string `json:"proxy,omitempty"      form:"proxy"`      // X-Proxy
	ProxyURL  string `json:"proxy_url,omitempty"  form:"proxy_url"`  // X-Proxy-Url
	Referer   string `json:"referer,omitempty"    form:"referer"`    // X-Referer
	UserAgent string `json:"user_agent,omitempty" form:"user_agent"` // X-User-Agent
	SetCookie string `json:"set_cookie,omitempty" form:"set_cookie"` // X-Set-Cookie

	// 透传 map：本期暂不在 adaptor 中启用，保留字段以便后续打开白名单透传。
	Headers map[string]string `json:"headers,omitempty" form:"-"`
}
