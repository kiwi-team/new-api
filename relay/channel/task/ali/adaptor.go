package ali

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

// AliVideoRequest 阿里通义万相视频生成请求
type AliVideoRequest struct {
	Model      string              `json:"model"`
	Input      AliVideoInput       `json:"input"`
	Parameters *AliVideoParameters `json:"parameters,omitempty"`
}

// AliMediaItem wan2.7 media 数组元素
type AliMediaItem struct {
	Type           string `json:"type"`                      // "first_frame"/"last_frame"/"reference_image"/"reference_video"
	URL            string `json:"url,omitempty"`             // 资源URL
	ReferenceVoice string `json:"reference_voice,omitempty"` // 音色参考（wan2.7，wav/mp3，1-10s）
}

// AliVideoInput 视频输入参数
type AliVideoInput struct {
	Prompt         string         `json:"prompt,omitempty"`          // 文本提示词
	Media          []AliMediaItem `json:"media,omitempty"`           // wan2.7 media 数组
	ImgURL         string         `json:"img_url,omitempty"`         // 首帧图像URL或Base64（图生视频）
	FirstFrameURL  string         `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string         `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	AudioURL       string         `json:"audio_url,omitempty"`       // 音频URL
	NegativePrompt string         `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string         `json:"template,omitempty"`        // 视频特效模板
}

// AliVideoParameters 视频参数
type AliVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P（图生视频、首尾帧生视频）
	Size         string `json:"size,omitempty"`          // 尺寸: 如 "832*480"（文生视频）
	Ratio        string `json:"ratio,omitempty"`         // 画面比例: "16:9"/"9:16"/"1:1"/"4:3"/"3:4"（wan2.7）
	Duration     int    `json:"duration,omitempty"`      // 时长: 3-10秒
	PromptExtend *bool  `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool  `json:"watermark,omitempty"`     // 是否添加水印（必须用指针：bool+omitempty 会丢弃 false）
	Audio        *bool  `json:"audio,omitempty"`         // 是否添加音频（wan2.5）
	Seed         *int   `json:"seed,omitempty"`          // 随机数种子（必须用指针：int+omitempty 会丢弃 0）
	ShotType     string `json:"shot_type,omitempty"`     // 镜头类型: "single"（图生视频）、"double"（首尾帧生视频）
}

// AliVideoResponse 阿里通义万相响应
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Usage     *AliUsage      `json:"usage,omitempty"`
}

// AliVideoOutput 输出信息
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AliUsage 使用统计
type AliUsage struct {
	Duration   int `json:"duration,omitempty"`
	VideoCount int `json:"video_count,omitempty"`
	SR         int `json:"SR,omitempty"`
}

type AliMetadata struct {
	// Input 相关
	AudioURL       string `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string `json:"img_url,omitempty"`         // 图片URL（图生视频）
	FirstFrameURL  string `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	NegativePrompt string `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string `json:"template,omitempty"`        // 视频特效模板

	// Parameters 相关
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"
	Ratio        *string `json:"ratio,omitempty"`         // 画面比例（wan2.7）
	Duration     *int    `json:"duration,omitempty"`      // 时长
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
	aliReq      *AliVideoRequest
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// 阿里通义万相支持 JSON 格式，不使用 multipart
	var taskReq relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &taskReq); err != nil {
		return service.TaskErrorWrapper(err, "unmarshal_task_request_failed", http.StatusBadRequest)
	}
	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		// 参数转换失败基本都是客户端入参问题（size/metadata 非法等），
		// 应返回 400 而非 500，避免被上层当作服务端故障跨渠道重试。
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	a.aliReq = aliReq
	logger.LogJson(c, "ali video request body", aliReq)
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader sets required headers for Ali API
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	bodyBytes, err := common.Marshal(a.aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}

	return bytes.NewReader(bodyBytes), nil
}

var (
	size480p = []string{
		"832*480",
		"480*832",
		"624*624",
	}
	size720p = []string{
		"1280*720",
		"720*1280",
		"960*960",
		"1088*832",
		"832*1088",
	}
	size1080p = []string{
		"1920*1080",
		"1080*1920",
		"1440*1440",
		"1632*1248",
		"1248*1632",
	}
)

func sizeToResolution(size string) (string, error) {
	if lo.Contains(size480p, size) {
		return "480P", nil
	} else if lo.Contains(size720p, size) {
		return "720P", nil
	} else if lo.Contains(size1080p, size) {
		return "1080P", nil
	}
	return "", fmt.Errorf("invalid size: %s", size)
}

// isResolutionRatioModel: 是否使用 resolution+ratio+media 协议（wan2.7 / happyhorse 系列）
func isResolutionRatioModel(model string) bool {
	return strings.HasPrefix(model, "wan2.7") || strings.HasPrefix(model, "happyhorse")
}

// isReferenceToVideoModel: 是否为参考生视频（r2v）模型。
// r2v 端点只认 input.media 数组，不认 input.img_url。
func isReferenceToVideoModel(model string) bool {
	return strings.Contains(model, "r2v")
}

// isHappyHorseModel: HappyHorse 系列（其 r2v 接口未提供 prompt_extend 参数）
func isHappyHorseModel(model string) bool {
	return strings.HasPrefix(model, "happyhorse")
}

// aliMediaTypeForRole 把统一 role 映射为阿里 media.type。
// 返回 false 表示该 role 阿里不支持（由统一层的能力校验拦截，此处兜底）。
func aliMediaTypeForRole(role string) (string, bool) {
	switch role {
	case relaycommon.RefRoleFirstFrame:
		return "first_frame", true
	case relaycommon.RefRoleLastFrame:
		return "last_frame", true
	case relaycommon.RefRoleReferenceImage:
		return "reference_image", true
	case relaycommon.RefRoleReferenceVideo:
		return "reference_video", true
	default:
		return "", false
	}
}

func ProcessAliOtherRatios(aliReq *AliVideoRequest) (map[string]float64, error) {
	otherRatios := make(map[string]float64)
	aliRatios := map[string]map[string]float64{
		"wan2.7-r2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"happyhorse-1.0-r2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"happyhorse-1.1-r2v": {
			"480P":  1,
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.7-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.7-t2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"happyhorse-1.0-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"happyhorse-1.0-t2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.6-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.5-t2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-t2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.5-i2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-i2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-kf2v-flash": {
			"480P":  1,
			"720P":  2,
			"1080P": 4.8,
		},
		"wan2.2-i2v-flash": {
			"480P": 1,
			"720P": 2,
		},
		"wan2.2-s2v": {
			"480P": 1,
			"720P": 0.9 / 0.5,
		},
	}
	var resolution string

	// size match
	if aliReq.Parameters.Size != "" {
		toResolution, err := sizeToResolution(aliReq.Parameters.Size)
		if err != nil {
			return nil, err
		}
		resolution = toResolution
	} else {
		resolution = strings.ToUpper(aliReq.Parameters.Resolution)
		if !strings.HasSuffix(resolution, "P") {
			resolution = resolution + "P"
		}
	}
	if otherRatio, ok := lookupAliRatios(aliRatios, aliReq.Model); ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}
	return otherRatios, nil
}

// buildAliMedia 按素材角色构造 wan2.7/happyhorse 的 input.media 数组。
//
// 顺序即上游顺序：提示词里的“图1/视频1”“[图片1]”依赖它与 references 严格一致，
// 因此这里保持 req.References 的原始顺序，不做重排。
//
// 未显式传 references 时（旧客户端），回退到 img_url / first_frame_url /
// last_frame_url 三个平行字段，行为与改动前一致。
func buildAliMedia(req *relaycommon.TaskSubmitReq, aliReq *AliVideoRequest) []AliMediaItem {
	if len(req.References) > 0 {
		media := make([]AliMediaItem, 0, len(req.References))
		for _, ref := range req.References {
			mediaType, ok := aliMediaTypeForRole(ref.Role)
			if !ok || ref.URL == "" {
				continue
			}
			media = append(media, AliMediaItem{
				Type:           mediaType,
				URL:            ref.URL,
				ReferenceVoice: ref.VoiceURL,
			})
		}
		if len(media) > 0 {
			return media
		}
	}

	// 回退路径：保持既有的 img_url/first_frame_url/last_frame_url 语义。
	var media []AliMediaItem
	firstFrameURL := aliReq.Input.ImgURL
	if firstFrameURL == "" {
		firstFrameURL = aliReq.Input.FirstFrameURL
	}
	if firstFrameURL != "" {
		media = append(media, AliMediaItem{Type: "first_frame", URL: firstFrameURL})
	}
	if aliReq.Input.LastFrameURL != "" {
		media = append(media, AliMediaItem{Type: "last_frame", URL: aliReq.Input.LastFrameURL})
	}
	return media
}

// lookupAliRatios 按模型名查倍率表，支持带日期后缀的快照版模型名。
//
// 阿里的部分模型有 "<base>-<yyyy-MM-dd>" 形式的快照版（如
// wan2.7-r2v-2026-06-12），它们与基础版计价一致。精确匹配失败时退化为
// 最长前缀匹配，避免快照版静默按 1 倍率计费（1080P 会被当成 720P 收费）。
func lookupAliRatios(table map[string]map[string]float64, model string) (map[string]float64, bool) {
	if r, ok := table[model]; ok {
		return r, true
	}
	bestKey := ""
	for key := range table {
		if !strings.HasPrefix(model, key) {
			continue
		}
		// 只接受在分隔符处截断的前缀，避免 "wan2.7-r2v" 误匹配 "wan2.7-r2vx"
		rest := model[len(key):]
		if rest != "" && rest[0] != '-' {
			continue
		}
		if len(key) > len(bestKey) {
			bestKey = key
		}
	}
	if bestKey == "" {
		return nil, false
	}
	return table[bestKey], true
}

func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	imageUrl := req.InputReference
	if imageUrl == "" && req.Image != "" {
		imageUrl = req.Image
	}
	aliReq := &AliVideoRequest{
		Model: req.Model,
		Input: AliVideoInput{
			Prompt:         req.Prompt,
			ImgURL:         imageUrl,
			NegativePrompt: req.NegativePrompt,
		},
		Parameters: &AliVideoParameters{
			Watermark: lo.ToPtr(false), // 默认不打水印
		},
	}
	// HappyHorse 的 r2v 接口未提供 prompt_extend 参数，传了会触发 InvalidParameter。
	if !isHappyHorseModel(req.Model) {
		aliReq.Parameters.PromptExtend = lo.ToPtr(true) // 默认开启智能改写
	}

	// wan2.7 / happyhorse 使用 resolution + ratio 新协议
	isResolutionRatioProto := isResolutionRatioModel(req.Model)

	// 统一的 resolution / aspect_ratio 优先于 size
	if req.Resolution != "" {
		resolution := strings.ToUpper(strings.TrimSpace(req.Resolution))
		if !strings.HasSuffix(resolution, "P") && !strings.Contains(resolution, "*") {
			resolution = resolution + "P"
		}
		aliReq.Parameters.Resolution = resolution
	}
	if req.AspectRatio != "" {
		aliReq.Parameters.Ratio = req.AspectRatio
	}

	// 处理分辨率映射
	if req.Size != "" && aliReq.Parameters.Resolution == "" && aliReq.Parameters.Size == "" {
		if isResolutionRatioProto {
			// 新协议不使用 size，只支持 resolution（如 "720P"/"1080P"）
			resolution := strings.ToUpper(req.Size)
			if !strings.HasSuffix(resolution, "P") {
				resolution = resolution + "P"
			}
			aliReq.Parameters.Resolution = resolution
		} else {
			// text to video size must be contained *
			if strings.Contains(req.Model, "t2v") && !strings.Contains(req.Size, "*") {
				return nil, fmt.Errorf("invalid size: %s, example: %s", req.Size, "1920*1080")
			}
			if strings.Contains(req.Size, "*") {
				aliReq.Parameters.Size = req.Size
			} else {
				resolution := strings.ToUpper(req.Size)
				// 支持 480p, 720p, 1080p 或 480P, 720P, 1080P
				if !strings.HasSuffix(resolution, "P") {
					resolution = resolution + "P"
				}
				aliReq.Parameters.Resolution = resolution
			}
		}
	} else if aliReq.Parameters.Resolution == "" && aliReq.Parameters.Size == "" {
		// 根据模型设置默认分辨率（仅在既没显式 resolution 也没 size 时）
		if isResolutionRatioProto {
			aliReq.Parameters.Resolution = "1080P"
			// 参考生视频与文生视频都需要 ratio：r2v 无首帧时画面比例无从推断，
			// 缺省按官方默认 16:9（传了 first_frame 时阿里会忽略 ratio）。
			if strings.Contains(req.Model, "t2v") || isReferenceToVideoModel(req.Model) {
				aliReq.Parameters.Ratio = "16:9"
			}
		} else if strings.Contains(req.Model, "t2v") {
			if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Size = "1920*1080"
			} else if strings.HasPrefix(req.Model, "wan2.2") {
				aliReq.Parameters.Size = "1920*1080"
			} else {
				aliReq.Parameters.Size = "1280*720"
			}
		} else {
			if strings.HasPrefix(req.Model, "wan2.6") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-flash") {
				aliReq.Parameters.Resolution = "720P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-plus") {
				aliReq.Parameters.Resolution = "1080P"
			} else {
				aliReq.Parameters.Resolution = "720P"
			}
		}
	}

	// 处理时长
	if req.Duration > 0 {
		aliReq.Parameters.Duration = req.Duration
	} else if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return nil, errors.Wrap(err, "convert seconds to int failed")
		} else {
			aliReq.Parameters.Duration = seconds
		}
	} else {
		aliReq.Parameters.Duration = 5 // 默认5秒
	}

	// 从 metadata 中提取额外参数
	if req.Metadata != nil {
		if metadataBytes, err := common.Marshal(req.Metadata); err == nil {
			err = common.Unmarshal(metadataBytes, aliReq)
			if err != nil {
				return nil, errors.Wrap(err, "unmarshal metadata failed")
			}
			if s, ok := req.Metadata["audio_url"]; ok {
				aliReq.Input.AudioURL = s.(string)
			}
			if v, ok := req.Metadata["img_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Input.ImgURL = s
				}
			}
			if v, ok := req.Metadata["first_frame_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Input.FirstFrameURL = s
				}
			}
			if v, ok := req.Metadata["last_frame_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Input.LastFrameURL = s
				}
			}
			if v, ok := req.Metadata["negative_prompt"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Input.NegativePrompt = s
				}
			}
			if v, ok := req.Metadata["shot_type"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Parameters.ShotType = s
				}
			}
			if v, ok := req.Metadata["prompt_extend"]; ok {
				if s, ok := v.(bool); ok {
					aliReq.Parameters.PromptExtend = &s
				}
			}
			if v, ok := req.Metadata["resolution"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Parameters.Resolution = s
				}
			}
			if v, ok := req.Metadata["ratio"]; ok {
				if s, ok := v.(string); ok && s != "" {
					aliReq.Parameters.Ratio = s
				}
			}
			if v, ok := req.Metadata["seed"]; ok {
				// JSON 数字 unmarshal 到 interface{} 默认是 float64，同时兼容 int / json.Number
				switch n := v.(type) {
				case float64:
					seed := int(n)
					aliReq.Parameters.Seed = &seed
				case int:
					seed := n
					aliReq.Parameters.Seed = &seed
				case int64:
					seed := int(n)
					aliReq.Parameters.Seed = &seed
				}
			}
			if v, ok := req.Metadata["watermark"]; ok {
				if s, ok := v.(bool); ok {
					aliReq.Parameters.Watermark = &s
				}
			}
			if v, ok := req.Metadata["audio"]; ok {
				if s, ok := v.(bool); ok {
					aliReq.Parameters.Audio = &s
				}
			}
		} else {
			return nil, errors.Wrap(err, "marshal metadata failed")
		}
	}

	// wan2.7 / happyhorse 使用 input.media 数组承载素材。
	// 注意：r2v 端点只认 media，完全不认 img_url —— 之前这里按模型名是否含 "i2v"
	// 判断，导致 *-r2v 命中新协议却仍发 img_url，请求必然失败。改为按素材角色构造。
	if isResolutionRatioProto {
		media := buildAliMedia(&req, aliReq)
		if len(media) > 0 {
			aliReq.Input.Media = media
			// 清除旧字段，避免与 media 同时发送
			aliReq.Input.ImgURL = ""
			aliReq.Input.FirstFrameURL = ""
			aliReq.Input.LastFrameURL = ""
		}
	}

	if aliReq.Model != req.Model {
		return nil, errors.New("can't change model with metadata")
	}

	info.PriceData.OtherRatios = map[string]float64{
		"seconds": float64(aliReq.Parameters.Duration),
	}

	ratios, err := ProcessAliOtherRatios(aliReq)
	if err != nil {
		return nil, err
	}
	for s, f := range ratios {
		info.PriceData.OtherRatios[s] = f
	}

	return aliReq, nil
}

// DoRequest delegates to common helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = aliResp.Output.TaskID
	openAIResp.TaskID = aliResp.Output.TaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return aliResp.Output.TaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

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

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 设置视频URL（核心字段）
	openAIResp.SetMetadata("url", aliResp.Output.VideoURL)

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
