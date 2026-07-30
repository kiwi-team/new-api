package ali_dashscope

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
)

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type Input struct {
	//Prompt   string       `json:"prompt"`
	Messages []Message `json:"messages"`
}

type Parameters struct {
	TopP                  *float64              `json:"top_p,omitempty"`
	TopK                  int                   `json:"top_k,omitempty"`
	Seed                  uint64                `json:"seed,omitempty"`
	EnableSearch          bool                  `json:"enable_search,omitempty"`
	IncrementalOutput     bool                  `json:"incremental_output,omitempty"`
	MaxTokens             int                   `json:"max_tokens,omitempty"`
	Temperature           *float64              `json:"temperature,omitempty"`
	ResultFormat          string                `json:"result_format,omitempty"`
	EnableCodeInterpreter bool                  `json:"enable_code_interpreter,omitempty"`
	EnableThinking        bool                  `json:"enable_thinking,omitempty"`
	Tools                 []dto.ToolCallRequest `json:"tools,omitempty"`
}

type ChatRequest struct {
	Model      string     `json:"model"`
	Input      Input      `json:"input"`
	Parameters Parameters `json:"parameters,omitempty"`
}

type ImageRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt         string `json:"prompt"`
		NegativePrompt string `json:"negative_prompt,omitempty"`
	} `json:"input"`
	Parameters struct {
		Size  string `json:"size,omitempty"`
		N     int    `json:"n,omitempty"`
		Steps string `json:"steps,omitempty"`
		Scale string `json:"scale,omitempty"`
	} `json:"parameters,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// Wan2.7 image generation/editing request (synchronous, messages-based)
type Wan27ImageRequest struct {
	Model      string               `json:"model"`
	Input      Wan27ImageInput      `json:"input"`
	Parameters Wan27ImageParameters `json:"parameters,omitempty"`
}

type Wan27ImageInput struct {
	Messages []Wan27Message `json:"messages"`
}

type Wan27Message struct {
	Role    string         `json:"role"`
	Content []Wan27Content `json:"content"`
}

type Wan27Content struct {
	Text  string `json:"text,omitempty"`
	Image string `json:"image,omitempty"`
}

type Wan27ImageParameters struct {
	N                int             `json:"n,omitempty"`
	Size             string          `json:"size,omitempty"`
	Watermark        *bool           `json:"watermark,omitempty"`
	Seed             int             `json:"seed,omitempty"`
	ThinkingMode     *bool           `json:"thinking_mode,omitempty"`
	EnableSequential *bool           `json:"enable_sequential,omitempty"`
	BboxList         json.RawMessage `json:"bbox_list,omitempty"`
	ColorPalette     json.RawMessage `json:"color_palette,omitempty"`
}

// Wan2.7 synchronous image response
type Wan27ImageResponse struct {
	Output struct {
		Choices []Wan27Choice `json:"choices"`
	} `json:"output"`
	Usage struct {
		ImageCount int    `json:"image_count"`
		Size       string `json:"size"`
	} `json:"usage"`
	RequestId string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type Wan27Choice struct {
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role    string         `json:"role"`
		Content []Wan27Content `json:"content"`
	} `json:"message"`
}

type TaskResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestId  string `json:"request_id,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Output     struct {
		TaskId     string `json:"task_id,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Results    []struct {
			B64Image string `json:"b64_image,omitempty"`
			Url      string `json:"url,omitempty"`
			Code     string `json:"code,omitempty"`
			Message  string `json:"message,omitempty"`
		} `json:"results,omitempty"`
		TaskMetrics struct {
			Total     int `json:"TOTAL,omitempty"`
			Succeeded int `json:"SUCCEEDED,omitempty"`
			Failed    int `json:"FAILED,omitempty"`
		} `json:"task_metrics,omitempty"`
	} `json:"output,omitempty"`
	Usage Usage `json:"usage"`
}

type Header struct {
	Action       string `json:"action,omitempty"`
	Streaming    string `json:"streaming,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	Event        string `json:"event,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Attributes   any    `json:"attributes,omitempty"`
}

type Payload struct {
	Model      string `json:"model,omitempty"`
	Task       string `json:"task,omitempty"`
	TaskGroup  string `json:"task_group,omitempty"`
	Function   string `json:"function,omitempty"`
	Parameters struct {
		SampleRate int     `json:"sample_rate,omitempty"`
		Rate       float64 `json:"rate,omitempty"`
		Format     string  `json:"format,omitempty"`
	} `json:"parameters,omitempty"`
	Input struct {
		Text string `json:"text,omitempty"`
	} `json:"input,omitempty"`
	Usage struct {
		Characters int `json:"characters,omitempty"`
	} `json:"usage,omitempty"`
}

type WSSMessage struct {
	Header  Header  `json:"header,omitempty"`
	Payload Payload `json:"payload,omitempty"`
}

type EmbeddingRequest struct {
	Model string `json:"model"`
	Input struct {
		Texts []string `json:"texts"`
	} `json:"input"`
	Parameters *struct {
		TextType string `json:"text_type,omitempty"`
	} `json:"parameters,omitempty"`
}

type Embedding struct {
	Embedding []float64 `json:"embedding"`
	TextIndex int       `json:"text_index"`
}

type EmbeddingResponse struct {
	Output struct {
		Embeddings []Embedding `json:"embeddings"`
	} `json:"output"`
	Usage Usage `json:"usage"`
	Error
}

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
}

// DeepResearch specific types

type DeepResearchParameters struct {
	Stream            bool     `json:"stream"`
	IncrementalOutput bool     `json:"incremental_output"`
	EnableFeedback    bool     `json:"enable_feedback"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
}

type DeepResearchChatRequest struct {
	Model      string                 `json:"model"`
	Input      Input                  `json:"input"`
	Parameters DeepResearchParameters `json:"parameters"`
}

type DeepResearchMessage struct {
	Phase   string          `json:"phase"`
	Status  string          `json:"status"`
	Content string          `json:"content"`
	Extra   json.RawMessage `json:"extra,omitempty"`
}

type DeepResearchOutput struct {
	Message DeepResearchMessage `json:"message"`
}

type DeepResearchChatResponse struct {
	Output    DeepResearchOutput `json:"output"`
	Usage     Usage              `json:"usage"`
	RequestId string             `json:"request_id"`
	Error
}

type Usage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	TotalTokens         int `json:"total_tokens"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type Chioce struct {
	Message struct {
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		Role             string `json:"role"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type Output struct {
	//Text         string                      `json:"text"`
	//FinishReason string                   `json:"finish_reason"`
	Choices  []dto.OpenAITextResponseChoice `json:"choices"`
	ToolInfo json.RawMessage                `json:"tool_info,omitempty"`
}

type ChatResponse struct {
	Output Output `json:"output"`
	Usage  Usage  `json:"usage"`
	Error
}
