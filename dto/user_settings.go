package dto

type UserSetting struct {
	NotifyType            string                        `json:"notify_type,omitempty"`                    // QuotaWarningType 额度预警类型
	QuotaWarningThreshold float64                       `json:"quota_warning_threshold,omitempty"`        // QuotaWarningThreshold 额度预警阈值
	WebhookUrl            string                        `json:"webhook_url,omitempty"`                    // WebhookUrl webhook地址
	WebhookSecret         string                        `json:"webhook_secret,omitempty"`                 // WebhookSecret webhook密钥
	NotificationEmail     string                        `json:"notification_email,omitempty"`             // NotificationEmail 通知邮箱地址
	BarkUrl               string                        `json:"bark_url,omitempty"`                       // BarkUrl Bark推送URL
	GotifyUrl             string                        `json:"gotify_url,omitempty"`                     // GotifyUrl Gotify服务器地址
	GotifyToken           string                        `json:"gotify_token,omitempty"`                   // GotifyToken Gotify应用令牌
	GotifyPriority        int                           `json:"gotify_priority"`                          // GotifyPriority Gotify消息优先级
	AcceptUnsetRatioModel bool                          `json:"accept_unset_model_ratio_model,omitempty"` // AcceptUnsetRatioModel 是否接受未设置价格的模型
	RecordIpLog           bool                          `json:"record_ip_log,omitempty"`                  // 是否记录请求和错误日志IP
	CheckUid              bool                          `json:"check_uid,omitempty"`                      // 是否要求请求携带 uid 进行鉴权（该用户下所有令牌生效）
	SidebarModules        string                        `json:"sidebar_modules,omitempty"`                // SidebarModules 左侧边栏模块配置
	BillingPreference     string                        `json:"billing_preference,omitempty"`             // BillingPreference 扣费策略（订阅/钱包）
	Language              string                        `json:"language,omitempty"`                       // Language 用户语言偏好 (zh, en)
	GroupDiscount         map[string]float64            `json:"group_discount,omitempty"`                 // 分组折扣 {"openai": 0.8}
	ModelExtraDiscount    map[string]map[string]float64 `json:"model_extra_discount,omitempty"`           // 模型额外折扣 {"openai": {"gpt-3.5-turbo": 0.9}}
	ModelLimitsEnabled    bool                          `json:"model_limits_enabled,omitempty"`           // 用户级模型限制开关（令牌单独设置限制时以令牌为准）
	ModelLimits           []string                      `json:"model_limits,omitempty"`                   // 用户级允许请求的模型列表，留空则不限制
}

// GetModelLimitsMap 将用户级模型限制列表转换为 map，便于快速查询
func (s *UserSetting) GetModelLimitsMap() map[string]bool {
	limitsMap := make(map[string]bool, len(s.ModelLimits))
	for _, limit := range s.ModelLimits {
		if limit == "" {
			continue
		}
		limitsMap[limit] = true
	}
	return limitsMap
}

var (
	NotifyTypeEmail   = "email"   // Email 邮件
	NotifyTypeWebhook = "webhook" // Webhook
	NotifyTypeBark    = "bark"    // Bark 推送
	NotifyTypeGotify  = "gotify"  // Gotify 推送
)
