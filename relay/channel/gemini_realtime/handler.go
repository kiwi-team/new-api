package gemini_realtime

import (
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// collectWsMessage appends a raw message summary to the relay info for logging.
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

// GeminiRealtimeHandler starts bidirectional message forwarding between the client
// and upstream Gemini Live API WebSocket connections. Messages are passed through
// transparently (the client speaks Gemini Live protocol directly). Usage is extracted
// from usageMetadata fields in server messages for billing.
func GeminiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
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
	lastUsage := &dto.RealtimeUsage{}

	var msgMu sync.Mutex

	// Client → Upstream (direct passthrough)
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
				collectWsMessage(c, info, &msgMu, "client→upstream", message)
				err = targetConn.WriteMessage(msgType, message)
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	// Upstream → Client (passthrough + parse usageMetadata for billing)
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

				// Try to parse for usage extraction; failures are non-fatal
				event := &GeminiLiveEvent{}
				parseErr := common.Unmarshal(message, event)
				if parseErr != nil {
					logger.LogWarn(c, fmt.Sprintf("gemini_realtime: failed to parse downstream message: %v", parseErr))
				} else if event.UsageMetadata != nil {
					// Gemini reports cumulative usage; compute incremental delta
					currentUsage := event.UsageMetadata.ToRealtimeUsage()
					deltaUsage := ComputeDelta(lastUsage, currentUsage)
					if deltaUsage.TotalTokens > 0 {
						consumeErr := geminiPreConsumeUsage(c, info, deltaUsage, sumUsage)
						if consumeErr != nil {
							errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
							return
						}
					}
					lastUsage = currentUsage
					logger.LogInfo(c, fmt.Sprintf("gemini_realtime usage: input=%d, output=%d, total=%d",
						currentUsage.InputTokens, currentUsage.OutputTokens, currentUsage.TotalTokens))
				}

				// Forward message to client
				err = helper.WssString(c, clientConn, string(message))
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
		logger.LogError(c, "gemini_realtime error: "+err.Error())
	case <-c.Done():
	}

	return nil, sumUsage
}

// computeDelta computes the incremental usage between the last reported cumulative usage
// and the current cumulative usage. Gemini Live API reports cumulative totals.
func ComputeDelta(last, current *dto.RealtimeUsage) *dto.RealtimeUsage {
	delta := &dto.RealtimeUsage{}
	delta.TotalTokens = max(0, current.TotalTokens-last.TotalTokens)
	delta.InputTokens = max(0, current.InputTokens-last.InputTokens)
	delta.OutputTokens = max(0, current.OutputTokens-last.OutputTokens)
	delta.InputTokenDetails.TextTokens = max(0, current.InputTokenDetails.TextTokens-last.InputTokenDetails.TextTokens)
	delta.InputTokenDetails.AudioTokens = max(0, current.InputTokenDetails.AudioTokens-last.InputTokenDetails.AudioTokens)
	delta.OutputTokenDetails.TextTokens = max(0, current.OutputTokenDetails.TextTokens-last.OutputTokenDetails.TextTokens)
	delta.OutputTokenDetails.AudioTokens = max(0, current.OutputTokenDetails.AudioTokens-last.OutputTokenDetails.AudioTokens)
	return delta
}

// geminiPreConsumeUsage accumulates usage into totalUsage and triggers pre-consumption billing.
func geminiPreConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens

	return service.PreWssConsumeQuota(ctx, info, usage)
}
