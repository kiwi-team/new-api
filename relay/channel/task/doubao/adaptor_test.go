package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 对照火山方舟官方文档锁定 references -> content[] 的映射。
// https://docs.volcengine.com/docs/82379/1520757

func TestConvertToRequestPayload_Seedance2MultiModal(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "一段广告",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
			{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4"},
			{Type: relaycommon.RefTypeAudio, Role: relaycommon.RefRoleReferenceAudio, URL: "https://x/a.mp3"},
		},
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantTypes := []string{"text", "image_url", "video_url", "audio_url"}
	if len(r.Content) != len(wantTypes) {
		t.Fatalf("content len = %d, want %d", len(r.Content), len(wantTypes))
	}
	for i, want := range wantTypes {
		if r.Content[i].Type != want {
			t.Errorf("content[%d].Type = %q, want %q", i, r.Content[i].Type, want)
		}
	}
	if r.Content[1].Role != "reference_image" {
		t.Errorf("image role = %q, want reference_image", r.Content[1].Role)
	}
	if r.Content[2].Role != "reference_video" || r.Content[2].VideoURL == nil {
		t.Errorf("video content = %+v", r.Content[2])
	}
	if r.Content[3].Role != "reference_audio" || r.Content[3].AudioURL == nil {
		t.Errorf("audio content = %+v", r.Content[3])
	}
}

func TestConvertToRequestPayload_FirstLastFrame(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/f.jpg"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "https://x/l.jpg"},
		},
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Content[1].Role != "first_frame" || r.Content[2].Role != "last_frame" {
		t.Fatalf("roles = %q,%q", r.Content[1].Role, r.Content[2].Role)
	}
}

// Seedance 2.0 不支持 seed / camera_fixed / frames，必须剔除，否则强校验报错。
func TestConvertToRequestPayload_Seedance2StripsUnsupportedParams(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "x",
		Metadata: map[string]any{
			"seed":         float64(42),
			"camera_fixed": true,
			"frames":       float64(121),
		},
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Seed != 0 {
		t.Errorf("seed = %d, want 0 (unsupported on Seedance 2.0)", r.Seed)
	}
	if r.CameraFixed != nil {
		t.Errorf("camera_fixed = %v, want nil (unsupported on Seedance 2.0)", *r.CameraFixed)
	}
	if r.Frames != 0 {
		t.Errorf("frames = %d, want 0 (unsupported on Seedance 2.0)", r.Frames)
	}
}

// 老模型仍应保留 seed / camera_fixed。
func TestConvertToRequestPayload_LegacyModelKeepsSeed(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-1-0-pro-250528",
		Prompt:   "x",
		Metadata: map[string]any{"seed": float64(42), "camera_fixed": true},
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Seed != dto.IntValue(42) {
		t.Errorf("seed = %d, want 42", r.Seed)
	}
	if r.CameraFixed == nil || !bool(*r.CameraFixed) {
		t.Error("camera_fixed should be preserved on legacy models")
	}
}

// 文档字段名是 camera_fixed；历史代码只认 camerafixed，两者都要接受。
func TestConvertToRequestPayload_AcceptsBothCameraFixedSpellings(t *testing.T) {
	a := &TaskAdaptor{}
	for _, key := range []string{"camera_fixed", "camerafixed"} {
		t.Run(key, func(t *testing.T) {
			req := &relaycommon.TaskSubmitReq{
				Model:    "doubao-seedance-1-0-pro-250528",
				Prompt:   "x",
				Metadata: map[string]any{key: true},
			}
			r, err := a.convertToRequestPayload(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.CameraFixed == nil || !bool(*r.CameraFixed) {
				t.Errorf("camera_fixed not picked up from %q", key)
			}
		})
	}
}

// duration=-1（模型自选时长）必须原样透传给 Seedance 2.0。
func TestConvertToRequestPayload_PassesThroughModelChosenDuration(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-0-260128",
		Prompt:  "x",
		Seconds: "-1",
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Duration != dto.IntValue(-1) {
		t.Errorf("duration = %d, want -1", r.Duration)
	}
}

// 非 Seedance 2.0 不应把 -1 透传上去。
func TestConvertToRequestPayload_LegacyModelDropsMinusOne(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-1-0-pro-250528",
		Prompt:  "x",
		Seconds: "-1",
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Duration != 0 {
		t.Errorf("duration = %d, want 0 (legacy models do not support -1)", r.Duration)
	}
}

// audio 三态语义映射到方舟的 generate_audio 布尔。
func TestConvertToRequestPayload_AudioMapping(t *testing.T) {
	a := &TaskAdaptor{}
	cases := map[string]bool{"native": true, "original": true, "off": false}
	for audio, want := range cases {
		t.Run(audio, func(t *testing.T) {
			req := &relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-0-260128", Prompt: "x", Audio: audio}
			r, err := a.convertToRequestPayload(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.GenerateAudio == nil {
				t.Fatal("generate_audio not set")
			}
			if bool(*r.GenerateAudio) != want {
				t.Errorf("generate_audio = %v, want %v", *r.GenerateAudio, want)
			}
		})
	}
}

// 未传 references 的旧客户端仍走 image_role 语义。
func TestConvertToRequestPayload_LegacyImageRole(t *testing.T) {
	a := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-1-0-pro-250528",
		Prompt:   "x",
		Images:   []string{"https://x/1.jpg", "https://x/2.jpg"},
		Metadata: map[string]any{"image_role": "first_frame,last_frame"},
	}
	r, err := a.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Content[1].Role != "first_frame" || r.Content[2].Role != "last_frame" {
		t.Fatalf("roles = %q,%q", r.Content[1].Role, r.Content[2].Role)
	}
}

// 实际时长必须回传，才能按时长重算计费。
func TestParseTaskResult_ReportsActualDuration(t *testing.T) {
	a := &TaskAdaptor{}
	body := []byte(`{"id":"t1","status":"succeeded","duration":7,
		"content":{"video_url":"https://x/v.mp4"},"usage":{"total_tokens":100}}`)
	ti, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ti.ActualSeconds != 7 {
		t.Errorf("ActualSeconds = %v, want 7", ti.ActualSeconds)
	}
	if ti.Url != "https://x/v.mp4" {
		t.Errorf("url = %q", ti.Url)
	}
}
