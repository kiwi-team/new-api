package novita

const (
	TASK_STATUS_QUEUED     = "TASK_STATUS_QUEUED"
	TASK_STATUS_SUCCEED    = "TASK_STATUS_SUCCEED"
	TASK_STATUS_FAILED     = "TASK_STATUS_FAILED"
	TASK_STATUS_PROCESSING = "TASK_STATUS_PROCESSING"
)

type NovitaTaskSubmitRequest struct {
	// 提示词
	Prompt          string   `json:"prompt"`
	Model           string   `json:"model"`
	Size            string   `json:"size,omitempty"`
	Seed            int      `json:"seed,omitempty"`
	Images          []string `json:"images,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"` // 1:1 4:3等
	SafetyTolerance string   `json:"safety_tolerance,omitempty"`
}

type NovitaTaskSubmitResponse struct {
	TaskID string `json:"task_id"`
}

type NovitaTaskResult struct {
	Extra struct {
		Seed      string `json:"seed"`
		DebugInfo struct {
			RequestInfo    string `json:"request_info"`
			SubmitTimeMs   string `json:"submit_time_ms"`
			ExecuteTimeMs  string `json:"execute_time_ms"`
			CompleteTimeMs string `json:"complete_time_ms"`
		} `json:"debug_info,omitempty"`
	} `json:"extra"`
	Task struct {
		TaskID          string `json:"task_id"`
		Status          string `json:"status"`
		Reason          string `json:"reason"`
		TaskType        string `json:"task_type"`
		Eta             int    `json:"eta"`
		ProgressPercent int    `json:"progress_percent"`
	} `json:"task"`
	Images []struct {
		ImageURL    string `json:"image_url"`
		ImageURLTTL string `json:"image_url_ttl"`
		ImageType   string `json:"image_type"`
	} `json:"images"`
	Videos []struct {
		VideoURL    string `json:"video_url"`
		VideoURLTTL string `json:"video_url_ttl"`
		VideoType   string `json:"video_type"`
	} `json:"videos,omitempty"`
	Audios []struct {
		AudioURL      string `json:"audio_url"`
		AudioURLTTL   string `json:"audio_url_ttl"`
		AudioType     string `json:"audio_type"`
		AudioMetadata struct {
			Text      string `json:"text"`
			StartTime int    `json:"start_time"`
			EndTime   int    `json:"end_time"`
		} `json:"audio_metadata"`
	} `json:"audios,omitempty"`
}
