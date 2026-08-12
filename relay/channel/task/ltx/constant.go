package ltx

type LtxTaskRequest struct {
	ImageURI      string `json:"image_uri,omitempty"`
	LastFrameURI  string `json:"last_frame_uri,omitempty"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model"`
	Duration      int    `json:"duration"`
	Resolution    string `json:"resolution"`
	FPS           *int   `json:"fps,omitempty"`
	CameraMotion  string `json:"camera_motion,omitempty"`
	GenerateAudio *bool  `json:"generate_audio,omitempty"`
}

type LtxJobCreatedResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

type LtxJobStatusResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Result      struct {
		VideoURL string `json:"video_url"`
	} `json:"result,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
