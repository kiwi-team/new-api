package ali

var ModelList = []string{
	"qwen-turbo",
	"qwen-plus",
	"qwen-max",
	"qwen-max-longcontext",
	"qwq-32b",
	"qwen3-235b-a22b",
	"text-embedding-v1",
	"gte-rerank-v2",
}

var ChannelName = "ali"

type QwenTTSReqeust struct {
	Model string       `json:"model"`
	Input QwenTTSInput `json:"input"`
}
type QwenTTSInput struct {
	Text         string `json:"text"`
	Voice        string `json:"voice"`
	LanguageType string `json:"language_type,omitempty"`
}

type TTSInstructions struct {
	LanguageType string `json:"language_type,omitempty"`
}

type Qwen3TTSResponse struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Output     struct {
		Text         interface{} `json:"text"`
		FinishReason string      `json:"finish_reason"`
		Choices      interface{} `json:"choices"`
		Audio        struct {
			Data      string `json:"data"`
			URL       string `json:"url"`
			ID        string `json:"id"`
			ExpiresAt int    `json:"expires_at"`
		} `json:"audio"`
	} `json:"output"`
	Usage struct {
		TotalTokens        int `json:"total_tokens"`
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		Characters         int `json:"characters"`
		InputTokensDetails struct {
			TextTokens int `json:"text_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails struct {
			AudioTokens int `json:"audio_tokens"`
			TextTokens  int `json:"text_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}
