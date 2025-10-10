package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}
	data = setDeltaRole(c, info, data)
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
		common.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer common.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var streamItems []string // store stream items
	var forceFormat bool
	var thinkToContent bool

	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	if info.ChannelSetting.ThinkingToContent {
		thinkToContent = true
	}

	var (
		lastStreamData string
	)

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if lastStreamData != "" {
			err := handleStreamFormat(c, info, lastStreamData, forceFormat, thinkToContent)
			if err != nil {
				common.SysError("error handling stream format: " + err.Error())
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
		lastStreamData = data
		streamItems = append(streamItems, data)
		return true
	})

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		common.SysError("error handling last response: " + err.Error())
	}

	// if shouldSendLastResp {
	// 	err = sendStreamData(c, info, lastStreamData, forceFormat, thinkToContent)
	// 	if err != nil {
	// 		common.SysError("error sending stream data: " + err.Error())
	// 	}
	// 	//err = handleStreamFormat(c, info, lastStreamData, forceFormat, thinkToContent)
	if shouldSendLastResp && info.RelayFormat == relaycommon.RelayFormatOpenAI {
		_ = sendStreamData(c, info, lastStreamData, forceFormat, thinkToContent)
	}

	// 处理token计算
	if err := processTokens(info.RelayMode, streamItems, &responseTextBuilder, &toolCount); err != nil {
		common.SysError("error processing tokens: " + err.Error())
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(responseTextBuilder.String(), info.UpstreamModelName, info.PromptTokens)
		usage.CompletionTokens += toolCount * 7
	} else {
		if info.ChannelType == constant.ChannelTypeDeepSeek {
			if usage.PromptCacheHitTokens != 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	}

	handleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	return usage, nil
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

	// 再尝试匹配base64格式: data:image/type;base64,data
	base64Pattern := `^(.*?)\s*(data:image/[^;]+;base64,[A-Za-z0-9+/=]+)\s*$`
	base64Re := regexp.MustCompile(`(?s)` + base64Pattern)

	if matches := base64Re.FindStringSubmatch(input); len(matches) == 3 {
		text = strings.TrimSpace(matches[1])
		imageData := matches[2]
		return text, imageData, nil
	}

	// 如果都不匹配，返回错误
	return "", "", fmt.Errorf("字符串格式不匹配，既不是Markdown格式也不是base64格式")

}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer common.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	if strings.Contains(info.BaseUrl, "aiguoguo") && strings.Contains(info.UpstreamModelName, "gemini-2.5-flash-image") {
		// "content": "没问题，这是添加了哆啦A梦的图片：\n![Image_1](https://img.aiguoguo199.com/file/BQACAgUAAyEGAASaOQ3XAALZo2jnt2LJXF_Do-uv5TWSIXZ6wAT0AAJIHQACAs44V_l87WpYb56INgQ.png)"
		// 处理图片地址，改成toiotech的地址
		for i, choice := range simpleResponse.Choices {
			if choice.Message.Content != nil {
				// 解析出图片地址
				text, imgUrl, err1 := ParseTextAndImageURL(choice.Message.Content.(string))
				if err1 != nil {
					continue
				}
				imgUrl, err1 = service.SimpleUploadToS3(c.Request.Context(), imgUrl)
				if err1 != nil {
					continue
				}
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
			}
		}

		responseBody, err = common.Marshal(simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	}
	if simpleResponse.Error != nil && simpleResponse.Error.Type != "" {
		return nil, types.WithOpenAIError(*simpleResponse.Error, resp.StatusCode)
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	if simpleResponse.Usage.TotalTokens == 0 || (simpleResponse.Usage.PromptTokens == 0 && simpleResponse.Usage.CompletionTokens == 0) {
		completionTokens := 0
		for _, choice := range simpleResponse.Choices {
			ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.ReasoningContent+choice.Message.Reasoning, info.UpstreamModelName)
			completionTokens += ctkm
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.PromptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      info.PromptTokens + completionTokens,
		}
	}

	switch info.RelayFormat {
	case relaycommon.RelayFormatOpenAI:
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case relaycommon.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(&simpleResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	}

	common.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func OpenaiTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) *dto.Usage {
	// the status code has been judged before, if there is a body reading failure,
	// it should be regarded as a non-recoverable error, so it should not return err for external retry.
	// Analogous to nginx's load balancing, it will only retry if it can't be requested or
	// if the upstream returns a specific status code, once the upstream has already written the header,
	// the subsequent failure of the response body should be regarded as a non-recoverable error,
	// and can be terminated directly.
	defer common.CloseResponseBodyGracefully(resp)
	usage := &dto.Usage{}
	usage.PromptTokens = info.PromptTokens
	usage.TotalTokens = info.PromptTokens
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		common.LogError(c, err.Error())
	}
	return usage
}

func OpenaiSTTHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*types.NewAPIError, *dto.Usage) {
	defer common.CloseResponseBodyGracefully(resp)

	// count tokens by audio file duration
	audioTokens, err := countAudioTokens(c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed), nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed), nil
	}
	// 写入新的 response body
	common.IOCopyBytesGracefully(c, resp, responseBody)

	usage := &dto.Usage{}
	usage.PromptTokens = audioTokens
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}

func countAudioTokens(c *gin.Context) (int, error) {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var reqBody struct {
		File *multipart.FileHeader `form:"file" binding:"required"`
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err = c.ShouldBind(&reqBody); err != nil {
		return 0, errors.WithStack(err)
	}
	ext := filepath.Ext(reqBody.File.Filename) // 获取文件扩展名
	reqFp, err := reqBody.File.Open()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer reqFp.Close()

	tmpFp, err := os.CreateTemp("", "audio-*"+ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer os.Remove(tmpFp.Name())

	_, err = io.Copy(tmpFp, reqFp)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if err = tmpFp.Close(); err != nil {
		return 0, errors.WithStack(err)
	}

	duration, err := common.GetAudioDuration(c.Request.Context(), tmpFp.Name(), ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(math.Round(math.Ceil(duration) / 60.0 * 1000)), nil // 1 minute 相当于 1k tokens
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
				common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
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

				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
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
						common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
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
					common.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
					common.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
					common.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

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
					common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
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
		common.LogError(c, "realtime error: "+err.Error())
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
	defer common.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	// 写入新的 response body
	common.IOCopyBytesGracefully(c, resp, responseBody)

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
	return &usageResp.Usage, nil
}
