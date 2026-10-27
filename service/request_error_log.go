package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type requestErrorLogKey struct{}

type requestErrorIdentity struct {
	channel int
	status  int
	code    string
	message string
}

// Shared by nested routers and race attempts, but never by separate WS requests.
type requestErrorLogState struct {
	sync.Mutex
	requestID    string
	started      time.Time
	invalidToken bool
	terminal     bool
	recorded     map[requestErrorIdentity]bool
}

func requestErrorState(c *gin.Context) *requestErrorLogState {
	requestID := c.GetString(common.RequestIdKey)
	if state, ok := c.Request.Context().Value(requestErrorLogKey{}).(*requestErrorLogState); ok && state.requestID == requestID {
		return state
	}
	state := &requestErrorLogState{requestID: requestID, started: time.Now(), recorded: make(map[requestErrorIdentity]bool)}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestErrorLogKey{}, state))
	return state
}

// InitRequestErrorLog must run before copying a request for parallel attempts.
func InitRequestErrorLog(c *gin.Context) { requestErrorState(c) }

// ExcludeInvalidTokenError excludes credential rejections from the business
// error table. Operational/authentication audit logging remains unchanged.
func ExcludeInvalidTokenError(c *gin.Context) {
	state := requestErrorState(c)
	state.Lock()
	defer state.Unlock()
	state.invalidToken = true
}

func RequestErrorHandled(c *gin.Context) bool {
	state := requestErrorState(c)
	state.Lock()
	defer state.Unlock()
	return state.invalidToken || state.terminal
}

// RecordRequestError records the original error before presentation/rewrite.
// A terminal response must not duplicate an already recorded channel attempt.
func RecordRequestError(c *gin.Context, apiErr *types.NewAPIError) {
	if apiErr == nil || c == nil || c.Request == nil {
		return
	}
	state := requestErrorState(c)
	state.Lock()
	defer state.Unlock()
	if state.invalidToken || c.GetBool("request_error_terminal_recorded") {
		return
	}
	c.Set("request_error_terminal_recorded", true)
	c.Set("request_error_original_message", apiErr.Error())
	state.terminal = true
	identity := requestErrorIdentity{c.GetInt("channel_id"), apiErr.StatusCode, string(apiErr.GetErrorCode()), apiErr.Error()}
	if state.recorded[identity] {
		return
	}
	saveRequestError(c, apiErr.ToOpenAIError(), c.GetInt("channel_id"), c.GetString("channel_name"), time.Since(state.started).Milliseconds(), !c.GetBool(common.KeyChannelRaceAttempt))
}

// RecordChannelErrorLog deliberately keeps every attempt, including repeated
// failures on the same channel. Only the later terminal response is deduplicated.
func RecordChannelErrorLog(c *gin.Context, channel types.ChannelError, apiErr *types.NewAPIError, elapsed int64, includeBody bool) {
	if apiErr == nil {
		return
	}
	state := requestErrorState(c)
	state.Lock()
	defer state.Unlock()
	if state.invalidToken {
		return
	}
	saveRequestError(c, apiErr.ToOpenAIError(), channel.ChannelId, channel.ChannelName, elapsed, includeBody)
	state.recorded[requestErrorIdentity{channel.ChannelId, apiErr.StatusCode, string(apiErr.GetErrorCode()), apiErr.Error()}] = true
}

func RecordTaskRequestError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr == nil {
		return
	}
	err := taskErr.Error
	if err == nil {
		message := taskErr.Message
		if message == "" {
			message = http.StatusText(taskErr.StatusCode)
		}
		err = errors.New(message)
	}
	RecordRequestError(c, types.NewOpenAIError(err, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
}

// ClientErrorMessage matches the original error even when a protocol needs a
// sanitized public fallback. It never exposes the original as that fallback.
func ClientErrorMessage(c *gin.Context, message string) string {
	original := c.GetString("request_error_original_message")
	if original == "" {
		original = message
	}
	if id := c.GetString(common.RequestIdKey); id != "" {
		return common.MessageWithRequestId(message, id, original)
	}
	if rewritten := common.RewriteErrorMessage(original); rewritten != original {
		return rewritten
	}
	return message
}

// RewriteClientErrorMessage preserves protocol text verbatim unless an
// administrator's rule matches the original failure.
func RewriteClientErrorMessage(c *gin.Context, message string) string {
	original := c.GetString("request_error_original_message")
	if original == "" {
		original = message
	}
	if common.RewriteErrorMessage(original) == original {
		return message
	}
	return ClientErrorMessage(c, message)
}

// RecordStreamError covers adaptors which settle delivered output and return
// success after a stream error. Recording must not change settlement or retry.
func RecordStreamError(c *gin.Context, stream *relaycommon.StreamStatus) {
	if stream == nil || c.GetBool("request_error_terminal_recorded") {
		return
	}
	outcome := stream.OutcomeSnapshot()
	missingTerminal := outcome.ExpectsTerminal && outcome.Response == relaycommon.ResponseOutcomeUnknown
	if !outcome.HasErrors && outcome.Response != relaycommon.ResponseOutcomeFailed && !missingTerminal && (stream.IsNormalEnd() || outcome.EndReason == relaycommon.StreamEndReasonNone) {
		return
	}
	status := outcome.ErrorStatus
	if status < 400 {
		status = http.StatusBadGateway
		if outcome.EndReason == relaycommon.StreamEndReasonTimeout {
			status = http.StatusGatewayTimeout
		}
		if outcome.EndReason == relaycommon.StreamEndReasonClientGone {
			status = 499
		}
	}
	message := stream.Summary()
	if messages := stream.ErrorMessages(); len(messages) > 0 {
		message += ": " + strings.Join(messages, "; ")
	}
	RecordRequestError(c, types.NewOpenAIError(errors.New(message), types.ErrorCode(outcome.ErrorCode), status))
}

// ClientError returns a copy so rewriting cannot mutate billing, retry or log snapshots.
func ClientError(c *gin.Context, apiErr *types.NewAPIError) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	RecordRequestError(c, apiErr)
	clientErr := *apiErr
	clientErr.SetMessage(ClientErrorMessage(c, apiErr.Error()))
	return &clientErr
}

func saveRequestError(c *gin.Context, apiErr types.OpenAIError, channelID int, channelName string, elapsed int64, includeBody bool) {
	if !common.SaveErrorLog {
		return
	}
	if apiErr.StatusCode == 0 {
		apiErr.StatusCode = http.StatusInternalServerError
	}
	// Error causes may echo credentials. Keep diagnostics, never usable keys.
	for _, key := range []constant.ContextKey{constant.ContextKeyTokenKey, constant.ContextKeyChannelKey} {
		if secret := common.GetContextKeyString(c, key); secret != "" {
			apiErr.Message = strings.ReplaceAll(apiErr.Message, secret, "[REDACTED]")
		}
	}
	if model.LOG_DB == nil {
		logger.LogError(c, "failed to record error_logs entry: log database is unavailable; original error: "+apiErr.Message)
		return
	}
	body := ""
	// Do not read an unvalidated request just for logging (authentication and
	// overload rejection must not allocate or wait for a potentially huge body).
	if value, ok := c.Get(common.KeyBodyStorage); ok && includeBody {
		if storage, ok := value.(common.BodyStorage); ok && storage != nil {
			if data, err := storage.Bytes(); err == nil {
				body = RedactGeminiFileRequest(c, string(data))
			}
		}
	}
	headers := make(map[string]string)
	_ = common.UnmarshalJsonStr(common.GetContextKeyString(c, constant.ContextKeyHeader), &headers)
	common.RedactLogHeaders(headers)
	header, _ := common.Marshal(headers)
	if err := model.SaveErrorLog(c.GetInt("id"), channelID, channelName,
		common.GetContextKeyString(c, constant.ContextKeyOriginalModel), apiErr, body,
		c.GetHeader("Content-Type"), c.GetString(common.RequestIdKey), c.ClientIP(),
		common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		common.GetContextKeyString(c, constant.ContextKeyClientUserId),
		common.GetContextKeyString(c, constant.ContextKeyClientScenairo),
		common.GetContextKeyString(c, constant.ContextKeyExtra), string(header),
		common.GetContextKeyString(c, constant.ContextKeyClaudeSessionId), max(elapsed, 0), includeBody); err != nil {
		logger.LogError(c, fmt.Sprintf("failed to record error_logs entry: %s; original error: %s", err, apiErr.Message))
	}
}
