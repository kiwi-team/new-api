package replicatetask

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

type replicateInput struct {
	Prompt      string `json:"prompt"`
	Image       string `json:"image,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Seed        *int64 `json:"seed,omitempty"`
}

type replicateRequest struct {
	Input replicateInput `json:"input"`
}

type replicateSubmitResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  any    `json:"error,omitempty"`
}

type replicateTaskResponse struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	Output      any    `json:"output,omitempty"`
	Error       any    `json:"error,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *hostdto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// EstimateBilling prices the request per second of generated video.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	return taskcommon.SecondsRatio(c, 5)
}

// BuildRequestURL constructs the Replicate predictions URL for the model.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	//fmt.Printf("upstream: %s, origin : %s\n", info.UpstreamModelName, info.OriginModelName)
	// upstream: runwayml/gen-4.5, origin : gen4.
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	// POST https://api.replicate.com/v1/models/{owner}/{model}/predictions
	return fmt.Sprintf("%s/v1/models/%s/predictions", a.baseURL, modelName), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	info.UpstreamModelName = req.Model

	input := replicateInput{
		Prompt: req.Prompt,
	}

	// image
	image := strings.TrimSpace(req.Image)
	if len(req.Images) > 0 {
		image = req.Images[0]
	}
	if image != "" {
		input.Image = image
	}

	// duration
	if req.Duration > 0 {
		input.Duration = req.Duration
	} else {
		seconds := common.String2Int(req.Seconds)
		if seconds > 0 {
			input.Duration = seconds
		} else {
			input.Duration = 5
		}
	}

	// metadata overrides
	if req.Metadata != nil {
		if ar, ok := req.Metadata["aspect_ratio"].(string); ok && ar != "" {
			input.AspectRatio = ar
		}
		//"enum": [ "16:9", "9:16", "4:3", "3:4", "1:1", "21:9" ],
		//"1280:720", "720:1280", "1104:832", "960:960", "832:1104", "1584:672"
		ratioMap := map[string]string{
			"1280:720": "16:9",
			"720:1280": "9:16",
			"1104:832": "4:3",
			"832:1104": "3:4",
			"960:960":  "1:1",
			"1584:672": " 21:9",
		}
		if ratio, ok := req.Metadata["ratio"].(string); ok && ratio != "" {
			if newRatio, ok := ratioMap[ratio]; ok {
				input.AspectRatio = newRatio
			}
		}
		if s, ok := req.Metadata["seed"].(float64); ok {
			v := int64(s)
			input.Seed = &v
		}
		if d, ok := req.Metadata["duration"].(float64); ok && int(d) > 0 {
			input.Duration = int(d)
		}
	}
	if input.AspectRatio == "" {
		input.AspectRatio = "16:9"
	}

	body := replicateRequest{Input: input}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body failed")
	}
	return bytes.NewReader(data), nil
}

// DoRequest sends the request and normalizes Replicate's 201 Created to 200 OK
// so the upstream relay_task.go status check passes.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	resp, err := channel.DoTaskApiRequest(a, c, info, requestBody)
	if err != nil {
		return nil, err
	}
	// Replicate returns 201 Created on successful prediction creation
	if resp != nil && resp.StatusCode == http.StatusCreated {
		resp.StatusCode = http.StatusOK
		resp.Status = "200 OK"
	}
	return resp, nil
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *hostdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var sResp replicateSubmitResponse
	if err := common.Unmarshal(responseBody, &sResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if sResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty, body: %s", responseBody), "invalid_response", http.StatusInternalServerError)
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

// FetchTask fetches task status from Replicate.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/predictions/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

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

// ParseTaskResult parses Replicate prediction response into internal TaskInfo.
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resTask replicateTaskResponse
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch strings.ToLower(resTask.Status) {
	case "starting":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "5%"
	case "processing":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = extractOutputURL(resTask.Output)
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task failed"
		if resTask.Error != nil {
			if errStr, ok := resTask.Error.(string); ok {
				taskResult.Reason = errStr
			}
		}
	case "canceled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task canceled"
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo converts stored task data into OpenAIVideo format.
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp replicateTaskResponse
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal replicate task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	outputURL := extractOutputURL(dResp.Output)
	if outputURL != "" {
		openAIVideo.SetMetadata("url", outputURL)
	}

	if resTask, err := a.ParseTaskResult(originTask.Data); err == nil && resTask != nil {
		if resTask.Url != "" {
			openAIVideo.SetMetadata("url", resTask.Url)
		}
	}

	if dResp.Status == "failed" || dResp.Status == "canceled" {
		reason := "task failed"
		if dResp.Status == "canceled" {
			reason = "task canceled"
		}
		if dResp.Error != nil {
			if errStr, ok := dResp.Error.(string); ok {
				reason = errStr
			}
		}
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: reason,
			Code:    dResp.Status,
		}
	}

	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}

// extractOutputURL extracts the video URL from the Replicate output field.
// Output can be a string (single URL) or an array of strings.
func extractOutputURL(output any) string {
	if output == nil {
		return ""
	}
	// single string output
	if s, ok := output.(string); ok {
		return s
	}
	// array output — take first element
	if arr, ok := output.([]any); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return s
		}
	}
	return ""
}
