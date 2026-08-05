package ltx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// 任务行由 controller 在 DoResponse 返回之后才落库，后台生成 goroutine
// 需要等它出现才能写回结果。
const (
	taskInsertWaitAttempts = 10
	taskInsertWaitInterval = time.Second
)

// https://docs.ltx.video/api-documentation/api-reference/video-generation/image-to-video
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
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *hostdto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	var taskReq relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &taskReq); err != nil {
		return service.TaskErrorWrapper(err, "unmarshal_task_request_failed", http.StatusBadRequest)
	}
	taskType := "t2v"
	if taskReq.HasImage() {
		taskType = "i2v"
	}
	size := taskReq.Size
	if len(size) == 0 {
		size = "1920x1080"
	}
	// LTX 按「任务类型 × 模型 × 分辨率」定价，配置里的模型价格不适用。
	// 这里给出按次单价，RelayTaskSubmit 会用它覆盖配置价格。
	info.DynamicModelPrice = getModelPrice(taskType, taskReq.Model, size)
	if info.DynamicModelPrice == 0 {
		return service.TaskErrorWrapper(errors.New("model price not found"), "model_price_not_found", http.StatusBadRequest)
	}

	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// EstimateBilling prices the request per second of generated video.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	return taskcommon.SecondsRatio(c, 6)
}

func getModelPrice(taskType string, modelName, Resolution string) float64 {
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
	if priceMap[taskType][modelName] == nil {
		return 0
	}
	if priceMap[taskType][modelName][Resolution] == 0 {
		return 0
	}
	return priceMap[taskType][modelName][Resolution] * 1.5 // 国外模型计费1.5 cover成本
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.baseURL, nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody converts request into Kling specific format.
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

// DoRequest 不发起上游请求：LTX 的生成接口是同步的，直接返回视频字节，
// 耗时远超一次提交请求的生命周期。这里返回一个已提交的 mock 响应，真正的
// 调用由 DoResponse 起的后台 goroutine 完成。
// mock 的 task_id 必须用预生成的公开 ID，否则它会被当成「上游 ID」存进
// PrivateData.UpstreamTaskID，与落库的 task_id 对不上，轮询和后台写回都会失败。
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	return getMockResponse(LtxTaskResponse{
		TaskID: info.PublicTaskID,
		Status: string(model.TaskStatusSubmitted),
	})
}

func getMockResponse(resp LtxTaskResponse) (*http.Response, error) {
	data, err := common.Marshal(resp)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mock response failed")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBuffer(data)),
	}, nil
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *hostdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var kResp LtxTaskResponse
	if err = common.Unmarshal(responseBody, &kResp); err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	// DoRequest 造的 mock 里 task_id 就是公开 ID，两者不一致说明链路被改坏了。
	if kResp.TaskID != info.PublicTaskID {
		taskErr = service.TaskErrorWrapper(errors.New("mock task id mismatch"), "task_id_mismatch", http.StatusInternalServerError)
		return
	}

	// 后台 goroutine 不能再碰 gin.Context（handler 返回后它会被放回池中复用），
	// 所以在响应前把需要的值取成副本。
	v, exists := c.Get("task_request")
	if !exists {
		taskErr = service.TaskErrorWrapper(errors.New("request not found in context"), "task_request_missing", http.StatusInternalServerError)
		return
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		taskErr = service.TaskErrorWrapper(errors.New("unexpected task request type in context"), "task_request_invalid", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)

	go a.runBackgroundGeneration(info.PublicTaskID, info.Action, req)

	return info.PublicTaskID, responseBody, nil
}

// runBackgroundGeneration 在提交响应返回之后补齐真正的生成结果。
//
// 只写 video_url / fail_reason，不把任务置为终态：终态流转、退款和差额结算
// 统一由轮询（service.updateVideoSingleTask）通过 CAS 完成。若在此直接写
// SUCCESS/FAILURE，未完成任务查询会跳过该行，失败任务永远拿不到退款。
func (a *TaskAdaptor) runBackgroundGeneration(publicTaskID, action string, req relaycommon.TaskSubmitReq) {
	ctx := context.Background()

	var task *model.Task
	for i := 0; i < taskInsertWaitAttempts; i++ {
		time.Sleep(taskInsertWaitInterval)
		t, exists, err := model.GetByOnlyTaskId(publicTaskID)
		if err == nil && exists {
			task = t
			break
		}
	}
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("ltx: task %s not persisted, generation skipped", publicTaskID))
		return
	}

	update := map[string]any{}
	videoURL, err := a.generateVideo(ctx, action, req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("ltx: task %s generation failed: %s", publicTaskID, err.Error()))
		update["fail_reason"] = err.Error()
	} else {
		update["video_url"] = videoURL
	}
	if err := model.TaskBulkUpdate([]string{publicTaskID}, update); err != nil {
		logger.LogError(ctx, fmt.Sprintf("ltx: write back result for task %s failed: %s", publicTaskID, err.Error()))
	}
}

// generateVideo 调用 LTX 同步生成接口，并把返回的视频流转存到 S3。
func (a *TaskAdaptor) generateVideo(ctx context.Context, action string, req relaycommon.TaskSubmitReq) (string, error) {
	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return "", err
	}
	jsonBody, err := common.Marshal(body)
	if err != nil {
		return "", err
	}

	path := lo.Ternary(action == constant.TaskActionImageGenerate, "/v1/image-to-video", "/v1/text-to-video")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s%s", a.baseURL, path), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ltx upstream returned %d: %s", resp.StatusCode, string(errBody))
	}

	videoURL, err := service.UploadIOReaderToS3(ctx, resp)
	if err != nil {
		return "", errors.Wrap(err, "upload video to s3 failed")
	}
	if videoURL == "" {
		return "", errors.New("upload video to s3 returned empty url")
	}
	return videoURL, nil
}

//	fetch task status
//
// 发起请求，获取视频数据，然后上传s3，更新tasks数据,最后返回mock的数据(taskId videoUrl)
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	task, exists, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("task not found")
	}
	// 后台生成只写结果字段、不写终态，这里据结果推导对外状态；两者都为空表示仍在生成中。
	var status model.TaskStatus = model.TaskStatusInProgress
	switch {
	case task.VideoUrl != "":
		status = model.TaskStatusSuccess
	case task.FailReason != "":
		status = model.TaskStatusFailure
	}
	return getMockResponse(LtxTaskResponse{
		TaskID:     taskID,
		Status:     string(status),
		VideoURL:   task.VideoUrl,
		FailReason: task.FailReason,
	})
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"ltx-2-fast", "ltx-2-pro", "ltx-2-3-pro", "ltx-2-3-fast"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "ltx"
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*LtxTaskRequest, error) {
	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 5
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}
	r := LtxTaskRequest{
		Prompt:     req.Prompt,
		Model:      req.Model, // Keep consistent with model_name, double writing improves compatibility
		Duration:   seconds,
		ImageURI:   req.Image,
		Resolution: req.Size,
	}
	metadata := req.Metadata
	medaBytes, err := common.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "metadata marshal metadata failed")
	}
	err = common.Unmarshal(medaBytes, &r)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if r.Resolution == "" {
		r.Resolution = "1920x1080"
	}
	return &r, nil
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
	taskInfo := &relaycommon.TaskInfo{}
	resPayload := LtxTaskResponse{}
	err := common.Unmarshal(respBody, &resPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = 0
	taskInfo.TaskID = resPayload.TaskID
	taskInfo.Reason = resPayload.FailReason
	taskInfo.Url = resPayload.VideoURL
	taskInfo.Status = resPayload.Status
	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if originTask.Status == model.TaskStatusSuccess {
		openAIVideo.SetMetadata("url", originTask.VideoUrl)
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
