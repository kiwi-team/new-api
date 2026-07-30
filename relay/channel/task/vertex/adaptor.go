package vertex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	geminitask "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	vertexcore "github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Instances  []map[string]any `json:"instances"`
	Parameters map[string]any   `json:"parameters,omitempty"`
}
type fetchOperationPayload struct {
	OperationName string `json:"operationName"`
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
	} `json:"response"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
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
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return "", fmt.Errorf("failed to decode credentials: %w", err)
	}
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = "veo-3.0-generate-001"
	}

	region := vertexcore.GetModelRegion(info.ApiVersion, modelName)
	if strings.TrimSpace(region) == "" {
		region = "global"
	}

	// Omni models use the interactions API instead of predictLongRunning.
	if geminitask.IsOmniModel(modelName) {
		if region == "global" {
			return fmt.Sprintf(
				"https://aiplatform.googleapis.com/v1beta1/projects/%s/locations/global/interactions",
				adc.ProjectID,
			), nil
		}
		return fmt.Sprintf(
			"https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/interactions",
			region,
			adc.ProjectID,
			region,
		), nil
	}
	return vertexcore.BuildGoogleModelURL(a.baseURL, vertexcore.DefaultAPIVersion, adc.ProjectID, region, modelName, "predictLongRunning"), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return fmt.Errorf("failed to decode credentials: %w", err)
	}

	proxy := ""
	if info != nil {
		proxy = info.ChannelSetting.Proxy
	}
	token, err := vertexcore.AcquireAccessToken(*adc, proxy)
	if err != nil {
		return fmt.Errorf("failed to acquire access token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	return nil
}

// EstimateBilling returns OtherRatios based on durationSeconds and resolution.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	v, ok := c.Get("task_request")
	if !ok {
		return nil
	}
	req := v.(relaycommon.TaskSubmitReq)

	// Omni models bill per second of generated video and have no resolution tier.
	if geminitask.IsOmniModel(info.OriginModelName) {
		return geminitask.OmniSecondsRatio(req.Seconds)
	}

	seconds := geminitask.ResolveVeoDuration(req.Metadata, req.Duration, req.Seconds)
	resolution := geminitask.ResolveVeoResolution(req.Metadata, req.Size)
	resRatio := geminitask.VeoResolutionRatio(info.UpstreamModelName, resolution)

	return map[string]float64{
		"seconds":    float64(seconds),
		"resolution": resRatio,
	}
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// Omni models use the interactions API with a different payload shape.
	if geminitask.IsOmniModel(info.OriginModelName) {
		data, err := geminitask.BuildOmniRequestBody(req, info.OriginModelName)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	instance := geminitask.VeoInstance{Prompt: req.Prompt}
	if img := geminitask.ExtractMultipartImage(c, info); img != nil {
		instance.Image = img
	} else if len(req.Images) > 0 {
		if parsed := geminitask.ParseImageInput(req.Images[0]); parsed != nil {
			instance.Image = parsed
			info.Action = constant.TaskActionGenerate
		}
	}

	params := &geminitask.VeoParameters{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, params); err != nil {
		return nil, fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	if params.DurationSeconds == 0 && req.Duration > 0 {
		params.DurationSeconds = req.Duration
	}
	if params.Resolution == "" && req.Size != "" {
		params.Resolution = geminitask.SizeToVeoResolution(req.Size)
	}
	if params.AspectRatio == "" && req.Size != "" {
		params.AspectRatio = geminitask.SizeToVeoAspectRatio(req.Size)
	}
	params.Resolution = strings.ToLower(params.Resolution)
	params.SampleCount = 1

	body := geminitask.VeoRequestPayload{
		Instances:  []geminitask.VeoInstance{instance},
		Parameters: params,
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// Omni interactions API returns an interaction object with an `id`.
	if geminitask.IsOmniModel(info.OriginModelName) {
		var os struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &os); err != nil {
			return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		}
		if os.Error != nil && os.Error.Message != "" {
			return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", os.Error.Message), "upstream_error", http.StatusBadRequest)
		}
		if strings.TrimSpace(os.ID) == "" {
			return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing interaction id"), "invalid_response", http.StatusInternalServerError)
		}
		adc := &vertexcore.Credentials{}
		if err := json.Unmarshal([]byte(a.apiKey), adc); err != nil {
			return "", nil, service.TaskErrorWrapper(err, "decode_credentials_failed", http.StatusInternalServerError)
		}
		modelName := info.OriginModelName
		region := vertexcore.GetModelRegion(info.ApiVersion, modelName)
		if strings.TrimSpace(region) == "" {
			region = "global"
		}
		localID := encodeOmniTaskID(region, adc.ProjectID, os.ID)
		ov := dto.NewOpenAIVideo()
		ov.ID = localID
		ov.TaskID = localID
		ov.Status = dto.VideoStatusQueued
		ov.Model = modelName
		c.JSON(http.StatusOK, ov)
		return localID, responseBody, nil
	}

	var s submitResponse
	if err := common.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	localID := taskcommon.EncodeLocalTaskID(s.Name)
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"veo-3.0-generate-001",
		"veo-3.0-fast-generate-001",
		"veo-3.1-generate-001",
		"veo-3.1-fast-generate-001",
		"veo-3.1-generate-preview",
		"veo-3.1-fast-generate-preview",
		"gemini-omni-flash-preview",
	}
}
func (a *TaskAdaptor) GetChannelName() string { return "vertex" }

func buildFetchOperationURL(baseURL, upstreamName string) (string, error) {
	region := extractRegionFromOperationName(upstreamName)
	if region == "" {
		region = "us-central1"
	}
	project := extractProjectFromOperationName(upstreamName)
	modelName := extractModelFromOperationName(upstreamName)
	if strings.TrimSpace(modelName) == "" {
		return "", fmt.Errorf("cannot extract model from operation name")
	}
	if strings.TrimSpace(project) == "" {
		return "", fmt.Errorf("cannot extract project from operation name")
	}
	return vertexcore.BuildGoogleModelURL(baseURL, vertexcore.DefaultAPIVersion, project, region, modelName, "fetchPredictOperation"), nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// Omni interactions API: GET the interaction by id.
	if geminitask.IsOmniTaskID(taskID) {
		region, project, interactionID, derr := decodeOmniTaskID(taskID)
		if derr != nil {
			return nil, fmt.Errorf("decode omni task_id failed: %w", derr)
		}
		if region == "" {
			region = "global"
		}
		var url string
		if region == "global" {
			url = fmt.Sprintf("https://aiplatform.googleapis.com/v1beta1/projects/%s/locations/global/interactions/%s", project, interactionID)
		} else {
			url = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/interactions/%s", region, project, region, interactionID)
		}
		adc := &vertexcore.Credentials{}
		if err := json.Unmarshal([]byte(key), adc); err != nil {
			return nil, fmt.Errorf("failed to decode credentials: %w", err)
		}
		token, err := vertexcore.AcquireAccessToken(*adc, proxy)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire access token: %w", err)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("x-goog-user-project", adc.ProjectID)
		client, err := service.GetHttpClientWithProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
		return client.Do(req)
	}

	upstreamName, err := taskcommon.DecodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}
	url, err := buildFetchOperationURL(baseUrl, upstreamName)
	if err != nil {
		return nil, err
	}
	payload := fetchOperationPayload{OperationName: upstreamName}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(key), adc); err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %w", err)
	}
	token, err := vertexcore.AcquireAccessToken(*adc, proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire access token: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// Omni interactions API has a different response shape. ParseOmniTaskResult already
	// uploads the generated video to S3 and sets ti.Url to the S3 address on success.
	if geminitask.IsOmniResponseBody(respBody) {
		return geminitask.ParseOmniTaskResult(respBody)
	}
	var op operationResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
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
	if len(op.Response.Videos) > 0 {
		v0 := op.Response.Videos[0]
		if v0.BytesBase64Encoded != "" {
			mime := strings.TrimSpace(v0.MimeType)
			if mime == "" {
				enc := strings.TrimSpace(v0.Encoding)
				if enc == "" {
					enc = "mp4"
				}
				if strings.Contains(enc, "/") {
					mime = enc
				} else {
					mime = "video/" + enc
				}
			}
			ti.Url = "data:" + mime + ";base64," + v0.BytesBase64Encoded
			file, err := service.SimpleUploadToS3(context.Background(), ti.Url)
			if err == nil {
				ti.Url = file
			}
			return ti, nil
		}
	}
	if op.Response.BytesBase64Encoded != "" {
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		ti.Url = "data:" + mime + ";base64," + op.Response.BytesBase64Encoded
		file, err := service.SimpleUploadToS3(context.Background(), ti.Url)
		if err == nil {
			ti.Url = file
		}
		return ti, nil
	}
	if op.Response.Video != "" { // some variants use `video` as base64
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		ti.Url = "data:" + mime + ";base64," + op.Response.Video
		if file, err := service.SimpleUploadToS3(context.Background(), ti.Url); err == nil {
			ti.Url = file
		}
		return ti, nil
	}
	return ti, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	modelName := ""
	if geminitask.IsOmniTaskID(task.TaskID) {
		modelName = task.Properties.OriginModelName
		if strings.TrimSpace(modelName) == "" {
			modelName = "gemini-omni-flash-preview"
		}
	} else {
		// Use GetUpstreamTaskID() to get the real upstream operation name for model extraction.
		// task.TaskID is now a public task_xxxx ID, no longer a base64-encoded upstream name.
		upstreamTaskID := task.GetUpstreamTaskID()
		upstreamName, err := taskcommon.DecodeLocalTaskID(upstreamTaskID)
		if err != nil {
			upstreamName = ""
		}
		modelName := extractModelFromOperationName(upstreamName)
		if strings.TrimSpace(modelName) == "" {
			modelName = "veo-3.0-generate-001"
		}
	}
	v := dto.NewOpenAIVideo()
	v.ID = task.TaskID
	v.Model = modelName
	v.Status = task.Status.ToVideoStatus()
	v.SetProgressStr(task.Progress)
	v.CreatedAt = task.CreatedAt
	v.CompletedAt = task.UpdatedAt
	if resultURL := task.GetResultURL(); strings.HasPrefix(resultURL, "data:") && len(resultURL) > 0 {
		v.SetMetadata("url", resultURL)
	}
	// 失败时把失败原因带上，便于级联下游（如 OpenAI 类型渠道）取到真实错误信息。
	if task.Status == model.TaskStatusFailure && strings.TrimSpace(task.FailReason) != "" {
		v.Error = &dto.OpenAIVideoError{Message: task.FailReason}
	}

	return common.Marshal(v)
}

// ============================
// helpers
// ============================

func encodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// encodeOmniTaskID packs region/project/interactionID into an omni-tagged local task id
// so FetchTask can rebuild the Vertex interactions polling URL. The decoded payload keeps
// the shared "omni:" prefix so gemini.IsOmniTaskID recognizes it.
func encodeOmniTaskID(region, project, interactionID string) string {
	raw := fmt.Sprintf("omni:%s|%s|%s", region, project, interactionID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeOmniTaskID recovers region/project/interactionID from a local task id.
func decodeOmniTaskID(local string) (region, project, interactionID string, err error) {
	b, derr := base64.RawURLEncoding.DecodeString(local)
	if derr != nil {
		return "", "", "", derr
	}
	s := string(b)
	if !strings.HasPrefix(s, "omni:") {
		return "", "", "", fmt.Errorf("not an omni task id")
	}
	parts := strings.SplitN(strings.TrimPrefix(s, "omni:"), "|", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid omni task id payload")
	}
	return parts[0], parts[1], parts[2], nil
}

func decodeLocalTaskID(local string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(local)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var regionRe = regexp.MustCompile(`locations/([a-z0-9-]+)/`)

func extractRegionFromOperationName(name string) string {
	m := regionRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

func extractModelFromOperationName(name string) string {
	m := modelRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	idx := strings.Index(name, "models/")
	if idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}

var projectRe = regexp.MustCompile(`projects/([^/]+)/locations/`)

func extractProjectFromOperationName(name string) string {
	m := projectRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
