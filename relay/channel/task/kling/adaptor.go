package kling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// ============================
// Request / Response structures
// ============================

type TrajectoryPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type DynamicMask struct {
	Mask         string            `json:"mask,omitempty"`
	Trajectories []TrajectoryPoint `json:"trajectories,omitempty"`
}

type CameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty"`
	Vertical   float64 `json:"vertical,omitempty"`
	Pan        float64 `json:"pan,omitempty"`
	Tilt       float64 `json:"tilt,omitempty"`
	Roll       float64 `json:"roll,omitempty"`
	Zoom       float64 `json:"zoom,omitempty"`
}

type CameraControl struct {
	Type   string        `json:"type,omitempty"`
	Config *CameraConfig `json:"config,omitempty"`
}

type requestPayload struct {
	Prompt         string         `json:"prompt,omitempty"`
	Image          string         `json:"image,omitempty"`
	ImageTail      string         `json:"image_tail,omitempty"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Duration       string         `json:"duration,omitempty"`
	AspectRatio    string         `json:"aspect_ratio,omitempty"`
	ModelName      string         `json:"model_name,omitempty"`
	Model          string         `json:"model,omitempty"` // Compatible with upstreams that only recognize "model"
	CfgScale       float64        `json:"cfg_scale,omitempty"`
	StaticMask     string         `json:"static_mask,omitempty"`
	DynamicMasks   []DynamicMask  `json:"dynamic_masks,omitempty"`
	CameraControl  *CameraControl `json:"camera_control,omitempty"`
	CallbackUrl    string         `json:"callback_url,omitempty"`
	ExternalTaskId string         `json:"external_task_id,omitempty"`
	Sound          string         `json:"sound,omitempty"`
}

type avatarRequestPayload struct {
	Prompt    string `json:"prompt,omitempty"`
	Image     string `json:"image,omitempty"`
	SoundFile string `json:"sound_file,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

type responsePayload struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	TaskId    string `json:"task_id"`
	RequestId string `json:"request_id"`
	Data      struct {
		TaskId        string `json:"task_id"`
		TaskStatus    string `json:"task_status"`
		TaskStatusMsg string `json:"task_status_msg"`
		TaskResult    struct {
			Videos []struct {
				Id       string `json:"id"`
				Url      string `json:"url"`
				Duration string `json:"duration"`
			} `json:"videos"`
		} `json:"task_result"`
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
	} `json:"data"`
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

	// apiKey 支持两种形式（见 createJWTTokenWithKey）：
	//   - "<API Key>"                 新版，原样作为 Bearer token
	//   - "<access_key>|<secret_key>" 旧版，需签发 JWT
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	// 可灵按时长 × 清晰度计价，需在定价前写入倍率（此处仍在 ValidateRequestAndSetAction）。
	if relaycommon.IsKlingOmniModel(req.Model) {
		applyOmniPriceRatios(info, &req)
	}
	return nil
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Omni 系列走独立端点：POST {base}/omni-video/{model}
	if relaycommon.IsKlingOmniModel(info.UpstreamModelName) || relaycommon.IsKlingOmniModel(info.OriginModelName) {
		modelName := info.UpstreamModelName
		if modelName == "" {
			modelName = info.OriginModelName
		}
		if isNewAPIRelay(info.ApiKey) {
			return fmt.Sprintf("%s/kling/omni-video/%s", a.baseURL, modelName), nil
		}
		return fmt.Sprintf("%s/omni-video/%s", a.baseURL, modelName), nil
	}

	if info.UpstreamModelName == "klingai_avatar" {
		return fmt.Sprintf("%s%s", a.baseURL, "/v1/videos/avatar/image2video"), nil
	}

	path := lo.Ternary(info.Action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")
	if isNewAPIRelay(info.ApiKey) {
		return fmt.Sprintf("%s/kling%s", a.baseURL, path), nil
	}

	return fmt.Sprintf("%s%s", a.baseURL, path), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	token, err := a.createJWTToken()
	if err != nil {
		return fmt.Errorf("failed to create JWT token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")
	return nil
}

// BuildRequestBody converts request into Kling specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// Omni 系列使用 contents/settings/options 三段式请求体。
	if relaycommon.IsKlingOmniModel(req.Model) {
		payload, err := buildOmniRequestPayload(&req)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	//if strings.Contains(req.Model, "avatar") {
	if req.Model == "klingai_avatar" {
		body, err := a.convertToAvatarRequestPayload(&req)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	} else {
		body, err := a.convertToRequestPayload(&req)
		if err != nil {
			return nil, err
		}
		if body.Image == "" && body.ImageTail == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
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

	var kResp responsePayload
	err = json.Unmarshal(responseBody, &kResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	if kResp.Code != 0 {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", kResp.Message), "task_failed", http.StatusBadRequest)
		return
	}
	// Omni 的任务 ID 在 data.id（旧接口在 data.task_id），两者结构不同需分别解析。
	taskId := kResp.Data.TaskId
	if relaycommon.IsKlingOmniModel(info.OriginModelName) {
		id, err := parseOmniSubmitResponse(responseBody)
		if err != nil {
			taskErr = service.TaskErrorWrapperLocal(err, "task_failed", http.StatusBadRequest)
			return
		}
		taskId = id
	}
	ov := dto.NewOpenAIVideo()
	ov.ID = taskId
	ov.TaskID = taskId
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return taskId, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var url string
	// Omni 系列统一走 GET /tasks?task_ids={id}，与旧接口的 /v1/videos/... 完全不同。
	if modelName, _ := body["model"].(string); relaycommon.IsKlingOmniModel(modelName) {
		if isNewAPIRelay(key) {
			url = fmt.Sprintf("%s/kling/tasks?task_ids=%s", baseUrl, taskID)
		} else {
			url = fmt.Sprintf("%s/tasks?task_ids=%s", baseUrl, taskID)
		}
	} else {
		action, ok := body["action"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid action")
		}
		path := lo.Ternary(action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")
		url = fmt.Sprintf("%s%s/%s", baseUrl, path, taskID)
		if isNewAPIRelay(key) {
			url = fmt.Sprintf("%s/kling%s/%s", baseUrl, path, taskID)
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	token, err := a.createJWTTokenWithKey(key)
	if err != nil {
		// 走到这里说明 key 配置有误（如 "accessKey|" 缺少 secretKey）。
		// 此前这里静默回退成裸 key，会把配置错误伪装成上游 401，难以排查。
		return nil, fmt.Errorf("failed to build kling credential: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"kling-v1", "kling-v1-6", "kling-v2-master", "kling-3.0-omni", "kling-o1"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "kling"
}

// ============================
// helpers
// ============================
func (a *TaskAdaptor) convertToAvatarRequestPayload(req *relaycommon.TaskSubmitReq) (*avatarRequestPayload, error) {
	r := avatarRequestPayload{
		Prompt: req.Prompt,
		Image:  req.Image,
		Mode:   defaultString(req.Mode, "std"),
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

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 5
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}
	r := requestPayload{
		Prompt:         req.Prompt,
		Image:          req.Image,
		Mode:           defaultString(req.Mode, "std"),
		Duration:       fmt.Sprintf("%d", defaultInt(seconds, 5)),
		AspectRatio:    a.getAspectRatio(req.Size),
		ModelName:      req.Model,
		Model:          req.Model, // Keep consistent with model_name, double writing improves compatibility
		CfgScale:       0.5,
		StaticMask:     "",
		DynamicMasks:   []DynamicMask{},
		CameraControl:  nil,
		CallbackUrl:    "",
		ExternalTaskId: "",
	}
	if r.ModelName == "" {
		r.ModelName = "kling-v1"
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

func defaultString(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func defaultInt(v int, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// ============================
// JWT helpers
// ============================

func (a *TaskAdaptor) createJWTToken() (string, error) {
	return a.createJWTTokenWithKey(a.apiKey)
}

// createJWTTokenWithKey 生成 Authorization 头里 "Bearer " 之后的凭证。
//
// 可灵支持两种鉴权方式，均需保留：
//  1. API Key（新版，推荐）：控制台直接生成的密钥，原样作为 Bearer token 使用，
//     不做 JWT 签名。特征是不含 "|" 分隔符。
//  2. Access Key / Secret Key（旧版）：以 "accessKey|secretKey" 形式配置，
//     需用 secretKey 对 accessKey 做 HS256 JWT 签名，有效期 30 分钟。
//
// new-api 级联（sk- 前缀）同样属于第 1 类，原样透传。
func (a *TaskAdaptor) createJWTTokenWithKey(apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", errors.New("api_key is required")
	}

	// 不含 "|" 即为新版 API Key（或 new-api 级联 key），直接作为 Bearer token。
	if !strings.Contains(apiKey, "|") {
		return apiKey, nil
	}

	// 旧版 AK|SK：用 secretKey 签发 JWT。
	keyParts := strings.SplitN(apiKey, "|", 2)
	accessKey := strings.TrimSpace(keyParts[0])
	secretKey := strings.TrimSpace(keyParts[1])
	if accessKey == "" || secretKey == "" {
		return "", errors.New("invalid api_key, required format is accessKey|secretKey (or a single API Key)")
	}
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iss": accessKey,
		"exp": now + 1800, // 30 minutes
		"nbf": now - 5,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "JWT"
	return token.SignedString([]byte(secretKey))
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// Omni 的查询响应结构不同（data.result[].outputs[]），按结构探测后分流。
	if isOmniQueryResponse(respBody) {
		return parseOmniTaskResult(respBody)
	}

	taskInfo := &relaycommon.TaskInfo{}
	resPayload := responsePayload{}
	err := json.Unmarshal(respBody, &resPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = resPayload.Code
	taskInfo.TaskID = resPayload.Data.TaskId
	taskInfo.Reason = resPayload.Data.TaskStatusMsg
	//任务状态，枚举值：submitted（已提交）、processing（处理中）、succeed（成功）、failed（失败）
	status := resPayload.Data.TaskStatus
	switch status {
	case "submitted":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "succeed":
		taskInfo.Status = model.TaskStatusSuccess
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %s", status)
	}
	if videos := resPayload.Data.TaskResult.Videos; len(videos) > 0 {
		video := videos[0]
		taskInfo.Url = video.Url
	}
	return taskInfo, nil
}

// isNewAPIRelay 判断上游是否为另一个 new-api 实例（级联中转），
// 此时请求路径需要加 /kling 前缀。仅用于 URL 路由，不参与鉴权判断：
// 鉴权见 createJWTTokenWithKey（按是否含 "|" 区分 API Key 与 AK|SK）。
func isNewAPIRelay(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-")
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	// Omni 的任务数据结构不同，单独处理。
	if isOmniQueryResponse(originTask.Data) {
		openAIVideo := dto.NewOpenAIVideo()
		openAIVideo.ID = originTask.TaskID
		openAIVideo.TaskID = originTask.TaskID
		openAIVideo.Status = originTask.Status.ToVideoStatus()
		openAIVideo.SetProgressStr(originTask.Progress)
		openAIVideo.CreatedAt = originTask.CreatedAt
		openAIVideo.CompletedAt = originTask.UpdatedAt
		openAIVideo.Model = originTask.Properties.OriginModelName
		if out, ok := extractOmniVideoOutput(originTask.Data); ok {
			if out.URL != "" {
				openAIVideo.SetMetadata("url", out.URL)
			}
			if out.WatermarkURL != "" {
				openAIVideo.SetMetadata("watermark_url", out.WatermarkURL)
			}
			if out.Duration != "" {
				openAIVideo.Seconds = out.Duration
			}
		}
		if originTask.Status == model.TaskStatusFailure {
			msg := originTask.FailReason
			if msg == "" {
				msg = "task failed"
			}
			openAIVideo.Error = &dto.OpenAIVideoError{Message: msg, Code: "task_failed"}
		}
		return common.Marshal(openAIVideo)
	}

	var klingResp responsePayload
	if err := json.Unmarshal(originTask.Data, &klingResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal kling task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = klingResp.Data.CreatedAt
	openAIVideo.CompletedAt = klingResp.Data.UpdatedAt

	if len(klingResp.Data.TaskResult.Videos) > 0 {
		video := klingResp.Data.TaskResult.Videos[0]
		if video.Url != "" {
			openAIVideo.SetMetadata("url", video.Url)
		}
		if video.Duration != "" {
			openAIVideo.Seconds = video.Duration
		}
	}

	if klingResp.Code != 0 && klingResp.Message != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: klingResp.Message,
			Code:    fmt.Sprintf("%d", klingResp.Code),
		}
	} else if originTask.Status == model.TaskStatusFailure || klingResp.Data.TaskStatus == "failed" {
		msg := klingResp.Data.TaskStatusMsg
		if msg == "" {
			msg = originTask.FailReason
		}
		if msg == "" {
			msg = "task failed"
		}
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: msg,
			Code:    "task_failed",
		}
	}
	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}
