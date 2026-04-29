package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10 // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 64 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string) bool) {

	if resp == nil || dataHandler == nil {
		return
	}

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	scanner.Split(bufio.ScanLines)

	ticker := time.NewTicker(streamingTimeout)
	defer ticker.Stop()

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}
	var pingTickerC <-chan time.Time
	if pingEnabled {
		pt := time.NewTicker(pingInterval)
		defer pt.Stop()
		pingTickerC = pt.C
	}

	if common.DebugEnabled {
		println("relay timeout seconds:", common.RelayTimeout)
		println("relay max idle conns:", common.RelayMaxIdleConns)
		println("relay max idle conns per host:", common.RelayMaxIdleConnsPerHost)
		println("streaming timeout seconds:", int64(streamingTimeout.Seconds()))
		println("ping interval seconds:", int64(pingInterval.Seconds()))
	}

	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(context.Background())

	// Buffered by 1 so the scanner can prefetch one line while main writes.
	// Closed by the scanner goroutine (under defer) when scanning ends —
	// signals end-of-stream to the main loop.
	lineCh := make(chan string, 1)

	var wg sync.WaitGroup

	// Cleanup order on return:
	//   1) cancel() — signals scanner via ctx.Done() if it's parked sending to lineCh.
	//   2) resp.Body.Close() — unblocks scanner.Scan() if it's parked reading from
	//      the upstream connection (ctx.Done() alone cannot unblock a network Read).
	//   3) wg.Wait() — guarantees scanner has fully exited before we return,
	//      so close(lineCh) has happened and no goroutine survives the handler.
	defer func() {
		cancel()
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		wg.Wait()
	}()

	// Scanner goroutine: parses upstream SSE lines and forwards stripped
	// payloads via lineCh. Any client write (data + ping) is performed by
	// the main goroutine below — keeping all writes on a single goroutine
	// removes the need for a write mutex.
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
			}
			close(lineCh)
			wg.Done()
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

		for scanner.Scan() {
			// Cheap stop-check; the blocking send below also watches ctx.
			if ctx.Err() != nil {
				return
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			if common.DebugEnabled {
				println(data)
			}

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimLeft(data, " ")
			data = strings.TrimSuffix(data, "\r")
			if strings.HasPrefix(data, "[DONE]") {
				if common.DebugEnabled {
					println("received [DONE], stopping scanner")
				}
				return
			}

			info.SetFirstResponseTime()
			info.ReceivedResponseCount++

			select {
			case lineCh <- data:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			logger.LogError(c, "scanner error: "+err.Error())
		}
	})

	// Main loop: single writer for both data lines and pings — no mutex,
	// no per-line goroutine spawn, no per-line timer allocation.
	for {
		select {
		case data, ok := <-lineCh:
			if !ok {
				// Scanner finished cleanly ([DONE] / EOF / error).
				logger.LogInfo(c, "streaming finished")
				return
			}
			if !dataHandler(data) {
				return
			}
		case <-pingTickerC:
			if err := PingData(c); err != nil {
				logger.LogError(c, "ping data error: "+err.Error())
				return
			}
			if common.DebugEnabled {
				println("ping data sent")
			}
		case <-ticker.C:
			logger.LogError(c, "streaming timeout")
			return
		case <-c.Request.Context().Done():
			logger.LogInfo(c, "client disconnected")
			return
		}
	}
}
