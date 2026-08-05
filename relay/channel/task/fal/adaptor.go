package fal

// https://fal.ai/models/fal-ai/flux-pro/kontext/api#queue-submit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/common"
	taskbilling "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

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
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate); taskErr != nil {
		return taskErr
	}
	// 此时模型映射尚未写入 UpstreamModelName，请求体里的 model 已经过渠道映射，可直接用。
	if req, err := relaycommon.GetTaskRequest(c); err == nil && isFlux3Video(req.Model) {
		return flux3SetAction(c, info)
	}
	return nil
}

// EstimateBilling prices the request per second of generated video.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if isFlux3Video(info.UpstreamModelName) {
		req, err := relaycommon.GetTaskRequest(c)
		if err != nil {
			return nil
		}
		// 与 BuildRequestBody 下发的 duration 用同一个函数，避免计费与实际时长漂移。
		return map[string]float64{"seconds": float64(flux3BilledSeconds(req))}
	}
	return taskbilling.SecondsRatio(c, 5)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isFlux3Video(info.UpstreamModelName) {
		return fmt.Sprintf("%s/%s/%s", a.baseURL, flux3AppPath, flux3Endpoint(info.Action)), nil
	}
	// https://queue.fal.run/fal-ai/flux-pro/kontext
	switch info.OriginModelName {
	case "flux-1-kontext-pro":
		return fmt.Sprintf("%s/fal-ai/flux-pro/kontext", a.baseURL), nil
	case "imagineart-1.5-preview":
		//https://queue.fal.run/imagineart/imagineart-1.5-preview/text-to-image
		return fmt.Sprintf("%s/imagineart/imagineart-1.5-preview/text-to-image", a.baseURL), nil
	case "hunyuan-video-v1.5":
		return fmt.Sprintf("%s/fal-ai/%s/image-to-video", a.baseURL, info.OriginModelName), nil
	}
	return "", fmt.Errorf("unsupported model name: %s", info.OriginModelName)
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("Authorization", "Key "+a.apiKey)
	return nil
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {

	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)
	if isFlux3Video(info.UpstreamModelName) {
		return Flux3VideoRequestBody(req, info.Action)
	}
	if strings.HasPrefix(info.OriginModelName, "hunyuan-video") {
		return HunyuanImage2VideoRequestBody(req)
	}

	image := req.Image
	if len(req.Images) > 0 {
		image = req.Images[0]
	}
	body := EditImageTaskRequest{
		Prompt:          req.Prompt,
		Seed:            -1,
		ImageURL:        image,
		SafetyTolerance: "5",
		GuidanceScale:   3.5,
		NumImages:       1,
		OutputFormat:    "jpeg",
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {
		if v, ok := req.Metadata["aspect_ratio"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.AspectRatio = s
			}
		} else {
			body.AspectRatio = "1:1"
		}

		metadata := req.Metadata
		medaBytes, err := common.Marshal(metadata)
		if err != nil {
			return nil, errors.Wrap(err, "metadata marshal metadata failed")
		}
		err = common.Unmarshal(medaBytes, &body)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
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
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var s EditImageTaskResponse
	if err := common.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.RequestID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing taskId"), "invalid_response", http.StatusInternalServerError)
	}
	localID := s.RequestID
	c.JSON(http.StatusOK, gin.H{"task_id": info.PublicTaskID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"flux-1-kontext-pro", "imagineart-1.5-preview", Flux3VideoModel}
}
func (a *TaskAdaptor) GetChannelName() string { return "fal" }

// falQueueStatusURL 返回某个模型在 fal 队列上的任务查询地址。
// fal 的 requests 路径是「应用路径」，不含 BuildRequestURL 里的子端点那一段
// （如 flux-3 的 text-to-video / image-to-video 共用同一个 requests 路径）。
func falQueueStatusURL(modelName, taskID string) string {
	switch {
	case isFlux3Video(modelName):
		return fmt.Sprintf("https://queue.fal.run/%s/requests/%s", flux3AppPath, taskID)
	case strings.Contains(modelName, "imagineart"):
		return fmt.Sprintf("https://queue.fal.run/imagineart/%s/requests/%s", modelName, taskID)
	case strings.Contains(modelName, "hunyuan-video"):
		return fmt.Sprintf("https://queue.fal.run/fal-ai/hunyuan-video-v1.5/requests/%s", taskID)
	default:
		return fmt.Sprintf("https://queue.fal.run/fal-ai/flux-pro/requests/%s", taskID)
	}
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	// 调用方传进来的是上游 request ID，而 task_id 列存的是公开 ID（task_xxxx），
	// 按它查库必然查不到。模型名优先取调用方透传的 model，查库只作为老数据的兜底。
	modelName, _ := body["model"].(string)
	if modelName == "" {
		task, exists, _ := model.GetByOnlyTaskId(taskID)
		if !exists || task == nil {
			return nil, fmt.Errorf("task not found")
		}
		modelName = task.Properties.UpstreamModelName
	}
	url := falQueueStatusURL(modelName, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Key "+key)
	/*
	 {"id":"xxxxxx","status":"pending",.....}
	*/
	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var op QueryTaskResponse
	if err = common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	op.RequestID = taskID

	data, err := common.Marshal(op)
	if err != nil {
		return nil, fmt.Errorf("marshal response body failed: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op QueryTaskResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	taskId := op.RequestID
	ti.TaskID = taskId
	ti.Status = model.TaskStatusInProgress
	ti.Progress = fmt.Sprintf("%d%%", 10)
	if len(op.Images) == 0 && op.Video.URL == "" {
		return ti, nil
	}
	if len(op.Images) > 0 { // some variants use `video` as base64
		ti.Progress = fmt.Sprintf("%d%%", 100)
		ti.Status = model.TaskStatusSuccess
		if v, ok := taskcommon.FinishedTaskCache.Get(taskId); ok {
			ti.Url = v
			return ti, nil
		}
		imageUrl := op.Images[0].URL
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
	}
	if op.Video.URL != "" {
		ti.Progress = fmt.Sprintf("%d%%", 100)
		ti.Status = model.TaskStatusSuccess
		if v, ok := taskcommon.FinishedTaskCache.Get(taskId); ok {
			ti.Url = v
			return ti, nil
		}
		ti.Url = op.Video.URL
		taskcommon.FinishedTaskCache.Set(taskId, ti.Url)
		return ti, nil
	}
	return ti, nil
}
