package gemini_realtime

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseUrl := strings.TrimRight(info.ChannelBaseUrl, "/")
	// Default to Google AI endpoint for Gemini Live API
	// wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=API_KEY
	if strings.Contains(baseUrl, "generativelanguage.googleapis.com") {
		// Convert https:// to wss://
		baseUrl = strings.Replace(baseUrl, "https://", "wss://", 1)
		baseUrl = strings.Replace(baseUrl, "http://", "ws://", 1)
		return fmt.Sprintf("%s/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
			baseUrl, info.ApiKey), nil
	}
	// For third-party proxies, try to convert the URL similarly
	if strings.HasPrefix(baseUrl, "https://") {
		baseUrl = strings.Replace(baseUrl, "https://", "wss://", 1)
	} else if strings.HasPrefix(baseUrl, "http://") {
		baseUrl = strings.Replace(baseUrl, "http://", "ws://", 1)
	}
	return fmt.Sprintf("%s/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
		baseUrl, info.ApiKey), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// Gemini Live API authenticates via the key query parameter, not headers.
	// No special headers needed for the WebSocket handshake.
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support OpenAI request conversion")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support rerank request conversion")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support embedding request conversion")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support audio request conversion")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support image request conversion")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support OpenAI responses request conversion")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support Claude request conversion")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, fmt.Errorf("not implemented: gemini_realtime does not support Gemini request conversion")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoWssRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	err, usage = GeminiRealtimeHandler(c, info)
	return usage, err
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "gemini_realtime"
}

var ModelList = []string{
	"gemini-2.0-flash-live-001",
	"gemini-3.1-flash-live-preview",
}
