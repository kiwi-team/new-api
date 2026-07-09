package openrouter

import "encoding/json"

type RequestReasoning struct {
	// One of the following (not both):
	Effort    string `json:"effort,omitempty"`     // Can be "high", "medium", or "low" (OpenAI-style)
	MaxTokens int    `json:"max_tokens,omitempty"` // Specific token limit (Anthropic-style)
	// Optional: Default is false. All models support this.
	Exclude bool `json:"exclude,omitempty"` // Set to true to exclude reasoning tokens from response
	// Or enable reasoning with the default parameters:
	// 用 *bool 区分未设置 / true / false：bool + omitempty 会把 false 一起省略，无法显式关闭推理
	Enabled *bool `json:"enabled,omitempty"` // true to enable thinking, false to disable thinking
}

type OpenRouterEnterpriseResponse struct {
	Data    json.RawMessage `json:"data"`
	Success bool            `json:"success"`
}

// ImageGenerationRequest 是 OpenRouter 统一生图接口 (POST /api/v1/images) 的请求体。
// 生成 (/v1/images/generations) 不带 InputReferences；
// 编辑 (/v1/images/edits) 通过 InputReferences 携带参考图。
// 参考：https://openrouter.ai/docs/api/api-reference/images/generate-an-image
type ImageGenerationRequest struct {
	Model           string                `json:"model"`
	Prompt          string                `json:"prompt"`
	N               uint                  `json:"n,omitempty"`
	Size            string                `json:"size,omitempty"`
	InputReferences []ImageInputReference `json:"input_references,omitempty"`
}

// ImageInputReference 为参考图元素，Type 固定为 "image_url"。
type ImageInputReference struct {
	Type     string            `json:"type"`
	ImageUrl ImageReferenceUrl `json:"image_url"`
}

// ImageReferenceUrl.Url 支持 HTTP(S) 链接或 base64 data URL。
type ImageReferenceUrl struct {
	Url string `json:"url"`
}
