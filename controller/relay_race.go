package controller

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	raceLogStateKey = "channel_race_log_state"
	raceTerminalKey = "channel_race_terminal"
)

var channelRaceAttempts sync.WaitGroup

func WaitChannelRaceAttempts(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		channelRaceAttempts.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

type raceLogState struct {
	mu      sync.Mutex
	success bool
}

func (s *raceLogState) markSuccess(requestId string) {
	s.mu.Lock()
	if s.success {
		s.mu.Unlock()
		return
	}
	s.success = true
	s.mu.Unlock()
	if model.LOG_DB == nil {
		return
	}
	if err := model.ClearErrorLogBodiesByRequestID(requestId); err != nil {
		common.SysError("failed to clear race error log bodies: " + err.Error())
	}
}

type raceCoordinator struct {
	mu             sync.Mutex
	winner         int
	completedCount int
	terminal       bool
	winnerCh       chan raceWinner
}

type raceWinner struct {
	index     int
	channelId int
	done      <-chan struct{}
}

func (c *raceCoordinator) tryWin(winner raceWinner) bool {
	c.mu.Lock()
	if c.winner != 0 || c.terminal {
		c.mu.Unlock()
		return false
	}
	c.winner = winner.index + 1
	c.mu.Unlock()
	c.winnerCh <- winner
	return true
}

func (c *raceCoordinator) markAttemptDone(totalCandidates int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedCount++
	if c.winner != 0 || c.terminal || c.completedCount < totalCandidates {
		return false
	}
	c.terminal = true
	return true
}

type raceResponseWriter struct {
	mu          sync.Mutex
	parent      gin.ResponseWriter
	header      http.Header
	status      int
	size        int
	wroteHeader bool
	buffer      bytes.Buffer
	committed   bool
	discarded   bool
	coordinator *raceCoordinator
	logState    *raceLogState
	requestId   string
	attempt     raceWinner
	ctx         *gin.Context
}

func newRaceResponseWriter(parent gin.ResponseWriter, coordinator *raceCoordinator, logState *raceLogState, requestId string, attempt raceWinner, ctx *gin.Context) *raceResponseWriter {
	return &raceResponseWriter{
		parent:      parent,
		header:      make(http.Header),
		status:      http.StatusOK,
		coordinator: coordinator,
		logState:    logState,
		requestId:   requestId,
		attempt:     attempt,
		ctx:         ctx,
	}
}

func (w *raceResponseWriter) Header() http.Header { return w.header }

func (w *raceResponseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed || w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
}

func (w *raceResponseWriter) WriteHeaderNow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		w.parent.WriteHeaderNow()
	}
}

func (w *raceResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.discarded {
		w.size += len(data)
		return len(data), nil
	}
	if w.committed {
		n, err := w.parent.Write(data)
		w.size += n
		return n, err
	}
	w.buffer.Write(data)
	w.size += len(data)
	if w.status >= http.StatusBadRequest || !w.hasMeaningfulData() {
		return len(data), nil
	}

	isWinner := w.coordinator.tryWin(w.attempt)
	w.logState.markSuccess(w.requestId)
	if !isWinner {
		w.ctx.Set(common.KeyChannelRaceResult, "loser")
		w.discarded = true
		w.buffer.Reset()
		return len(data), nil
	}
	w.ctx.Set(common.KeyChannelRaceResult, "winner")
	w.committed = true
	copyHeaders(w.parent.Header(), w.header)
	w.parent.WriteHeader(w.status)
	_, err := w.parent.Write(w.buffer.Bytes())
	w.buffer.Reset()
	return len(data), err
}

func (w *raceResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *raceResponseWriter) hasMeaningfulData() bool {
	data := w.buffer.Bytes()
	contentType := strings.ToLower(w.header.Get("Content-Type"))
	isStream := common.GetContextKeyBool(w.ctx, constant.ContextKeyIsStream)
	if !isStream && !strings.Contains(contentType, "text/event-stream") {
		return len(bytes.TrimSpace(data)) > 0
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") && strings.TrimSpace(strings.TrimPrefix(line, "data:")) != "" {
			return true
		}
	}
	return false
}

func (w *raceResponseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		w.parent.Flush()
	}
}

func (w *raceResponseWriter) Status() int              { w.mu.Lock(); defer w.mu.Unlock(); return w.status }
func (w *raceResponseWriter) Size() int                { w.mu.Lock(); defer w.mu.Unlock(); return w.size }
func (w *raceResponseWriter) Written() bool            { w.mu.Lock(); defer w.mu.Unlock(); return w.size > 0 }
func (w *raceResponseWriter) CloseNotify() <-chan bool { return make(chan bool) }
func (w *raceResponseWriter) Pusher() http.Pusher      { return nil }
func (w *raceResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijacking is unsupported in channel race mode")
}

func (w *raceResponseWriter) errorSnapshot() (int, http.Header, []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status, w.header.Clone(), bytes.Clone(w.buffer.Bytes())
}

func (w *raceResponseWriter) recordInternalFailure(message string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed || w.discarded {
		return
	}
	w.status = http.StatusInternalServerError
	w.wroteHeader = true
	w.buffer.Reset()
	_, _ = fmt.Fprintf(&w.buffer, `{"error":{"message":%q}}`, message)
}

func copyHeaders(dst http.Header, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}

type raceAttemptDone struct {
	index  int
	writer *raceResponseWriter
}

func isRaceSupported(c *gin.Context, relayFormat types.RelayFormat) bool {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return false
	}
	switch relayFormat {
	case types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses,
		types.RelayFormatOpenAIResponsesCompaction:
		return true
	case types.RelayFormatOpenAI:
		mode := relayconstant.Path2RelayMode(c.Request.URL.Path)
		return mode == relayconstant.RelayModeChatCompletions || mode == relayconstant.RelayModeCompletions
	default:
		return false
	}
}

func relayRace(c *gin.Context, relayFormat types.RelayFormat, plan hostdto.ChannelRacePlan) {
	groups := make([][]int, 0, len(plan.Groups))
	for _, configured := range plan.Groups {
		group := append([]int(nil), configured...)
		common.ShuffleSlice(group)
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	candidates := make([]int, 0)
	for _, group := range groups {
		candidates = append(candidates, group...)
	}
	if len(candidates) == 0 {
		RelayWithoutRace(c, relayFormat)
		return
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	base := c.Copy()
	c.Set(common.KeyBodyStorage, nil)
	requestId := c.GetString(common.RequestIdKey)
	coordinator := &raceCoordinator{winnerCh: make(chan raceWinner, 1)}
	logState := &raceLogState{}
	doneCh := make(chan raceAttemptDone, len(candidates))
	var attempts sync.WaitGroup
	current := 0
	var terminalFailureWriter *raceResponseWriter

	launch := func(index int, channelId int) {
		attempts.Add(1)
		channelRaceAttempts.Add(1)
		done := make(chan struct{})
		child := base.Copy()
		child.Set(common.KeyChannelRacePlan, nil)
		child.Set(common.KeyChannelRaceAttempt, true)
		child.Set(common.KeyChannelRaceResult, "loser")
		child.Set(common.KeyChannelRaceBillingRequestId, common.NewRequestId())
		child.Set(raceLogStateKey, logState)
		child.Set(raceTerminalKey, index == len(candidates)-1)
		child.Set("token_channel_ids", []int{channelId})
		child.Set("new_retry_times", 0)
		child.Set("use_channel", []string{})
		common.SetContextKey(child, constant.ContextKeyUseChannelTime, []int64{})
		if channel, channelErr := model.CacheGetChannel(channelId); channelErr == nil && channel != nil {
			if setupErr := middleware.SetupContextForSelectedChannel(child, channel, common.GetContextKeyString(child, constant.ContextKeyOriginalModel)); setupErr == nil {
				child.Set(common.KeyChannelRaceChannelPrepared, true)
			}
		}
		attempt := raceWinner{index: index, channelId: channelId, done: done}
		writer := newRaceResponseWriter(c.Writer, coordinator, logState, requestId, attempt, child)
		child.Writer = writer
		go func() {
			defer attempts.Done()
			defer channelRaceAttempts.Done()
			defer close(done)
			defer func() {
				if recovered := recover(); recovered != nil {
					writer.recordInternalFailure(fmt.Sprintf("race attempt panic: %v", recovered))
				}
				doneCh <- raceAttemptDone{index: index, writer: writer}
			}()
			view, viewErr := common.NewBodyStorageView(storage)
			if viewErr != nil {
				writer.recordInternalFailure(viewErr.Error())
				return
			}
			defer view.Close()
			child.Set(common.KeyBodyStorage, view)
			request := base.Request.Clone(context.WithoutCancel(base.Request.Context()))
			request.Body = view
			request.GetBody = view.NewReader
			child.Request = request
			RelayWithoutRace(child, relayFormat)
		}()
	}
	launchNext := func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		if coordinator.winner != 0 || coordinator.terminal || current >= len(candidates)-1 {
			return false
		}
		current++
		launch(current, candidates[current])
		return true
	}

	cleanup := func() {
		go func() {
			attempts.Wait()
			_ = storage.Close()
		}()
	}

	timeout := time.Duration(plan.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(hostdto.DefaultRaceTimeoutSeconds) * time.Second
	}
	launch(current, candidates[current])
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case winner := <-coordinator.winnerCh:
			c.Set(common.KeyChannelRaceWinnerChannelId, winner.channelId)
			<-winner.done
			cleanup()
			return
		case completed := <-doneCh:
			if completed.index == len(candidates)-1 {
				terminalFailureWriter = completed.writer
			}
			if coordinator.markAttemptDone(len(candidates)) {
				failureWriter := terminalFailureWriter
				if failureWriter == nil {
					failureWriter = completed.writer
				}
				status, header, body := failureWriter.errorSnapshot()
				if status < http.StatusBadRequest {
					status = http.StatusBadGateway
					body = []byte(`{"error":{"message":"all race channels failed before returning data"}}`)
				}
				copyHeaders(c.Writer.Header(), header)
				c.Writer.WriteHeader(status)
				_, _ = c.Writer.Write(body)
				cleanup()
				return
			}
			if completed.index != current || current == len(candidates)-1 {
				continue
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if launchNext() {
				timer.Reset(timeout)
			}
		case <-timer.C:
			if launchNext() {
				timer.Reset(timeout)
			}
		}
	}
}
