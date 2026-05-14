package openrouter

import "encoding/json"

type RequestReasoning struct {
	// One of the following (not both):
	Effort    string `json:"effort,omitempty"`     // Can be "high", "medium", or "low" (OpenAI-style)
	MaxTokens int    `json:"max_tokens,omitempty"` // Specific token limit (Anthropic-style)
	// Optional: Default is false. All models support this.
	Exclude bool `json:"exclude,omitempty"` // Set to true to exclude reasoning tokens from response
	// Or enable reasoning with the default parameters:
	Enabled bool `json:"enabled,omitempty"` // true to enable thinking ,false to disable thinking
}

type OpenRouterEnterpriseResponse struct {
	Data    json.RawMessage `json:"data"`
	Success bool            `json:"success"`
}
