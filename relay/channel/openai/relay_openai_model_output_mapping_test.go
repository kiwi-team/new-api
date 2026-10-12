package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestRemapResponseModelNameMatchesLowercaseRule(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ModelOutputMapping: `{
					"kimi/kimi-k3": "kimi-k3",
					"zhipu/glm-5.3-flash": "glm-5.3-flash",
					"zhipu/glm-5.3": "glm-5.3"
				}`,
			},
		},
	}

	tests := []struct {
		name          string
		upstreamModel string
		want          string
	}{
		{name: "already lowercase", upstreamModel: "kimi/kimi-k3", want: "kimi-k3"},
		{name: "uppercase provider and model", upstreamModel: "ZHIPU/GLM-5.3-Flash", want: "glm-5.3-flash"},
		{name: "mixed case provider and model", upstreamModel: "Zhipu/GLM-5.3", want: "glm-5.3"},
		{name: "unmatched model", upstreamModel: "Other/Model", want: "Other/Model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, remapResponseModelName(info, tt.upstreamModel))
		})
	}

	t.Run("requested model takes priority for mapped upstream model", func(t *testing.T) {
		info.OriginModelName = "deepseek-flash"
		info.UpstreamModelName = "deepseek-v4-1-flash-260910"
		info.ChannelSetting.ResponseModelUseRequestModel = true
		info.ChannelSetting.ModelOutputMapping = `{"deepseek-v4-1-flash-260910":"fixed-alias"}`

		assert.Equal(t, "deepseek-flash", remapResponseModelName(info, "DeepSeek-V4-1-Flash-260910"))
	})

	t.Run("requested model placeholder resolves without global switch", func(t *testing.T) {
		info.OriginModelName = "deepseek-flash"
		info.UpstreamModelName = "deepseek-v4-1-flash-260910"
		info.ChannelSetting.ResponseModelUseRequestModel = false
		info.ChannelSetting.ModelOutputMapping = `{"deepseek-v4-1-flash-260910":"$requested_model"}`

		assert.Equal(t, "deepseek-flash", remapResponseModelName(info, "DeepSeek-V4-1-Flash-260910"))
	})
}
