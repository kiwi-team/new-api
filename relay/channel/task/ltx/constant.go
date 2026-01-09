package ltx

type LtxTaskRequest struct {
	ImageURI      string `json:"image_uri,omitempty"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model"`
	Duration      int    `json:"duration"`
	Resolution    string `json:"resolution,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	CameraMotion  string `json:"camera_motion,omitempty"`
	GenerateAudio bool   `json:"generate_audio,omitempty"`
}

// mock response
type LtxTaskResponse struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	VideoURL   string `json:"video_url,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
}
