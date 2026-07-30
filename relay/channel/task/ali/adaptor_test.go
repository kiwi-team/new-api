package ali

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 对照阿里官方文档锁定 references -> input.media 的映射。
// wan2.7:      https://help.aliyun.com/zh/model-studio/wan-video-to-video-api-reference
// HappyHorse:  https://help.aliyun.com/zh/model-studio/happyhorse-reference-to-video-api-reference

func TestBuildAliMedia_ReferenceRoles(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model: "wan2.7-r2v",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/f.png"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png",
				VoiceURL: "https://x/voice.mp3"},
			{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4"},
		},
	}
	media := buildAliMedia(req, &AliVideoRequest{})

	wantTypes := []string{"first_frame", "reference_image", "reference_video"}
	if len(media) != len(wantTypes) {
		t.Fatalf("media len = %d, want %d", len(media), len(wantTypes))
	}
	for i, want := range wantTypes {
		if media[i].Type != want {
			t.Errorf("media[%d].Type = %q, want %q", i, media[i].Type, want)
		}
	}
	// reference_voice 挂在单个 media 元素上
	if media[1].ReferenceVoice != "https://x/voice.mp3" {
		t.Errorf("media[1].ReferenceVoice = %q", media[1].ReferenceVoice)
	}
}

// 顺序必须与 references 一致 —— prompt 里的“图1/视频1”依赖它。
func TestBuildAliMedia_PreservesOrder(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "one"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "two"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "three"},
		},
	}
	media := buildAliMedia(req, &AliVideoRequest{})
	for i, want := range []string{"one", "two", "three"} {
		if media[i].URL != want {
			t.Fatalf("media[%d].URL = %q, want %q", i, media[i].URL, want)
		}
	}
}

// 未传 references 时回退到 img_url / last_frame_url，行为与改动前一致。
func TestBuildAliMedia_LegacyFallback(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{}
	aliReq := &AliVideoRequest{Input: AliVideoInput{
		ImgURL:       "https://x/first.png",
		LastFrameURL: "https://x/last.png",
	}}
	media := buildAliMedia(req, aliReq)
	if len(media) != 2 {
		t.Fatalf("media len = %d, want 2", len(media))
	}
	if media[0].Type != "first_frame" || media[0].URL != "https://x/first.png" {
		t.Errorf("media[0] = %+v", media[0])
	}
	if media[1].Type != "last_frame" || media[1].URL != "https://x/last.png" {
		t.Errorf("media[1] = %+v", media[1])
	}
}

// 回归：r2v 模型必须构造 media 而不是继续发 img_url —— r2v 端点不认 img_url。
func TestConvertToAliRequest_R2VUsesMediaNotImgURL(t *testing.T) {
	for _, modelName := range []string{"wan2.7-r2v", "happyhorse-1.1-r2v", "happyhorse-1.0-r2v"} {
		t.Run(modelName, func(t *testing.T) {
			a := &TaskAdaptor{}
			info := &relaycommon.RelayInfo{}
			req := relaycommon.TaskSubmitReq{
				Model:  modelName,
				Prompt: "图1 的女性",
				References: []relaycommon.TaskReference{
					{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
				},
			}
			aliReq, err := a.convertToAliRequest(info, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(aliReq.Input.Media) == 0 {
				t.Fatal("input.media must be populated for r2v models")
			}
			if aliReq.Input.Media[0].Type != "reference_image" {
				t.Errorf("media[0].Type = %q, want reference_image", aliReq.Input.Media[0].Type)
			}
			if aliReq.Input.ImgURL != "" {
				t.Errorf("input.img_url = %q, want empty (r2v endpoint rejects it)", aliReq.Input.ImgURL)
			}
		})
	}
}

// HappyHorse 的 r2v 接口未提供 prompt_extend，传了会触发 InvalidParameter。
func TestConvertToAliRequest_HappyHorseOmitsPromptExtend(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
		},
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Parameters.PromptExtend != nil {
		t.Errorf("prompt_extend = %v, want nil for HappyHorse", *aliReq.Parameters.PromptExtend)
	}
}

// 非 HappyHorse 仍保留默认开启智能改写的既有行为。
func TestConvertToAliRequest_WanKeepsPromptExtend(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
		},
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Parameters.PromptExtend == nil || !*aliReq.Parameters.PromptExtend {
		t.Error("prompt_extend should default to true for wan models")
	}
}

// r2v 无首帧时需要 ratio（画面比例无从推断）。
func TestConvertToAliRequest_R2VDefaultsRatio(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
		},
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Parameters.Ratio == "" {
		t.Error("ratio should default for r2v when no first_frame is given")
	}
	if aliReq.Parameters.Resolution != "1080P" {
		t.Errorf("resolution = %q, want 1080P", aliReq.Parameters.Resolution)
	}
}

// 统一的 resolution / aspect_ratio 字段应优先于 size 推断。
func TestConvertToAliRequest_UnifiedResolutionWins(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:       "wan2.7-r2v",
		Prompt:      "x",
		Resolution:  "720p",
		AspectRatio: "9:16",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
		},
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Parameters.Resolution != "720P" {
		t.Errorf("resolution = %q, want 720P (uppercased)", aliReq.Parameters.Resolution)
	}
	if aliReq.Parameters.Ratio != "9:16" {
		t.Errorf("ratio = %q, want 9:16", aliReq.Parameters.Ratio)
	}
}

// negative_prompt 走一等字段而非 metadata。
func TestConvertToAliRequest_NegativePrompt(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-r2v",
		Prompt:         "x",
		NegativePrompt: "blurry",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png"},
		},
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Input.NegativePrompt != "blurry" {
		t.Errorf("negative_prompt = %q", aliReq.Input.NegativePrompt)
	}
}

// r2v 的分辨率倍率必须存在，否则 1080P 会按 1 计费。
func TestProcessAliOtherRatios_R2V(t *testing.T) {
	for _, modelName := range []string{"wan2.7-r2v", "happyhorse-1.0-r2v", "happyhorse-1.1-r2v"} {
		t.Run(modelName, func(t *testing.T) {
			ratios, err := ProcessAliOtherRatios(&AliVideoRequest{
				Model:      modelName,
				Parameters: &AliVideoParameters{Resolution: "1080P"},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, ok := ratios["resolution-1080P"]
			if !ok {
				t.Fatalf("missing resolution-1080P ratio for %s", modelName)
			}
			if got == 1 {
				t.Errorf("1080P ratio = 1 for %s, want a premium over 720P", modelName)
			}
		})
	}
}

// 既有的 i2v 行为不能被改动破坏。
func TestConvertToAliRequest_I2VLegacyStillBuildsMedia(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "x",
		Image:  "https://x/first.png",
	}
	aliReq, err := a.convertToAliRequest(&relaycommon.RelayInfo{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(aliReq.Input.Media) != 1 || aliReq.Input.Media[0].Type != "first_frame" {
		t.Fatalf("media = %+v, want a single first_frame", aliReq.Input.Media)
	}
	if aliReq.Input.ImgURL != "" {
		t.Errorf("img_url should be cleared once media is built, got %q", aliReq.Input.ImgURL)
	}
}

// 带日期后缀的快照版模型（如 wan2.7-r2v-2026-06-12）与基础版计价一致，
// 必须能匹配到倍率表，否则 1080P 会被静默按 720P 计费。
func TestLookupAliRatios_DatedModelSnapshot(t *testing.T) {
	table := map[string]map[string]float64{
		"wan2.7-r2v": {"720P": 1, "1080P": 1 / 0.6},
		"wan2.7-i2v": {"720P": 1, "1080P": 1 / 0.6},
	}
	cases := []struct {
		model   string
		wantHit bool
	}{
		{"wan2.7-r2v", true},
		{"wan2.7-r2v-2026-06-12", true},
		{"wan2.7-i2v-2026-06-12", true},
		{"wan2.7-r2vx", false}, // 不能被 wan2.7-r2v 误匹配
		{"wan9.9-r2v", false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got, ok := lookupAliRatios(table, tc.model)
			if ok != tc.wantHit {
				t.Fatalf("lookupAliRatios(%q) hit = %v, want %v", tc.model, ok, tc.wantHit)
			}
			if tc.wantHit && got["1080P"] == 1 {
				t.Errorf("%s: 1080P ratio = 1, want a premium over 720P", tc.model)
			}
		})
	}
}

func TestProcessAliOtherRatios_DatedR2V(t *testing.T) {
	ratios, err := ProcessAliOtherRatios(&AliVideoRequest{
		Model:      "wan2.7-r2v-2026-06-12",
		Parameters: &AliVideoParameters{Resolution: "1080P"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ratios["resolution-1080P"] == 0 {
		t.Fatal("dated wan2.7-r2v snapshot must still get the 1080P ratio")
	}
}
