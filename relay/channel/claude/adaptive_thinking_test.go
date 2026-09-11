package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiresAdaptiveThinking(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-4-6", want: false},
		{model: "claude-opus-4-7", want: true},
		{model: "claude-opus-4-8-20260901", want: true},
		{model: "claude-opus-5", want: true},
		{model: "claude-sonnet-5-20260901", want: true},
		{model: "claude-6-opus", want: true},
		{model: "other-opus-5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, RequiresAdaptiveThinking(tt.model))
		})
	}
}

func TestNormalizeAdaptiveThinkingRewritesClaude5Budget(t *testing.T) {
	budget := 31999
	temperature := 0.7
	topP := 0.9
	topK := 40
	request := &dto.ClaudeRequest{
		Model:       "claude-opus-5",
		Thinking:    &dto.Thinking{Type: "enabled", BudgetTokens: &budget},
		Temperature: &temperature,
		TopP:        &topP,
		TopK:        &topK,
	}

	require.True(t, NormalizeAdaptiveThinking(request))
	require.NotNil(t, request.Thinking)
	assert.Equal(t, "adaptive", request.Thinking.Type)
	assert.Nil(t, request.Thinking.BudgetTokens)
	assert.JSONEq(t, `{"effort":"high"}`, string(request.OutputConfig))
	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.TopP)
	assert.Nil(t, request.TopK)
}

func TestNormalizeAdaptiveThinkingLeavesOlderClaudeModelUnchanged(t *testing.T) {
	budget := 31999
	request := &dto.ClaudeRequest{
		Model:    "claude-opus-4-6",
		Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: &budget},
	}

	require.False(t, NormalizeAdaptiveThinking(request))
	assert.Equal(t, "enabled", request.Thinking.Type)
	assert.Equal(t, budget, request.Thinking.GetBudgetTokens())
	assert.Nil(t, request.OutputConfig)
}
