package awsv2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	bedrockTypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/auth/bearer"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// newAwsClient creates a new AWS Bedrock client
func newAwsClient(c *gin.Context, info *relaycommon.RelayInfo) (*bedrockruntime.Client, error) {
	var httpClient *http.Client
	var err error

	if info.ChannelSetting.Proxy != "" {
		httpClient, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		httpClient = service.GetHttpClient()
	}

	awsSecret := strings.Split(info.ApiKey, "|")
	var client *bedrockruntime.Client

	switch len(awsSecret) {
	case 2:
		// API Key mode: apiKey|region
		apiKey := awsSecret[0]
		region := awsSecret[1]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:                  region,
			BearerAuthTokenProvider: bearer.StaticTokenProvider{Token: bearer.Token{Value: apiKey}},
			HTTPClient:              httpClient,
		})
	case 3:
		// AKSK mode: accessKey|secretKey|region
		ak := awsSecret[0]
		sk := awsSecret[1]
		region := awsSecret[2]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:      region,
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(ak, sk, "")),
			HTTPClient:  httpClient,
		})
	default:
		return nil, errors.New("invalid aws secret key format, expected: apiKey|region or accessKey|secretKey|region")
	}

	return client, nil
}

// getAwsErrorStatusCode extracts HTTP status code from AWS SDK error
func getAwsErrorStatusCode(err error) int {
	var httpErr interface{ HTTPStatusCode() int }
	if errors.As(err, &httpErr) {
		return httpErr.HTTPStatusCode()
	}
	return http.StatusInternalServerError
}

// doAwsConverseRequest performs the AWS Converse API request
func doAwsConverseRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor, requestBody io.Reader) (any, error) {
	awsCli, err := newAwsClient(c, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelAwsClientError)
	}
	a.AwsClient = awsCli

	// Get AWS model ID
	awsModelId := getAwsModelID(info.UpstreamModelName)

	// Handle cross-region inference
	awsRegionPrefix := getAwsRegionPrefix(awsCli.Options().Region)
	if awsModelCanCrossRegion(awsModelId, awsRegionPrefix) {
		awsModelId = awsModelCrossRegion(awsModelId, awsRegionPrefix)
	}
	a.AwsModelId = awsModelId

	// Parse the request
	var converseReq *ConverseRequest
	err = common.DecodeJson(requestBody, &converseReq)
	if err != nil {
		return nil, types.NewError(errors.Wrap(err, "decode converse request fail"), types.ErrorCodeBadRequestBody)
	}

	// Build SDK request
	messages := buildConverseMessages(converseReq)
	system := buildSystemContent(converseReq.System)
	inferenceConfig := buildInferenceConfig(converseReq.InferenceConfig)
	toolConfig := buildToolConfig(converseReq.ToolConfig)

	if info.IsStream {
		// Stream request
		streamInput := &bedrockruntime.ConverseStreamInput{
			ModelId:         aws.String(awsModelId),
			Messages:        messages,
			System:          system,
			InferenceConfig: inferenceConfig,
			ToolConfig:      toolConfig,
		}
		// Add AdditionalModelRequestFields for reasoning config
		if len(converseReq.AdditionalModelRequestFields) > 0 {
			streamInput.AdditionalModelRequestFields = document.NewLazyDocument(converseReq.AdditionalModelRequestFields)
		}
		a.AwsReq = streamInput
	} else {
		// Non-stream request
		converseInput := &bedrockruntime.ConverseInput{
			ModelId:         aws.String(awsModelId),
			Messages:        messages,
			System:          system,
			InferenceConfig: inferenceConfig,
			ToolConfig:      toolConfig,
		}
		// Add AdditionalModelRequestFields for reasoning config
		if len(converseReq.AdditionalModelRequestFields) > 0 {
			converseInput.AdditionalModelRequestFields = document.NewLazyDocument(converseReq.AdditionalModelRequestFields)
		}
		a.AwsReq = converseInput
	}

	return nil, nil
}

// awsConverseHandler handles non-streaming Converse API response
func awsConverseHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NewAPIError, *dto.Usage) {
	ctx := c.Request.Context()

	converseInput, ok := a.AwsReq.(*bedrockruntime.ConverseInput)
	if !ok {
		return types.NewError(errors.New("invalid request type"), types.ErrorCodeInvalidRequest), nil
	}

	resp, err := a.AwsClient.Converse(ctx, converseInput)
	if err != nil {
		statusCode := getAwsErrorStatusCode(err)
		return types.NewOpenAIError(errors.Wrap(err, "Converse API error"), types.ErrorCodeAwsInvokeError, statusCode), nil
	}

	// Convert response to OpenAI format
	openaiResp := convertConverseResponseToOpenAI(c, resp, info)

	// 记录响应到 context 供日志使用
	if respJson, err := json.Marshal(openaiResp); err == nil {
		c.Set(string(constant.ContextKeySdkResponseStr), string(respJson))
	}

	c.JSON(http.StatusOK, openaiResp)
	return nil, &openaiResp.Usage
}

// awsConverseStreamHandler handles streaming Converse API response
func awsConverseStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NewAPIError, *dto.Usage) {
	ctx := c.Request.Context()

	streamInput, ok := a.AwsReq.(*bedrockruntime.ConverseStreamInput)
	if !ok {
		return types.NewError(errors.New("invalid request type"), types.ErrorCodeInvalidRequest), nil
	}

	resp, err := a.AwsClient.ConverseStream(ctx, streamInput)
	if err != nil {
		statusCode := getAwsErrorStatusCode(err)
		return types.NewOpenAIError(errors.Wrap(err, "ConverseStream API error"), types.ErrorCodeAwsInvokeError, statusCode), nil
	}

	stream := resp.GetStream()
	defer stream.Close()

	return handleConverseStream(c, info, stream, a.AwsModelId)
}

// handleConverseStream processes the streaming response
func handleConverseStream(c *gin.Context, info *relaycommon.RelayInfo, stream *bedrockruntime.ConverseStreamEventStream, modelId string) (*types.NewAPIError, *dto.Usage) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	responseId := helper.GetResponseID(c)
	created := common.GetTimestamp()
	usage := &dto.Usage{}
	var responseText strings.Builder
	var reasoningText strings.Builder

	// Track tool use state for streaming
	var currentToolUseId string
	var currentToolName string
	var currentToolInput strings.Builder
	var toolCalls []dto.ToolCallRequest
	var currentContentBlockIndex int
	var finishReason string

	for event := range stream.Events() {
		info.SetFirstResponseTime()

		switch v := event.(type) {
		case *bedrockTypes.ConverseStreamOutputMemberContentBlockStart:
			// Content block started - check if it's a tool use
			if v.Value.Start != nil {
				if toolUseStart, ok := v.Value.Start.(*bedrockTypes.ContentBlockStartMemberToolUse); ok {
					currentToolUseId = aws.ToString(toolUseStart.Value.ToolUseId)
					currentToolName = aws.ToString(toolUseStart.Value.Name)
					currentToolInput.Reset()

					// Send tool call start chunk
					chunk := createToolCallStartChunk(responseId, created, info.UpstreamModelName, currentContentBlockIndex, currentToolUseId, currentToolName)
					sendSSEEvent(c, chunk)
				}
			}
			currentContentBlockIndex++

		case *bedrockTypes.ConverseStreamOutputMemberContentBlockDelta:
			delta := v.Value.Delta
			var text string
			var isReasoning bool

			switch d := delta.(type) {
			case *bedrockTypes.ContentBlockDeltaMemberText:
				text = d.Value
			case *bedrockTypes.ContentBlockDeltaMemberReasoningContent:
				// Handle reasoning content for Nova 2 models
				if textDelta, ok := d.Value.(*bedrockTypes.ReasoningContentBlockDeltaMemberText); ok {
					text = textDelta.Value
					isReasoning = true
				}
			case *bedrockTypes.ContentBlockDeltaMemberToolUse:
				// Tool use input delta
				if d.Value.Input != nil {
					currentToolInput.WriteString(*d.Value.Input)
					// Send tool call arguments delta chunk
					chunk := createToolCallDeltaChunk(responseId, created, info.UpstreamModelName, currentContentBlockIndex-1, *d.Value.Input)
					sendSSEEvent(c, chunk)
				}
				continue
			}

			if text != "" {
				if isReasoning {
					reasoningText.WriteString(text)
				} else {
					responseText.WriteString(text)
				}

				// Send SSE event
				chunk := createStreamChunk(responseId, created, info.UpstreamModelName, text, isReasoning)
				sendSSEEvent(c, chunk)
			}

		case *bedrockTypes.ConverseStreamOutputMemberContentBlockStop:
			// Content block ended - if it was a tool use, save it
			if currentToolUseId != "" {
				toolCall := dto.ToolCallRequest{
					ID:   currentToolUseId,
					Type: "function",
					Function: dto.FunctionRequest{
						Name:      currentToolName,
						Arguments: currentToolInput.String(),
					},
				}
				toolCalls = append(toolCalls, toolCall)
				currentToolUseId = ""
				currentToolName = ""
			}

		case *bedrockTypes.ConverseStreamOutputMemberMessageStop:
			// Message completed
			finishReason = convertStopReason(v.Value.StopReason)

		case *bedrockTypes.ConverseStreamOutputMemberMetadata:
			// Extract usage from metadata
			if v.Value.Usage != nil {
				usage.PromptTokens = int(aws.ToInt32(v.Value.Usage.InputTokens))
				usage.CompletionTokens = int(aws.ToInt32(v.Value.Usage.OutputTokens))
				usage.TotalTokens = int(aws.ToInt32(v.Value.Usage.TotalTokens))
			}
		}
	}

	// Determine finish reason
	if finishReason == "" {
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	}

	// Send final chunk with finish_reason
	finalChunk := createFinalStreamChunk(responseId, created, info.UpstreamModelName, usage, info.ShouldIncludeUsage)
	finalChunk.Choices[0].FinishReason = &finishReason
	sendSSEEvent(c, finalChunk)

	// Send [DONE]
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	c.Writer.Flush()

	// 记录流式响应的最终内容到 context 供日志使用
	streamResponse := map[string]any{
		"id":      responseId,
		"model":   info.UpstreamModelName,
		"content": responseText.String(),
		"usage":   usage,
	}
	if reasoningText.Len() > 0 {
		streamResponse["reasoning_content"] = reasoningText.String()
	}
	if len(toolCalls) > 0 {
		streamResponse["tool_calls"] = toolCalls
	}
	if respJson, err := json.Marshal(streamResponse); err == nil {
		c.Set(string(constant.ContextKeySdkResponseStr), string(respJson))
	}

	return nil, usage
}

// convertConverseResponseToOpenAI converts Converse API response to OpenAI format
func convertConverseResponseToOpenAI(c *gin.Context, resp *bedrockruntime.ConverseOutput, info *relaycommon.RelayInfo) *dto.OpenAITextResponse {
	var content string
	var reasoningContent string
	var toolCalls []dto.ToolCallRequest

	if resp.Output != nil {
		if msgOutput, ok := resp.Output.(*bedrockTypes.ConverseOutputMemberMessage); ok {
			for _, block := range msgOutput.Value.Content {
				switch b := block.(type) {
				case *bedrockTypes.ContentBlockMemberText:
					content = b.Value
				case *bedrockTypes.ContentBlockMemberReasoningContent:
					// Handle reasoning content
					if textBlock, ok := b.Value.(*bedrockTypes.ReasoningContentBlockMemberReasoningText); ok {
						if textBlock.Value.Text != nil {
							reasoningContent = *textBlock.Value.Text
						}
					}
				case *bedrockTypes.ContentBlockMemberToolUse:
					// Handle tool use
					toolUse := b.Value
					// Input is document.Interface, need to unmarshal it properly
					var inputMap map[string]any
					if toolUse.Input != nil {
						_ = toolUse.Input.UnmarshalSmithyDocument(&inputMap)
					}
					// Convert input map to JSON string for arguments
					argsJSON, _ := json.Marshal(inputMap)
					toolCall := dto.ToolCallRequest{
						ID:   aws.ToString(toolUse.ToolUseId),
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      aws.ToString(toolUse.Name),
							Arguments: string(argsJSON),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				}
			}
		}
	}

	// Build response
	response := &dto.OpenAITextResponse{
		Id:      helper.GetResponseID(c),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: convertStopReason(resp.StopReason),
			},
		},
	}

	// Add tool_calls if present
	if len(toolCalls) > 0 {
		response.Choices[0].Message.SetToolCalls(toolCalls)
	}

	// Add reasoning content if present
	if reasoningContent != "" {
		response.Choices[0].Message.ReasoningContent = common.GetPointer(reasoningContent)
	}

	// Set usage
	if resp.Usage != nil {
		response.Usage = dto.Usage{
			PromptTokens:     int(aws.ToInt32(resp.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(resp.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(resp.Usage.TotalTokens)),
		}
	}

	return response
}

// convertStopReason converts AWS stop reason to OpenAI finish_reason
func convertStopReason(reason bedrockTypes.StopReason) string {
	switch reason {
	case bedrockTypes.StopReasonEndTurn:
		return "stop"
	case bedrockTypes.StopReasonMaxTokens:
		return "length"
	case bedrockTypes.StopReasonStopSequence:
		return "stop"
	case bedrockTypes.StopReasonToolUse:
		return "tool_calls"
	case bedrockTypes.StopReasonContentFiltered:
		return "content_filter"
	default:
		return "stop"
	}
}

// createStreamChunk creates an SSE chunk for streaming
func createStreamChunk(id string, created int64, model string, content string, isReasoning bool) *dto.ChatCompletionsStreamResponse {
	delta := dto.ChatCompletionsStreamResponseChoice{
		Index: 0,
		Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
	}

	if isReasoning {
		delta.Delta.SetReasoningContent(content)
	} else {
		delta.Delta.SetContentString(content)
	}

	return &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{delta},
	}
}

// createToolCallStartChunk creates an SSE chunk for tool call start
func createToolCallStartChunk(id string, created int64, model string, index int, toolUseId string, toolName string) *dto.ChatCompletionsStreamResponse {
	toolCallIndex := index
	return &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &toolCallIndex,
							ID:    toolUseId,
							Type:  "function",
							Function: dto.FunctionResponse{
								Name:      toolName,
								Arguments: "",
							},
						},
					},
				},
			},
		},
	}
}

// createToolCallDeltaChunk creates an SSE chunk for tool call arguments delta
func createToolCallDeltaChunk(id string, created int64, model string, index int, arguments string) *dto.ChatCompletionsStreamResponse {
	toolCallIndex := index
	return &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &toolCallIndex,
							Function: dto.FunctionResponse{
								Arguments: arguments,
							},
						},
					},
				},
			},
		},
	}
}

// createFinalStreamChunk creates the final SSE chunk with finish_reason
func createFinalStreamChunk(id string, created int64, model string, usage *dto.Usage, includeUsage bool) *dto.ChatCompletionsStreamResponse {
	finishReason := "stop"
	chunk := &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index:        0,
				Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
				FinishReason: &finishReason,
			},
		},
	}

	if includeUsage && usage != nil {
		chunk.Usage = usage
	}

	return chunk
}

// sendSSEEvent sends an SSE event to the client
func sendSSEEvent(c *gin.Context, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	c.Writer.Write([]byte("data: "))
	c.Writer.Write(jsonData)
	c.Writer.Write([]byte("\n\n"))
	c.Writer.Flush()
}
