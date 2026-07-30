package gemini

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// 对照 Gemini Omni 官方文档锁定 video_config.task 的选择与标签注入。
// https://ai.google.dev/gemini-api/docs/omni

func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

// 一个最小的合法 base64 图片引用，走裸 base64 分支，不触发网络请求。
const testImageRef = "data:image/png;base64,iVBORw0KGgo="

func TestBuildOmniInput_TaskSelection(t *testing.T) {
	cases := []struct {
		name     string
		refs     []relaycommon.TaskReference
		wantTask string
	}{
		{
			name:     "no reference -> text_to_video",
			refs:     nil,
			wantTask: "text_to_video",
		},
		{
			name: "first frame only -> image_to_video",
			refs: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: testImageRef},
			},
			wantTask: "image_to_video",
		},
		{
			name: "reference images -> reference_to_video",
			refs: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: testImageRef},
			},
			wantTask: "reference_to_video",
		},
		{
			name: "base video -> edit",
			refs: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleBaseVideo, URL: "data:video/mp4;base64,AAAA"},
			},
			wantTask: "edit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &relaycommon.TaskSubmitReq{Prompt: "hello", References: tc.refs}
			_, task, _, err := buildOmniInput(newTestContext(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if task != tc.wantTask {
				t.Fatalf("task = %q, want %q", task, tc.wantTask)
			}
		})
	}
}

// 参考图必须注入 <IMAGE_REF_n>（0-indexed），首帧注入 <FIRST_FRAME>。
// 这是 Omni 强制的结构化标签，不是对用户指代语法的改写。
func TestBuildOmniInput_InjectsRoleTags(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Prompt: "a woman is walking",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: testImageRef},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: testImageRef},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: testImageRef},
		},
	}
	_, _, prompt, err := buildOmniInput(newTestContext(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"<FIRST_FRAME>", "<IMAGE_REF_0>", "<IMAGE_REF_1>"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt %q missing tag %q", prompt, want)
		}
	}
	// 用户原始 prompt 必须完整保留
	if !strings.Contains(prompt, "a woman is walking") {
		t.Errorf("original prompt text was lost: %q", prompt)
	}
}

// 用户已自行书写标签时不再重复注入。
func TestBuildOmniInput_DoesNotDoubleInjectTags(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{
		Prompt: "in the style of <IMAGE_REF_0> a woman walks",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: testImageRef},
		},
	}
	_, _, prompt, err := buildOmniInput(newTestContext(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != req.Prompt {
		t.Errorf("prompt was rewritten to %q, want it left as-is", prompt)
	}
}

// 未传 references 的旧客户端仍按 image_to_video 处理。
func TestBuildOmniInput_LegacyImagesStillImageToVideo(t *testing.T) {
	req := &relaycommon.TaskSubmitReq{Prompt: "x", Images: []string{testImageRef}}
	parts, task, _, err := buildOmniInput(newTestContext(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != "image_to_video" {
		t.Errorf("task = %q, want image_to_video", task)
	}
	if len(parts) != 2 { // 1 image + 1 text
		t.Errorf("parts len = %d, want 2", len(parts))
	}
}

// 纯文生视频时 input 用 prompt 字符串而不是 parts 数组。
func TestBuildOmniRequestBody_TextOnlyUsesStringInput(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Prompt: "a cat"}
	data, err := BuildOmniRequestBody(newTestContext(), req, "gemini-omni-flash-preview")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]any
	if err := common.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if input, ok := m["input"].(string); !ok || input != "a cat" {
		t.Errorf("input = %v, want the prompt string", m["input"])
	}
	if m["background"] != true {
		t.Error("background must be true for async submission")
	}
}
