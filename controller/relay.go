package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}
	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryTimes := common.RetryTimes
	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	}

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		Retry:       common.GetPointer(0),
		ChannelIds:  tokenChannelIds,
		RequestPath: c.Request.URL.Path,
	}

	// 用于收集重试过程中的错误信息，延迟处理以优化错误日志存储
	type pendingError struct {
		channelError types.ChannelError
		apiError     *types.NewAPIError
		// useTimeMs 是该渠道本次尝试的真实耗时（毫秒），在失败现场捕获，
		// 避免延迟处理时 ContextKeyRequestStartTime 已被后续重试覆盖导致耗时失真
		useTimeMs int64
	}
	var pendingErrors []pendingError

	channelFound := false
	for ; retryParam.GetRetry() <= retryTimes; retryParam.IncreaseRetry() {
		if len(tokenChannelIds) > 0 {
			if retryParam.GetRetry() >= len(tokenChannelIds) {
				break
			}
		}
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			// 当使用 tokenChannelIds 时，如果获取渠道失败（如 no enabled keys），继续尝试下一个渠道
			if len(tokenChannelIds) > 0 {
				newAPIError = channelErr
				continue
			}
			newAPIError = channelErr
			break
		}
		if len(tokenChannelIds) > 0 {
			if channel.Status != common.ChannelStatusEnabled {
				continue
			}
			if !slices.Contains(strings.Split(channel.Models, ","), relayInfo.OriginModelName) {
				continue
			}
		}
		channelFound = true

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		// 在失败现场捕获本渠道真实耗时：每次尝试单独计时，与延迟处理解耦
		attemptStart := time.Now()
		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}
		attemptUseTimeMs := time.Since(attemptStart).Milliseconds()
		// 记录本渠道耗时（毫秒），与 use_channel 一一对应（含成功的最后一个渠道）
		addUsedChannelTime(c, attemptUseTimeMs)

		if newAPIError == nil {
			// 请求成功，处理之前收集的错误（不包含request body，因为消耗日志会记录）
			for _, pe := range pendingErrors {
				processChannelError(c, pe.channelError, pe.apiError, pe.useTimeMs, false, false)
			}
			return
		}

		// 收集错误信息，延迟处理
		pendingErrors = append(pendingErrors, pendingError{
			channelError: *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
			//apiError:     newAPIError,
			apiError:  service.NormalizeViolationFeeError(newAPIError),
			useTimeMs: attemptUseTimeMs,
		})
		//newAPIError = service.NormalizeViolationFeeError(newAPIError)
		//processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		if !shouldRetry(c, newAPIError, retryTimes-retryParam.GetRetry()) {
			break
		}
	}

	// 所有渠道都失败了，处理收集的错误
	// 只有最后一个错误记录request body
	for i, pe := range pendingErrors {
		includeBody := (i == len(pendingErrors)-1) // 只有最后一个错误包含body
		processChannelError(c, pe.channelError, pe.apiError, pe.useTimeMs, includeBody, true)
	}

	// Check if no valid channel was found when using tokenChannelIds
	if len(tokenChannelIds) > 0 && !channelFound && newAPIError == nil {
		newAPIError = types.NewError(
			fmt.Errorf("暂无可用的渠道支持模型 %s）", relayInfo.OriginModelName),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", buildUseChannelWithTimeStr(c, useChannel))
		logger.LogInfo(c, retryLogStr)
	}
}

// buildUseChannelWithTimeStr 把渠道链与各自耗时拼成 "1544(1203ms)->1541(812ms)->1543" 形式。
// 耗时数组缺失或长度不齐时优雅降级为纯渠道链，不影响主流程。
func buildUseChannelWithTimeStr(c *gin.Context, useChannel []string) string {
	times := getUsedChannelTime(c)
	parts := make([]string, 0, len(useChannel))
	for i, ch := range useChannel {
		if i < len(times) {
			parts = append(parts, fmt.Sprintf("%s(%dms)", ch, times[i]))
		} else {
			parts = append(parts, ch)
		}
	}
	return strings.Join(parts, "->")
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

// addUsedChannelTime 记录本次渠道尝试的耗时（毫秒），与 use_channel 一一对应。
func addUsedChannelTime(c *gin.Context, useTimeMs int64) {
	times := getUsedChannelTime(c)
	times = append(times, useTimeMs)
	common.SetContextKey(c, constant.ContextKeyUseChannelTime, times)
}

// getUsedChannelTime 取出已累积的各渠道耗时（毫秒）数组，未设置时返回 nil。
func getUsedChannelTime(c *gin.Context) []int64 {
	if v, ok := common.GetContextKey(c, constant.ContextKeyUseChannelTime); ok {
		if times, ok := v.([]int64); ok {
			return times
		}
	}
	return nil
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if r.MaxCompletionTokens > r.MaxTokens {
			meta.MaxTokens = int(r.MaxCompletionTokens)
		} else {
			meta.MaxTokens = int(r.MaxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(r.MaxOutputTokens)
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(r.MaxTokens)
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil && len(retryParam.ChannelIds) == 0 {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	if retryParam.GetRetry() > 0 {
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	}
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	}
	if len(tokenChannelIds) > 0 {
		return true
	}
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if openaiErr.StatusCode == 307 {
		return true
	}
	if openaiErr.StatusCode/100 == 5 {
		// 超时不重试
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if openaiErr.StatusCode == http.StatusBadRequest {
		return false
	}

	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

var sendLogMap = map[int]int{}

func sendFeishuQianfeiNotify(channelError types.ChannelError, err *types.NewAPIError) {
	webhookUrl := common.OptionMap["feishu_qianfei_webhook_url"]
	secret := common.OptionMap["feishu_qianfei_secret"]
	service.SendFeishuNotify(webhookUrl, secret, dto.FeishuNotify{
		MsgType: "text",
		Content: dto.FeishuContent{
			Text: fmt.Sprintf("【渠道】%s（%d） 可能欠费了,请及时处理，错误信息：%s", channelError.ChannelName, channelError.ChannelId, err.Error()),
		},
	})
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, useTimeMs int64, includeBody bool, notifySlow bool) {
	openaiError := err.ToOpenAIError()
	requestStorage, _ := common.GetBodyStorage(c)
	var requestBytes []byte
	if requestStorage != nil {
		requestBytes, _ = requestStorage.Bytes()
	}
	body := string(requestBytes)

	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	clientUserId := common.GetContextKeyString(c, constant.ContextKeyClientUserId)
	clientScenairo := common.GetContextKeyString(c, constant.ContextKeyClientScenairo)
	extra := common.GetContextKeyString(c, constant.ContextKeyExtra)
	header := common.GetContextKeyString(c, constant.ContextKeyHeader)
	sessionId := common.GetContextKeyString(c, constant.ContextKeyClaudeSessionId)
	requestId := c.GetString(common.RequestIdKey)
	originModelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	contentType := c.Request.Header.Get("Content-Type")
	if common.SaveErrorLog {
		model.SaveErrorLog(c.GetInt("id"), channelError.ChannelId, channelError.ChannelName, originModelName, openaiError, body, contentType, requestId, c.ClientIP(), tokenId, clientUserId, clientScenairo, extra, header, sessionId, useTimeMs, includeBody)
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(channelError.ChannelId, err) {
		if channelError.AutoBan {
			gopool.Go(func() {
				service.DisableChannel(channelError, err.ErrorWithStatusCode())
			})
		}
		// 阿里云的服务很奇葩，429的提示，就像欠费一样。。所以阿里云类型的渠道， 间隔时间设为1小时
		// 海外的渠道，因为飞书通知频率限制，所以间隔时间设为1小时
		gap := time.Minute
		if strings.Contains(channelError.ChannelName, "海外") ||
			channelError.ChannelType == constant.ChannelTypeAli ||
			strings.Contains(channelError.ChannelName, "theapi") ||
			strings.Contains(err.Error(), "received empty response from Gemini: no meaningful content in candidates") ||
			strings.Contains(err.Error(), "aliyun") {
			gap = 60 * time.Minute
		}
		prevSend, exist := sendLogMap[channelError.ChannelId]
		// 防止频繁发送飞书通知，控制一下，每分钟最多发送一次
		if exist {
			if time.Since(time.Unix(int64(prevSend), 0)) > gap {
				// 发送欠费等通知到飞书
				if service.IsInAutoDisableList(strings.ToLower(err.Error())) {
					sendFeishuQianfeiNotify(channelError, err)
					sendLogMap[channelError.ChannelId] = int(time.Now().Unix())
				}
			}
		} else {
			if service.IsInAutoDisableList(strings.ToLower(err.Error())) {
				sendFeishuQianfeiNotify(channelError, err)
				sendLogMap[channelError.ChannelId] = int(time.Now().Unix())
			}
		}
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		// 使用失败现场捕获的本渠道耗时（毫秒），换算为秒写入 logs 表；
		// 旧逻辑用 ContextKeyRequestStartTime 在延迟处理时计算，会因被后续重试覆盖而失真
		other["use_time_ms"] = useTimeMs
		useTimeSeconds := int(useTimeMs / 1000)
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, false, userGroup, other)
	}

	// 慢错误预警：某渠道耗时超过阈值才报错，飞书提醒（仅在最终失败路径触发，重试后整体成功的不报）
	if notifySlow {
		notifyFeishuSlowError(channelError, err, useTimeMs, requestId)
	}
}

var slowErrorSendMap = map[int]int64{}
var slowErrorSendMapMutex sync.Mutex

// 默认慢错误阈值（秒），当 options 表未配置 slow_error_threshold_seconds 时使用
const defaultSlowErrorThresholdSeconds = 180

// notifyFeishuSlowError 当单个渠道本次尝试耗时超过阈值后才报错时，发送飞书提醒。
// 配置读取自 options 表：
//   - slow_error_feishu_webhook_url：飞书机器人地址（为空则功能关闭）
//   - slow_error_feishu_secret：可选签名密钥
//   - slow_error_threshold_seconds：阈值（秒），默认 180
func notifyFeishuSlowError(channelError types.ChannelError, err *types.NewAPIError, useTimeMs int64, requestId string) {
	webhookUrl := common.OptionMap["slow_error_feishu_webhook_url"]
	if webhookUrl == "" {
		return
	}

	thresholdSeconds := defaultSlowErrorThresholdSeconds
	if v := strings.TrimSpace(common.OptionMap["slow_error_threshold_seconds"]); v != "" {
		if parsed, parseErr := strconv.Atoi(v); parseErr == nil && parsed > 0 {
			thresholdSeconds = parsed
		}
	}
	if useTimeMs < int64(thresholdSeconds)*1000 {
		return
	}

	// 防刷屏：同一渠道推送一次后休息 10 分钟
	now := time.Now().Unix()
	slowErrorSendMapMutex.Lock()
	if prev, exist := slowErrorSendMap[channelError.ChannelId]; exist && now-prev < 600 {
		slowErrorSendMapMutex.Unlock()
		return
	}
	slowErrorSendMap[channelError.ChannelId] = now
	slowErrorSendMapMutex.Unlock()

	secret := common.OptionMap["slow_error_feishu_secret"]
	envName := common.OptionMap["ErrorWarningEnvName"]
	useTimeSeconds := useTimeMs / 1000
	content := fmt.Sprintf("【慢错误预警】渠道 %s（%d）耗时 %d 秒后才报错，超过阈值 %d 秒\nStatusCode：%d\nRequestId：%s\n错误信息：%s",
		channelError.ChannelName, channelError.ChannelId, useTimeSeconds, thresholdSeconds, err.StatusCode, requestId, err.Error())
	if envName != "" {
		content = envName + "\n" + content
	}

	gopool.Go(func() {
		notifyErr := service.SendFeishuNotify(webhookUrl, secret, dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		})
		if notifyErr != nil {
			common.SysError("failed to send slow error feishu notify: " + notifyErr.Error())
		}
	})
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	retryTimes := common.RetryTimes
	// channelId := c.GetInt("channel_id")
	// c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return
	}
	// taskErr := taskRelayHandler(c, relayInfo)
	// if taskErr == nil {
	// 	retryTimes = 0
	// }

	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	}

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		Retry:       common.GetPointer(0),
		ChannelIds:  tokenChannelIds,
		RequestPath: c.Request.URL.Path,
	}
	var taskErr *dto.TaskError
	channelFound := false
	for ; shouldRetryTaskRelay(c, retryParam.GetRetry(), taskErr, retryTimes) && retryParam.GetRetry() <= retryTimes; retryParam.IncreaseRetry() {
		if len(tokenChannelIds) > 0 {
			if retryParam.GetRetry() >= len(tokenChannelIds) {
				break
			}
		}
		channel, newAPIError := getChannel(c, relayInfo, retryParam)
		if len(tokenChannelIds) > 0 {
			if channel.Status != common.ChannelStatusEnabled {
				taskErr = &dto.TaskError{Code: "disabled_channel"}
				continue
			}
			if !slices.Contains(strings.Split(channel.Models, ","), relayInfo.OriginModelName) {
				taskErr = &dto.TaskError{Code: "not_supported_channel"}
				continue
			}

		}
		channelFound = true
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
			taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			break
		}
		channelId := channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, retryParam.GetRetry()))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		bodyStorage, err := common.GetBodyStorage(c)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		taskErr = taskRelayHandler(c, relayInfo)
	}

	// Check if no valid channel was found when using tokenChannelIds
	if len(tokenChannelIds) > 0 && !channelFound && (taskErr == nil || taskErr.Code == "disabled_channel" || taskErr.Code == "not_supported_channel") {
		taskErr = service.TaskErrorWrapperLocal(
			fmt.Errorf("暂无可用渠道支持模型 %s）", relayInfo.OriginModelName),
			"get_channel_failed",
			http.StatusServiceUnavailable,
		)
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		taskErr.Message = common.MessageWithRequestId(taskErr.Message, c.GetString(common.RequestIdKey))
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		err = relay.RelayTaskFetch(c, relayInfo)
	default:
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, round int, taskErr *dto.TaskError, retryTimes int) bool {
	if round == 0 {
		return true
	}
	//  第一次taskErr 肯定是nil，但是这个时候是需要继续retry的
	// 当第二次或者更多次的时候，如果 taskErr是nil，说明任务提交成功，不用在retry了
	if taskErr == nil {
		if round > 0 {
			return false
		}
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	}
	if len(tokenChannelIds) > 0 {
		return true
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
