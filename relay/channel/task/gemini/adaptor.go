package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

// GeminiVideoGenerationConfig represents the video generation configuration
// Based on: https://ai.google.dev/gemini-api/docs/video
type GeminiVideoGenerationConfig struct {
	AspectRatio      string  `json:"aspectRatio,omitempty"`      // "16:9" or "9:16"
	DurationSeconds  float64 `json:"durationSeconds,omitempty"`  // 4, 6, or 8 (as number)
	NegativePrompt   string  `json:"negativePrompt,omitempty"`   // unwanted elements
	PersonGeneration string  `json:"personGeneration,omitempty"` // "allow_all" for text-to-video, "allow_adult" for image-to-video
	Resolution       string  `json:"resolution,omitempty"`       // video resolution
}

// GeminiVideoRequest represents a single video generation instance
type GeminiVideoRequest struct {
	Prompt string `json:"prompt"`
}

// GeminiVideoPayload represents the complete video generation request payload
type GeminiVideoPayload struct {
	Instances  []GeminiVideoRequest        `json:"instances"`
	Parameters GeminiVideoGenerationConfig `json:"parameters,omitempty"`
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
		Type                  string           `json:"@type"`
		RaiMediaFilteredCount int              `json:"raiMediaFilteredCount"`
		Videos                []operationVideo `json:"videos"`
		BytesBase64Encoded    string           `json:"bytesBase64Encoded"`
		Encoding              string           `json:"encoding"`
		Video                 string           `json:"video"`
		GenerateVideoResponse struct {
			GeneratedSamples []struct {
				Video struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generatedSamples"`
		} `json:"generateVideoResponse"`
	} `json:"response"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Omni and Veo models bill per second of generated video. Set the seconds ratio here
	// (before the task pricing step) so the generated duration participates in quota calculation.
	if isOmniModel(info.OriginModelName) || isVeoModel(info.OriginModelName) {
		var probe relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &probe); err != nil {
			return service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		}
		if isOmniModel(info.OriginModelName) {
			ApplyOmniSecondsRatio(info, probe.Seconds)
		} else {
			ApplyVeoSecondsRatio(info, probe.Seconds, probe.Duration)
		}
	}
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	modelName := info.OriginModelName
	version := model_setting.GetGeminiVersionSetting(modelName)

	// Omni models use the interactions endpoint instead of predictLongRunning.
	if isOmniModel(modelName) {
		return fmt.Sprintf("%s/%s/interactions", a.baseURL, version), nil
	}

	return fmt.Sprintf(
		"%s/%s/models/%s:predictLongRunning",
		a.baseURL,
		version,
		modelName,
	), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", a.apiKey)
	return nil
}

// BuildRequestBody converts request into Gemini specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("unexpected task_request type")
	}

	// Omni models use the interactions API with a different payload shape.
	if isOmniModel(info.OriginModelName) {
		data, err := BuildOmniRequestBody(c, req, info.OriginModelName)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	// Parameters 仍走既有的结构化字段 + metadata 合并。
	params := GeminiVideoGenerationConfig{}
	metadata := req.Metadata
	medaBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "metadata marshal metadata failed")
	}
	if err = json.Unmarshal(medaBytes, &params); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if req.NegativePrompt != "" && params.NegativePrompt == "" {
		params.NegativePrompt = req.NegativePrompt
	}
	if req.AspectRatio != "" && params.AspectRatio == "" {
		params.AspectRatio = req.AspectRatio
	}
	if req.Resolution != "" && params.Resolution == "" {
		params.Resolution = req.Resolution
	}

	// instance 用 map 承载，便于挂载 referenceImages / image / lastFrame 等
	// 结构化字段（注意这些字段在 instances 内，不在 parameters 内）。
	instance := map[string]any{"prompt": req.Prompt}

	if err := applyVeoInstanceReferences(c, &req, instance, &params); err != nil {
		return nil, err
	}

	body := map[string]any{
		"instances":  []map[string]any{instance},
		"parameters": params,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// applyVeoInstanceReferences 把统一 references 映射到 Veo 的 instance 字段：
// reference_image -> referenceImages（并施加 8s / allow_adult 硬约束）
// first_frame     -> image
// last_frame      -> lastFrame（官方要求必须与 image 搭配使用）
func applyVeoInstanceReferences(c *gin.Context, req *relaycommon.TaskSubmitReq,
	instance map[string]any, params *GeminiVideoGenerationConfig) error {

	// Gemini API 的 image 用 inlineData 包裹（Vertex 用 bytesBase64Encoded）。
	refImages, err := BuildVeoReferenceImages(c, req, false)
	if err != nil {
		return err
	}
	if len(refImages) > 0 {
		instance["referenceImages"] = refImages
		// 硬约束需要读写 parameters.personGeneration，这里用一个临时 map 承接，
		// 再回写到结构化的 params 上。
		pmap := map[string]any{}
		if params.PersonGeneration != "" {
			pmap["personGeneration"] = params.PersonGeneration
		}
		if err := ApplyVeoReferenceConstraints(instance, pmap, req); err != nil {
			return err
		}
		if pg, ok := pmap["personGeneration"].(string); ok {
			params.PersonGeneration = pg
		}
		params.DurationSeconds = VeoReferenceDurationSeconds
	}

	// 首帧 / 尾帧（与 referenceImages 是相互独立的机制）
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleFirstFrame); ok && ref.URL != "" {
		img, err := BuildVeoImageObject(c, ref.URL)
		if err != nil {
			return fmt.Errorf("resolve first_frame failed: %w", err)
		}
		if img != nil {
			instance["image"] = img
		}
	}
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleLastFrame); ok && ref.URL != "" {
		if _, hasFirst := instance["image"]; !hasFirst {
			return fmt.Errorf("last_frame must be used together with a first_frame image")
		}
		img, err := BuildVeoImageObject(c, ref.URL)
		if err != nil {
			return fmt.Errorf("resolve last_frame failed: %w", err)
		}
		if img != nil {
			instance["lastFrame"] = img
		}
	}
	return nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// Omni interactions API returns an interaction object with an `id`.
	if isOmniModel(info.OriginModelName) {
		var os omniSubmitResponse
		if err := json.Unmarshal(responseBody, &os); err != nil {
			return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		}
		if os.Error != nil && os.Error.Message != "" {
			return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", os.Error.Message), "upstream_error", http.StatusBadRequest)
		}
		if strings.TrimSpace(os.ID) == "" {
			return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing interaction id"), "invalid_response", http.StatusInternalServerError)
		}
		taskID = encodeOmniTaskID(os.ID)
		ov := dto.NewOpenAIVideo()
		ov.ID = taskID
		ov.TaskID = taskID
		ov.Status = dto.VideoStatusQueued
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return taskID, responseBody, nil
	}

	var s submitResponse
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	taskID = encodeLocalTaskID(s.Name)
	ov := dto.NewOpenAIVideo()
	ov.ID = taskID
	ov.TaskID = taskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return taskID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"veo-3.0-generate-001", "veo-3.1-generate-preview", "veo-3.1-fast-generate-preview", "gemini-omni-flash-preview"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "gemini"
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// For Gemini API, we use GET request to the operations/interactions endpoint
	version := model_setting.GetGeminiVersionSetting("default")

	var url string
	if IsOmniTaskID(taskID) {
		interactionID, err := decodeOmniInteractionID(taskID)
		if err != nil {
			return nil, fmt.Errorf("decode omni task_id failed: %w", err)
		}
		url = fmt.Sprintf("%s/%s/interactions/%s", baseUrl, version, interactionID)
	} else {
		upstreamName, err := decodeLocalTaskID(taskID)
		if err != nil {
			return nil, fmt.Errorf("decode task_id failed: %w", err)
		}
		url = fmt.Sprintf("%s/%s/%s", baseUrl, version, upstreamName)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// Omni interactions API has a different response shape.
	if isOmniResponseBody(respBody) {
		return parseOmniTaskResult(respBody)
	}

	var op operationResponse
	if err := json.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}

	ti := &relaycommon.TaskInfo{}

	if op.Error.Message != "" {
		ti.Status = model.TaskStatusFailure
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}

	if !op.Done {
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
		return ti, nil
	}

	ti.Status = model.TaskStatusSuccess
	ti.Progress = "100%"

	taskID := encodeLocalTaskID(op.Name)
	ti.TaskID = taskID
	ti.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)

	// Extract URL from generateVideoResponse if available
	if len(op.Response.GenerateVideoResponse.GeneratedSamples) > 0 {
		if uri := op.Response.GenerateVideoResponse.GeneratedSamples[0].Video.URI; uri != "" {
			ti.RemoteUrl = uri
		}
	}

	return ti, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	modelName := ""
	if IsOmniTaskID(task.TaskID) {
		modelName = task.Properties.OriginModelName
		if strings.TrimSpace(modelName) == "" {
			modelName = "gemini-omni-flash-preview"
		}
	} else {
		upstreamName, err := decodeLocalTaskID(task.TaskID)
		if err != nil {
			upstreamName = ""
		}
		modelName = extractModelFromOperationName(upstreamName)
		if strings.TrimSpace(modelName) == "" {
			modelName = "veo-3.0-generate-001"
		}
	}

	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.Model = modelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	if task.FinishTime > 0 {
		video.CompletedAt = task.FinishTime
	} else if task.UpdatedAt > 0 {
		video.CompletedAt = task.UpdatedAt
	}
	// 失败时把失败原因带上，便于级联下游（如 OpenAI 类型渠道）取到真实错误信息。
	if task.Status == model.TaskStatusFailure && strings.TrimSpace(task.FailReason) != "" {
		video.Error = &dto.OpenAIVideoError{Message: task.FailReason}
	}

	return common.Marshal(video)
}

// ============================
// helpers
// ============================

func encodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

func decodeLocalTaskID(local string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(local)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

func extractModelFromOperationName(name string) string {
	if name == "" {
		return ""
	}
	if m := modelRe.FindStringSubmatch(name); len(m) == 2 {
		return m[1]
	}
	if idx := strings.Index(name, "models/"); idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}
