package volcengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

const (
	seeduplexDialogueURL = "wss://openspeech.bytedance.com/api/v3/duplex/realtime/dialogue"
	seeduplexModel       = "seeduplex-1.2.6.1"
	seeduplexVersion     = "1.2.6.1"
	seeduplexFrameLimit  = 1 << 20
	seeduplexAuditLimit  = 256 << 10
)

var errSeeduplexProvider = errors.New("Seeduplex returned an error event")

func isSeeduplex(info *relaycommon.RelayInfo) bool {
	return info != nil && info.UpstreamModelName == seeduplexModel
}

type seeduplexEvent struct {
	Type       string          `json:"type"`
	EventID    string          `json:"event_id"`
	ResponseID string          `json:"response_id"`
	Usage      json.RawMessage `json:"usage"`
	Response   *struct {
		ID     string          `json:"id"`
		Status string          `json:"status"`
		Usage  json.RawMessage `json:"usage"`
	} `json:"response"`
}

// Seeduplex documents response.done as per-turn usage. Recognize the native
// six disjoint counters and the inclusive Realtime form explicitly; never
// substitute local OpenAI audio estimates for an unrecognized usage payload.
func parseSeeduplexUsage(data []byte) (*dto.RealtimeUsage, error) {
	var reported struct {
		InputText   *int `json:"input_text_tokens"`
		InputAudio  *int `json:"input_audio_tokens"`
		CachedText  *int `json:"cached_text_tokens"`
		CachedAudio *int `json:"cached_audio_tokens"`
		OutputText  *int `json:"output_text_tokens"`
		OutputAudio *int `json:"output_audio_tokens"`

		InputTokens   *int                    `json:"input_tokens"`
		OutputTokens  *int                    `json:"output_tokens"`
		TotalTokens   *int                    `json:"total_tokens"`
		InputDetails  *dto.InputTokenDetails  `json:"input_token_details"`
		OutputDetails *dto.OutputTokenDetails `json:"output_token_details"`
	}
	if err := common.Unmarshal(data, &reported); err != nil {
		return nil, errors.New("invalid Seeduplex usage JSON")
	}
	var counts [6]int
	if reported.InputText != nil || reported.InputAudio != nil || reported.CachedText != nil || reported.CachedAudio != nil || reported.OutputText != nil || reported.OutputAudio != nil {
		for i, value := range []*int{reported.InputText, reported.InputAudio, reported.CachedText, reported.CachedAudio, reported.OutputText, reported.OutputAudio} {
			if value == nil {
				return nil, errors.New("incomplete Seeduplex native usage counters")
			}
			counts[i] = *value
		}
	} else {
		if reported.InputTokens == nil || reported.OutputTokens == nil || reported.InputDetails == nil || reported.OutputDetails == nil {
			return nil, errors.New("unrecognized Seeduplex usage; modality counters are required")
		}
		input, output := reported.InputDetails, reported.OutputDetails
		if input.ImageTokens != 0 || input.VideoTokens != 0 || output.ImageTokens != 0 || input.CachedCreationTokens != 0 || input.CacheWriteTokens != 0 {
			return nil, errors.New("unsupported Seeduplex usage modality")
		}
		cachedText, cachedAudio := 0, 0
		if input.CachedTokens != 0 || input.CachedTokensDetails != nil {
			details := input.CachedTokensDetails
			if details == nil || details.TextTokens == nil || details.AudioTokens == nil {
				return nil, errors.New("Seeduplex cache usage requires text and audio breakdown")
			}
			cachedText, cachedAudio = *details.TextTokens, *details.AudioTokens
			if details.ImageTokens != nil && *details.ImageTokens != 0 {
				return nil, errors.New("unsupported Seeduplex image cache usage")
			}
			if cachedText < 0 || cachedAudio < 0 || cachedText > input.CachedTokens || cachedAudio != input.CachedTokens-cachedText {
				return nil, errors.New("inconsistent Seeduplex cache usage")
			}
		}
		if cachedText > input.TextTokens || cachedAudio > input.AudioTokens {
			return nil, errors.New("Seeduplex cache usage exceeds modality totals")
		}
		counts = [6]int{input.TextTokens - cachedText, input.AudioTokens - cachedAudio, cachedText, cachedAudio, output.TextTokens, output.AudioTokens}
	}
	// Bound before adding or converting any provider-controlled counters.
	total := 0
	for _, count := range counts {
		if count < 0 || count > common.MaxQuota-total {
			return nil, errors.New("Seeduplex usage counters are outside the supported range")
		}
		total += count
	}
	input := counts[0] + counts[1] + counts[2] + counts[3]
	output := counts[4] + counts[5]
	if reported.InputTokens != nil && *reported.InputTokens != input || reported.OutputTokens != nil && *reported.OutputTokens != output || reported.TotalTokens != nil && *reported.TotalTokens != total {
		return nil, errors.New("Seeduplex usage totals do not match modality counters")
	}
	return &dto.RealtimeUsage{
		InputTokens: input, OutputTokens: output, TotalTokens: total,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: counts[0] + counts[2], AudioTokens: counts[1] + counts[3], CachedTokens: counts[2] + counts[3],
			CachedTokensDetails: &dto.CachedTokenDetails{TextTokens: &counts[2], AudioTokens: &counts[3]},
		},
		OutputTokenDetails: dto.OutputTokenDetails{TextTokens: counts[4], AudioTokens: counts[5]},
	}, nil
}

type seeduplexUsageTracker struct {
	total dto.RealtimeUsage
	seen  map[string][6]int
}

func (t *seeduplexUsageTracker) consume(event seeduplexEvent) (bool, error) {
	if event.Type != "response.done" {
		return false, nil
	}
	data, id := event.Usage, event.EventID
	if len(data) == 0 && event.Response != nil {
		data = event.Response.Usage
	}
	usage, err := parseSeeduplexUsage(data)
	if err != nil {
		return false, err
	}
	if id == "" {
		id = event.ResponseID
		if id == "" && event.Response != nil {
			id = event.Response.ID
		}
		if id != "" {
			id = "response:" + id
		}
	} else {
		id = "event:" + id
	}
	if len(id) > 256 {
		return false, errors.New("Seeduplex usage identifier is too long")
	}
	details := usage.InputTokenDetails.CachedTokensDetails
	counts := [6]int{usage.InputTokenDetails.TextTokens, usage.InputTokenDetails.AudioTokens, *details.TextTokens, *details.AudioTokens, usage.OutputTokenDetails.TextTokens, usage.OutputTokenDetails.AudioTokens}
	if previous, exists := t.seen[id]; id != "" && exists {
		if previous != counts {
			return false, errors.New("conflicting Seeduplex usage for the same event")
		}
		return false, nil
	}
	if usage.TotalTokens > common.MaxQuota-t.total.TotalTokens {
		return false, errors.New("Seeduplex session usage exceeds the supported range")
	}
	if id != "" {
		if len(t.seen) >= 10000 {
			return false, errors.New("Seeduplex session has too many usage events; reconnect")
		}
		if t.seen == nil {
			t.seen = make(map[string][6]int)
		}
		t.seen[id] = counts
	}
	if t.total.InputTokenDetails.CachedTokensDetails == nil {
		t.total.InputTokenDetails.CachedTokensDetails = &dto.CachedTokenDetails{TextTokens: new(int), AudioTokens: new(int)}
	}
	t.total.TotalTokens += usage.TotalTokens
	t.total.InputTokens += usage.InputTokens
	t.total.OutputTokens += usage.OutputTokens
	t.total.InputTokenDetails.TextTokens += counts[0]
	t.total.InputTokenDetails.AudioTokens += counts[1]
	t.total.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	*t.total.InputTokenDetails.CachedTokensDetails.TextTokens += counts[2]
	*t.total.InputTokenDetails.CachedTokensDetails.AudioTokens += counts[3]
	t.total.OutputTokenDetails.TextTokens += counts[4]
	t.total.OutputTokenDetails.AudioTokens += counts[5]
	return true, nil
}

type seeduplexRelay struct {
	c          *gin.Context
	info       *relaycommon.RelayInfo
	tracker    *service.RealtimeAlertTracker
	usage      seeduplexUsageTracker // Owned by the upstream reader until it exits.
	targetMu   sync.Mutex
	logMu      sync.Mutex
	auditBytes int
	clientGone atomic.Bool
	closeSent  atomic.Bool
}

func (s *seeduplexRelay) writeUpstream(message []byte) error {
	s.targetMu.Lock()
	defer s.targetMu.Unlock()
	if err := s.info.TargetWs.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return s.info.TargetWs.WriteMessage(websocket.TextMessage, message)
}

// Session configuration can contain search API keys. Audit event metadata and
// bounded usage evidence, never audio payloads or arbitrary extension fields.
func (s *seeduplexRelay) audit(client bool, event seeduplexEvent, message []byte) {
	summary := fmt.Sprintf("%s (%d bytes)", event.Type, len(message))
	if !client && event.Type == "response.done" {
		data := event.Usage
		if len(data) == 0 && event.Response != nil {
			data = event.Response.Usage
		}
		if usage, err := parseSeeduplexUsage(data); err == nil {
			if encoded, err := common.Marshal(usage); err == nil {
				summary += ": " + string(encoded)
			}
		}
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if len(summary) > seeduplexAuditLimit-s.auditBytes {
		return
	}
	s.auditBytes += len(summary)
	if client {
		s.info.WsRequestMessages = append(s.info.WsRequestMessages, summary)
	} else {
		s.info.WsResponseMessages = append(s.info.WsResponseMessages, summary)
	}
}

func (s *seeduplexRelay) readClient() error {
	created := false
	for {
		kind, message, err := s.info.ClientWs.ReadMessage()
		if err != nil {
			s.clientGone.Store(true)
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return errors.New("Seeduplex client connection closed before session.close")
		}
		var event seeduplexEvent
		if kind != websocket.TextMessage || common.Unmarshal(message, &event) != nil || event.Type == "" {
			return errors.New("Seeduplex requires JSON text events with a type")
		}
		if event.Type == "session.create" || event.Type == "session.update" {
			if event.Type == "session.create" && created || event.Type == "session.update" && !created {
				return errors.New("Seeduplex requires one session.create before session.update")
			}
			if gjson.GetBytes(message, "session.model").String() != seeduplexVersion {
				return errors.New("Seeduplex session.model must be 1.2.6.1")
			}
			created = true
		} else if !created && event.Type != "session.close" {
			return errors.New("Seeduplex requires session.create before input events")
		}
		inference := event.Type
		if inference == "speech_text_buffer.commit" || inference == "speech_text_buffer.replacement.commit" || inference == "conversation.item.create" && gjson.GetBytes(message, `items.#(role=="tool")`).Exists() {
			inference = "response.create"
		}
		if err := s.tracker.Admit(s.c.Request.Context(), inference); err != nil {
			return err
		}
		s.audit(true, event, message)
		if event.Type == "session.close" {
			s.closeSent.Store(true)
		}
		if err := s.writeUpstream(message); err != nil {
			return errors.New("failed to send Seeduplex input")
		}
		if event.Type == "session.close" {
			return nil
		}
	}
}

func (s *seeduplexRelay) readUpstream() error {
	for {
		kind, message, err := s.info.TargetWs.ReadMessage()
		if err != nil {
			return errors.New("Seeduplex upstream closed before session.closed")
		}
		var event seeduplexEvent
		if kind != websocket.TextMessage || common.Unmarshal(message, &event) != nil || event.Type == "" {
			return errors.New("invalid Seeduplex upstream event")
		}
		s.audit(false, event, message)
		if event.Type == "conversation.item.input_audio_transcription.completed" {
			s.tracker.NativeResponseStart(s.c.Request.Context())
		}
		status := ""
		if event.Response != nil {
			status = event.Response.Status
		}
		hasOutput := gjson.GetBytes(message, "delta").String() != ""
		s.tracker.Observe(s.c.Request.Context(), event.Type, status, hasOutput)
		if hasOutput && (event.Type == "response.output_text.delta" || event.Type == "response.output_audio.delta") {
			s.info.SetFirstResponseTime()
		}
		changed, usageErr := s.usage.consume(event)
		if changed {
			usageErr = service.ReserveWssUsage(s.c, s.info, &s.usage.total)
		}
		if !s.clientGone.Load() {
			_ = s.info.ClientWs.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := helper.WssResponseString(s.c, s.info, s.info.ClientWs, string(message)); err != nil {
				s.clientGone.Store(true)
				_ = s.info.ClientWs.Close() // Unblock the client reader; continue collecting tail usage.
			}
		}
		if usageErr != nil {
			return usageErr
		}
		switch event.Type {
		case "session.closed":
			return nil
		case "error":
			s.tracker.Observe(s.c.Request.Context(), "response.done", "failed", false)
			return errSeeduplexProvider
		case "response.canceled":
			s.tracker.Observe(s.c.Request.Context(), "response.done", "cancelled", false)
		}
	}
}

func handleSeeduplex(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(errors.New("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}
	info.IsStream = true
	info.ClientWs.SetReadLimit(seeduplexFrameLimit)
	info.TargetWs.SetReadLimit(seeduplexFrameLimit)
	session := &seeduplexRelay{c: c, info: info, tracker: service.NewRealtimeAlertTracker(common.AlertHealthEvent{
		RequestID: c.GetString(common.RequestIdKey), UserID: info.UserId, TokenID: info.TokenId,
		ChannelID: info.ChannelId, ChannelName: c.GetString("channel_name"), Model: info.OriginModelName,
	})}
	defer session.tracker.Close(c.Request.Context())
	type relayResult struct {
		client bool
		err    error
	}
	results := make(chan relayResult, 2)
	var readers sync.WaitGroup
	for _, client := range []bool{true, false} {
		readers.Go(func() {
			result := relayResult{client: client}
			defer func() {
				if recover() != nil {
					result.err = errors.New("Seeduplex relay reader failed")
				}
				results <- result
			}()
			if client {
				result.err = session.readClient()
			} else {
				result.err = session.readUpstream()
			}
		})
	}
	var relayErr error
	upstreamDone := false
	select {
	case result := <-results:
		relayErr, upstreamDone = result.err, !result.client
		info.RealtimeUsageIncomplete = !result.client && result.err != nil
	case <-c.Request.Context().Done():
		session.clientGone.Store(true)
		_ = info.ClientWs.Close()
	}
	if !upstreamDone {
		if !session.closeSent.Swap(true) {
			_ = session.writeUpstream([]byte(`{"type":"session.close"}`))
		}
		// Close is safe concurrently with ReadMessage. A read deadline would
		// race with the reader's gorilla/websocket read state.
		timer := time.AfterFunc(2*time.Second, func() { _ = info.TargetWs.Close() })
		for !upstreamDone {
			result := <-results
			upstreamDone = !result.client
			if !result.client && result.err != nil {
				info.RealtimeUsageIncomplete = true
			}
			if relayErr == nil {
				relayErr = result.err
			}
		}
		timer.Stop()
	}
	// The upstream reader has exited; this is the only remaining client writer.
	if relayErr != nil {
		logger.LogError(c, common.LogText("seeduplex realtime: %s", relayErr.Error()))
		if !session.clientGone.Load() && !errors.Is(relayErr, errSeeduplexProvider) {
			_ = info.ClientWs.SetWriteDeadline(time.Now().Add(5 * time.Second))
			helper.WssError(c, info.ClientWs, types.NewErrorWithStatusCode(relayErr, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry()).ToOpenAIError())
		}
	}
	_ = info.TargetWs.Close()
	_ = info.ClientWs.Close()
	readers.Wait()
	// Runtime errors must still reach WssHelper's final settlement with all
	// confirmed usage; returning an error here would bypass it and refund it.
	return nil, &session.usage.total
}
