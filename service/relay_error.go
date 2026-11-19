package service

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/alerting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	defaultSlowErrorThresholdSeconds = 180
	slowErrorAlertCooldown           = 10 * time.Minute
)

type channelAlertLimiter struct {
	sync.Mutex
	lastSent map[int]time.Time
}

func (l *channelAlertLimiter) allow(channelID int, now time.Time, cooldown time.Duration) bool {
	l.Lock()
	defer l.Unlock()
	if l.lastSent == nil {
		l.lastSent = make(map[int]time.Time)
	}
	if previous, exists := l.lastSent[channelID]; exists && now.Sub(previous) < cooldown {
		return false
	}
	l.lastSent[channelID] = now
	return true
}

var (
	potentialArrearsAlertLimiter = channelAlertLimiter{lastSent: make(map[int]time.Time)}
	slowErrorAlertLimiter        = channelAlertLimiter{lastSent: make(map[int]time.Time)}
)

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	if plan := GetGeminiFilePlan(c); plan != nil && plan.Submitted {
		return PolicyDecision{Action: "stop", Reason: "file_inference_submitted", Source: "file_routing"}
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if types.IsChannelError(err) {
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if retryTimes <= 0 {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if len(GetChannelRetryPlan(c).ChannelIds) > 0 {
		// The client WebSocket handshake precedes upstream channel selection;
		// upstream handshake failures can still try the next route candidate.
		clientWebSocket := c.Request != nil && websocket.IsWebSocketUpgrade(c.Request)
		if c.Writer != nil && c.Writer.Written() && !clientWebSocket {
			return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
		}
		// Explicit routes try their remaining candidates independently of the
		// status rules used by the default channel selector.
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "routing"}
	}
	if operation_setting.IsAlwaysSkipRetryStatusCode(code) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, upstreamRequestId string, attemptUseTimeMs int64, notifySlowError bool) {
	if err == nil {
		return
	}
	upstreamURLs := common.UpstreamAddresses(c)
	logger.LogError(c, common.LogText("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode(upstreamURLs...))))
	if ShouldDisableChannel(channelError.ChannelType, err) {
		if channelError.AutoBan {
			reason := err.MaskSensitiveErrorWithStatusCode(upstreamURLs...)
			modelName := ""
			if relayInfo != nil {
				modelName = relayInfo.OriginModelName
			}
			gopool.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				mode, err := model.AlertChannelProtectionMode(ctx, channelError.ChannelId)
				if err != nil {
					return
				}
				if mode == "scoped" {
					if modelName == "" {
						return
					}
					hold, err := model.CommitScopedLegacyProtection(ctx, channelError.ChannelId, modelName)
					if err != nil {
						common.SysError(common.LogText("scoped channel protection failed"))
						return
					}
					if hold != nil {
						NotifyChannelWatchers("channel_model_isolated_"+strconv.FormatInt(hold.ID, 10), func(lang string) (string, string) {
							params := map[string]any{"Channel": html.EscapeString(channelError.ChannelName), "Model": html.EscapeString(hold.ModelKey), "Reason": html.EscapeString(reason)}
							return i18n.Translate(lang, "alert.model_isolated_subject", params), i18n.Translate(lang, "alert.model_isolated_content", params)
						})
					}
					return
				}
				DisableChannel(channelError, reason)
			})
		}
	}
	notifyPotentialChannelArrears(channelError, err)

	recordRelayErrorLog(c, channelError.ChannelId, err, relayInfo, upstreamRequestId)
	if notifySlowError {
		notifySlowChannelError(c, channelError, err, attemptUseTimeMs)
	}
}

// RecordRelayErrorLog writes the error log of a failed relay request when
// error logs are on. channelId is 0 for a request that reached no channel.
func RecordRelayErrorLog(c *gin.Context, channelId int, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	recordRelayErrorLog(c, channelId, err, relayInfo, c.GetString(common.UpstreamRequestIdKey))
}

func recordRelayErrorLog(c *gin.Context, channelId int, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, upstreamRequestID string) {
	if !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
	}
	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	modelName := c.GetString("original_model")
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	other := model.NewLogOther()
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	other.SetPublic("error_type", err.GetErrorType())
	other.SetPublic("error_code", err.GetErrorCode())
	other.SetPublic("status_code", err.StatusCode)
	AppendRelayLogAdminInfo(c, relayInfo, other)
	appendRequestPluginRecord(relayInfo, other)
	AppendResponseModelLogInfo(relayInfo, other)
	AppendTaskPluginContextAuditInfo(c, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(common.UpstreamAddresses(c)...), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, upstreamRequestID, other)
}

func notifyPotentialChannelArrears(channelError types.ChannelError, err *types.NewAPIError) {
	if channelError.ChannelId <= 0 {
		return
	}
	message := strings.ToLower(err.Error())
	// Billing notifications are independent of channel-disable policy. A generic
	// 403 alone does not establish a balance problem; retain the known provider
	// signal and existing keyword matches as possible-arrears notifications.
	aliAccessDenied := err.StatusCode == http.StatusForbidden && strings.Contains(message, "access denied") && strings.Contains(message, "model-studio/error-code#access-denied")
	if err.StatusCode != http.StatusPaymentRequired && !aliAccessDenied && !strings.Contains(message, "insufficient balance") && !strings.Contains(message, "insufficient_balance") && !IsInAutoDisableList(message) {
		return
	}
	webhookURL := strings.TrimSpace(common.OptionMap["feishu_qianfei_webhook_url"])
	if webhookURL == "" {
		return
	}

	cooldown := time.Minute
	if strings.Contains(channelError.ChannelName, "海外") ||
		channelError.ChannelType == constant.ChannelTypeAli ||
		strings.Contains(channelError.ChannelName, "theapi") ||
		strings.Contains(err.Error(), "received empty response from Gemini: no meaningful content in candidates") ||
		strings.Contains(strings.ToLower(err.Error()), "aliyun") {
		cooldown = time.Hour
	}
	if !potentialArrearsAlertLimiter.allow(channelError.ChannelId, time.Now(), cooldown) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	content := fmt.Sprintf("【渠道】%s（%d） 可能欠费了,请及时处理，错误信息：%s", channelError.ChannelName, channelError.ChannelId, err.MaskSensitiveErrorWithStatusCode())
	notifyErr := alerting.DispatchLegacyNotice(ctx, "channel_arrears", "site", model.AlertIdentity(channelError.ChannelId, time.Now().Unix()/int64(cooldown/time.Second)), channelError.ChannelName, alerting.LegacyNotice{ChannelID: channelError.ChannelId, Title: i18n.Translate("", "alert.legacy_channel_balance"), Content: content}, func(ctx context.Context) error {
		return SendFeishuNotifyContext(ctx, webhookURL, common.OptionMap["feishu_qianfei_secret"], dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: content}})
	}, alerting.LegacyNoticeCooldown{Key: strconv.Itoa(channelError.ChannelId), Duration: cooldown})

	if notifyErr != nil {
		common.SysError("failed to send potential channel arrears feishu notify: " + notifyErr.Error())
	}
}

func notifySlowChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, attemptUseTimeMs int64) {
	webhookURL := strings.TrimSpace(common.OptionMap["slow_error_feishu_webhook_url"])
	if webhookURL == "" {
		return
	}

	thresholdSeconds := defaultSlowErrorThresholdSeconds
	if configured := strings.TrimSpace(common.OptionMap["slow_error_threshold_seconds"]); configured != "" {
		if parsed, parseErr := strconv.Atoi(configured); parseErr == nil && parsed > 0 {
			thresholdSeconds = parsed
		}
	}
	if attemptUseTimeMs < int64(thresholdSeconds)*1000 {
		return
	}
	if !slowErrorAlertLimiter.allow(channelError.ChannelId, time.Now(), slowErrorAlertCooldown) {
		return
	}

	content := fmt.Sprintf("【慢错误预警】渠道 %s（%d）耗时 %d 秒后才报错，超过阈值 %d 秒\nStatusCode：%d\nRequestId：%s\n错误信息：%s",
		channelError.ChannelName,
		channelError.ChannelId,
		attemptUseTimeMs/1000,
		thresholdSeconds,
		err.StatusCode,
		c.GetString(common.RequestIdKey),
		err.MaskSensitiveErrorWithStatusCode(),
	)
	if envName := strings.TrimSpace(common.OptionMap["ErrorWarningEnvName"]); envName != "" {
		content = envName + "\n" + content
	}
	secret := common.OptionMap["slow_error_feishu_secret"]
	gopool.Go(func() {

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if notifyErr := alerting.DispatchLegacyNotice(ctx, "slow_error", "site", model.AlertIdentity(channelError.ChannelId, time.Now().Unix()/int64(slowErrorAlertCooldown/time.Second)), channelError.ChannelName, alerting.LegacyNotice{ChannelID: channelError.ChannelId, Title: i18n.Translate("", "alert.legacy_slow_error"), Content: content}, func(ctx context.Context) error {
			return SendFeishuNotifyContext(ctx, webhookURL, secret, dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: content}})
		}, alerting.LegacyNoticeCooldown{Key: strconv.Itoa(channelError.ChannelId), Duration: slowErrorAlertCooldown}); notifyErr != nil {
			common.SysError("failed to send slow error feishu notify: " + notifyErr.Error())
		}
	})
}
