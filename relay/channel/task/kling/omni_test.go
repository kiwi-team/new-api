package kling

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 这些用例对照可灵 3.0 Omni 官方文档的请求/响应结构，锁定 references -> contents 的映射。
// 文档：https://klingai.com/document-api/api/video/3-0-omni/video-omni

func TestBuildOmniRequestPayload_RoleMapping(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model:  "kling-3.0-omni",
		Prompt: "@image_1 的女性走向 @video_1",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/f.png", ID: "image_1"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/r.png", ID: "image_2"},
			{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4", ID: "video_1"},
			{Type: relaycommon.RefTypeElement, Role: relaycommon.RefRoleElement, ElementID: "el-1", ID: "Zhang"},
		},
		Seconds:    "10",
		Resolution: "1080p",
		Audio:      "native",
	}

	payload, err := buildOmniRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// prompt 必须排在首位
	if payload.Contents[0].Type != "prompt" || payload.Contents[0].Text != req.Prompt {
		t.Fatalf("contents[0] = %+v, want prompt content", payload.Contents[0])
	}

	wantTypes := []string{"prompt", "first_frame", "refer_image", "feature_video", "element"}
	if len(payload.Contents) != len(wantTypes) {
		t.Fatalf("contents len = %d, want %d", len(payload.Contents), len(wantTypes))
	}
	for i, want := range wantTypes {
		if payload.Contents[i].Type != want {
			t.Errorf("contents[%d].Type = %q, want %q", i, payload.Contents[i].Type, want)
		}
	}

	// id 必须透传，prompt 里的 @id 指代依赖它
	if payload.Contents[1].ID != "image_1" {
		t.Errorf("first_frame ID = %q, want image_1", payload.Contents[1].ID)
	}
	if payload.Contents[4].ElementID != "el-1" {
		t.Errorf("element ElementID = %q, want el-1", payload.Contents[4].ElementID)
	}
	// element 不应带 url
	if payload.Contents[4].URL != "" {
		t.Errorf("element URL = %q, want empty", payload.Contents[4].URL)
	}

	if payload.Settings.Duration != 10 {
		t.Errorf("duration = %d, want 10", payload.Settings.Duration)
	}
	if payload.Settings.Resolution != "1080p" {
		t.Errorf("resolution = %q, want 1080p", payload.Settings.Resolution)
	}
	if payload.Settings.Audio != "native" {
		t.Errorf("audio = %q, want native", payload.Settings.Audio)
	}
	// 有首帧时不应强塞 aspect_ratio（官方：有首帧/参考视频时该参数非必填）
	if payload.Settings.AspectRatio != "" {
		t.Errorf("aspect_ratio = %q, want empty when a first_frame is present", payload.Settings.AspectRatio)
	}
}

func TestBuildOmniRequestPayload_TextOnlyRequiresAspectRatio(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Model: "kling-3.0-omni", Prompt: "a cat"}
	payload, err := buildOmniRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Settings.AspectRatio == "" {
		t.Error("aspect_ratio must be set when there is no first_frame and no reference video")
	}
	if payload.Settings.Duration != omniDefaultSeconds {
		t.Errorf("duration = %d, want default %d", payload.Settings.Duration, omniDefaultSeconds)
	}
	if payload.Settings.Resolution != omniDefaultResolution {
		t.Errorf("resolution = %q, want default %q", payload.Settings.Resolution, omniDefaultResolution)
	}
}

func TestBuildOmniRequestPayload_RejectsInvalidAudio(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Model: "kling-3.0-omni", Prompt: "x", Audio: "loud"}
	if _, err := buildOmniRequestPayload(req); err == nil {
		t.Fatal("expected an error for invalid audio value")
	}
}

func TestBuildOmniRequestPayload_ElementRequiresElementID(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model:      "kling-3.0-omni",
		Prompt:     "x",
		References: []relaycommon.TaskReference{{Type: relaycommon.RefTypeElement, Role: relaycommon.RefRoleElement}},
	}
	if _, err := buildOmniRequestPayload(req); err == nil {
		t.Fatal("expected an error when element_id is missing")
	}
}

// 未传 references 的旧客户端仍按图片数量映射首帧/尾帧。
func TestBuildOmniRequestPayload_LegacyImages(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model:  "kling-3.0-omni",
		Prompt: "x",
		Images: []string{"https://x/1.png", "https://x/2.png"},
	}
	payload, err := buildOmniRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTypes := []string{"prompt", "first_frame", "last_frame"}
	for i, want := range wantTypes {
		if payload.Contents[i].Type != want {
			t.Errorf("contents[%d].Type = %q, want %q", i, payload.Contents[i].Type, want)
		}
	}
}

func TestParseOmniSubmitResponse(t *testing.T) {
	body := []byte(`{"code":0,"message":"","request_id":"r1","data":{"id":"task-123","status":"submitted"}}`)
	id, err := parseOmniSubmitResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "task-123" {
		t.Fatalf("id = %q, want task-123", id)
	}

	if _, err := parseOmniSubmitResponse([]byte(`{"code":1001,"message":"bad key"}`)); err == nil {
		t.Fatal("expected an error for non-zero code")
	}
}

func TestParseOmniTaskResult(t *testing.T) {
	body := []byte(`{"code":0,"data":{"result":[{"id":"t1","status":"succeeded","outputs":[
		{"type":"video","id":"v1","url":"https://x/v.mp4","watermark_url":"https://x/w.mp4","duration":"7"}]}],"count":1}}`)

	if !isOmniQueryResponse(body) {
		t.Fatal("expected body to be detected as an omni query response")
	}

	ti, err := parseOmniTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ti.Status != "SUCCESS" {
		t.Errorf("status = %q, want SUCCESS", ti.Status)
	}
	if ti.Url != "https://x/v.mp4" {
		t.Errorf("url = %q", ti.Url)
	}
	// 实际时长要回传，供按时长重算计费
	if ti.ActualSeconds != 7 {
		t.Errorf("ActualSeconds = %v, want 7", ti.ActualSeconds)
	}
}

func TestParseOmniTaskResult_Failed(t *testing.T) {
	body := []byte(`{"code":0,"data":{"result":[{"id":"t1","status":"failed","message":"nsfw"}],"count":1}}`)
	ti, err := parseOmniTaskResult(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ti.Status != "FAILURE" {
		t.Errorf("status = %q, want FAILURE", ti.Status)
	}
	if ti.Reason != "nsfw" {
		t.Errorf("reason = %q, want nsfw", ti.Reason)
	}
}

// 旧接口的响应不能被误判成 Omni 结构。
func TestIsOmniQueryResponse_RejectsLegacyShape(t *testing.T) {
	legacy := []byte(`{"code":0,"data":{"task_id":"t1","task_status":"succeed","task_result":{"videos":[{"url":"u"}]}}}`)
	if isOmniQueryResponse(legacy) {
		t.Fatal("legacy kling response must not be detected as an omni query response")
	}
}

func TestOmniPayloadJSONShape(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Model:      "kling-3.0-omni",
		Prompt:     "hi",
		Seconds:    "5",
		Resolution: "1080p",
	}
	payload, err := buildOmniRequestPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	// 官方请求体是 contents/settings/options 三段式
	if _, ok := m["contents"]; !ok {
		t.Error("payload must contain contents")
	}
	if _, ok := m["settings"]; !ok {
		t.Error("payload must contain settings")
	}
}

// 可灵支持两种鉴权方式，均需保留：
//   - API Key（新版，控制台直接生成）：原样作为 Bearer token，不做 JWT 签名
//   - Access Key|Secret Key（旧版）：用 secretKey 对 accessKey 做 HS256 JWT 签名
func TestCreateJWTTokenWithKey_ApiKeyPassthrough(t *testing.T) {
	a := &TaskAdaptor{}
	cases := []string{
		"abcdef1234567890",    // 可灵新版 API Key
		"sk-newapi-relay-key", // new-api 级联
		"Kx9_ABC.def-123",     // 含各种符号但无 "|"
	}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			got, err := a.createJWTTokenWithKey(key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != key {
				t.Errorf("token = %q, want the API Key passed through verbatim", got)
			}
		})
	}
}

func TestCreateJWTTokenWithKey_AccessSecretSignsJWT(t *testing.T) {
	a := &TaskAdaptor{}
	got, err := a.createJWTTokenWithKey("myAccessKey|mySecretKey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "myAccessKey|mySecretKey" {
		t.Fatal("AK|SK must be exchanged for a signed JWT, not passed through")
	}
	// JWT 是三段点分结构
	if strings.Count(got, ".") != 2 {
		t.Errorf("token %q does not look like a JWT", got)
	}
	// 能用 secretKey 验签，且 iss 为 accessKey
	tok, err := jwt.Parse(got, func(*jwt.Token) (interface{}, error) {
		return []byte("mySecretKey"), nil
	})
	if err != nil {
		t.Fatalf("JWT verification failed: %v", err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || claims["iss"] != "myAccessKey" {
		t.Errorf("iss = %v, want myAccessKey", claims["iss"])
	}
}

func TestCreateJWTTokenWithKey_TrimsWhitespace(t *testing.T) {
	a := &TaskAdaptor{}
	got, err := a.createJWTTokenWithKey("  myApiKey  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myApiKey" {
		t.Errorf("token = %q, want whitespace trimmed", got)
	}
}

func TestCreateJWTTokenWithKey_Rejects(t *testing.T) {
	a := &TaskAdaptor{}
	for _, key := range []string{"", "   ", "accessKeyOnly|", "|secretOnly"} {
		t.Run(key, func(t *testing.T) {
			if _, err := a.createJWTTokenWithKey(key); err == nil {
				t.Errorf("expected an error for key %q", key)
			}
		})
	}
}
