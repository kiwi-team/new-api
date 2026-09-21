package qwen_realtime

import (
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// collectWsMessage parses the event type and appends the full message to the appropriate
// message slice on info for database logging.
func collectWsMessage(c *gin.Context, info *relaycommon.RelayInfo, mu *sync.Mutex, direction string, message []byte) {
	summary := fmt.Sprintf("[%s] %s", direction, string(message))

	mu.Lock()
	if direction == "client→upstream" {
		info.WsRequestMessages = append(info.WsRequestMessages, summary)
	} else {
		info.WsResponseMessages = append(info.WsResponseMessages, summary)
	}
	mu.Unlock()
}

// QwenRealtimeHandler starts bidirectional message forwarding between the client
// and upstream Qwen Realtime WebSocket connections. It accumulates usage from
// response.done events and returns the total RealtimeUsage when the session ends.
func QwenRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	var msgMu sync.Mutex
	isFunASR := isFunASRRealtimeModel(info.UpstreamModelName)
	isQwen3ASR := isQwen3ASRRealtimeModel(info.UpstreamModelName)
	asrAccounted := make(map[string]int)
	qwen3ASRTracker := newQwen3ASRUsageTracker()

	// Upstream goroutine: Client → Upstream (direct passthrough, no parsing)
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
				msgType, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}
				if isQwen3ASR && msgType != websocket.TextMessage {
					errChan <- fmt.Errorf("Qwen3 ASR only accepts JSON text messages with base64 PCM audio")
					return
				}
				if isFunASR && msgType == websocket.TextMessage {
					rewritten, rewriteErr := rewriteASRRunTaskModel(message, info.UpstreamModelName)
					if rewriteErr != nil {
						errChan <- fmt.Errorf("invalid ASR control message: %v", rewriteErr)
						return
					}
					message = rewritten
				}
				if isQwen3ASR && msgType == websocket.TextMessage {
					delta, clamp, trackErr := qwen3ASRTracker.consume(message)
					if trackErr != nil {
						errChan <- fmt.Errorf("invalid Qwen3 ASR audio event: %v", trackErr)
						return
					}
					if clamp != nil && info.QuotaClamp == nil {
						info.QuotaClamp = clamp
					}
					if delta != nil {
						if consumeErr := qwenReserveUsage(c, info, delta, sumUsage); consumeErr != nil {
							errChan <- fmt.Errorf("reserve Qwen3 ASR usage: %v", consumeErr)
							return
						}
					}
				}
				// Binary audio and text control messages retain their original frame type.
				if msgType != websocket.BinaryMessage {
					collectWsMessage(c, info, &msgMu, "client→upstream", message)
				}
				err = targetConn.WriteMessage(msgType, message)
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	// Downstream goroutine: Upstream → Client (passthrough + parse response.done for usage)
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
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				collectWsMessage(c, info, &msgMu, "upstream→client", message)

				// Parse usage while preserving the provider-native messages sent to the client.
				if isFunASR {
					delta, clamp, parseErr := asrUsageDelta(message, asrAccounted)
					if parseErr != nil {
						logger.LogWarn(c, fmt.Sprintf("qwen_realtime: failed to parse ASR downstream message: %v", parseErr))
					} else if delta != nil {
						if clamp != nil && info.QuotaClamp == nil {
							info.QuotaClamp = clamp
						}
						if consumeErr := qwenReserveUsage(c, info, delta, sumUsage); consumeErr != nil {
							errChan <- fmt.Errorf("error consume ASR usage: %v", consumeErr)
							return
						}
					}
				} else if !isQwen3ASR {
					event := &QwenRealtimeEvent{}
					parseErr := common.Unmarshal(message, event)
					if parseErr != nil {
						logger.LogWarn(c, fmt.Sprintf("qwen_realtime: failed to parse downstream message: %v", parseErr))
					} else if event.Type == QwenEventResponseDone && event.Response != nil && event.Response.Usage != nil {
						accumulateUsage(usage, event.Response.Usage)
						consumeErr := qwenReserveUsage(c, info, usage, sumUsage)
						if consumeErr != nil {
							errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
							return
						}
						usage = &dto.RealtimeUsage{}
					}
				}

				// Forward message to client
				err = helper.WssResponseString(c, info, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}
			}
		}
	})

	// Wait for either side to close or an error
	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		logger.LogError(c, "qwen_realtime error: "+err.Error())
	case <-c.Done():
	}

	// Flush any remaining usage before returning
	if usage.TotalTokens != 0 {
		_ = qwenReserveUsage(c, info, usage, sumUsage)
	}

	return nil, sumUsage
}

// qwenReserveUsage accumulates usage and raises the billing reservation to the
// cumulative cost. Final settlement charges only the remaining delta.
func qwenReserveUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.InputTokenDetails.VideoTokens += usage.InputTokenDetails.VideoTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens

	return service.ReserveWssUsage(ctx, info, totalUsage)
}
