package fal_sync

// ============================================================================
// FAL API Response Types
// ============================================================================

// FALQueueStatus represents the FAL Queue Status Response
// Returned when submitting a request to FAL's async API
type FALQueueStatus struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"` // IN_QUEUE, IN_PROGRESS, COMPLETED
}

// FALResultResponse represents the FAL Result Response
// Generic structure to handle various model outputs
type FALResultResponse struct {
	Images      []FALImage `json:"images,omitempty"`
	Image       *FALImage  `json:"image,omitempty"`  // Some models return single image
	Output      any        `json:"output,omitempty"` // Fallback for other output formats
	Seed        int64      `json:"seed,omitempty"`
	Description string     `json:"description,omitempty"` // nano-banana-2 returns a description string
}

// FALImage represents an image in FAL's response
type FALImage struct {
	URL         string `json:"url"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ContentType string `json:"content_type,omitempty"`
}

// FALErrorResponse represents the FAL Error Response
type FALErrorResponse struct {
	Detail string `json:"detail"`
}

// ============================================================================
// FAL Model-Specific Request Types
// All models support multiple images via image_urls (array)
// ============================================================================

// Flux2ProRequest represents the request for flux-2-pro model
// API endpoint: fal-ai/flux-2-pro/edit
type Flux2ProRequest struct {
	Prompt          string   `json:"prompt"`
	ImageURLs       []string `json:"image_urls"`
	ImageSize       string   `json:"image_size,omitempty"`
	Seed            int64    `json:"seed,omitempty"`
	SafetyTolerance int      `json:"safety_tolerance,omitempty"`
	OutputFormat    string   `json:"output_format,omitempty"`
	NumImages       int      `json:"num_images,omitempty"`
}

// HunyuanImageV3Request represents the request for hunyuan-image-v3 model
// API endpoint: fal-ai/hunyuan-image/v3/instruct/edit
type HunyuanImageV3Request struct {
	Prompt       string   `json:"prompt"`
	ImageURLs    []string `json:"image_urls"`
	ImageSize    string   `json:"image_size,omitempty"`
	Seed         int64    `json:"seed,omitempty"`
	OutputFormat string   `json:"output_format,omitempty"`
}

// QwenImageMaxRequest represents the request for qwen-image-max model
// API endpoint: fal-ai/qwen-image-edit-2511
type QwenImageMaxRequest struct {
	Prompt         string   `json:"prompt"`
	ImageURLs      []string `json:"image_urls"`
	ImageSize      string   `json:"image_size,omitempty"`
	Seed           int64    `json:"seed,omitempty"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
	Acceleration   string   `json:"acceleration,omitempty"`
	OutputFormat   string   `json:"output_format,omitempty"`
}

// NanoBanana2Request represents the request for fal-ai/nano-banana-2 (Gemini 3.1 Flash Image)
// API endpoints:
//   - text-to-image: fal-ai/nano-banana-2
//   - edit:          fal-ai/nano-banana-2/edit (requires image_urls)
type NanoBanana2Request struct {
	Prompt           string   `json:"prompt"`
	ImageURLs        []string `json:"image_urls,omitempty"`
	NumImages        int      `json:"num_images,omitempty"`
	Seed             int64    `json:"seed,omitempty"`
	AspectRatio      string   `json:"aspect_ratio,omitempty"`
	OutputFormat     string   `json:"output_format,omitempty"`
	SafetyTolerance  string   `json:"safety_tolerance,omitempty"`
	SyncMode         bool     `json:"sync_mode,omitempty"`
	Resolution       string   `json:"resolution,omitempty"`
	LimitGenerations *bool    `json:"limit_generations,omitempty"`
	EnableWebSearch  *bool    `json:"enable_web_search,omitempty"`
	ThinkingLevel    string   `json:"thinking_level,omitempty"`
}

// GenericFALRequest represents a generic FAL request for unknown models
// Uses map for maximum flexibility
type GenericFALRequest map[string]any

// GptImage2Request represents the request for openai/gpt-image-2/edit model.
// API endpoint: openai/gpt-image-2/edit
// Docs: https://fal.ai/models/openai/gpt-image-2/edit
type GptImage2Request struct {
	Prompt       string   `json:"prompt"`
	ImageURLs    []string `json:"image_urls"`
	ImageSize    string   `json:"image_size,omitempty"` // size string or "auto"
	Quality      string   `json:"quality,omitempty"`    // auto | low | medium | high
	NumImages    int      `json:"num_images,omitempty"` // 1-4
	OutputFormat string   `json:"output_format,omitempty"`
	SyncMode     bool     `json:"sync_mode,omitempty"`
	MaskURL      string   `json:"mask_url,omitempty"`
}

// ============================================================================
// FAL CDN Upload Types
// ============================================================================

// FALCDNTokenResponse is returned by the fal CDN token endpoint.
type FALCDNTokenResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	BaseURL   string `json:"base_url"`
}

// FALCDNUploadResponse is returned by the fal CDN simple upload endpoint.
type FALCDNUploadResponse struct {
	AccessURL string `json:"access_url"`
}
