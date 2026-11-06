package qwen_realtime

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	info *relaycommon.RelayInfo
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.info = info
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL, err := url.Parse(strings.TrimRight(info.ChannelBaseUrl, "/"))
	if err != nil {
		return "", fmt.Errorf("parse Qwen realtime base URL: %w", err)
	}
	if baseURL.Hostname() == "" || baseURL.User != nil || baseURL.Fragment != "" {
		return "", fmt.Errorf("Qwen realtime base URL must contain a host without credentials or a fragment")
	}
	if isBeijingQwenRealtimeModel(info.UpstreamModelName) {
		if baseURL.Path != "" && baseURL.Path != "/" {
			return "", fmt.Errorf("Qwen realtime base URL must be a domain root without an API path")
		}
		host := strings.ToLower(baseURL.Hostname())
		if strings.HasSuffix(host, ".aliyuncs.com") && baseURL.Scheme != "https" && baseURL.Scheme != "wss" {
			return "", fmt.Errorf("Qwen realtime workspace endpoints require HTTPS or WSS")
		}
		if strings.HasSuffix(host, ".aliyuncs.com") && !strings.HasSuffix(host, ".cn-beijing.maas.aliyuncs.com") {
			return "", fmt.Errorf("this Qwen realtime model requires a Beijing workspace endpoint")
		}
		if strings.HasSuffix(host, ".cn-beijing.maas.aliyuncs.com") && info.Organization != "" && strings.TrimSuffix(host, ".cn-beijing.maas.aliyuncs.com") != info.Organization {
			return "", fmt.Errorf("Qwen realtime workspace header must match the endpoint workspace")
		}
	}
	switch baseURL.Scheme {
	case "http":
		baseURL.Scheme = "ws"
	case "https":
		baseURL.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported Qwen realtime URL scheme %q", baseURL.Scheme)
	}
	if isFunASRRealtimeModel(info.UpstreamModelName) {
		baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/api-ws/v1/inference"
		return baseURL.String(), nil
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/api-ws/v1/realtime"
	query := baseURL.Query()
	query.Set("model", info.UpstreamModelName)
	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	req.Set("Authorization", "Bearer "+info.ApiKey)
	if info.Organization != "" {
		req.Set("X-DashScope-WorkSpace", info.Organization)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support OpenAI request conversion")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support rerank request conversion")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support embedding request conversion")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support audio request conversion")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support image request conversion")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support OpenAI responses request conversion")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support Claude request conversion")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: qwen_realtime does not support Gemini request conversion")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoWssRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	err, usage = QwenRealtimeHandler(c, info)
	return usage, err
}

func (a *Adaptor) GetModelList() []string {
	return []string{
		"qwen3.8-omni-flash-realtime", "qwen-audio-3.1-realtime-plus", "qwen-audio-3.1-realtime-flash",
		"qwen3-omni-flash-realtime",
		"fun-asr-realtime", "fun-asr-realtime-2026-02-28", "fun-asr-realtime-2025-11-07", "fun-asr-realtime-2025-09-15",
		"fun-asr-flash-8k-realtime", "fun-asr-flash-8k-realtime-2026-01-28",
		"qwen3-asr-flash-realtime", "qwen3-asr-flash-realtime-2026-02-10", "qwen3-asr-flash-realtime-2025-10-27",
		"qwen-audio-3.0-asr-flash-streaming",
	}
}

func (a *Adaptor) GetChannelName() string {
	return "qwen_realtime"
}
