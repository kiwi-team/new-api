package fal

// FLUX 3 视频：同一个模型名 flux-3 对应 fal 上的三个子端点，按素材角色路由。
//
//	blackforestlabs/flux-3/text-to-video
//	blackforestlabs/flux-3/image-to-video
//	blackforestlabs/flux-3/first-last-frame-to-video
//
// https://fal.ai/models/blackforestlabs/flux-3/text-to-video

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	taskbilling "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"net/http"
)

const (
	// Flux3VideoModel 是 FLUX 3 视频在 new-api 侧暴露的唯一模型名。
	Flux3VideoModel = "flux-3"

	flux3AppPath = "blackforestlabs/flux-3"

	// 上游 duration 枚举为 5~20 秒的整数。
	flux3MinDuration     = 5
	flux3MaxDuration     = 20
	flux3DefaultDuration = 5

	flux3DefaultAspectRatio = "auto"
	flux3DefaultResolution  = "720p"
)

func isFlux3Video(modelName string) bool {
	return modelName == Flux3VideoModel
}

// Flux3VideoRequest 覆盖三个子端点的输入并集：image_url 仅用于 image-to-video，
// start_image_url / end_image_url 仅用于 first-last-frame-to-video。
type Flux3VideoRequest struct {
	Prompt          string `json:"prompt"`
	AspectRatio     string `json:"aspect_ratio,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	Duration        int    `json:"duration"`
	GenerateAudio   *bool  `json:"generate_audio,omitempty"`
	SafetyTolerance *int   `json:"safety_tolerance,omitempty"`
	ImageURL        string `json:"image_url,omitempty"`
	StartImageURL   string `json:"start_image_url,omitempty"`
	EndImageURL     string `json:"end_image_url,omitempty"`
}

// flux3Endpoint 把 task action 映射到 fal 的子端点路径。
func flux3Endpoint(action string) string {
	switch action {
	case constant.TaskActionFirstTailGenerate:
		return "first-last-frame-to-video"
	case constant.TaskActionGenerate:
		return "image-to-video"
	default:
		return "text-to-video"
	}
}

// flux3BilledSeconds 返回本次请求计费与下发上游共用的时长。
//
// 上游 duration 默认值是 "auto"（由模型决定长度），但本渠道按秒计费，"auto" 会让
// 预扣的秒数与实际产出时长脱节。因此这里始终解析出一个确定的秒数，BuildRequestBody
// 也用同一个值显式下发 duration，保证「计费秒数 == 上游生成秒数」。
func flux3BilledSeconds(req relaycommon.TaskSubmitReq) int {
	seconds := common.String2Int(req.Seconds)
	if req.Duration > 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = flux3DefaultDuration
	}
	if seconds < flux3MinDuration {
		seconds = flux3MinDuration
	}
	if seconds > flux3MaxDuration {
		seconds = flux3MaxDuration
	}
	return seconds
}

// flux3SetAction 按素材角色重新判定 action，决定走哪个子端点。
//
// ValidateBasicTaskRequest 只在 Vidu 渠道把「两张图」识别成首尾帧，其余渠道一律
// 记成 generate；FLUX 3 需要区分首尾帧，所以在这里按 references 的角色重判一次。
// 不受支持的素材组合直接 400，避免静默丢图后照常计费。
func flux3SetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if req.HasReferenceMaterial() {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s only supports first_frame and last_frame references", Flux3VideoModel),
			"invalid_request", http.StatusBadRequest)
	}
	hasFirst := req.HasRefRole(relaycommon.RefRoleFirstFrame)
	hasLast := req.HasRefRole(relaycommon.RefRoleLastFrame)
	switch {
	case hasFirst && hasLast:
		info.Action = constant.TaskActionFirstTailGenerate
	case hasFirst:
		info.Action = constant.TaskActionGenerate
	case hasLast:
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s requires a first_frame image when last_frame is provided", Flux3VideoModel),
			"invalid_request", http.StatusBadRequest)
	default:
		info.Action = constant.TaskActionTextGenerate
	}
	return nil
}

// Flux3VideoRequestBody 构造 FLUX 3 视频的上游请求体。
func Flux3VideoRequestBody(req relaycommon.TaskSubmitReq, action string) (io.Reader, error) {
	body := Flux3VideoRequest{
		Prompt:      req.Prompt,
		AspectRatio: req.AspectRatio,
		Resolution:  req.Resolution,
	}

	// 先套用厂商自定义 metadata（generate_audio / safety_tolerance 等），
	// 再回填受控字段，确保 duration 与素材不会被 metadata 绕过。
	if err := taskbilling.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return nil, err
	}

	body.AspectRatio = taskbilling.DefaultString(body.AspectRatio, flux3DefaultAspectRatio)
	body.Resolution = taskbilling.DefaultString(body.Resolution, flux3DefaultResolution)
	body.Duration = flux3BilledSeconds(req)

	body.ImageURL = ""
	body.StartImageURL = ""
	body.EndImageURL = ""
	switch action {
	case constant.TaskActionFirstTailGenerate:
		first := req.RefsByRole(relaycommon.RefRoleFirstFrame)
		last := req.RefsByRole(relaycommon.RefRoleLastFrame)
		if len(first) == 0 || len(last) == 0 {
			return nil, fmt.Errorf("first_frame and last_frame images are required")
		}
		body.StartImageURL = first[0].URL
		body.EndImageURL = last[0].URL
	case constant.TaskActionGenerate:
		first := req.RefsByRole(relaycommon.RefRoleFirstFrame)
		if len(first) == 0 {
			return nil, fmt.Errorf("image is required")
		}
		body.ImageURL = first[0].URL
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
