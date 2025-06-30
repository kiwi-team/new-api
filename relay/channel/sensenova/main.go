package sensenova

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/relay/helper"
	"one-api/service"
	"strings"

	"github.com/gin-gonic/gin"
)

func requestOpenAI2Sensenova(request *dto.GeneralOpenAIRequest) *ChatRequest {
	sensenovaMessages := make([]Message, len(request.Messages))
	for i, message := range request.Messages {
		contentArray := make([]ContentItem, 0)
		for _, content := range message.ParseContent() {
			if content.Type == dto.ContentTypeText {
				contentArray = append(contentArray, ContentItem{
					Type: "text",
					Text: content.Text,
				})
			} else if content.Type == dto.ContentTypeImageURL {
				imageUrl := content.ImageUrl.(*dto.MessageImageUrl).Url
				if strings.HasPrefix(imageUrl, "data:") {
					contentArray = append(contentArray, ContentItem{
						Type:        "image_base64",
						ImageBase64: imageUrl,
					})
				} else {
					contentArray = append(contentArray, ContentItem{
						Type:     "image_url",
						ImageUrl: imageUrl,
					})

				}
			} else if content.Type == dto.ContentTypeVideoUrl {
				contentArray = append(contentArray, ContentItem{
					Type:     "video_url",
					VideoUrl: content.VideoUrl.(*dto.MessageVideoUrl).Url,
				})
			}
		}
		sensenovaMessages[i] = Message{
			Role:    message.Role,
			Content: contentArray,
		}
	}
	chatRequest := &ChatRequest{
		Model:    request.Model,
		Messages: sensenovaMessages,
		MaxNewTokens: func() int {
			if request.MaxTokens > 0 {
				return int(request.MaxTokens)
			}
			return 1024
		}(),
		Temperature: func() float64 {
			if request.Temperature != nil && *request.Temperature > 0 {
				return *request.Temperature
			}
			return 0.8
		}(),
		TopP: func() float64 {
			if request.TopP > 0 {
				return request.TopP
			}
			return 0.95
		}(),
		RepetitionPenalty: 1.0,
		Stream:            request.Stream,
		User:              request.User,
	}
	return chatRequest
}

func sensenovaHandler(c *gin.Context, resp *http.Response) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	var newResponse ChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return service.OpenAIErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &newResponse)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	fullTextResponse := responseSensenova2OpenAI(&newResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func sensenovaStreamHandler(c *gin.Context, resp *http.Response) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	var usage dto.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "\n"); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	helper.SetEventStreamHeaders(c)

	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < 5 || data[:5] != "data:" {
			continue
		}
		data = data[5:]

		var newResponse StreamResponse
		err := json.Unmarshal([]byte(data), &newResponse)
		if err != nil {
			common.SysError("error unmarshalling stream response: " + err.Error())
			continue
		}
		if newResponse.Data.Usage.PromptTokens > usage.PromptTokens {
			usage.PromptTokens = newResponse.Data.Usage.PromptTokens
		}
		if newResponse.Data.Usage.CompletionTokens > usage.CompletionTokens {
			usage.CompletionTokens = newResponse.Data.Usage.CompletionTokens
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		response := streamResponse2OpenAI(&newResponse)
		if response == nil {
			continue
		}
		err = helper.ObjectData(c, response)
		if err != nil {
			common.SysError("error writing stream response: " + err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		common.SysError("error reading stream: " + err.Error())
	}

	helper.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return service.OpenAIErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, &usage
}

func responseSensenova2OpenAI(response *ChatResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, len(response.Data.Choices))
	for i, choice := range response.Data.Choices {
		choices[i] = dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:             choice.Role,
				Content:          choice.Message,
				ReasoningContent: choice.ReasoningContent,
			},
			FinishReason: choice.FinishReason,
		}
	}
	newResponse := &dto.OpenAITextResponse{
		Id:      response.Data.ID,
		Choices: choices,
		Usage: dto.Usage{
			PromptTokens:     response.Data.Usage.PromptTokens,
			CompletionTokens: response.Data.Usage.CompletionTokens,
			TotalTokens:      response.Data.Usage.TotalTokens,
		},
	}
	return newResponse
}

func streamResponse2OpenAI(newResponse *StreamResponse) *dto.ChatCompletionsStreamResponse {
	if len(newResponse.Data.Choices) == 0 {
		return nil
	}
	newChoice := newResponse.Data.Choices[0]
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta = dto.ChatCompletionsStreamResponseChoiceDelta{
		Role:             newChoice.Role,
		Content:          &newChoice.Delta,
		ReasoningContent: &newChoice.ReasoningContent,
	}
	if newChoice.FinishReason != "null" {
		finishReason := newChoice.FinishReason
		choice.FinishReason = &finishReason
	}
	response := dto.ChatCompletionsStreamResponse{
		Id:      newResponse.Data.ID,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}
