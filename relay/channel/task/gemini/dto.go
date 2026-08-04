package gemini

// VeoImageInput represents an image input for Veo image-to-video.
// Used by both Gemini and Vertex adaptors.
type VeoImageInput struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded,omitempty"`
	MimeType           string `json:"mimeType,omitempty"`
	// GcsUri 是 Vertex AI 专有的 gs:// 引用；Gemini API 不接受该形式。
	GcsUri string `json:"gcsUri,omitempty"`
}

// VeoReferenceImage 是 instances[].referenceImages 的一项（Veo 3.1 参考图）。
type VeoReferenceImage struct {
	Image         *VeoImageInput `json:"image"`
	ReferenceType string         `json:"referenceType,omitempty"`
}

// VeoInstance represents a single instance in the Veo predictLongRunning request.
type VeoInstance struct {
	Prompt string         `json:"prompt"`
	Image  *VeoImageInput `json:"image,omitempty"`
	// LastFrame 用于首尾帧插值（Veo 3.1），必须与 Image 搭配使用。
	LastFrame *VeoImageInput `json:"lastFrame,omitempty"`
	// ReferenceImages 是风格/主体参考图（Veo 3.1，最多 3 张），与首尾帧互斥。
	ReferenceImages []VeoReferenceImage `json:"referenceImages,omitempty"`
}

// VeoParameters represents the parameters block for Veo predictLongRunning.
type VeoParameters struct {
	SampleCount        int    `json:"sampleCount"`
	DurationSeconds    int    `json:"durationSeconds,omitempty"`
	AspectRatio        string `json:"aspectRatio,omitempty"`
	Resolution         string `json:"resolution,omitempty"`
	NegativePrompt     string `json:"negativePrompt,omitempty"`
	PersonGeneration   string `json:"personGeneration,omitempty"`
	StorageUri         string `json:"storageUri,omitempty"`
	CompressionQuality string `json:"compressionQuality,omitempty"`
	ResizeMode         string `json:"resizeMode,omitempty"`
	Seed               *int   `json:"seed,omitempty"`
	GenerateAudio      *bool  `json:"generateAudio,omitempty"`
}

// VeoRequestPayload is the top-level request body for the Veo
// predictLongRunning endpoint (used by both Gemini and Vertex).
type VeoRequestPayload struct {
	Instances  []VeoInstance  `json:"instances"`
	Parameters *VeoParameters `json:"parameters,omitempty"`
}

type submitResponse struct {
	Name string `json:"name"`
}

type operationVideo struct {
	MimeType           string `json:"mimeType"`
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	Encoding           string `json:"encoding"`
}

type operationResponse struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response struct {
		Type                  string `json:"@type"`
		RaiMediaFilteredCount int    `json:"raiMediaFilteredCount"`
		// RaiMediaFilteredReasons 是内容安全过滤的具体原因，
		// operation done 但无产物时用它区分“被拦截”与“上游异常”。
		RaiMediaFilteredReasons []string         `json:"raiMediaFilteredReasons"`
		Videos                  []operationVideo `json:"videos"`
		BytesBase64Encoded      string           `json:"bytesBase64Encoded"`
		Encoding                string           `json:"encoding"`
		Video                   string           `json:"video"`
		GenerateVideoResponse   struct {
			// Gemini API 返回 generatedVideos，Vertex AI 返回 generatedSamples，
			// 两个字段结构相同，都要解，取到哪个用哪个。
			GeneratedVideos  []generatedVideo `json:"generatedVideos"`
			GeneratedSamples []generatedVideo `json:"generatedSamples"`
		} `json:"generateVideoResponse"`
	} `json:"response"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// generatedVideo 是 Veo 产物条目，Gemini/Vertex 两种字段名共用同一结构。
type generatedVideo struct {
	Video struct {
		URI string `json:"uri"`
	} `json:"video"`
}
