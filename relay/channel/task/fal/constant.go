package fal

type EditImageTaskRequest struct {
	Prompt          string  `json:"prompt"`
	GuidanceScale   float32 `json:"guidance_scale"`
	Seed            int     `json:"seed"`
	NumImages       int     `json:"num_images"`
	ImageURL        string  `json:"image_url"`
	OutputFormat    string  `json:"output_format"`
	SafetyTolerance string  `json:"safety_tolerance"` // The safety tolerance level for the generated image. 1 being the most strict and 5 being the most permissive. Default value: "2"
	AspectRatio     string  `json:"aspect_ratio"`
}

type EditImageTaskResponse struct {
	Status      string      `json:"status"`
	RequestID   string      `json:"request_id"`
	ResponseURL string      `json:"response_url"`
	StatusURL   string      `json:"status_url"`
	CancelURL   string      `json:"cancel_url"`
	Logs        interface{} `json:"logs"`
	Metrics     struct {
	} `json:"metrics"`
	QueuePosition int `json:"queue_position"`
}

type QueryTaskTatus struct {
	Status      string      `json:"status"`
	RequestID   string      `json:"request_id"`
	ResponseURL interface{} `json:"response_url"`
	StatusURL   interface{} `json:"status_url"`
	CancelURL   interface{} `json:"cancel_url"`
	Logs        interface{} `json:"logs"`
	Metrics     struct {
		InferenceTime float64 `json:"inference_time"`
	} `json:"metrics"`
}

type QueryTaskResponse struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	Images []struct {
		URL         string      `json:"url"`
		ContentType string      `json:"content_type"`
		FileName    interface{} `json:"file_name"`
		FileSize    interface{} `json:"file_size"`
		Width       int         `json:"width"`
		Height      int         `json:"height"`
	} `json:"images,omitempty"`
	Timings struct {
	} `json:"timings,omitzero"`
	Seed            int    `json:"seed,omitempty"`
	HasNsfwConcepts []bool `json:"has_nsfw_concepts,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	Video           struct {
		URL         string `json:"url,omitempty"`
		ContentType string `json:"content_type,omitempty"`
		FileName    string `json:"file_name,omitempty"`
		FileSize    int    `json:"file_size,omitempty"`
	} `json:"video,omitzero"`
}

const (
	COMPLETED_STATUS = "COMPLETED"
	IN_QUEUE_STATUS  = "IN_QUEUE"
)
