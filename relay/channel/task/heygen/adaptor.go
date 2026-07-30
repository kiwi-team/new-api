package heygen

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

const (
	heygenDefaultBaseURL = "https://api.heygen.com"
	ChannelName          = "heygen"
)

var ModelList = []string{"heygen-image-to-video"}

// ============================
// Request / Response structures
// ============================

type imageSource struct {
	Type string `json:"type"` // "url"
	URL  string `json:"url"`
}

type createVideoReq struct {
	Type        string      `json:"type"` // always "image"
	Image       imageSource `json:"image"`
	AudioURL    string      `json:"audio_url,omitempty"`
	Title       string      `json:"title,omitempty"`
	Resolution  string      `json:"resolution,omitempty"`
	AspectRatio string      `json:"aspect_ratio,omitempty"`
}

type createVideoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		VideoID string `json:"video_id"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type videoStatusResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		VideoID  string `json:"video_id"`
		Status   string `json:"status"` // pending, processing, completed, failed
		VideoURL string `json:"video_url,omitempty"`
		GifURL   string `json:"gif_url,omitempty"`
		Error    string `json:"error,omitempty"`
	} `json:"data"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	proxy       string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	if a.baseURL == "" {
		a.baseURL = heygenDefaultBaseURL
	}
	a.apiKey = info.ApiKey
	if info.ChannelMeta != nil {
		a.proxy = info.ChannelMeta.ChannelSetting.Proxy
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v3/videos", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	// Image URL: prefer Images[0] over Image
	imageURL := taskReq.Image
	if len(taskReq.Images) > 0 {
		imageURL = taskReq.Images[0]
	}
	if imageURL == "" {
		return nil, fmt.Errorf("image URL is required")
	}

	// Audio URL from metadata.audio
	audioURL := ""
	if taskReq.Metadata != nil {
		if v, ok := taskReq.Metadata["audio"].(string); ok {
			audioURL = v
		}
	}
	if audioURL == "" {
		return nil, fmt.Errorf("audio URL is required (pass as metadata.audio)")
	}

	videoReq := createVideoReq{
		Type: "image",
		Image: imageSource{
			Type: "url",
			URL:  imageURL,
		},
		AudioURL: audioURL,
	}

	if taskReq.Metadata != nil {
		if v, ok := taskReq.Metadata["title"].(string); ok && v != "" {
			videoReq.Title = v
		}
		if v, ok := taskReq.Metadata["resolution"].(string); ok && v != "" {
			videoReq.Resolution = v
		}
		if v, ok := taskReq.Metadata["aspect_ratio"].(string); ok && v != "" {
			videoReq.AspectRatio = v
		}
	}
	data, err := common.Marshal(videoReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal create video request")
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var cvResp createVideoResp
	if err := common.Unmarshal(responseBody, &cvResp); err != nil {
		taskErr = service.TaskErrorWrapper(
			errors.Wrapf(err, "body: %s", responseBody),
			"unmarshal_response_failed",
			http.StatusInternalServerError,
		)
		return
	}

	if cvResp.Error != nil {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("heygen error %s: %s", cvResp.Error.Code, cvResp.Error.Message),
			"upstream_error",
			http.StatusBadGateway,
		)
		return
	}

	if cvResp.Data.VideoID == "" {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("video_id is empty, response: %s", string(responseBody)),
			"invalid_response",
			http.StatusInternalServerError,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = cvResp.Data.VideoID
	ov.TaskID = cvResp.Data.VideoID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return cvResp.Data.VideoID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	if baseUrl == "" {
		baseUrl = heygenDefaultBaseURL
	}

	uri := fmt.Sprintf("%s/v3/videos/%s", baseUrl, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new http client: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var statusResp videoStatusResp
	if err := common.Unmarshal(respBody, &statusResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result")
	}

	ti := &relaycommon.TaskInfo{}
	switch statusResp.Data.Status {
	case "processing":
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
	case "completed":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		ti.Url = statusResp.Data.VideoURL
	case "failed":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		ti.Reason = statusResp.Data.Error
	default:
		// pending or unknown
		ti.Status = model.TaskStatusSubmitted
		ti.Progress = "5%"
	}
	return ti, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	ov := dto.NewOpenAIVideo()
	ov.ID = originTask.TaskID
	ov.TaskID = originTask.TaskID
	ov.Status = originTask.Status.ToVideoStatus()
	ov.SetProgressStr(originTask.Progress)
	ov.CreatedAt = originTask.CreatedAt
	ov.CompletedAt = originTask.UpdatedAt
	ov.Model = originTask.Properties.OriginModelName

	if originTask.Status == model.TaskStatusSuccess && originTask.FailReason != "" {
		ov.SetMetadata("url", originTask.FailReason)
	}

	jsonData, _ := common.Marshal(ov)
	return jsonData, nil
}
