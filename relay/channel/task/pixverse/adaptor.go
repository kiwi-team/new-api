package pixverse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// https://docs.platform.pixverse.ai/model-pricing-796039m0
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
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if strings.Contains(info.UpstreamModelName, "t2v") {
		//https://app-api.pixverse.ai/openapi/v2/video/text/generate
		return fmt.Sprintf("%s%s", a.baseURL, "/openapi/v2/video/text/generate"), nil
	} else if strings.Contains(info.UpstreamModelName, "i2v") {
		//https://app-api.pixverse.ai/openapi/v2/video/img/generate
		return fmt.Sprintf("%s%s", a.baseURL, "/openapi/v2/video/img/generate"), nil
	} else if strings.Contains(info.UpstreamModelName, "transition") {
		// https://app-api.pixverse.ai/openapi/v2/video/transition/generate
		return fmt.Sprintf("%s%s", a.baseURL, "/openapi/v2/video/transition/generate"), nil
	}
	return "", fmt.Errorf("unsupported model: %s", info.UpstreamModelName)
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("API-KEY", info.ApiKey)
	traceId := c.GetString(common.RequestIdKey)
	req.Header.Set("Ai-Trace-Id", traceId)
	return nil
}

// BuildRequestBody converts request into Kling specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(info, &req)
	if err != nil {
		return nil, err
	}

	if body.ImgID == 0 && len(body.ImgIDS) == 0 && strings.Contains(info.UpstreamModelName, "t2v") {
		c.Set("action", constant.TaskActionTextGenerate)
	} else if body.ImgID > 0 && strings.Contains(info.UpstreamModelName, "i2v") {
		c.Set("action", constant.TaskActionImageGenerate)
	} else if len(body.ImgIDS) == 2 && strings.Contains(info.UpstreamModelName, "transition") {
		c.Set("action", constant.TaskActionFirstTailGenerate)
	} else {
		return nil, fmt.Errorf("params error for model: %s,image count %v", info.UpstreamModelName, len(body.ImgIDS))
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
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var taskResp ToVideoResponse
	err = json.Unmarshal(responseBody, &taskResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	if taskResp.ErrCode != 0 {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", taskResp.ErrMsg), "task_failed", http.StatusBadRequest)
		return
	}
	ov := dto.NewOpenAIVideo()
	videoID := strconv.FormatInt(taskResp.Resp.VideoID, 10)
	ov.ID = videoID
	ov.TaskID = ov.ID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return videoID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	url := fmt.Sprintf("%s%s%s", baseUrl, "/openapi/v2/video/result/", taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("API-KEY", key)
	req.Header.Set("Ai-Trace-Id", common.GetUUID())

	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"pixverse-v5.5-t2v", "pixverse-v5.5-i2v", "pixverse-v5.5-transition"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "pixverse"
}

// ============================
// helpers
// ============================
/*
curl --location --request POST 'https://app-api.pixverse.ai/openapi/v2/image/upload' \
--header 'API-KEY: your-api-key' \
--header 'Ai-trace-id: {{$string.uuid}}' \
--form 'image=@""' \
--form 'image_url="https://media.pixverse.ai/openapi%2Ff4c512d1-0110-4360-8515-d84d788ca8d1test_image_auto.jpg"'
*/

func uploadFile(info *relaycommon.RelayInfo, imageUrl string) (int64, error) {
	//https://app-api.pixverse.ai/openapi/v2/image/upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("image_url", imageUrl); err != nil {
		return 0, err
	}
	// curl example shows image=@"", simulating empty file
	if _, err := writer.CreateFormFile("image", ""); err != nil {
		return 0, err
	}

	if err := writer.Close(); err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodPost, info.ChannelBaseUrl+"/openapi/v2/image/upload", body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("API-KEY", info.ApiKey)
	req.Header.Set("Ai-Trace-Id", common.GetUUID())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return 0, err
	}

	if uploadResp.ErrCode != 0 {
		return 0, fmt.Errorf("upload failed: %s", uploadResp.ErrMsg)
	}

	return uploadResp.Resp.ImgID, nil
}

func (a *TaskAdaptor) convertToRequestPayload(info *relaycommon.RelayInfo, req *relaycommon.TaskSubmitReq) (*ToVideoRequest, error) {
	r := ToVideoRequest{
		Prompt: req.Prompt,
	}
	// pixverse-v5.5-t2v
	// pixverse-v5.5-i2v
	// pixverse-v5.5-transition
	arr := strings.Split(req.Model, "-")
	if len(arr) != 3 {
		return nil, fmt.Errorf("invalid model: %s", req.Model)
	}
	r.Model = arr[1]

	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 4
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}
	r.Duration = int64(seconds)

	var imgIDs []int64
	imgIDs = make([]int64, 0)
	for _, imgUrl := range req.Images {
		imgID, err := uploadFile(info, imgUrl)
		if err != nil {
			return nil, fmt.Errorf("upload image failed: %w", err)
		}
		imgIDs = append(imgIDs, imgID)
	}
	if len(imgIDs) == 1 {
		r.ImgID = imgIDs[0]
	} else {
		r.ImgIDS = imgIDs
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
	return &r, nil
}

// ============================
// JWT helpers
// ============================

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}
	resPayload := VideoTaskResult{}
	err := json.Unmarshal(respBody, &resPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = resPayload.ErrCode
	taskInfo.TaskID = strconv.FormatInt(resPayload.Resp.ID, 10)
	taskInfo.Reason = resPayload.ErrMsg
	//任务状态，枚举值：submitted（已提交）、processing（处理中）、succeed（成功）、failed（失败）
	status := resPayload.Resp.Status
	switch status {
	case VideoStatusGenerationSuccessful:
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Url = resPayload.Resp.URL
		taskInfo.CompletionTokens = resPayload.Resp.Credits
		taskInfo.TotalTokens = resPayload.Resp.Credits
	case VideoStatusGenerating:
		taskInfo.Status = model.TaskStatusInProgress
	case VideoStatusDeleted:
		taskInfo.Status = model.TaskStatusDeleted
	case VideoStatusContentsModerationFailed:
		taskInfo.Status = model.TaskStatusFailure
	case VideoStatusGenerationFailed:
		taskInfo.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %v", status)
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var resp VideoTaskResult
	if err := json.Unmarshal(originTask.Data, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal kling task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = resp.Resp.CreateTime.Unix()
	openAIVideo.CompletedAt = resp.Resp.ModifyTime.Unix()

	if resp.ErrCode != 0 && resp.ErrMsg != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: resp.ErrMsg,
			Code:    fmt.Sprintf("%d", resp.ErrCode),
		}
	}

	openAIVideo.SetMetadata("url", resp.Resp.URL)
	openAIVideo.Seconds = strconv.Itoa(resp.Resp.Size)

	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}
