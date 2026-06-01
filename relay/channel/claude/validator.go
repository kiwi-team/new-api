package claude

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// 校验项对齐 Anthropic Messages API 的服务端约束，目的是在「最终请求体即将发往上游
// Claude 渠道之前」提前拦截会被上游拒绝的非法参数，避免把明显的参数错误透传给上游
// （既浪费上游配额，也会触发无意义的跨渠道重试）。
//
// 典型被拦截的上游错误：
//   - {"type":"invalid_request_error","message":"messages: text content blocks must be non-empty"}
//   - {"type":"invalid_request_error","message":"`top_k` must be unset when thinking is enabled or in adaptive mode"}
//
// 校验覆盖（对齐官方文档 https://platform.claude.com/docs/en/api/cli/messages/create
// 与扩展思考文档 .../build-with-claude/extended-thinking）：
//   - messages：非空、数量 <= 100000、role 仅 user/assistant、文本内容块非空
//   - system：结构化 system 的文本块非空
//   - 采样参数：temperature ∈ [0,1]、top_p ∈ [0,1]、top_k >= 0
//   - thinking（enabled/adaptive）：top_k/top_p 必须未设置、temperature 只能为 1、
//     不允许强制工具调用（tool_choice any/tool）、enabled 模式 budget_tokens >= 1024
//
// 取舍说明：
//   - 不校验 budget_tokens < max_tokens：开启 interleaved thinking 时允许超过。
//   - 仅拦截字面空字符串文本，whitespace-only 视为合法，避免误杀上游可接受的请求。

// ValidateClaudeRequestBody 校验最终序列化后的 Claude 请求体。
//
// 之所以基于最终的 JSON 字节而非中间的请求结构体进行校验，是为了确保 model mapping、
// thinking 适配、字段裁剪（RemoveDisabledFields）、参数覆盖（ParamOverride）等所有
// 参数转换都已完成，校验的对象与真正发往上游的内容完全一致。
//
// 校验不通过时返回 error，调用方据此返回参数异常，不再请求上游。
func ValidateClaudeRequestBody(body []byte) error {
	var request dto.ClaudeRequest
	if err := common.Unmarshal(body, &request); err != nil {
		// 无法解析为 Claude 请求结构时，不在此处拦截，交由后续流程/上游处理，避免误判。
		return nil
	}
	return ValidateClaudeRequest(&request)
}

// ValidateClaudeRequest 对 Claude 请求结构体做参数校验。
func ValidateClaudeRequest(request *dto.ClaudeRequest) error {
	if request == nil {
		return fmt.Errorf("request is nil")
	}

	if strings.TrimSpace(request.Model) == "" {
		return fmt.Errorf("model: field required")
	}

	if err := validateClaudeMessages(request); err != nil {
		return err
	}

	if err := validateClaudeSystem(request); err != nil {
		return err
	}

	if err := validateClaudeSampling(request); err != nil {
		return err
	}

	if err := validateClaudeThinking(request); err != nil {
		return err
	}

	return nil
}

// maxClaudeMessages 单次请求的消息数量上限（Anthropic Messages API 限制）。
const maxClaudeMessages = 100000

// validateClaudeMessages 校验 messages：至少有一条消息，且不包含空的文本内容块。
func validateClaudeMessages(request *dto.ClaudeRequest) error {
	if len(request.Messages) == 0 {
		return fmt.Errorf("messages: at least one message is required")
	}
	if len(request.Messages) > maxClaudeMessages {
		return fmt.Errorf("messages: a maximum of %d messages is allowed, got %d", maxClaudeMessages, len(request.Messages))
	}

	for i, message := range request.Messages {
		// role 只能是 user 或 assistant。
		if message.Role != "user" && message.Role != "assistant" {
			return fmt.Errorf("messages.%d.role: must be one of \"user\" or \"assistant\", got %q", i, message.Role)
		}

		// content 为字符串：等价于单个 text 内容块，空字符串会被上游拒绝。
		if message.IsStringContent() {
			if message.GetStringContent() == "" {
				return fmt.Errorf("messages.%d: text content blocks must be non-empty", i)
			}
			continue
		}

		// content 为结构化内容块数组。
		blocks, err := message.ParseContent()
		if err != nil {
			// 解析失败说明 content 既不是字符串也不是标准内容块数组，
			// 不在此处拦截，避免误判非常规但合法的结构。
			continue
		}
		if len(blocks) == 0 {
			return fmt.Errorf("messages.%d.content: at least one content block is required", i)
		}
		if err := validateClaudeContentBlocks(blocks, fmt.Sprintf("messages.%d.content", i)); err != nil {
			return err
		}
	}

	return nil
}

// validateClaudeContentBlocks 校验内容块数组，递归处理 tool_result 中的嵌套内容块。
func validateClaudeContentBlocks(blocks []dto.ClaudeMediaMessage, path string) error {
	for j, block := range blocks {
		switch block.Type {
		case "", dto.ContentTypeText:
			// 空类型按 text 处理；text 内容块的文本不能为空。
			if block.GetText() == "" {
				return fmt.Errorf("%s.%d: text content blocks must be non-empty", path, j)
			}
		case "tool_result":
			// tool_result 的 content 可能是字符串，也可能是嵌套内容块数组。
			// 仅递归校验嵌套内容块中的空文本；字符串内容（含空字符串）保持透传，避免误判。
			if block.Content == nil || block.IsStringContent() {
				continue
			}
			nested := block.ParseMediaContent()
			if len(nested) == 0 {
				continue
			}
			if err := validateClaudeContentBlocks(nested, fmt.Sprintf("%s.%d.content", path, j)); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateClaudeSystem 校验 system：结构化 system 不能包含空的文本块。
func validateClaudeSystem(request *dto.ClaudeRequest) error {
	if request.System == nil {
		return nil
	}
	// 字符串形式的 system 为空时视为未设置，上游允许，不拦截。
	if request.IsStringSystem() {
		return nil
	}
	for j, block := range request.ParseSystem() {
		switch block.Type {
		case "", dto.ContentTypeText:
			if block.GetText() == "" {
				return fmt.Errorf("system.%d: text content blocks must be non-empty", j)
			}
		}
	}
	return nil
}

// validateClaudeSampling 校验采样参数的取值范围（与 thinking 无关的通用约束）。
//
// DTO 中 TopP / TopK 为非指针类型且带 omitempty，因此 0 等价于「未设置」，
// 范围校验只拦截明显越界的值，避免把「未设置」误判为非法。
func validateClaudeSampling(request *dto.ClaudeRequest) error {
	// temperature 取值范围为 0.0 ~ 1.0。
	if request.Temperature != nil && (*request.Temperature < 0 || *request.Temperature > 1) {
		return fmt.Errorf("temperature: must be between 0 and 1, got %v", *request.Temperature)
	}
	// top_p 取值范围为 0 ~ 1（0 视为未设置，仅拦截越界）。
	if request.TopP < 0 || request.TopP > 1 {
		return fmt.Errorf("top_p: must be between 0 and 1, got %v", request.TopP)
	}
	// top_k 不能为负数（0 视为未设置）。
	if request.TopK < 0 {
		return fmt.Errorf("top_k: must be a non-negative integer, got %d", request.TopK)
	}
	return nil
}

// validateClaudeThinking 校验开启 thinking（enabled / adaptive）时的相关约束。
//
// DTO 中 TopP / TopK 为非指针类型且带 omitempty，因此 0 等价于「未设置」。
func validateClaudeThinking(request *dto.ClaudeRequest) error {
	if request.Thinking == nil {
		return nil
	}

	thinkingType := request.Thinking.Type
	if thinkingType != "enabled" && thinkingType != "adaptive" {
		return nil
	}

	// top_k 必须在 thinking 开启或 adaptive 模式下不设置。
	if request.TopK != 0 {
		return fmt.Errorf("`top_k` must be unset when thinking is enabled or in adaptive mode")
	}
	// top_p 同样不允许设置。
	if request.TopP != 0 {
		return fmt.Errorf("`top_p` must be unset when thinking is enabled or in adaptive mode")
	}
	// temperature 在 thinking 开启时只能为 1。
	if request.Temperature != nil && *request.Temperature != 1 {
		return fmt.Errorf("`temperature` may only be set to 1 when thinking is enabled or in adaptive mode")
	}

	// 强制工具调用与 thinking 不兼容：thinking 下 tool_choice 只允许 auto / none，
	// any 与 tool（指定工具）会被上游拒绝。
	if choice := getClaudeToolChoiceType(request.ToolChoice); choice == "any" || choice == "tool" {
		return fmt.Errorf("tool_choice: forced tool use (type %q) is not compatible with extended thinking; only \"auto\" or \"none\" is allowed", choice)
	}

	// enabled 模式下 budget_tokens 至少为 1024。
	// 注意：不校验 budget_tokens < max_tokens —— 开启 interleaved thinking 时
	// budget_tokens 允许超过 max_tokens（代表整轮所有 thinking 块的总预算）。
	// adaptive 模式不使用 budget_tokens，跳过该校验。
	if thinkingType == "enabled" {
		if budget := request.Thinking.GetBudgetTokens(); budget < 1024 {
			return fmt.Errorf("thinking.budget_tokens: must be greater than or equal to 1024, got %d", budget)
		}
	}

	return nil
}

// getClaudeToolChoiceType 从 tool_choice 中提取 type 字段，兼容多种动态类型。
func getClaudeToolChoiceType(toolChoice any) string {
	switch tc := toolChoice.(type) {
	case nil:
		return ""
	case dto.ClaudeToolChoice:
		return tc.Type
	case *dto.ClaudeToolChoice:
		if tc == nil {
			return ""
		}
		return tc.Type
	case map[string]any:
		if t, ok := tc["type"].(string); ok {
			return t
		}
	}
	return ""
}
