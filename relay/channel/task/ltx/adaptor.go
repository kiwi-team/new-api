package ltx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// https://docs.ltx.video/api-documentation/api-reference/video-generation/image-to-video
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
		seconds = 6
	}
	if taskReq.Duration > 0 {
		seconds = taskReq.Duration
	}
	taskType := "t2v"
	if taskReq.HasImage() {
		taskType = "i2v"
	}
	size := taskReq.Size
	if len(size) == 0 {
		size = "1920x1080"
	}
	info.PriceData.ModelPrice = getModelPrice(taskType, taskReq.Model, size)
	if info.PriceData.ModelPrice == 0 {
		return service.TaskErrorWrapper(errors.New("model price not found"), "model_price_not_found", http.StatusBadRequest)
	}
	info.PriceData.OtherRatios = map[string]float64{
		"seconds": float64(seconds),
	}

	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
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
		},
	}
	if priceMap[taskType][modelName] == nil {
		return 0
	}
	if priceMap[taskType][modelName][Resolution] == 0 {
		return 0
	}
	return priceMap[taskType][modelName][Resolution]
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

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	//return channel.DoTaskApiRequest(a, c, info, requestBody)
	taskId := common.GetRandomString(32)
	mockResp := getMockResponse(LtxTaskResponse{
		TaskID: taskId,
		Status: string(model.TaskStatusSubmitted),
	})

	return mockResp, nil
}

func getMockResponse(resp LtxTaskResponse) *http.Response {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBuffer(data)),
	}
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var kResp LtxTaskResponse
	err = json.Unmarshal(responseBody, &kResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	ov := dto.NewOpenAIVideo()
	ov.ID = kResp.TaskID
	ov.TaskID = kResp.TaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	go func() {
		time.Sleep(2 * time.Second)
		task, exists, err := model.GetByOnlyTaskId(kResp.TaskID)
		if err != nil || !exists {
			return
		}
		request := task.Request
		var reqBody relaycommon.TaskSubmitReq
		err = json.Unmarshal([]byte(request), &reqBody)
		if err != nil {
			return
		}
		body, err := a.convertToRequestPayload(&reqBody)
		if err != nil {
			return
		}
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return
		}
		action := c.GetString("action")
		path := lo.Ternary(action == constant.TaskActionImageGenerate, "/v1/image-to-video", "/v1/text-to-video")
		url := fmt.Sprintf("%s%s", a.baseURL, path)
		httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
		if err != nil {
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

		client := service.GetHttpClient()
		resp, err := client.Do(httpReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			if err != nil {
				task.FailReason = err.Error()
			} else {
				body, _ := io.ReadAll(resp.Body)
				task.FailReason = string(body)
			}
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
			task.FinishTime = time.Now().Unix()
			task.Update()
			return
		}
		videoUrl, err := service.UploadIOReaderToS3(context.Background(), resp)
		if err != nil || videoUrl == "" {
			task.FailReason = err.Error()
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
		} else {
			task.FailReason = videoUrl
			task.Status = model.TaskStatusSuccess
			task.VideoUrl = videoUrl
			task.Progress = "100%"
		}
		task.FinishTime = time.Now().Unix()
		task.Update()
	}()
	return kResp.TaskID, responseBody, nil
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
	mockResp := getMockResponse(LtxTaskResponse{
		TaskID:     taskID,
		Status:     string(task.Status),
		VideoURL:   task.VideoUrl,
		FailReason: task.FailReason,
	})
	return mockResp, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"ltx-2-fast", "ltx-2-pro"}
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
	medaBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "metadata marshal metadata failed")
	}
	err = json.Unmarshal(medaBytes, &r)
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
	err := json.Unmarshal(respBody, &resPayload)
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
