package claudemessages

import (
	"testing"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseClaude2OpenAIPreservesThinkingSignature(t *testing.T) {
	for _, tc := range []struct {
		name        string
		content     string
		stopReason  string
		wantReason  string
		wantMessage string
	}{
		{
			name: "signed thinking with tool call",
			content: `[
				{"type":"thinking","thinking":"Inspect the files.\n\n","signature":"sig-tool+/="},
				{"type":"tool_use","id":"toolu-1","name":"exec","input":{"command":"ls -la"}}
			]`,
			stopReason:  "tool_use",
			wantReason:  "tool_calls",
			wantMessage: `{"role":"assistant","content":"","reasoning_content":"Inspect the files.\n\n","signature":"sig-tool+/=","tool_calls":[{"id":"toolu-1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"ls -la\"}"}}]}`,
		},
		{
			name: "signed thinking with text",
			content: `[
				{"type":"thinking","thinking":"Check the answer.","signature":"sig-text+/="},
				{"type":"text","text":"Done."}
			]`,
			stopReason:  "end_turn",
			wantReason:  "stop",
			wantMessage: `{"role":"assistant","content":"Done.","reasoning_content":"Check the answer.","signature":"sig-text+/="}`,
		},
		{
			name: "unsigned thinking",
			content: `[
				{"type":"thinking","thinking":"Check the answer."},
				{"type":"text","text":"Done."}
			]`,
			stopReason:  "end_turn",
			wantReason:  "stop",
			wantMessage: `{"role":"assistant","content":"Done.","reasoning_content":"Check the answer."}`,
		},
		{
			name:        "text without thinking",
			content:     `[{"type":"text","text":"Done."}]`,
			stopReason:  "end_turn",
			wantReason:  "stop",
			wantMessage: `{"role":"assistant","content":"Done."}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := dto.ClaudeResponse{StopReason: tc.stopReason}
			require.NoError(t, kitutil.UnmarshalJsonStr(tc.content, &upstream.Content))

			resp := ResponseClaude2OpenAI(&upstream)
			require.NotNil(t, resp)
			require.Len(t, resp.Choices, 1)
			assert.Equal(t, tc.wantReason, resp.Choices[0].FinishReason)

			wire, err := kitutil.Marshal(resp)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantMessage, ojson.GetBytes(wire, "choices.0.message").Raw)
		})
	}
}

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
