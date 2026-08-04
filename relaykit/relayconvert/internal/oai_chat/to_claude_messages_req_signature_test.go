package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OpenAI 格式的客户端在多轮对话里回传 assistant 的 reasoning_content + signature 时，
// 必须还原成 Claude 的 thinking 内容块，否则上游会因缺少已签名的思考块而拒绝复用推理。
func TestOpenAIChatRequestToClaudeMessagesRestoresSignedThinkingBlock(t *testing.T) {
	reasoning := "step by step"
	maxTokens := uint(1024)
	req := dto.GeneralOpenAIRequest{
		Model:     "claude-test",
		MaxTokens: &maxTokens,
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
			{
				Role:             "assistant",
				Content:          "answer",
				ReasoningContent: &reasoning,
				Signature:        "sig-abc",
			},
			{Role: "user", Content: "go on"},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)

	blocks, ok := got.Messages[1].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok, "assistant content should be a content-block array, got %T", got.Messages[1].Content)
	require.Len(t, blocks, 2)

	assert.Equal(t, "thinking", blocks[0].Type)
	require.NotNil(t, blocks[0].Thinking)
	assert.Equal(t, reasoning, *blocks[0].Thinking)
	assert.Equal(t, "sig-abc", blocks[0].Signature)

	assert.Equal(t, "text", blocks[1].Type)
	require.NotNil(t, blocks[1].Text)
	assert.Equal(t, "answer", *blocks[1].Text)
}

// 没有 signature 时保持原有的纯字符串 content，避免影响非 thinking 模型。
func TestOpenAIChatRequestToClaudeMessagesKeepsStringContentWithoutSignature(t *testing.T) {
	reasoning := "step by step"
	maxTokens := uint(1024)
	req := dto.GeneralOpenAIRequest{
		Model:     "claude-test",
		MaxTokens: &maxTokens,
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "answer", ReasoningContent: &reasoning},
			{Role: "user", Content: "go on"},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	assert.Equal(t, "answer", got.Messages[1].Content)
}
