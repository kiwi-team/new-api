package fal

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestFlux3BilledSecondsIsBoundedToUpstreamEnum(t *testing.T) {
	cases := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want int
	}{
		{"defaults to 5s when unspecified", relaycommon.TaskSubmitReq{}, 5},
		{"duration wins over seconds", relaycommon.TaskSubmitReq{Duration: 12, Seconds: "8"}, 12},
		{"seconds string is used", relaycommon.TaskSubmitReq{Seconds: "8"}, 8},
		{"below minimum is raised", relaycommon.TaskSubmitReq{Duration: 1}, 5},
		{"above maximum is capped", relaycommon.TaskSubmitReq{Duration: 3600}, 20},
		{"huge seconds string is capped", relaycommon.TaskSubmitReq{Seconds: "999999"}, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, flux3BilledSeconds(tc.req))
		})
	}
}

func TestFlux3VideoRequestBodyRoutesMaterialsByAction(t *testing.T) {
	first := relaycommon.TaskReference{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://a/first.png"}
	last := relaycommon.TaskReference{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "https://a/last.png"}

	cases := []struct {
		name   string
		action string
		req    relaycommon.TaskSubmitReq
		want   map[string]any
	}{
		{
			name:   "text to video sends no image",
			action: constant.TaskActionTextGenerate,
			req:    relaycommon.TaskSubmitReq{Prompt: "a red panda"},
			want:   map[string]any{"prompt": "a red panda", "aspect_ratio": "auto", "resolution": "720p", "duration": float64(5)},
		},
		{
			name:   "image to video sends image_url",
			action: constant.TaskActionGenerate,
			req: relaycommon.TaskSubmitReq{
				Prompt: "a red panda", Duration: 8, Resolution: "1080p", AspectRatio: "16:9",
				References: []relaycommon.TaskReference{first},
			},
			want: map[string]any{
				"prompt": "a red panda", "aspect_ratio": "16:9", "resolution": "1080p",
				"duration": float64(8), "image_url": "https://a/first.png",
			},
		},
		{
			name:   "first last frame sends start and end urls",
			action: constant.TaskActionFirstTailGenerate,
			req: relaycommon.TaskSubmitReq{
				Prompt: "a red panda", Duration: 10,
				References: []relaycommon.TaskReference{first, last},
			},
			want: map[string]any{
				"prompt": "a red panda", "aspect_ratio": "auto", "resolution": "720p", "duration": float64(10),
				"start_image_url": "https://a/first.png", "end_image_url": "https://a/last.png",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := Flux3VideoRequestBody(tc.req, tc.action)
			require.NoError(t, err)
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, common.Unmarshal(raw, &got))
			assert.Equal(t, tc.want, got)
		})
	}
}

// metadata 是厂商透传通道，绝不能从这里改写 duration —— duration 是按秒计费的乘数，
// EstimateBilling 只看 req.Duration/req.Seconds，metadata 覆盖会让用户白拿时长。
func TestFlux3VideoRequestBodyMetadataCannotOverrideBilledDuration(t *testing.T) {
	audioOff := false
	req := relaycommon.TaskSubmitReq{
		Prompt:   "a red panda",
		Duration: 6,
		Metadata: map[string]any{
			"duration":         20,
			"image_url":        "https://evil/injected.png",
			"generate_audio":   audioOff,
			"safety_tolerance": 4,
		},
	}

	reader, err := Flux3VideoRequestBody(req, constant.TaskActionTextGenerate)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)

	var got Flux3VideoRequest
	require.NoError(t, common.Unmarshal(raw, &got))

	assert.Equal(t, flux3BilledSeconds(req), got.Duration)
	assert.Empty(t, got.ImageURL, "metadata must not inject materials for text-to-video")
	require.NotNil(t, got.GenerateAudio)
	assert.False(t, *got.GenerateAudio, "explicit false in metadata must reach upstream")
	require.NotNil(t, got.SafetyTolerance)
	assert.Equal(t, 4, *got.SafetyTolerance)
}

func TestFlux3EndpointMatchesAction(t *testing.T) {
	assert.Equal(t, "text-to-video", flux3Endpoint(constant.TaskActionTextGenerate))
	assert.Equal(t, "image-to-video", flux3Endpoint(constant.TaskActionGenerate))
	assert.Equal(t, "first-last-frame-to-video", flux3Endpoint(constant.TaskActionFirstTailGenerate))
}

// 查询地址必须只由「模型名 + 上游 request ID」决定。FetchTask 拿到的 task_id 是上游
// request ID，而 task_id 列存的是公开 ID（task_xxxx），一旦退回查库就必然查不到行，
// 实时查询会静默降级成通用 TaskDto 结构、后台轮询则直接报错。
func TestFalQueueStatusURLBuildsFromModelName(t *testing.T) {
	const upstreamID = "9712c5c7-4409-4d67-9c3f-46b8467e06a3"
	cases := []struct {
		modelName string
		want      string
	}{
		{Flux3VideoModel, "https://queue.fal.run/blackforestlabs/flux-3/requests/" + upstreamID},
		{"hunyuan-video-v1.5", "https://queue.fal.run/fal-ai/hunyuan-video-v1.5/requests/" + upstreamID},
		{"imagineart-1.5-preview", "https://queue.fal.run/imagineart/imagineart-1.5-preview/requests/" + upstreamID},
		{"flux-1-kontext-pro", "https://queue.fal.run/fal-ai/flux-pro/requests/" + upstreamID},
	}
	for _, tc := range cases {
		t.Run(tc.modelName, func(t *testing.T) {
			assert.Equal(t, tc.want, falQueueStatusURL(tc.modelName, upstreamID))
		})
	}
}
