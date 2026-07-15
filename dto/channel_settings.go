package dto

type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	Proxy                  string `json:"proxy"`
	SetRole                string `json:"set_role,omitempty"`
	GoogleFileBucket       string `json:"google_file_bucket,omitempty"`
	GoogleFileUpload       string `json:"google_file_upload,omitempty"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	SystemPrompt           string `json:"system_prompt,omitempty"`
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`
	// ModelOutputMapping 输出模型重命名：键为上游响应中返回的模型名称，值为返回给用户的模型名称。
	// 存储为 JSON 字符串，例如：{"zai-org/glm-4.7-flash": "glm-4.7"}
	ModelOutputMapping string `json:"model_output_mapping,omitempty"`
	// ClaudeCodeGuardEnabled 是否开启 Claude Code 客户端检测（仅 Anthropic 渠道有效）。
	// 开启后，分配到该渠道的请求会先经过 CC 真伪校验，不通过则视为该渠道请求失败，
	// 记录 error_logs 并转由其他渠道重试。默认关闭。
	ClaudeCodeGuardEnabled bool `json:"claude_code_guard_enabled,omitempty"`
	// ClaudeCodeBillingHeader 当开启 Claude Code 客户端检测且请求通过检测后，若 system 中缺少
	// billing 签名块（x-anthropic-billing-header: cc_version=...），则在 system 首位插入的文本。
	// 留空时使用内置默认值。仅在 ClaudeCodeGuardEnabled 为 true 时生效。
	ClaudeCodeBillingHeader string `json:"claude_code_billing_header,omitempty"`
	// PathWhitelist 请求路径白名单（前缀匹配，忽略 query）。非空时，只有请求路径命中列表中
	// 任意一项前缀的请求才会选中该渠道；未命中则跳过该渠道，交由其他渠道处理。为空则不生效。
	PathWhitelist []string `json:"path_whitelist,omitempty"`
	// PathBlacklist 请求路径黑名单（前缀匹配，忽略 query）。请求路径命中列表中任意一项前缀时，
	// 该渠道被跳过，不处理该请求。为空则不生效。黑名单优先级高于白名单。
	PathBlacklist []string `json:"path_blacklist,omitempty"`
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // 默认
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ChannelOtherSettings struct {
	AzureResponsesVersion string        `json:"azure_responses_version,omitempty"`
	VertexKeyType         VertexKeyType `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	OpenRouterEnterprise  *bool         `json:"openrouter_enterprise,omitempty"`
	ClaudeBetaQuery       bool          `json:"claude_beta_query,omitempty"`       // Claude 渠道是否强制追加 ?beta=true
	AllowServiceTier      bool          `json:"allow_service_tier,omitempty"`      // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	DisableStore          bool          `json:"disable_store,omitempty"`           // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowSafetyIdentifier bool          `json:"allow_safety_identifier,omitempty"` // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	AwsKeyType            AwsKeyType    `json:"aws_key_type,omitempty"`
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}
