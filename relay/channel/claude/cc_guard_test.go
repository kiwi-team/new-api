package claude

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// mustParseRequest 把 JSON body 解析为 ClaudeRequest（模拟真实运行时的解析路径）。
func mustParseRequest(t *testing.T, body string) *dto.ClaudeRequest {
	t.Helper()
	var req dto.ClaudeRequest
	if err := common.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	return &req
}

func ccHeader() http.Header {
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.181 (external, claude-vscode, agent-sdk/0.3.181)")
	h.Set("X-App", "cli")
	h.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27")
	h.Set("Anthropic-Version", "2023-06-01")
	return h
}

const messagesPath = "/v1/messages"

// 真实 CC 主 agent 请求（billing 签名块 + CC 官方 prompt + 合法 metadata）。
const realCCMainBody = `{
	"model": "claude-opus-4-8",
	"max_tokens": 64000,
	"messages": [{"role": "user", "content": [{"type": "text", "text": "hi"}]}],
	"metadata": {"user_id": "{\"device_id\":\"abc\",\"account_uuid\":\"\",\"session_id\":\"2eac572b-72e4-4cbb-abd2-6507b661e2f2\"}"},
	"system": [
		{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.181.604; cc_entrypoint=claude-vscode;"},
		{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
	],
	"tools": [{"name": "Bash"}, {"name": "Read"}]
}`

// 真实 CC 子请求（标题生成）：system prompt 正文与主 agent 不同，但仍带 billing 签名块。
const realCCSubBody = `{
	"model": "claude-opus-4-8",
	"max_tokens": 512,
	"messages": [{"role": "user", "content": [{"type": "text", "text": "title this"}]}],
	"metadata": {"user_id": "{\"device_id\":\"abc\",\"session_id\":\"xyz\"}"},
	"system": [
		{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.207.899; cc_entrypoint=claude-vscode;"},
		{"type": "text", "text": "You are a Claude agent, built on Anthropic's Claude Agent SDK."},
		{"type": "text", "text": "Generate a concise, sentence-case title (3-7 words)."}
	]
}`

func TestDetectClaudeCode_RealMainAgent(t *testing.T) {
	req := mustParseRequest(t, realCCMainBody)
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err != nil {
		t.Fatalf("expected genuine CC request to pass, got: %v", err)
	}
}

func TestDetectClaudeCode_RealSubRequest(t *testing.T) {
	req := mustParseRequest(t, realCCSubBody)
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err != nil {
		t.Fatalf("expected genuine CC sub-request to pass, got: %v", err)
	}
}

func TestDetectClaudeCode_NonMessagesPathUAOnly(t *testing.T) {
	// count_tokens 等路径：UA 通过即放行，不要求 system/metadata。
	req := mustParseRequest(t, `{"model":"claude-opus-4-8"}`)
	if err := DetectClaudeCode(req, ccHeader(), "/v1/messages/count_tokens"); err == nil {
		// count_tokens 路径仍含 "messages"，走完整校验，这里应被拦截（缺 system）。
		t.Fatalf("count_tokens contains 'messages' so full validation applies; expected rejection")
	}
	// 纯 models 路径应放行。
	if err := DetectClaudeCode(req, ccHeader(), "/v1/models"); err != nil {
		t.Fatalf("expected models path to pass with valid UA, got: %v", err)
	}
}

func TestDetectClaudeCode_FakeUserAgent(t *testing.T) {
	req := mustParseRequest(t, realCCMainBody)
	h := ccHeader()
	h.Set("User-Agent", "opencode/1.2.3")
	if err := DetectClaudeCode(req, h, messagesPath); err == nil {
		t.Fatalf("expected fake UA (opencode) to be rejected")
	}
}

func TestDetectClaudeCode_FakePromptAndTool(t *testing.T) {
	body := `{
		"model": "claude-opus-4-8",
		"messages": [{"role":"user","content":"hi"}],
		"system": "You are OpenCode, the best coding agent on the planet.",
		"tools": [{"name":"execute_command"}]
	}`
	req := mustParseRequest(t, body)
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.181 (external, cli)") // 即便伪造合法 UA，黑名单也应命中
	if err := DetectClaudeCode(req, h, messagesPath); err == nil {
		t.Fatalf("expected fake prompt/tool to be rejected")
	}
}

func TestDetectClaudeCode_MissingBetaHeader(t *testing.T) {
	req := mustParseRequest(t, realCCMainBody)
	h := ccHeader()
	h.Del("Anthropic-Beta")
	if err := DetectClaudeCode(req, h, messagesPath); err == nil {
		t.Fatalf("expected missing anthropic-beta to be rejected")
	}
}

func TestDetectClaudeCode_WrongBetaMarker(t *testing.T) {
	req := mustParseRequest(t, realCCMainBody)
	h := ccHeader()
	h.Set("Anthropic-Beta", "some-other-beta-2025-01-01")
	if err := DetectClaudeCode(req, h, messagesPath); err == nil {
		t.Fatalf("expected wrong anthropic-beta marker to be rejected")
	}
}

func TestDetectClaudeCode_InvalidMetadata(t *testing.T) {
	// 合法 headers + billing 块，但 metadata.user_id 不是含 session_id 的 JSON。
	body := `{
		"model": "claude-opus-4-8",
		"messages": [{"role":"user","content":"hi"}],
		"metadata": {"user_id": "plain-user-123"},
		"system": [{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.181.604;"}]
	}`
	req := mustParseRequest(t, body)
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err == nil {
		t.Fatalf("expected invalid metadata.user_id to be rejected")
	}
}

func TestDetectClaudeCode_SystemPromptMismatchNoBilling(t *testing.T) {
	// 合法 headers + 合法 metadata，但 system 既非 CC 前缀也无 billing 块。
	body := `{
		"model": "claude-opus-4-8",
		"messages": [{"role":"user","content":"hi"}],
		"metadata": {"user_id": "{\"session_id\":\"x\"}"},
		"system": "You are a generic helpful assistant."
	}`
	req := mustParseRequest(t, body)
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err == nil {
		t.Fatalf("expected non-CC system prompt without billing block to be rejected")
	}
}

func TestDetectClaudeCode_NilRequest(t *testing.T) {
	if err := DetectClaudeCode(nil, ccHeader(), messagesPath); err == nil {
		t.Fatalf("expected nil request to be rejected")
	}
}

func TestDiceCoefficient(t *testing.T) {
	if d := diceCoefficient("abc", "abc"); d != 1.0 {
		t.Fatalf("identical strings should have dice 1.0, got %v", d)
	}
	if d := diceCoefficient("you are claude code, anthropic's official cli for claude.",
		"you are claude code, anthropic's official cli for claude!"); d < ccSimilarityThreshold {
		t.Fatalf("near-identical CC prompts should exceed threshold, got %v", d)
	}
	if d := diceCoefficient("completely different text about cats",
		"you are claude code official cli"); d >= ccSimilarityThreshold {
		t.Fatalf("unrelated strings should be below threshold, got %v", d)
	}
}

func TestNormalizeClaudeCodeMetadata_UnderscoreToJSON(t *testing.T) {
	body := `{
		"model": "claude-opus-4-8",
		"metadata": {"user_id": "user_e4cd78f1517a6f52130047d93ac4f4b9f50b4aaa4155df2be738b37b033b937e_account__session_684fec4d-d682-4baa-be3a-e4ef4b2efe6e"}
	}`
	req := mustParseRequest(t, body)
	NormalizeClaudeCodeMetadata(req)

	var meta struct {
		UserID string `json:"user_id"`
	}
	if err := common.Unmarshal(req.Metadata, &meta); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	want := `{"device_id":"e4cd78f1517a6f52130047d93ac4f4b9f50b4aaa4155df2be738b37b033b937e","account_uuid":"","session_id":"684fec4d-d682-4baa-be3a-e4ef4b2efe6e"}`
	if meta.UserID != want {
		t.Fatalf("user_id mismatch:\n got: %s\nwant: %s", meta.UserID, want)
	}
}

func TestNormalizeClaudeCodeMetadata_ThenPassesGuard(t *testing.T) {
	// 下划线格式原本无法通过白名单（invalid_metadata_user_id），规整后应通过。
	body := `{
		"model": "claude-opus-4-8",
		"messages": [{"role":"user","content":"hi"}],
		"metadata": {"user_id": "user_dev123_account__session_sess-abc"},
		"system": [{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.181.604;"}]
	}`
	req := mustParseRequest(t, body)

	// 规整前：应被拒。
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err == nil {
		t.Fatalf("expected underscore metadata to be rejected before normalization")
	}
	// 规整后：应通过。
	NormalizeClaudeCodeMetadata(req)
	if err := DetectClaudeCode(req, ccHeader(), messagesPath); err != nil {
		t.Fatalf("expected normalized request to pass guard, got: %v", err)
	}
}

func TestNormalizeClaudeCodeMetadata_AlreadyJSONUnchanged(t *testing.T) {
	orig := `{"user_id":"{\"device_id\":\"d\",\"account_uuid\":\"\",\"session_id\":\"s\"}"}`
	body := `{"model":"claude-opus-4-8","metadata":` + orig + `}`
	req := mustParseRequest(t, body)
	before := string(req.Metadata)
	NormalizeClaudeCodeMetadata(req)
	// 已是 JSON 形态：user_id 内容不应被改动。
	var meta struct {
		UserID string `json:"user_id"`
	}
	_ = common.Unmarshal(req.Metadata, &meta)
	if meta.UserID != `{"device_id":"d","account_uuid":"","session_id":"s"}` {
		t.Fatalf("already-json user_id should be unchanged, got: %s (before=%s)", meta.UserID, before)
	}
}

func TestNormalizeClaudeCodeMetadata_NonMatchingUnchanged(t *testing.T) {
	body := `{"model":"claude-opus-4-8","metadata":{"user_id":"plain-123","extra":"keep"}}`
	req := mustParseRequest(t, body)
	NormalizeClaudeCodeMetadata(req)
	var meta map[string]any
	if err := common.Unmarshal(req.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if meta["user_id"] != "plain-123" {
		t.Fatalf("non-matching user_id should be unchanged, got: %v", meta["user_id"])
	}
	if meta["extra"] != "keep" {
		t.Fatalf("other metadata fields should be preserved, got: %v", meta["extra"])
	}
}

func TestNormalizeClaudeCodeMetadata_NoMetadataNoPanic(t *testing.T) {
	req := mustParseRequest(t, `{"model":"claude-opus-4-8"}`)
	NormalizeClaudeCodeMetadata(req) // 应安全返回
	NormalizeClaudeCodeMetadata(nil) // 应安全返回
}

// firstSystemBlockText 返回 system 数组首块文本（若非数组返回空）。
func firstSystemBlockText(t *testing.T, req *dto.ClaudeRequest) string {
	t.Helper()
	blocks := req.ParseSystem()
	if len(blocks) == 0 {
		t.Fatalf("expected structured system array, got: %#v", req.System)
	}
	return blocks[0].GetText()
}

func TestInsertBilling_NilSystem(t *testing.T) {
	req := &dto.ClaudeRequest{Model: "claude-opus-4-8"}
	InsertClaudeCodeBillingHeader(req, "")
	got := firstSystemBlockText(t, req)
	if got != defaultCCBillingHeader {
		t.Fatalf("expected default billing block, got: %s", got)
	}
}

func TestInsertBilling_StringSystemPrependsAndKeepsOriginal(t *testing.T) {
	req := mustParseRequest(t, `{"model":"claude-opus-4-8","system":"You are Claude Code, Anthropic's official CLI for Claude."}`)
	InsertClaudeCodeBillingHeader(req, "")
	blocks := req.ParseSystem()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (billing + original), got %d", len(blocks))
	}
	if blocks[0].GetText() != defaultCCBillingHeader {
		t.Fatalf("first block should be billing, got: %s", blocks[0].GetText())
	}
	if blocks[1].GetText() != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("original system text must be preserved, got: %s", blocks[1].GetText())
	}
}

func TestInsertBilling_ArraySystemPrepends(t *testing.T) {
	req := mustParseRequest(t, `{"model":"claude-opus-4-8","system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}]}`)
	InsertClaudeCodeBillingHeader(req, "")
	blocks := req.ParseSystem()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].GetText() != defaultCCBillingHeader {
		t.Fatalf("billing block must be first, got: %s", blocks[0].GetText())
	}
}

func TestInsertBilling_AlreadyPresentNoOp(t *testing.T) {
	body := `{"model":"claude-opus-4-8","system":[
		{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.181.604;"},
		{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}
	]}`
	req := mustParseRequest(t, body)
	before := len(req.ParseSystem())
	InsertClaudeCodeBillingHeader(req, "")
	after := req.ParseSystem()
	if len(after) != before {
		t.Fatalf("should not insert when billing block already present: before=%d after=%d", before, len(after))
	}
	if after[0].GetText() != "x-anthropic-billing-header: cc_version=2.1.181.604;" {
		t.Fatalf("existing billing block must remain first, got: %s", after[0].GetText())
	}
}

func TestInsertBilling_CustomContent(t *testing.T) {
	custom := "x-anthropic-billing-header: cc_version=9.9.9.xyz; cc_entrypoint=cli;"
	req := &dto.ClaudeRequest{Model: "claude-opus-4-8"}
	InsertClaudeCodeBillingHeader(req, custom)
	if got := firstSystemBlockText(t, req); got != custom {
		t.Fatalf("expected custom billing header, got: %s", got)
	}
}

func TestInsertBilling_NilRequestNoPanic(t *testing.T) {
	InsertClaudeCodeBillingHeader(nil, "")
}
