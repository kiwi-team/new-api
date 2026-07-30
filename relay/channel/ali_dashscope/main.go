package ali_dashscope

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

// https://help.aliyun.com/document_detail/613695.html?spm=a2c4g.2399480.0.0.1adb778fAdzP9w#341800c0f8w0r

const EnableSearchModelSuffix = "-internet"

func ConvertRequest(request dto.GeneralOpenAIRequest) *ChatRequest {
	messages := make([]Message, 0, len(request.Messages))
	for i := 0; i < len(request.Messages); i++ {
		message := request.Messages[i]
		messages = append(messages, Message{
			Content: message.StringContent(),
			Role:    strings.ToLower(message.Role),
		})
	}
	enableSearch := false
	aliModel := request.Model
	if strings.HasSuffix(aliModel, EnableSearchModelSuffix) {
		enableSearch = true
		aliModel = strings.TrimSuffix(aliModel, EnableSearchModelSuffix)
	}
	// DashScope 的 top_p 是开区间 (0, 1)，边界值会被上游拒绝，这里钳到合法范围。
	// request.TopP 为 nil 表示客户端未传，保持不下发。
	var topP *float64
	if request.TopP != nil {
		v := *request.TopP
		if v < 0.000001 {
			v = 0.000001
		} else if v > 0.9999 {
			v = 0.9999
		}
		topP = &v
	}
	extraBody := make(map[string]any)
	var codeInterpreter bool
	var enableThinking bool
	json.Unmarshal(request.ExtraBody, &extraBody)
	if enableCodeInterpreter, ok := extraBody["enable_code_interpreter"]; ok {
		codeInterpreter = enableCodeInterpreter.(bool)
	}
	if tmp, ok := extraBody["enable_thinking"]; ok {
		enableThinking = tmp.(bool)
	} else if len(request.EnableThinking) > 0 {
		_ = common.Unmarshal(request.EnableThinking, &enableThinking)
	}

	return &ChatRequest{
		Model: aliModel,
		Input: Input{
			Messages: messages,
		},
		Parameters: Parameters{
			EnableSearch:          enableSearch,
			IncrementalOutput:     lo.FromPtr(request.Stream),
			Seed:                  uint64(lo.FromPtr(request.Seed)),
			MaxTokens:             int(lo.FromPtr(request.MaxTokens)),
			Temperature:           request.Temperature,
			TopP:                  topP,
			TopK:                  lo.FromPtr(request.TopK),
			ResultFormat:          "message",
			Tools:                 request.Tools,
			EnableCodeInterpreter: codeInterpreter,
			EnableThinking:        enableThinking,
		},
	}
}

func IsDeepResearchModel(model string) bool {
	return strings.HasPrefix(model, "qwen-deep-research")
}

func ConvertDeepResearchRequest(request dto.GeneralOpenAIRequest) *DeepResearchChatRequest {
	messages := make([]Message, 0, len(request.Messages))
	for i := 0; i < len(request.Messages); i++ {
		message := request.Messages[i]
		messages = append(messages, Message{
			Content: message.StringContent(),
			Role:    strings.ToLower(message.Role),
		})
	}
	params := DeepResearchParameters{
		Stream:            true,
		IncrementalOutput: true,
		EnableFeedback:    false,
		MaxTokens:         int(lo.FromPtr(request.MaxTokens)),
		Temperature:       request.Temperature,
	}
	if params.MaxTokens == 0 {
		params.MaxTokens = 32768
	}
	return &DeepResearchChatRequest{
		Model: request.Model,
		Input: Input{
			Messages: messages,
		},
		Parameters: params,
	}
}

func deepResearchStreamResponseToOpenAI(response *DeepResearchChatResponse, info *relaycommon.RelayInfo) *dto.ChatCompletionsStreamResponse {
	// Skip KeepAlive phase
	if response.Output.Message.Phase == "KeepAlive" {
		return nil
	}

	content := response.Output.Message.Content
	if content == "" {
		return nil
	}

	var choice dto.ChatCompletionsStreamResponseChoice
	phase := response.Output.Message.Phase

	if phase == "answer" {
		// Answer phase -> regular content
		choice.Delta.Content = &content
	} else {
		// ResearchPlanning, WebResearch -> reasoning_content
		choice.Delta.ReasoningContent = &content
	}

	if response.Output.Message.Status == "finished" && phase == "answer" {
		finishReason := "stop"
		choice.FinishReason = &finishReason
	}

	return &dto.ChatCompletionsStreamResponse{
		Id:      response.RequestId,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
}

func DeepResearchStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
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

		var aliResponse DeepResearchChatResponse
		err := json.Unmarshal([]byte(data), &aliResponse)
		if err != nil {
			common.SysError("error unmarshalling deep research stream response: " + err.Error())
			continue
		}
		if aliResponse.Usage.OutputTokens != 0 {
			usage.PromptTokens = aliResponse.Usage.InputTokens
			usage.CompletionTokens = aliResponse.Usage.OutputTokens
			usage.TotalTokens = aliResponse.Usage.InputTokens + aliResponse.Usage.OutputTokens
		}
		response := deepResearchStreamResponseToOpenAI(&aliResponse, info)
		if response == nil {
			continue
		}
		response.Usage = &usage
		err = helper.ObjectData(c, response)
		if err != nil {
			common.SysError(err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		common.SysError("error reading deep research stream: " + err.Error())
	}

	helper.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return types.NewOpenAIError(fmt.Errorf("close_response_body_failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	return nil, &usage
}

func ConvertEmbeddingRequest(request dto.GeneralOpenAIRequest) *EmbeddingRequest {
	return &EmbeddingRequest{
		Model: request.Model,
		Input: struct {
			Texts []string `json:"texts"`
		}{
			Texts: request.ParseInput(),
		},
	}
}

func ConvertImageRequest(request dto.ImageRequest) *ImageRequest {
	var imageRequest ImageRequest
	imageRequest.Input.Prompt = request.Prompt
	imageRequest.Model = request.Model
	imageRequest.Parameters.Size = strings.Replace(request.Size, "x", "*", -1)
	imageRequest.Parameters.N = int(lo.FromPtr(request.N))
	imageRequest.ResponseFormat = request.ResponseFormat

	return &imageRequest
}

func responseAli2OpenAI(response *ChatResponse, info *relaycommon.RelayInfo) *dto.TextResponse {

	fullTextResponse := dto.TextResponse{
		Id:      response.RequestId,
		Object:  "chat.completion",
		Model:   info.UpstreamModelName,
		Created: common.GetTimestamp(),
		Choices: response.Output.Choices,
		Usage: dto.Usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}
	fullTextResponse.Usage.PromptTokensDetails = dto.InputTokenDetails{
		CachedTokens: response.Usage.PromptTokensDetails.CachedTokens,
	}
	fullTextResponse.Usage.CompletionTokenDetails = dto.OutputTokenDetails{
		ReasoningTokens: response.Usage.OutputTokensDetails.ReasoningTokens,
	}
	return &fullTextResponse
}

func streamResponseAli2OpenAI(aliResponse *ChatResponse, info *relaycommon.RelayInfo) *dto.ChatCompletionsStreamResponse {
	if len(aliResponse.Output.Choices) == 0 {
		return nil
	}
	aliChoice := aliResponse.Output.Choices[0]
	var choice dto.ChatCompletionsStreamResponseChoice
	if contentStr, ok := aliChoice.Message.Content.(string); ok {
		choice.Delta.Content = &contentStr
	}
	choice.Delta.ReasoningContent = aliChoice.Message.ReasoningContent
	if aliChoice.FinishReason != "null" {
		finishReason := aliChoice.FinishReason
		choice.FinishReason = &finishReason
	}
	response := dto.ChatCompletionsStreamResponse{
		Id:       aliResponse.RequestId,
		Object:   "chat.completion.chunk",
		Created:  common.GetTimestamp(),
		Model:    info.UpstreamModelName,
		Choices:  []dto.ChatCompletionsStreamResponseChoice{choice},
		ToolInfo: aliResponse.Output.ToolInfo,
	}
	return &response
}

func StreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
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

		var aliResponse ChatResponse
		err := json.Unmarshal([]byte(data), &aliResponse)
		if err != nil {
			common.SysError("error unmarshalling stream response: " + err.Error())
			continue
		}
		if aliResponse.Usage.OutputTokens != 0 {
			usage.PromptTokens = aliResponse.Usage.InputTokens
			usage.CompletionTokens = aliResponse.Usage.OutputTokens
			usage.TotalTokens = aliResponse.Usage.InputTokens + aliResponse.Usage.OutputTokens
			usage.PromptTokensDetails = dto.InputTokenDetails{
				CachedTokens: aliResponse.Usage.PromptTokensDetails.CachedTokens,
			}
			usage.CompletionTokenDetails = dto.OutputTokenDetails{
				ReasoningTokens: aliResponse.Usage.OutputTokensDetails.ReasoningTokens,
			}
		}
		response := streamResponseAli2OpenAI(&aliResponse, info)
		if response != nil {
			response.Usage = &usage
		}
		if response == nil {
			continue
		}
		err = helper.ObjectData(c, response)
		if err != nil {
			common.SysError(err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		common.SysError("error reading stream: " + err.Error())
	}

	helper.Done(c)

	err := resp.Body.Close()
	if err != nil {
		//return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
		return types.NewOpenAIError(fmt.Errorf("close_response_body_failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	return nil, &usage
}

func Handler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
	var aliResponse ChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		//return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
		return types.NewOpenAIError(fmt.Errorf("read_response_body_failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		//return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
		return types.NewOpenAIError(fmt.Errorf("close_response_body_failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &aliResponse)
	if err != nil {
		//return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
		return types.NewOpenAIError(errors.New("unmarshal_response_body_failed"), types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError), nil
	}
	if aliResponse.Code != "" {
		return types.NewOpenAIError(errors.New(aliResponse.Message), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
		/*
			return &model.ErrorWithStatusCode{
				Error: model.Error{
					Message: aliResponse.Message,
					Type:    aliResponse.Code,
					Param:   aliResponse.RequestId,
					Code:    aliResponse.Code,
				},
				StatusCode: resp.StatusCode,
			}, nil
		*/
	}
	fullTextResponse := responseAli2OpenAI(&aliResponse, info)
	fullTextResponse.Model = info.UpstreamModelName
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return types.NewOpenAIError(fmt.Errorf("marshal_response_body_failed"), types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}
