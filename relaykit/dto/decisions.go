package dto

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// DecisionsRequest follows OpenAI's POST /v1/decisions schema. Input remains
// raw JSON so message roles, tool history and cache breakpoints survive relay.
type DecisionsRequest struct {
	Model              string             `json:"model"`
	Input              json.RawMessage    `json:"input"`
	Questions          []DecisionQuestion `json:"questions"`
	PromptCacheKey     *string            `json:"prompt_cache_key,omitempty"`
	PromptCacheOptions json.RawMessage    `json:"prompt_cache_options,omitempty"`
	SafetyIdentifier   *string            `json:"safety_identifier,omitempty"`
}

type DecisionQuestion struct {
	Type         string           `json:"type"`
	Instructions *string          `json:"instructions"`
	Name         *string          `json:"name,omitempty"`
	Choices      []DecisionChoice `json:"choices,omitempty"`
	Levels       []DecisionLevel  `json:"levels,omitempty"`
}

type DecisionChoice struct {
	Value       any     `json:"value"`
	Description *string `json:"description,omitempty"`
}

type DecisionLevel struct {
	Label       *string `json:"label"`
	Description *string `json:"description,omitempty"`
}

func (r *DecisionsRequest) GetTokenCountMeta() *types.TokenCountMeta {
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	var input any
	_ = kitutil.Unmarshal(r.Input, &input)
	if items, ok := input.([]any); ok {
		for _, item := range items {
			object, _ := item.(map[string]any)
			for _, field := range []string{"content", "output"} {
				parts, _ := object[field].([]any)
				for _, part := range parts {
					content, ok := part.(map[string]any)
					if !ok || content["type"] != "input_image" {
						continue
					}
					url, _ := content["image_url"].(string)
					detail, _ := content["detail"].(string)
					if url != "" {
						meta.Files = append(meta.Files, types.NewImageFileMeta(types.NewFileSourceFromData(url, ""), detail))
					}
					// Count the image through Files, never as base64 text. The
					// decoded copy leaves the original request untouched.
					delete(content, "image_url")
				}
			}
		}
	}
	payload, _ := kitutil.Marshal(struct {
		Input     any                `json:"input"`
		Questions []DecisionQuestion `json:"questions"`
	}{input, r.Questions})
	meta.CombineText = string(payload)
	return meta
}

func (r *DecisionsRequest) IsStream(*http.Request) bool { return false }

func (r *DecisionsRequest) SetModelName(model string) { r.Model = model }

type DecisionsResponse struct {
	Model   string           `json:"model"`
	Answers []DecisionAnswer `json:"answers"`
	Usage   *DecisionUsage   `json:"usage"`
}

type DecisionAnswer struct {
	Type          string                `json:"type"`
	Name          *string               `json:"name"`
	Probability   *float64              `json:"probability,omitempty"`
	Choice        any                   `json:"choice,omitempty"`
	Score         *float64              `json:"score,omitempty"`
	Confidence    *float64              `json:"confidence,omitempty"`
	Probabilities []DecisionProbability `json:"probabilities,omitempty"`
}

type DecisionProbability struct {
	Value       any     `json:"value"`
	Probability float64 `json:"probability"`
	Label       *string `json:"label,omitempty"`
}

// Pointers distinguish missing usage from an explicitly free/zero-token result.
// Keep this wire DTO separate from Usage, whose JSON includes host-only fields.
type DecisionUsage struct {
	InputTokens        *int `json:"input_tokens"`
	OutputTokens       *int `json:"output_tokens"`
	TotalTokens        int  `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}
