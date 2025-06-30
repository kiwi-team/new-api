package sensenova

type ChatRequest struct {
	Model             string    `json:"model"`
	Messages          []Message `json:"messages"`
	MaxNewTokens      int       `json:"max_new_tokens" default:"1024"`
	RepetitionPenalty float64   `json:"repetition_penalty" default:"1.0"`
	Temperature       float64   `json:"temperature" default:"0.8"`
	TopP              float64   `json:"top_p" default:"0.95"`
	Stream            bool      `json:"stream" default:"false"`
	User              string    `json:"user" default:""`
}

type Message struct {
	Role    string        `json:"role"`
	Content []ContentItem `json:"content"`
}

type ContentItem struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	ImageUrl    string `json:"image_url,omitempty"`
	ImageFileId string `json:"image_file_id,omitempty"`
	ImageBase64 string `json:"image_base64,omitempty"`
	VideoUrl    string `json:"video_url,omitempty"`
	VideoFileId string `json:"video_file_id,omitempty"`
}

type ChatResponse struct {
	Data struct {
		ID    string `json:"id"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Role             string `json:"role"`
			Message          string `json:"message"`
			FinishReason     string `json:"finish_reason"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"choices"`
	} `json:"data"`
}

type StreamResponse struct {
	Data struct {
		ID    string `json:"id"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Role             string `json:"role"`
			Delta            string `json:"delta"`
			ReasoningContent string `json:"reasoning_content"`
			FinishReason     string `json:"finish_reason"`
		} `json:"choices"`
	} `json:"data"`
	Status struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
}
