package volcengine

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// 豆包端到端实时语音大模型 Realtime Dialogue
// 参考文档: wss://openspeech.bytedance.com/api/v3/realtime/dialogue

const (
	doubaoRealtimeDialogueURL = "wss://openspeech.bytedance.com/api/v3/realtime/dialogue"
	doubaoRealtimeAppKey      = "PlgvMymc7f3tQnJ6"
	doubaoRealtimeResourceID  = "volc.speech.dialog"
)

// ============================================================
// Adaptor — 接入 relay pipeline
// ============================================================

type RealtimeAdaptor struct{}

func (a *RealtimeAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *RealtimeAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return doubaoRealtimeDialogueURL, nil
}

func (a *RealtimeAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	appID, accessToken, err := parseVolcengineAuth(info.ApiKey)
	if err != nil {
		return fmt.Errorf("invalid volcengine key: %w", err)
	}
	req.Set("X-Api-App-ID", appID)
	req.Set("X-Api-Access-Key", accessToken)
	req.Set("X-Api-Resource-Id", doubaoRealtimeResourceID)
	req.Set("X-Api-App-Key", doubaoRealtimeAppKey)
	req.Set("X-Api-Connect-Id", uuid.NewString())
	return nil
}

func (a *RealtimeAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoWssRequest(a, c, info, requestBody)
}

func (a *RealtimeAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	err, usage = DoubaoRealtimeHandler(c, info)
	return usage, err
}

func (a *RealtimeAdaptor) GetModelList() []string {
	return []string{"doubao-realtime"}
}

func (a *RealtimeAdaptor) GetChannelName() string {
	return "volcengine_realtime"
}

// 以下方法 realtime 场景不使用
func (a *RealtimeAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}
func (a *RealtimeAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented: volcengine_realtime")
}

// ============================================================
// Handler — 双向透传 + 解析 UsageResponse 计费
// ============================================================

// collectWsMessage 记录 ws 消息摘要用于日志审计
func collectRealtimeWsMessage(c *gin.Context, info *relaycommon.RelayInfo, mu *sync.Mutex, direction string, data []byte) {
	// 二进制帧：只记录方向和大小，避免日志过大
	// 但如果是文本类事件（FullServer），尝试提取 event 和 payload 摘要
	summary := fmt.Sprintf("[%s] %d bytes", direction, len(data))

	if len(data) >= 4 {
		msgTypeNibble := (data[1] >> 4) & 0x0F
		// FullServer(0x09) 或 FullClient(0x01) 带 JSON payload，记录更多信息
		if msgTypeNibble == 0x09 || msgTypeNibble == 0x01 {
			msg, err := parseRealtimeFrame(data)
			if err == nil {
				evtName := eventNameForLog(msg.event)
				if len(msg.payload) > 0 && len(msg.payload) <= 500 {
					summary = fmt.Sprintf("[%s] %s: %s", direction, evtName, string(msg.payload))
				} else if len(msg.payload) > 500 {
					// Truncate at a valid UTF-8 boundary to avoid broken sequences in DB
					truncated := msg.payload[:500]
					for len(truncated) > 0 && !utf8.Valid(truncated) {
						truncated = truncated[:len(truncated)-1]
					}
					summary = fmt.Sprintf("[%s] %s: %s...(truncated, total %d)", direction, evtName, string(truncated), len(msg.payload))
				} else {
					summary = fmt.Sprintf("[%s] %s", direction, evtName)
				}
			}
		} else if msgTypeNibble == 0x0B { // AudioOnlyServer
			summary = fmt.Sprintf("[%s] TTSAudio %d bytes", direction, len(data))
		} else if msgTypeNibble == 0x02 { // AudioOnlyClient
			summary = fmt.Sprintf("[%s] AudioInput %d bytes", direction, len(data))
		}
	}

	mu.Lock()
	if direction == "client→upstream" {
		info.WsRequestMessages = append(info.WsRequestMessages, summary)
	} else {
		info.WsResponseMessages = append(info.WsResponseMessages, summary)
	}
	mu.Unlock()
}

// DoubaoRealtimeHandler 在 client 和 upstream 之间双向转发二进制帧。
// 解析上游 UsageResponse(event=154) 事件进行计费。
func DoubaoRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	errChan := make(chan error, 2)

	sumUsage := &dto.RealtimeUsage{}
	var msgMu sync.Mutex

	// Client → Upstream: 直接透传
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				msgType, data, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}
				collectRealtimeWsMessage(c, info, &msgMu, "client→upstream", data)
				if err := targetConn.WriteMessage(msgType, data); err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	// Upstream → Client: 透传 + 解析 UsageResponse 计费
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				msgType, data, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				collectRealtimeWsMessage(c, info, &msgMu, "upstream→client", data)

				// 尝试从二进制帧中解析 UsageResponse(event=154)
				if usage := tryParseUsageResponse(data); usage != nil {
					sumUsage.TotalTokens += usage.TotalTokens
					sumUsage.InputTokens += usage.InputTokens
					sumUsage.OutputTokens += usage.OutputTokens
					sumUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
					sumUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
					sumUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
					sumUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens

					logger.LogInfo(c, fmt.Sprintf("doubao_realtime usage: input=%d(text=%d,audio=%d), output=%d(text=%d,audio=%d), total=%d",
						usage.InputTokens, usage.InputTokenDetails.TextTokens, usage.InputTokenDetails.AudioTokens,
						usage.OutputTokens, usage.OutputTokenDetails.TextTokens, usage.OutputTokenDetails.AudioTokens,
						usage.TotalTokens))

					if consumeErr := service.PreWssConsumeQuota(c, info, usage); consumeErr != nil {
						errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
						return
					}
				}

				// 转发给客户端
				if err := clientConn.WriteMessage(msgType, data); err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}
			}
		}
	})

	// 等待任一方关闭或出错
	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		logger.LogError(c, "doubao_realtime error: "+err.Error())
	case <-c.Done():
	}

	logger.LogInfo(c, fmt.Sprintf("doubao_realtime finished: input=%d, output=%d, total=%d",
		sumUsage.InputTokens, sumUsage.OutputTokens, sumUsage.TotalTokens))

	return nil, sumUsage
}

// ============================================================
// 二进制帧解析 — 仅解析需要的字段
// ============================================================

// realtimeFrameInfo 精简的帧解析结果
type realtimeFrameInfo struct {
	msgType uint8 // 高 4 位
	flags   uint8 // 低 4 位
	event   int32
	payload []byte
}

// parseRealtimeFrame 从豆包二进制帧中提取 msgType、event、payload
func parseRealtimeFrame(data []byte) (*realtimeFrameInfo, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("frame too short: %d", len(data))
	}

	typeAndFlag := data[1]
	msgTypeNibble := (typeAndFlag >> 4) & 0x0F
	flags := typeAndFlag & 0x0F

	pos := 4 // skip header

	// Sequence: AudioOnlyServer(0x0B) 或 AudioOnlyClient(0x02) 可能带 sequence
	if msgTypeNibble == 0x0B || msgTypeNibble == 0x02 {
		if flags&0x01 == 0x01 { // POS_SEQ or NEG_SEQ
			if pos+4 > len(data) {
				return nil, fmt.Errorf("truncated sequence")
			}
			pos += 4
		}
	}
	// Error(0x0F) 带 errorCode
	if msgTypeNibble == 0x0F {
		if pos+4 > len(data) {
			return nil, fmt.Errorf("truncated error code")
		}
		pos += 4
	}

	var event int32

	// Event bit (0b0100)
	if flags&0x04 == 0x04 {
		if pos+4 > len(data) {
			return nil, fmt.Errorf("truncated event")
		}
		event = int32(data[pos])<<24 | int32(data[pos+1])<<16 | int32(data[pos+2])<<8 | int32(data[pos+3])
		pos += 4

		// SessionID: skip for connection-level events (1,2,50,51,52)
		if event != 1 && event != 2 && event != 50 && event != 51 && event != 52 {
			if pos+4 > len(data) {
				return nil, fmt.Errorf("truncated sessionId size")
			}
			sidLen := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
			pos += 4 + sidLen
		}

		// ConnectID: only for events 50,51,52
		if event == 50 || event == 51 || event == 52 {
			if pos+4 <= len(data) {
				cidLen := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
				pos += 4 + cidLen
			}
		}
	}

	// Payload
	var payload []byte
	if pos+4 <= len(data) {
		payloadLen := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4
		if pos+payloadLen <= len(data) {
			payload = data[pos : pos+payloadLen]
		}
	}

	return &realtimeFrameInfo{
		msgType: msgTypeNibble,
		flags:   flags,
		event:   event,
		payload: payload,
	}, nil
}

// doubaoUsagePayload 对应豆包 UsageResponse 事件的 JSON payload
type doubaoUsagePayload struct {
	Usage struct {
		InputTextTokens   int `json:"input_text_tokens"`
		InputAudioTokens  int `json:"input_audio_tokens"`
		CachedTextTokens  int `json:"cached_text_tokens"`
		CachedAudioTokens int `json:"cached_audio_tokens"`
		OutputTextTokens  int `json:"output_text_tokens"`
		OutputAudioTokens int `json:"output_audio_tokens"`
	} `json:"usage"`
}

// tryParseUsageResponse 尝试从二进制帧中解析 UsageResponse(event=154)
// 返回 nil 表示不是 UsageResponse 或解析失败
func tryParseUsageResponse(data []byte) *dto.RealtimeUsage {
	frame, err := parseRealtimeFrame(data)
	if err != nil {
		return nil
	}

	// FullServer(0x09) + event=154(UsageResponse)
	if frame.msgType != 0x09 || frame.event != 154 {
		return nil
	}

	if len(frame.payload) == 0 {
		return nil
	}

	var up doubaoUsagePayload
	if err := common.Unmarshal(frame.payload, &up); err != nil {
		return nil
	}

	inputTokens := up.Usage.InputTextTokens + up.Usage.InputAudioTokens + up.Usage.CachedTextTokens + up.Usage.CachedAudioTokens
	outputTokens := up.Usage.OutputTextTokens + up.Usage.OutputAudioTokens

	return &dto.RealtimeUsage{
		TotalTokens:  inputTokens + outputTokens,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens:  up.Usage.InputTextTokens + up.Usage.CachedTextTokens,
			AudioTokens: up.Usage.InputAudioTokens + up.Usage.CachedAudioTokens,
		},
		OutputTokenDetails: dto.OutputTokenDetails{
			TextTokens:  up.Usage.OutputTextTokens,
			AudioTokens: up.Usage.OutputAudioTokens,
		},
	}
}

// eventNameForLog 返回事件 ID 的可读名称（用于日志）
func eventNameForLog(event int32) string {
	names := map[int32]string{
		1: "StartConnection", 2: "FinishConnection",
		50: "ConnectionStarted", 51: "ConnectionFailed", 52: "ConnectionFinished",
		100: "StartSession", 102: "FinishSession",
		150: "SessionStarted", 152: "SessionFinished", 153: "SessionFailed",
		154: "UsageResponse",
		200: "TaskRequest", 201: "UpdateConfig", 251: "ConfigUpdated",
		300: "SayHello",
		350: "TTSSentenceStart", 351: "TTSSentenceEnd", 352: "TTSResponse", 359: "TTSEnded",
		400: "EndASR",
		450: "ASRInfo", 451: "ASRResponse", 459: "ASREnded",
		500: "ChatTTSText", 501: "ChatTextQuery", 502: "ChatRAGText",
		550: "ChatResponse", 553: "ChatTextQueryConfirmed", 559: "ChatEnded",
		599: "DialogCommonError",
	}
	if name, ok := names[event]; ok {
		return name
	}
	return fmt.Sprintf("Event(%d)", event)
}
