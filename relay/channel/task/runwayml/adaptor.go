package runwayml

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type contentModeration struct {
	PublicFigureThreshold string `json:"publicFigureThreshold,omitempty"`
}

type textToVideoRequest struct {
	PromptText        string             `json:"promptText"`
	Ratio             string             `json:"ratio,omitempty"`
	Duration          int                `json:"duration,omitempty"`
	Seed              *int64             `json:"seed,omitempty"`
	ContentModeration *contentModeration `json:"contentModeration,omitempty"`
	Model             string             `json:"model"`
}

type imageToVideoRequest struct {
	PromptText        string             `json:"promptText,omitempty"`
	PromptImage       string             `json:"promptImage"`
	Seed              *int64             `json:"seed,omitempty"`
	Ratio             string             `json:"ratio,omitempty"`
	Duration          int                `json:"duration,omitempty"`
	ContentModeration *contentModeration `json:"contentModeration,omitempty"`
	Model             string             `json:"model"`
}

type submitResponse struct {
	ID string `json:"id"`
}

type taskDetailResponse struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"createdAt"`
	Output    []string `json:"output,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

const (
	actionTextToVideo  = "text_to_video"
	actionImageToVideo = "image_to_video"
	runwayVersion      = "2024-11-06"
)

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

// ValidateRequestAndSetAction validates the request and sets the action based on image presence.
// Priority: Images[0] first, then fall back to Image field.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *hostdto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// EstimateBilling prices the request per second of generated video.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	return taskcommon.SecondsRatio(c, 2)
}

// BuildRequestURL constructs the upstream URL based on the action.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	action := a.determineAction(info)
	return fmt.Sprintf("%s/v1/%s", a.baseURL, action), nil
}

// BuildRequestHeader sets required RunwayML headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("X-Runway-Version", runwayVersion)
	return nil
}

// BuildRequestBody converts TaskSubmitReq into RunwayML request format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	action := a.determineAction(info)
	info.UpstreamModelName = req.Model

	var body any
	if action == actionImageToVideo {
		body = a.buildImageToVideoRequest(&req)
	} else {
		body = a.buildTextToVideoRequest(&req)
	}
	//common.PrintJson("runway:", body)
	data, err := common.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body failed")
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
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

	var sResp submitResponse
	if err := common.Unmarshal(responseBody, &sResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if sResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = sResp.ID
	ov.TaskID = sResp.ID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return sResp.ID, responseBody, nil
}

// FetchTask fetches task status from RunwayML.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Runway-Version", runwayVersion)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult parses RunwayML task status response into internal TaskInfo.
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resTask taskDetailResponse
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch strings.ToUpper(resTask.Status) {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "THROTTLED":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "5%"
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		if len(resTask.Output) > 0 {
			taskResult.Url = resTask.Output[0]
		}
	case "FAILED":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task failed"
	case "CANCELLED":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task cancelled"
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo converts stored task data into OpenAIVideo format.
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp taskDetailResponse
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal runwayml task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if len(dResp.Output) > 0 {
		openAIVideo.SetMetadata("url", dResp.Output[0])
	}

	if dResp.Status == "FAILED" || dResp.Status == "CANCELLED" {
		reason := "task failed"
		if dResp.Status == "CANCELLED" {
			reason = "task cancelled"
		}
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: reason,
			Code:    strings.ToLower(dResp.Status),
		}
	}

	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}

// ============================
// Internal helpers
// ============================

// determineAction checks if the request has an image to decide the action.
func (a *TaskAdaptor) determineAction(info *relaycommon.RelayInfo) string {
	if info.Action == constant.TaskActionGenerate {
		return actionImageToVideo
	}
	return actionTextToVideo
}

// getImageURL extracts the image URL from the request, prioritizing Images[0] over Image.
func getImageURL(req *relaycommon.TaskSubmitReq) string {
	if len(req.Images) > 0 {
		return req.Images[0]
	}
	return strings.TrimSpace(req.Image)
}

func (a *TaskAdaptor) buildTextToVideoRequest(req *relaycommon.TaskSubmitReq) *textToVideoRequest {
	r := &textToVideoRequest{
		PromptText: req.Prompt,
		Model:      req.Model,
	}
	if req.Duration > 0 {
		r.Duration = req.Duration
	} else {
		seconds := common.String2Int(req.Seconds)
		if seconds > 0 {
			r.Duration = seconds
		} else {
			r.Duration = 2 // 默认2秒
		}
	}
	a.applyMetadata(req.Metadata, &r.Ratio, &r.Seed, &r.ContentModeration)
	return r
}

func (a *TaskAdaptor) buildImageToVideoRequest(req *relaycommon.TaskSubmitReq) *imageToVideoRequest {
	r := &imageToVideoRequest{
		PromptText:  req.Prompt,
		PromptImage: getImageURL(req),
		Model:       req.Model,
	}
	if req.Duration > 0 {
		r.Duration = req.Duration
	} else {
		seconds := common.String2Int(req.Seconds)
		if seconds > 0 {
			r.Duration = seconds
		} else {
			r.Duration = 2 // 默认2秒
		}
	}
	a.applyMetadata(req.Metadata, &r.Ratio, &r.Seed, &r.ContentModeration)
	return r
}

// applyMetadata extracts ratio, seed, and contentModeration from metadata.
func (a *TaskAdaptor) applyMetadata(metadata map[string]interface{}, ratio *string, seed **int64, cm **contentModeration) {
	if metadata == nil {
		*ratio = "1280:720"
		return
	}
	if r, ok := metadata["ratio"].(string); ok {
		*ratio = r
	} else {
		*ratio = "1280:720"
	}
	if s, ok := metadata["seed"].(float64); ok {
		v := int64(s)
		*seed = &v
	}
	if cmVal, ok := metadata["contentModeration"]; ok {
		// contentModeration can be a map with publicFigureThreshold
		if cmMap, ok := cmVal.(map[string]interface{}); ok {
			mod := &contentModeration{}
			if threshold, ok := cmMap["publicFigureThreshold"].(string); ok {
				mod.PublicFigureThreshold = threshold
			}
			*cm = mod
		}
	}
}
