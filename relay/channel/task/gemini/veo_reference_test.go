package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 官方只在 Veo 3.1（非 Lite）上提供参考图；命中错误的模型会被上游静默忽略，
// 所以这里必须在本地判死。
func TestSupportsVeoReferenceImages(t *testing.T) {
	cases := map[string]bool{
		"veo-3.1-generate-preview":      true,
		"veo-3-1-generate-001":          true,
		"veo-3.1-fast-generate-preview": true,
		"veo-3.1-lite":                  false,
		"veo-3.0-generate-001":          false,
		"veo-2.0-generate-001":          false,
		"gemini-omni-flash-preview":     false,
		"":                              false,
	}
	for model, want := range cases {
		t.Run(model, func(t *testing.T) {
			assert.Equal(t, want, SupportsVeoReferenceImages(model))
		})
	}
}

// 使用参考图时官方强制 durationSeconds=8 且 personGeneration=allow_adult。
// 客户端传了冲突值要报错，而不是被静默改写。
func TestApplyVeoReferenceConstraints(t *testing.T) {
	cases := []struct {
		name    string
		req     relaycommon.TaskSubmitReq
		params  VeoParameters
		wantErr bool
	}{
		{
			name:   "unset duration gets forced to 8",
			req:    relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"},
			params: VeoParameters{},
		},
		{
			name:   "matching duration accepted",
			req:    relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview", Duration: 8},
			params: VeoParameters{},
		},
		{
			name:    "conflicting request duration rejected",
			req:     relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview", Duration: 6},
			params:  VeoParameters{},
			wantErr: true,
		},
		{
			name:    "conflicting metadata duration rejected",
			req:     relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"},
			params:  VeoParameters{DurationSeconds: 4},
			wantErr: true,
		},
		{
			name:    "conflicting personGeneration rejected",
			req:     relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"},
			params:  VeoParameters{PersonGeneration: "dont_allow"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := tc.params
			req := tc.req
			err := ApplyVeoReferenceConstraints(&params, &req)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, VeoReferenceDurationSeconds, params.DurationSeconds)
			assert.Equal(t, VeoReferencePersonGeneration, params.PersonGeneration)
		})
	}
}

func TestBuildVeoReferenceImages(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)

	t.Run("no refs returns nil", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"}
		got, err := BuildVeoReferenceImages(c, &req, false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("unsupported model rejected", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model: "veo-3.0-generate-001",
			References: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "gs://b/a.png"},
			},
		}
		_, err := BuildVeoReferenceImages(c, &req, true)
		require.Error(t, err)
	})

	t.Run("too many rejected", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{Model: "veo-3.1-generate-preview"}
		for i := 0; i < VeoMaxReferenceImages+1; i++ {
			req.References = append(req.References, relaycommon.TaskReference{
				Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "gs://b/a.png",
			})
		}
		_, err := BuildVeoReferenceImages(c, &req, true)
		require.Error(t, err)
	})

	t.Run("gcs passthrough on vertex", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model: "veo-3.1-generate-preview",
			References: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "gs://bucket/a.png"},
			},
		}
		got, err := BuildVeoReferenceImages(c, &req, true)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].Image)
		assert.Equal(t, "gs://bucket/a.png", got[0].Image.GcsUri)
		assert.Equal(t, "asset", got[0].ReferenceType)
	})

	t.Run("gcs rejected on gemini api", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model: "veo-3.1-generate-preview",
			References: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "gs://bucket/a.png"},
			},
		}
		_, err := BuildVeoReferenceImages(c, &req, false)
		require.Error(t, err)
	})

	// 官方约束：参考图与首帧/尾帧互斥。
	t.Run("frame combined with refs rejected", func(t *testing.T) {
		req := relaycommon.TaskSubmitReq{
			Model: "veo-3.1-generate-preview",
			References: []relaycommon.TaskReference{
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleReferenceImage, URL: "gs://b/a.png"},
				{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "gs://b/f.png"},
			},
		}
		_, err := BuildVeoReferenceImages(c, &req, true)
		require.Error(t, err)
	})
}

func TestBuildVeoImageObjectGCSPassthroughAndEmpty(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)

	got, err := BuildVeoImageObject(c, "gs://bucket/a.png")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "gs://bucket/a.png", got.GcsUri)
	assert.Empty(t, got.BytesBase64Encoded)

	got, err = BuildVeoImageObject(c, "   ")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// lastFrame 必须与 image 搭配，单独出现要报错。
func TestApplyVeoInstanceReferencesRejectsLoneLastFrame(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	req := relaycommon.TaskSubmitReq{
		Model: "veo-3.1-generate-preview",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "gs://b/l.png"},
		},
	}
	instance := VeoInstance{Prompt: "x"}
	params := VeoParameters{}
	err := ApplyVeoInstanceReferences(c, &req, &instance, &params, true)
	require.Error(t, err)
}

func TestApplyVeoInstanceReferencesFirstAndLastFrame(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	req := relaycommon.TaskSubmitReq{
		Model: "veo-3.1-generate-preview",
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "gs://b/f.png"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "gs://b/l.png"},
		},
	}
	instance := VeoInstance{Prompt: "x"}
	params := VeoParameters{}

	require.NoError(t, ApplyVeoInstanceReferences(c, &req, &instance, &params, true))
	require.NotNil(t, instance.Image)
	require.NotNil(t, instance.LastFrame)
	assert.Equal(t, "gs://b/f.png", instance.Image.GcsUri)
	assert.Equal(t, "gs://b/l.png", instance.LastFrame.GcsUri)
	// 没有参考图时不应触发 8s / allow_adult 硬约束
	assert.Zero(t, params.DurationSeconds)
	assert.Empty(t, params.PersonGeneration)
}
