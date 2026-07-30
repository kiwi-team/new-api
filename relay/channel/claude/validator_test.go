package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateBody 解析请求体后走结构体校验。
//
// 这些用例断言的是 ValidateClaudeRequest 的校验规则本身。ValidateClaudeRequestBody
// 目前在解析成功后直接放行（见 validator.go），是本项目刻意保留的运行时开关，
// 因此不能用它来断言各项规则是否生效。
func validateBody(t *testing.T, body string) error {
	t.Helper()
	var request dto.ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(body), &request))
	return ValidateClaudeRequest(&request)
}

func TestValidateClaudeRequest_EmptyTextBlock(t *testing.T) {
	// assistant 消息中包含空文本块（Claude Code 常见模式：空 text + tool_use）
	body := `{
		"model": "claude-opus-4-6",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": [
				{"type": "text", "text": ""},
				{"type": "tool_use", "id": "t1", "name": "x", "input": {}}
			]}
		]
	}`
	err := validateBody(t, body)
	require.Error(t, err, "expected error for empty text block")
	assert.Contains(t, err.Error(), "text content blocks must be non-empty")
}

func TestValidateClaudeRequest_EmptyStringContent(t *testing.T) {
	body := `{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"user","content":""}]}`
	assert.Error(t, validateBody(t, body), "expected error for empty string content")
}

func TestValidateClaudeRequest_TopKWithThinking(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"top_k":40,"thinking":{"type":"enabled","budget_tokens":1600},"messages":[{"role":"user","content":"hi"}]}`
	err := validateBody(t, body)
	require.Error(t, err, "expected error for top_k with thinking")
	assert.Contains(t, err.Error(), "`top_k` must be unset")
}

func TestValidateClaudeRequest_TopKWithAdaptive(t *testing.T) {
	body := `{"model":"claude-opus-4-7","max_tokens":2048,"top_k":40,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hi"}]}`
	assert.Error(t, validateBody(t, body), "expected error for top_k with adaptive thinking")
}

func TestValidateClaudeRequest_ThinkingBudgetTooLow(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":512},"messages":[{"role":"user","content":"hi"}]}`
	assert.Error(t, validateBody(t, body), "expected error for budget_tokens < 1024")
}

func TestValidateClaudeRequest_ForcedToolChoiceWithThinking(t *testing.T) {
	for _, tc := range []string{`{"type":"any"}`, `{"type":"tool","name":"x"}`} {
		body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"adaptive"},"tool_choice":` + tc + `,"messages":[{"role":"user","content":"hi"}]}`
		assert.Errorf(t, validateBody(t, body), "expected error for forced tool_choice %s with thinking", tc)
	}
}

func TestValidateClaudeRequest_TemperatureOutOfRange(t *testing.T) {
	body := `{"model":"claude-opus-4-6","max_tokens":1024,"temperature":2,"messages":[{"role":"user","content":"hi"}]}`
	assert.Error(t, validateBody(t, body), "expected error for temperature out of range")
}

func TestValidateClaudeRequest_SystemRoleAllowed(t *testing.T) {
	// Claude Code 会在 messages 数组中注入 role:"system" 的消息，不应被拦截。
	body := `{"model":"claude-opus-4-8","max_tokens":1024,"messages":[
		{"role":"user","content":"1+1?"},
		{"role":"system","content":"The following skills are available..."},
		{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"abc"},{"type":"text","text":"2"}]}
	]}`
	assert.NoError(t, validateBody(t, body), "expected system-role message to pass")
}

func TestValidateClaudeRequest_InterleavedBudgetExceedsMaxTokens(t *testing.T) {
	// 开启 interleaved thinking 时，budget_tokens 允许超过 max_tokens，不应拦截
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":8000},"messages":[{"role":"user","content":"hi"}]}`
	assert.NoError(t, validateBody(t, body), "expected valid (interleaved thinking budget > max_tokens allowed)")
}

func TestValidateClaudeRequest_Valid(t *testing.T) {
	cases := []string{
		// 普通请求
		`{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`,
		// adaptive 模式且未设置 top_k/top_p
		`{"model":"claude-opus-4-7","max_tokens":2048,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hi"}]}`,
		// thinking enabled，temperature=1，budget 合法
		`{"model":"claude-sonnet-4-5","max_tokens":4096,"temperature":1,"thinking":{"type":"enabled","budget_tokens":2048},"messages":[{"role":"user","content":"hi"}]}`,
		// 结构化 system + 含 tool_result 的多内容块消息
		`{"model":"claude-opus-4-6","max_tokens":1024,"system":[{"type":"text","text":"you are helpful"}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`,
		// whitespace-only 文本块视为合法（仅拦截真正空字符串）
		`{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":" "}]}]}`,
		// thinking + tool_choice auto 合法
		`{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"adaptive"},"tool_choice":{"type":"auto"},"messages":[{"role":"user","content":"hi"}]}`,
	}
	for i, body := range cases {
		assert.NoErrorf(t, validateBody(t, body), "case %d: expected valid", i)
	}
}

func TestValidateClaudeRequest_StructDeepCopyConsistency(t *testing.T) {
	// 确保对结构体直接校验与对序列化字节校验结果一致
	req := &dto.ClaudeRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: lo.ToPtr(uint(1024)),
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
	}
	require.NoError(t, ValidateClaudeRequest(req), "expected valid struct")
	body, err := common.Marshal(req)
	require.NoError(t, err)
	assert.NoError(t, validateBody(t, string(body)), "expected valid body")
}

// TestValidateClaudeRequestBody_ParseFailurePassesThrough 保证无法解析为 Claude
// 请求结构的请求体不会被本地校验拦截，而是原样交给上游判定。
func TestValidateClaudeRequestBody_ParseFailurePassesThrough(t *testing.T) {
	assert.NoError(t, ValidateClaudeRequestBody([]byte(`not json`)))
}
