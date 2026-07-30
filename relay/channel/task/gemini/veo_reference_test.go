package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 对照官方文档锁定 Veo 3.1 参考图的能力边界与硬约束。
// https://ai.google.dev/gemini-api/docs/veo#reference-images

func TestSupportsVeoReferenceImages(t *testing.T) {
	cases := map[string]bool{
		"veo-3.1-generate-preview":      true,
		"veo-3.1-fast-generate-preview": true,
		"veo-3.1-lite":                  false, // Lite 不支持
		"veo-3.0-generate-001":          false,
		"veo-2":                         false,
		"gemini-omni-flash-preview":     false,
	}
	for model, want := range cases {
		t.Run(model, func(t *testing.T) {
			if got := SupportsVeoReferenceImages(model); got != want {
				t.Errorf("SupportsVeoReferenceImages(%q) = %v, want %v", model, got, want)
			}
		})
	}
}

// 使用参考图时官方强制 durationSeconds=8、personGeneration=allow_adult。
func TestApplyVeoReferenceConstraints(t *testing.T) {
	instance := map[string]any{}
	params := map[string]any{}
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"}

	if err := ApplyVeoReferenceConstraints(instance, params, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// durationSeconds 归属 parameters（官方 parameters 表所列位置），不应在 instances 内重复
	if params["durationSeconds"] != VeoReferenceDurationSeconds {
		t.Errorf("parameters.durationSeconds = %v, want %d", params["durationSeconds"], VeoReferenceDurationSeconds)
	}
	if _, dup := instance["durationSeconds"]; dup {
		t.Error("durationSeconds must not be duplicated into instances")
	}
	if params["personGeneration"] != VeoReferencePersonGeneration {
		t.Errorf("personGeneration = %v, want %q", params["personGeneration"], VeoReferencePersonGeneration)
	}
}

// 客户端显式传了冲突的时长时应报错，而不是静默改写。
func TestApplyVeoReferenceConstraints_RejectsConflictingDuration(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview", Seconds: "5"}
	err := ApplyVeoReferenceConstraints(map[string]any{}, map[string]any{}, req)
	if err == nil {
		t.Fatal("expected an error when durationSeconds conflicts with the required 8s")
	}
}

func TestApplyVeoReferenceConstraints_AcceptsMatchingDuration(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview", Seconds: "8"}
	if err := ApplyVeoReferenceConstraints(map[string]any{}, map[string]any{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyVeoReferenceConstraints_RejectsConflictingPersonGeneration(t *testing.T) {
	params := map[string]any{"personGeneration": "allow_all"}
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"}
	if err := ApplyVeoReferenceConstraints(map[string]any{}, params, req); err == nil {
		t.Fatal("expected an error when personGeneration conflicts with allow_adult")
	}
}

// 不支持参考图的模型必须报错而不是静默丢弃。
func TestBuildVeoReferenceImages_RejectsUnsupportedModel(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model: "veo-3.0-generate-001",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "data:image/png;base64,AAAA"},
		},
	}
	if _, err := BuildVeoReferenceImages(nil, req, true); err == nil {
		t.Fatal("expected an error for a model without reference-image support")
	}
}

func TestBuildVeoReferenceImages_RejectsTooMany(t *testing.T) {
	refs := make([]relaycommon.TaskReference, 0, 4)
	for i := 0; i < 4; i++ {
		refs = append(refs, relaycommon.TaskReference{
			Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage,
			URL: "data:image/png;base64,AAAA",
		})
	}
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview", References: refs}
	if _, err := BuildVeoReferenceImages(nil, req, true); err == nil {
		t.Fatalf("expected an error for more than %d reference images", VeoMaxReferenceImages)
	}
}

// 没有参考图时返回 nil，不应影响既有的文生/图生视频路径。
func TestBuildVeoReferenceImages_NoRefsIsNil(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"}
	got, err := BuildVeoReferenceImages(nil, req, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// gs:// 直接透传为 gcsUri，不下载。
func TestBuildVeoImageObject_GCSPassthrough(t *testing.T) {
	obj, err := BuildVeoImageObject(nil, "gs://bucket/a.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["gcsUri"] != "gs://bucket/a.png" {
		t.Errorf("gcsUri = %v", obj["gcsUri"])
	}
	if _, ok := obj["bytesBase64Encoded"]; ok {
		t.Error("gs:// reference must not be inlined as base64")
	}
}

func TestBuildVeoImageObject_EmptyIsNil(t *testing.T) {
	obj, err := BuildVeoImageObject(nil, "  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj != nil {
		t.Fatalf("got %v, want nil", obj)
	}
}

// Vertex 与 Gemini API 的 image 字段形状不同，用错会被上游静默忽略后报 "image is empty"。
func TestBuildVeoReferenceImages_ProviderShapes(t *testing.T) {
	newReq := func() *relaycommon.TaskSubmitReq {
		return &relaycommon.TaskSubmitReq{
			Model: "veo-3.1-generate-001",
			References: []relaycommon.TaskReference{{
				Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage,
				URL: "data:image/png;base64,UE5HREFUQS1PTkU=",
			}},
		}
	}

	// Vertex: {bytesBase64Encoded, mimeType}
	got, err := BuildVeoReferenceImages(nil, newReq(), true)
	if err != nil {
		t.Fatalf("vertex: unexpected error: %v", err)
	}
	img := got[0]["image"].(map[string]any)
	if _, ok := img["bytesBase64Encoded"]; !ok {
		t.Errorf("vertex image = %v, want bytesBase64Encoded", img)
	}
	if _, bad := img["inlineData"]; bad {
		t.Error("vertex image must not use Gemini's inlineData wrapper")
	}

	// Gemini API: {inlineData:{data, mimeType}}
	got, err = BuildVeoReferenceImages(nil, newReq(), false)
	if err != nil {
		t.Fatalf("gemini: unexpected error: %v", err)
	}
	img = got[0]["image"].(map[string]any)
	inline, ok := img["inlineData"].(map[string]any)
	if !ok {
		t.Fatalf("gemini image = %v, want inlineData wrapper", img)
	}
	if _, ok := inline["data"]; !ok {
		t.Errorf("gemini inlineData = %v, want data", inline)
	}
	if _, bad := img["bytesBase64Encoded"]; bad {
		t.Error("gemini image must not use Vertex's bytesBase64Encoded")
	}
}

// gs:// 仅 Vertex 支持。
func TestBuildVeoReferenceImages_GcsUri(t *testing.T) {
	mk := func() *relaycommon.TaskSubmitReq {
		return &relaycommon.TaskSubmitReq{
			Model: "veo-3.1-generate-001",
			References: []relaycommon.TaskReference{{
				Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage,
				URL: "gs://bucket/a.png",
			}},
		}
	}
	got, err := BuildVeoReferenceImages(nil, mk(), true)
	if err != nil {
		t.Fatalf("vertex: unexpected error: %v", err)
	}
	if got[0]["image"].(map[string]any)["gcsUri"] != "gs://bucket/a.png" {
		t.Errorf("gcsUri not passed through: %v", got[0]["image"])
	}
	if _, err := BuildVeoReferenceImages(nil, mk(), false); err == nil {
		t.Error("gemini API must reject gs:// reference images")
	}
}

// Veo 3.1 不允许首帧与参考图并存。
func TestBuildVeoReferenceImages_RejectsFrameWithRefs(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model: "veo-3.1-generate-001",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "data:image/png;base64,UE5HREFUQS1PTkU="},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "data:image/png;base64,UE5HREFUQS1UV08="},
		},
	}
	if _, err := BuildVeoReferenceImages(nil, req, true); err == nil {
		t.Fatal("expected an error when first_frame is combined with reference images")
	}
}

// Vertex 的 -001 命名必须被识别为支持参考图。
func TestSupportsVeoReferenceImages_VertexNaming(t *testing.T) {
	for _, m := range []string{"veo-3.1-generate-001", "veo-3.1-fast-generate-001"} {
		if !SupportsVeoReferenceImages(m) {
			t.Errorf("SupportsVeoReferenceImages(%q) = false, want true", m)
		}
	}
	if SupportsVeoReferenceImages("veo-3.1-lite-generate-001") {
		t.Error("lite must not be treated as supporting reference images")
	}
}
