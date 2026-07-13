package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	//old	if len(textRequest.Messages) == 0 {
	//		return nil, errors.New("field messages is required")
	//	}
	//	if textRequest.Model == "" {
	//		return nil, errors.New("field model is required")
	//	}
	//	return textRequest, nil
	//}

	// old
	//	err = helper.ModelMappedHelper(c, info, request)
	//	if err != nil {
	//		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	//	}
	//
	//	if textRequest.Stream {
	//		relayInfo.IsStream = true
	//	}
	//
	//	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	//	requestStr := ""
	//	responseStr := ""
	//
	//	err = helper.ModelMappedHelper(c, relayInfo, textRequest)
	//	if err != nil {
	//		return types.NewError(err, types.ErrorCodeChannelModelMappedError)
	//	}
	//
	//	promptTokens, err := getClaudePromptTokens(textRequest, relayInfo)
	//	// count messages token error 计算promptTokens错误
	//	if err != nil {
	//		return types.NewError(err, types.ErrorCodeCountTokenFailed)
	//	}
	//
	//	priceData, err := helper.ModelPriceHelper(c, relayInfo, promptTokens, int(textRequest.MaxTokens))
	//	if err != nil {
	//		return types.NewError(err, types.ErrorCodeModelPriceError)
	//	}
	//
	//	// pre-consume quota 预消耗配额
	//	preConsumedQuota, userQuota, newAPIError := preConsumeQuota(c, priceData.ShouldPreConsumedQuota, relayInfo)
	//
	//	if newAPIError != nil {
	//		return newAPIError
	//	}
	//	defer func() {
	//		if newAPIError != nil {
	//			returnPreConsumedQuota(c, relayInfo, userQuota, preConsumedQuota)
	//		}
	//	}()
	//

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	if request.Stream {
		info.IsStream = true
	}

	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	requestStr := ""
	responseStr := ""
	// old
	//adaptor := GetAdaptor(relayInfo.ApiType)
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// Claude Code 客户端检测：渠道开启后，仅放行真实 Claude Code 客户端请求。
	// 检测以「客户端原始请求头叠加渠道 Header Override 后的合并结果」为准，即在请求头覆盖之后进行，
	// 使渠道对头的改写/透传/{client_header} 占位符都能被检测感知。
	// 未通过检测视为该渠道请求失败——用 channel: 前缀错误码触发跨渠道重试并写入 error_logs，
	// 同时该错误码在 ShouldDisableChannel 中被显式跳过，不会误禁用当前渠道。
	if info.ChannelSetting.ClaudeCodeGuardEnabled {
		guardHeader, headerErr := channel.BuildOverriddenClientHeader(c, info)
		if headerErr != nil {
			return types.NewErrorWithStatusCode(headerErr, types.ErrorCodeChannelHeaderOverrideInvalid, http.StatusBadRequest)
		}
		if guardErr := claude.DetectClaudeCode(request, guardHeader, c.Request.URL.Path); guardErr != nil {
			return types.NewErrorWithStatusCode(guardErr, types.ErrorCodeChannelClaudeCodeGuardReject, http.StatusBadRequest)
		}
	}

	if request.MaxTokens == 0 {
		request.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
	}

	isOpus47 := strings.HasPrefix(request.Model, "claude-opus-4-7")
	isOpus46 := strings.HasPrefix(request.Model, "claude-opus-4-6")
	isOpus48 := strings.HasPrefix(request.Model, "claude-opus-4-8")

	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(request.Model); ok && effortLevel != "" &&
		(isOpus46 || isOpus47 || isOpus48) {
		request.Model = baseModel
		request.Thinking = &dto.Thinking{
			Type: "adaptive",
		}
		request.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
		request.TopP = 0
		request.Temperature = common.GetPointer[float64](1.0)
		info.UpstreamModelName = request.Model
	} else if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(request.Model, "-thinking") {
		if request.Thinking == nil {
			// 因为BudgetTokens 必须大于1024
			if request.MaxTokens < 1280 {
				request.MaxTokens = 1280
			}

			// BudgetTokens 为 max_tokens 的 80%
			request.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](int(float64(request.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
			}
			// TODO: 临时处理
			// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
			request.TopP = 0
			request.Temperature = common.GetPointer[float64](1.0)
		}
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
			request.Model = strings.TrimSuffix(request.Model, "-thinking")
		}
		info.UpstreamModelName = request.Model
	}

	// claude-opus-4-7 / claude-opus-4-8 breaking changes:
	// 1. thinking: {type: "enabled"} returns 400 → must use {type: "adaptive"}
	// 2. temperature/top_p/top_k non-default values return 400
	if isOpus47 || isOpus48 {
		if request.Thinking != nil && request.Thinking.Type == "enabled" {
			request.Thinking = &dto.Thinking{
				Type: "adaptive",
			}
			if request.OutputConfig == nil {
				request.OutputConfig = json.RawMessage(`{"effort":"high"}`)
			}
		}
		request.Temperature = nil
		request.TopP = 0
		request.TopK = 0
		request.Model = strings.TrimSuffix(request.Model, "-thinking")
		info.UpstreamModelName = request.Model
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		openAIRequest, convErr := service.ClaudeToOpenAIRequest(*request, info)
		if convErr != nil {
			return types.NewError(convErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		usage, newApiErr := chatCompletionsViaResponses(c, info, adaptor, openAIRequest)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostClaudeConsumeQuota(c, info, usage, requestStr, responseStr)
		return nil
	}

	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		//requestBody = bytes.NewBuffer(body)
		requestBody = common.ReaderOnly(storage)
		// 序列化 textRequest 为 JSON 字符串
		if saveRequestResponse {
			val, _ := io.ReadAll(requestBody)
			requestStr = string(val)
		}
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride, relaycommon.BuildParamOverrideContext(info))
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
			}
		}

		// 在最终请求体发往上游 Claude 渠道之前做参数校验（此时各种参数转换均已完成）。
		// 拦截会被上游拒绝的非法参数（如空文本内容块、thinking 下设置了 top_k 等），
		// 直接返回参数异常，不再透传给上游。
		if validateErr := claude.ValidateClaudeRequestBody(jsonData); validateErr != nil {
			return types.NewErrorWithStatusCode(validateErr, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)

		// 序列化 textRequest 为 JSON 字符串
		if saveRequestResponse {
			requestStr = string(jsonData)
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	var streamRecorder *helper.StreamResponseRecorder
	if saveRequestResponse && httpResp != nil {
		// 使用流式记录器包装原始响应体，不影响实时传输
		streamRecorder = helper.NewStreamResponseRecorder(httpResp.Body)
		httpResp.Body = streamRecorder
		defer streamRecorder.Release()
	}

	//usage, newAPIError := adaptor.DoResponse(c, httpResp, relayInfo)
	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	//log.Printf("usage: %v", usage)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// 在流式传输完成后，从记录器中获取完整的响应数据
	if saveRequestResponse && streamRecorder != nil {
		responseStr = streamRecorder.GetRecordedString()
	}

	service.PostClaudeConsumeQuota(c, info, usage.(*dto.Usage), requestStr, responseStr)
	//service.PostClaudeConsumeQuota(c, info, usage.(*dto.Usage))
	return nil
}
