package ltx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// https://docs.ltx.io/api-documentation/api-reference/async-video-generation/submit-text-to-video
// https://docs.ltx.io/api-documentation/api-reference/async-video-generation/submit-image-to-video
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	uploadVideo func(context.Context, string) (string, error)
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *hostdto.TaskError) {
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "task_request_missing", http.StatusInternalServerError)
	}
	payload, err := a.convertToRequestPayload(&taskReq)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_ltx_request", http.StatusBadRequest)
	}
	if err := validateLtxRequest(payload); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_ltx_parameters", http.StatusBadRequest)
	}
	if payload.Model == "ltx-2-5-fast" || payload.Model == "ltx-2-5-pro" {
		// LTX 2.5 uses the configured model price as its 720p per-second base.
		// Duration and resolution are applied by EstimateBilling.
		info.DynamicModelPrice = 0
		return nil
	}

	taskType := "t2v"
	if payload.ImageURI != "" {
		taskType = "i2v"
	}
	// Legacy LTX models retain their adaptor-owned prices.
	info.DynamicModelPrice = getLegacyModelPrice(taskType, payload.Model, payload.Resolution)
	if info.DynamicModelPrice == 0 {
		return service.TaskErrorWrapperLocal(errors.New("model price not found"), "model_price_not_found", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling prices the request per second of generated video.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	ratios := taskcommon.SecondsRatio(c, 6)
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil || (req.Model != "ltx-2-5-fast" && req.Model != "ltx-2-5-pro") {
		return ratios
	}
	resolutionRatio := getLtx25ResolutionRatio(req.Model, resolveLtxResolution(&req))
	if resolutionRatio > 0 {
		ratios["resolution"] = resolutionRatio
	}
	return ratios
}

func getLtx25ResolutionRatio(modelName, resolution string) float64 {
	switch modelName {
	case "ltx-2-5-fast":
		switch resolution {
		case "1280x720", "720x1280":
			return 1
		case "1920x1080", "1080x1920":
			return 0.13 / 0.09
		case "2560x1440", "1440x2560":
			return 0.19 / 0.09
		case "3840x2160", "2160x3840":
			return 0.30 / 0.09
		}
	case "ltx-2-5-pro":
		switch resolution {
		case "1280x720", "720x1280":
			return 1
		case "1920x1080", "1080x1920":
			return 0.17 / 0.12
		}
	}
	return 0
}

func getLegacyModelPrice(taskType string, modelName, resolution string) float64 {
	priceMap := map[string]map[string]map[string]float64{
		"t2v": {
			"ltx-2-fast": {
				"1920x1080": 0.04,
				"2560x1440": 0.08,
				"3840x2160": 0.16,
			},
			"ltx-2-pro": {
				"1920x1080": 0.06,
				"2560x1440": 0.12,
				"3840x2160": 0.24,
			},
			"ltx-2-3-fast": {
				"1920x1080": 0.04,
				"1080x1920": 0.04,
				"2560x1440": 0.08,
				"1440x2560": 0.08,
				"3840x2160": 0.16,
				"2160x3840": 0.16,
			},
			"ltx-2-3-pro": {
				"1920x1080": 0.06,
				"1080x1920": 0.06,
				"2560x1440": 0.12,
				"1440x2560": 0.12,
				"3840x2160": 0.24,
				"2160x3840": 0.24,
			},
		},
		"i2v": {
			"ltx-2-fast": {
				"1920x1080": 0.04,
				"2560x1440": 0.08,
				"3840x2160": 0.16,
			},
			"ltx-2-pro": {
				"1920x1080": 0.06,
				"2560x1440": 0.12,
				"3840x2160": 0.24,
			},
			"ltx-2-3-fast": {
				"1920x1080": 0.04,
				"1080x1920": 0.04,
				"2560x1440": 0.08,
				"1440x2560": 0.08,
				"3840x2160": 0.16,
				"2160x3840": 0.16,
			},
			"ltx-2-3-pro": {
				"1920x1080": 0.06,
				"1080x1920": 0.06,
				"2560x1440": 0.12,
				"1440x2560": 0.12,
				"3840x2160": 0.24,
				"2160x3840": 0.24,
			},
		},
	}
	if priceMap[taskType] == nil || priceMap[taskType][modelName] == nil {
		return 0
	}
	if priceMap[taskType][modelName][resolution] == 0 {
		return 0
	}
	return priceMap[taskType][modelName][resolution] * 1.5 // 国外模型计费1.5 cover成本
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v2/%s", strings.TrimRight(a.baseURL, "/"), ltxEndpointForAction(info.Action)), nil
}

func ltxEndpointForAction(action string) string {
	if action == constant.TaskActionTextGenerate {
		return "text-to-video"
	}
	return "image-to-video"
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody converts the unified task request into the LTX payload.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, err
	}
	if body.ImageURI == "" {
		c.Set("action", constant.TaskActionTextGenerate)
	} else {
		c.Set("action", constant.TaskActionImageGenerate)
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *hostdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	_ = resp.Body.Close()

	var ltxResp LtxJobCreatedResponse
	if err = common.Unmarshal(responseBody, &ltxResp); err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(ltxResp.ID) == "" {
		taskErr = service.TaskErrorWrapper(errors.New("job id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)

	return ltxResp.ID, responseBody, nil
}

// FetchTask polls the same endpoint family used to submit the LTX job.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	action, _ := body["action"].(string)
	uri := ltxJobStatusURL(baseUrl, action, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func ltxJobStatusURL(baseURL, action, taskID string) string {
	return fmt.Sprintf("%s/v2/%s/%s",
		strings.TrimRight(baseURL, "/"), ltxEndpointForAction(action), url.PathEscape(taskID))
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"ltx-2-5-fast", "ltx-2-5-pro",
		"ltx-2-3-fast", "ltx-2-3-pro",
		"ltx-2-fast", "ltx-2-pro",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "ltx"
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*LtxTaskRequest, error) {
	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 6
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}

	imageURI := strings.TrimSpace(req.Image)
	if firstFrame, ok := req.FirstRefByRole(relaycommon.RefRoleFirstFrame); ok {
		imageURI = strings.TrimSpace(firstFrame.URL)
	} else if imageURI == "" && len(req.Images) > 0 {
		imageURI = strings.TrimSpace(req.Images[0])
	}
	lastFrameURI := strings.TrimSpace(req.LastFrameURI)
	if lastFrame, ok := req.FirstRefByRole(relaycommon.RefRoleLastFrame); ok {
		lastFrameURI = strings.TrimSpace(lastFrame.URL)
	}

	options := struct {
		FPS           *int   `json:"fps,omitempty"`
		CameraMotion  string `json:"camera_motion,omitempty"`
		GenerateAudio *bool  `json:"generate_audio,omitempty"`
		LastFrameURI  string `json:"last_frame_uri,omitempty"`
	}{}
	if req.Metadata != nil {
		metadataBytes, err := common.Marshal(req.Metadata)
		if err != nil {
			return nil, errors.Wrap(err, "metadata marshal metadata failed")
		}
		if err := common.Unmarshal(metadataBytes, &options); err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
	}
	if req.FPS != nil {
		options.FPS = req.FPS
	}
	if req.CameraMotion != "" {
		options.CameraMotion = req.CameraMotion
	}
	if req.GenerateAudio != nil {
		options.GenerateAudio = req.GenerateAudio
	}
	if lastFrameURI != "" {
		options.LastFrameURI = lastFrameURI
	}

	r := LtxTaskRequest{
		Prompt:        req.Prompt,
		Model:         req.Model,
		Duration:      seconds,
		ImageURI:      imageURI,
		LastFrameURI:  options.LastFrameURI,
		Resolution:    resolveLtxResolution(req),
		FPS:           options.FPS,
		CameraMotion:  options.CameraMotion,
		GenerateAudio: options.GenerateAudio,
	}
	return &r, nil
}

func resolveLtxResolution(req *relaycommon.TaskSubmitReq) string {
	if resolution := strings.TrimSpace(req.Size); resolution != "" {
		return resolution
	}
	if resolution := strings.TrimSpace(req.Resolution); resolution != "" {
		return resolution
	}
	return "1920x1080"
}

func validateLtxRequest(req *LtxTaskRequest) error {
	if req == nil {
		return errors.New("request is required")
	}
	if req.LastFrameURI != "" && req.ImageURI == "" {
		return errors.New("last_frame_uri requires image_uri")
	}
	if req.LastFrameURI != "" && (req.Model == "ltx-2-fast" || req.Model == "ltx-2-pro") {
		return fmt.Errorf("model %s does not support last_frame_uri", req.Model)
	}
	if req.CameraMotion != "" && !lo.Contains([]string{
		"dolly_in", "dolly_out", "dolly_left", "dolly_right",
		"jib_up", "jib_down", "static", "focus_shift",
	}, req.CameraMotion) {
		return fmt.Errorf("camera_motion %q is not supported", req.CameraMotion)
	}
	if req.Model != "ltx-2-5-fast" && req.Model != "ltx-2-5-pro" {
		return nil
	}

	fps := 24
	if req.FPS != nil {
		fps = *req.FPS
	}
	shortDuration := lo.Contains([]int{6, 8, 10}, req.Duration)
	longDuration := lo.Contains([]int{6, 8, 10, 12, 14, 16, 18, 20}, req.Duration)
	is720Or1080 := lo.Contains([]string{
		"1280x720", "720x1280", "1920x1080", "1080x1920",
	}, req.Resolution)

	if req.Model == "ltx-2-5-pro" {
		if !is720Or1080 || !lo.Contains([]int{24, 25, 50}, fps) || !shortDuration {
			return fmt.Errorf("model %s does not support resolution=%s, fps=%d, duration=%d",
				req.Model, req.Resolution, fps, req.Duration)
		}
		return nil
	}

	if !lo.Contains([]string{
		"1280x720", "720x1280", "1920x1080", "1080x1920",
		"2560x1440", "1440x2560", "3840x2160", "2160x3840",
	}, req.Resolution) {
		return fmt.Errorf("model %s does not support resolution=%s", req.Model, req.Resolution)
	}
	if !lo.Contains([]int{24, 25, 48, 50}, fps) {
		return fmt.Errorf("model %s does not support fps=%d", req.Model, fps)
	}
	if is720Or1080 && lo.Contains([]int{24, 25}, fps) {
		if longDuration {
			return nil
		}
	} else if shortDuration {
		return nil
	}
	return fmt.Errorf("model %s does not support resolution=%s, fps=%d, duration=%d",
		req.Model, req.Resolution, fps, req.Duration)
}

func (a *TaskAdaptor) getAspectRatio(size string) string {
	switch size {
	case "1024x1024", "512x512":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	default:
		return "1:1"
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response LtxJobStatusResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	taskInfo := &relaycommon.TaskInfo{Code: 0, TaskID: response.ID}
	switch response.Status {
	case "pending":
		taskInfo.Status = model.TaskStatusQueued
		taskInfo.Progress = taskcommon.ProgressQueued
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	case "completed":
		if strings.TrimSpace(response.Result.VideoURL) == "" {
			return nil, errors.New("completed LTX job has no result.video_url")
		}
		uploadVideo := service.UploadOnceToS3
		if a.uploadVideo != nil {
			uploadVideo = a.uploadVideo
		}
		videoURL, err := uploadVideo(context.Background(), response.Result.VideoURL)
		if err != nil {
			return nil, errors.Wrap(err, "upload completed LTX video to S3")
		}
		if strings.TrimSpace(videoURL) == "" {
			return nil, errors.New("upload completed LTX video to S3 returned empty URL")
		}
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Url = videoURL
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		if response.Error != nil {
			taskInfo.Reason = strings.TrimSpace(response.Error.Message)
			if taskInfo.Reason == "" {
				taskInfo.Reason = strings.TrimSpace(response.Error.Type)
			}
		}
		if taskInfo.Reason == "" {
			taskInfo.Reason = "task failed"
		}
	default:
		return nil, fmt.Errorf("unknown LTX job status: %s", response.Status)
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if resultURL := taskcommon.ExternalResultURL(originTask); resultURL != "" {
		openAIVideo.VideoUrl = resultURL
		openAIVideo.Url = resultURL
		openAIVideo.SetMetadata("url", resultURL)
	}

	if originTask.Status == model.TaskStatusFailure {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: originTask.FailReason,
			Code:    "ltx_failure",
		}
	}
	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}
