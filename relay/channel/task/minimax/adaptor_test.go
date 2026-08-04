package minimax

import (
	"encoding/json"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 对照 MiniMax v2 官方文档锁定四种模式的 content[] 组装。
// https://platform.minimaxi.com/docs/api-reference/video-generation-v2-create

func build(t *testing.T, req *relaycommon.TaskSubmitReq) *requestPayload {
	t.Helper()
	if err := validateRequest(req); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	p, err := buildRequestPayload(req)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return p
}

// 模式一：文生视频。ratio 必填且不能 adaptive。
func TestBuild_TextToVideo(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "镜头拍摄一个女性坐在咖啡馆里",
		Seconds: "5", AspectRatio: "16:9",
	})
	if len(p.Content) != 1 || p.Content[0].Type != ContentTypeText {
		t.Fatalf("content = %+v, want a single text item", p.Content)
	}
	if p.Ratio != "16:9" || p.Resolution != "2K" || p.Duration != 5 {
		t.Errorf("ratio=%q resolution=%q duration=%d", p.Ratio, p.Resolution, p.Duration)
	}
}

// 未指定 ratio 的文生视频要自动补默认值（不能留空，也不能是 adaptive）。
func TestBuild_TextToVideoDefaultsRatio(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{Model: "MiniMax-H3", Prompt: "x"})
	if p.Ratio == "" || p.Ratio == "adaptive" {
		t.Fatalf("ratio = %q, want an explicit non-adaptive default", p.Ratio)
	}
}

func TestValidate_TextToVideoRejectsAdaptive(t *testing.T) {
	err := validateRequest(&relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x", AspectRatio: "adaptive",
	})
	if err == nil {
		t.Fatal("expected an error: t2v cannot use adaptive ratio")
	}
}

// 模式二：首帧图生视频。ratio 由图片决定，不应下发。
func TestBuild_ImageToVideo(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "Contemporary dance", Seconds: "5",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleFirstFrame, URL: "https://x/1.png"},
		},
	})
	if len(p.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(p.Content))
	}
	if p.Content[1].Type != ContentTypeImageURL || p.Content[1].Role != RoleFirstFrame {
		t.Errorf("content[1] = %+v", p.Content[1])
	}
	if p.Content[1].ImageURL == nil || p.Content[1].ImageURL.URL != "https://x/1.png" {
		t.Errorf("image_url = %+v", p.Content[1].ImageURL)
	}
	if p.Ratio != "" {
		t.Errorf("ratio = %q, want empty (derived from the image)", p.Ratio)
	}
}

// 模式三：首尾帧。
func TestBuild_FirstLastFrame(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "A little girl grows up.", Seconds: "5",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleFirstFrame, URL: "https://x/a.jpg"},
			{Type: "image", Role: relaycommon.RefRoleLastFrame, URL: "https://x/b.jpg"},
		},
	})
	if p.Content[1].Role != RoleFirstFrame || p.Content[2].Role != RoleLastFrame {
		t.Fatalf("roles = %q,%q", p.Content[1].Role, p.Content[2].Role)
	}
}

// 模式四：多模态参考（图 + 视频 + 音频）。
func TestBuild_ReferenceToVideo(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "the model walks", Seconds: "5",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleReferenceImage, URL: "https://x/i.png"},
			{Type: "video", Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4"},
			{Type: "audio", Role: relaycommon.RefRoleReferenceAudio, URL: "https://x/a.mp3"},
		},
	})
	wantTypes := []string{ContentTypeText, ContentTypeImageURL, ContentTypeVideoURL, ContentTypeAudioURL}
	if len(p.Content) != len(wantTypes) {
		t.Fatalf("content len = %d, want %d", len(p.Content), len(wantTypes))
	}
	for i, w := range wantTypes {
		if p.Content[i].Type != w {
			t.Errorf("content[%d].Type = %q, want %q", i, p.Content[i].Type, w)
		}
	}
	if p.Content[2].VideoURL == nil || p.Content[3].AudioURL == nil {
		t.Error("video_url/audio_url not populated")
	}
	if p.Content[1].Role != RoleReferenceImage {
		t.Errorf("role = %q", p.Content[1].Role)
	}
}

// 官方约束：首尾帧与参考素材互斥。
func TestValidate_FramesAndRefsAreExclusive(t *testing.T) {
	err := validateRequest(&relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleFirstFrame, URL: "a"},
			{Type: "image", Role: relaycommon.RefRoleReferenceImage, URL: "b"},
		},
	})
	if err == nil {
		t.Fatal("expected an error: frames cannot mix with reference materials")
	}
}

// 官方约束：音频不能单独输入。
func TestValidate_AudioAlone(t *testing.T) {
	err := validateRequest(&relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: "audio", Role: relaycommon.RefRoleReferenceAudio, URL: "a"},
		},
	})
	if err == nil {
		t.Fatal("expected an error: audio cannot be submitted alone")
	}
}

func TestValidate_Counts(t *testing.T) {
	mk := func(role string, n int) []relaycommon.TaskReference {
		out := make([]relaycommon.TaskReference, 0, n)
		typ := "image"
		if role == relaycommon.RefRoleReferenceVideo {
			typ = "video"
		} else if role == relaycommon.RefRoleReferenceAudio {
			typ = "audio"
		}
		for i := 0; i < n; i++ {
			out = append(out, relaycommon.TaskReference{Type: typ, Role: role, URL: "u"})
		}
		return out
	}
	cases := []struct {
		name string
		refs []relaycommon.TaskReference
	}{
		{"10 reference images", mk(relaycommon.RefRoleReferenceImage, 10)},
		{"4 reference videos", append(mk(relaycommon.RefRoleReferenceImage, 1), mk(relaycommon.RefRoleReferenceVideo, 4)...)},
		{"4 reference audios", append(mk(relaycommon.RefRoleReferenceImage, 1), mk(relaycommon.RefRoleReferenceAudio, 4)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRequest(&relaycommon.TaskSubmitReq{
				Model: "MiniMax-H3", Prompt: "x", References: tc.refs,
			}); err == nil {
				t.Fatal("expected a count-limit error")
			}
		})
	}
}

func TestValidate_Duration(t *testing.T) {
	for _, d := range []string{"3", "16"} {
		if err := validateRequest(&relaycommon.TaskSubmitReq{
			Model: "MiniMax-H3", Prompt: "x", Seconds: d,
		}); err == nil {
			t.Errorf("duration %s should be rejected", d)
		}
	}
	for _, d := range []string{"4", "5", "15"} {
		if err := validateRequest(&relaycommon.TaskSubmitReq{
			Model: "MiniMax-H3", Prompt: "x", Seconds: d, AspectRatio: "16:9",
		}); err != nil {
			t.Errorf("duration %s should be accepted: %v", d, err)
		}
	}
}

func TestValidate_InvalidRatio(t *testing.T) {
	if err := validateRequest(&relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x", AspectRatio: "5:4",
	}); err == nil {
		t.Fatal("expected an error for an unsupported ratio")
	}
}

// 未传 references 的旧客户端仍按图片数量映射首/尾帧。
func TestBuild_LegacyImages(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x", Images: []string{"https://x/1.png", "https://x/2.png"},
	})
	if p.Content[1].Role != RoleFirstFrame || p.Content[2].Role != RoleLastFrame {
		t.Fatalf("roles = %q,%q", p.Content[1].Role, p.Content[2].Role)
	}
}

func TestParseTaskResult_Succeeded(t *testing.T) {
	body := []byte(`{"task":{"id":"t1","model":"MiniMax-H3","status":"succeeded",
	  "content":{"url":"https://cdn/v.mp4"},"duration":5,"resolution":"2K","ratio":"16:9",
	  "usage":{"total_seconds":7,"input_seconds":2,"output_seconds":5}}}`)
	ti, err := (&TaskAdaptor{}).ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ti.Status != "SUCCESS" || ti.Url != "https://cdn/v.mp4" {
		t.Errorf("status=%q url=%q", ti.Status, ti.Url)
	}
	// 计费按 usage.total_seconds（含输入素材时长）
	if ti.ActualSeconds != 7 {
		t.Errorf("ActualSeconds = %v, want 7", ti.ActualSeconds)
	}
}

func TestParseTaskResult_Statuses(t *testing.T) {
	cases := map[string]string{
		"queued": "QUEUED", "running": "IN_PROGRESS",
		"failed": "FAILURE", "cancelled": "FAILURE", "expired": "FAILURE",
	}
	for status, want := range cases {
		t.Run(status, func(t *testing.T) {
			body := []byte(`{"task":{"id":"t1","status":"` + status + `"}}`)
			ti, err := (&TaskAdaptor{}).ParseTaskResult(body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(ti.Status) != want {
				t.Errorf("status = %q, want %q", ti.Status, want)
			}
		})
	}
}

func TestParseTaskResult_FailureCarriesReason(t *testing.T) {
	body := []byte(`{"task":{"id":"t1","status":"failed",
	  "error":{"code":"1026","message":"video description contains sensitive content"}}}`)
	ti, err := (&TaskAdaptor{}).ParseTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ti.Reason == "" || ti.Status != "FAILURE" {
		t.Fatalf("reason=%q status=%q", ti.Reason, ti.Status)
	}
}

// v2 用 OpenAI 风格错误体（没有 base_resp），必须能识别。
func TestParseOaiError(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"rate_limit_error",
	  "message":"rate limit, please retry later (1002)","http_code":"429"},
	  "request_id":"abc"}`)
	msg, ok := parseOaiError(body)
	if !ok || msg == "" {
		t.Fatalf("failed to parse the v2 error envelope: %q", msg)
	}
	if _, err := (&TaskAdaptor{}).ParseTaskResult(body); err == nil {
		t.Error("ParseTaskResult should surface the upstream error")
	}
	// 正常任务体不能被误判为错误
	if _, ok := parseOaiError([]byte(`{"task":{"id":"t","status":"queued"}}`)); ok {
		t.Error("a normal task body must not be detected as an error")
	}
}

func TestPayloadJSONShape(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x", AspectRatio: "16:9",
	})
	b, _ := json.Marshal(p)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, k := range []string{"model", "content", "resolution", "duration"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload missing required field %q", k)
		}
	}
}

// 参考生视频官方默认 adaptive，网关不应代客户端指定 ratio。
func TestBuild_ReferenceToVideoOmitsRatio(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleReferenceImage, URL: "https://x/i.png"},
		},
	})
	if p.Ratio != "" {
		t.Fatalf("ratio = %q, want empty so upstream applies its adaptive default", p.Ratio)
	}
}

// 但客户端显式指定时必须透传。
func TestBuild_ExplicitRatioWins(t *testing.T) {
	p := build(t, &relaycommon.TaskSubmitReq{
		Model: "MiniMax-H3", Prompt: "x", AspectRatio: "9:16",
		References: []relaycommon.TaskReference{
			{Type: "image", Role: relaycommon.RefRoleReferenceImage, URL: "https://x/i.png"},
		},
	})
	if p.Ratio != "9:16" {
		t.Fatalf("ratio = %q, want 9:16", p.Ratio)
	}
}
