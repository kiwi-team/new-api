package moonshot

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestKimiK26UsesOnlyAllowedTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

func TestConvertOpenAIRequestKimiK26KeepsOmittedTemperatureOmitted(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "kimi-k2.6",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Nil(t, convertedRequest.Temperature)
}

func TestConvertOpenAIRequestOtherMoonshotModelKeepsTemperature(t *testing.T) {
	// 用没有专属参数改写规则的模型，避免与 kimi-k2.5 的参数适配互相干扰。
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2-0905",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2-0905",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 0.7, *convertedRequest.Temperature)
}

// kimi-k2.5 的采样参数由上游文档规定：top_p 固定 0.95，temperature 随 thinking 开关
// 取 1.0 / 0.6，客户端自定义值会被覆盖。
// https://platform.moonshot.cn/docs/guide/kimi-k2-5-quickstart
func TestConvertOpenAIRequestKimiK25AppliesDocumentedSampling(t *testing.T) {
	cases := []struct {
		name                string
		thinking            json.RawMessage
		expectedTemperature float64
		expectedThinking    string
	}{
		{name: "thinking omitted", thinking: nil, expectedTemperature: 1.0},
		{name: "thinking enabled", thinking: json.RawMessage(`{"type":"enabled","budget_tokens":1024}`), expectedTemperature: 1.0, expectedThinking: `{"type":"enabled"}`},
		{name: "thinking disabled", thinking: json.RawMessage(`{"type":"disabled"}`), expectedTemperature: 0.6, expectedThinking: `{"type":"disabled"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{
				Model:       "kimi-k2.5",
				Temperature: common.GetPointer[float64](0.7),
				TopP:        common.GetPointer[float64](0.1),
				THINKING:    tc.thinking,
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "kimi-k2.5",
				},
			}

			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

			require.NoError(t, err)
			convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			require.NotNil(t, convertedRequest.Temperature)
			assert.Equal(t, tc.expectedTemperature, *convertedRequest.Temperature)
			require.NotNil(t, convertedRequest.TopP)
			assert.Equal(t, 0.95, *convertedRequest.TopP)
			if tc.expectedThinking != "" {
				assert.JSONEq(t, tc.expectedThinking, string(convertedRequest.THINKING))
			}
		})
	}
}
