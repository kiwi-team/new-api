package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	filterParmas(request)
	transParmas(request, info)

	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	requestStr := ""
	responseStr := ""
	// 序列化 textRequest 为 JSON 字符串
	if saveRequestResponse {
		requestBytes, marshalErr := common.Marshal(request)
		if marshalErr != nil {
			logger.LogError(c, fmt.Sprintf("marshal textRequest failed: %s", marshalErr.Error()))
		} else {
			requestStr = string(requestBytes)
		}
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !lo.FromPtrOr(request.Stream, false) {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	passThroughGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	if info.RelayMode == relayconstant.RelayModeChatCompletions &&
		!passThroughGlobal &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		applySystemPromptIfNeeded(c, info, request)
		usage, newApiErr := chatCompletionsViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		var containAudioTokens = usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
		var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

		if containAudioTokens && containsAudioRatios {
			service.PostAudioConsumeQuota(c, info, usage, "", "", "")
		} else {
			//extraContent := []string{}
			//postConsumeQuota(c, info, usage, extraContent, "", "")
			service.PostTextConsumeQuota(c, info, usage, nil, "", "")
		}
		return nil
	}

	var requestBody io.Reader

	if passThroughGlobal || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.DebugEnabled {
			if debugBytes, bErr := storage.Bytes(); bErr == nil {
				logger.LogDebug(c, "requestBody: %s", debugBytes)
			}
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err1 := adaptor.ConvertOpenAIRequest(c, info, request)
		//common.PrintJson("\n 66666 convertedRequest:\n", convertedRequest)
		if err1 != nil {
			return types.NewError(err1, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		if info.ChannelSetting.SystemPrompt != "" {
			// 如果有系统提示，则将其添加到请求中
			request, ok := convertedRequest.(*dto.GeneralOpenAIRequest)
			if ok {
				containSystemPrompt := false
				for _, message := range request.Messages {
					if message.Role == request.GetSystemRoleName() {
						containSystemPrompt = true
						break
					}
				}
				if !containSystemPrompt {
					// 如果没有系统提示，则添加系统提示
					systemMessage := dto.Message{
						Role:    request.GetSystemRoleName(),
						Content: info.ChannelSetting.SystemPrompt,
					}
					request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
				} else if info.ChannelSetting.SystemPromptOverride {
					common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
					// 如果有系统提示，且允许覆盖，则拼接到前面
					for i, message := range request.Messages {
						if message.Role == request.GetSystemRoleName() {
							if message.IsStringContent() {
								request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
							} else {
								contents := message.ParseContent()
								contents = append([]dto.MediaContent{
									{
										Type: dto.ContentTypeText,
										Text: info.ChannelSetting.SystemPrompt,
									},
								}, contents...)
								request.Messages[i].Content = contents
							}
							break
						}
					}
				}
			}
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		logger.LogDebug(c, "text request body: %s", jsonData)

		body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		info.UpstreamRequestBodySize = size
		requestBody = body
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}

	// 创建流式响应记录器（如果需要记录响应）
	var streamRecorder *helper.StreamResponseRecorder
	if saveRequestResponse && httpResp != nil {
		// 使用流式记录器包装原始响应体，不影响实时传输
		streamRecorder = helper.NewStreamResponseRecorder(httpResp.Body)
		httpResp.Body = streamRecorder
		defer streamRecorder.Release()
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}
	// 在流式传输完成后，从记录器中获取完整的响应数据
	if saveRequestResponse && streamRecorder != nil {
		responseStr = streamRecorder.GetRecordedString()
	}
	// 对于 SDK 渠道（如 AWS），从 context 获取响应字符串
	if saveRequestResponse && responseStr == "" {
		if sdkRespStr, exists := c.Get(string(constant.ContextKeySdkResponseStr)); exists {
			responseStr = sdkRespStr.(string)
		}
	}

	//if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
	//	service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	//} else {
	//	postConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	//var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	//var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	} else {
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil, requestStr, responseStr)
	}
	return nil

}

func transParmas(textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) error {
	isOpenRouter := info.ChannelType == common.ChannelTypeOpenRouter || strings.Contains(info.ChannelBaseUrl, "openrouter")
	isChat := strings.Contains(info.ChannelBaseUrl, "chataiapi")
	isNuwa := strings.Contains(info.ChannelBaseUrl, "nuwaapi")
	isYunwu := strings.Contains(info.ChannelBaseUrl, "yunwu")
	isSiliconflow := strings.Contains(info.ChannelBaseUrl, "siliconflow")
	// https://docs.siliconflow.cn/cn/api-reference/chat-completions/chat-completions#body-enable-thinking
	// 硅基当前只有部分模型支持 enable_thinking
	/*
		- zai-org/GLM-4.6
		- Qwen/Qwen3-8B
		- Qwen/Qwen3-14B
		- Qwen/Qwen3-32B
		- wen/Qwen3-30B-A3B
		- Qwen/Qwen3-235B-A22B
		- tencent/Hunyuan-A13B-Instruct
		- zai-org/GLM-4.5V
		- deepseek-ai/DeepSeek-V3.1
		- Pro/deepseek-ai/DeepSeek-V3.1
	*/
	supportedModels := []string{
		"glm-4.6",
		"qwen3-8b",
		"qwen3-14b",
		"qwen3-32b",
		"qwen3-30b-a3b",
		"qwen3-235b-a22b",
		"hunyuan-a13b-instruct",
		"glm-4.5v",
		"deepseek-v3.1",
		"zai-org/GLM-4.6",
		"Qwen/Qwen3-8B",
		"Qwen/Qwen3-14B",
		"Qwen/Qwen3-32B",
		"wen/Qwen3-30B-A3B",
		"Qwen/Qwen3-235B-A22B",
		"tencent/Hunyuan-A13B-Instruct",
		"zai-org/GLM-4.5V",
		"deepseek-ai/DeepSeek-V3.1",
		"Pro/deepseek-ai/DeepSeek-V3.1",
	}
	isVolcengine := strings.Contains(info.ChannelBaseUrl, "volces")
	isDeepseek := strings.Contains(info.ChannelBaseUrl, "deepseek")
	isMoonshot := strings.Contains(info.ChannelBaseUrl, "moonshot")
	isPPIO := strings.Contains(info.ChannelBaseUrl, "ppinfra")
	isGlm := strings.Contains(strings.ToLower(info.UpstreamModelName), "glm")
	// openrouter 用的是openai的格式，但是claude的模型需要开启thinking
	var thinking dto.AnthropicThinking

	if textRequest.THINKING != nil {
		err := json.Unmarshal(textRequest.THINKING, &thinking)
		if err != nil {
			return err
		}
	}
	if isOpenRouter && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			if thinking.BudgetTokens > 0 {
				reasoning := openrouter.RequestReasoning{
					MaxTokens: thinking.BudgetTokens,
				}
				reasoningJSON, err := json.Marshal(reasoning)
				if err != nil {
					return fmt.Errorf("failed to marshal reasoning: %w", err)
				}
				textRequest.Reasoning = reasoningJSON
			} else {
				reasoning := openrouter.RequestReasoning{
					Enabled: common.GetPointer(true),
				}
				if textRequest.ReasoningEffort != "" {
					reasoning.Effort = textRequest.ReasoningEffort
				}
				reasoningJSON, err := json.Marshal(reasoning)
				if err != nil {
					return fmt.Errorf("failed to marshal reasoning: %w", err)
				}
				textRequest.Reasoning = reasoningJSON
			}
		} else if thinking.Type == "disabled" {
			reasoning := openrouter.RequestReasoning{
				Enabled: common.GetPointer(false),
			}
			reasoningJSON, _ := json.Marshal(reasoning)
			textRequest.Reasoning = reasoningJSON
		}
	} else if isChat && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" || strings.Contains(textRequest.Model, "claude") {
				textRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking") + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
		} else {
			textRequest.THINKING = json.RawMessage(`{"type": "disabled"}`)
		}
	} else if isNuwa && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking") + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
			if !strings.Contains(textRequest.Model, "claude") &&
				!strings.Contains(textRequest.Model, "gemini") {
				if thinking.BudgetTokens > 0 {
					// extraBody.Google.ThinkingConfig.IncludeThoughts = true
					// extraBody.Google.ThinkingConfig.ThinkingBudget = thinking.BudgetTokens
					// extraBodyJSON, err := json.Marshal(struct {
					// 	ExtraBody dto.ExtraBody `json:"extra_body"`
					// }{ExtraBody: extraBody})
					// if err != nil {
					// 	return fmt.Errorf("failed to marshal reasoning: %w", err)
					// }
					// textRequest.ExtraBody = extraBodyJSON
					//OpenAI API 提供三种思考控制级别："low"、"medium" 和 "high"，分别对应于 1,024、8,192 和 24,576 个令牌
					if thinking.BudgetTokens >= 24576 {
						textRequest.ReasoningEffort = "high"
					} else if thinking.BudgetTokens >= 8000 {
						textRequest.ReasoningEffort = "medium"
					} else {
						textRequest.ReasoningEffort = "low"
					}

				} else {
					if textRequest.ReasoningEffort == "" {
						textRequest.ReasoningEffort = "medium"
					}
				}
			}
		} else if thinking.Type != "adaptive" {
			//if strings.Contains(strings.ToLower(textRequest.Model), "gemini") {
			if textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = textRequest.Model + "-nothinking"
				info.UpstreamModelName = textRequest.Model
			}
			textRequest.THINKING = json.RawMessage(`{"type": "disabled"}`)
		}
		// claude-opus-4-1-20250805 开启thinking后，对于nuwa来说，要append一个空的message item
		if strings.Contains(textRequest.Model, "claude-opus-4-1") {
			textRequest.Messages = append(textRequest.Messages, dto.Message{
				Role:    "user",
				Content: "",
			})
		}
	} else if isYunwu && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking") + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
		} else if thinking.Type != "adaptive" {
			if textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = textRequest.Model + "-nothinking"
				info.UpstreamModelName = textRequest.Model
			}
			textRequest.THINKING = json.RawMessage(`{"type": "disabled"}`)
		}
	} else if isSiliconflow && textRequest.THINKING != nil {
		if thinking.Type == "enabled" && slices.Contains(supportedModels, textRequest.Model) {
			textRequest.SetEnableThinking(true)
			if thinking.BudgetTokens > 0 {
				textRequest.ThinkingBudget = thinking.BudgetTokens
			}
		} else {
			textRequest.EnableThinking = nil
		}
	} else if isPPIO {
		defaultEnbaledThinking := strings.Contains(textRequest.Model, "glm-4.7") ||
			strings.Contains(textRequest.Model, "glm-5") ||
			strings.Contains(textRequest.Model, "glm-4.5v")
		if textRequest.THINKING == nil && defaultEnbaledThinking {
			// https://docs.bigmodel.cn/cn/guide/capabilities/thinking#%E6%A0%B8%E5%BF%83%E5%8F%82%E6%95%B0%E8%AF%B4%E6%98%8E
			//enabled（默认）：启用动态思考，glm-4.7 glm-4.5v为强制思考，其它模型自动判断是否需要深度思考
			//textRequest.SetEnableThinking(true)
			// 默认是要开启思考的，和官方保持一致的行为
			textRequest.THINKING = json.RawMessage(`{"type": "enabled"}`)
		} else if strings.Contains(textRequest.Model, "glm-4.7-flash") {
			if textRequest.THINKING != nil {
				if thinking.Type == "enabled" {
					textRequest.SetEnableThinking(true)
				} else if thinking.Type == "disabled" {
					textRequest.SetEnableThinking(false)
				}
			}
		}
	} else if info.ChannelType == common.ChannelTypeAli {
		if strings.Contains(textRequest.Model, "glm") {
			if textRequest.THINKING != nil && thinking.Type == "disabled" {
				textRequest.SetEnableThinking(false)
			}
		} else if strings.Contains(textRequest.Model, "kimi") {
			if textRequest.THINKING != nil && thinking.Type == "disabled" {
				textRequest.SetEnableThinking(false)
			} else if textRequest.THINKING != nil && thinking.Type == "enabled" {
				textRequest.SetEnableThinking(true)
			} else {
				// 和官方的kimi-2.5的行为保持一一致，默认是开启thinking的
				textRequest.SetEnableThinking(true)
			}
		} else if strings.Contains(strings.ToLower(textRequest.Model), "minimax") {
			// 阿里云的minimax 只能开启thinking
			textRequest.SetEnableThinking(true)
		}
	}

	if isSiliconflow {
		if !slices.Contains(supportedModels, textRequest.Model) {
			textRequest.EnableThinking = nil
		}
	}

	if textRequest.Model == "deepseek-reasoner" {
		if isSiliconflow {
			textRequest.SetEnableThinking(true)
		} else if isVolcengine {
			textRequest.THINKING = json.RawMessage(`{"type": "enabled"}`)
		}
	}
	// 对于deepseek和moonshot官方的api，assistant 的content只能是字符串
	if isDeepseek || isMoonshot || isGlm {
		for i, msg := range textRequest.Messages {
			if msg.Role != "user" {
				cnt := msg.ParseContent()
				if len(cnt) == 1 && cnt[0].Type == "text" {
					textRequest.Messages[i].Content = cnt[0].Text
				}
			}
		}
	}

	// 把claude的stop 设置为空
	if strings.Contains(textRequest.Model, "claude") {
		textRequest.Stop = nil
	}

	return nil
}

func filterParmas(textRequest *dto.GeneralOpenAIRequest) {
	filterConfig := common.OptionMap["ModelParamsFilter"]
	if filterConfig == "" {
		return
	}
	filterConfigMap := &model.ModelParamsFilterMap{}
	err := json.Unmarshal([]byte(filterConfig), filterConfigMap)
	if err != nil {
		common.SysError("filterParmas Unmarshal failed: " + err.Error())
		return
	}
	if filterConfigMap.SetTopKZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetTopKZero, textRequest.Model)
		if hasSet {
			textRequest.TopK = nil
		} else {
			for _, model := range *filterConfigMap.SetTopKZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.TopK = nil
					hasSet = true
					break
				}
			}
		}
	}
	if filterConfigMap.SetTopPZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetTopPZero, textRequest.Model)
		if hasSet {
			textRequest.TopP = nil
			hasSet = true
		} else {
			for _, model := range *filterConfigMap.SetTopPZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.TopP = nil
					hasSet = true
					break
				}
			}
		}
	}
	if filterConfigMap.SetMaxTokensZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetMaxTokensZero, textRequest.Model)
		if hasSet {
			textRequest.MaxTokens = nil
		} else {
			for _, model := range *filterConfigMap.SetMaxTokensZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.MaxTokens = nil
					break
				}
			}
		}
	}
	if filterConfigMap.SetTemperatureZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetTemperatureZero, textRequest.Model)
		if hasSet {
			textRequest.Temperature = nil
		} else {
			for _, model := range *filterConfigMap.SetTemperatureZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.Temperature = nil
					break
				}
			}
		}
	}
	if filterConfigMap.SetTemperatureOne != nil {
		hasSet := slices.Contains(*filterConfigMap.SetTemperatureOne, textRequest.Model)
		if hasSet {
			temp := 1.0
			textRequest.Temperature = &temp
		} else {
			for _, model := range *filterConfigMap.SetTemperatureOne {
				if common.RegMatch(model, textRequest.Model) {
					temp := 1.0
					textRequest.Temperature = &temp
					break
				}
			}
		}
	}
	if filterConfigMap.SetMaxTokens != nil {
		for _, item := range *filterConfigMap.SetMaxTokens {
			// get key from item
			for key, val := range item {
				if key == textRequest.Model {
					if textRequest.MaxTokens != nil && int(*textRequest.MaxTokens) > val && val > 0 {
						textRequest.MaxTokens = lo.ToPtr(uint(val))
						break
					}
				}
			}
			for key, val := range item {
				if common.RegMatch(key, textRequest.Model) {
					if textRequest.MaxTokens != nil && int(*textRequest.MaxTokens) > val && val > 0 {
						textRequest.MaxTokens = lo.ToPtr(uint(val))
						break
					}
				}
			}
		}
	}
	if filterConfigMap.SetStopNil != nil {
		hasSet := slices.Contains(*filterConfigMap.SetStopNil, textRequest.Model)
		if hasSet {
			textRequest.Stop = nil
		} else {
			for _, item := range *filterConfigMap.SetStopNil {
				if common.RegMatch(item, textRequest.Model) {
					textRequest.Stop = nil
					break
				}
			}
		}
	}
}

func uploadFileToS3(textRequest *dto.GeneralOpenAIRequest) {
	modelName := strings.ToLower(textRequest.Model)
	// kimi的模型，不支持image_url的图片。只支持base64的图片
	if strings.Contains(modelName, "kimi") || strings.Contains(modelName, "moonshot") {
		return
	}
	ctx := context.Background()
	bucket := common.OptionMap["S3Bucket"]
	endpoint := common.OptionMap["S3Endpoint"]
	s3AK := common.OptionMap["S3AK"]
	s3SK := common.OptionMap["S3SK"]
	s3Region := common.OptionMap["S3Region"]
	if bucket == "" || endpoint == "" || s3AK == "" || s3SK == "" || s3Region == "" {
		return
	}
	s3Client := s3.New(s3.Options{
		Region:      s3Region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(s3AK, s3SK, "")),
	})
	for i, message := range textRequest.Messages {
		contentArr := message.ParseContent()
		for j, item := range contentArr {
			switch item.Type {
			case dto.ContentTypeImageURL:
				// 上传图片到 S3
				fileURL, err := service.UploadFileToS3(ctx, s3Client, bucket, endpoint, item.GetImageMedia().Url)
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("upload image to s3 failed: %s", err.Error()))
				} else {
					// 替换 content 中的 url 为 s3 url
					contentArr[j].ImageUrl = dto.MessageImageUrl{
						Url:      fileURL,
						Detail:   item.GetImageMedia().Detail,
						MimeType: item.GetImageMedia().MimeType,
					}
				}
			case dto.ContentTypeAudioUrl:
				// 上传音频到 S3
				fileURL, err := service.UploadFileToS3(ctx, s3Client, bucket, endpoint, item.GetAudioMedia().Url)
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("upload audio to s3 failed: %s", err.Error()))
				} else {
					// 替换 content 中的 url 为 s3 url
					contentArr[j].AudioUrl = dto.MessageAudioUrl{
						Url:    fileURL,
						Detail: item.GetAudioMedia().Detail,
					}
				}
			case dto.ContentTypeVideoUrl:
				// 上传视频到 S3
				fileURL, err := service.UploadFileToS3(ctx, s3Client, bucket, endpoint, item.GetVideoMedia().Url)
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("upload video to s3 failed: %s", err.Error()))
				} else {
					// 替换 content 中的 url 为 s3 url
					contentArr[j].VideoUrl = dto.MessageVideoUrl{
						Url: fileURL,
					}
				}
			}
		}
		textRequest.Messages[i].Content = contentArr
	}
}

func getPromptTokens(c *gin.Context, textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) (int, error) {
	var promptTokens int
	var err error
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions:
		meta := textRequest.GetTokenCountMeta()
		//promptTokens, err = service.CountRequestToken(c, meta, info)
		promptTokens, err = service.EstimateRequestToken(c, meta, info)
	case relayconstant.RelayModeCompletions:
		promptTokens = service.CountTokenInput(textRequest.Prompt, textRequest.Model)
	case relayconstant.RelayModeModerations:
		promptTokens = service.CountTokenInput(textRequest.Input, textRequest.Model)
	case relayconstant.RelayModeEmbeddings:
		promptTokens = service.CountTokenInput(textRequest.Input, textRequest.Model)
	default:
		err = errors.New("unknown relay mode")
		promptTokens = 0
	}
	info.Usage.PromptTokens = promptTokens
	return promptTokens, err
}

func checkRequestSensitive(textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) ([]string, error) {
	var err error
	var words []string
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions:
		words, err = service.CheckSensitiveMessages(textRequest.Messages)
	case relayconstant.RelayModeCompletions:
		//words, err = service.CheckSensitiveInput(textRequest.Prompt)
	case relayconstant.RelayModeModerations:
		//words, err = service.CheckSensitiveInput(textRequest.Input)
	case relayconstant.RelayModeEmbeddings:
		//words, err = service.CheckSensitiveInput(textRequest.Input)
	}
	return words, err
}
