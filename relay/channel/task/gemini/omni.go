package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/pkg/errors"
)

// ============================
// Gemini Omni (interactions API) support
//
// The Omni video models (e.g. gemini-omni-flash-preview) use a different API
// surface than the Veo models: submission goes to `/{version}/interactions`
// (background=true) and polling uses `GET /{version}/interactions/{id}`.
// Docs: https://ai.google.dev/gemini-api/docs/omni
// ============================

const (
	omniModelPrefix  = "gemini-omni"
	omniTaskIDPrefix = "omni:"
	// omniDefaultSeconds is the default video length (seconds) billed when the client
	// does not specify `seconds`. Omni generates 10-second videos by default.
	omniDefaultSeconds = 10
)

// ApplyOmniSecondsRatio sets the per-second billing multiplier for an Omni video task.
// It reads the top-level `seconds` field (defaulting to omniDefaultSeconds) and writes it
// into info.PriceData.OtherRatios["seconds"], which the task billing path multiplies into
// the final ratio (quota = modelPrice(per-second) × seconds × groupRatio).
//
// This MUST be called from ValidateRequestAndSetAction (which runs before the task pricing
// step) rather than BuildRequestBody (which runs after pricing).
func ApplyOmniSecondsRatio(info *relaycommon.RelayInfo, seconds string) {
	sec := common.String2Int(seconds)
	if sec <= 0 {
		sec = omniDefaultSeconds
	}
	if info.PriceData.OtherRatios == nil {
		info.PriceData.OtherRatios = map[string]float64{}
	}
	info.PriceData.OtherRatios["seconds"] = float64(sec)
}

// isOmniModel reports whether the model name refers to a Gemini Omni video model.
func isOmniModel(name string) bool {
	return IsOmniModel(name)
}

// IsOmniModel reports whether the model name refers to a Gemini Omni video model.
// Exported for reuse by other providers (e.g. Vertex AI) sharing the interactions API.
func IsOmniModel(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), omniModelPrefix)
}

// encodeOmniTaskID encodes an Omni interaction id into a local task id, tagged so
// downstream code can distinguish it from a Veo operation task id.
func encodeOmniTaskID(id string) string {
	return EncodeOmniTaskID(id)
}

// EncodeOmniTaskID encodes an Omni interaction id into a local task id, tagged so
// downstream code can distinguish it from a Veo operation task id. Exported for reuse.
func EncodeOmniTaskID(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(omniTaskIDPrefix + id))
}

// IsOmniTaskID reports whether the local task id refers to a Gemini Omni interaction.
func IsOmniTaskID(local string) bool {
	b, err := base64.RawURLEncoding.DecodeString(local)
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(b), omniTaskIDPrefix)
}

// decodeOmniInteractionID recovers the upstream interaction id from a local task id.
func decodeOmniInteractionID(local string) (string, error) {
	return DecodeOmniInteractionID(local)
}

// DecodeOmniInteractionID recovers the upstream interaction id from a local task id. Exported for reuse.
func DecodeOmniInteractionID(local string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(local)
	if err != nil {
		return "", err
	}
	s := string(b)
	if !strings.HasPrefix(s, omniTaskIDPrefix) {
		return "", fmt.Errorf("not an omni task id")
	}
	return strings.TrimPrefix(s, omniTaskIDPrefix), nil
}

// ============================
// Request / Response structures
// ============================

// omniPart is a single typed input part for the interactions API.
type omniPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// omniContent is a single output content item inside an interaction step.
type omniContent struct {
	Type     string `json:"type"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
	URI      string `json:"uri"`
}

type omniStep struct {
	Type    string        `json:"type"`
	Content []omniContent `json:"content"`
}

// omniSubmitResponse is the response returned when submitting a background interaction.
type omniSubmitResponse struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// omniInteractionResponse is the polled interaction object.
type omniInteractionResponse struct {
	ID     string     `json:"id"`
	Object string     `json:"object"`
	Status string     `json:"status"`
	Model  string     `json:"model"`
	Steps  []omniStep `json:"steps"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ============================
// Request building
// ============================

// BuildOmniRequestBody converts a task submit request into the Omni interactions payload.
// Gemini-specific parameters are passed through the request `metadata` map.
// It is exported so other providers (e.g. Vertex AI) that share the interactions API
// can reuse the same payload shape.
func BuildOmniRequestBody(req relaycommon.TaskSubmitReq, modelName string) ([]byte, error) {
	reqMap := map[string]any{
		"model":      modelName,
		"background": true,
		"store":      true,
	}

	// support single Image field or the Images slice
	images := req.Images
	if len(images) == 0 && strings.TrimSpace(req.Image) != "" {
		images = []string{req.Image}
	}

	if len(images) > 0 {
		parts := make([]omniPart, 0, len(images)+1)
		for _, img := range images {
			data, mimeType, err := resolveOmniImage(img)
			if err != nil {
				return nil, errors.Wrap(err, "resolve image failed")
			}
			parts = append(parts, omniPart{Type: "image", Data: data, MimeType: mimeType})
		}
		parts = append(parts, omniPart{Type: "text", Text: req.Prompt})
		reqMap["input"] = parts
		reqMap["generation_config"] = map[string]any{
			"video_config": map[string]any{"task": "image_to_video"},
		}
	} else {
		reqMap["input"] = req.Prompt
		reqMap["generation_config"] = map[string]any{
			"video_config": map[string]any{"task": "text_to_video"},
		}
	}

	// Merge gemini-specific params from metadata (metadata wins), but never let it
	// clobber the core fields we control.
	protected := map[string]bool{"input": true, "model": true, "background": true, "store": true}
	for k, v := range req.Metadata {
		if protected[k] {
			continue
		}
		reqMap[k] = v
	}

	return common.Marshal(reqMap)
}

// resolveOmniImage normalizes an image reference (data URI / http(s) url / raw base64)
// into base64 data plus its mime type, as required by the interactions API.
func resolveOmniImage(image string) (data string, mimeType string, err error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", "", fmt.Errorf("empty image")
	}
	if strings.HasPrefix(image, "data:") {
		// data:<mime>;base64,<data>
		idx := strings.Index(image, ",")
		if idx < 0 {
			return "", "", fmt.Errorf("invalid data uri")
		}
		meta := image[len("data:"):idx]
		b64 := image[idx+1:]
		mt := meta
		if semi := strings.Index(meta, ";"); semi >= 0 {
			mt = meta[:semi]
		}
		if strings.TrimSpace(mt) == "" {
			mt = "image/jpeg"
		}
		return b64, mt, nil
	}
	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		mt, b64, err := service.GetImageFromUrl(image)
		if err != nil {
			return "", "", err
		}
		return b64, mt, nil
	}
	// assume raw base64 without a data-uri prefix
	return image, "image/jpeg", nil
}

// ============================
// Response parsing
// ============================

// isOmniResponseBody reports whether a fetched task body is an Omni interaction object.
func isOmniResponseBody(respBody []byte) bool {
	return IsOmniResponseBody(respBody)
}

// IsOmniResponseBody reports whether a fetched task body is an Omni interaction object. Exported for reuse.
func IsOmniResponseBody(respBody []byte) bool {
	var probe struct {
		Object string `json:"object"`
		ID     string `json:"id"`
	}
	if err := common.Unmarshal(respBody, &probe); err != nil {
		return false
	}
	return probe.Object == "interaction"
}

// parseOmniTaskResult maps an Omni interaction response into a generic TaskInfo.
func parseOmniTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	return ParseOmniTaskResult(respBody)
}

// ParseOmniTaskResult maps an Omni interaction response into a generic TaskInfo. Exported for reuse.
func ParseOmniTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op omniInteractionResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal omni response failed: %w", err)
	}

	ti := &relaycommon.TaskInfo{}

	if op.Error != nil && op.Error.Message != "" {
		ti.Status = model.TaskStatusFailure
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}

	switch strings.ToLower(strings.TrimSpace(op.Status)) {
	case "completed", "succeeded", "success":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		taskID := encodeOmniTaskID(op.ID)
		ti.TaskID = taskID

		uri, mimeType, b64 := extractOmniVideoFromSteps(op.Steps)
		if uri != "" {
			ti.RemoteUrl = uri
		}
		// Upload the generated video to S3 and expose the S3 address directly, so the
		// task fetch response returns the S3 url instead of the content-proxy url.
		if s3url := uploadOmniVideoToS3(uri, mimeType, b64); s3url != "" {
			ti.Url = s3url
		} else if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
			ti.Url = uri
		} else {
			// Fallback: no S3 result and no directly usable uri (e.g. inline base64 only
			// or a gs:// uri) — serve via the content proxy.
			ti.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
		}
		return ti, nil
	case "failed", "error", "cancelled", "canceled":
		ti.Status = model.TaskStatusFailure
		ti.Reason = "omni interaction failed"
		ti.Progress = "100%"
		return ti, nil
	default:
		// queued / in_progress / processing / pending
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
		return ti, nil
	}
}

// uploadOmniVideoToS3 uploads a generated Omni video to S3 and returns the S3 url.
// It prefers inline base64 data; otherwise a downloadable http(s) uri. Returns "" on
// failure or when the source is not directly uploadable (e.g. a gs:// uri).
func uploadOmniVideoToS3(uri, mimeType, base64Data string) string {
	if base64Data != "" {
		mime := strings.TrimSpace(mimeType)
		if mime == "" {
			mime = "video/mp4"
		}
		dataURI := "data:" + mime + ";base64," + base64Data
		if file, err := service.SimpleUploadToS3(context.Background(), dataURI); err == nil {
			return file
		}
		return ""
	}
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		if file, err := service.SimpleUploadToS3(context.Background(), uri); err == nil {
			return file
		}
	}
	return ""
}

// ExtractOmniVideo extracts the generated video from an Omni interaction response.
// It returns a file uri (when delivery=uri) and/or inline base64 data with its mime type.
func ExtractOmniVideo(respBody []byte) (uri string, mimeType string, base64Data string) {
	var op omniInteractionResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return "", "", ""
	}
	return extractOmniVideoFromSteps(op.Steps)
}

func extractOmniVideoFromSteps(steps []omniStep) (uri string, mimeType string, base64Data string) {
	for _, step := range steps {
		for _, content := range step.Content {
			if content.Type != "video" {
				continue
			}
			if content.URI != "" {
				return content.URI, content.MimeType, ""
			}
			if content.Data != "" {
				return "", content.MimeType, content.Data
			}
		}
	}
	return "", "", ""
}
