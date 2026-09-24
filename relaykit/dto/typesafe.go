package dto

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// TypeSafeRequest is the native request format accepted by System One and the
// OpenRouter Decisions endpoint. RawMessage keeps structured fields intact.
type TypeSafeRequest struct {
	State     json.RawMessage            `json:"state"`
	Model     string                     `json:"model"`
	Questions map[string]json.RawMessage `json:"questions"`
	Provider  json.RawMessage            `json:"provider,omitempty"`
	SessionID *string                    `json:"session_id,omitempty"`
	Trace     json.RawMessage            `json:"trace,omitempty"`
	User      *string                    `json:"user,omitempty"`
}

func (r *TypeSafeRequest) GetTokenCountMeta() *types.TokenCountMeta {
	if r == nil {
		return &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	}
	payload, _ := kitutil.Marshal(struct {
		State     json.RawMessage            `json:"state"`
		Questions map[string]json.RawMessage `json:"questions"`
	}{State: r.State, Questions: r.Questions})
	return &types.TokenCountMeta{
		TokenType:   types.TokenTypeTokenizer,
		CombineText: string(payload),
	}
}

func (r *TypeSafeRequest) IsStream(*http.Request) bool {
	return false
}

func (r *TypeSafeRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}
