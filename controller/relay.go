package controller

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/middleware"
	"one-api/model"
	"one-api/relay"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, relayMode int) *types.NewAPIError {
	var err *types.NewAPIError
	switch relayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, relayMode)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c)
	case relayconstant.RelayModeResponses:
		err = relay.ResponsesHelper(c)
	case relayconstant.RelayModeGemini:
		err = relay.GeminiHelper(c)
	default:
		err = relay.TextHelper(c)
	}

	if constant.ErrorLogEnabled && err != nil {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		other["error_type"] = err.ErrorType
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")

		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.Error(), tokenId, 0, false, userGroup, other)
	}

	return err
}

func Relay(c *gin.Context) {
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	requestId := c.GetString(common.RequestIdKey)
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	//var openaiErr *dto.OpenAIErrorWithStatusCode
	var newAPIError *types.NewAPIError
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
			common.LogError(c, err.Error())
			//newAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed)
			break
		}

		newAPIError = relayRequest(c, relayMode, channel)

		if newAPIError == nil {
			return // 成功处理请求，直接返回
		}

		go processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		body, _ := common.GetRequestBody(c)
		go model.SaveErrorLog(c.GetInt("id"), channel.Id, channel.Name, originalModel, newAPIError, string(body), requestId, c.ClientIP())

		//if !shouldRetry(c, openaiErr, retryTimes-i) {
		if !shouldRetry(c, newAPIError, retryTimes-i) {
			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
	}

	// if openaiErr != nil {
	// 	if openaiErr.StatusCode == http.StatusTooManyRequests {
	// 		common.LogError(c, fmt.Sprintf("origin 429 error: %s", openaiErr.Error.Message))
	// 		openaiErr.Error.Message = "当前分组上游负载已饱和，请稍后再试"
	// 	}
	// 	openaiErr.Error.Message = common.MessageWithRequestId(openaiErr.Error.Message, requestId)
	// 	openaiErr.Error.Type = "toio_api_error"
	// 	c.JSON(openaiErr.StatusCode, gin.H{
	// 		"error": openaiErr.Error,
	// 	})
	// }
	// newAPIError will never be nil at this point since we're already in the error handling branch
	//if newAPIError.StatusCode == http.StatusTooManyRequests {
	//	common.LogError(c, fmt.Sprintf("origin 429 error: %s", newAPIError.Error()))
	//	newAPIError.SetMessage("当前分组上游负载已饱和，请稍后再试")
	//}
	if newAPIError != nil {
		newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
		c.JSON(newAPIError.StatusCode, gin.H{
			"error": newAPIError.ToOpenAIError(),
		})
	}
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
			common.LogError(c, err.Error())
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
		common.LogInfo(c, retryLogStr)
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

	for i := 0; i <= retryTimes; i++ {
		channel, err := getChannel(c, group, originalModel, i)
		if err != nil {
			common.LogError(c, err.Error())
			newAPIError = err
			break
		}

		newAPIError = claudeRequest(c, channel)

		if newAPIError == nil {
			return // 成功处理请求，直接返回
		}

		go processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		//go processChannelError(c, channel.Id, channel.Type, channel.Name, channel.GetAutoBan(), openaiErr)

		//if !shouldRetry(c, openaiErr, retryTimes-i) {
		if !shouldRetry(c, newAPIError, retryTimes-i) {
			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
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
	return relayHandler(c, relayMode)
}

func wssRequest(c *gin.Context, ws *websocket.Conn, relayMode int, channel *model.Channel) *types.NewAPIError {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return relay.WssHelper(c, ws)
}

func claudeRequest(c *gin.Context, channel *model.Channel) *types.NewAPIError {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return relay.ClaudeHelper(c)
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
	tagsAny, _ := c.Get("multi_model_tag")
	tags := tagsAny.([]string)
	channel, selectGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, retryCount, tags)
	//channel, selectGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, retryCount)
	if err != nil {
		if group == "auto" {
			//return nil, types.NewError(errors.New(fmt.Sprintf("获取自动分组下模型 %s 的可用渠道失败: %s", originalModel, err.Error())), types.ErrorCodeGetChannelFailed)
			return nil, types.NewError(fmt.Errorf("获取自动分组下模型 %s 的可用渠道失败: %s", originalModel, err.Error()), types.ErrorCodeGetChannelFailed)
		}
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", selectGroup, originalModel, err.Error()), types.ErrorCodeGetChannelFailed)
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
	if types.IsLocalError(openaiErr) {
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
		channelType := c.GetInt("channel_type")
		if channelType == constant.ChannelTypeAnthropic {
			return true
		}
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
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	common.LogError(c, fmt.Sprintf("relay error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	if service.ShouldDisableChannel(channelError.ChannelId, err) && channelError.AutoBan {
		service.DisableChannel(channelError, err.Error())
	}
}

func RelayMidjourney(c *gin.Context) {
	relayMode := c.GetInt("relay_mode")
	var err *dto.MidjourneyResponse
	switch relayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		err = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		err = relay.RelayMidjourneyTask(c, relayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		err = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		err = relay.RelaySwapFace(c)
	default:
		err = relay.RelayMidjourneySubmit(c, relayMode)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(err)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Code == 30 {
			err.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", err.Description, err.Result),
			"type":        "upstream_error",
			"code":        err.Code,
		})
		channelId := c.GetInt("channel_id")
		common.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", err.Description, err.Result)))
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
	relayMode := c.GetInt("relay_mode")
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	taskErr := taskRelayHandler(c, relayMode)
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
			common.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", err.Error()))
			/*
				channel, newAPIError := getChannel(c, group, originalModel, i)
				if newAPIError != nil {
					common.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
					taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			*/
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		common.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, i))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayMode)
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayMode int) *dto.TaskError {
	var err *dto.TaskError
	switch relayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeKlingFetchByID:
		err = relay.RelayTaskFetch(c, relayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayMode)
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
