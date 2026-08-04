package gemini

import (
	"fmt"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// veo_reference.go 实现 Veo 3.1 的参考图（referenceImages）与首尾帧支持。
//
// 官方结构（注意 referenceImages 在 instances 里，不在 parameters 里）：
//
//	{"instances":[{"prompt":"...","referenceImages":[
//	   {"image":{"bytesBase64Encoded":"<base64>","mimeType":"image/png"},
//	    "referenceType":"asset"}]}]}
//
// 硬约束（来自官方文档）：
//   - 仅 Veo 3.1 支持，最多 3 张；
//   - 使用参考图时 durationSeconds 必须为 8；
//   - 使用参考图时 personGeneration 只能是 "allow_adult"；
//   - 参考图与首帧/尾帧互斥。
//
// 文档参考：https://ai.google.dev/gemini-api/docs/veo#reference-images
const (
	// VeoMaxReferenceImages 是 Veo 3.1 参考图数量上限。
	VeoMaxReferenceImages = 3
	// VeoReferenceDurationSeconds 是使用参考图时被强制的视频时长。
	VeoReferenceDurationSeconds = 8
	// VeoReferencePersonGeneration 是使用参考图时唯一允许的 personGeneration 取值。
	VeoReferencePersonGeneration = "allow_adult"
	// veoDefaultReferenceType 是官方示例中唯一出现的 referenceType。
	veoDefaultReferenceType = "asset"
)

// SupportsVeoReferenceImages 报告该 Veo 模型是否支持参考图。
// 官方明确只有 Veo 3.1（且非 Lite）支持；3.0 / 2.x / 3.1-lite 均为 n/a。
func SupportsVeoReferenceImages(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(m, "veo") {
		return false
	}
	if strings.Contains(m, "lite") {
		return false
	}
	return strings.Contains(m, "3.1") || strings.Contains(m, "3-1")
}

// BuildVeoImageObject 把素材引用归一化为 Veo 的 image 对象。
// gs:// 直接透传为 gcsUri（仅 Vertex 支持）；其余形态统一转成 inline base64。
// 返回 nil 表示引用为空。
func BuildVeoImageObject(c *gin.Context, ref string) (*VeoImageInput, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	if service.ClassifyMediaRef(ref) == service.MediaRefGSURL {
		return &VeoImageInput{GcsUri: ref}, nil
	}
	data, mimeType, err := service.ResolveMediaRef(c, ref, "image/jpeg")
	if err != nil {
		return nil, err
	}
	return &VeoImageInput{BytesBase64Encoded: data, MimeType: mimeType}, nil
}

// BuildVeoReferenceImages 由统一 references 构造 Veo 的 referenceImages 数组。
//
// 只取 reference_image 角色；首帧/尾帧走 instances 的 image/lastFrame，不在此列。
// 返回空切片表示本次请求没有参考图。
//
// allowGCS 区分两家 API：Vertex AI 接受 gs:// 引用，Gemini API 不接受。
func BuildVeoReferenceImages(c *gin.Context, req *relaycommon.TaskSubmitReq, allowGCS bool) ([]VeoReferenceImage, error) {
	refs := req.RefsByRole(relaycommon.RefRoleReferenceImage)
	if len(refs) == 0 {
		return nil, nil
	}
	if !SupportsVeoReferenceImages(req.Model) {
		return nil, fmt.Errorf("model %s does not support reference images (Veo 3.1 only)", req.Model)
	}
	if len(refs) > VeoMaxReferenceImages {
		return nil, fmt.Errorf("model %s supports at most %d reference images, got %d",
			req.Model, VeoMaxReferenceImages, len(refs))
	}
	// 官方约束：Veo 3.1 不允许首帧与参考图同时出现，必须二选一。
	if req.HasRefRole(relaycommon.RefRoleFirstFrame, relaycommon.RefRoleLastFrame) {
		return nil, fmt.Errorf(
			"model %s cannot combine first_frame/last_frame with reference images; choose one", req.Model)
	}

	out := make([]VeoReferenceImage, 0, len(refs))
	for _, ref := range refs {
		var image *VeoImageInput
		if service.ClassifyMediaRef(ref.URL) == service.MediaRefGSURL {
			if !allowGCS {
				return nil, fmt.Errorf("gs:// reference images are only supported on Vertex AI")
			}
			image = &VeoImageInput{GcsUri: ref.URL}
		} else {
			data, mimeType, err := service.ResolveMediaRef(c, ref.URL, "image/jpeg")
			if err != nil {
				return nil, fmt.Errorf("resolve reference image failed: %w", err)
			}
			image = &VeoImageInput{BytesBase64Encoded: data, MimeType: mimeType}
		}
		referenceType := strings.TrimSpace(ref.ReferenceType)
		if referenceType == "" {
			referenceType = veoDefaultReferenceType
		}
		out = append(out, VeoReferenceImage{Image: image, ReferenceType: referenceType})
	}
	return out, nil
}

// ApplyVeoReferenceConstraints 施加使用参考图时的官方硬约束：
// durationSeconds 必须为 8，personGeneration 必须为 allow_adult。
// 客户端若显式传了冲突值，返回错误而不是静默改写。
func ApplyVeoReferenceConstraints(params *VeoParameters, req *relaycommon.TaskSubmitReq) error {
	if seconds := req.GetSeconds(); seconds > 0 && seconds != VeoReferenceDurationSeconds {
		return fmt.Errorf("veo reference images require durationSeconds=%d, got %d",
			VeoReferenceDurationSeconds, seconds)
	}
	if params.DurationSeconds != 0 && params.DurationSeconds != VeoReferenceDurationSeconds {
		return fmt.Errorf("veo reference images require durationSeconds=%d, got %d",
			VeoReferenceDurationSeconds, params.DurationSeconds)
	}
	params.DurationSeconds = VeoReferenceDurationSeconds

	if params.PersonGeneration != "" && params.PersonGeneration != VeoReferencePersonGeneration {
		return fmt.Errorf("veo reference images require personGeneration=%q, got %q",
			VeoReferencePersonGeneration, params.PersonGeneration)
	}
	params.PersonGeneration = VeoReferencePersonGeneration
	return nil
}

// ApplyVeoInstanceReferences 把统一 references 映射到 Veo 的 instance 字段：
// reference_image -> referenceImages（并施加 8s / allow_adult 硬约束）
// first_frame     -> image
// last_frame      -> lastFrame（官方要求必须与 image 搭配使用）
func ApplyVeoInstanceReferences(c *gin.Context, req *relaycommon.TaskSubmitReq,
	instance *VeoInstance, params *VeoParameters, allowGCS bool) error {

	refImages, err := BuildVeoReferenceImages(c, req, allowGCS)
	if err != nil {
		return err
	}
	if len(refImages) > 0 {
		instance.ReferenceImages = refImages
		if err := ApplyVeoReferenceConstraints(params, req); err != nil {
			return err
		}
	}

	// 首帧 / 尾帧（与 referenceImages 是相互独立的机制）
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleFirstFrame); ok && ref.URL != "" {
		img, err := BuildVeoImageObject(c, ref.URL)
		if err != nil {
			return fmt.Errorf("resolve first_frame failed: %w", err)
		}
		if img != nil {
			instance.Image = img
		}
	}
	if ref, ok := req.FirstRefByRole(relaycommon.RefRoleLastFrame); ok && ref.URL != "" {
		if instance.Image == nil {
			return fmt.Errorf("last_frame must be used together with a first_frame image")
		}
		img, err := BuildVeoImageObject(c, ref.URL)
		if err != nil {
			return fmt.Errorf("resolve last_frame failed: %w", err)
		}
		if img != nil {
			instance.LastFrame = img
		}
	}
	return nil
}
