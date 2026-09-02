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
}
