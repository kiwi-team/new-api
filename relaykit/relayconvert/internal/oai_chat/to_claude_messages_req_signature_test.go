package oaichat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OpenAI 格式的客户端在多轮对话里回传 assistant 的 reasoning_content + signature 时，
// 必须还原成 Claude 的 thinking 内容块，否则上游会因缺少已签名的思考块而拒绝复用推理。
func TestOpenAIChatRequestToClaudeMessagesRestoresSignedThinkingBlock(t *testing.T) {
	reasoning := "step by step"
	maxTokens := uint(1024)
	textBlock := dto.ClaudeMediaMessage{Type: "text", Text: kitutil.GetPointer("answer")}
	toolBlock := dto.ClaudeMediaMessage{
		Type: "tool_use", Id: "call-1", Name: "get_weather", Input: map[string]any{"city": "Paris"},
	}
	for _, test := range []struct {
		name     string
		content  any
		toolCall bool
		want     []dto.ClaudeMediaMessage
	}{
		{name: "text", content: "answer", want: []dto.ClaudeMediaMessage{textBlock}},
		{name: "null content with tool call", toolCall: true, want: []dto.ClaudeMediaMessage{toolBlock}},
		{name: "empty text with tool call", content: "", toolCall: true, want: []dto.ClaudeMediaMessage{toolBlock}},
		{name: "text with tool call", content: "answer", toolCall: true, want: []dto.ClaudeMediaMessage{textBlock, toolBlock}},
		{name: "content array", content: []any{map[string]any{"type": "text", "text": "answer"}}, want: []dto.ClaudeMediaMessage{textBlock}},
		{name: "content array with tool call", content: []any{map[string]any{"type": "text", "text": "answer"}}, toolCall: true, want: []dto.ClaudeMediaMessage{textBlock, toolBlock}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assistant := dto.Message{
				Role:             "assistant",
				Content:          test.content,
				ReasoningContent: &reasoning,
				Signature:        "sig-abc",
			}
			followup := dto.Message{Role: "user", Content: "go on"}
			if test.toolCall {
				assistant.ToolCalls = json.RawMessage(`[{"id":"call-1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]`)
				followup = dto.Message{Role: "tool", ToolCallId: "call-1", Content: "sunny"}
			}
			req := dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages: []dto.Message{
					{Role: "user", Content: "hi"},
					assistant,
					followup,
				},
			}

			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, req)
			require.NoError(t, err)
			require.Len(t, got.Messages, 3)
			encoded, err := kitutil.Marshal(got)
			require.NoError(t, err)
			var wire struct {
				Messages []struct {
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, kitutil.Unmarshal(encoded, &wire))
			require.Len(t, wire.Messages, 3)
			var blocks []dto.ClaudeMediaMessage
			require.NoError(t, kitutil.Unmarshal(wire.Messages[1].Content, &blocks))
			want := append([]dto.ClaudeMediaMessage{{
				Type: "thinking", Thinking: &reasoning, Signature: "sig-abc",
			}}, test.want...)
			assert.Equal(t, want, blocks)
			if test.toolCall {
				var results []dto.ClaudeMediaMessage
				require.NoError(t, kitutil.Unmarshal(wire.Messages[2].Content, &results))
				assert.Equal(t, []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call-1", Content: "sunny"}}, results)
			}
		})
	}
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

func TestOpenAIChatRequestToClaudeMessagesPreservesToolOnlyContent(t *testing.T) {
	toolCalls := json.RawMessage(`[{"id":"call-1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"ls -la\"}"}}]`)
	toolOnly := `[{"type":"tool_use","id":"call-1","name":"exec","input":{"command":"ls -la"}}]`
	for _, tc := range []struct {
		name        string
		content     any
		reasoning   string
		toolCalls   json.RawMessage
		wantContent string
	}{
		{name: "null content", toolCalls: toolCalls, wantContent: toolOnly},
		{name: "empty text", content: "", toolCalls: toolCalls, wantContent: toolOnly},
		{name: "empty array", content: []any{}, toolCalls: toolCalls, wantContent: toolOnly},
		{name: "unsigned reasoning", reasoning: "Check the files.", toolCalls: toolCalls, wantContent: toolOnly},
		{
			name: "explicit ellipsis text", content: "...", toolCalls: toolCalls,
			wantContent: `[{"type":"text","text":"..."},{"type":"tool_use","id":"call-1","name":"exec","input":{"command":"ls -la"}}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assistant := dto.Message{Role: "assistant", Content: tc.content, ToolCalls: tc.toolCalls}
			if tc.reasoning != "" {
				assistant.ReasoningContent = &tc.reasoning
			}
			req := dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: kitutil.GetPointer(uint(1024)),
				Messages: []dto.Message{
					{Role: "user", Content: "List files."},
					assistant,
				},
			}

			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, req)
			require.NoError(t, err)
			require.Len(t, got.Messages, 2)
			wire, err := kitutil.Marshal(got.Messages[1].Content)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantContent, string(wire))
		})
	}
}
