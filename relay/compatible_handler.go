package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bytedance/gopkg/util/gopool"

	"github.com/shopspring/decimal"

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
		requestBytes, marshalErr := json.Marshal(request)
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
	if !info.SupportStreamOptions || !request.Stream {
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
			extraContent := []string{}
			postConsumeQuota(c, info, usage, extraContent, "", "")
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
				println("requestBody: ", string(debugBytes))
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

		jsonData, err2 := common.Marshal(convertedRequest)
		if err2 != nil {
			return types.NewError(err2, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
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

		logger.LogDebug(c, fmt.Sprintf("text request body: %s", string(jsonData)))

		requestBody = bytes.NewBuffer(jsonData)
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
	var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	extraContent := []string{}
	if containAudioTokens && containsAudioRatios {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	} else {
		postConsumeQuota(c, info, usage.(*dto.Usage), extraContent, requestStr, responseStr)
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
		if textRequest.THINKING != nil && thinking.Type == "enabled" {
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
					Effort: "medium",
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
		} else {
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
		} else {
			if textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = textRequest.Model + "-nothinking"
				info.UpstreamModelName = textRequest.Model
			}
			textRequest.THINKING = json.RawMessage(`{"type": "disabled"}`)
		}
	} else if isSiliconflow && textRequest.THINKING != nil {
		if thinking.Type == "enabled" && slices.Contains(supportedModels, textRequest.Model) {
			textRequest.EnableThinking = true
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
			//textRequest.EnableThinking = true
			// 默认是要开启思考的，和官方保持一致的行为
			textRequest.THINKING = json.RawMessage(`{"type": "enabled"}`)
		}
	}

	if isSiliconflow {
		if !slices.Contains(supportedModels, textRequest.Model) {
			textRequest.EnableThinking = nil
		}
	}

	if textRequest.Model == "deepseek-reasoner" {
		if isSiliconflow {
			textRequest.EnableThinking = true
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
			textRequest.TopK = 0
		} else {
			for _, model := range *filterConfigMap.SetTopKZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.TopK = 0
					hasSet = true
					break
				}
			}
		}
	}
	if filterConfigMap.SetTopPZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetTopPZero, textRequest.Model)
		if hasSet {
			textRequest.TopP = 0
			hasSet = true
		} else {
			for _, model := range *filterConfigMap.SetTopPZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.TopP = 0
					hasSet = true
					break
				}
			}
		}
	}
	if filterConfigMap.SetMaxTokensZero != nil {
		hasSet := slices.Contains(*filterConfigMap.SetMaxTokensZero, textRequest.Model)
		if hasSet {
			textRequest.MaxTokens = 0
		} else {
			for _, model := range *filterConfigMap.SetMaxTokensZero {
				if common.RegMatch(model, textRequest.Model) {
					textRequest.MaxTokens = 0
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
					if int(textRequest.MaxTokens) > val && val > 0 {
						textRequest.MaxTokens = uint(val)
						break
					}
				}
			}
			for key, val := range item {
				if common.RegMatch(key, textRequest.Model) {
					if int(textRequest.MaxTokens) > val && val > 0 {
						textRequest.MaxTokens = uint(val)
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

// 预扣费并返回用户剩余配额
func preConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) (int, int, *types.NewAPIError) {
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return 0, 0, types.NewError(err, types.ErrorCodeQueryDataError)
	}
	if userQuota <= 0 {
		return 0, 0, types.NewErrorWithStatusCode(errors.New("user quota is not enough"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	}
	if userQuota-preConsumedQuota < 0 {
		return 0, 0, types.NewErrorWithStatusCode(fmt.Errorf("pre-consume quota failed, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	}
	relayInfo.UserQuota = userQuota
	if userQuota > 100*preConsumedQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > 100*preConsumedQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				logger.LogInfo(c, fmt.Sprintf("user %d quota %s and token %d quota %d are enough, trusted and no need to pre-consume", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			logger.LogInfo(c, fmt.Sprintf("user %d with unlimited token has enough quota %s, trusted and no need to pre-consume", relayInfo.UserId, logger.FormatQuota(userQuota)))
		}
	}

	if preConsumedQuota > 0 {
		err := service.PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return 0, 0, types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return 0, 0, types.NewError(err, types.ErrorCodeUpdateDataError)
		}
	}
	return preConsumedQuota, userQuota, nil
}

func returnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo, userQuota int, preConsumedQuota int) {
	if preConsumedQuota != 0 {
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := service.PostConsumeQuota(&relayInfoCopy, -preConsumedQuota, 0, false)
			if err != nil {
				common.SysError("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

// old
// func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, preConsumedQuota int, userQuota int, priceData helper.PriceData, extraContent string, requestStr string, responseStr string) {
func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent []string, requestStr string, responseStr string) {
	//func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent ...string) {
	originUsage := usage
	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.GetEstimatePromptTokens(),
			CompletionTokens: 0,
			TotalTokens:      relayInfo.GetEstimatePromptTokens(),
		}
		extraContent = append(extraContent, "上游无计费信息")
	}

	if originUsage != nil {
		service.ObserveChannelAffinityUsageCacheFromContext(ctx, usage)
	}

	adminRejectReason := common.GetContextKeyString(ctx, constant.ContextKeyAdminRejectReason)

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	cacheTokens := usage.PromptTokensDetails.CachedTokens
	imageTokens := usage.PromptTokensDetails.ImageTokens
	audioTokens := usage.PromptTokensDetails.AudioTokens
	completionTokens := usage.CompletionTokens
	cachedCreationTokens := usage.PromptTokensDetails.CachedCreationTokens

	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	cacheRatio := relayInfo.PriceData.CacheRatio
	imageRatio := relayInfo.PriceData.ImageRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cachedCreationRatio := relayInfo.PriceData.CacheCreationRatio

	// Convert values to decimal for precise calculation
	dPromptTokens := decimal.NewFromInt(int64(promptTokens))
	dCacheTokens := decimal.NewFromInt(int64(cacheTokens))
	dImageTokens := decimal.NewFromInt(int64(imageTokens))
	dAudioTokens := decimal.NewFromInt(int64(audioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(completionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(cachedCreationTokens))
	dCompletionRatio := decimal.NewFromFloat(completionRatio)
	dCacheRatio := decimal.NewFromFloat(cacheRatio)
	dImageRatio := decimal.NewFromFloat(imageRatio)
	dModelRatio := decimal.NewFromFloat(modelRatio)
	dGroupRatio := decimal.NewFromFloat(groupRatio)
	dModelPrice := decimal.NewFromFloat(modelPrice)
	dCachedCreationRatio := decimal.NewFromFloat(cachedCreationRatio)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)

	// openai web search 工具计费
	var dWebSearchQuota decimal.Decimal
	var webSearchPrice float64
	// response api 格式工具计费
	if relayInfo.ResponsesUsageInfo != nil {
		if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool.CallCount > 0 {
			// 计算 web search 调用的配额 (配额 = 价格 * 调用次数 / 1000 * 分组倍率)
			webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, webSearchTool.SearchContextSize)
			dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent = append(extraContent, fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
				webSearchTool.CallCount, webSearchTool.SearchContextSize, dWebSearchQuota.String()))
		}
	} else if strings.HasSuffix(modelName, "search-preview") {
		// search-preview 模型不支持 response api
		searchContextSize := ctx.GetString("chat_completion_web_search_context_size")
		if searchContextSize == "" {
			searchContextSize = "medium"
		}
		webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
		dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		extraContent = append(extraContent, fmt.Sprintf("Web Search 调用 1 次，上下文大小 %s，调用花费 %s",
			searchContextSize, dWebSearchQuota.String()))
	}
	// claude web search tool 计费
	var dClaudeWebSearchQuota decimal.Decimal
	var claudeWebSearchPrice float64
	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
		dClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		extraContent = append(extraContent, fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			claudeWebSearchCallCount, dClaudeWebSearchQuota.String()))
	}
	// file search tool 计费
	var dFileSearchQuota decimal.Decimal
	var fileSearchPrice float64
	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			dFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent = append(extraContent, fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
				fileSearchTool.CallCount, dFileSearchQuota.String()))
		}
	}
	var dImageGenerationCallQuota decimal.Decimal
	var imageGenerationCallPrice float64
	if ctx.GetBool("image_generation_call") {
		imageGenerationCallPrice = operation_setting.GetGPTImage1PriceOnceCall(ctx.GetString("image_generation_call_quality"), ctx.GetString("image_generation_call_size"))
		dImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		extraContent = append(extraContent, fmt.Sprintf("Image Generation Call 花费 %s", dImageGenerationCallQuota.String()))
	}

	var quotaCalculateDecimal decimal.Decimal

	var audioInputQuota decimal.Decimal
	var audioInputPrice float64
	isClaudeUsageSemantic := relayInfo.FinalRequestRelayFormat == types.RelayFormatClaude
	if !relayInfo.PriceData.UsePrice {
		baseTokens := dPromptTokens
		// 减去 cached tokens
		// Anthropic API 的 input_tokens 已经不包含缓存 tokens，不需要减去
		// OpenAI/OpenRouter 等 API 的 prompt_tokens 包含缓存 tokens，需要减去
		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			if !isClaudeUsageSemantic {
				baseTokens = baseTokens.Sub(dCacheTokens)
			}
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}
		var dCachedCreationTokensWithRatio decimal.Decimal
		if !dCachedCreationTokens.IsZero() {
			if !isClaudeUsageSemantic {
				baseTokens = baseTokens.Sub(dCachedCreationTokens)
			}
			dCachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCachedCreationRatio)
		}

		// 减去 image tokens
		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			baseTokens = baseTokens.Sub(dImageTokens)
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		// 减去 Gemini audio tokens
		if !dAudioTokens.IsZero() {
			audioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(modelName)
			if audioInputPrice > 0 {
				// 重新计算 base tokens
				baseTokens = baseTokens.Sub(dAudioTokens)
				audioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
				extraContent = append(extraContent, fmt.Sprintf("Audio Input 花费 %s", audioInputQuota.String()))
			}
		}
		promptQuota := baseTokens.Add(cachedTokensWithRatio).
			Add(imageTokensWithRatio).
			Add(dCachedCreationTokensWithRatio)

		completionQuota := dCompletionTokens.Mul(dCompletionRatio)

		quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(ratio)

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
	} else {
		quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
	}
	// 添加 responses tools call 调用的配额
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dWebSearchQuota)
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dFileSearchQuota)
	// 添加 audio input 独立计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
	// 添加 image generation call 计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dImageGenerationCallQuota)

	if len(relayInfo.PriceData.OtherRatios) > 0 {
		for key, otherRatio := range relayInfo.PriceData.OtherRatios {
			dOtherRatio := decimal.NewFromFloat(otherRatio)
			quotaCalculateDecimal = quotaCalculateDecimal.Mul(dOtherRatio)
			extraContent = append(extraContent, fmt.Sprintf("其他倍率 %s: %f", key, otherRatio))
		}
	}

	quota := int(quotaCalculateDecimal.Round(0).IntPart())
	totalTokens := promptTokens + completionTokens

	//var logContent string

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		extraContent = append(extraContent, "上游没有返回计费信息，无法扣费（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if !ratio.IsZero() && quota == 0 {
			quota = 1
		}
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := service.SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := modelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", modelName))
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", modelName))
	}
	logContent := strings.Join(extraContent, ", ")
	other := service.GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if adminRejectReason != "" {
		other["reject_reason"] = adminRejectReason
	}
	// For chat-based calls to the Claude model, tagging is required. Using Claude's rendering logs, the two approaches handle input rendering differently.
	if isClaudeUsageSemantic {
		other["claude"] = true
		other["usage_semantic"] = "anthropic"
	}
	if imageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = imageRatio
		other["image_output"] = imageTokens
	}
	if cachedCreationTokens != 0 {
		other["cache_creation_tokens"] = cachedCreationTokens
		other["cache_creation_ratio"] = cachedCreationRatio
	}
	if !dWebSearchQuota.IsZero() {
		if relayInfo.ResponsesUsageInfo != nil {
			if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists {
				other["web_search"] = true
				other["web_search_call_count"] = webSearchTool.CallCount
				other["web_search_price"] = webSearchPrice
			}
		} else if strings.HasSuffix(modelName, "search-preview") {
			other["web_search"] = true
			other["web_search_call_count"] = 1
			other["web_search_price"] = webSearchPrice
		}
	} else if !dClaudeWebSearchQuota.IsZero() {
		other["web_search"] = true
		other["web_search_call_count"] = claudeWebSearchCallCount
		other["web_search_price"] = claudeWebSearchPrice
	}
	if !dFileSearchQuota.IsZero() && relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists {
			other["file_search"] = true
			other["file_search_call_count"] = fileSearchTool.CallCount
			other["file_search_price"] = fileSearchPrice
		}
	}
	if !audioInputQuota.IsZero() {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = audioTokens
		other["audio_input_price"] = audioInputPrice
	}
	if !dImageGenerationCallQuota.IsZero() {
		other["image_generation_call"] = true
		other["image_generation_call_price"] = imageGenerationCallPrice
	}

	// 记录请求体读取耗时（毫秒）
	if bodyReadMs := common.GetContextKeyInt(ctx, constant.ContextKeyRequestBodyReadTime); bodyReadMs > 0 {
		other["body_read_time_ms"] = bodyReadMs
	}

	ttsCount := common.GetContextKeyInt(ctx, constant.ContextKeyTTSCount)
	if ttsCount > 0 {
		ttsRatio := ratio_setting.GetTTSRatio(relayInfo.UpstreamModelName)
		quota = int(float64(ttsCount) / 1000 * ttsRatio)
		logContent = fmt.Sprintf("ttsRatio:%.2f，TTS 输入字符数:%d，TTS语音计费: %s", ttsRatio, ttsCount, logger.FormatQuota(quota))
	}
	clientUserId := common.GetContextKeyString(ctx, constant.ContextKeyClientUserId)
	clientScenairo := common.GetContextKeyString(ctx, constant.ContextKeyClientScenairo)
	requestId := ctx.GetString(common.RequestIdKey)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Request:          requestStr,
		Response:         responseStr,
		ClientUserId:     clientUserId,
		ClientScenairo:   clientScenairo,
		RequestId:        requestId,
	})
}
