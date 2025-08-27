package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"one-api/constant"
	"one-api/dto"
	"one-api/relay/channel"
	"one-api/relay/channel/ai360"
	"one-api/relay/channel/lingyiwanwu"
	"one-api/relay/channel/minimax"
	"one-api/relay/channel/moonshot"
	"one-api/relay/channel/openrouter"
	"one-api/relay/channel/xinference"
	relaycommon "one-api/relay/common"
	"one-api/relay/common_handler"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"one-api/types"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	ChannelType    int
	ResponseFormat string
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	//if !strings.Contains(request.Model, "claude") {
	//	return nil, fmt.Errorf("you are using openai channel type with path /v1/messages, only claude model supported convert, but got %s", request.Model)
	//}
	aiRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	if info.SupportStreamOptions {
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
	if info.RelayFormat == relaycommon.RelayFormatClaude {
		return fmt.Sprintf("%s/v1/chat/completions", info.BaseUrl), nil
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		if strings.HasPrefix(info.BaseUrl, "https://") {
			baseUrl := strings.TrimPrefix(info.BaseUrl, "https://")
			baseUrl = "wss://" + baseUrl
			info.BaseUrl = baseUrl
		} else if strings.HasPrefix(info.BaseUrl, "http://") {
			baseUrl := strings.TrimPrefix(info.BaseUrl, "http://")
			baseUrl = "ws://" + baseUrl
			info.BaseUrl = baseUrl
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

		// 特殊处理 responses API
		if info.RelayMode == relayconstant.RelayModeResponses {
			requestURL = fmt.Sprintf("/openai/v1/responses?api-version=preview")
			return relaycommon.GetFullRequestURL(info.BaseUrl, requestURL, info.ChannelType), nil
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
		return relaycommon.GetFullRequestURL(info.BaseUrl, requestURL, info.ChannelType), nil
	case constant.ChannelTypeMiniMax:
		return minimax.GetRequestURL(info)
	case constant.ChannelTypeCustom:
		url := info.BaseUrl
		url = strings.Replace(url, "{model}", info.UpstreamModelName, -1)
		return url, nil
	default:
		return relaycommon.GetFullRequestURL(info.BaseUrl, info.RequestURLPath, info.ChannelType), nil
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
			header.Set("Authorization", "Bearer "+info.ApiKey)
		}
	} else {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		header.Set("HTTP-Referer", "https://www.newapi.ai")
		header.Set("X-Title", "New API")
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
		newRequest.Temperature = float64(*request.Temperature)
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
		newRequest.Tools = tools
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
	}
	if strings.HasPrefix(request.Model, "o") {
		if request.MaxCompletionTokens == 0 && request.MaxTokens != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = 0
		}
		request.Temperature = nil
		if strings.HasSuffix(request.Model, "-high") {
			request.ReasoningEffort = "high"
			request.Model = strings.TrimSuffix(request.Model, "-high")
		} else if strings.HasSuffix(request.Model, "-low") {
			request.ReasoningEffort = "low"
			request.Model = strings.TrimSuffix(request.Model, "-low")
		} else if strings.HasSuffix(request.Model, "-medium") {
			request.ReasoningEffort = "medium"
			request.Model = strings.TrimSuffix(request.Model, "-medium")
		}
		info.ReasoningEffort = request.ReasoningEffort
		info.UpstreamModelName = request.Model

		// o系列模型developer适配（o1-mini除外）
		if !strings.HasPrefix(request.Model, "o1-mini") && !strings.HasPrefix(request.Model, "o1-preview") {
			//修改第一个Message的内容，将system改为developer
			if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
				request.Messages[0].Role = "developer"
			}
		}
	}

	if strings.HasPrefix(request.Model, "gemini") {
		//把给chat的gemini请求  audio_url, video_url 转换为 image_url
		if strings.Contains(info.BaseUrl, "chataiapi") ||
			strings.Contains(info.BaseUrl, "guoguo") {
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
		request.MaxTokens = 0
	}
	dealFunctionCall(request)
	return request, nil
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
		jsonData, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("error marshalling object: %w", err)
		}
		return bytes.NewReader(jsonData), nil
	} else {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)

		// 获取所有表单字段
		formData := c.Request.PostForm

		// 遍历表单字段并打印输出
		for key, values := range formData {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
			}
		}

		// 添加文件字段
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			return nil, errors.New("file is required")
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", header.Filename)
		if err != nil {
			return nil, errors.New("create form file failed")
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, errors.New("copy file failed")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &requestBody, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesEdits:

		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)
		// 获取所有表单字段
		formData := c.Request.PostForm
		// 遍历表单字段并打印输出
		for key, values := range formData {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
			}
		}

		// Parse the multipart form to handle both single image and multiple images
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory
			return nil, errors.New("failed to parse multipart form")
		}

		if c.Request.MultipartForm != nil && c.Request.MultipartForm.File != nil {
			// Check if "image" field exists in any form, including array notation
			var imageFiles []*multipart.FileHeader
			var exists bool

			// First check for standard "image" field
			if imageFiles, exists = c.Request.MultipartForm.File["image"]; !exists || len(imageFiles) == 0 {
				// If not found, check for "image[]" field
				if imageFiles, exists = c.Request.MultipartForm.File["image[]"]; !exists || len(imageFiles) == 0 {
					// If still not found, iterate through all fields to find any that start with "image["
					foundArrayImages := false
					for fieldName, files := range c.Request.MultipartForm.File {
						if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
							foundArrayImages = true
							for _, file := range files {
								imageFiles = append(imageFiles, file)
							}
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
				defer file.Close()

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
			}

			// Handle mask file if present
			if maskFiles, exists := c.Request.MultipartForm.File["mask"]; exists && len(maskFiles) > 0 {
				maskFile, err := maskFiles[0].Open()
				if err != nil {
					return nil, errors.New("failed to open mask file")
				}
				defer maskFile.Close()

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
			}
		} else {
			return nil, errors.New("no multipart form data found")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return bytes.NewReader(requestBody.Bytes()), nil

	default:
		return request, nil
	}
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
	// 模型后缀转换 reasoning effort
	if strings.HasSuffix(request.Model, "-high") {
		request.Reasoning.Effort = "high"
		request.Model = strings.TrimSuffix(request.Model, "-high")
	} else if strings.HasSuffix(request.Model, "-low") {
		request.Reasoning.Effort = "low"
		request.Model = strings.TrimSuffix(request.Model, "-low")
	} else if strings.HasSuffix(request.Model, "-medium") {
		request.Reasoning.Effort = "medium"
		request.Model = strings.TrimSuffix(request.Model, "-medium")
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == relayconstant.RelayModeAudioTranscription ||
		info.RelayMode == relayconstant.RelayModeAudioTranslation ||
		info.RelayMode == relayconstant.RelayModeImagesEdits {
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
		usage, err = OpenaiHandlerWithUsage(c, info, resp)
	case relayconstant.RelayModeRerank:
		usage, err = common_handler.RerankHandler(c, info, resp)
	case relayconstant.RelayModeResponses:
		if info.IsStream {
			usage, err = OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = OaiResponsesHandler(c, info, resp)
		}
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
	case constant.ChannelTypeMoonshot:
		return moonshot.ModelList
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ModelList
	case constant.ChannelTypeMiniMax:
		return minimax.ModelList
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
	case constant.ChannelTypeMoonshot:
		return moonshot.ChannelName
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ChannelName
	case constant.ChannelTypeMiniMax:
		return minimax.ChannelName
	case constant.ChannelTypeXinference:
		return xinference.ChannelName
	case constant.ChannelTypeOpenRouter:
		return openrouter.ChannelName
	default:
		return ChannelName
	}
}
