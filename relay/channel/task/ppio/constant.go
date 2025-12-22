package ppio

const (
	TASK_STATUS_QUEUED     = "TASK_STATUS_QUEUED"
	TASK_STATUS_SUCCEED    = "TASK_STATUS_SUCCEED"
	TASK_STATUS_FAILED     = "TASK_STATUS_FAILED"
	TASK_STATUS_PROCESSING = "TASK_STATUS_PROCESSING"
)

type PPIOTaskSubmitRequest struct {
	// 提示词
	Prompt          string   `json:"prompt"`
	Model           string   `json:"model"`
	Size            string   `json:"size,omitempty"`
	Seed            int      `json:"seed,omitempty"`
	ImageURL        string   `json:"image_url,omitempty"`
	Images          []string `json:"images,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"` // 1:1 4:3等
	SafetyTolerance string   `json:"safety_tolerance,omitempty"`
}

// https://ppio.com/docs/models/reference-vidu-2.0-img2video
type ViduTaskSubmitRequest struct {
	Images            []string `json:"images,omitempty"`
	Prompt            string   `json:"prompt"`
	Duration          int      `json:"duration,omitempty"`
	Seed              int      `json:"seed,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	MovementAmplitude string   `json:"movement_amplitude,omitempty"`
	BGM               bool     `json:"bgm,omitempty"`
}

// https://ppio.com/docs/models/reference-wan2.6-t2v
type WanTaskSubmitRequest struct {
	Input      *TaskInput      `json:"input"`
	Parameters *TaskParameters `json:"parameters,omitempty"`
}

type TaskInput struct {
	Prompt         string `json:"prompt"`
	AudioURL       string `json:"audio_url,omitempty"`
	ImageURL       string `json:"img_url,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
}
type TaskParameters struct {
	Seed         int    `json:"seed,omitempty"`
	Size         string `json:"size,omitempty"`  //1280*720, 720*1280, 960*960, 1088*832, 832*1088, 1920*1080, 1080*1920, 1440*1440, 1632*1248, 1248*1632
	Audio        bool   `json:"audio,omitempty"` //是否添加音频。参数优先级：audio_url > audio，仅在audio_url为空时生效。true：默认值，自动为视频添加音频；false：不添加音频，输出无声视频。
	Duration     int    `json:"duration,omitempty"`
	ShotType     string `json:"shot_type,omitempty"`
	Watermark    bool   `json:"watermark,omitempty"`
	PromptExtend bool   `json:"prompt_extend,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
}

type PPIOTaskSubmitResponse struct {
	TaskID string `json:"task_id"`
}

type PPIOTaskResult struct {
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
