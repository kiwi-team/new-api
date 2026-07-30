package gemini

import (
	"fmt"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// veo_reference.go 实现 Veo 3.1 的参考图（referenceImages）支持。
//
// 官方结构（注意 referenceImages 在 instances 里，不在 parameters 里）：
//
//	{"instances":[{"prompt":"...","referenceImages":[
//	   {"image":{"inlineData":{"mimeType":"image/png","data":"<base64>"}},
//	    "referenceType":"asset"}]}]}
//
// 硬约束（来自官方文档）：
//   - 仅 Veo 3.1 支持，最多 3 张；
//   - 使用参考图时 durationSeconds 必须为 8；
//   - 使用参考图时 personGeneration 只能是 "allow_adult"。
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
// gs:// 直接透传为 gcsUri；其余形态统一转成 inline base64。
// 返回 nil 表示引用为空。
func BuildVeoImageObject(c *gin.Context, ref string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	if service.ClassifyMediaRef(ref) == service.MediaRefGSURL {
		return map[string]any{"gcsUri": ref}, nil
	}
	data, mimeType, err := service.ResolveMediaRef(c, ref, "image/jpeg")
	if err != nil {
		return nil, err
	}
	return map[string]any{"bytesBase64Encoded": data, "mimeType": mimeType}, nil
}

// BuildVeoReferenceImages 由统一 references 构造 Veo 的 referenceImages 数组。
// 只取 reference_image 角色；首帧/尾帧走 instances 的 image/lastFrame，不在此列。
// 返回空切片表示本次请求没有参考图。
func BuildVeoReferenceImages(c *gin.Context, req *relaycommon.TaskSubmitReq) ([]map[string]any, error) {
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

	out := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		// Gemini API（非 Vertex）用 inlineData 包裹；Vertex 侧同样接受该结构。
		data, mimeType, err := service.ResolveMediaRef(c, ref.URL, "image/jpeg")
		if err != nil {
			return nil, fmt.Errorf("resolve reference image failed: %w", err)
		}
		referenceType := strings.TrimSpace(ref.ReferenceType)
		if referenceType == "" {
			referenceType = veoDefaultReferenceType
		}
		out = append(out, map[string]any{
			"image":         map[string]any{"inlineData": map[string]any{"mimeType": mimeType, "data": data}},
			"referenceType": referenceType,
		})
	}
	return out, nil
}

// ApplyVeoReferenceConstraints 施加使用参考图时的官方硬约束：
// durationSeconds 必须为 8，personGeneration 必须为 allow_adult。
// 客户端若显式传了冲突值，返回错误而不是静默改写。
//
// 两者都写入 parameters（官方 parameters 表所列位置）；instance 参数保留
// 以便调用方按需在 instances 内补充自己的时长字段（如 Vertex 的 duration）。
func ApplyVeoReferenceConstraints(instance map[string]any, parameters map[string]any, req *relaycommon.TaskSubmitReq) error {
	if seconds := req.GetSeconds(); seconds > 0 && seconds != VeoReferenceDurationSeconds {
		return fmt.Errorf("veo reference images require durationSeconds=%d, got %d",
			VeoReferenceDurationSeconds, seconds)
	}
	parameters["durationSeconds"] = VeoReferenceDurationSeconds

	if pg, ok := parameters["personGeneration"].(string); ok && pg != "" && pg != VeoReferencePersonGeneration {
		return fmt.Errorf("veo reference images require personGeneration=%q, got %q",
			VeoReferencePersonGeneration, pg)
	}
	parameters["personGeneration"] = VeoReferencePersonGeneration
	return nil
}
