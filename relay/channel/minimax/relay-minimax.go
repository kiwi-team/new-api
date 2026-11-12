package minimax

import (
	"fmt"
	"strings"

	channelconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = channelconstant.GetChannelBaseURL(channelconstant.ChannelTypeMiniMax)
	}
	baseUrl = strings.TrimRight(baseUrl, "/")
	model := info.GetUpstreamModelName()
	if model == "" {
		model = info.OriginModelName
	}
	if isH3ImageModel(model) {
		if info.ChannelBaseUrl == "" {
			baseUrl = "https://api.minimax.cn"
		}
		switch info.RelayMode {
		case constant.RelayModeImagesGenerations:
			return baseUrl + "/v1/images/generations", nil
		case constant.RelayModeImagesEdits:
			return baseUrl + "/v1/images/edits", nil
		}
	}
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		switch info.RelayMode {
		case constant.RelayModeChatCompletions:
			return fmt.Sprintf("%s/v1/text/chatcompletion_v2", baseUrl), nil
		case constant.RelayModeImagesGenerations:
			return fmt.Sprintf("%s/v1/image_generation", baseUrl), nil
		case constant.RelayModeAudioSpeech:
			return fmt.Sprintf("%s/v1/t2a_v2", baseUrl), nil
		default:
			return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
		}
	}
}
