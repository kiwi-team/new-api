package hedra

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

const (
	hedraDefaultBaseURL = "https://api.hedra.com/web-app/public"
	ChannelName         = "hedra"
)

var ModelList = []string{"hedra-character-3"}

// ============================
// Request / Response structures
// ============================

type assetCreateReq struct {
	Name string `json:"name"`
	Type string `json:"type"` // "image" or "audio"
}

type assetCreateResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type generatedVideoInputs struct {
	TextPrompt  string `json:"text_prompt,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	DurationMs  int    `json:"duration_ms,omitempty"`
}

type generationReq struct {
	Type                 string               `json:"type"`
	AiModelID            string               `json:"ai_model_id"`
	StartKeyframeID      string               `json:"start_keyframe_id"`
	AudioID              string               `json:"audio_id,omitempty"`
	GeneratedVideoInputs generatedVideoInputs `json:"generated_video_inputs"`
}

type generationResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	JobID  string `json:"jobId,omitempty"`
}

type generationStatusResp struct {
	Status       string `json:"status"`
	DownloadURL  string `json:"download_url,omitempty"`
	AssetID      string `json:"asset_id,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type hedraModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// global model ID cache (model IDs are platform-wide, not per-user)
var (
	modelIDCache   string
	modelIDCacheMu sync.Mutex
)

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	proxy       string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	if a.baseURL == "" {
		a.baseURL = hedraDefaultBaseURL
	}
	a.apiKey = info.ApiKey
	if info.ChannelMeta != nil {
		a.proxy = info.ChannelMeta.ChannelSetting.Proxy
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/generations", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	return nil
}

// BuildRequestBody uploads image and audio assets to Hedra, then builds the generation request.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	// Image URL: prefer Images[0] over Image
	imageURL := taskReq.Image
	if len(taskReq.Images) > 0 {
		imageURL = taskReq.Images[0]
	}
	if imageURL == "" {
		return nil, fmt.Errorf("image URL is required")
	}
	// seconds := common.String2Int(taskReq.Seconds)
	// if seconds <= 0 {
	// 	seconds = 5
	// }
	// if taskReq.Duration > 0 {
	// 	seconds = taskReq.Duration
	// }

	// Audio URL must be in metadata.audio
	audioURL := ""
	if taskReq.Metadata != nil {
		if v, ok := taskReq.Metadata["audio"].(string); ok {
			audioURL = v
		}
	}
	if audioURL == "" {
		return nil, fmt.Errorf("audio URL is required (pass as metadata.audio)")
	}
	seconds := 5

	aspectRatio := "1:1"
	resolution := "540p"
	if taskReq.Metadata != nil {
		if v, ok := taskReq.Metadata["aspect_ratio"].(string); ok && v != "" {
			aspectRatio = v
		}
		if v, ok := taskReq.Metadata["resolution"].(string); ok && v != "" {
			resolution = v
		}
	}

	// Upload image and audio assets to Hedra
	imageAssetID, err := a.uploadAsset(imageURL, "image")
	if err != nil {
		return nil, fmt.Errorf("upload image asset: %w", err)
	}

	audioAssetID, err := a.uploadAsset(audioURL, "audio")
	if err != nil {
		return nil, fmt.Errorf("upload audio asset: %w", err)
	}

	// Probe audio duration to drive duration_ms. On failure, fall back to the default.
	if audioDuration, err := a.probeAudioDuration(audioURL); err != nil {
		common.SysLog(fmt.Sprintf("hedra: probe audio duration failed: %v", err))
	} else if audioDuration > 0 {
		seconds = int(audioDuration)
		if float64(seconds) < audioDuration {
			seconds++ // ceil so we don't cut off the tail
		}
	}

	// Resolve model ID: allow override via metadata.ai_model_id
	modelID := ""
	if taskReq.Model == "hedra-avatar" {
		modelID = "26f0fc66-152b-40ab-abed-76c43df99bc8" // the avatar model
	} else if taskReq.Model == "hedra-omnia" {
		modelID = "ab372b84-432f-44f5-bacc-c2542465f712"
	}
	// if taskReq.Metadata != nil {
	// 	if v, ok := taskReq.Metadata["ai_model_id"].(string); ok && v != "" {
	// 		modelID = v
	// 	}
	// }
	// if modelID == "" {
	// 	modelID, err = a.fetchModelID()
	// 	if err != nil {
	// 		return nil, fmt.Errorf("fetch Hedra model ID: %w", err)
	// 	}
	// }

	genReq := generationReq{
		Type:            "video",
		AiModelID:       modelID,
		StartKeyframeID: imageAssetID,
		AudioID:         audioAssetID,
		GeneratedVideoInputs: generatedVideoInputs{
			TextPrompt:  taskReq.Prompt,
			Resolution:  resolution,
			AspectRatio: aspectRatio,
			DurationMs:  seconds * 1000,
		},
	}

	data, err := common.Marshal(genReq)
	//common.PrintJson("hedra", genReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal generation request")
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var genResp generationResp
	if err := common.Unmarshal(responseBody, &genResp); err != nil {
		taskErr = service.TaskErrorWrapper(
			errors.Wrapf(err, "body: %s", responseBody),
			"unmarshal_response_failed",
			http.StatusInternalServerError,
		)
		return
	}

	if genResp.ID == "" {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("generation ID is empty, response: %s", string(responseBody)),
			"invalid_response",
			http.StatusInternalServerError,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = genResp.ID
	ov.TaskID = genResp.ID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return genResp.ID, responseBody, nil
}

// FetchTask polls Hedra for generation status.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	if baseUrl == "" {
		baseUrl = hedraDefaultBaseURL
	}

	uri := fmt.Sprintf("%s/generations/%s/status", baseUrl, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new http client: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// Upstream may return an error envelope (e.g. {"error":{"code":"unauthorized","message":"..."}})
	// instead of a normal status payload. Treat that as a task failure so we surface the reason
	// and trigger refund, rather than silently marking the task as submitted.
	var errResp struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil &&
		(errResp.Error.Message != "" || errResp.Error.Code != "") {
		reason := errResp.Error.Message
		if errResp.Error.Code != "" && reason != "" {
			reason = fmt.Sprintf("%s: %s", errResp.Error.Code, reason)
		} else if reason == "" {
			reason = errResp.Error.Code
		}
		return &relaycommon.TaskInfo{
			Status:   model.TaskStatusFailure,
			Progress: "100%",
			Reason:   reason,
		}, nil
	}

	var statusResp generationStatusResp
	if err := common.Unmarshal(respBody, &statusResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result")
	}

	ti := &relaycommon.TaskInfo{}
	switch statusResp.Status {
	case "processing":
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
	case "complete":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		ti.Url = statusResp.DownloadURL
	case "error":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		ti.Reason = statusResp.ErrorMessage
	default:
		// pending or unknown state
		ti.Status = model.TaskStatusSubmitted
		ti.Progress = "5%"
	}
	return ti, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	ov := dto.NewOpenAIVideo()
	ov.ID = originTask.TaskID
	ov.TaskID = originTask.TaskID
	ov.Status = originTask.Status.ToVideoStatus()
	ov.SetProgressStr(originTask.Progress)
	ov.CreatedAt = originTask.CreatedAt
	ov.CompletedAt = originTask.UpdatedAt
	ov.Model = originTask.Properties.OriginModelName

	if url := originTask.FailReason; strings.HasPrefix(url, "https://") {
		ov.SetMetadata("url", url)
	}

	jsonData, _ := common.Marshal(ov)
	return jsonData, nil
}

// ============================
// Internal helpers
// ============================

// uploadAsset downloads a file from fileURL and uploads it to Hedra, returning the asset ID.
func (a *TaskAdaptor) uploadAsset(fileURL, assetType string) (string, error) {
	assetID, err := a.createAsset(assetType)
	if err != nil {
		return "", fmt.Errorf("create %s asset slot: %w", assetType, err)
	}

	fileData, err := downloadFile(fileURL, a.proxy)
	if err != nil {
		return "", fmt.Errorf("download %s file: %w", assetType, err)
	}

	if err := a.doUploadFile(assetID, fileURL, fileData); err != nil {
		return "", fmt.Errorf("upload %s file: %w", assetType, err)
	}
	return assetID, nil
}

// probeAudioDuration downloads an audio file and returns its duration in seconds.
// The file extension is derived from the URL path to pick the right parser.
func (a *TaskAdaptor) probeAudioDuration(audioURL string) (float64, error) {
	ext := audioExtFromURL(audioURL)
	if ext == "" {
		return 0, fmt.Errorf("cannot determine audio extension from URL")
	}
	data, err := downloadFile(audioURL, a.proxy)
	if err != nil {
		return 0, err
	}
	return common.GetAudioDuration(context.Background(), bytes.NewReader(data), ext)
}

// audioExtFromURL extracts a lower-cased file extension (with dot) from a URL path,
// stripping any query string. Returns empty string if no extension is present.
func audioExtFromURL(fileURL string) string {
	u := fileURL
	if idx := strings.Index(u, "?"); idx != -1 {
		u = u[:idx]
	}
	if idx := strings.LastIndex(u, "."); idx != -1 && idx > strings.LastIndex(u, "/") {
		return strings.ToLower(u[idx:])
	}
	return ""
}

func (a *TaskAdaptor) createAsset(assetType string) (string, error) {
	reqBody := assetCreateReq{
		Name: fmt.Sprintf("%s_%d", assetType, time.Now().UnixMilli()),
		Type: assetType,
	}
	data, err := common.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/assets", a.baseURL), bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)

	client, err := service.GetHttpClientWithProxy(a.proxy)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create asset failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var ar assetCreateResp
	if err := common.Unmarshal(respBody, &ar); err != nil {
		return "", errors.Wrap(err, "unmarshal asset response")
	}
	if ar.ID == "" {
		return "", fmt.Errorf("asset ID is empty in response: %s", string(respBody))
	}
	return ar.ID, nil
}

func (a *TaskAdaptor) doUploadFile(assetID, fileURL string, fileData []byte) error {
	// Derive a filename from the URL (strip query string first)
	urlPath := fileURL
	if idx := strings.Index(urlPath, "?"); idx != -1 {
		urlPath = urlPath[:idx]
	}
	parts := strings.Split(urlPath, "/")
	filename := parts[len(parts)-1]
	if filename == "" {
		filename = "file"
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(fileData); err != nil {
		return err
	}
	w.Close()

	uri := fmt.Sprintf("%s/assets/%s/upload", a.baseURL, assetID)
	req, err := http.NewRequest(http.MethodPost, uri, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-API-Key", a.apiKey)

	client, err := service.GetHttpClientWithProxy(a.proxy)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload file failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// fetchModelID fetches the first available model ID from Hedra, with global caching.
func (a *TaskAdaptor) fetchModelID() (string, error) {
	modelIDCacheMu.Lock()
	if modelIDCache != "" {
		id := modelIDCache
		modelIDCacheMu.Unlock()
		return id, nil
	}
	modelIDCacheMu.Unlock()

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/models", a.baseURL), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", a.apiKey)

	client, err := service.GetHttpClientWithProxy(a.proxy)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get models failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var models []hedraModel
	if err := common.Unmarshal(respBody, &models); err != nil {
		return "", errors.Wrap(err, "unmarshal models response")
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no models available from Hedra")
	}

	modelIDCacheMu.Lock()
	modelIDCache = models[0].ID
	modelIDCacheMu.Unlock()

	return models[0].ID, nil
}

func downloadFile(url, proxy string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download file failed (HTTP %d)", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
