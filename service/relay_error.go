package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
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
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) || operation_setting.IsAlwaysSkipRetryStatusCode(code) {
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
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if ShouldDisableChannel(channelError.ChannelType, err) {
		if channelError.AutoBan {
			reason := err.MaskSensitiveErrorWithStatusCode()
			gopool.Go(func() {
				DisableChannel(channelError, reason)
			})
		}
		notifyPotentialChannelArrears(channelError, err)
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
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
		AppendResponseModelLogInfo(relayInfo, other)
		AppendTaskPluginContextAuditInfo(c, other)
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelError.ChannelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, upstreamRequestId, other)
	}

	if notifySlowError {
		notifySlowChannelError(c, channelError, err, attemptUseTimeMs)
	}
}

func notifyPotentialChannelArrears(channelError types.ChannelError, err *types.NewAPIError) {
	if !IsInAutoDisableList(strings.ToLower(err.Error())) {
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

	notifyErr := SendFeishuNotify(webhookURL, common.OptionMap["feishu_qianfei_secret"], dto.FeishuNotify{
		MsgType: "text",
		Content: dto.FeishuContent{
			Text: fmt.Sprintf("【渠道】%s（%d） 可能欠费了,请及时处理，错误信息：%s", channelError.ChannelName, channelError.ChannelId, err.MaskSensitiveErrorWithStatusCode()),
		},
	})
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
		if notifyErr := SendFeishuNotify(webhookURL, secret, dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		}); notifyErr != nil {
			common.SysError("failed to send slow error feishu notify: " + notifyErr.Error())
		}
	})
}
