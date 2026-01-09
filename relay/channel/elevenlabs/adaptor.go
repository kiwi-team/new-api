package elevenlabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return nil, errors.New("unsupported audio relay mode")
	}

	voiceID, ok := VoideNameIDMap[request.Voice]
	if !ok {
		return nil, fmt.Errorf("invalid voice name: %s", request.Voice)
	}
	info.VoideId = voiceID
	//speed := request.Speed

	ttsRequest := TTSRequest{
		ModelID: info.OriginModelName,
		Text:    request.Input,
	}

	// 同步扩展字段的厂商自定义metadata
	if len(request.Metadata) > 0 {
		if err := json.Unmarshal(request.Metadata, &ttsRequest); err != nil {
			return nil, fmt.Errorf("error unmarshalling metadata to minimax request: %w", err)
		}
	}

	jsonData, err := json.Marshal(ttsRequest)
	if err != nil {
		return nil, fmt.Errorf("error marshalling minimax request: %w", err)
	}
	// Debug: log the request structure
	// fmt.Printf("MiniMax TTS Request: %s\n", string(jsonData))

	return bytes.NewReader(jsonData), nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		return fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", info.VoideId), nil
	}
	return "", errors.New("unsupported audio relay mode")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("xi-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		return handleTTSResponse(c, resp, info)
	}

	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "elevenlabs"
}

func handleTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	defer resp.Body.Close()

	// Check base_resp status code
	if resp.StatusCode != 200 {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("elevenlabs TTS error: %d - %s", resp.StatusCode, resp.Status),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	audioUrl, err1 := service.UploadIOReaderToS3(c, resp)
	if err1 != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to upload audio to S3: %w", err1),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	common.SetContextKey(c, constant.ContextKeyAudioUrl, audioUrl)
	common.ApiSuccess(c, gin.H{"audio_url": audioUrl})
	// Determine content type - default to mp3
	//contentType := "audio/mpeg"
	//c.Data(http.StatusOK, contentType, audioData)

	usage = &dto.Usage{
		PromptTokens:     info.Usage.PromptTokens, //info.PromptTokens,
		CompletionTokens: 0,
		TotalTokens:      info.Usage.PromptTokens, //info.PromptTokens,
	}

	return usage, nil
}
