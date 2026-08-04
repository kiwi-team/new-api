package minimax

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

// MediaURL 是 image_url / video_url / audio_url 的公共结构。
type MediaURL struct {
	URL string `json:"url"`
}

// ContentItem 是 content[] 的一项。
type ContentItem struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type requestPayload struct {
	Model         string        `json:"model"`
	Content       []ContentItem `json:"content"`
	Resolution    string        `json:"resolution"`
	Duration      int           `json:"duration"`
	Ratio         string        `json:"ratio,omitempty"`
	CallbackURL   string        `json:"callback_url,omitempty"`
	AigcWatermark *bool         `json:"aigc_watermark,omitempty"`
}

// submitResponse 提交任务的响应（v2 只有 task_id，没有 base_resp）。
type submitResponse struct {
	TaskID string `json:"task_id"`
}

// oaiError 是 v2 的错误结构（OpenAI 风格）。
type oaiError struct {
	Type  string `json:"type"`
	Error struct {
		Type     string `json:"type"`
		Message  string `json:"message"`
		HTTPCode string `json:"http_code"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// queryResponse 查询任务的响应。
type queryResponse struct {
	Task struct {
		ID        string `json:"id"`
		Model     string `json:"model"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
		Content   struct {
			URL string `json:"url"`
		} `json:"content"`
		Resolution string `json:"resolution"`
		Duration   int    `json:"duration"`
		Ratio      string `json:"ratio"`
		TaskType   string `json:"task_type"`
		Usage      struct {
			TotalSeconds    float64 `json:"total_seconds"`
			InputSeconds    float64 `json:"input_seconds"`
			OutputSeconds   float64 `json:"output_seconds"`
			InputImageCount int     `json:"input_image_count"`
		} `json:"usage"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"task"`
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	if err := validateRequest(&req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling 按秒计费：时长作为倍率参与配额计算。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	return map[string]float64{"seconds": float64(resolveDuration(&req))}
}

// validateRequest 施加官方约束，不支持的组合直接报错（而非静默丢弃）。
func validateRequest(req *relaycommon.TaskSubmitReq) error {
	if strings.TrimSpace(req.Prompt) == "" {
		// 官方：每种场景都必须带一条非空 text。
		return errors.New("prompt is required for MiniMax video generation")
	}
	if d := resolveDuration(req); d < MinDuration || d > MaxDuration {
		return fmt.Errorf("duration must be between %d and %d seconds, got %d", MinDuration, MaxDuration, d)
	}

	frames := req.CountRefsByRole(relaycommon.RefRoleFirstFrame, relaycommon.RefRoleLastFrame)
	refImgs := req.CountRefsByRole(relaycommon.RefRoleReferenceImage)
	refVids := req.CountRefsByRole(relaycommon.RefRoleReferenceVideo)
	refAuds := req.CountRefsByRole(relaycommon.RefRoleReferenceAudio)

	// 首尾帧与参考素材互斥
	if frames > 0 && (refImgs > 0 || refVids > 0 || refAuds > 0) {
		return errors.New("first_frame/last_frame cannot be combined with reference materials; choose one mode")
	}
	// 音频不能单独输入
	if refAuds > 0 && refImgs == 0 && refVids == 0 {
		return errors.New("reference_audio requires at least one reference_image or reference_video")
	}
	if n := req.CountRefsByRole(relaycommon.RefRoleFirstFrame); n > MaxFirstFrame {
		return fmt.Errorf("at most %d first_frame allowed, got %d", MaxFirstFrame, n)
	}
	if n := req.CountRefsByRole(relaycommon.RefRoleLastFrame); n > MaxLastFrame {
		return fmt.Errorf("at most %d last_frame allowed, got %d", MaxLastFrame, n)
	}
	if refImgs > MaxReferenceImages {
		return fmt.Errorf("at most %d reference images allowed, got %d", MaxReferenceImages, refImgs)
	}
	if refVids > MaxReferenceVideos {
		return fmt.Errorf("at most %d reference videos allowed, got %d", MaxReferenceVideos, refVids)
	}
	if refAuds > MaxReferenceAudios {
		return fmt.Errorf("at most %d reference audios allowed, got %d", MaxReferenceAudios, refAuds)
	}
	if total := frames + refImgs + refVids + refAuds; total > MaxTotalFiles {
		return fmt.Errorf("at most %d media files allowed in total, got %d", MaxTotalFiles, total)
	}

	// ratio 校验：文生视频场景必填且不能为 adaptive。
	ratio := resolveRatio(req)
	if ratio != "" && !common.StringsContains(ValidRatios, ratio) {
		return fmt.Errorf("invalid ratio %q, expected one of %s", ratio, strings.Join(ValidRatios, ", "))
	}
	if frames == 0 && refImgs == 0 && refVids == 0 && refAuds == 0 {
		if ratio == "adaptive" {
			return errors.New("text-to-video requires an explicit ratio; adaptive is not allowed")
		}
	}
	return nil
}

func resolveDuration(req *relaycommon.TaskSubmitReq) int {
	if d := req.GetSeconds(); d > 0 {
		return d
	}
	return DefaultDuration
}

// resolveRatio 取统一的 aspect_ratio，其次 metadata.ratio。
func resolveRatio(req *relaycommon.TaskSubmitReq) string {
	if r := strings.TrimSpace(req.AspectRatio); r != "" {
		return r
	}
	if req.Metadata != nil {
		if v, ok := req.Metadata["ratio"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s%s", a.baseURL, SubmitEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	payload, err := buildRequestPayload(&req)
	if err != nil {
		return nil, err
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// roleForRef 把统一 role 映射为 MiniMax content[].role。
func roleForRef(role string) (string, bool) {
	switch role {
	case relaycommon.RefRoleFirstFrame:
		return RoleFirstFrame, true
	case relaycommon.RefRoleLastFrame:
		return RoleLastFrame, true
	case relaycommon.RefRoleReferenceImage:
		return RoleReferenceImage, true
	case relaycommon.RefRoleReferenceVideo:
		return RoleReferenceVideo, true
	case relaycommon.RefRoleReferenceAudio:
		return RoleReferenceAudio, true
	default:
		return "", false
	}
}

// buildRequestPayload 由统一请求构造 v2 的多模态 content[]。
// 顺序保持与 references 一致，text 固定排在首位。
func buildRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	p := &requestPayload{
		Model:      req.Model,
		Content:    make([]ContentItem, 0, len(req.References)+1),
		Resolution: DefaultResolution,
		Duration:   resolveDuration(req),
	}
	// text 必填，排首位
	p.Content = append(p.Content, ContentItem{Type: ContentTypeText, Text: req.Prompt})

	if len(req.References) > 0 {
		for _, ref := range req.References {
			role, ok := roleForRef(ref.Role)
			if !ok || strings.TrimSpace(ref.URL) == "" {
				continue
			}
			item := ContentItem{Role: role}
			switch ref.Type {
			case relaycommon.RefTypeVideo:
				item.Type = ContentTypeVideoURL
				item.VideoURL = &MediaURL{URL: ref.URL}
			case relaycommon.RefTypeAudio:
				item.Type = ContentTypeAudioURL
				item.AudioURL = &MediaURL{URL: ref.URL}
			default:
				item.Type = ContentTypeImageURL
				item.ImageURL = &MediaURL{URL: ref.URL}
			}
			p.Content = append(p.Content, item)
		}
	} else if req.HasImage() {
		// 回退路径：单图为首帧，双图为首尾帧。
		for i, img := range req.Images {
			role := RoleFirstFrame
			if i == 1 {
				role = RoleLastFrame
			} else if i > 1 {
				break // v2 首尾帧最多两张
			}
			p.Content = append(p.Content, ContentItem{
				Type:     ContentTypeImageURL,
				ImageURL: &MediaURL{URL: img},
				Role:     role,
			})
		}
	}

	// resolution 目前只有 2K，但仍允许显式覆盖以便官方后续扩展。
	if r := strings.TrimSpace(req.Resolution); r != "" {
		p.Resolution = strings.ToUpper(r)
	}

	ratio := resolveRatio(req)
	hasAnyMedia := len(req.References) > 0 || req.HasImage()
	switch {
	case ratio != "":
		p.Ratio = ratio
	case hasAnyMedia:
		// 图生视频由输入图片决定宽高比；参考生视频官方默认 adaptive。
		// 两种情况都不代客户端指定 ratio，交给上游用默认值。
	default:
		// 纯文生视频：官方要求 ratio 必填且不能 adaptive，补默认值。
		p.Ratio = DefaultRatio
	}

	if req.Metadata != nil {
		if v, ok := req.Metadata["callback_url"].(string); ok && v != "" {
			p.CallbackURL = v
		}
		if v, ok := req.Metadata["aigc_watermark"].(bool); ok {
			p.AigcWatermark = &v
		}
		if v, ok := req.Metadata["resolution"].(string); ok && v != "" {
			p.Resolution = strings.ToUpper(v)
		}
	}
	return p, nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// v2 用 OpenAI 风格错误体，没有 base_resp。
	if msg, ok := parseOaiError(responseBody); ok {
		taskErr = service.TaskErrorWrapperLocal(errors.New(msg), "minimax_error", http.StatusBadRequest)
		return
	}

	var sResp submitResponse
	if err := common.Unmarshal(responseBody, &sResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(sResp.TaskID) == "" {
		taskErr = service.TaskErrorWrapper(errors.New("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = sResp.TaskID
	ov.TaskID = sResp.TaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return sResp.TaskID, responseBody, nil
}

// parseOaiError 尝试把响应体解析成 v2 的错误结构。
func parseOaiError(body []byte) (string, bool) {
	var e oaiError
	if err := common.Unmarshal(body, &e); err != nil {
		return "", false
	}
	if e.Type != "error" || e.Error.Message == "" {
		return "", false
	}
	if e.Error.Type != "" {
		return fmt.Sprintf("%s: %s", e.Error.Type, e.Error.Message), true
	}
	return e.Error.Message, true
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	// v2 用 path 参数（v1 是 ?task_id=）
	uri := fmt.Sprintf("%s%s/%s", baseUrl, QueryEndpoint, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	if msg, ok := parseOaiError(respBody); ok {
		return nil, errors.New(msg)
	}
	var q queryResponse
	if err := common.Unmarshal(respBody, &q); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	t := q.Task

	ti := &relaycommon.TaskInfo{TaskID: t.ID}
	switch t.Status {
	case TaskStatusQueued:
		ti.Status = model.TaskStatusQueued
		ti.Progress = "20%"
	case TaskStatusRunning:
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
	case TaskStatusSucceeded:
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		ti.Url = t.Content.URL
		// 按秒计费：回传实际时长（usage.total_seconds 含输入素材时长）。
		if t.Usage.TotalSeconds > 0 {
			ti.ActualSeconds = t.Usage.TotalSeconds
		} else if t.Duration > 0 {
			ti.ActualSeconds = float64(t.Duration)
		}
	case TaskStatusFailed, TaskStatusCancelled, TaskStatusExpired:
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		if t.Error != nil && t.Error.Message != "" {
			ti.Reason = fmt.Sprintf("%s (%s)", t.Error.Message, t.Error.Code)
		} else {
			ti.Reason = fmt.Sprintf("task %s", t.Status)
		}
	default:
		if t.Status == "" {
			return nil, errors.New("task status is empty")
		}
		return nil, fmt.Errorf("unknown task status: %s", t.Status)
	}
	return ti, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var q queryResponse
	if err := common.Unmarshal(originTask.Data, &q); err != nil {
		return nil, errors.Wrap(err, "unmarshal minimax task data failed")
	}
	t := q.Task

	ov := dto.NewOpenAIVideo()
	ov.ID = originTask.TaskID
	ov.TaskID = originTask.TaskID
	ov.Status = originTask.Status.ToVideoStatus()
	ov.SetProgressStr(originTask.Progress)
	ov.Model = originTask.Properties.OriginModelName
	ov.CreatedAt = originTask.CreatedAt
	ov.CompletedAt = originTask.UpdatedAt
	if t.Content.URL != "" {
		ov.SetMetadata("url", t.Content.URL)
	}
	if t.Duration > 0 {
		ov.Seconds = fmt.Sprintf("%d", t.Duration)
	}
	if t.Resolution != "" {
		ov.SetMetadata("resolution", t.Resolution)
	}
	if t.Ratio != "" {
		ov.SetMetadata("ratio", t.Ratio)
	}
	if t.Error != nil && t.Error.Message != "" {
		ov.Error = &dto.OpenAIVideoError{Message: t.Error.Message, Code: t.Error.Code}
	}
	return common.Marshal(ov)
}
