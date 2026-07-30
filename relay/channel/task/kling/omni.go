package kling

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/pkg/errors"
)

// omni.go 实现可灵 3.0 Omni / O1 的参考生视频接入。
//
// 与旧版可灵（/v1/videos/{image2video,text2video}）完全不同的协议：
//   - 提交： POST {base}/omni-video/{model}   请求体为 contents/settings/options 三段式
//   - 查询： GET  {base}/tasks?task_ids={id}  结果在 data.result[].outputs[]
//
// 素材通过 contents 数组承载，每项带 type 与可选的 id（供 prompt 里用 @id 指代）。
// 文档：https://klingai.com/document-api/api/video/3-0-omni/video-omni

const (
	// omniDefaultSeconds 是可灵 Omni 未指定时长时的默认值（官方默认 5s）。
	omniDefaultSeconds = 5
	// omniDefaultResolution 是官方默认清晰度。
	omniDefaultResolution = "720p"
)

// ============================
// Request structures
// ============================

// omniContent 是 contents 数组的一项。
type omniContent struct {
	Type      string `json:"type"`                 // prompt/first_frame/last_frame/refer_image/feature_video/base_video/element
	Text      string `json:"text,omitempty"`       // type=prompt 时的提示词
	URL       string `json:"url,omitempty"`        // 素材地址
	ID        string `json:"id,omitempty"`         // 素材索引 ID，供 prompt 中 @id 指代
	ElementID string `json:"element_id,omitempty"` // type=element 时的主体 ID
}

// omniSettings 对应请求体的 settings 段。
type omniSettings struct {
	MultiShot   *bool  `json:"multi_shot,omitempty"`
	Audio       string `json:"audio,omitempty"`        // native/original/off
	Resolution  string `json:"resolution,omitempty"`   // 720p/1080p/4k
	AspectRatio string `json:"aspect_ratio,omitempty"` // 16:9/9:16/1:1
	Duration    int    `json:"duration,omitempty"`     // 3~15
}

// omniWatermarkInfo 对应 options.watermark_info。
type omniWatermarkInfo struct {
	Enabled bool `json:"enabled"`
}

// omniOptions 对应请求体的 options 段。
type omniOptions struct {
	CallbackURL    string             `json:"callback_url,omitempty"`
	ExternalTaskID string             `json:"external_task_id,omitempty"`
	WatermarkInfo  *omniWatermarkInfo `json:"watermark_info,omitempty"`
}

type omniRequestPayload struct {
	Contents []omniContent `json:"contents"`
	Settings *omniSettings `json:"settings,omitempty"`
	Options  *omniOptions  `json:"options,omitempty"`
}

// ============================
// Response structures
// ============================

// omniSubmitResponse 是提交任务的响应。
type omniSubmitResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		CreateTime int64  `json:"create_time"`
		UpdateTime int64  `json:"update_time"`
		ExternalID string `json:"external_id"`
	} `json:"data"`
}

// omniOutput 是查询结果里 outputs 数组的一项。
type omniOutput struct {
	Type         string `json:"type"` // image/video/audio/element/voice
	ID           string `json:"id"`
	URL          string `json:"url"`
	WatermarkURL string `json:"watermark_url"`
	Duration     string `json:"duration"`
}

// omniQueryResponse 是查询任务的响应（GET /tasks?task_ids=）。
type omniQueryResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      struct {
		Result []struct {
			ID         string       `json:"id"`
			Status     string       `json:"status"`
			Message    string       `json:"message"`
			CreateTime int64        `json:"create_time"`
			UpdateTime int64        `json:"update_time"`
			ExternalID string       `json:"external_id"`
			Outputs    []omniOutput `json:"outputs"`
		} `json:"result"`
		Count int `json:"count"`
	} `json:"data"`
}

// ============================
// Request building
// ============================

// omniContentTypeForRole 把统一 role 映射为可灵 contents[].type。
func omniContentTypeForRole(role string) (string, bool) {
	switch role {
	case relaycommon.RefRoleFirstFrame:
		return "first_frame", true
	case relaycommon.RefRoleLastFrame:
		return "last_frame", true
	case relaycommon.RefRoleReferenceImage:
		return "refer_image", true
	case relaycommon.RefRoleReferenceVideo:
		return "feature_video", true
	case relaycommon.RefRoleBaseVideo:
		return "base_video", true
	case relaycommon.RefRoleElement:
		return "element", true
	default:
		return "", false
	}
}

// buildOmniRequestPayload 由统一请求构造可灵 Omni 的请求体。
//
// contents 顺序与 references 顺序严格一致：prompt 里的 @id 指代依赖它。
func buildOmniRequestPayload(req *relaycommon.TaskSubmitReq) (*omniRequestPayload, error) {
	payload := &omniRequestPayload{
		Contents: make([]omniContent, 0, len(req.References)+1),
	}

	// prompt 固定排在首位
	if strings.TrimSpace(req.Prompt) != "" {
		payload.Contents = append(payload.Contents, omniContent{
			Type: "prompt",
			Text: req.Prompt,
		})
	}

	if len(req.References) > 0 {
		for _, ref := range req.References {
			contentType, ok := omniContentTypeForRole(ref.Role)
			if !ok {
				continue
			}
			item := omniContent{Type: contentType, ID: ref.ID}
			if contentType == "element" {
				if strings.TrimSpace(ref.ElementID) == "" {
					return nil, errors.New("element reference requires element_id")
				}
				item.ElementID = ref.ElementID
			} else {
				if strings.TrimSpace(ref.URL) == "" {
					continue
				}
				item.URL = ref.URL
			}
			payload.Contents = append(payload.Contents, item)
		}
	} else if req.HasImage() {
		// 回退路径：单图为首帧，双图为首尾帧。
		for i, img := range req.Images {
			contentType := "first_frame"
			if i == 1 {
				contentType = "last_frame"
			} else if i > 1 {
				contentType = "refer_image"
			}
			payload.Contents = append(payload.Contents, omniContent{Type: contentType, URL: img})
		}
	}

	settings := &omniSettings{
		Duration:   resolveOmniSeconds(req),
		Resolution: strings.ToLower(strings.TrimSpace(req.Resolution)),
	}
	if settings.Resolution == "" {
		settings.Resolution = omniDefaultResolution
	}
	if req.AspectRatio != "" {
		settings.AspectRatio = req.AspectRatio
	} else if !hasOmniFrameOrVideo(req) {
		// 官方要求：没有首帧也没有参考视频时 aspect_ratio 必填。
		settings.AspectRatio = "16:9"
	}
	if req.Audio != "" {
		audio := strings.ToLower(strings.TrimSpace(req.Audio))
		switch audio {
		case "native", "original", "off":
			settings.Audio = audio
		default:
			return nil, fmt.Errorf("invalid audio %q, expected one of native/original/off", req.Audio)
		}
	}

	// metadata 承载可灵特有的其余开关
	if req.Metadata != nil {
		if v, ok := req.Metadata["multi_shot"].(bool); ok {
			settings.MultiShot = &v
		}
		if v, ok := req.Metadata["resolution"].(string); ok && v != "" {
			settings.Resolution = strings.ToLower(v)
		}
		if v, ok := req.Metadata["aspect_ratio"].(string); ok && v != "" {
			settings.AspectRatio = v
		}
		if v, ok := req.Metadata["audio"].(string); ok && v != "" {
			settings.Audio = strings.ToLower(v)
		}

		opts := &omniOptions{}
		hasOpts := false
		if v, ok := req.Metadata["callback_url"].(string); ok && v != "" {
			opts.CallbackURL = v
			hasOpts = true
		}
		if v, ok := req.Metadata["external_task_id"].(string); ok && v != "" {
			opts.ExternalTaskID = v
			hasOpts = true
		}
		if v, ok := req.Metadata["watermark"].(bool); ok {
			opts.WatermarkInfo = &omniWatermarkInfo{Enabled: v}
			hasOpts = true
		}
		if hasOpts {
			payload.Options = opts
		}
	}
	payload.Settings = settings

	if len(payload.Contents) == 0 {
		return nil, errors.New("at least a prompt or one reference is required")
	}
	return payload, nil
}

// hasOmniFrameOrVideo 判断是否存在首帧或参考视频（决定 aspect_ratio 是否必填）。
func hasOmniFrameOrVideo(req *relaycommon.TaskSubmitReq) bool {
	if req.HasRefRole(relaycommon.RefRoleFirstFrame, relaycommon.RefRoleReferenceVideo, relaycommon.RefRoleBaseVideo) {
		return true
	}
	return len(req.References) == 0 && req.HasImage()
}

// resolveOmniSeconds 归一化时长，缺省取官方默认 5s。
func resolveOmniSeconds(req *relaycommon.TaskSubmitReq) int {
	seconds := req.GetSeconds()
	if seconds <= 0 {
		return omniDefaultSeconds
	}
	return seconds
}

// ============================
// Billing
// ============================

// omniResolutionRatios 是各清晰度相对 720p 的价格倍率。
// 可灵按清晰度分档计价，720p 为基准档。
var omniResolutionRatios = map[string]float64{
	"720p":  1,
	"1080p": 2,
	"4k":    4,
}

// applyOmniPriceRatios 把时长与清晰度写入计费倍率。
// 必须在 ValidateRequestAndSetAction 内调用（定价发生在其后）。
func applyOmniPriceRatios(info *relaycommon.RelayInfo, req *relaycommon.TaskSubmitReq) {
	if info.PriceData.OtherRatios == nil {
		info.PriceData.OtherRatios = map[string]float64{}
	}
	info.PriceData.OtherRatios["seconds"] = float64(resolveOmniSeconds(req))

	resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
	if resolution == "" {
		if req.Metadata != nil {
			if v, ok := req.Metadata["resolution"].(string); ok {
				resolution = strings.ToLower(strings.TrimSpace(v))
			}
		}
	}
	if resolution == "" {
		resolution = omniDefaultResolution
	}
	if ratio, ok := omniResolutionRatios[resolution]; ok && ratio != 1 {
		info.PriceData.OtherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
	}
}

// ============================
// Response parsing
// ============================

// parseOmniSubmitResponse 解析提交响应，返回任务 ID。
func parseOmniSubmitResponse(body []byte) (string, error) {
	var resp omniSubmitResponse
	if err := common.Unmarshal(body, &resp); err != nil {
		return "", errors.Wrap(err, "unmarshal omni submit response failed")
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("kling omni error %d: %s", resp.Code, resp.Message)
	}
	if strings.TrimSpace(resp.Data.ID) == "" {
		return "", errors.New("kling omni returned empty task id")
	}
	return resp.Data.ID, nil
}

// isOmniQueryResponse 判断响应体是否为 Omni 的查询结构（data.result 数组）。
func isOmniQueryResponse(body []byte) bool {
	var probe struct {
		Data struct {
			Result []struct {
				ID string `json:"id"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &probe); err != nil {
		return false
	}
	return len(probe.Data.Result) > 0
}

// parseOmniTaskResult 把 Omni 查询响应映射为统一的 TaskInfo。
func parseOmniTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var resp omniQueryResponse
	if err := common.Unmarshal(body, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal omni query response failed")
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("kling omni error %d: %s", resp.Code, resp.Message)
	}
	if len(resp.Data.Result) == 0 {
		return nil, errors.New("kling omni query returned no result")
	}

	r := resp.Data.Result[0]
	ti := &relaycommon.TaskInfo{
		Code:   resp.Code,
		TaskID: r.ID,
		Reason: r.Message,
	}

	switch strings.ToLower(strings.TrimSpace(r.Status)) {
	case "submitted":
		ti.Status = model.TaskStatusSubmitted
		ti.Progress = "10%"
	case "processing":
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
	case "succeeded":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		for _, out := range r.Outputs {
			if out.Type != "video" {
				continue
			}
			ti.Url = out.URL
			if seconds := common.String2Float64(out.Duration); seconds > 0 {
				ti.ActualSeconds = seconds
			}
			break
		}
	case "failed":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		if strings.TrimSpace(ti.Reason) == "" {
			ti.Reason = "task failed"
		}
	default:
		return nil, fmt.Errorf("unknown kling omni task status: %s", r.Status)
	}
	return ti, nil
}

// extractOmniVideoOutput 取出成功任务的视频输出，供 ConvertToOpenAIVideo 使用。
func extractOmniVideoOutput(body []byte) (omniOutput, bool) {
	var resp omniQueryResponse
	if err := common.Unmarshal(body, &resp); err != nil {
		return omniOutput{}, false
	}
	if len(resp.Data.Result) == 0 {
		return omniOutput{}, false
	}
	for _, out := range resp.Data.Result[0].Outputs {
		if out.Type == "video" {
			return out, true
		}
	}
	return omniOutput{}, false
}
