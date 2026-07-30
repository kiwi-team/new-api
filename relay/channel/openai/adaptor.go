package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/ai360"
	"github.com/QuantumNous/new-api/relay/channel/lingyiwanwu"

	//"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	"github.com/QuantumNous/new-api/relay/channel/xinference"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/common_handler"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	ChannelType    int
	ResponseFormat string
}

// parseReasoningEffortFromModelSuffix 从模型名称中解析推理级别
// support OAI models: o1-mini/o3-mini/o4-mini/o1/o3 etc...
// minimal effort only available in gpt-5
func parseReasoningEffortFromModelSuffix(model string) (string, string) {
	effortSuffixes := []string{"-high", "-minimal", "-low", "-medium", "-none", "-xhigh"}
	for _, suffix := range effortSuffixes {
		if strings.HasSuffix(model, suffix) {
			effort := strings.TrimPrefix(suffix, "-")
			originModel := strings.TrimSuffix(model, suffix)
			return effort, originModel
		}
	}
	return "", model
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, request)
	if err != nil {
		return nil, err
	}
	openaiRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	//if !strings.Contains(request.Model, "claude") {
	//	return nil, fmt.Errorf("you are using openai channel type with path /v1/messages, only claude model supported convert, but got %s", request.Model)
	//}
	//if common.DebugEnabled {
	//	bodyBytes := []byte(common.GetJsonString(request))
	//	err := os.WriteFile(fmt.Sprintf("claude_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, request)
	if err != nil {
		return nil, err
	}
	aiRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	//if common.DebugEnabled {
	//	println(fmt.Sprintf("convert claude to openai request result: %s", common.GetJsonString(aiRequest)))
	//	// Save request body to file for debugging
	//	bodyBytes := []byte(common.GetJsonString(aiRequest))
	//	err = os.WriteFile(fmt.Sprintf("claude_to_openai_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	if info.SupportStreamOptions && info.IsStream {
		aiRequest.StreamOptions = &dto.StreamOptions{
			IncludeUsage: true,
		}
	}
	return a.ConvertOpenAIRequest(c, info, aiRequest)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType

	// initialize ThinkingContentInfo when thinking_to_content is enabled
	if info.ChannelSetting.ThinkingToContent {
		info.ThinkingContentInfo = relaycommon.ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
			HasSentThinkingContent:  false,
		}
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeRealtime {
		if strings.HasPrefix(info.ChannelBaseUrl, "https://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "https://")
			baseUrl = "wss://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		} else if strings.HasPrefix(info.ChannelBaseUrl, "http://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "http://")
			baseUrl = "ws://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		}
	}
	switch info.ChannelType {
	case constant.ChannelTypeAzure:
		apiVersion := info.ApiVersion
		if apiVersion == "" {
			apiVersion = constant.AzureDefaultAPIVersion
		}
		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(info.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, apiVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")

		if info.RelayFormat == types.RelayFormatClaude {
			task = strings.TrimPrefix(task, "messages")
			task = "chat/completions" + task
		}

		// 特殊处理 responses API（包含 compact）
		if info.RelayMode == relayconstant.RelayModeResponses || info.RelayMode == relayconstant.RelayModeResponsesCompact {
			responsesApiVersion := "preview"

			subUrl := "/openai/v1/responses"
			if strings.Contains(info.ChannelBaseUrl, "cognitiveservices.azure.com") {
				subUrl = "/openai/responses"
				responsesApiVersion = apiVersion
			}

			if info.ChannelOtherSettings.AzureResponsesVersion != "" {
				responsesApiVersion = info.ChannelOtherSettings.AzureResponsesVersion
			}

			// compact 模式追加 /compact
			if info.RelayMode == relayconstant.RelayModeResponsesCompact {
				subUrl = subUrl + "/compact"
			}

			requestURL = fmt.Sprintf("%s?api-version=%s", subUrl, responsesApiVersion)
			return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
		}

		model_ := info.UpstreamModelName
		// 2025年5月10日后创建的渠道不移除.
		if info.ChannelCreateTime < constant.AzureNoRemoveDotTime {
			model_ = strings.Replace(model_, ".", "", -1)
		}
		// https://github.com/songquanpeng/one-api/issues/67
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		if info.RelayMode == relayconstant.RelayModeRealtime {
			requestURL = fmt.Sprintf("/openai/realtime?deployment=%s&api-version=%s", model_, apiVersion)
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
	//case constant.ChannelTypeMiniMax:
	//	return minimax.GetRequestURL(info)
	case constant.ChannelTypeCustom:
		url := info.ChannelBaseUrl
		url = strings.Replace(url, "{model}", info.UpstreamModelName, -1)
		return url, nil
	default:
		// if info.ChannelType == constant.ChannelTypeOpenRouter &&
		// 	(info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		// 	// OpenRouter 统一生图接口，生成与编辑均走 /api/v1/images
		// 	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, "/v1/images", info.ChannelType), nil
		//}
		if (info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini) &&
			info.RelayMode != relayconstant.RelayModeResponses &&
			info.RelayMode != relayconstant.RelayModeResponsesCompact {
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	if info.ChannelType == constant.ChannelTypeAzure {
		header.Set("api-key", info.ApiKey)
		return nil
	}
	if info.ChannelType == constant.ChannelTypeOpenAI && "" != info.Organization {
		header.Set("OpenAI-Organization", info.Organization)
	}
	// 检查 Header Override 是否已设置 Authorization，如果已设置则跳过默认设置
	// 这样可以避免在 Header Override 应用时被覆盖（虽然 Header Override 会在之后应用，但这里作为额外保护）
	hasAuthOverride := false
	if len(info.HeadersOverride) > 0 {
		for k := range info.HeadersOverride {
			if strings.EqualFold(k, "Authorization") {
				hasAuthOverride = true
				break
			}
		}
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		swp := c.Request.Header.Get("Sec-WebSocket-Protocol")
		if swp != "" {
			items := []string{
				"realtime",
				"openai-insecure-api-key." + info.ApiKey,
				"openai-beta.realtime-v1",
			}
			header.Set("Sec-WebSocket-Protocol", strings.Join(items, ","))
			//req.Header.Set("Sec-WebSocket-Key", c.Request.Header.Get("Sec-WebSocket-Key"))
			//req.Header.Set("Sec-Websocket-Extensions", c.Request.Header.Get("Sec-Websocket-Extensions"))
			//req.Header.Set("Sec-Websocket-Version", c.Request.Header.Get("Sec-Websocket-Version"))
		} else {
			header.Set("openai-beta", "realtime=v1")
			if !hasAuthOverride {
				header.Set("Authorization", "Bearer "+info.ApiKey)
			}
		}
	} else {
		if !hasAuthOverride {
			header.Set("Authorization", "Bearer "+info.ApiKey)
		}
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if header.Get("HTTP-Referer") == "" {
			header.Set("HTTP-Referer", "https://www.newapi.ai")
		}
		if header.Get("X-OpenRouter-Title") == "" {
			header.Set("X-OpenRouter-Title", "New API")
		}
	}
	return nil
}

func ConvertChatRequestToResponseRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// 把completions的请求，转化为responses的格式
	var newRequest dto.OpenAIResponsesRequest

	// 基本字段映射
	newRequest.Model = request.Model
	newRequest.Stream = request.Stream
	newRequest.MaxOutputTokens = request.MaxTokens
	if request.Temperature != nil {
		newRequest.Temperature = request.Temperature
	}
	newRequest.TopP = request.TopP
	newRequest.User = request.User
	if request.Reasoning != nil {
		var reasoning dto.Reasoning
		err := json.Unmarshal(request.Reasoning, &reasoning)
		if err != nil {
			return nil, err
		}
		newRequest.Reasoning = &reasoning
	}

	// 将Messages转换为Input
	if len(request.Messages) > 0 {
		var input []dto.OpenAIResponsesRequestInputItem
		for _, m := range request.Messages {
			contentList := make([]dto.OpenAIResponsesRequestInputItemContent, 0)
			for _, cnt := range m.ParseContent() {
				switch cnt.Type {
				case dto.ContentTypeText:
					contentList = append(contentList, dto.OpenAIResponsesRequestInputItemContent{
						Type: "input_text",
						Text: cnt.Text,
					})
				case dto.ContentTypeImageURL:
					contentList = append(contentList, dto.OpenAIResponsesRequestInputItemContent{
						Type:     "input_image",
						ImageUrl: cnt.ImageUrl.(dto.MessageImageUrl).Url,
					})
				}
			}
			input = append(input, dto.OpenAIResponsesRequestInputItem{
				Role:    m.Role,
				Content: contentList,
			})
		}
		messagesJson, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		newRequest.Input = json.RawMessage(messagesJson)
	}

	// 转换Tools
	if len(request.Tools) > 0 {
		var responseTools []dto.ResponsesToolsCall
		for _, tool := range request.Tools {
			responseTool := dto.ResponsesToolsCall{
				Type:        tool.Type,
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
			}
			if tool.Function.Parameters != nil {
				paramsJson, err := json.Marshal(tool.Function.Parameters)
				if err != nil {
					return nil, err
				}
				responseTool.Parameters = json.RawMessage(paramsJson)
			}
			responseTools = append(responseTools, responseTool)
		}
		// Convert ResponsesToolsCall slice to []map[string]any
		tools := make([]map[string]any, len(responseTools))
		for i, tool := range responseTools {
			toolMap := map[string]any{
				"type":        tool.Type,
				"name":        tool.Name,
				"description": tool.Description,
			}
			if tool.Parameters != nil {
				toolMap["parameters"] = tool.Parameters
			}
			tools[i] = toolMap
		}
		toolsStr, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}
		newRequest.Tools = json.RawMessage(toolsStr)
	}

	// 转换ToolChoice
	if request.ToolChoice != nil {
		toolChoiceJson, err := json.Marshal(request.ToolChoice)
		if err != nil {
			return nil, err
		}
		newRequest.ToolChoice = json.RawMessage(toolChoiceJson)
	}

	return newRequest, nil
}

// # For Gemini, only `enum` field when the type is `string`.
/*
当 某一个 property type 不是string，同时还有enum字段，需要把enum的内容，追加到description中 同时去掉enum属性
例如：
修改前：
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the person"
    },
    "age": {
      "type": "number",
      "description": "The age of the person"
    },
    "gender": {
      "type": "array",
	  "items": {
		"type": "string"
	  },
      "description": "The gender of the person ",
	  "enum": [
		"male",
		"female",
		"other"
	  ]
	}
  }
}

修改后：
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the person"
    },
    "age": {
      "type": "number",
      "description": "The age of the person"
    },
    "gender": {
      "type": "array",
	  "items": {
		"type": "string"
	  },
      "description": "The gender of the person  Enum values:[male, female, other]"
	   }
  }
}
*/
func dealFunctionCall(request *dto.GeneralOpenAIRequest) {
	if request.Tools == nil {
		return
	}
	for i, tool := range request.Tools {
		if tool.Function.Parameters == nil {
			continue
		}
		params, ok := tool.Function.Parameters.(map[string]any)
		if !ok {
			continue
		}
		if properties, ok := params["properties"]; ok {
			if properties, ok := properties.(map[string]any); ok {
				for j, prop := range properties {
					if prop, ok := prop.(map[string]any); ok {
						if typ, ok := prop["type"]; ok {
							if enum, ok := prop["enum"]; ok {
								if typ != "string" {
									if description, ok := prop["description"].(string); ok {
										if enum, ok := enum.([]any); ok {
											var enumValues []string
											for _, v := range enum {
												enumValues = append(enumValues, fmt.Sprintf("%v", v))
											}
											prop["description"] = description + " \n Enum values:[" + strings.Join(enumValues, ", ") + "]"
											delete(prop, "enum")
											request.Tools[i].Function.Parameters.(map[string]any)["properties"].(map[string]any)[j] = prop
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	if c.GetString(constant.ContextKeyCompletionsResponses) == "yes" {
		return ConvertChatRequestToResponseRequest(c, info, request)
	}
	//if info.ChannelType != common.ChannelTypeOpenAI && info.ChannelType != common.ChannelTypeAzure {
	if info.ChannelType != constant.ChannelTypeOpenAI && info.ChannelType != constant.ChannelTypeAzure {
		request.StreamOptions = nil
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if len(request.Usage) == 0 {
			request.Usage = json.RawMessage(`{"include":true}`)
		}
		// 适配 OpenRouter 的 thinking 后缀
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) &&
			strings.HasSuffix(info.UpstreamModelName, "-thinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
			request.Model = info.UpstreamModelName
			if len(request.Reasoning) == 0 {
				reasoning := map[string]any{
					"enabled": true,
				}
				if request.ReasoningEffort != "" && request.ReasoningEffort != "none" {
					reasoning["effort"] = request.ReasoningEffort
				}
				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}
				request.Reasoning = marshal
			}
			// 清空多余的ReasoningEffort
			request.ReasoningEffort = ""
		} else {
			if len(request.Reasoning) == 0 {
				// 适配 OpenAI 的 ReasoningEffort 格式
				if request.ReasoningEffort != "" {
					reasoning := map[string]any{
						"enabled": true,
					}
					if request.ReasoningEffort != "none" {
						reasoning["effort"] = request.ReasoningEffort
						marshal, err := common.Marshal(reasoning)
						if err != nil {
							return nil, fmt.Errorf("error marshalling reasoning: %w", err)
						}
						request.Reasoning = marshal
					}
				}
			}
			request.ReasoningEffort = ""
		}

		// https://docs.anthropic.com/en/api/openai-sdk#extended-thinking-support
		// 没有做排除3.5Haiku等，要出问题再加吧，最佳兼容性（不是
		if request.THINKING != nil && strings.HasPrefix(info.UpstreamModelName, "anthropic") {
			var thinking dto.Thinking // Claude标准Thinking格式
			if err := json.Unmarshal(request.THINKING, &thinking); err != nil {
				return nil, fmt.Errorf("error Unmarshal thinking: %w", err)
			}

			// 只有当 thinking.Type 是 "enabled" 时才处理
			if thinking.Type == "enabled" {
				// 检查 BudgetTokens 是否为 nil
				if thinking.BudgetTokens == nil {
					return nil, fmt.Errorf("BudgetTokens is nil when thinking is enabled")
				}

				reasoning := openrouter.RequestReasoning{
					Enabled:   common.GetPointer(true),
					MaxTokens: *thinking.BudgetTokens,
				}

				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}

				request.Reasoning = marshal
			}

			// 清空 THINKING
			request.THINKING = nil
		}

	}
	isOModel := dto.IsOpenAIReasoningOModel(info.UpstreamModelName)
	isGPT5Model := dto.IsOpenAIGPT5Model(info.UpstreamModelName)
	if isOModel || isGPT5Model {
		if lo.FromPtrOr(request.MaxCompletionTokens, uint(0)) == 0 && lo.FromPtrOr(request.MaxTokens, uint(0)) != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = nil
		}

		if isOModel {
			request.Temperature = nil
		}

		// gpt-5系列模型适配 归零不再支持的参数
		if isGPT5Model {
			request.Temperature = nil
			request.TopP = nil
			request.LogProbs = nil
		}

		// 转换模型推理力度后缀
		effort, originModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(info.UpstreamModelName)
		if effort != "" {
			request.ReasoningEffort = effort
			info.UpstreamModelName = originModel
			request.Model = originModel
		}

		info.ReasoningEffort = request.ReasoningEffort

		// o系列模型developer适配（o1-mini除外）
		if !strings.HasPrefix(info.UpstreamModelName, "o1-mini") && !strings.HasPrefix(info.UpstreamModelName, "o1-preview") {
			//修改第一个Message的内容，将system改为developer
			if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
				request.Messages[0].Role = "developer"
			}
		}
	}

	if strings.HasPrefix(request.Model, "gemini") {
		//把给chat的gemini请求  audio_url, video_url 转换为 image_url
		isGuoguo := strings.Contains(info.ChannelMeta.ChannelBaseUrl, "guoguo")
		isChataiapi := strings.Contains(info.ChannelMeta.ChannelBaseUrl, "chataiapi")
		if isChataiapi || isGuoguo {
			newMessages := make([]dto.Message, 0, len(request.Messages))
			for _, message := range request.Messages {
				newContentArr := make([]dto.MediaContent, 0)
				arr := message.ParseContent()
				for _, content := range arr {
					var newContent dto.MediaContent
					switch content.Type {
					case dto.ContentTypeAudioUrl:
						audioUrl := content.AudioUrl.(*dto.MessageAudioUrl)
						newContent = dto.MediaContent{
							Type: dto.ContentTypeImageURL,
							ImageUrl: dto.MessageImageUrl{
								Url: audioUrl.Url,
							},
						}
					case dto.ContentTypeVideoUrl:
						videoUrl := content.VideoUrl.(*dto.MessageVideoUrl)
						newContent = dto.MediaContent{
							Type: dto.ContentTypeImageURL,
							ImageUrl: dto.MessageImageUrl{
								Url: videoUrl.Url,
							},
						}
					default:
						newContent = content
					}
					newContentArr = append(newContentArr, newContent)
				}
				newMessage := message
				newMessage.Content = newContentArr
				newMessages = append(newMessages, newMessage)
			}
			request.Messages = newMessages
		}
		// gemini 模型去掉max_tokens参数
		//request.MaxTokens = 0
		if isGuoguo {
			var thinking dto.AnthropicThinking
			if request.THINKING != nil {
				err := json.Unmarshal(request.THINKING, &thinking)
				if err != nil {
					return nil, fmt.Errorf("error unmarshalling thinking: %w", err)
				}
				request.THINKING = json.RawMessage(fmt.Sprintf(`{"type":"%s","thinking_budget":%d}`, thinking.Type, thinking.BudgetTokens))
			}
		}
	}
	dealFunctionCall(request)

	// https://platform.claude.com/docs/en/build-with-claude/extended-thinking#feature-compatibility
	if strings.HasPrefix(info.UpstreamModelName, "claude") {
		if IsThinkingEnabled(request) {
			// Claude 扩展思考模式不允许同时下发 top_k / temperature
			request.TopK = nil
			request.Temperature = nil
		}
	}
	//common.PrintJson("\nopenaiRequest", request)
	return request, nil
}

func IsThinkingEnabled(request *dto.GeneralOpenAIRequest) bool {
	if len(request.EnableThinking) > 0 {
		var enabled bool
		if err := common.Unmarshal(request.EnableThinking, &enabled); err == nil && enabled {
			return true
		}
	}
	if strings.HasSuffix(request.Model, "-thinking") {
		if strings.Contains(request.Model, "claude") ||
			strings.Contains(request.Model, "gemini") {
			return true
		}
	}
	if request.THINKING == nil {
		return false
	}
	var thinking dto.AnthropicThinking
	err := json.Unmarshal(request.THINKING, &thinking)
	if err != nil {
		return false
	}
	return thinking.Type == "enabled"

}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	a.ResponseFormat = request.ResponseFormat
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		jsonData, err := common.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("error marshalling object: %w", err)
		}
		return bytes.NewReader(jsonData), nil
	} else {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)

		formData, err2 := common.ParseMultipartFormReusable(c)
		if err2 != nil {
			return nil, fmt.Errorf("error parsing multipart form: %w", err2)
		}

		// 打印类似 curl 命令格式的信息
		logger.LogDebug(c.Request.Context(), "--form 'model=\"%s\"'", request.Model)

		// 遍历表单字段并打印输出
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
				logger.LogDebug(c.Request.Context(), "--form '%s=\"%s\"'", key, value)
			}
		}

		// 从 formData 中获取文件
		fileHeaders := formData.File["file"]
		if len(fileHeaders) == 0 {
			return nil, errors.New("file is required")
		}

		// 使用 formData 中的第一个文件
		fileHeader := fileHeaders[0]
		logger.LogDebug(c.Request.Context(), "--form 'file=@\"%s\"' (size: %d bytes, content-type: %s)",
			fileHeader.Filename, fileHeader.Size, fileHeader.Header.Get("Content-Type"))

		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("error opening audio file: %v", err)
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", fileHeader.Filename)
		if err != nil {
			return nil, errors.New("create form file failed")
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, errors.New("copy file failed")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		logger.LogDebug(c.Request.Context(), "--header 'Content-Type: %s'", writer.FormDataContentType())
		return &requestBody, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesEdits:
		if isJSONRequest(c) {
			return request, nil
		}

		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)
		// 使用已解析的 multipart 表单，避免重复解析
		mf := c.Request.MultipartForm
		if mf == nil {
			form, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, fmt.Errorf("failed to parse multipart form: %w", err)
			}
			c.Request.MultipartForm = form
			c.Request.PostForm = url.Values(form.Value)
			mf = form
		}

		// 写入所有非文件字段
		if mf != nil {
			for key, values := range mf.Value {
				if key == "model" {
					continue
				}
				for _, value := range values {
					writer.WriteField(key, value)
				}
			}
		}

		if mf != nil && mf.File != nil {
			// Check if "image" field exists in any form, including array notation
			var imageFiles []*multipart.FileHeader
			var exists bool

			// First check for standard "image" field
			if imageFiles, exists = mf.File["image"]; !exists || len(imageFiles) == 0 {
				// If not found, check for "image[]" field
				if imageFiles, exists = mf.File["image[]"]; !exists || len(imageFiles) == 0 {
					// If still not found, iterate through all fields to find any that start with "image["
					foundArrayImages := false
					for fieldName, files := range mf.File {
						if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
							foundArrayImages = true
							imageFiles = append(imageFiles, files...)
						}
					}

					// If no image fields found at all
					if !foundArrayImages && (len(imageFiles) == 0) {
						return nil, errors.New("image is required")
					}
				}
			}

			// Process all image files
			for i, fileHeader := range imageFiles {
				file, err := fileHeader.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open image file %d: %w", i, err)
				}

				// If multiple images, use image[] as the field name
				fieldName := "image"
				if len(imageFiles) > 1 {
					fieldName = "image[]"
				}

				// Determine MIME type based on file extension
				mimeType := detectImageMimeType(fileHeader.Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
				h.Set("Content-Type", mimeType)

				part, err := writer.CreatePart(h)
				if err != nil {
					return nil, fmt.Errorf("create form part failed for image %d: %w", i, err)
				}

				if _, err := io.Copy(part, file); err != nil {
					return nil, fmt.Errorf("copy file failed for image %d: %w", i, err)
				}

				// 复制完立即关闭，避免在循环内使用 defer 占用资源
				_ = file.Close()
			}

			// Handle mask file if present
			if maskFiles, exists := mf.File["mask"]; exists && len(maskFiles) > 0 {
				maskFile, err := maskFiles[0].Open()
				if err != nil {
					return nil, errors.New("failed to open mask file")
				}
				// 复制完立即关闭，避免在循环内使用 defer 占用资源

				// Determine MIME type for mask file
				mimeType := detectImageMimeType(maskFiles[0].Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="mask"; filename="%s"`, maskFiles[0].Filename))
				h.Set("Content-Type", mimeType)

				maskPart, err := writer.CreatePart(h)
				if err != nil {
					return nil, errors.New("create form file failed for mask")
				}

				if _, err := io.Copy(maskPart, maskFile); err != nil {
					return nil, errors.New("copy mask file failed")
				}
				_ = maskFile.Close()
			}
		} else {
			return nil, errors.New("no multipart form data found")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &requestBody, nil

	default:
		return request, nil
	}
}

// convertOpenRouterImageRequest 将 OpenAI 风格的生图/改图请求转换为 OpenRouter 统一生图接口
// (POST /api/v1/images) 的 JSON 请求体。
//   - 生成 (RelayModeImagesGenerations)：不携带 input_references。
//   - 编辑 (RelayModeImagesEdits)：从 multipart 上传的图片文件（转为 base64 data URL）
//     以及 JSON 请求体中的 image/images 字段收集 input_references。
func convertOpenRouterImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	openRouterReq := openrouter.ImageGenerationRequest{
		Model:  request.Model,
		Prompt: request.Prompt,
		N:      lo.FromPtr(request.N),
		Size:   request.Size,
	}

	if info.RelayMode != relayconstant.RelayModeImagesEdits {
		return openRouterReq, nil
	}

	// 编辑请求：收集参考图
	references := make([]openrouter.ImageInputReference, 0)

	// 1. multipart 上传的图片文件，转为 base64 data URL
	if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		mf := c.Request.MultipartForm
		if mf == nil {
			if _, err := c.MultipartForm(); err != nil {
				return nil, errors.New("failed to parse multipart form")
			}
			mf = c.Request.MultipartForm
		}
		if mf != nil && mf.File != nil {
			imageFiles := collectImageFileHeaders(mf)
			for i, fileHeader := range imageFiles {
				dataURL, err := imageFileHeaderToDataURL(fileHeader)
				if err != nil {
					return nil, fmt.Errorf("failed to read image file %d: %w", i, err)
				}
				references = append(references, openrouter.ImageInputReference{
					Type:     "image_url",
					ImageUrl: openrouter.ImageReferenceUrl{Url: dataURL},
				})
			}
		}
	}

	// 2. JSON 请求体中的 image/images 字段（HTTP(S) 链接或 base64 data URL）
	if urls, err := request.GetImageURLs(); err == nil {
		for _, url := range urls {
			references = append(references, openrouter.ImageInputReference{
				Type:     "image_url",
				ImageUrl: openrouter.ImageReferenceUrl{Url: url},
			})
		}
	}

	if len(references) == 0 {
		return nil, errors.New("image is required for image edits")
	}
	openRouterReq.InputReferences = references

	// 请求体已转为 JSON，覆盖 multipart 的 Content-Type
	c.Request.Header.Set("Content-Type", "application/json")
	return openRouterReq, nil
}

// collectImageFileHeaders 从已解析的 multipart 表单中收集 image / image[] / image[N] 字段的文件。
func collectImageFileHeaders(mf *multipart.Form) []*multipart.FileHeader {
	if files, ok := mf.File["image"]; ok && len(files) > 0 {
		return files
	}
	if files, ok := mf.File["image[]"]; ok && len(files) > 0 {
		return files
	}
	var imageFiles []*multipart.FileHeader
	for fieldName, files := range mf.File {
		if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
			imageFiles = append(imageFiles, files...)
		}
	}
	return imageFiles
}

// imageFileHeaderToDataURL 读取上传的图片文件并编码为 base64 data URL。
func imageFileHeaderToDataURL(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	mimeType := detectImageMimeType(fileHeader.Filename)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}
func isJSONRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json")
}

// detectImageMimeType determines the MIME type based on the file extension
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// Try to detect from extension if possible
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		// Default to png as a fallback
		return "image/png"
	}
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	//  转换模型推理力度后缀
	effort, originModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(request.Model)
	if effort != "" {
		if request.Reasoning == nil {
			request.Reasoning = &dto.Reasoning{
				Effort: effort,
			}
		} else {
			request.Reasoning.Effort = effort
		}
		request.Model = originModel
	}
	if info != nil && request.Reasoning != nil && request.Reasoning.Effort != "" {
		info.ReasoningEffort = request.Reasoning.Effort
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == relayconstant.RelayModeAudioTranscription ||
		info.RelayMode == relayconstant.RelayModeAudioTranslation ||
		(info.RelayMode == relayconstant.RelayModeImagesEdits && !isJSONRequest(c)) {
		return channel.DoFormRequest(a, c, info, requestBody)
	} else if info.RelayMode == relayconstant.RelayModeRealtime {
		return channel.DoWssRequest(a, c, info, requestBody)
	} else {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {

	switch info.RelayMode {
	case relayconstant.RelayModeRealtime:
		err, usage = OpenaiRealtimeHandler(c, info)
	case relayconstant.RelayModeAudioSpeech:
		usage = OpenaiTTSHandler(c, resp, info)
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err, usage = OpenaiSTTHandler(c, resp, info, a.ResponseFormat)
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		if info.IsStream {
			usage, err = OpenaiImageStreamHandler(c, info, resp)
		} else {
			usage, err = OpenaiImageHandler(c, info, resp)
		}
	case relayconstant.RelayModeRerank:
		usage, err = common_handler.RerankHandler(c, info, resp)
	case relayconstant.RelayModeResponses:
		if info.IsStream {
			usage, err = OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = OaiResponsesHandler(c, info, resp)
		}
	case relayconstant.RelayModeResponsesCompact:
		usage, err = OaiResponsesCompactionHandler(c, resp)
	default:
		if info.IsStream {
			usage, err = OaiStreamHandler(c, info, resp)
		} else {
			usage, err = OpenaiHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ModelList
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ModelList
	//case constant.ChannelTypeMiniMax:
	//	return minimax.ModelList
	case constant.ChannelTypeXinference:
		return xinference.ModelList
	case constant.ChannelTypeOpenRouter:
		return openrouter.ModelList
	default:
		return ModelList
	}
}

func (a *Adaptor) GetChannelName() string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ChannelName
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ChannelName
	//case constant.ChannelTypeMiniMax:
	//	return minimax.ChannelName
	case constant.ChannelTypeXinference:
		return xinference.ChannelName
	case constant.ChannelTypeOpenRouter:
		return openrouter.ChannelName
	default:
		return ChannelName
	}
}
