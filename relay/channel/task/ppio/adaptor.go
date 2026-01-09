package ppio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

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
	// Use the standard validation method for TaskSubmitReq
	var taskReq relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &taskReq); err != nil {
		return service.TaskErrorWrapper(err, "unmarshal_task_request_failed", http.StatusBadRequest)
	}
	seconds := common.String2Int(taskReq.Seconds)
	if seconds <= 0 {
		seconds = 5
	}
	if taskReq.Duration > 0 {
		seconds = taskReq.Duration
	}

	info.PriceData.OtherRatios = map[string]float64{
		"seconds": float64(seconds),
	}
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	///v3/async/flux-1-kontext-pro
	modeName := info.OriginModelName
	switch modeName {
	case "hailuo-2.3-i2v", "hailuo-2.3-t2v":
		return fmt.Sprintf("https://api.ppinfra.com/v3/async/minimax-%s", modeName), nil
	case "vidu-2.0-img2video":
		return fmt.Sprintf("https://api.ppinfra.com/v3/async/%s", modeName), nil
	default:
		return fmt.Sprintf("https://api.ppinfra.com/v3/async/%s", modeName), nil
	}
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)
	if strings.HasPrefix(info.OriginModelName, "wan") {
		return WanRequestBody(req)
	} else if strings.HasPrefix(info.OriginModelName, "vidu") {
		return ViduRequestBody(req)
	} else if strings.HasPrefix(info.OriginModelName, "hailuo") {
		return HailuoRequestBody(req)
	}
	return nil, fmt.Errorf("model %s not supported", info.OriginModelName)
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

	var s PPIOTaskSubmitResponse
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.TaskID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing taskId"), "invalid_response", http.StatusInternalServerError)
	}
	localID := s.TaskID
	c.JSON(http.StatusOK, gin.H{"task_id": localID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"flux-1-kontext-pro"} }
func (a *TaskAdaptor) GetChannelName() string { return "novita" }

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	url := fmt.Sprintf("https://api.ppinfra.com/v3/async/task-result?task_id=%s", taskID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	/*
	 {"id":"xxxxxx","status":"pending",.....}
	*/
	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op PPIOTaskResult
	if err := json.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	taskId := op.Task.TaskID
	ti.TaskID = taskId
	status := op.Task.Status
	if status == TASK_STATUS_FAILED {
		ti.Status = model.TaskStatusFailure
		ti.Reason = fmt.Sprintf("%v", op.Task.Reason)
		ti.Progress = "100%"
		return ti, nil
	}
	ti.Status = model.TaskStatusInProgress
	ti.Progress = fmt.Sprintf("%d%%", op.Task.ProgressPercent)
	if status == TASK_STATUS_PROCESSING || status == TASK_STATUS_QUEUED {
		return ti, nil
	}
	if status == TASK_STATUS_SUCCEED {
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
	}
	if len(op.Images) > 0 { // some variants use `video` as base64
		if v, ok := taskcommon.FinishedTaskCache.Get(taskId); ok {
			ti.Url = v
			return ti, nil
		}
		imageUrl := op.Images[0].ImageURL
		task, exists, _ := model.GetByOnlyTaskId(taskId)
		if exists && task != nil {
			if strings.HasPrefix(task.FailReason, "https://toio") {
				ti.Url = task.FailReason
				return ti, nil
			}
		}
		if file, err := service.SimpleUploadToS3(context.Background(), imageUrl); err == nil {
			ti.Url = file
			taskcommon.FinishedTaskCache.Set(taskId, file)
		} else {
			ti.Url = imageUrl
		}
		return ti, nil
	}
	if len(op.Videos) > 0 {
		// some variants use `video` as base64
		if v, ok := taskcommon.FinishedTaskCache.Get(taskId); ok {
			ti.Url = v
			return ti, nil
		}
		vidoeUrl := op.Videos[0].VideoURL
		task, exists, _ := model.GetByOnlyTaskId(taskId)
		if exists && task != nil {
			if strings.HasPrefix(task.FailReason, "https://toio") {
				ti.Url = task.FailReason
				return ti, nil
			}
		}
		if file, err := service.SimpleUploadToS3(context.Background(), vidoeUrl); err == nil {
			ti.Url = file
			taskcommon.FinishedTaskCache.Set(taskId, file)
		} else {
			ti.Url = vidoeUrl
		}
		return ti, nil
	}
	return ti, nil
}
