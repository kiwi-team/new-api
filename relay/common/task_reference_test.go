package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这些用例锁定两件事：
//  1. 未传 references 时，归一化与 action 派生必须与改动前完全一致（向后兼容）。
//  2. 传了 references 时，按角色派生 action，并对不支持的能力返回 400。

func TestNormalizeReferences_LegacyImageCompat(t *testing.T) {
	cases := []struct {
		name       string
		req        TaskSubmitReq
		wantImages []string
		wantRoles  []string
	}{
		{
			name:       "single image field",
			req:        TaskSubmitReq{Image: "https://a/1.png"},
			wantImages: []string{"https://a/1.png"},
			wantRoles:  []string{RefRoleFirstFrame},
		},
		{
			name:       "two images",
			req:        TaskSubmitReq{Images: []string{"a", "b"}},
			wantImages: []string{"a", "b"},
			wantRoles:  []string{RefRoleFirstFrame, RefRoleLastFrame},
		},
		{
			name:       "three images become reference images",
			req:        TaskSubmitReq{Images: []string{"a", "b", "c"}},
			wantImages: []string{"a", "b", "c"},
			wantRoles:  []string{RefRoleReferenceImage, RefRoleReferenceImage, RefRoleReferenceImage},
		},
		{
			name:       "no image at all",
			req:        TaskSubmitReq{Prompt: "hi"},
			wantImages: nil,
			wantRoles:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			normalizeReferences(&req)
			if len(req.Images) != len(tc.wantImages) {
				t.Fatalf("Images = %v, want %v", req.Images, tc.wantImages)
			}
			for i := range tc.wantImages {
				if req.Images[i] != tc.wantImages[i] {
					t.Fatalf("Images[%d] = %q, want %q", i, req.Images[i], tc.wantImages[i])
				}
			}
			if len(req.References) != len(tc.wantRoles) {
				t.Fatalf("References len = %d, want %d", len(req.References), len(tc.wantRoles))
			}
			for i, want := range tc.wantRoles {
				if req.References[i].Role != want {
					t.Fatalf("References[%d].Role = %q, want %q", i, req.References[i].Role, want)
				}
			}
		})
	}
}

// 归一化不得回填 req.Image：既有 adaptor（如 ppio/vidu.go）用 req.Image 是否为空
// 判断“是否图生视频”，回填会改变其行为。
func TestNormalizeReferences_LegacyDoesNotBackfillImage(t *testing.T) {
	req := TaskSubmitReq{Images: []string{"a", "b"}}
	normalizeReferences(&req)
	if req.Image != "" {
		t.Fatalf("Image = %q, want empty (legacy path must not backfill)", req.Image)
	}
}

func TestNormalizeReferences_PreservesOrder(t *testing.T) {
	req := TaskSubmitReq{References: []TaskReference{
		{Type: RefTypeImage, Role: RefRoleReferenceImage, URL: "first"},
		{Type: RefTypeVideo, Role: RefRoleReferenceVideo, URL: "second"},
		{Type: RefTypeImage, Role: RefRoleReferenceImage, URL: "third"},
	}}
	normalizeReferences(&req)
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if req.References[i].URL != w {
			t.Fatalf("References[%d].URL = %q, want %q (order must be preserved)", i, req.References[i].URL, w)
		}
	}
	// 只有图片类素材进 Images
	if len(req.Images) != 2 || req.Images[0] != "first" || req.Images[1] != "third" {
		t.Fatalf("Images = %v, want [first third]", req.Images)
	}
}

// 客户端拼接 JSON 时常带上尾随空格，原样透传会被上游判为无效地址
// （方舟返回 image_url "resource not found"），归一化必须去掉首尾空白。
func TestNormalizeReferences_TrimsURLWhitespace(t *testing.T) {
	t.Run("legacy images", func(t *testing.T) {
		req := TaskSubmitReq{Images: []string{" https://a/1.png ", "\thttps://a/2.png\n"}}
		normalizeReferences(&req)
		assert.Equal(t, []string{"https://a/1.png", "https://a/2.png"}, req.Images)
		require.Len(t, req.References, 2)
		assert.Equal(t, "https://a/1.png", req.References[0].URL)
		assert.Equal(t, "https://a/2.png", req.References[1].URL)
	})

	t.Run("explicit references", func(t *testing.T) {
		req := TaskSubmitReq{References: []TaskReference{
			{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: " https://a/1.png "},
			{Type: RefTypeAudio, Role: RefRoleReferenceAudio, URL: " https://a/v.mp3 ", VoiceURL: " https://a/voice.mp3 "},
		}}
		normalizeReferences(&req)
		assert.Equal(t, "https://a/1.png", req.References[0].URL)
		assert.Equal(t, "https://a/v.mp3", req.References[1].URL)
		assert.Equal(t, "https://a/voice.mp3", req.References[1].VoiceURL)
		assert.Equal(t, []string{"https://a/1.png"}, req.Images)
	})

	t.Run("whitespace only url stays empty for validation", func(t *testing.T) {
		req := TaskSubmitReq{References: []TaskReference{
			{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: "   "},
		}}
		normalizeReferences(&req)
		assert.Equal(t, "", req.References[0].URL)
		taskErr := ValidateReferenceCapability(constant.ChannelTypeDoubaoVideo, "doubao-seedance-1-0", &req)
		require.NotNil(t, taskErr)
		assert.Equal(t, "invalid_reference", taskErr.Code)
	})
}

func TestNormalizeReferences_InfersTypeAndRole(t *testing.T) {
	req := TaskSubmitReq{References: []TaskReference{
		{Role: RefRoleReferenceVideo, URL: "v"},
		{Type: RefTypeAudio, URL: "a"},
		{Role: RefRoleElement, ElementID: "e1"},
	}}
	normalizeReferences(&req)
	if req.References[0].Type != RefTypeVideo {
		t.Errorf("ref0 Type = %q, want %q", req.References[0].Type, RefTypeVideo)
	}
	if req.References[1].Role != RefRoleReferenceAudio {
		t.Errorf("ref1 Role = %q, want %q", req.References[1].Role, RefRoleReferenceAudio)
	}
	if req.References[2].Type != RefTypeElement {
		t.Errorf("ref2 Type = %q, want %q", req.References[2].Type, RefTypeElement)
	}
}

func TestDeriveActionFromReferences(t *testing.T) {
	cases := []struct {
		name string
		refs []TaskReference
		want string
	}{
		{
			name: "reference image -> referenceGenerate",
			refs: []TaskReference{{Type: RefTypeImage, Role: RefRoleReferenceImage, URL: "a"}},
			want: constant.TaskActionReferenceGenerate,
		},
		{
			name: "reference video -> referenceGenerate",
			refs: []TaskReference{{Type: RefTypeVideo, Role: RefRoleReferenceVideo, URL: "a"}},
			want: constant.TaskActionReferenceGenerate,
		},
		{
			name: "first+last -> firstTailGenerate",
			refs: []TaskReference{
				{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: "a"},
				{Type: RefTypeImage, Role: RefRoleLastFrame, URL: "b"},
			},
			want: constant.TaskActionFirstTailGenerate,
		},
		{
			name: "first only -> generate",
			refs: []TaskReference{{Type: RefTypeImage, Role: RefRoleFirstFrame, URL: "a"}},
			want: constant.TaskActionGenerate,
		},
		{
			name: "element -> referenceGenerate",
			refs: []TaskReference{{Type: RefTypeElement, Role: RefRoleElement, ElementID: "e"}},
			want: constant.TaskActionReferenceGenerate,
		},
		{
			name: "empty -> default",
			refs: nil,
			want: constant.TaskActionTextGenerate,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := TaskSubmitReq{References: tc.refs}
			if got := deriveActionFromReferences(&req, constant.TaskActionTextGenerate); got != tc.want {
				t.Fatalf("action = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateReferenceCapability(t *testing.T) {
	img := func(role string) TaskReference {
		return TaskReference{Type: RefTypeImage, Role: role, URL: "https://x/i.png"}
	}
	vid := func(role string) TaskReference {
		return TaskReference{Type: RefTypeVideo, Role: role, URL: "https://x/v.mp4"}
	}
	aud := TaskReference{Type: RefTypeAudio, Role: RefRoleReferenceAudio, URL: "https://x/a.mp3"}

	cases := []struct {
		name        string
		channelType int
		model       string
		refs        []TaskReference
		wantErr     bool
		wantCode    string
	}{
		{
			name: "veo rejects reference video", channelType: constant.ChannelTypeGemini, model: "veo-3.1-generate-preview",
			refs: []TaskReference{vid(RefRoleReferenceVideo)}, wantErr: true, wantCode: "unsupported_reference_role",
		},
		{
			name: "veo allows up to 3 reference images", channelType: constant.ChannelTypeGemini, model: "veo-3.1-generate-preview",
			refs: []TaskReference{img(RefRoleReferenceImage), img(RefRoleReferenceImage), img(RefRoleReferenceImage)},
		},
		{
			name: "veo rejects 4 reference images", channelType: constant.ChannelTypeGemini, model: "veo-3.1-generate-preview",
			refs: []TaskReference{img(RefRoleReferenceImage), img(RefRoleReferenceImage),
				img(RefRoleReferenceImage), img(RefRoleReferenceImage)},
			wantErr: true, wantCode: "too_many_references",
		},
		{
			name: "gemini omni rejects reference audio", channelType: constant.ChannelTypeGemini, model: "gemini-omni-flash-preview",
			refs: []TaskReference{aud}, wantErr: true, wantCode: "unsupported_reference_role",
		},
		{
			name: "seedance2 rejects mixing frame with reference", channelType: constant.ChannelTypeDoubaoVideo,
			model:   "doubao-seedance-2-0-260128",
			refs:    []TaskReference{img(RefRoleFirstFrame), img(RefRoleReferenceImage)},
			wantErr: true, wantCode: "conflicting_references",
		},
		{
			name: "seedance2 allows image+video+audio", channelType: constant.ChannelTypeDoubaoVideo,
			model: "doubao-seedance-2-0-260128",
			refs:  []TaskReference{img(RefRoleReferenceImage), vid(RefRoleReferenceVideo), aud},
		},
		{
			name: "seedance2 rejects audio alone", channelType: constant.ChannelTypeDoubaoVideo,
			model: "doubao-seedance-2-0-260128", refs: []TaskReference{aud},
			wantErr: true, wantCode: "invalid_reference",
		},
		{
			name: "seedance2 rejects 4 reference videos", channelType: constant.ChannelTypeDoubaoVideo,
			model: "doubao-seedance-2-0-260128",
			refs: []TaskReference{img(RefRoleReferenceImage), vid(RefRoleReferenceVideo),
				vid(RefRoleReferenceVideo), vid(RefRoleReferenceVideo), vid(RefRoleReferenceVideo)},
			wantErr: true, wantCode: "too_many_references",
		},
		{
			name: "happyhorse r2v rejects first_frame", channelType: constant.ChannelTypeAli, model: "happyhorse-1.1-r2v",
			refs: []TaskReference{img(RefRoleFirstFrame)}, wantErr: true, wantCode: "unsupported_reference_role",
		},
		{
			name: "happyhorse r2v accepts reference images", channelType: constant.ChannelTypeAli, model: "happyhorse-1.1-r2v",
			refs: []TaskReference{img(RefRoleReferenceImage), img(RefRoleReferenceImage)},
		},
		{
			name: "wan2.7 r2v rejects 6 combined", channelType: constant.ChannelTypeAli, model: "wan2.7-r2v",
			refs: []TaskReference{img(RefRoleReferenceImage), img(RefRoleReferenceImage), img(RefRoleReferenceImage),
				img(RefRoleReferenceImage), img(RefRoleReferenceImage), vid(RefRoleReferenceVideo)},
			wantErr: true, wantCode: "too_many_references",
		},
		{
			name: "wan2.7 r2v requires a reference material", channelType: constant.ChannelTypeAli, model: "wan2.7-r2v",
			refs: []TaskReference{img(RefRoleFirstFrame)}, wantErr: true, wantCode: "invalid_reference",
		},
		{
			name: "wan2.7 r2v rejects last_frame", channelType: constant.ChannelTypeAli, model: "wan2.7-r2v",
			refs: []TaskReference{img(RefRoleLastFrame)}, wantErr: true, wantCode: "unsupported_reference_role",
		},
		{
			name: "kling omni allows video+element", channelType: constant.ChannelTypeKling, model: "kling-3.0-omni",
			refs: []TaskReference{vid(RefRoleReferenceVideo),
				{Type: RefTypeElement, Role: RefRoleElement, ElementID: "e1"}},
		},
		{
			name: "kling omni limits to 4 when video present", channelType: constant.ChannelTypeKling, model: "kling-3.0-omni",
			refs: []TaskReference{vid(RefRoleReferenceVideo), img(RefRoleReferenceImage), img(RefRoleReferenceImage),
				img(RefRoleReferenceImage), img(RefRoleReferenceImage), img(RefRoleReferenceImage)},
			wantErr: true, wantCode: "too_many_references",
		},
		{
			name: "element without element_id is rejected", channelType: constant.ChannelTypeKling, model: "kling-3.0-omni",
			refs:    []TaskReference{{Type: RefTypeElement, Role: RefRoleElement}},
			wantErr: true, wantCode: "invalid_reference",
		},
		{
			name: "last frame without first frame is rejected", channelType: constant.ChannelTypeKling, model: "kling-3.0-omni",
			refs: []TaskReference{img(RefRoleLastFrame)}, wantErr: true, wantCode: "invalid_reference",
		},
		{
			name: "unknown channel skips validation", channelType: 9999, model: "whatever",
			refs: []TaskReference{aud},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &TaskSubmitReq{References: tc.refs}
			normalizeReferences(req)
			taskErr := ValidateReferenceCapability(tc.channelType, tc.model, req)
			if tc.wantErr {
				if taskErr == nil {
					t.Fatalf("expected error, got nil")
				}
				if taskErr.StatusCode != 400 {
					t.Errorf("StatusCode = %d, want 400", taskErr.StatusCode)
				}
				if !taskErr.LocalError {
					t.Errorf("LocalError = false, want true (must not retry across channels)")
				}
				if tc.wantCode != "" && taskErr.Code != tc.wantCode {
					t.Errorf("Code = %q, want %q", taskErr.Code, tc.wantCode)
				}
				return
			}
			if taskErr != nil {
				t.Fatalf("unexpected error: %+v", taskErr)
			}
		})
	}
}

func TestValidateReferenceCapability_NoRefsIsNoop(t *testing.T) {
	req := &TaskSubmitReq{Images: []string{"a"}}
	normalizeReferences(req)
	// 走降级路径构造出的 references 不参与能力校验（由调用方的 explicitRefs 控制），
	// 但即便直接调用也不应报错。
	if err := ValidateReferenceCapability(constant.ChannelTypeAli, "wan2.2-i2v-plus", req); err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}
}

func TestGetSeconds(t *testing.T) {
	cases := []struct {
		name string
		req  TaskSubmitReq
		want int
	}{
		{"seconds string wins", TaskSubmitReq{Seconds: "8", Duration: 5}, 8},
		{"falls back to duration", TaskSubmitReq{Duration: 5}, 5},
		{"model-decides sentinel", TaskSubmitReq{Seconds: "-1"}, -1},
		{"model-decides via duration", TaskSubmitReq{Duration: -1}, -1},
		{"unset", TaskSubmitReq{}, 0},
		{"garbage falls back to duration", TaskSubmitReq{Seconds: "abc", Duration: 7}, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.GetSeconds(); got != tc.want {
				t.Fatalf("GetSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestModelFamilyDetection(t *testing.T) {
	if !IsSeedance2Model("doubao-seedance-2-0-260128") {
		t.Error("expected seedance 2.0 detection")
	}
	if IsSeedance2Model("doubao-seedance-1-5-pro-251215") {
		t.Error("seedance 1.5 must not be detected as 2.0")
	}
	if !IsKlingOmniModel("kling-3.0-omni") || !IsKlingOmniModel("kling-o1") {
		t.Error("expected kling omni detection")
	}
	if IsKlingOmniModel("kling-v2-master") {
		t.Error("kling v2 must not be detected as omni")
	}
}
