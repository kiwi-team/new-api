package gemini

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ConvertOpenAI2DeepResearch converts an OpenAI chat completion request to a Gemini deep research request.
// It extracts the last user message as the research input query.
func ConvertOpenAI2DeepResearch(request *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) (*dto.GeminiDeepResearchRequest, error) {
	// Extract the input from the last user message, concatenating all messages as context
	var inputParts []string
	for _, msg := range request.Messages {
		content := msg.StringContent()
		if content == "" {
			continue
		}
		switch msg.Role {
		case "system":
			inputParts = append(inputParts, content)
		case "user":
			inputParts = append(inputParts, content)
		case "assistant":
			inputParts = append(inputParts, content)
		}
	}

	if len(inputParts) == 0 {
		return nil, fmt.Errorf("no input content found in messages")
	}

	// Use the last user message as input; if there are system/assistant messages, combine them
	input := strings.Join(inputParts, "\n\n")

	deepResearchReq := &dto.GeminiDeepResearchRequest{
		Input:      input,
		Agent:      info.UpstreamModelName,
		Background: true,
		Stream:     true,
		AgentConfig: &dto.GeminiDeepResearchAgentConfig{
			ThinkingSummaries: "auto",
		},
	}

	return deepResearchReq, nil
}

// DeepResearchStreamHandler handles the SSE stream from the deep research interactions API
// and converts it to OpenAI chat completions streaming format.
func DeepResearchStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	id := helper.GetResponseID(c)
	createdAt := common.GetTimestamp()
	responseText := strings.Builder{}

	// SSE headers are already set by doRequest when info.IsStream is true

	// Send the initial empty response with role
	emptyResponse := helper.GenerateStartEmptyResponse(id, createdAt, info.UpstreamModelName, nil)
	err := deepResearchHandleStream(c, info, emptyResponse)
	if err != nil {
		logger.LogError(c, "failed to send start response: "+err.Error())
	}

	// Parse SSE events from the deep research response
	deepResearchSSEHandler(c, info, resp, func(eventType string, data string) {
		var event dto.GeminiDeepResearchSSEEvent
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			logger.LogError(c, "error unmarshalling deep research SSE event: "+err.Error())
			return
		}
		event.Event = eventType

		switch eventType {
		case "content.delta":
			if event.Delta == nil {
				return
			}
			var text string
			switch event.Delta.Type {
			case "text":
				text = event.Delta.Text
			case "thought_summary":
				if event.Delta.Content != nil {
					// Map thought summaries to reasoning content (thinking)
					text = event.Delta.Content.Text
				}
			}
			if text == "" {
				return
			}
			responseText.WriteString(text)

			response := &dto.ChatCompletionsStreamResponse{
				Id:      id,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   info.UpstreamModelName,
				Choices: []dto.ChatCompletionsStreamResponseChoice{
					{
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							Content: &text,
						},
					},
				},
			}
			if err := deepResearchHandleStream(c, info, response); err != nil {
				logger.LogError(c, "failed to send content delta: "+err.Error())
			}

		case "interaction.complete":
			// Send stop response
			finishReason := constant.FinishReasonStop
			stopResponse := helper.GenerateStopResponse(id, createdAt, info.UpstreamModelName, finishReason)
			if err := deepResearchHandleStream(c, info, stopResponse); err != nil {
				logger.LogError(c, "failed to send stop response: "+err.Error())
			}
		}
	})

	// Calculate usage - deep research doesn't return token counts, so estimate
	usage := service.ResponseText2Usage(c, responseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())

	// Send final usage response
	finalResponse := helper.GenerateFinalUsageResponse(id, createdAt, info.UpstreamModelName, *usage)
	if handleErr := deepResearchHandleFinalStream(c, info, finalResponse); handleErr != nil {
		common.SysLog("send final response failed: " + handleErr.Error())
	}

	return usage, nil
}

// deepResearchSSEHandler reads SSE events from the deep research response.
// Deep research SSE format has "event:" and "data:" lines.
func deepResearchSSEHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, callback func(eventType string, data string)) {
	if resp == nil || resp.Body == nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0), 1024*1024) // 1MB buffer

	var currentEvent string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line signals end of an event
		if line == "" && currentEvent != "" && len(dataLines) > 0 {
			data := strings.Join(dataLines, "\n")
			callback(currentEvent, data)
			currentEvent = ""
			dataLines = nil
		}
	}

	// Handle last event if stream ended without trailing empty line
	if currentEvent != "" && len(dataLines) > 0 {
		data := strings.Join(dataLines, "\n")
		callback(currentEvent, data)
	}

	if err := scanner.Err(); err != nil {
		logger.LogError(c, "deep research SSE scanner error: "+err.Error())
	}
}

func deepResearchHandleStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	err = openai.HandleStreamFormat(c, info, string(streamData), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	if err != nil {
		return fmt.Errorf("failed to handle stream format: %w", err)
	}
	return nil
}

func deepResearchHandleFinalStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	openai.HandleFinalResponse(c, info, string(streamData), resp.Id, resp.Created, resp.Model, resp.GetSystemFingerprint(), resp.Usage, false)
	return nil
}
