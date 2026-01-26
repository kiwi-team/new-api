package awsv2

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// ConverseRequest represents the request structure for AWS Bedrock Converse API
type ConverseRequest struct {
	Messages                     []ConverseMessage    `json:"messages"`
	System                       []SystemContentBlock `json:"system,omitempty"`
	InferenceConfig              *InferenceConfig     `json:"inferenceConfig,omitempty"`
	AdditionalModelRequestFields map[string]any       `json:"additionalModelRequestFields,omitempty"`
	ToolConfig                   *ToolConfig          `json:"toolConfig,omitempty"`
}

// ToolConfig represents tool configuration for the Converse API
type ToolConfig struct {
	Tools      []Tool      `json:"tools,omitempty"`
	ToolChoice *ToolChoice `json:"toolChoice,omitempty"`
}

// Tool represents a tool definition
type Tool struct {
	ToolSpec *ToolSpec `json:"toolSpec,omitempty"`
}

// ToolSpec represents the specification of a tool
type ToolSpec struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	InputSchema *InputSchema `json:"inputSchema,omitempty"`
}

// InputSchema represents the input schema for a tool
type InputSchema struct {
	JSON map[string]any `json:"json,omitempty"`
}

// ToolChoice represents tool choice configuration
type ToolChoice struct {
	Auto *struct{} `json:"auto,omitempty"`
	Any  *struct{} `json:"any,omitempty"`
	Tool *ToolName `json:"tool,omitempty"`
}

// ToolName represents a specific tool name for tool choice
type ToolName struct {
	Name string `json:"name"`
}

// ToolUseBlock represents a tool use in content
type ToolUseBlock struct {
	ToolUseId string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

// ToolResultBlock represents a tool result in content
type ToolResultBlock struct {
	ToolUseId string              `json:"toolUseId"`
	Content   []ToolResultContent `json:"content"`
	Status    string              `json:"status,omitempty"` // "success" or "error"
}

// ToolResultContent represents content in a tool result
type ToolResultContent struct {
	Text string `json:"text,omitempty"`
}

// ConverseMessage represents a message in the Converse API
type ConverseMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents content in a message
type ContentBlock struct {
	Text       string           `json:"text,omitempty"`
	Image      *ImageBlock      `json:"image,omitempty"`
	ToolUse    *ToolUseBlock    `json:"toolUse,omitempty"`
	ToolResult *ToolResultBlock `json:"toolResult,omitempty"`
}

// ImageBlock represents an image in content
type ImageBlock struct {
	Format string      `json:"format"`
	Source ImageSource `json:"source"`
}

// ImageSource represents the source of an image
type ImageSource struct {
	Bytes []byte `json:"bytes,omitempty"`
}

// SystemContentBlock represents system content
type SystemContentBlock struct {
	Text string `json:"text"`
}

// InferenceConfig represents inference configuration
type InferenceConfig struct {
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   *float32 `json:"temperature,omitempty"`
	TopP          *float32 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// ReasoningConfig for Nova 2 models
type ReasoningConfig struct {
	Type               string `json:"type"`
	MaxReasoningEffort string `json:"maxReasoningEffort,omitempty"`
}

// convertOpenAIToConverse converts an OpenAI request to Converse API format
func convertOpenAIToConverse(req *dto.GeneralOpenAIRequest) (*ConverseRequest, error) {
	converseReq := &ConverseRequest{
		Messages: make([]ConverseMessage, 0),
	}

	// Process messages
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			// System messages go to the system field
			converseReq.System = append(converseReq.System, SystemContentBlock{
				Text: msg.StringContent(),
			})
			continue
		}

		converseMsg := ConverseMessage{
			Role:    convertRole(msg.Role),
			Content: make([]ContentBlock, 0),
		}

		// Handle tool role (tool result)
		if msg.Role == "tool" {
			// Tool result message
			toolResult := &ToolResultBlock{
				ToolUseId: msg.ToolCallId,
				Content: []ToolResultContent{
					{Text: msg.StringContent()},
				},
				Status: "success",
			}
			converseMsg.Content = append(converseMsg.Content, ContentBlock{
				ToolResult: toolResult,
			})
			converseReq.Messages = append(converseReq.Messages, converseMsg)
			continue
		}

		// Handle assistant messages with tool_calls
		if msg.Role == "assistant" && msg.ToolCalls != nil {
			toolCalls := msg.ParseToolCalls()
			for _, tc := range toolCalls {
				// Parse arguments from JSON string to map
				var inputMap map[string]any
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &inputMap)
				}
				if inputMap == nil {
					inputMap = make(map[string]any)
				}

				toolUse := &ToolUseBlock{
					ToolUseId: tc.ID,
					Name:      tc.Function.Name,
					Input:     inputMap,
				}
				converseMsg.Content = append(converseMsg.Content, ContentBlock{
					ToolUse: toolUse,
				})
			}
			// Also add text content if present
			if msg.IsStringContent() && msg.StringContent() != "" {
				converseMsg.Content = append([]ContentBlock{{Text: msg.StringContent()}}, converseMsg.Content...)
			}
			converseReq.Messages = append(converseReq.Messages, converseMsg)
			continue
		}

		// Handle content
		if msg.IsStringContent() {
			converseMsg.Content = append(converseMsg.Content, ContentBlock{
				Text: msg.StringContent(),
			})
		} else {
			// Handle multimodal content
			contents := msg.ParseContent()
			for _, content := range contents {
				switch content.Type {
				case dto.ContentTypeText:
					converseMsg.Content = append(converseMsg.Content, ContentBlock{
						Text: content.Text,
					})
					// Image handling can be added here if needed
				}
			}
		}

		converseReq.Messages = append(converseReq.Messages, converseMsg)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		converseReq.ToolConfig = convertOpenAIToolsToConverse(req.Tools, req.ToolChoice)
	}

	if len(req.ReasoningEffort) > 0 {
		converseReq.AdditionalModelRequestFields = map[string]any{
			"reasoningConfig": map[string]any{
				"type":               "enabled",
				"maxReasoningEffort": req.ReasoningEffort,
			},
		}
	}
	isHigh := req.ReasoningEffort == "high"

	// Set inference config
	// Temperature, topP and topK cannot be used with maxReasoningEffort set to high. This will cause an error.
	if req.MaxTokens != 0 || req.Temperature != nil || req.TopP != 0 || req.Stop != nil {
		converseReq.InferenceConfig = &InferenceConfig{}
		if req.MaxTokens != 0 && !isHigh {
			converseReq.InferenceConfig.MaxTokens = int(req.MaxTokens)
		}
		if req.Temperature != nil && !isHigh {
			temp := float32(*req.Temperature)
			converseReq.InferenceConfig.Temperature = &temp
		}
		if req.TopP != 0 && !isHigh {
			topP := float32(req.TopP)
			converseReq.InferenceConfig.TopP = &topP
		}
		if req.Stop != nil {
			converseReq.InferenceConfig.StopSequences = parseStopSequences(req.Stop)
		}
	}

	return converseReq, nil
}

// convertOpenAIToolsToConverse converts OpenAI tools format to AWS Converse API ToolConfig
func convertOpenAIToolsToConverse(tools []dto.ToolCallRequest, toolChoice any) *ToolConfig {
	if len(tools) == 0 {
		return nil
	}

	toolConfig := &ToolConfig{
		Tools: make([]Tool, 0, len(tools)),
	}

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}

		// Convert parameters to map
		var paramsMap map[string]any
		if tool.Function.Parameters != nil {
			switch p := tool.Function.Parameters.(type) {
			case map[string]any:
				paramsMap = p
			default:
				// Try to marshal and unmarshal to get map
				data, _ := json.Marshal(tool.Function.Parameters)
				_ = json.Unmarshal(data, &paramsMap)
			}
		}

		toolSpec := &ToolSpec{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}
		if paramsMap != nil {
			toolSpec.InputSchema = &InputSchema{
				JSON: paramsMap,
			}
		}

		toolConfig.Tools = append(toolConfig.Tools, Tool{
			ToolSpec: toolSpec,
		})
	}

	// Handle tool_choice
	if toolChoice != nil {
		toolConfig.ToolChoice = convertToolChoice(toolChoice)
	}

	return toolConfig
}

// convertToolChoice converts OpenAI tool_choice to AWS format
func convertToolChoice(toolChoice any) *ToolChoice {
	if toolChoice == nil {
		return nil
	}

	switch tc := toolChoice.(type) {
	case string:
		switch tc {
		case "auto":
			return &ToolChoice{Auto: &struct{}{}}
		case "required", "any":
			return &ToolChoice{Any: &struct{}{}}
		case "none":
			return nil
		}
	case map[string]any:
		// Handle {"type": "function", "function": {"name": "xxx"}}
		if tcType, ok := tc["type"].(string); ok && tcType == "function" {
			if fn, ok := tc["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					return &ToolChoice{Tool: &ToolName{Name: name}}
				}
			}
		}
	}

	return nil
}

// convertRole converts OpenAI role to Converse API role
func convertRole(role string) string {
	switch role {
	case "assistant":
		return "assistant"
	case "user":
		return "user"
	default:
		return "user"
	}
}

// parseStopSequences parses stop sequences from various formats
func parseStopSequences(stop any) []string {
	if stop == nil {
		return nil
	}

	switch v := stop.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return v
	case []interface{}:
		var sequences []string
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				sequences = append(sequences, str)
			}
		}
		return sequences
	}
	return nil
}

// buildConverseMessages converts ConverseRequest messages to SDK types
func buildConverseMessages(req *ConverseRequest) []types.Message {
	messages := make([]types.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		var content []types.ContentBlock
		for _, c := range msg.Content {
			if c.Text != "" {
				content = append(content, &types.ContentBlockMemberText{Value: c.Text})
			}
			if c.ToolUse != nil {
				content = append(content, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: &c.ToolUse.ToolUseId,
						Name:      &c.ToolUse.Name,
						Input:     document.NewLazyDocument(c.ToolUse.Input),
					},
				})
			}
			if c.ToolResult != nil {
				var resultContent []types.ToolResultContentBlock
				for _, rc := range c.ToolResult.Content {
					if rc.Text != "" {
						resultContent = append(resultContent, &types.ToolResultContentBlockMemberText{Value: rc.Text})
					}
				}
				toolResultBlock := types.ToolResultBlock{
					ToolUseId: &c.ToolResult.ToolUseId,
					Content:   resultContent,
				}
				if c.ToolResult.Status == "error" {
					toolResultBlock.Status = types.ToolResultStatusError
				}
				content = append(content, &types.ContentBlockMemberToolResult{
					Value: toolResultBlock,
				})
			}
		}
		messages = append(messages, types.Message{
			Role:    types.ConversationRole(msg.Role),
			Content: content,
		})
	}
	return messages
}

// buildSystemContent converts system content to SDK types
func buildSystemContent(system []SystemContentBlock) []types.SystemContentBlock {
	if len(system) == 0 {
		return nil
	}
	result := make([]types.SystemContentBlock, 0, len(system))
	for _, s := range system {
		result = append(result, &types.SystemContentBlockMemberText{Value: s.Text})
	}
	return result
}

// buildInferenceConfig converts inference config to SDK types
func buildInferenceConfig(config *InferenceConfig) *types.InferenceConfiguration {
	if config == nil {
		return nil
	}
	result := &types.InferenceConfiguration{}
	if config.MaxTokens > 0 {
		result.MaxTokens = ptrInt32(int32(config.MaxTokens))
	}
	if config.Temperature != nil {
		result.Temperature = config.Temperature
	}
	if config.TopP != nil {
		result.TopP = config.TopP
	}
	if len(config.StopSequences) > 0 {
		result.StopSequences = config.StopSequences
	}
	return result
}

func ptrInt32(v int32) *int32 {
	return &v
}

// buildToolConfig converts ToolConfig to SDK types
func buildToolConfig(config *ToolConfig) *types.ToolConfiguration {
	if config == nil || len(config.Tools) == 0 {
		return nil
	}

	result := &types.ToolConfiguration{
		Tools: make([]types.Tool, 0, len(config.Tools)),
	}

	for _, tool := range config.Tools {
		if tool.ToolSpec == nil {
			continue
		}

		sdkTool := &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        &tool.ToolSpec.Name,
				Description: &tool.ToolSpec.Description,
			},
		}

		if tool.ToolSpec.InputSchema != nil && tool.ToolSpec.InputSchema.JSON != nil {
			sdkTool.Value.InputSchema = &types.ToolInputSchemaMemberJson{
				Value: document.NewLazyDocument(tool.ToolSpec.InputSchema.JSON),
			}
		}

		result.Tools = append(result.Tools, sdkTool)
	}

	// Handle tool choice
	if config.ToolChoice != nil {
		if config.ToolChoice.Auto != nil {
			result.ToolChoice = &types.ToolChoiceMemberAuto{Value: types.AutoToolChoice{}}
		} else if config.ToolChoice.Any != nil {
			result.ToolChoice = &types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}}
		} else if config.ToolChoice.Tool != nil {
			result.ToolChoice = &types.ToolChoiceMemberTool{
				Value: types.SpecificToolChoice{
					Name: &config.ToolChoice.Tool.Name,
				},
			}
		}
	}

	return result
}
