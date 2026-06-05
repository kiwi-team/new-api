package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	gemini_realtime "github.com/QuantumNous/new-api/relay/channel/gemini_realtime"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"

	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}
	if info.UpstreamModelName == "Ring-1T" || info.UpstreamModelName == "Ling-1T" {
		if strings.Contains(data, "令牌token未开通百灵大模型服务") ||
			strings.Contains(data, `cn.com.antcloud.common.exception`) ||
			strings.Contains(data, "RATE_LIMIT") ||
			strings.Contains(data, `{"code":"500"`) {
			return errors.New(data)
		}
	}
	data = setDeltaRole(c, info, data)
	data = setResponseModel(c, info, data)
	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}

	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}
	return helper.ObjectData(c, lastStreamResponse)
}

// remapResponseModelName 计算返回给用户的模型名称。
// 优先使用渠道配置的「输出模型重命名」(ModelOutputMapping，按完整模型名精确匹配)，
// 未命中时回退到内置的 glm 重命名逻辑以保持向后兼容。
func remapResponseModelName(info *relaycommon.RelayInfo, model string) string {
	if mapping := info.ChannelSetting.ModelOutputMapping; mapping != "" && mapping != "{}" {
		modelMap := make(map[string]string)
		if err := common.UnmarshalJsonStr(mapping, &modelMap); err != nil {
			common.SysError("error unmarshalling model_output_mapping: " + err.Error())
		} else if mapped, ok := modelMap[model]; ok && mapped != "" {
			return mapped
		}
	}
	// 内置兼容逻辑
	if strings.Contains(model, "glm-4.7") {
		return "glm-4.7"
	}
	if strings.Contains(model, "glm-5") {
		return "glm-5"
	}
	return model
}

func setResponseModel(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string) string {
	hasOutputMapping := info.ChannelSetting.ModelOutputMapping != "" && info.ChannelSetting.ModelOutputMapping != "{}"
	isBuiltinRemap := strings.Contains(info.UpstreamModelName, "glm-4.7") || strings.Contains(info.UpstreamModelName, "glm-5")
	// 无配置且非内置场景时跳过解析，避免逐个流式分片的额外开销
	if !hasOutputMapping && !isBuiltinRemap {
		return lastStreamData
	}
	var lastStreamResponse dto.ChatCompletionsStreamResponse
	err := common.UnmarshalJsonStr(lastStreamData, &lastStreamResponse)
	if err != nil {
		common.SysError("error setting response model: " + err.Error())
		return lastStreamData
	}
	newModel := remapResponseModelName(info, lastStreamResponse.Model)
	if newModel == lastStreamResponse.Model {
		return lastStreamData
	}
	lastStreamResponse.Model = newModel
	byteArr, err1 := common.Marshal(lastStreamResponse)
	if err1 != nil {
		common.SysError("error setting response model: " + err1.Error())
		return lastStreamData
	}
	return string(byteArr)
}

func setDeltaRole(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string) string {
	var setRoleValue string
	if len(info.ChannelSetting.SetRole) > 0 {
		setRoleValue = info.ChannelSetting.SetRole
	}
	if setRoleValue == "" {
		return lastStreamData
	}
	var lastStreamResponse dto.ChatCompletionsStreamResponse
	err := common.UnmarshalJsonStr(lastStreamData, &lastStreamResponse)
	if err != nil {
		common.SysError("error setting delta role: " + err.Error())
		return lastStreamData
	}

	if len(lastStreamResponse.Choices) == 0 {
		return lastStreamData
	}
	for i := range lastStreamResponse.Choices {
		if len(lastStreamResponse.Choices[i].Delta.Role) == 0 {
			lastStreamResponse.Choices[i].Delta.Role = setRoleValue
		}
	}
	byteArr, err1 := common.Marshal(lastStreamResponse)
	if err1 != nil {
		common.SysError("error setting delta role: " + err1.Error())
		return lastStreamData
	}
	return string(byteArr)
}

// func OaiStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var lastStreamData string
	var secondLastStreamData string // 存储倒数第二个stream data，用于音频模型

	// 检查是否为音频模型
	isAudioModel := strings.Contains(strings.ToLower(model), "audio")

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if lastStreamData != "" {
			err := HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
			if err != nil {
				common.SysLog("error handling stream format: " + err.Error())
				if info.UpstreamModelName == "Ring-1T" || info.UpstreamModelName == "Ling-1T" {
					return false
				}
			}
		}
		if c.GetString(constant.ContextKeyCompletionsResponses) == "yes" {
			var responsesItem dto.ResponsesStreamResponse
			var chatItem dto.ChatCompletionsStreamResponse
			err1 := common.UnmarshalJsonStr(data, &responsesItem)
			if err1 != nil {
				common.SysError("error handling stream format1 : " + err1.Error())
			}

			// 通过responsesItem的值构建chatItem
			chatItem = dto.ChatCompletionsStreamResponse{
				Id:      "chatcmpl-" + common.GetRandomString(32),
				Object:  "chat.completion.chunk",
				Created: common.GetTimestamp(),
				Choices: []dto.ChatCompletionsStreamResponseChoice{},
			}

			// 根据responsesItem的类型设置chatItem的内容
			switch responsesItem.Type {
			//case "response.created", "response.output_item.added", "response.in_progress", "response.content_part.added":
			//return false
			case "response.output_text.delta", "response.reasoning_summary_text.delta":
				// 处理文本增量输出
				var choice dto.ChatCompletionsStreamResponseChoice
				choice.Delta.SetContentString(responsesItem.Delta)
				chatItem.Choices = append(chatItem.Choices, choice)

			case "response.completed":
				// 处理完成状态
				var choice dto.ChatCompletionsStreamResponseChoice
				finishReason := constant.FinishReasonStop
				choice.FinishReason = &finishReason
				chatItem.Choices = append(chatItem.Choices, choice)
				if responsesItem.Response != nil {
					chatItem.Id = responsesItem.Response.ID
					chatItem.Model = responsesItem.Response.Model
					chatItem.Usage = &dto.Usage{}
					if responsesItem.Response.Usage != nil {
						//chatItem.Usage = responsesItem.Response.Usage
						chatItem.Usage.PromptTokens = responsesItem.Response.Usage.InputTokens
						chatItem.Usage.CompletionTokens = responsesItem.Response.Usage.OutputTokens
						chatItem.Usage.TotalTokens = responsesItem.Response.Usage.TotalTokens
						chatItem.Usage.PromptTokensDetails = *responsesItem.Response.Usage.InputTokensDetails
						chatItem.Usage.CompletionTokenDetails = responsesItem.Response.Usage.OutputTokenDetails
					}
				}

			case "response.output_item.added":
				// 处理输出项添加
				if responsesItem.Item != nil && len(responsesItem.Item.Content) > 0 {
					var choice dto.ChatCompletionsStreamResponseChoice
					choice.Delta.SetContentString(responsesItem.Item.Content[0].Text)
					chatItem.Choices = append(chatItem.Choices, choice)
				}

			case "response.output_item.done":
				// 处理输出项完成
				if responsesItem.Item != nil {
					var choice dto.ChatCompletionsStreamResponseChoice
					if responsesItem.Item.Status == "completed" {
						finishReason := constant.FinishReasonStop
						choice.FinishReason = &finishReason
					}
					chatItem.Choices = append(chatItem.Choices, choice)
				}
			}

			// 如果成功构建了chatItem，则发送数据
			chatItemJson, jsonErr := json.Marshal(chatItem)
			if jsonErr != nil {
				common.SysError("error marshalling chat item: " + jsonErr.Error())
			}
			data = string(chatItemJson)
		}
		if len(data) > 0 {
			// 对音频模型，保存倒数第二个stream data
			if isAudioModel && lastStreamData != "" {
				secondLastStreamData = lastStreamData
			}

			lastStreamData = data
			// 流式增量记账：在数据到达时立即解析并把 delta content / reasoning /
			// tool args 累计进 responseTextBuilder。替代旧的"累积所有 streamItems
			// 到流结束再 processTokens 一次性 Unmarshal"，避免 O(单流体积 × 并发数)
			// 的内存放大（旧路径在高并发下 inuse 占比 42% 且每 10s alloc 16+ GB）。
			ingestStreamItem(info.RelayMode, data, &responseTextBuilder, &toolCount)
		}
		return true
	})

	if info.UpstreamModelName == "Ring-1T" || info.UpstreamModelName == "Ling-1T" {
		if strings.Contains(lastStreamData, "令牌token未开通百灵大模型服务") ||
			strings.Contains(lastStreamData, `cn.com.antcloud.common.exception`) ||
			strings.Contains(lastStreamData, "RATE_LIMIT") ||
			strings.Contains(lastStreamData, `{"code":"500"`) {
			return nil, types.NewError(errors.New(lastStreamData), types.ErrorCodeRateLimit)
		}
	}
	// 对音频模型，从倒数第二个stream data中提取usage信息
	if isAudioModel && secondLastStreamData != "" {
		var streamResp struct {
			Usage *dto.Usage `json:"usage"`
		}
		err := common.Unmarshal([]byte(secondLastStreamData), &streamResp)
		if err == nil && streamResp.Usage != nil && service.ValidUsage(streamResp.Usage) {
			usage = streamResp.Usage
			containStreamUsage = true

			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("Audio model usage extracted from second last SSE: PromptTokens=%d, CompletionTokens=%d, TotalTokens=%d, InputTokens=%d, OutputTokens=%d",
					usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
					usage.InputTokens, usage.OutputTokens))
			}
		}
	}

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		if shouldSendLastResp {
			_ = sendStreamData(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
		}
	}

	// token 累计已在 ingestStreamItem 内逐行完成；此处不再需要批量 processTokens。

	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	applyUsagePostProcessing(info, usage, common.StringToByteSlice(lastStreamData))

	HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	// 检查 finish_reason 是否表示上游错误（如 model_context_window_exceeded）
	// 这类响应虽然格式上是正常的 SSE，但实际上表示请求失败
	if errFinishReason := checkErrorFinishReason(lastStreamData); errFinishReason != "" {
		logger.LogError(c, fmt.Sprintf("upstream returned error finish_reason: %s", errFinishReason))
		return usage, types.NewError(
			fmt.Errorf("upstream error finish_reason: %s", errFinishReason),
			types.ErrorCodeBadResponse,
			types.ErrOptionWithSkipRetry(),
		)
	}

	return usage, nil
}

// checkErrorFinishReason 检查最后的流式数据中是否包含表示错误的 finish_reason
func checkErrorFinishReason(lastStreamData string) string {
	if lastStreamData == "" {
		return ""
	}
	var resp dto.ChatCompletionsStreamResponseSimple
	if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &resp); err != nil {
		return ""
	}
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != nil {
		fr := *resp.Choices[0].FinishReason
		if constant.ErrorFinishReasons[fr] {
			return fr
		}
	}
	return ""
}

// ParseTextAndImageURL 解析包含文本和图片URL的字符串
func ParseTextAndImageURL(input string) (text string, imageURL string, err error) {
	// 正则表达式：匹配Markdown图片格式
	// (.*?) - 非贪婪匹配文本内容
	// !\[.*?\]\((https?://[^)]+)\) - 匹配 ![alt](URL) 格式
	pattern := `^(.*?)!\[.*?\]\((https?://[^)]+)\)\s*$`

	// 使用DOTALL标志处理多行文本
	re := regexp.MustCompile(`(?s)` + pattern)

	matches := re.FindStringSubmatch(input)
	if len(matches) == 3 {
		text = strings.TrimSpace(matches[1])
		imageURL = matches[2]
		return text, imageURL, nil
	}

	// 匹配Markdown格式中的base64图片: ![alt](data:image/type;base64,data)
	base64MarkdownPattern := `^(.*?)!\[.*?\]\((data:image/[^;]+;base64,[A-Za-z0-9+/=]+)\)\s*$`
	base64MarkdownRe := regexp.MustCompile(`(?s)` + base64MarkdownPattern)
	if matches := base64MarkdownRe.FindStringSubmatch(input); len(matches) == 3 {
		text = strings.TrimSpace(matches[1])
		imageData := matches[2]
		return text, imageData, nil
	}

	// 再尝试匹配base64格式: data:image/type;base64,data
	base64Pattern := `^(.*?)\s*(data:image/[^;]+;base64,[A-Za-z0-9+/=]+)\s*$`
	base64Re := regexp.MustCompile(`(?s)` + base64Pattern)

	if matches := base64Re.FindStringSubmatch(input); len(matches) == 3 {
		text = strings.TrimSpace(matches[1])
		imageData := matches[2]
		return text, imageData, nil
	}

	index := strings.Index(input, "data:image/")
	if index != -1 {
		imageData := input[index:]
		imageData = strings.Trim(imageData, ")")
		text := strings.TrimSpace(input[:index])
		return text, imageData, nil
	}

	// 如果都不匹配，返回错误
	return "", "", fmt.Errorf("字符串格式不匹配，既不是Markdown格式也不是base64格式")

}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if common.DebugEnabled {
		println("upstream response body:", string(responseBody))
	}
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if newModel := remapResponseModelName(info, simpleResponse.Model); newModel != simpleResponse.Model {
		simpleResponse.Model = newModel
		responseBody, _ = common.Marshal(simpleResponse)
	}
	isGuoguo := strings.Contains(info.ChannelBaseUrl, "aiguoguo")
	isChat := strings.Contains(info.ChannelBaseUrl, "chataiapi")
	isNuwa := strings.Contains(info.ChannelBaseUrl, "nuwa")
	isYunwu := strings.Contains(info.ChannelBaseUrl, "yunwu")
	isToio := strings.Contains(info.ChannelBaseUrl, "aice") || strings.Contains(info.ChannelBaseUrl, "seedsnote") || strings.Contains(info.ChannelBaseUrl, "deeparena")
	isGeminiImageModel := strings.Contains(info.UpstreamModelName, "gemini") && strings.Contains(info.UpstreamModelName, "image")
	if (isGuoguo || isChat || isNuwa || isYunwu || isToio) && isGeminiImageModel {
		// "content": "没问题，这是添加了哆啦A梦的图片：\n![Image_1](https://img.aiguoguo199.com/file/BQACAgUAAyEGAASaOQ3XAALZo2jnt2LJXF_Do-uv5TWSIXZ6wAT0AAJIHQACAs44V_l87WpYb56INgQ.png)"
		// 处理图片地址，改成toiotech的地址
		for i, choice := range simpleResponse.Choices {
			if choice.Message.Content != nil {
				// 解析出图片地址
				contentStr := choice.Message.StringContent()
				if contentStr == "" {
					continue
				}
				text, imgUrl, err1 := ParseTextAndImageURL(contentStr)
				if err1 != nil {
					continue
				}
				imgUrl, err1 = service.SimpleUploadToS3(c.Request.Context(), imgUrl)
				if err1 != nil {
					continue
				}
				if len(text) > 0 {
					simpleResponse.Choices[i].Message.SetMediaContent([]dto.MediaContent{
						{
							Type: "text",
							Text: text,
						},
						{
							Type: "image_url",
							ImageUrl: &dto.MessageImageUrl{
								Url: imgUrl,
							},
						},
					})
				} else {
					simpleResponse.Choices[i].Message.SetMediaContent([]dto.MediaContent{
						{
							Type: "image_url",
							ImageUrl: &dto.MessageImageUrl{
								Url: imgUrl,
							},
						},
					})
				}
			}
		}

		responseBody, err = common.Marshal(simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	}
	//	if simpleResponse.Error != nil && simpleResponse.Error.Type != "" {
	//	return nil, types.WithOpenAIError(*simpleResponse.Error, resp.StatusCode)

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			break
		}
		if constant.ErrorFinishReasons[choice.FinishReason] {
			return nil, types.NewError(
				fmt.Errorf("upstream error finish_reason: %s", choice.FinishReason),
				types.ErrorCodeBadResponse,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	if simpleResponse.Usage.PromptTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.ReasoningContent+choice.Message.Reasoning, info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(&simpleResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(&simpleResponse, info)
		geminiRespStr, err := common.Marshal(geminiResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func streamTTSResponse(c *gin.Context, resp *http.Response) {
	c.Writer.WriteHeaderNow()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		logger.LogWarn(c, "streaming not supported")
		_, err := io.Copy(c.Writer, resp.Body)
		if err != nil {
			logger.LogWarn(c, err.Error())
		}
		return
	}

	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)
		//logger.LogInfo(c, fmt.Sprintf("streamTTSResponse read %d bytes", n))
		if n > 0 {
			if _, writeErr := c.Writer.Write(buffer[:n]); writeErr != nil {
				logger.LogError(c, writeErr.Error())
				break
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				logger.LogError(c, err.Error())
			}
			break
		}
	}
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}
	// For detecting Gemini-format upstream responses (e.g. when proxying through another new-api instance
	// that has a Gemini channel and passes through Gemini protocol transparently)
	geminiLastUsage := &dto.RealtimeUsage{}
	isGeminiProtocol := false

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken

				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				// Detect Gemini protocol: upstream may be another new-api instance with a Gemini
			// channel that passes through Gemini Live protocol transparently (no "type" field).
			if !isGeminiProtocol && realtimeEvent.Type == "" {
				geminiEvent := &gemini_realtime.GeminiLiveEvent{}
				if parseErr := common.Unmarshal(message, geminiEvent); parseErr == nil {
					if geminiEvent.SetupComplete != nil || geminiEvent.ServerContent != nil || geminiEvent.UsageMetadata != nil || geminiEvent.GoAway != nil {
						isGeminiProtocol = true
						logger.LogInfo(c, "detected Gemini protocol from upstream, switching to Gemini usage extraction")
					}
				}
			}

			if isGeminiProtocol {
				// Handle Gemini-format messages: extract usageMetadata for billing
				geminiEvent := &gemini_realtime.GeminiLiveEvent{}
				if parseErr := common.Unmarshal(message, geminiEvent); parseErr == nil && geminiEvent.UsageMetadata != nil {
					currentUsage := geminiEvent.UsageMetadata.ToRealtimeUsage()
					deltaUsage := gemini_realtime.ComputeDelta(geminiLastUsage, currentUsage)
					if deltaUsage.TotalTokens > 0 {
						consumeErr := preConsumeUsage(c, info, deltaUsage, sumUsage)
						if consumeErr != nil {
							errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
							return
						}
					}
					geminiLastUsage = currentUsage
					logger.LogInfo(c, fmt.Sprintf("gemini upstream usage: input=%d, output=%d, total=%d",
						currentUsage.InputTokens, currentUsage.OutputTokens, currentUsage.TotalTokens))
				}
			} else if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						err := preConsumeUsage(c, info, usage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						usage = &dto.RealtimeUsage{}

						localUsage = &dto.RealtimeUsage{}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						err = preConsumeUsage(c, info, localUsage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						localUsage = &dto.RealtimeUsage{}
						// print now usage
					}
					logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		logger.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
	}

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	// clear usage
	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}

func OpenaiHandlerWithUsage(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// Once we've written to the client, we should not return errors anymore
	// because the upstream has already consumed resources and returned content
	// We should still perform billing even if parsing fails
	// format
	if usageResp.InputTokens > 0 {
		usageResp.PromptTokens += usageResp.InputTokens
	}
	if usageResp.OutputTokens > 0 {
		usageResp.CompletionTokens += usageResp.OutputTokens
	}
	if usageResp.InputTokensDetails != nil {
		usageResp.PromptTokensDetails.ImageTokens += usageResp.InputTokensDetails.ImageTokens
		usageResp.PromptTokensDetails.TextTokens += usageResp.InputTokensDetails.TextTokens
	}
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)
	return &usageResp.Usage, nil
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}
