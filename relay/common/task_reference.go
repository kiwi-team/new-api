package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

// task_reference.go 负责 /v1/videos 统一 references 字段的归一化、action 派生
// 与按渠道的能力校验。
//
// 设计要点：
//   - references 为空时，由旧字段 image/images/input_reference 降级填充，保证零破坏。
//   - 不重排 references：prompt 中的指代（阿里“图1”、可灵“@id”、Gemini
//     “<IMAGE_REF_n>”）依赖素材顺序与上游数组顺序严格一致。
//   - 能力不支持一律返回 400（见 ValidateReferenceCapability）。校验必须在
//     ValidateRequestAndSetAction 阶段完成——放到 BuildRequestURL/BuildRequestBody
//     会被上层包装成 500。

// referenceCapability 描述一个渠道/模型family支持的素材能力。
type referenceCapability struct {
	name string // 用于错误信息，如 "wan2.7-r2v"

	roles map[string]bool // 支持的 role 集合

	maxFirstFrame int // first_frame 上限，0 表示不支持，-1 表示不限
	maxLastFrame  int
	maxRefImage   int
	maxRefVideo   int
	maxRefAudio   int
	maxElement    int

	// maxImagePlusVideo 为“参考图 + 参考视频”的合计上限，0 表示不限制。
	maxImagePlusVideo int

	// exclusiveScenes 为 true 时，首帧/首尾帧 与 参考生视频 三种场景互斥
	// （Seedance 2.0 明确要求，不可混用）。
	exclusiveScenes bool

	// allowLastFrameAlone 为 true 时允许只传尾帧（MiniMax v2 支持该模式）；
	// 可灵、豆包等则要求尾帧必须与首帧搭配。
	allowLastFrameAlone bool
}

func (c referenceCapability) supports(role string) bool {
	return c.roles[role]
}

func capRoles(roles ...string) map[string]bool {
	m := make(map[string]bool, len(roles))
	for _, r := range roles {
		m[r] = true
	}
	return m
}

// getReferenceCapability 按渠道类型与模型名返回能力描述。
// 返回 nil 表示该渠道未接入 references 能力矩阵，此时跳过校验（保持既有行为）。
func getReferenceCapability(channelType int, model string) *referenceCapability {
	m := strings.ToLower(strings.TrimSpace(model))

	switch channelType {
	case constant.ChannelTypeAli:
		switch {
		case strings.HasPrefix(m, "happyhorse") && strings.Contains(m, "r2v"):
			// HappyHorse 参考生视频：只支持 reference_image，1~9 张。
			return &referenceCapability{
				name:        "happyhorse r2v",
				roles:       capRoles(RefRoleReferenceImage),
				maxRefImage: 9,
			}
		case strings.HasPrefix(m, "wan2.7") && strings.Contains(m, "r2v"):
			// wan2.7 参考生视频：reference_image / reference_video / first_frame，
			// first_frame 最多 1 个，参考图+参考视频合计 ≤ 5。
			return &referenceCapability{
				name:              "wan2.7-r2v",
				roles:             capRoles(RefRoleReferenceImage, RefRoleReferenceVideo, RefRoleFirstFrame),
				maxFirstFrame:     1,
				maxRefImage:       5,
				maxRefVideo:       5,
				maxImagePlusVideo: 5,
			}
		default:
			// 其余万相模型：首帧 / 首尾帧。
			return &referenceCapability{
				name:          "ali video",
				roles:         capRoles(RefRoleFirstFrame, RefRoleLastFrame),
				maxFirstFrame: 1,
				maxLastFrame:  1,
			}
		}

	case constant.ChannelTypeKling:
		if isKlingOmniModel(m) {
			// 可灵 3.0 Omni / O1：素材种类最全。
			// 图片与主体的组合上限随是否存在参考视频变化，见 validateKlingOmniCounts。
			return &referenceCapability{
				name: "kling omni",
				roles: capRoles(RefRoleFirstFrame, RefRoleLastFrame, RefRoleReferenceImage,
					RefRoleReferenceVideo, RefRoleBaseVideo, RefRoleElement),
				maxFirstFrame: 1,
				maxLastFrame:  1,
				maxRefImage:   7,
				maxRefVideo:   1,
				maxElement:    7,
			}
		}
		return &referenceCapability{
			name:          "kling",
			roles:         capRoles(RefRoleFirstFrame, RefRoleLastFrame),
			maxFirstFrame: 1,
			maxLastFrame:  1,
		}

	case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
		if isSeedance2Model(m) {
			// Seedance 2.0 多模态参考生视频：图 0~9 + 视频 0~3 + 音频 0~3，
			// 且首帧/首尾帧/多模态参考三种场景互斥。
			return &referenceCapability{
				name: "seedance 2.0",
				roles: capRoles(RefRoleFirstFrame, RefRoleLastFrame, RefRoleReferenceImage,
					RefRoleReferenceVideo, RefRoleReferenceAudio),
				maxFirstFrame:   1,
				maxLastFrame:    1,
				maxRefImage:     9,
				maxRefVideo:     3,
				maxRefAudio:     3,
				exclusiveScenes: true,
			}
		}
		return &referenceCapability{
			name:          "seedance",
			roles:         capRoles(RefRoleFirstFrame, RefRoleLastFrame),
			maxFirstFrame: 1,
			maxLastFrame:  1,
		}

	case constant.ChannelTypeMiniMaxVideo:
		// MiniMax v2（MiniMax-H3）多模态参考生视频：
		// 首尾帧与参考素材互斥；音频不能单独输入；混合输入总数 ≤ 12。
		return &referenceCapability{
			name: "minimax v2",
			roles: capRoles(RefRoleFirstFrame, RefRoleLastFrame, RefRoleReferenceImage,
				RefRoleReferenceVideo, RefRoleReferenceAudio),
			maxFirstFrame:       1,
			maxLastFrame:        1,
			maxRefImage:         9,
			maxRefVideo:         3,
			maxRefAudio:         3,
			exclusiveScenes:     true,
			allowLastFrameAlone: true,
		}

	case constant.ChannelTypeGemini, constant.ChannelTypeVertexAi:
		switch {
		case isOmniVideoModel(m):
			// Gemini Omni：图片参考 + 待编辑视频；不支持参考视频/音频。
			return &referenceCapability{
				name:          "gemini omni",
				roles:         capRoles(RefRoleFirstFrame, RefRoleReferenceImage, RefRoleBaseVideo),
				maxFirstFrame: 1,
				maxRefImage:   -1,
				maxElement:    0,
			}
		case strings.HasPrefix(m, "veo"):
			// Veo 3.1：referenceImages 最多 3 张；另有独立的首帧/尾帧插值机制。
			return &referenceCapability{
				name:          "veo",
				roles:         capRoles(RefRoleFirstFrame, RefRoleLastFrame, RefRoleReferenceImage),
				maxFirstFrame: 1,
				maxLastFrame:  1,
				maxRefImage:   3,
			}
		}
		return nil
	}
	return nil
}

// isKlingOmniModel 判断是否为可灵 Omni 系列（走 /omni-video 端点）。
func isKlingOmniModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "omni") || strings.HasPrefix(m, "kling-o1") || strings.HasPrefix(m, "kling-3.0-omni")
}

// IsKlingOmniModel 导出版本，供 kling adaptor 复用。
func IsKlingOmniModel(model string) bool {
	return isKlingOmniModel(model)
}

// isSeedance2Model 判断是否为 Seedance 2.0 系列。
func isSeedance2Model(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "seedance-2-0") || strings.Contains(m, "seedance-2.0")
}

// IsSeedance2Model 导出版本，供 doubao adaptor 复用。
func IsSeedance2Model(model string) bool {
	return isSeedance2Model(model)
}

// normalizeReferences 归一化素材：
//   - references 非空时按其填充 Images（保持顺序），供仅认 Images 的旧 adaptor 继续工作；
//   - references 为空时，仅做与改动前完全一致的 Image -> Images 兼容，并反向构造
//     references 供新 adaptor 统一读取。
//
// 该函数不重排 references。
func normalizeReferences(req *TaskSubmitReq) {
	// 素材 URL 两端的空白必须清掉：客户端拼接 JSON 时很容易带上尾随空格，
	// 原样透传给上游会被判为无效地址（方舟返回 image_url "resource not found"）。
	// data URI 与裸 base64 同样只受首尾空白影响，去掉是安全的。
	req.Image = strings.TrimSpace(req.Image)
	req.InputReference = strings.TrimSpace(req.InputReference)
	for i := range req.Images {
		req.Images[i] = strings.TrimSpace(req.Images[i])
	}
	for i := range req.References {
		req.References[i].URL = strings.TrimSpace(req.References[i].URL)
		req.References[i].VoiceURL = strings.TrimSpace(req.References[i].VoiceURL)
	}

	if len(req.References) > 0 {
		// 补全缺省的 type/role：只给其一时相互推断。
		for i := range req.References {
			ref := &req.References[i]
			ref.Role = strings.TrimSpace(ref.Role)
			ref.Type = strings.TrimSpace(ref.Type)
			if ref.Type == "" {
				ref.Type = inferRefType(ref.Role, ref.ElementID)
			}
			if ref.Role == "" {
				ref.Role = inferRefRole(ref.Type)
			}
		}
		// 把图片类素材同步进 Images，保证既有依赖 HasImage()/Images 的逻辑不失效。
		if len(req.Images) == 0 {
			for _, ref := range req.References {
				if ref.Type == RefTypeImage && ref.URL != "" {
					req.Images = append(req.Images, ref.URL)
				}
			}
		}
		if req.Image == "" && len(req.Images) > 0 {
			req.Image = req.Images[0]
		}
		return
	}

	// 降级路径：保持与改动前完全一致的语义——只做单图兼容，
	// 不引入 InputReference，也不回填 req.Image（避免改变既有 adaptor 的行为）。
	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}

	// 由 Images 反向构造 references，供新 adaptor 统一读取。
	// 这里只是同一份数据的另一种视图，不影响 action 派生（见 ValidateBasicTaskRequest）。
	switch {
	case len(req.Images) == 1:
		req.References = []TaskReference{
			{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: req.Images[0]},
		}
	case len(req.Images) == 2:
		req.References = []TaskReference{
			{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: req.Images[0]},
			{Type: RefTypeImage, Role: RefRoleLastFrame, URL: req.Images[1]},
		}
	case len(req.Images) > 2:
		for _, img := range req.Images {
			req.References = append(req.References, TaskReference{
				Type: RefTypeImage, Role: RefRoleReferenceImage, URL: img,
			})
		}
	}
}

func inferRefType(role, elementID string) string {
	switch role {
	case RefRoleReferenceVideo, RefRoleBaseVideo:
		return RefTypeVideo
	case RefRoleReferenceAudio:
		return RefTypeAudio
	case RefRoleElement:
		return RefTypeElement
	case RefRoleFirstFrame, RefRoleLastFrame, RefRoleReferenceImage:
		return RefTypeImage
	}
	if elementID != "" {
		return RefTypeElement
	}
	return RefTypeImage
}

func inferRefRole(refType string) string {
	switch refType {
	case RefTypeVideo:
		return RefRoleReferenceVideo
	case RefTypeAudio:
		return RefRoleReferenceAudio
	case RefTypeElement:
		return RefRoleElement
	default:
		return RefRoleReferenceImage
	}
}

// deriveActionFromReferences 按素材角色派生 action。
// 优先级：参考生视频 > 首尾帧 > 首帧 > 文生视频。
func deriveActionFromReferences(req *TaskSubmitReq, defaultAction string) string {
	if len(req.References) == 0 {
		if req.HasImage() {
			return constant.TaskActionGenerate
		}
		return defaultAction
	}
	if req.HasReferenceMaterial() {
		return constant.TaskActionReferenceGenerate
	}
	hasFirst := req.HasRefRole(RefRoleFirstFrame)
	hasLast := req.HasRefRole(RefRoleLastFrame)
	switch {
	case hasFirst && hasLast:
		return constant.TaskActionFirstTailGenerate
	case hasFirst || hasLast:
		return constant.TaskActionGenerate
	default:
		return defaultAction
	}
}

// ValidateReferenceCapability 按渠道/模型校验 references 的角色与数量。
// 不支持的能力一律返回 400，避免静默丢弃导致“看似成功但没用上参考”还照常计费。
func ValidateReferenceCapability(channelType int, model string, req *TaskSubmitReq) *dto.TaskError {
	if req == nil || len(req.References) == 0 {
		return nil
	}
	cap := getReferenceCapability(channelType, model)
	if cap == nil {
		return nil
	}

	// 1. 角色支持性
	for _, ref := range req.References {
		if !cap.supports(ref.Role) {
			return createTaskError(
				fmt.Errorf("model %s does not support reference role %q", model, ref.Role),
				"unsupported_reference_role", http.StatusBadRequest, true)
		}
		if ref.Role == RefRoleElement {
			if strings.TrimSpace(ref.ElementID) == "" {
				return createTaskError(
					fmt.Errorf("reference of role element requires element_id"),
					"invalid_reference", http.StatusBadRequest, true)
			}
		} else if strings.TrimSpace(ref.URL) == "" {
			return createTaskError(
				fmt.Errorf("reference of role %q requires url", ref.Role),
				"invalid_reference", http.StatusBadRequest, true)
		}
	}

	// 2. 各角色数量上限
	counts := []struct {
		role  string
		limit int
	}{
		{RefRoleFirstFrame, cap.maxFirstFrame},
		{RefRoleLastFrame, cap.maxLastFrame},
		{RefRoleReferenceImage, cap.maxRefImage},
		{RefRoleReferenceVideo, cap.maxRefVideo},
		{RefRoleReferenceAudio, cap.maxRefAudio},
		{RefRoleElement, cap.maxElement},
	}
	for _, c := range counts {
		if c.limit < 0 {
			continue // 不限
		}
		if n := req.CountRefsByRole(c.role); n > c.limit {
			return createTaskError(
				fmt.Errorf("model %s supports at most %d %s reference(s), got %d", model, c.limit, c.role, n),
				"too_many_references", http.StatusBadRequest, true)
		}
	}

	// 3. 参考图 + 参考视频 合计上限（wan2.7-r2v ≤ 5）
	if cap.maxImagePlusVideo > 0 {
		n := req.CountRefsByRole(RefRoleReferenceImage, RefRoleReferenceVideo)
		if n > cap.maxImagePlusVideo {
			return createTaskError(
				fmt.Errorf("model %s supports at most %d reference image(s)+video(s) combined, got %d",
					model, cap.maxImagePlusVideo, n),
				"too_many_references", http.StatusBadRequest, true)
		}
	}

	// 4. 场景互斥（Seedance 2.0：首帧 / 首尾帧 / 多模态参考 不可混用）
	if cap.exclusiveScenes {
		hasFrame := req.HasRefRole(RefRoleFirstFrame, RefRoleLastFrame)
		hasRefMaterial := req.HasReferenceMaterial()
		if hasFrame && hasRefMaterial {
			return createTaskError(
				fmt.Errorf("model %s cannot mix first_frame/last_frame with reference materials; they are mutually exclusive scenes", model),
				"conflicting_references", http.StatusBadRequest, true)
		}
	}

	// 5. 只有尾帧没有首帧：多数厂商要求首帧存在（MiniMax v2 例外）
	if !cap.allowLastFrameAlone &&
		req.HasRefRole(RefRoleLastFrame) && !req.HasRefRole(RefRoleFirstFrame) {
		return createTaskError(
			fmt.Errorf("last_frame requires a first_frame reference"),
			"invalid_reference", http.StatusBadRequest, true)
	}

	// 6. 可灵 Omni 的组合数量规则
	if cap.name == "kling omni" {
		if taskErr := validateKlingOmniCounts(model, req); taskErr != nil {
			return taskErr
		}
	}

	// 7. 阿里 wan2.7-r2v 至少要有一个参考图或参考视频
	if cap.name == "wan2.7-r2v" {
		if req.CountRefsByRole(RefRoleReferenceImage, RefRoleReferenceVideo) == 0 {
			return createTaskError(
				fmt.Errorf("model %s requires at least one reference_image or reference_video", model),
				"invalid_reference", http.StatusBadRequest, true)
		}
	}

	// 8. HappyHorse r2v 至少 1 张参考图
	if cap.name == "happyhorse r2v" && req.CountRefsByRole(RefRoleReferenceImage) == 0 {
		return createTaskError(
			fmt.Errorf("model %s requires at least one reference_image", model),
			"invalid_reference", http.StatusBadRequest, true)
	}

	// 9. Seedance 2.0 不可只传音频
	if cap.name == "seedance 2.0" {
		if req.CountRefsByRole(RefRoleReferenceAudio) > 0 &&
			req.CountRefsByRole(RefRoleReferenceImage, RefRoleReferenceVideo, RefRoleFirstFrame, RefRoleLastFrame) == 0 {
			return createTaskError(
				fmt.Errorf("model %s cannot take audio references alone; include at least one image or video", model),
				"invalid_reference", http.StatusBadRequest, true)
		}
	}

	return nil
}

// validateKlingOmniCounts 实现可灵 Omni 随参考视频变化的组合上限：
//   - 无参考视频 + 仅多图主体：参考图 + 主体 ≤ 7
//   - 有参考视频：参考图 + 主体 ≤ 4，且参考视频 ≤ 1
//
// 主体类型（视频角色主体 / 多图主体）由上游 element_id 决定，网关侧无法区分，
// 因此这里按最宽松的合计上限校验，更细的规则交由上游返回。
func validateKlingOmniCounts(model string, req *TaskSubmitReq) *dto.TaskError {
	refImages := req.CountRefsByRole(RefRoleReferenceImage)
	elements := req.CountRefsByRole(RefRoleElement)
	videos := req.CountRefsByRole(RefRoleReferenceVideo, RefRoleBaseVideo)

	limit := 7
	if videos > 0 {
		limit = 4
	}
	if refImages+elements > limit {
		return createTaskError(
			fmt.Errorf("model %s supports at most %d reference image(s)+element(s) combined%s, got %d",
				model, limit,
				map[bool]string{true: " when a reference video is present", false: ""}[videos > 0],
				refImages+elements),
			"too_many_references", http.StatusBadRequest, true)
	}
	// 首帧/首尾帧生成时最多 3 个主体
	if req.HasRefRole(RefRoleFirstFrame) && elements > 3 {
		return createTaskError(
			fmt.Errorf("model %s supports at most 3 element(s) when generating from frames, got %d", model, elements),
			"too_many_references", http.StatusBadRequest, true)
	}
	return nil
}
