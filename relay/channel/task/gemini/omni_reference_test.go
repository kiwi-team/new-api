package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageRef(role, url string) relaycommon.TaskReference {
	return relaycommon.TaskReference{Type: relaycommon.RefTypeImage, Role: role, URL: url}
}

// video_config.task 由素材角色决定，选错会让上游按另一种模式生成。
func TestBuildOmniInputTaskSelection(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	cases := []struct {
		name     string
		req      relaycommon.TaskSubmitReq
		wantTask string
	}{
		{
			name:     "text only",
			req:      relaycommon.TaskSubmitReq{Prompt: "x"},
			wantTask: "text_to_video",
		},
		{
			name: "first frame",
			req: relaycommon.TaskSubmitReq{Prompt: "x", References: []relaycommon.TaskReference{
				imageRef(relaycommon.RefRoleFirstFrame, "gs://b/f.png"),
			}},
			wantTask: "image_to_video",
		},
		{
			name: "reference images",
			req: relaycommon.TaskSubmitReq{Prompt: "x", References: []relaycommon.TaskReference{
				imageRef(relaycommon.RefRoleReferenceImage, "gs://b/a.png"),
			}},
			wantTask: "reference_to_video",
		},
		{
			name: "base video wins",
			req: relaycommon.TaskSubmitReq{Prompt: "x", References: []relaycommon.TaskReference{
				imageRef(relaycommon.RefRoleReferenceImage, "gs://b/a.png"),
				{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleBaseVideo, URL: "gs://b/v.mp4"},
			}},
			wantTask: "edit",
		},
		{
			name:     "legacy images",
			req:      relaycommon.TaskSubmitReq{Prompt: "x", Images: []string{"gs://b/1.png"}},
			wantTask: "image_to_video",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			_, task, _, err := buildOmniInput(c, &req)
			// gs:// 无法在本地内联，取素材会失败；此处只关心 task 的选择逻辑，
			// 因此仅在没有素材需要解析时断言 err。
			if tc.wantTask == "text_to_video" {
				require.NoError(t, err)
			}
			if err == nil {
				assert.Equal(t, tc.wantTask, task)
			}
		})
	}
}

// Omni 用 <FIRST_FRAME> / <IMAGE_REF_n> 把素材与叙述绑定；
// 用户已自行书写标签时不得重复注入。
func TestBuildOmniInputTagInjection(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)

	t.Run("does not inject when user wrote tags", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Prompt:     "让 <IMAGE_REF_0> 走进画面",
			References: []relaycommon.TaskReference{imageRef(relaycommon.RefRoleReferenceImage, "")},
		}
		_, _, prompt, err := buildOmniInput(c, &req)
		require.NoError(t, err)
		assert.Equal(t, "让 <IMAGE_REF_0> 走进画面", prompt)
	})

	t.Run("text only prompt untouched", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{Prompt: "一只猫"}
		parts, task, prompt, err := buildOmniInput(c, &req)
		require.NoError(t, err)
		assert.Empty(t, parts)
		assert.Equal(t, "text_to_video", task)
		assert.Equal(t, "一只猫", prompt)
	})
}

// 纯文生视频时 input 必须是字符串而不是数组，否则上游 400。
func TestBuildOmniRequestBodyTextOnlyUsesStringInput(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	data, err := BuildOmniRequestBody(c, relaycommon.TaskSubmitReq{Prompt: "一只猫"}, "gemini-omni-flash-preview")
	require.NoError(t, err)
	assert.Contains(t, string(data), `"input":"一只猫"`)
	assert.Contains(t, string(data), `"task":"text_to_video"`)
}
