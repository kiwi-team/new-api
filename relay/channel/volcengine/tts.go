package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	//relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// https://www.volcengine.com/docs/6561/1329505
// 双向流式websocket-V3
type VolcengineBidirectionalTTSRequest struct {
	User      VolcengineTTSUser                   `json:"user"`
	Namespace string                              `json:"namespace"`
	ReqParams VolcengineBidirectionalTTSReqParams `json:"req_params"`
}
type VolcengineBidirectionalTTSReqParams struct {
	Text        string      `json:"text"`
	Speaker     string      `json:"speaker"`
	AudioParams AudioParams `json:"audio_params"`
	Addtions    string      `json:"addtions,omitempty"`
}
type AudioParams struct {
	Format          string `json:"format"`
	SampleRate      int    `json:"sample_rate"`
	EnableTimestamp bool   `json:"enable_timestamp,omitempty"`
}

type VolcengineTTSRequest struct {
	App     VolcengineTTSApp     `json:"app"`
	User    VolcengineTTSUser    `json:"user"`
	Audio   VolcengineTTSAudio   `json:"audio"`
	Request VolcengineTTSReqInfo `json:"request"`
}

type VolcengineTTSApp struct {
	AppID   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

type VolcengineTTSUser struct {
	UID string `json:"uid"`
}

type VolcengineTTSAudio struct {
	VoiceType        string  `json:"voice_type"`
	Encoding         string  `json:"encoding"`
	SpeedRatio       float64 `json:"speed_ratio"`
	Rate             int     `json:"rate"`
	Bitrate          int     `json:"bitrate,omitempty"`
	LoudnessRatio    float64 `json:"loudness_ratio,omitempty"`
	EnableEmotion    bool    `json:"enable_emotion,omitempty"`
	Emotion          string  `json:"emotion,omitempty"`
	EmotionScale     float64 `json:"emotion_scale,omitempty"`
	ExplicitLanguage string  `json:"explicit_language,omitempty"`
	ContextLanguage  string  `json:"context_language,omitempty"`
}

type VolcengineTTSReqInfo struct {
	ReqID           string                   `json:"reqid"`
	Text            string                   `json:"text"`
	Operation       string                   `json:"operation"`
	Model           string                   `json:"model,omitempty"`
	TextType        string                   `json:"text_type,omitempty"`
	SilenceDuration float64                  `json:"silence_duration,omitempty"`
	WithTimestamp   interface{}              `json:"with_timestamp,omitempty"`
	ExtraParam      *VolcengineTTSExtraParam `json:"extra_param,omitempty"`
}

type VolcengineTTSExtraParam struct {
	DisableMarkdownFilter      bool                      `json:"disable_markdown_filter,omitempty"`
	EnableLatexTn              bool                      `json:"enable_latex_tn,omitempty"`
	MuteCutThreshold           string                    `json:"mute_cut_threshold,omitempty"`
	MuteCutRemainMs            string                    `json:"mute_cut_remain_ms,omitempty"`
	DisableEmojiFilter         bool                      `json:"disable_emoji_filter,omitempty"`
	UnsupportedCharRatioThresh float64                   `json:"unsupported_char_ratio_thresh,omitempty"`
	AigcWatermark              bool                      `json:"aigc_watermark,omitempty"`
	CacheConfig                *VolcengineTTSCacheConfig `json:"cache_config,omitempty"`
}

type VolcengineTTSCacheConfig struct {
	TextType int  `json:"text_type,omitempty"`
	UseCache bool `json:"use_cache,omitempty"`
}

type VolcengineTTSResponse struct {
	ReqID    string                     `json:"reqid"`
	Code     int                        `json:"code"`
	Message  string                     `json:"message"`
	Sequence int                        `json:"sequence"`
	Data     string                     `json:"data"`
	Addition *VolcengineTTSAdditionInfo `json:"addition,omitempty"`
}

type VolcengineTTSAdditionInfo struct {
	Duration string `json:"duration"`
}

var openAIToVolcengineVoiceMap = map[string]string{
	"alloy":   "zh_male_M392_conversation_wvae_bigtts",
	"echo":    "zh_male_wenhao_mars_bigtts",
	"fable":   "zh_female_tianmei_mars_bigtts",
	"onyx":    "zh_male_zhibei_mars_bigtts",
	"nova":    "zh_female_shuangkuaisisi_mars_bigtts",
	"shimmer": "zh_female_cancan_mars_bigtts",
}

var responseFormatToEncodingMap = map[string]string{
	"mp3":  "mp3",
	"opus": "ogg_opus",
	"aac":  "mp3",
	"flac": "mp3",
	"wav":  "wav",
	"pcm":  "pcm",
}

func parseVolcengineAuth(apiKey string) (appID, token string, err error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return "", "", errors.New("invalid api key format, expected: appid|access_token")
	}
	return parts[0], parts[1], nil
}

func mapVoiceType(openAIVoice string) string {
	if voice, ok := openAIToVolcengineVoiceMap[openAIVoice]; ok {
		return voice
	}
	return openAIVoice
}

func mapEncoding(responseFormat string) string {
	if encoding, ok := responseFormatToEncodingMap[responseFormat]; ok {
		return encoding
	}
	return "mp3"
}

func getContentTypeByEncoding(encoding string) string {
	contentTypeMap := map[string]string{
		"mp3":      "audio/mpeg",
		"ogg_opus": "audio/ogg",
		"wav":      "audio/wav",
		"pcm":      "audio/pcm",
	}
	if ct, ok := contentTypeMap[encoding]; ok {
		return ct
	}
	return "application/octet-stream"
}

func handleTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, encoding string) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read volcengine response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	var volcResp VolcengineTTSResponse
	if unmarshalErr := json.Unmarshal(body, &volcResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to parse volcengine response"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	if volcResp.Code != 3000 {
		return nil, types.NewErrorWithStatusCode(
			errors.New(volcResp.Message),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	audioData, decodeErr := base64.StdEncoding.DecodeString(volcResp.Data)
	if decodeErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to decode audio data"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	audioStr := base64.StdEncoding.EncodeToString(audioData)
	audioUrl, err1 := service.SimpleUploadToS3(c, audioStr)
	if err1 != nil {
		audioUrl = audioStr
	}
	common.SetContextKey(c, constant.ContextKeyAudioUrl, audioUrl)

	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}

	return usage, nil
}

func generateRequestID() string {
	return uuid.New().String()
}

func handleTTSWebSocketResponse(c *gin.Context, requestURL string, volcRequest VolcengineTTSRequest, info *relaycommon.RelayInfo, encoding string) (usage any, err *types.NewAPIError) {
	_, token, parseErr := parseVolcengineAuth(info.ApiKey)
	if parseErr != nil {
		return nil, types.NewErrorWithStatusCode(
			parseErr,
			types.ErrorCodeChannelInvalidKey,
			http.StatusUnauthorized,
		)
	}

	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer;%s", token))

	conn, resp, dialErr := websocket.DefaultDialer.DialContext(context.Background(), requestURL, header)
	if dialErr != nil {
		if resp != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to connect to websocket: %w, status: %d", dialErr, resp.StatusCode),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadGateway,
			)
		}
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket: %w", dialErr),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	defer conn.Close()

	payload, marshalErr := json.Marshal(volcRequest)
	if marshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to marshal request: %w", marshalErr),
			types.ErrorCodeBadRequestBody,
			http.StatusInternalServerError,
		)
	}

	if sendErr := FullClientRequest(conn, payload); sendErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to send request: %w", sendErr),
			types.ErrorCodeBadRequestBody,
			http.StatusInternalServerError,
		)
	}

	contentType := getContentTypeByEncoding(encoding)
	c.Header("Content-Type", contentType)
	c.Header("Transfer-Encoding", "chunked")
	var audio []byte

	for {
		msg, recvErr := ReceiveMessage(conn)
		if recvErr != nil {
			if websocket.IsCloseError(recvErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to receive message: %w", recvErr),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			)
		}

		switch msg.MsgType {
		case MsgTypeError:
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("received error from server: code=%d, %s", msg.ErrorCode, string(msg.Payload)),
				types.ErrorCodeBadResponse,
				http.StatusBadRequest,
			)
		case MsgTypeFrontEndResultServer:
			continue
		case MsgTypeAudioOnlyServer:
			if len(msg.Payload) > 0 {
				audio = append(audio, msg.Payload...)
			}

			if msg.Sequence < 0 {
				if len(audio) == 0 {
					return nil, types.NewErrorWithStatusCode(
						errors.New("empty audio data"),
						types.ErrorCodeBadResponse,
						http.StatusInternalServerError,
					)
				}
				base64Audio := base64.StdEncoding.EncodeToString(audio)
				audioUrl, err1 := service.SimpleUploadToS3(c, base64Audio)
				if err1 != nil {
					audioUrl = base64Audio
				}
				common.SetContextKey(c, constant.ContextKeyAudioUrl, audioUrl)
				common.ApiSuccess(c, gin.H{"audio_url": audioUrl})
				usage = &dto.Usage{
					PromptTokens:     info.GetEstimatePromptTokens(),
					CompletionTokens: 0,
					TotalTokens:      info.GetEstimatePromptTokens(),
				}
				return usage, nil
			}
		default:
			continue
		}
	}
	if len(audio) == 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("empty audio data"),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		)
	}
	base64Audio := base64.StdEncoding.EncodeToString(audio)
	audioUrl, err1 := service.SimpleUploadToS3(c, base64Audio)
	if err1 != nil {
		audioUrl = base64Audio
	}
	common.SetContextKey(c, constant.ContextKeyAudioUrl, audioUrl)
	common.ApiSuccess(c, gin.H{"audio_url": audioUrl})
	//c.Status(http.StatusOK)
	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(), //info.PromptTokens,
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(), //info.PromptTokens,
	}
	return usage, nil
}

func VoiceToResourceId(voice string) string {
	if strings.HasPrefix(voice, "S_") {
		return "volc.megatts.default"
	}
	return "volc.service_type.10029"
}

// seed-tts-2.0
func handleTTSWebSocketResponse2(c *gin.Context, requestURL string, volcRequest *VolcengineBidirectionalTTSRequest, info *relaycommon.RelayInfo, encoding string) (usage any, err *types.NewAPIError) {
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	appId, token, parseErr := parseVolcengineAuth(info.ApiKey)
	if parseErr != nil {
		return nil, types.NewErrorWithStatusCode(
			parseErr,
			types.ErrorCodeChannelInvalidKey,
			http.StatusUnauthorized,
		)
	}

	header := http.Header{}
	header.Set("X-Api-App-Key", appId)
	header.Set("X-Api-Access-Key", token)
	header.Set("X-Api-Resource-Id", modelName)
	requestURL = "wss://openspeech.bytedance.com/api/v3/tts/bidirection"

	conn, resp, dialErr := websocket.DefaultDialer.DialContext(context.Background(), requestURL, header)
	if dialErr != nil {
		if resp != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to connect to websocket: %w, status: %d", dialErr, resp.StatusCode),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadGateway,
			)
		}
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket: %w", dialErr),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	defer conn.Close()

	// ----------------start connection----------------
	if err := StartConnection(conn); err != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket: %w", dialErr),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	// ----------------wait connection----------------
	msg, err1 := WaitForEvent(conn, MsgTypeFullServerResponse, EventType_ConnectionStarted)
	if err1 != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket: %w, msg: %s", err1, msg),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	defer func() {
		// ----------------finish connection----------------
		if err := FinishConnection(conn); err != nil {
			fmt.Printf("failed to finish connection: %v", err)
		}
		// ----------------wait connection finished----------------
		msg, err := WaitForEvent(conn, MsgTypeFullServerResponse, EventType_ConnectionFinished)
		if err != nil {
			fmt.Printf(" wait connection finished failed: %v, msg: %s", err, msg)
		}
	}()

	// payload, marshalErr := json.Marshal(volcRequest)
	// if marshalErr != nil {
	// 	return nil, types.NewErrorWithStatusCode(
	// 		fmt.Errorf("failed to marshal request: %w", marshalErr),
	// 		types.ErrorCodeBadRequestBody,
	// 		http.StatusInternalServerError,
	// 	)
	// }

	sessionID := uuid.New().String()
	startReq := map[string]any{
		"user":       volcRequest.User,
		"event":      int(EventType_StartSession),
		"namespace":  volcRequest.Namespace,
		"req_params": volcRequest.ReqParams,
	}
	startPayload, err1 := json.Marshal(startReq)
	if err1 != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to marshal start request: %w", err1),
			types.ErrorCodeBadRequestBody,
			http.StatusInternalServerError,
		)
	}
	// ----------------start session----------------
	if err := StartSession(conn, startPayload, sessionID); err != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket sart: %w", err),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	// ----------------wait session started----------------
	msg, err1 = WaitForEvent(conn, MsgTypeFullServerResponse, EventType_SessionStarted)
	if err1 != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket wait session started: %w msg: %s", err1, msg),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}

	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()

		ttsReq := map[string]any{
			"user":       volcRequest.User,
			"event":      int(EventType_TaskRequest),
			"namespace":  volcRequest.Namespace,
			"req_params": volcRequest.ReqParams,
		}
		payload, _ := json.Marshal(&ttsReq)
		// ----------------send task request----------------
		if err := TaskRequest(conn, payload, sessionID); err != nil {
			fmt.Printf("failed to send task request: %v", err)
		}
		<-t.C

		// ----------------finish session----------------
		if err := FinishSession(conn, sessionID); err != nil {
		}
	}()

	var audio []byte
	for {
		msg, err := ReceiveMessage(conn)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to get message from ws : %w msg: %s", err, msg),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadGateway,
			)
		}
		switch msg.MsgType {
		case MsgTypeFullServerResponse:
		case MsgTypeAudioOnlyServer:
			if msg.Payload != nil && len(msg.Payload) > 0 {
				audio = append(audio, msg.Payload...)
			}
		default:
			fmt.Printf("unknown msg type: %d", msg.MsgType)
			continue
		}
		if msg.EventType == EventType_SessionFinished {
			break
		}
	}
	if len(audio) == 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("empty audio data"),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		)
	}
	base64Audio := base64.StdEncoding.EncodeToString(audio)
	audioUrl, err1 := service.SimpleUploadToS3(c, base64Audio)
	if err1 != nil {
		audioUrl = base64Audio
	}
	common.SetContextKey(c, constant.ContextKeyAudioUrl, audioUrl)
	common.ApiSuccess(c, gin.H{"audio_url": audioUrl})
	//c.Status(http.StatusOK)
	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}
	return usage, nil
}
