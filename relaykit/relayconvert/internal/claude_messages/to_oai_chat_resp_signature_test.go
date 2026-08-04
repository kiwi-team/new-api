package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signature_delta 必须以 signature 字段透传给 OpenAI 格式的客户端，
// 客户端下一轮回传后才能还原出已签名的 thinking 块。
// 旧行为把它当成一个换行塞进 reasoning_content，签名就此丢失。
func TestStreamResponseClaude2OpenAIPassesThroughSignature(t *testing.T) {
	resp := StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Type:      "signature_delta",
			Signature: "sig-abc",
		},
	})

	require.NotNil(t, resp)
	require.Len(t, resp.Choices, 1)
	require.NotNil(t, resp.Choices[0].Delta.Signature)
	assert.Equal(t, "sig-abc", *resp.Choices[0].Delta.Signature)
	assert.Nil(t, resp.Choices[0].Delta.ReasoningContent)
}

// thinking_delta 仍走 reasoning_content，不受签名透传影响。
func TestStreamResponseClaude2OpenAIKeepsThinkingDelta(t *testing.T) {
	thinking := "step by step"
	resp := StreamResponseClaude2OpenAI(&dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Type:     "thinking_delta",
			Thinking: &thinking,
		},
	})

	require.NotNil(t, resp)
	require.Len(t, resp.Choices, 1)
	require.NotNil(t, resp.Choices[0].Delta.ReasoningContent)
	assert.Equal(t, thinking, *resp.Choices[0].Delta.ReasoningContent)
	assert.Nil(t, resp.Choices[0].Delta.Signature)
}
