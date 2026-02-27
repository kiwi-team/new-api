package qwen_realtime

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
				// Direct passthrough: write raw message to upstream without parsing
				collectWsMessage(c, info, &msgMu, "client→upstream", message)
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

				// Try to parse for usage extraction; failures are non-fatal
				event := &QwenRealtimeEvent{}
				parseErr := common.Unmarshal(message, event)
				if parseErr != nil {
					logger.LogWarn(c, fmt.Sprintf("qwen_realtime: failed to parse downstream message: %v", parseErr))
				} else if event.Type == QwenEventResponseDone && event.Response != nil && event.Response.Usage != nil {
					accumulateUsage(usage, event.Response.Usage)
					consumeErr := qwenPreConsumeUsage(c, info, usage, sumUsage)
					if consumeErr != nil {
						errChan <- fmt.Errorf("error consume usage: %v", consumeErr)
						return
					}
					// Billing done for this round, reset
					usage = &dto.RealtimeUsage{}
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
		logger.LogError(c, "qwen_realtime error: "+err.Error())
	case <-c.Done():
	}

	// Flush any remaining usage before returning
	if usage.TotalTokens != 0 {
		_ = qwenPreConsumeUsage(c, info, usage, sumUsage)
	}

	return nil, sumUsage
}

// qwenPreConsumeUsage accumulates usage into totalUsage and triggers pre-consumption billing.
// Follows the same pattern as preConsumeUsage in relay/channel/openai/relay-openai.go.
func qwenPreConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
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

	return service.PreWssConsumeQuota(ctx, info, usage)
}
