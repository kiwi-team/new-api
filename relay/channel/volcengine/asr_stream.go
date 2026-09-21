package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	seedASRStreamingURL        = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
	seedASRStreamingResourceID = "volc.seedasr.sauc.duration"
	seedASRMaxFrameBytes       = 8 << 20
)

type seedASRFrame struct {
	messageType uint8
	flags       uint8
	compression uint8
	errorCode   uint32
	payload     []byte
}

func isSeedASRStreaming(info *relaycommon.RelayInfo) bool {
	return info != nil && info.UpstreamModelName == "doubao-seed-asr-2.0-streaming"
}

func handleSeedASRStreaming(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(errors.New("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}
	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs
	type relayResult struct {
		err error
	}
	results := make(chan relayResult, 2)
	var stateMu sync.Mutex
	var logMu sync.Mutex
	var maxDurationMS int64

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				results <- relayResult{err: fmt.Errorf("panic in Seed-ASR client relay: %v", recovered)}
			}
		}()
		receivedConfig := false
		finished := false
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				results <- relayResult{err: normalizeSeedASRWebSocketError(err)}
				return
			}
			if messageType != websocket.BinaryMessage {
				results <- relayResult{err: errors.New("Seed-ASR only accepts binary WebSocket messages")}
				return
			}
			frame, err := parseSeedASRFrame(data)
			if err != nil {
				results <- relayResult{err: err}
				return
			}
			switch frame.messageType {
			case 0x1:
				if receivedConfig {
					results <- relayResult{err: errors.New("duplicate Seed-ASR configuration frame")}
					return
				}
				receivedConfig = true
			case 0x2:
				if !receivedConfig || finished {
					results <- relayResult{err: errors.New("invalid Seed-ASR audio frame sequence")}
					return
				}
				if frame.flags == 0x2 || frame.flags == 0x3 {
					finished = true
				}
			default:
				results <- relayResult{err: fmt.Errorf("unsupported Seed-ASR client message type 0x%x", frame.messageType)}
				return
			}
			appendSeedASRAudit(info, &logMu, true, frame.messageType, len(data))
			if err := targetConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				results <- relayResult{err: fmt.Errorf("write Seed-ASR upstream: %w", err)}
				return
			}
		}
	}()

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				results <- relayResult{err: fmt.Errorf("panic in Seed-ASR upstream relay: %v", recovered)}
			}
		}()
		for {
			messageType, data, err := targetConn.ReadMessage()
			if err != nil {
				results <- relayResult{err: normalizeSeedASRWebSocketError(err)}
				return
			}
			if messageType != websocket.BinaryMessage {
				results <- relayResult{err: errors.New("Seed-ASR upstream returned a non-binary message")}
				return
			}
			frame, err := parseSeedASRFrame(data)
			if err != nil {
				results <- relayResult{err: fmt.Errorf("invalid Seed-ASR upstream frame: %w", err)}
				return
			}
			info.SetFirstResponseTime()
			appendSeedASRAudit(info, &logMu, false, frame.messageType, len(data))
			if frame.messageType == 0x9 {
				if duration, ok := seedASRFrameDuration(frame); ok {
					stateMu.Lock()
					maxDurationMS = max(maxDurationMS, duration)
					stateMu.Unlock()
				}
			}
			if err := clientConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				results <- relayResult{err: fmt.Errorf("write Seed-ASR client: %w", err)}
				return
			}
			if frame.messageType == 0xf {
				results <- relayResult{err: fmt.Errorf("Seed-ASR upstream error %d", frame.errorCode)}
				return
			}
		}
	}()

	first := <-results
	_ = clientConn.Close()
	_ = targetConn.Close()
	select {
	case <-results:
	default:
	}
	if first.err != nil {
		logger.LogError(c, "Seed-ASR streaming ended: "+first.err.Error())
	}
	stateMu.Lock()
	durationMS := maxDurationMS
	stateMu.Unlock()
	seconds := int(math.Ceil(float64(durationMS) / 1000))
	audioTokens, clamp := common.QuotaRoundChecked(float64(seconds) / 60 * 1000)
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	usage := &dto.RealtimeUsage{
		InputTokens: audioTokens,
		TotalTokens: audioTokens,
		InputTokenDetails: dto.InputTokenDetails{
			AudioTokens: audioTokens,
		},
	}
	return nil, usage
}

func parseSeedASRFrame(data []byte) (*seedASRFrame, error) {
	if len(data) < 8 || len(data) > seedASRMaxFrameBytes {
		return nil, fmt.Errorf("invalid Seed-ASR frame size %d", len(data))
	}
	if data[0]>>4 != 1 {
		return nil, fmt.Errorf("unsupported Seed-ASR protocol version %d", data[0]>>4)
	}
	headerBytes := int(data[0]&0x0f) * 4
	if headerBytes < 4 || headerBytes+4 > len(data) {
		return nil, errors.New("invalid Seed-ASR header length")
	}
	frame := &seedASRFrame{
		messageType: data[1] >> 4,
		flags:       data[1] & 0x0f,
		compression: data[2] & 0x0f,
	}
	position := headerBytes
	if frame.messageType == 0x2 || frame.messageType == 0x9 {
		if frame.flags&0x1 != 0 {
			if position+4 > len(data) {
				return nil, errors.New("truncated Seed-ASR sequence")
			}
			position += 4
		}
	}
	if frame.messageType == 0xf {
		if position+4 > len(data) {
			return nil, errors.New("truncated Seed-ASR error code")
		}
		frame.errorCode = binary.BigEndian.Uint32(data[position : position+4])
		position += 4
	}
	if position+4 > len(data) {
		return nil, errors.New("missing Seed-ASR payload length")
	}
	payloadLength := int(binary.BigEndian.Uint32(data[position : position+4]))
	position += 4
	if payloadLength < 0 || payloadLength != len(data)-position {
		return nil, errors.New("Seed-ASR payload length does not match frame")
	}
	frame.payload = data[position:]
	return frame, nil
}

func seedASRFrameDuration(frame *seedASRFrame) (int64, bool) {
	payload := frame.payload
	if frame.compression == 1 {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return 0, false
		}
		decompressed, err := io.ReadAll(io.LimitReader(reader, seedASRMaxFrameBytes+1))
		_ = reader.Close()
		if err != nil || len(decompressed) > seedASRMaxFrameBytes {
			return 0, false
		}
		payload = decompressed
	} else if frame.compression != 0 {
		return 0, false
	}
	var result struct {
		AudioInfo struct {
			Duration int64 `json:"duration"`
		} `json:"audio_info"`
	}
	if err := common.Unmarshal(payload, &result); err != nil || result.AudioInfo.Duration < 0 {
		return 0, false
	}
	return result.AudioInfo.Duration, true
}

func appendSeedASRAudit(info *relaycommon.RelayInfo, mu *sync.Mutex, clientToUpstream bool, messageType uint8, bytes int) {
	summary := fmt.Sprintf("[Seed-ASR] type=0x%x %d bytes", messageType, bytes)
	mu.Lock()
	defer mu.Unlock()
	if clientToUpstream {
		info.WsRequestMessages = append(info.WsRequestMessages, summary)
	} else {
		info.WsResponseMessages = append(info.WsResponseMessages, summary)
	}
}

func normalizeSeedASRWebSocketError(err error) error {
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return nil
	}
	return err
}

func setupSeedASRStreamingHeaders(c *gin.Context, header http.Header, info *relaycommon.RelayInfo) error {
	if err := setupSeedASRHeaders(c, header, info, seedASRStreamingResourceID); err != nil {
		return err
	}
	requestID := c.GetString(contextKeyASRRequest)
	header.Set("X-Api-Sequence", "-1")
	header.Set("X-Api-Connect-Id", requestID)
	return nil
}
