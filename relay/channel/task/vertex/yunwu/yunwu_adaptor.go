package yunwu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// 用对接yunwu的veo
// ============================
// Request / Response structures
// ============================
// https://yunwu.apifox.cn/api-311044999
type YunwuVeoTaskSubmitRequest struct {
	// ⚠️仅veo3支持，“16:9”或“9:16”
	AspectRatio    *string `json:"aspect_ratio,omitempty"`
	EnableUpsample *bool   `json:"enable_upsample,omitempty"`
	// 由于 veo 只支持英文提示词，所以如果需要中文自动转成英文提示词，可以开启此开关
	EnhancePrompt *bool `json:"enhance_prompt,omitempty"`
	// 当模型是带 veo2-fast-frames 最多支持两个，分别是首尾帧，当模型是 veo3-pro-frames 最多支持一个首帧，当模型是
	// veo2-fast-components 最多支持 3 个，此时图片为视频中的元素
	Images []string `json:"images,omitempty"`
	Model  string   `json:"model"`
	// 提示词
	Prompt string `json:"prompt"`
}

type YunwuVeoTaskSubmitResponse struct {
	EnhancedPrompt   string `json:"enhanced_prompt"`
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time"`
}

type YunwuVeoTaskResult struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time"`
	VideoURL         string `json:"video_url"`
	Detail           any    `json:"detail,omitempty"`
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
	return "http://yunwu.ai/v1/video/create", nil
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

	body := YunwuVeoTaskSubmitRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
	}

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

	var s YunwuVeoTaskSubmitResponse
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.ID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing taskId"), "invalid_response", http.StatusInternalServerError)
	}
	localID := s.ID
	c.JSON(http.StatusOK, gin.H{"task_id": localID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"veo-3.0-generate-001"} }
func (a *TaskAdaptor) GetChannelName() string { return "yunwu-veo" }

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	url := fmt.Sprintf("http://yunwu.ai/v1/video/query?id=%s", taskID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// http://yunwu.ai/v1/video/query?id=veo3.1-fast:1761101292-mmzJb1urEI
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	/*
	 {"id":"xxxxxx","status":"pending",.....}
	*/
	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op YunwuVeoTaskResult
	if err := json.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	var errorStatus []string
	ti.TaskID = op.ID
	errorStatus = append(errorStatus, "failed", "error", "video_generation_failed", "video_upsampling_failed")
	if slices.Contains(errorStatus, op.Status) {
		ti.Status = model.TaskStatusFailure
		ti.Reason = fmt.Sprintf("%v", op.Detail)
		ti.Progress = "100%"
		return ti, nil
	}
	ti.Status = model.TaskStatusInProgress
	ti.Progress = "50%"
	pendingStatus := []string{"pending", "image_downloading", "video_generating", "video_generation_completed", "video_upsampling", "video_upsampling_completed"}
	if slices.Contains(pendingStatus, op.Status) {
		return ti, nil
	}
	if op.Status == "completed" {
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
	}
	if op.VideoURL != "" { // some variants use `video` as base64
		if v, ok := taskcommon.FinishedTaskCache.Get(op.ID); ok {
			ti.Url = v
			return ti, nil
		}
		if file, err := service.SimpleUploadToS3(context.Background(), op.VideoURL); err == nil {
			ti.Url = file
			taskcommon.FinishedTaskCache.Set(op.ID, file)
		}
		return ti, nil
	}
	return ti, nil
}
