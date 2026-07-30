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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskgemini "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	vertexcore "github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Instances  []map[string]any `json:"instances"`
	Parameters map[string]any   `json:"parameters,omitempty"`
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
		Type                    string           `json:"@type"`
		RaiMediaFilteredCount   int              `json:"raiMediaFilteredCount"`
		RaiMediaFilteredReasons []string         `json:"raiMediaFilteredReasons"`
		Videos                  []operationVideo `json:"videos"`
		BytesBase64Encoded      string           `json:"bytesBase64Encoded"`
		Encoding                string           `json:"encoding"`
		Video                   string           `json:"video"`
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
	// Omni models bill per second of generated video. Set the seconds ratio here (before
	// the task pricing step) so the generated duration participates in quota calculation.
	if taskgemini.IsOmniModel(info.OriginModelName) {
		var probe relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &probe); err != nil {
			return service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		}
		taskgemini.ApplyOmniSecondsRatio(info, probe.Seconds)
	}
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return "", fmt.Errorf("failed to decode credentials: %w", err)
	}
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = "veo-3.0-generate-001"
	}

	region := vertexcore.GetModelRegion(info.ApiVersion, modelName)
	if strings.TrimSpace(region) == "" {
		region = "global"
	}

	// Omni models use the interactions API instead of predictLongRunning.
	if taskgemini.IsOmniModel(modelName) {
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

	if region == "global" {
		return fmt.Sprintf(
			"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:predictLongRunning",
			adc.ProjectID,
			modelName,
		), nil
	}
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predictLongRunning",
		region,
		adc.ProjectID,
		region,
		modelName,
	), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(a.apiKey), adc); err != nil {
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

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// Omni models use the interactions API with a different payload shape.
	if taskgemini.IsOmniModel(info.OriginModelName) {
		data, err := taskgemini.BuildOmniRequestBody(c, req, info.OriginModelName)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	body := requestPayload{
		Instances:  []map[string]any{{"prompt": req.Prompt}},
		Parameters: map[string]any{},
	}

	// 参考生视频：referenceImages 挂在 instances 内（不是 parameters）。
	// Vertex 的 image 用 {bytesBase64Encoded, mimeType}，与 Gemini API 的 inlineData 不同。
	refImages, err := taskgemini.BuildVeoReferenceImages(c, &req, true)
	if err != nil {
		return nil, err
	}
	if len(refImages) > 0 {
		body.Instances[0]["referenceImages"] = refImages
		// 官方硬约束：使用参考图时 durationSeconds 必须为 8、personGeneration 必须 allow_adult。
		if err := taskgemini.ApplyVeoReferenceConstraints(body.Instances[0], body.Parameters, &req); err != nil {
			return nil, err
		}
	}

	// Image-to-video: attach the reference image to the instance when provided.
	// 显式 references 时首帧取 first_frame，否则回退到旧的 image/images 语义。
	imageRef := ""
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleFirstFrame); ok {
		imageRef = strings.TrimSpace(ref.URL)
	}
	if imageRef == "" && len(req.References) == 0 {
		imageRef = strings.TrimSpace(req.Image)
		if imageRef == "" && len(req.Images) > 0 {
			imageRef = strings.TrimSpace(req.Images[0])
		}
	}
	if imageRef != "" {
		img, err := buildVeoImage(imageRef)
		if err != nil {
			return nil, fmt.Errorf("resolve image failed: %w", err)
		}
		if img != nil {
			body.Instances[0]["image"] = img
		}
	}
	// 尾帧（插值），必须与首帧搭配使用。
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleLastFrame); ok && strings.TrimSpace(ref.URL) != "" {
		if _, hasFirst := body.Instances[0]["image"]; !hasFirst {
			return nil, fmt.Errorf("last_frame must be used together with a first_frame image")
		}
		img, err := buildVeoImage(strings.TrimSpace(ref.URL))
		if err != nil {
			return nil, fmt.Errorf("resolve last_frame failed: %w", err)
		}
		if img != nil {
			body.Instances[0]["lastFrame"] = img
		}
	}

	if len(refImages) == 0 {
		seconds := common.String2Int(req.Seconds)
		if seconds > 0 {
			body.Instances[0]["duration"] = seconds
		}
		if req.Duration > 0 {
			body.Instances[0]["duration"] = req.Duration
		}
	}
	if req.Metadata != nil {
		if v, ok := req.Metadata["storageUri"]; ok {
			body.Parameters["storageUri"] = v
		}
		if v, ok := req.Metadata["sampleCount"]; ok {
			if i, ok := v.(int); ok {
				body.Parameters["sampleCount"] = i
			}
			if f, ok := v.(float64); ok {
				body.Parameters["sampleCount"] = int(f)
			}
		}
	}
	if _, ok := body.Parameters["sampleCount"]; !ok {
		body.Parameters["sampleCount"] = 1
	}

	if body.Parameters["sampleCount"].(int) <= 0 {
		return nil, fmt.Errorf("sampleCount must be greater than 0")
	}

	// if req.Duration > 0 {
	// 	body.Parameters["durationSeconds"] = req.Duration
	// } else if req.Seconds != "" {
	// 	seconds, err := strconv.Atoi(req.Seconds)
	// 	if err != nil {
	// 		return nil, errors.Wrap(err, "convert seconds to int failed")
	// 	}
	// 	body.Parameters["durationSeconds"] = seconds
	// }

	info.PriceData.OtherRatios = map[string]float64{
		"sampleCount": float64(body.Parameters["sampleCount"].(int)),
	}

	// if v, ok := body.Parameters["durationSeconds"]; ok {
	// 	info.PriceData.OtherRatios["durationSeconds"] = float64(v.(int))
	// }

	data, err := json.Marshal(body)
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
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// Omni interactions API returns an interaction object with an `id`.
	if taskgemini.IsOmniModel(info.OriginModelName) {
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
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	localID := encodeLocalTaskID(s.Name)
	c.JSON(http.StatusOK, gin.H{"task_id": localID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"veo-3.0-generate-001", "gemini-omni-flash-preview"}
}
func (a *TaskAdaptor) GetChannelName() string { return "vertex" }

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// Omni interactions API: GET the interaction by id.
	if taskgemini.IsOmniTaskID(taskID) {
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

	upstreamName, err := decodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}
	region := extractRegionFromOperationName(upstreamName)
	if region == "" {
		region = "us-central1"
	}
	project := extractProjectFromOperationName(upstreamName)
	modelName := extractModelFromOperationName(upstreamName)
	if project == "" || modelName == "" {
		return nil, fmt.Errorf("cannot extract project/model from operation name")
	}
	var url string
	if region == "global" {
		url = fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:fetchPredictOperation", project, modelName)
	} else {
		url = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:fetchPredictOperation", region, project, region, modelName)
	}
	payload := map[string]string{"operationName": upstreamName}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(key), adc); err != nil {
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
	if taskgemini.IsOmniResponseBody(respBody) {
		return taskgemini.ParseOmniTaskResult(respBody)
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
	// 走到这里说明 operation done 但没提取到任何视频产物。
	// Veo 内容安全过滤（done 但视频被 RAI 拦截）会命中这里：带上过滤原因判失败，
	// 否则会被误判为成功（既不退款，下游也拿不到视频地址）。
	ti.Status = model.TaskStatusFailure
	if op.Response.RaiMediaFilteredCount > 0 && len(op.Response.RaiMediaFilteredReasons) > 0 {
		ti.Reason = strings.Join(op.Response.RaiMediaFilteredReasons, "; ")
	} else if op.Response.RaiMediaFilteredCount > 0 {
		ti.Reason = "video generation blocked by safety filter"
	} else {
		ti.Reason = "operation done but no video returned"
	}
	return ti, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	modelName := ""
	if taskgemini.IsOmniTaskID(task.TaskID) {
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
	v := dto.NewOpenAIVideo()
	v.ID = task.TaskID
	v.Model = modelName
	v.Status = task.Status.ToVideoStatus()
	v.SetProgressStr(task.Progress)
	v.CreatedAt = task.CreatedAt
	v.CompletedAt = task.UpdatedAt
	if strings.HasPrefix(task.FailReason, "data:") && len(task.FailReason) > 0 {
		v.SetMetadata("url", task.FailReason)
	}
	// 成功且 FailReason 存的是 S3 直链时，输出顶层 video_url/url，供级联下游
	// （openai/sora 类型渠道，其 ParseTaskResult 读顶层 video_url/url/result_url）取到真实地址，
	// 否则下游只能退回拼接自身 /v1/videos/{id}/content 的兜底地址。
	if task.Status == model.TaskStatusSuccess && strings.HasPrefix(task.FailReason, "https://") {
		v.VideoUrl = task.FailReason
		v.Url = task.FailReason
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

// buildVeoImage normalizes an image reference into the Vertex Veo predict image object.
// Supports gs:// (gcsUri), data: URIs, http(s) URLs (downloaded to base64) and raw base64.
// Returns nil when the reference is empty.
func buildVeoImage(image string) (map[string]any, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil, nil
	}
	// GCS reference is passed through directly.
	if strings.HasPrefix(image, "gs://") {
		return map[string]any{"gcsUri": image}, nil
	}
	// data:<mime>;base64,<data>
	if strings.HasPrefix(image, "data:") {
		idx := strings.Index(image, ",")
		if idx < 0 {
			return nil, fmt.Errorf("invalid data uri")
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
		return map[string]any{"bytesBase64Encoded": b64, "mimeType": mt}, nil
	}
	// http(s) URL: download and convert to base64.
	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		mt, b64, err := service.GetImageFromUrl(image)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(mt) == "" {
			mt = "image/jpeg"
		}
		return map[string]any{"bytesBase64Encoded": b64, "mimeType": mt}, nil
	}
	// assume raw base64 without a data-uri prefix
	return map[string]any{"bytesBase64Encoded": image, "mimeType": "image/jpeg"}, nil
}

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
