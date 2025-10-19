package controller

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
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
	case relayconstant.RelayModeResponses:
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
	//group := c.GetString("group")
	//originalModel := c.GetString("original_model")
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	//var openaiErr *dto.OpenAIErrorWithStatusCode
	//var newAPIError *types.NewAPIError
	retryTimes := common.RetryTimes
	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	} else {
		idsStr := common.OptionMap["GlobalFirstChannels"]
		theSwitch := common.OptionMap["GlobalFirstChannelsSwitch"]
		if theSwitch == "true" {
			idsArr := strings.Split(idsStr, ",")
			// todo 66666 去掉不支持模型的渠道
			for _, id := range idsArr {
				if idInt, er := strconv.Atoi(id); er == nil {
					ch, err2 := model.GetChannelById(idInt, false)
					if err2 != nil {
						continue
					}
					if ch.Status != common.ChannelStatusEnabled {
						continue
					}
					if slices.Contains(strings.Split(ch.Models, ","), c.GetString("original_model")) {
						tokenChannelIds = append(tokenChannelIds, idInt)
					}
				}
			}
		}
	}
	var channel *model.Channel
	var err *types.NewAPIError
	var err1 error
	for i := 0; i <= retryTimes; i++ {
		if len(tokenChannelIds) > 0 {
			if i >= len(tokenChannelIds) {
				break
			}
			channel, err1 = model.GetChannelById(tokenChannelIds[i], true)
			if err1 != nil {
				err = types.NewError(err1, types.ErrorCodeChannelGetError)
			}
			//c.Set(constant.ContextKeyRequestStartTime, time.Now())
			common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
			middleware.SetupContextForSelectedChannel(c, channel, c.GetString("original_model"))
		} else {
			channel, err = getChannel(c, group, originalModel, i)
		}
		//var newAPIError *types.NewAPIError
		if err != nil {
			logger.LogError(c, err.Error())
			break
		}
		//newAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed)

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
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
			return
		}

		relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
		if err != nil {
			newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
			return
		}
		// Initialize channel meta before using relayInfo
		relayInfo.InitChannelMeta(c)

		meta := request.GetTokenCountMeta()

		if setting.ShouldCheckPromptSensitive() {
			contains, words := service.CheckSensitiveText(meta.CombineText)
			if contains {
				logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
				newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
				return
			}
		}

		tokens, err := service.CountRequestToken(c, meta, relayInfo)
		if err != nil {
			newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
			return
		}

		relayInfo.SetPromptTokens(tokens)

		priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
		if err != nil {
			newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
			return
		}

		// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

		newAPIError = service.PreConsumeQuota(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}

		defer func() {
			// Only return quota if downstream failed and quota was actually pre-consumed
			if newAPIError != nil && relayInfo.FinalPreConsumedQuota != 0 {
				service.ReturnPreConsumedQuota(c, relayInfo)
			}
		}()

		addUsedChannel(c, channel.Id)
		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

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

		if newAPIError == nil {
			return
		}

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		body := "{}"
		openaiError := newAPIError.ToOpenAIError()
		if newAPIError.StatusCode == 413 || newAPIError.StatusCode == 429 ||
			strings.Contains(strings.ToLower(openaiError.Message), "too many") {
			body = "{}"
		} else {
			bodyBytes, _ := common.GetRequestBody(c)
			body = string(bodyBytes)
		}

		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
		clientUserId := common.GetContextKeyString(c, constant.ContextKeyClientUserId)
		go model.SaveErrorLog(c.GetInt("id"), channel.Id, channel.Name, originalModel, openaiError, body, requestId, c.ClientIP(), tokenId, clientUserId)

		if !shouldRetry(c, newAPIError, retryTimes-i) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// if newAPIError != nil {
	// 	newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
	// 	c.JSON(newAPIError.StatusCode, gin.H{
	// 		"error": newAPIError.ToOpenAIError(),
	// 	})
	// }
	//newAPIError.ErrorType = types.ErrorTypeNewAPIError
	//newAPIError.SetErrorCode(types.ErrorCode(newAPIError.GetErrorCode()))
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func WssRelay(c *gin.Context) {
	// 将 HTTP 连接升级为 WebSocket 连接

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	defer ws.Close()

	if err != nil {
		helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed).ToOpenAIError())
		return
	}

	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	requestId := c.GetString(common.RequestIdKey)
	group := c.GetString("group")
	//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
	originalModel := c.GetString("original_model")
	var newAPIError *types.NewAPIError

	retryTimes := common.RetryTimes
	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}

	for i := 0; i <= retryTimes; i++ {
		channel, err := getChannel(c, group, originalModel, i)
		if err != nil {
			logger.LogError(c, err.Error())
			newAPIError = err
			break
		}

		newAPIError = wssRequest(c, ws, relayMode, channel)

		if newAPIError == nil {
			return // 成功处理请求，直接返回
		}

		go processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		if !shouldRetry(c, newAPIError, retryTimes-i) {
			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	if newAPIError != nil {
		//if newAPIError.StatusCode == http.StatusTooManyRequests {
		//	newAPIError.SetMessage("当前分组上游负载已饱和，请稍后再试")
		//}
		newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
		helper.WssError(c, ws, newAPIError.ToOpenAIError())
	}
}

func RelayClaude(c *gin.Context) {
	//relayMode := constant.Path2RelayMode(c.Request.URL.Path)
	requestId := c.GetString(common.RequestIdKey)
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	//var claudeErr *dto.ClaudeErrorWithStatusCode
	retryTimes := common.RetryTimes
	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}
	var newAPIError *types.NewAPIError
	tokenChannelIdsAny, ok := c.Get("token_channel_ids")
	var tokenChannelIds []int
	if ok {
		tokenChannelIds = tokenChannelIdsAny.([]int)
	}
	var channel *model.Channel
	var err *types.NewAPIError
	var err1 error

	for i := 0; i <= retryTimes; i++ {
		if len(tokenChannelIds) > 0 {
			if i >= len(tokenChannelIds) {
				break
			}
			channel, err1 = model.GetChannelById(tokenChannelIds[i], true)
			if err1 != nil {
				err = types.NewError(err1, types.ErrorCodeChannelGetError)
			}
			common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
			middleware.SetupContextForSelectedChannel(c, channel, c.GetString("original_model"))
		} else {
			channel, err = getChannel(c, group, originalModel, i)
		}
		if err != nil {
			logger.LogError(c, err.Error())
			newAPIError = err
			break
		}

		newAPIError = claudeRequest(c, channel)

		if newAPIError == nil {
			return // 成功处理请求，直接返回
		}

		go processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		body := "{}"
		openaiError := newAPIError.ToOpenAIError()
		if newAPIError.StatusCode == 413 || newAPIError.StatusCode == 429 ||
			strings.Contains(strings.ToLower(openaiError.Message), "too many") {
			body = "{}"
		} else {
			bodyBytes, _ := common.GetRequestBody(c)
			body = string(bodyBytes)
		}
		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
		clientUserId := common.GetContextKeyString(c, constant.ContextKeyClientUserId)
		go model.SaveErrorLog(c.GetInt("id"), channel.Id, channel.Name, originalModel, openaiError, body, requestId, c.ClientIP(), tokenId, clientUserId)

		//go processChannelError(c, channel.Id, channel.Type, channel.Name, channel.GetAutoBan(), openaiErr)

		//if !shouldRetry(c, openaiErr, retryTimes-i) {
		if !shouldRetry(c, newAPIError, retryTimes-i) {
			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	if newAPIError != nil {
		newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
		c.JSON(newAPIError.StatusCode, gin.H{
			"type":  "error",
			"error": newAPIError.ToClaudeError(),
		})
	}
}

func relayRequest(c *gin.Context, relayMode int, channel *model.Channel) *types.NewAPIError {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	if err != nil {
		logger.LogError(c, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
	}
	return relayHandler(c, info)
}

func wssRequest(c *gin.Context, ws *websocket.Conn, relayMode int, channel *model.Channel) *types.NewAPIError {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIRealtime, nil, ws)
	if err != nil {
		logger.LogError(c, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
	}
	return relay.WssHelper(c, info)
}

func claudeRequest(c *gin.Context, channel *model.Channel) *types.NewAPIError {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatClaude, nil, nil)
	if err != nil {
		logger.LogError(c, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
	}
	return relay.ClaudeHelper(c, info)
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func getChannel(c *gin.Context, group, originalModel string, retryCount int) (*model.Channel, *types.NewAPIError) {
	if retryCount == 0 {
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
	tags := make([]string, 0)
	tagsAny, t := c.Get("multi_model_tags")
	if t {
		tags = tagsAny.([]string)
	}
	channel, selectGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, retryCount, tags)
	//channel, selectGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, retryCount)
	if err != nil {
		if group == "auto" {
			//return nil, types.NewError(errors.New(fmt.Sprintf("获取自动分组下模型 %s 的可用渠道失败: %s", originalModel, err.Error())), types.ErrorCodeGetChannelFailed)
			return nil, types.NewError(fmt.Errorf("获取自动分组下模型 %s 的可用渠道失败: %s", originalModel, err.Error()), types.ErrorCodeGetChannelFailed)
		}
		//return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", selectGroup, originalModel, err.Error()), types.ErrorCodeGetChannelFailed)
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, originalModel), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, originalModel)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
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
	if openaiErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if openaiErr.StatusCode/100 == 2 {
		return false
	}

	return true
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(channelError.ChannelId, err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.Error())
		})
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
		other["admin_info"] = adminInfo
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveError(), tokenId, 0, false, userGroup, other)
	}

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
	err := dto.OpenAIError{
		Message: "API not implemented",
		Type:    "toio_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := dto.OpenAIError{
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
	if c.GetInt("new_retry_times") > 0 {
		retryTimes = c.GetInt("new_retry_times")
	}
	channelId := c.GetInt("channel_id")
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return
	}
	taskErr := taskRelayHandler(c, relayInfo)
	if taskErr == nil {
		retryTimes = 0
	}
	tags := make([]string, 0)
	tagsAny, tagOk := c.Get("multi_model_tags")
	if tagOk {
		tags = tagsAny.([]string)
	}
	for i := 0; shouldRetryTaskRelay(c, channelId, taskErr, retryTimes) && i < retryTimes; i++ {
		channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, i, tags)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", err.Error()))
			/*
						channel, newAPIError := getChannel(c, group, originalModel, i)
						if newAPIError != nil {
							common.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
							taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
				channel, newAPIError := getChannel(c, group, originalModel, i)
				if newAPIError != nil {
					logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
					taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			*/
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, i))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayInfo)
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
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		err = relay.RelayTaskFetch(c, relayInfo.RelayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
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
