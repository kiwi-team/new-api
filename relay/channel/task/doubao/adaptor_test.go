package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 对照火山方舟官方文档锁定 references -> content[] 的映射。
// https://docs.volcengine.com/docs/82379/1520757

func TestConvertToRequestPayloadReferences(t *testing.T) {
	cases := []struct {
		name       string
		req        relaycommon.TaskSubmitReq
		wantTypes  []string
		wantRoles  []string
		assertMore func(t *testing.T, r *requestPayload)
	}{
		{
			name: "seedance2 multi modal",
			req: relaycommon.TaskSubmitReq{
				Model:  "doubao-seedance-2-0-260128",
				Prompt: "一段广告",
				References: []relaycommon.TaskReference{
					{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "https://x/1.jpg"},
					{Type: relaycommon.RefTypeVideo, Role: relaycommon.RefRoleReferenceVideo, URL: "https://x/v.mp4"},
					{Type: relaycommon.RefTypeAudio, Role: relaycommon.RefRoleReferenceAudio, URL: "https://x/a.mp3"},
				},
			},
			wantTypes: []string{"text", "image_url", "video_url", "audio_url"},
			wantRoles: []string{"", "reference_image", "reference_video", "reference_audio"},
			assertMore: func(t *testing.T, r *requestPayload) {
				require.NotNil(t, r.Content[2].VideoURL)
				assert.Equal(t, "https://x/v.mp4", r.Content[2].VideoURL.URL)
				require.NotNil(t, r.Content[3].AudioURL)
				assert.Equal(t, "https://x/a.mp3", r.Content[3].AudioURL.URL)
			},
		},
		{
			name: "first and last frame keep order",
			req: relaycommon.TaskSubmitReq{
				Model:  "doubao-seedance-2-0-260128",
				Prompt: "x",
				References: []relaycommon.TaskReference{
					{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://x/f.jpg"},
					{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "https://x/l.jpg"},
				},
			},
			wantTypes: []string{"text", "image_url", "image_url"},
			wantRoles: []string{"", "first_frame", "last_frame"},
		},
		{
			name: "legacy images fall back to image_role",
			req: relaycommon.TaskSubmitReq{
				Model:    "doubao-seedance-1-0-pro-250528",
				Prompt:   "x",
				Images:   []string{"https://x/1.jpg"},
				Metadata: map[string]interface{}{"image_role": "reference_image"},
			},
			wantTypes: []string{"text", "image_url"},
			wantRoles: []string{"", "reference_image"},
		},
	}

	a := &TaskAdaptor{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			r, err := a.convertToRequestPayload(&req)
			require.NoError(t, err)
			require.Len(t, r.Content, len(tc.wantTypes))
			for i := range tc.wantTypes {
				assert.Equal(t, tc.wantTypes[i], r.Content[i].Type, "content[%d].Type", i)
				assert.Equal(t, tc.wantRoles[i], r.Content[i].Role, "content[%d].Role", i)
			}
			if tc.assertMore != nil {
				tc.assertMore(t, r)
			}
		})
	}
}

// Seedance 2.0 不支持 seed / camera_fixed / frames，必须剔除，否则上游强校验报错；
// 老模型则要保留这些参数。
func TestConvertToRequestPayloadSeedance2StripsUnsupportedParams(t *testing.T) {
	a := &TaskAdaptor{}
	metadata := map[string]interface{}{
		"seed":         float64(42),
		"frames":       float64(121),
		"camera_fixed": true,
	}

	seedance2 := relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-0-260128", Prompt: "x", Metadata: metadata}
	r, err := a.convertToRequestPayload(&seedance2)
	require.NoError(t, err)
	assert.Nil(t, r.Seed)
	assert.Nil(t, r.Frames)
	assert.Nil(t, r.CameraFixed)

	legacy := relaycommon.TaskSubmitReq{Model: "doubao-seedance-1-0-pro-250528", Prompt: "x", Metadata: metadata}
	r, err = a.convertToRequestPayload(&legacy)
	require.NoError(t, err)
	require.NotNil(t, r.Seed)
	require.NotNil(t, r.Frames)
	require.NotNil(t, r.CameraFixed)
	assert.True(t, bool(*r.CameraFixed))
}

// duration=-1 表示由模型自选时长，仅 Seedance 2.0 系列可原样透传；
// 其他模型的 -1 不得进入上游请求体。
func TestConvertToRequestPayloadModelChosenDuration(t *testing.T) {
	a := &TaskAdaptor{}

	seedance2 := relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: -1}
	r, err := a.convertToRequestPayload(&seedance2)
	require.NoError(t, err)
	require.NotNil(t, r.Duration)
	assert.Equal(t, -1, int(*r.Duration))

	legacy := relaycommon.TaskSubmitReq{Model: "doubao-seedance-1-0-pro-250528", Prompt: "x", Duration: -1}
	r, err = a.convertToRequestPayload(&legacy)
	require.NoError(t, err)
	assert.Nil(t, r.Duration)
}

// 统一层的 resolution / aspect_ratio / audio 需要翻译成方舟方言。
func TestConvertToRequestPayloadUnifiedScalarFields(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:       "doubao-seedance-2-0-260128",
		Prompt:      "x",
		Resolution:  "1080P",
		AspectRatio: "16:9",
		Audio:       "on",
	}
	r, err := a.convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "1080p", r.Resolution)
	assert.Equal(t, "16:9", r.Ratio)
	require.NotNil(t, r.GenerateAudio)
	assert.True(t, bool(*r.GenerateAudio))

	req.Audio = "off"
	r, err = a.convertToRequestPayload(&req)
	require.NoError(t, err)
	require.NotNil(t, r.GenerateAudio)
	assert.False(t, bool(*r.GenerateAudio))
}

// camera_fixed 是文档字段名，camerafixed 是历史写法，两者都要接受。
func TestConvertToRequestPayloadAcceptsBothCameraFixedSpellings(t *testing.T) {
	a := &TaskAdaptor{}
	for _, key := range []string{"camera_fixed", "camerafixed"} {
		t.Run(key, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{
				Model:    "doubao-seedance-1-0-pro-250528",
				Prompt:   "x",
				Metadata: map[string]interface{}{key: true},
			}
			r, err := a.convertToRequestPayload(&req)
			require.NoError(t, err)
			require.NotNil(t, r.CameraFixed)
			assert.True(t, bool(*r.CameraFixed))
		})
	}
}

// duration=-1 时上游返回的实际时长要回填，供日志与后续计费核对使用。
func TestParseTaskResultReportsActualDuration(t *testing.T) {
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult([]byte(`{"id":"t1","status":"succeeded","duration":12,"content":{"video_url":"https://x/v.mp4"}}`))
	require.NoError(t, err)
	assert.Equal(t, float64(12), info.ActualSeconds)
}
