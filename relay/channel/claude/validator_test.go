package claude

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

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
	err := ValidateClaudeRequestBody([]byte(body))
	if err == nil {
		t.Fatalf("expected error for empty text block, got nil")
	}
	if !strings.Contains(err.Error(), "text content blocks must be non-empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaudeRequest_EmptyStringContent(t *testing.T) {
	body := `{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"user","content":""}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
		t.Fatalf("expected error for empty string content, got nil")
	}
}

func TestValidateClaudeRequest_TopKWithThinking(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"top_k":40,"thinking":{"type":"enabled","budget_tokens":1600},"messages":[{"role":"user","content":"hi"}]}`
	err := ValidateClaudeRequestBody([]byte(body))
	if err == nil {
		t.Fatalf("expected error for top_k with thinking, got nil")
	}
	if !strings.Contains(err.Error(), "`top_k` must be unset") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaudeRequest_TopKWithAdaptive(t *testing.T) {
	body := `{"model":"claude-opus-4-7","max_tokens":2048,"top_k":40,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hi"}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
		t.Fatalf("expected error for top_k with adaptive thinking, got nil")
	}
}

func TestValidateClaudeRequest_ThinkingBudgetTooLow(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":512},"messages":[{"role":"user","content":"hi"}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
		t.Fatalf("expected error for budget_tokens < 1024, got nil")
	}
}

func TestValidateClaudeRequest_ForcedToolChoiceWithThinking(t *testing.T) {
	for _, tc := range []string{`{"type":"any"}`, `{"type":"tool","name":"x"}`} {
		body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"adaptive"},"tool_choice":` + tc + `,"messages":[{"role":"user","content":"hi"}]}`
		if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
			t.Fatalf("expected error for forced tool_choice %s with thinking, got nil", tc)
		}
	}
}

func TestValidateClaudeRequest_TemperatureOutOfRange(t *testing.T) {
	body := `{"model":"claude-opus-4-6","max_tokens":1024,"temperature":2,"messages":[{"role":"user","content":"hi"}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
		t.Fatalf("expected error for temperature out of range, got nil")
	}
}

func TestValidateClaudeRequest_InvalidRole(t *testing.T) {
	body := `{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"system","content":"hi"}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err == nil {
		t.Fatalf("expected error for invalid role, got nil")
	}
}

func TestValidateClaudeRequest_InterleavedBudgetExceedsMaxTokens(t *testing.T) {
	// 开启 interleaved thinking 时，budget_tokens 允许超过 max_tokens，不应拦截
	body := `{"model":"claude-sonnet-4-5","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":8000},"messages":[{"role":"user","content":"hi"}]}`
	if err := ValidateClaudeRequestBody([]byte(body)); err != nil {
		t.Fatalf("expected valid (interleaved thinking budget > max_tokens allowed), got: %v", err)
	}
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
		if err := ValidateClaudeRequestBody([]byte(body)); err != nil {
			t.Fatalf("case %d: expected valid, got error: %v", i, err)
		}
	}
}

func TestValidateClaudeRequest_StructDeepCopyConsistency(t *testing.T) {
	// 确保对结构体直接校验与对序列化字节校验结果一致
	req := &dto.ClaudeRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 1024,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
	}
	if err := ValidateClaudeRequest(req); err != nil {
		t.Fatalf("expected valid struct, got: %v", err)
	}
	body, _ := common.Marshal(req)
	if err := ValidateClaudeRequestBody(body); err != nil {
		t.Fatalf("expected valid body, got: %v", err)
	}
}
