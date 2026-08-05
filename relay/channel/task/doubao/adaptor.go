package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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

// seedance2MaxSeconds 是 Seedance 2.0 系列的时长上限，用于校验 duration 的取值范围。
const seedance2MaxSeconds = 15

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	// duration=-1 表示由模型自选时长，只有 Seedance 2.0 系列支持。
	// 方舟按 token 计费（见 videoPriceTable），时长不参与倍率，因此这里只做能力校验，
	// 不需要按假定时长预扣、也无需事后按实际时长重算。
	if req.GetSeconds() == -1 && !relaycommon.IsSeedance2Model(req.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s does not support duration=-1 (model-chosen duration)", req.Model),
			"invalid_duration", http.StatusBadRequest)
	}
	if req.GetSeconds() > seedance2MaxSeconds && relaycommon.IsSeedance2Model(req.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s supports at most %d seconds", req.Model, seedance2MaxSeconds),
			"invalid_duration", http.StatusBadRequest)
	}
	return nil
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求 metadata 中的输出分辨率与是否包含视频输入，返回相对基准价的计费 OtherRatio。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	// 参考视频既可能来自 metadata 透传的 content，也可能来自统一的 references。
	hasVideo := hasVideoInMetadata(req.Metadata) || req.HasRefRole(relaycommon.RefRoleReferenceVideo, relaycommon.RefRoleBaseVideo)
	resolution, _ := req.Metadata["resolution"].(string)
	if resolution == "" {
		resolution = req.Resolution
	}
	ratio, ok := GetVideoInputRatio(info.OriginModelName, resolution, hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
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
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	//ov.ID = dResp.ID
	//ov.TaskID = dResp.ID
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
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

// doubaoRoleForRef 把统一 role 映射为方舟 content[].role。
// 返回 false 表示方舟不支持该 role（由统一层能力校验拦截，此处兜底跳过）。
func doubaoRoleForRef(role string) (string, bool) {
	switch role {
	case relaycommon.RefRoleFirstFrame:
		return "first_frame", true
	case relaycommon.RefRoleLastFrame:
		return "last_frame", true
	case relaycommon.RefRoleReferenceImage:
		return "reference_image", true
	case relaycommon.RefRoleReferenceVideo:
		return "reference_video", true
	case relaycommon.RefRoleReferenceAudio:
		return "reference_audio", true
	default:
		return "", false
	}
}

// appendReferenceContents 按素材角色追加 content 条目，保持 references 原始顺序。
func appendReferenceContents(r *requestPayload, req *relaycommon.TaskSubmitReq) {
	for _, ref := range req.References {
		role, ok := doubaoRoleForRef(ref.Role)
		if !ok || ref.URL == "" {
			continue
		}
		switch ref.Type {
		case relaycommon.RefTypeVideo:
			r.Content = append(r.Content, ContentItem{
				Type:     "video_url",
				VideoURL: &MediaURL{URL: ref.URL},
				Role:     role,
			})
		case relaycommon.RefTypeAudio:
			r.Content = append(r.Content, ContentItem{
				Type:     "audio_url",
				AudioURL: &MediaURL{URL: ref.URL},
				Role:     role,
			})
		default:
			r.Content = append(r.Content, ContentItem{
				Type:     "image_url",
				ImageURL: &MediaURL{URL: ref.URL},
				Role:     role,
			})
		}
	}
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}
	isSeedance2 := relaycommon.IsSeedance2Model(req.Model)

	// duration=-1 表示由模型自选时长（仅 Seedance 2.0 系列支持），需原样透传。
	duration := req.GetSeconds()
	if duration > 0 || (duration == -1 && isSeedance2) {
		r.Duration = common.GetPointer(dto.IntValue(duration))
	}

	// Add text prompt
	if req.Prompt != "" {
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: req.Prompt,
		})
	}

	// 统一的 resolution / aspect_ratio 优先于 metadata
	if req.Resolution != "" {
		r.Resolution = strings.ToLower(strings.TrimSpace(req.Resolution))
	}
	if req.AspectRatio != "" {
		r.Ratio = req.AspectRatio
	}
	// audio: 统一层三态语义映射到方舟的 generate_audio 布尔。
	if req.Audio != "" {
		v := dto.BoolValue(!strings.EqualFold(req.Audio, "off"))
		r.GenerateAudio = &v
	}

	// https://www.volcengine.com/docs/82379/1520757?lang=zh
	metadata := req.Metadata
	imageRole := "first_frame"
	if metadata != nil {
		if imageRole1, ok := metadata["image_role"].(string); ok {
			imageRole = imageRole1
		}
		if resolution, ok := metadata["resolution"].(string); ok {
			r.Resolution = resolution
		}
		if ratio, ok := metadata["ratio"].(string); ok {
			r.Ratio = ratio
		}
		if frames, ok := metadata["frames"].(float64); ok {
			r.Frames = common.GetPointer(dto.IntValue(frames))
		}
		if seed, ok := metadata["seed"].(float64); ok {
			r.Seed = common.GetPointer(dto.IntValue(seed))
		}
		if camerafixed, ok := metadata["camera_fixed"].(bool); ok {
			// 文档字段名是 camera_fixed；历史上这里只认 camerafixed，两者都接受。
			v := dto.BoolValue(camerafixed)
			r.CameraFixed = &v
		} else if camerafixed, ok := metadata["camerafixed"].(bool); ok {
			v := dto.BoolValue(camerafixed)
			r.CameraFixed = &v
		}
		if watermark, ok := metadata["watermark"].(bool); ok {
			v := dto.BoolValue(watermark)
			r.Watermark = &v
		}
		if returnLastFrame, ok := metadata["return_last_frame"].(bool); ok {
			v := dto.BoolValue(returnLastFrame)
			r.ReturnLastFrame = &v
		}
		if generateAudio, ok := metadata["generate_audio"].(bool); ok {
			v := dto.BoolValue(generateAudio)
			r.GenerateAudio = &v
		}
	}

	if len(req.References) > 0 {
		// 显式 references：按角色组装，覆盖首帧/首尾帧/多模态参考三种场景。
		appendReferenceContents(&r, req)
	} else if req.HasImage() {
		// 回退路径：保持既有的 image_role 语义。
		imageNum := len(req.Images)
		if imageNum == 2 && imageRole == "first_frame,last_frame" {
			r.Content = append(r.Content, ContentItem{
				Role: "first_frame",
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: req.Images[0],
				},
			})
			r.Content = append(r.Content, ContentItem{
				Role: "last_frame",
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: req.Images[1],
				},
			})
		} else {
			for _, img := range req.Images {
				r.Content = append(r.Content, ContentItem{
					Type: "image_url",
					ImageURL: &MediaURL{
						URL: img,
					},
					Role: imageRole,
				})
			}

		}
	}

	// TODO: Add support for additional parameters from metadata
	// such as ratio, duration, seed, etc.

	//common.PrintJson("requestPayload", r)
	//metadata := req.Metadata
	medaBytes, err := common.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "metadata marshal metadata failed")
	}
	err = common.Unmarshal(medaBytes, &r)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	// Seedance 2.0 系列不支持 seed / camera_fixed / frames，
	// 传了会触发强校验报错，这里统一剔除（放在 metadata 合并之后才能兜住直接透传的情况）。
	if isSeedance2 {
		r.Seed = nil
		r.Frames = nil
		r.CameraFixed = nil
	}

	// // Add images if present
	// if req.HasImage() {
	// 	for _, imgURL := range req.Images {
	// 		r.Content = append(r.Content, ContentItem{
	// 			Type: "image_url",
	// 			ImageURL: &MediaURL{
	// 				URL: imgURL,
	// 			},
	// 		})
	// 	}
	// }

	// metadata := req.Metadata
	// if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
	// 	return nil, errors.Wrap(err, "unmarshal metadata failed")
	// }

	// if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
	// 	r.Duration = lo.ToPtr(dto.IntValue(sec))
	// }

	// r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	// r.Content = append(r.Content, ContentItem{
	// 	Type: "text",
	// 	Text: req.Prompt,
	// })

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		// 实际生成时长：duration=-1 时由模型自选，需回传以便按实际时长重算计费。
		if resTask.Duration > 0 {
			taskResult.ActualSeconds = float64(resTask.Duration)
		}
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if dResp.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: dResp.Error.Message,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
