package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmniBillingEstimateUsesRequestedDurationResolutionAndImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:      "gemini-omni-1.1-flash-preview",
		Duration:   5,
		Resolution: "1080P",
		Images:     []string{"data:image/png;base64,one", "data:image/png;base64,two"},
	}

	ratios := OmniBillingEstimate(req, omni720OutputUSDPerSecond)
	require.Contains(t, ratios, "omni_usage_estimate")

	wantCost := float64(2*1120)*omniInputUSDPerMillionTokens/1_000_000 +
		float64(5*8688)*omniVideoOutputUSDPerMillionTokens/1_000_000
	assert.InDelta(t, wantCost/omni720OutputUSDPerSecond, ratios["omni_usage_estimate"], 1e-12)
}

func TestParseOmniTaskResultCapturesUsageByModality(t *testing.T) {
	body := []byte(`{
		"id":"interaction-1",
		"status":"completed",
		"steps":[{"type":"model_output","content":[{"type":"video","uri":"https://example.com/video.mp4"}]}],
		"usage":{
			"total_input_tokens":1220,
			"total_output_tokens":5800,
			"total_thought_tokens":20,
			"input_tokens_by_modality":[{"modality":"text","tokens":100},{"modality":"image","tokens":1120}],
			"output_tokens_by_modality":[{"modality":"text","tokens":8},{"modality":"video","tokens":5792}]
		}
	}`)

	result, err := ParseOmniTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, 1220, result.InputTokens)
	assert.Equal(t, 5800, result.OutputTokens)
	assert.Equal(t, 20, result.ThoughtTokens)
	assert.Equal(t, map[string]int{"text": 100, "image": 1120}, result.InputTokensByModality)
	assert.Equal(t, map[string]int{"text": 8, "video": 5792}, result.OutputTokensByModality)
}

func TestOmniUsageCostUsesPublishedMixedTokenPrices(t *testing.T) {
	usage := &relaycommon.TaskInfo{
		InputTokens:   1220,
		OutputTokens:  5800,
		ThoughtTokens: 20,
		OutputTokensByModality: map[string]int{
			"text":  8,
			"video": 5792,
		},
	}

	want := float64(1220)*1.50/1_000_000 +
		float64(8+20)*9.00/1_000_000 +
		float64(5792)*17.50/1_000_000
	assert.InDelta(t, want, omniUsageCostUSD(omni720OutputUSDPerSecond, usage), 1e-12)
}

func TestOmniAdjustBillingOnCompleteAppliesGroupRatio(t *testing.T) {
	task := &model.Task{Properties: model.Properties{OriginModelName: "gemini-omni-1.1-flash-preview"}}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice: omni720OutputUSDPerSecond,
		GroupRatio: 1.25,
	}
	usage := &relaycommon.TaskInfo{
		InputTokens:  100,
		OutputTokens: 5792,
		OutputTokensByModality: map[string]int{
			"video": 5792,
		},
	}
	wantCost := float64(100)*1.50/1_000_000 + float64(5792)*17.50/1_000_000
	wantQuota, err := common.QuotaFromFloatStrict(wantCost * common.QuotaPerUnit * 1.25)
	require.NoError(t, err)

	assert.Equal(t, wantQuota, (&TaskAdaptor{}).AdjustBillingOnComplete(task, usage))
}

func TestBuildOmniRequestBodyNormalizesBillableOutputSettings(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:      "a cat",
		Seconds:     "6",
		Resolution:  "4K",
		AspectRatio: "9:16",
		Metadata: map[string]any{
			"response_format": map[string]any{"delivery": "uri", "resolution": "360p"},
		},
	}

	body, err := BuildOmniRequestBody(nil, req, "gemini-omni-1.1-flash-preview")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"gemini-omni-1.1-flash-preview",
		"background":true,
		"store":true,
		"input":"a cat",
		"generation_config":{"video_config":{"task":"text_to_video"}},
		"response_format":{"type":"video","duration":"6s","resolution":"4k","aspect_ratio":"9:16","delivery":"uri"}
	}`, string(body))
}
