package ltx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLtx25ResolutionRatios(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		want       float64
	}{
		{name: "fast landscape 720p", model: "ltx-2-5-fast", resolution: "1280x720", want: 1},
		{name: "fast portrait 1080p", model: "ltx-2-5-fast", resolution: "1080x1920", want: 0.13 / 0.09},
		{name: "fast landscape 1440p", model: "ltx-2-5-fast", resolution: "2560x1440", want: 0.19 / 0.09},
		{name: "fast portrait 4k", model: "ltx-2-5-fast", resolution: "2160x3840", want: 0.30 / 0.09},
		{name: "pro portrait 720p", model: "ltx-2-5-pro", resolution: "720x1280", want: 1},
		{name: "pro landscape 1080p", model: "ltx-2-5-pro", resolution: "1920x1080", want: 0.17 / 0.12},
		{name: "pro does not support 4k", model: "ltx-2-5-pro", resolution: "3840x2160", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, getLtx25ResolutionRatio(tt.model, tt.resolution), 1e-12)
		})
	}
}

func TestLegacyLtxModelPriceIsUnchanged(t *testing.T) {
	assert.InDelta(t, 0.04*1.5, getLegacyModelPrice("t2v", "ltx-2-3-fast", "1920x1080"), 1e-12)
}

func TestLtx25ConfiguredPriceBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalModelPrices := ratio_setting.ModelPrice2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"ltx-2-5-fast":0.09,"ltx-2-5-pro":0.12}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	tests := []struct {
		name       string
		model      string
		resolution string
		duration   int
		wantCost   float64
		wantQuota  int
	}{
		{
			name:       "pro 1080p for 8 seconds",
			model:      "ltx-2-5-pro",
			resolution: "1920x1080",
			duration:   8,
			wantCost:   1.36,
			wantQuota:  680000,
		},
		{
			name:       "fast 4k for 6 seconds",
			model:      "ltx-2-5-fast",
			resolution: "3840x2160",
			duration:   6,
			wantCost:   1.80,
			wantQuota:  900000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			context.Set("task_request", relaycommon.TaskSubmitReq{
				Model: tt.model, Resolution: tt.resolution, Duration: tt.duration,
			})
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				UsingGroup:      "default",
				UserGroup:       "default",
			}

			priceData, err := relayhelper.ModelPriceHelperPerCall(context, info)
			require.NoError(t, err)
			for name, ratio := range (&TaskAdaptor{}).EstimateBilling(context, info) {
				priceData.AddOtherRatio(name, ratio)
			}
			cost := priceData.ModelPrice * priceData.OtherRatioMultiplier()
			quota, clamp := common.QuotaFromFloatChecked(priceData.ApplyOtherRatiosToFloat(float64(priceData.Quota)))

			assert.InDelta(t, tt.wantCost, cost, 1e-12)
			assert.Equal(t, tt.wantQuota, quota)
			assert.Nil(t, clamp)
		})
	}
}

func TestValidateLtx25RequestSupportMatrix(t *testing.T) {
	fps24 := 24
	fps48 := 48
	fps50 := 50
	tests := []struct {
		name    string
		req     LtxTaskRequest
		wantErr bool
	}{
		{
			name: "fast 1080p default fps long duration",
			req: LtxTaskRequest{
				Model: "ltx-2-5-fast", Resolution: "1920x1080", Duration: 20,
			},
		},
		{
			name: "fast 4k high fps short duration",
			req: LtxTaskRequest{
				Model: "ltx-2-5-fast", Resolution: "3840x2160", Duration: 10, FPS: &fps48,
			},
		},
		{
			name: "fast high fps rejects long duration",
			req: LtxTaskRequest{
				Model: "ltx-2-5-fast", Resolution: "1920x1080", Duration: 12, FPS: &fps50,
			},
			wantErr: true,
		},
		{
			name: "fast 4k rejects long duration at 24fps",
			req: LtxTaskRequest{
				Model: "ltx-2-5-fast", Resolution: "3840x2160", Duration: 12, FPS: &fps24,
			},
			wantErr: true,
		},
		{
			name: "pro portrait 720p",
			req: LtxTaskRequest{
				Model: "ltx-2-5-pro", Resolution: "720x1280", Duration: 8, FPS: &fps50,
			},
		},
		{
			name: "pro rejects 1440p",
			req: LtxTaskRequest{
				Model: "ltx-2-5-pro", Resolution: "2560x1440", Duration: 8, FPS: &fps24,
			},
			wantErr: true,
		},
		{
			name: "last frame requires first frame",
			req: LtxTaskRequest{
				Model: "ltx-2-5-fast", Resolution: "1920x1080", Duration: 6, LastFrameURI: "https://example.com/last.png",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLtxRequest(&tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConvertToLtx25RequestPayload(t *testing.T) {
	generateAudio := false
	fps := 25
	req := relaycommon.TaskSubmitReq{
		Model:         "ltx-2-5-fast",
		Prompt:        "A connected sequence",
		Resolution:    "1440x2560",
		FPS:           &fps,
		GenerateAudio: &generateAudio,
		References: []relaycommon.TaskReference{
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleFirstFrame, URL: "https://example.com/first.png"},
			{Type: relaycommon.RefTypeImage, Role: relaycommon.RefRoleLastFrame, URL: "https://example.com/last.png"},
		},
		Metadata: map[string]any{
			"model":      "billing-bypass",
			"duration":   20,
			"resolution": "3840x2160",
		},
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "ltx-2-5-fast", payload.Model)
	assert.Equal(t, 6, payload.Duration)
	assert.Equal(t, "1440x2560", payload.Resolution)
	assert.Equal(t, "https://example.com/first.png", payload.ImageURI)
	assert.Equal(t, "https://example.com/last.png", payload.LastFrameURI)
	require.NotNil(t, payload.FPS)
	assert.Equal(t, 25, *payload.FPS)
	require.NotNil(t, payload.GenerateAudio)
	assert.False(t, *payload.GenerateAudio)

	encoded, err := common.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"generate_audio":false`)
}

func TestValidateLtx25RequestUsesConfiguredPriceAndRequestRatios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"ltx-2-5-pro",
		"prompt":"Move from the first frame to the last",
		"duration":8,
		"resolution":"1920x1080",
		"generate_audio":false,
		"references":[
			{"type":"image","role":"first_frame","url":"https://example.com/first.png"},
			{"type":"image","role":"last_frame","url":"https://example.com/last.png"}
		]
	}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeLtx},
	}
	adaptor := &TaskAdaptor{}

	taskErr := adaptor.ValidateRequestAndSetAction(context, info)
	require.Nil(t, taskErr)
	assert.Zero(t, info.DynamicModelPrice)
	ratios := adaptor.EstimateBilling(context, info)
	assert.Equal(t, 8.0, ratios["seconds"])
	assert.InDelta(t, 0.17/0.12, ratios["resolution"], 1e-12)
	assert.InDelta(t, 1.36, 0.12*ratios["resolution"]*ratios["seconds"], 1e-12)

	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	assert.Equal(t, constant.TaskActionFirstTailGenerate, info.Action)
	assert.Equal(t, constant.TaskActionImageGenerate, context.GetString("action"))
	assert.Contains(t, string(encoded), `"image_uri":"https://example.com/first.png"`)
	assert.Contains(t, string(encoded), `"last_frame_uri":"https://example.com/last.png"`)
	assert.Contains(t, string(encoded), `"generate_audio":false`)
}

func TestValidateLtx25RequestRejectsUnsupportedProResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"ltx-2-5-pro","prompt":"A scene","duration":8,"size":"3840x2160"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeLtx},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_ltx_parameters", taskErr.Code)
}

func TestLtxModelListIncludes25(t *testing.T) {
	models := (&TaskAdaptor{}).GetModelList()
	assert.Contains(t, models, "ltx-2-5-fast")
	assert.Contains(t, models, "ltx-2-5-pro")
}
