package reve

// CreateRequest 对应 Reve 文生图接口 /v1/image/create 的请求体。
type CreateRequest struct {
	Prompt      string `json:"prompt"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Version     string `json:"version,omitempty"`
}

// EditRequest 对应 Reve 图生图接口 /v1/image/edit 的请求体。
// Image 字段既可以是图片 URL，也可以是 base64 字符串。
type EditRequest struct {
	EditInstruction string `json:"edit_instruction"`
	ReferenceImage  string `json:"reference_image"`
	AspectRatio     string `json:"aspect_ratio,omitempty"`
	Version         string `json:"version,omitempty"`
}

// Response 是 Reve 文生图 / 图生图接口的返回体。
// 图片以 base64 字符串形式放在顶层 image 字段中。
type Response struct {
	Image            string `json:"image"`
	Version          string `json:"version,omitempty"`
	CreditsUsed      int    `json:"credits_used,omitempty"`
	CreditsRemaining int    `json:"credits_remaining,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	ContentViolation bool   `json:"content_violation,omitempty"`
}
