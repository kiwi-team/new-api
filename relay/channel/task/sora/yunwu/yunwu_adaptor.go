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
	taskbilling "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// 用对接yunwu的soar-2-pro
// ============================
// Request / Response structures
// ============================
// https://yunwu.apifox.cn/api-311044999
type YunwuSoraTaskSubmitRequest struct {
	// 支持 15，25
	Duration int `json:"duration"`
	// 图片链接
	Images []string `json:"images,omitempty"`
	// 模型名字
	Model string `json:"model"`
	// portrait 竖屏
	// landscape 横屏
	Orientation string `json:"orientation"`
	// 提示词
	Prompt string `json:"prompt"`
	// large 高清1080p
	Size string `json:"size"`
	// 默认为： true  会优先无水印，如果出错，会兜底到有水印
	// 传递 false 的话 会强制让视频无水印，遇到去水印错误的会一直自动重试
	Watermark bool `json:"watermark,omitempty"`
}

type YunwuSoraTaskSubmitResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time"`
}

type YunwuSoraTaskResult struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time"`
	VideoURL         string `json:"video_url,omitempty"`
	Detail           any    `json:"detail,omitempty"`
	Progress         int    `json:"progress,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskbilling.BaseBilling
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
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return "https://yunwu.ai/v1/video/create", nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func getOrientationAndSize(req *relaycommon.TaskSubmitReq) (string, string) {
	size := req.Size
	if len(size) == 0 {
		return "portrait", "large"
	}
	size = strings.ToLower(size)
	yunwuSize := "large"
	if strings.Contains(size, "x") {
		arr := strings.Split(size, "x")
		width := common.String2Int(arr[0])
		height := common.String2Int(arr[1])
		if width >= 1080 || height >= 1080 {
			yunwuSize = "large"
		}
		if width > height {
			return "landscape", yunwuSize
		} else {
			return "portrait", yunwuSize
		}
	}
	return "portrait", yunwuSize
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)
	orientation, size := getOrientationAndSize(&req)
	seconds := common.String2Int(req.Seconds)
	if seconds == 0 {
		seconds = 10
	}
	body := YunwuSoraTaskSubmitRequest{
		Model:       req.Model,
		Prompt:      req.Prompt,
		Duration:    seconds,
		Size:        size,
		Orientation: orientation,
		Images:      req.Images,
		Watermark:   true,
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

	var s YunwuSoraTaskSubmitResponse
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

func (a *TaskAdaptor) GetModelList() []string { return []string{"sora-2-pro", "sora-2"} }
func (a *TaskAdaptor) GetChannelName() string { return "yunwu-sora" }

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
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	/*
	 {"id":"xxxxxx","status":"pending",.....}
	*/
	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op YunwuSoraTaskResult
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
	pendingStatus := []string{"pending", "running", "image_downloading", "video_generating", "video_generation_completed", "video_upsampling", "video_upsampling_completed"}
	if slices.Contains(pendingStatus, op.Status) {
		return ti, nil
	}
	if op.Status == "completed" {
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
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
		if op.Detail != nil {
			detail, ok := op.Detail.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid detail type")
			}
			if file, err := service.SimpleUploadToS3(context.Background(), detail["url"].(string)); err == nil {
				ti.Url = file
				taskcommon.FinishedTaskCache.Set(op.ID, file)
			}
			return ti, nil
		}
	}
	return ti, nil
}
