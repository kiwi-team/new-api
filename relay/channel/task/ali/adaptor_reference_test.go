package ali

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// r2v 端点只认 input.media，完全不认 img_url；media 顺序必须与 references 一致，
// 否则提示词里的「图1/视频1」会指错素材。
func TestConvertToAliRequestReferenceToVideoBuildsMediaInOrder(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "让图1中的人物出现在视频1的场景里",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
			{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/f.jpg", VoiceURL: "https://x/voice.mp3"},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "reference_image", URL: "https://x/1.jpg"},
		{Type: "reference_video", URL: "https://x/v.mp4"},
		{Type: "first_frame", URL: "https://x/f.jpg", ReferenceVoice: "https://x/voice.mp3"},
	}, aliReq.Input.Media)
	assert.Empty(t, aliReq.Input.ImgURL)
}

func TestConvertToAliRequestWan3AllInOneMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "use all references",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/first.jpg"},
			{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/ref.mp4"},
			{Type: relaycommon.RefTypeAudio, Role: relaycommon.RefRoleReferenceAudio, URL: "https://x/ref.mp3"},
		},
		Resolution: "720p",
		Duration:   -1,
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	assert.Equal(t, "720P", aliReq.Parameters.Resolution)
	assert.Equal(t, -1, aliReq.Parameters.Duration)
	assert.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://x/first.jpg"},
		{Type: "reference_video", URL: "https://x/ref.mp4"},
		{Type: "reference_audio", URL: "https://x/ref.mp3"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan3TextToVideoDoesNotRequireMedia(t *testing.T) {
	aliReq, err := (&TaskAdaptor{}).convertToAliRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video-prime",
		Prompt: "generate a video",
	})

	require.NoError(t, err)
	assert.Empty(t, aliReq.Input.Media)
	assert.Equal(t, "1080P", aliReq.Parameters.Resolution)
	assert.Equal(t, 5, aliReq.Parameters.Duration)
}

func TestProcessAliOtherRatiosWan3Pricing(t *testing.T) {
	for _, modelName := range []string{"wan3.0-video", "wan3.0-video-prime"} {
		for resolution, want := range map[string]float64{"480P": 1, "720P": 2, "1080P": 4} {
			t.Run(modelName+"/"+resolution, func(t *testing.T) {
				ratios, err := ProcessAliOtherRatios(&AliVideoRequest{
					Model: modelName, Parameters: &AliVideoParameters{Resolution: resolution},
				})
				require.NoError(t, err)
				assert.Equal(t, want, ratios["resolution-"+resolution])
			})
		}
	}
}

func TestWan3CompletionBillingUsesReportedResolutionAndDuration(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{UpstreamModelName: "wan3.0-video"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			ModelPrice: 0.3,
			GroupRatio: 1.5,
			OtherRatios: map[string]float64{
				"seconds":          30,
				"resolution-1080P": 4,
			},
		}},
	}
	result := &relaycommon.TaskInfo{ActualSeconds: 8, ActualResolution: "720P"}

	quota := (&TaskAdaptor{}).AdjustBillingOnComplete(task, result)

	want := common.QuotaFromFloat(0.3 * common.QuotaPerUnit * 1.5 * 8 * 2)
	assert.Equal(t, want, quota)
	assert.Equal(t, map[string]float64{"seconds": 8, "resolution-720P": 2},
		task.PrivateData.BillingContext.OtherRatios)
}

func TestParseTaskResultWan3Usage(t *testing.T) {
	body := []byte(`{"output":{"task_status":"SUCCEEDED","video_url":"https://x/out.mp4"},"usage":{"duration":12,"SR":1080}}`)

	result, err := (&TaskAdaptor{}).ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, float64(12), result.ActualSeconds)
	assert.Equal(t, "1080P", result.ActualResolution)
}

func TestValidateWan3RejectsBillingMultiplierOutsideModelBounds(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "duration", body: `{"model":"wan3.0-video","prompt":"x","duration":31}`, code: "invalid_duration"},
		{name: "metadata duration", body: `{"model":"wan3.0-video","prompt":"x","metadata":{"parameters":{"duration":31}}}`, code: "invalid_duration"},
		{name: "resolution", body: `{"model":"wan3.0-video-prime","prompt":"x","resolution":"4k"}`, code: "invalid_resolution"},
	}

	gin.SetMode(gin.TestMode)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAli},
			}

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)

			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, test.code, taskErr.Code)
		})
	}
}

// HappyHorse 的 r2v 接口没有 prompt_extend 参数，传了会被上游判 InvalidParameter。
func TestConvertToAliRequestHappyHorseOmitsPromptExtend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	assert.Nil(t, aliReq.Parameters.PromptExtend)
}

// r2v 无首帧时画面比例无从推断，需要给出官方默认 16:9，否则上游报错。
func TestConvertToAliRequestReferenceToVideoDefaultsRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	assert.Equal(t, "1080P", aliReq.Parameters.Resolution)
	assert.Equal(t, "16:9", aliReq.Parameters.Ratio)
}

// 统一层的 resolution / aspect_ratio / negative_prompt 需要覆盖 size 推断。
func TestConvertToAliRequestUnifiedScalarFields(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "x",
		Image:          "https://x/f.jpg",
		Size:           "480p",
		Resolution:     "1080p",
		AspectRatio:    "9:16",
		NegativePrompt: "模糊",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	assert.Equal(t, "1080P", aliReq.Parameters.Resolution)
	assert.Equal(t, "9:16", aliReq.Parameters.Ratio)
	assert.Equal(t, "模糊", aliReq.Input.NegativePrompt)
}

// 快照版模型名（<base>-<date>）与基础版同价，倍率表必须命中，
// 否则 1080P 会静默按 720P 的 1 倍率计费。
func TestLookupAliRatiosMatchesSnapshotModelNames(t *testing.T) {
	table := map[string]map[string]float64{
		"wan2.7-r2v":  {"1080P": 2},
		"wan2.7-r2vx": {"1080P": 3},
	}

	got, ok := lookupAliRatios(table, "wan2.7-r2v-2026-06-12")
	require.True(t, ok)
	assert.Equal(t, table["wan2.7-r2v"], got)

	got, ok = lookupAliRatios(table, "wan2.7-r2vx")
	require.True(t, ok)
	assert.Equal(t, table["wan2.7-r2vx"], got)

	_, ok = lookupAliRatios(table, "wan2.5-i2v")
	assert.False(t, ok)
}

// r2v 的分辨率倍率直接决定扣费：1080P 必须比基准档贵，
// 且带日期后缀的快照版模型名要能命中同一张倍率表——命不中会静默按 1 倍收费。
func TestProcessAliOtherRatiosReferenceToVideo(t *testing.T) {
	models := []string{
		"wan2.7-r2v",
		"happyhorse-1.0-r2v",
		"happyhorse-1.1-r2v",
		"wan2.7-r2v-2026-06-12", // 快照版，与基础版同价
	}

	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			ratios, err := ProcessAliOtherRatios(&AliVideoRequest{
				Model:      modelName,
				Parameters: &AliVideoParameters{Resolution: "1080P"},
			})
			require.NoError(t, err)

			got, ok := ratios["resolution-1080P"]
			require.True(t, ok, "missing resolution-1080P ratio")
			assert.Greater(t, got, 1.0, "1080P should cost a premium over the 720P baseline")
		})
	}
}

// prompt_extend 只对 HappyHorse 系列省略；万相仍要默认开启智能改写，
// 否则会连带关掉一个既有能力。
func TestConvertToAliRequestWanKeepsPromptExtend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "x",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.NotNil(t, aliReq.Parameters.PromptExtend)
	assert.True(t, *aliReq.Parameters.PromptExtend)
}
