package common

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Alert collection is independent of request/response logging and debug traces.
// The host installs the sink at startup; common never imports model or service.
var AlertingEnabled bool
var AlertingActionsEnabled bool
var AlertingControlsEnabled bool
var AlertingShadowMode bool
var AlertingBootID = GetUUID()

type AlertHealthEvent struct {
	SourceID, RequestID, Layer, Kind, Model, Protocol            string
	ChannelID, UserID, TokenID                                   int
	ChannelName, Outcome                                         string
	ConfigVersion                                                string
	SampleClass                                                  string
	Sequence, StartedAt, OccurredAt, FirstOutputAt, LastOutputAt int64
	DurationMS, TTFTMS                                           *int64
	Stream, OutputCovered, AttributionCovered                    bool
}

type AlertHealthSink func(context.Context, AlertHealthEvent) error

var alertHealthSink atomic.Pointer[AlertHealthSink]

var alertPublications = struct {
	sync.Mutex
	spans    map[*AlertSpan]struct{}
	overflow int
}{spans: make(map[*AlertSpan]struct{})}

// AlertPublicationWatermark includes buffered progress and publications still
// awaiting durable acceptance. A producer must sample its offered sequence only
// after this snapshot; otherwise its heartbeat could overtake real output.
func AlertPublicationWatermark(now time.Time) (int64, bool) {
	alertPublications.Lock()
	spans := make([]*AlertSpan, 0, len(alertPublications.spans))
	for span := range alertPublications.spans {
		spans = append(spans, span)
	}
	covered := alertPublications.overflow == 0
	alertPublications.Unlock()
	cutoff := now.Add(-time.Second).UnixMilli()
	for _, span := range spans {
		span.mu.Lock()
		if span.pendingSince > 0 {
			cutoff = min(cutoff, span.pendingSince-1)
		}
		span.mu.Unlock()
	}
	return cutoff, covered
}

func SetAlertHealthSink(sink AlertHealthSink) { alertHealthSink.Store(&sink) }

func EmitAlertHealth(ctx context.Context, event AlertHealthEvent) {
	if !AlertingEnabled {
		return
	}
	sink := alertHealthSink.Load()
	if sink == nil || *sink == nil {
		return
	}
	if err := (*sink)(ctx, event); err != nil {
		SysError(LogText("alert health observation could not be accepted"))
	}
}

type alertSpanKey struct{}
type alertProbeKey struct{}

func WithAlertRecoveryProbe(ctx context.Context) context.Context {
	return context.WithValue(ctx, alertProbeKey{}, true)
}
func IsAlertRecoveryProbe(ctx context.Context) bool {
	value, _ := ctx.Value(alertProbeKey{}).(bool)
	return value
}

func (span *AlertSpan) Snapshot() AlertHealthEvent {
	if span == nil {
		return AlertHealthEvent{}
	}
	span.mu.Lock()
	defer span.mu.Unlock()
	return span.event
}

type AlertSpan struct {
	mu                sync.Mutex
	event             AlertHealthEvent
	started           time.Time
	lastPublished     time.Time
	ended             bool
	parent            *AlertSpan
	contributes       bool
	flushTimer        *time.Timer
	pendingSince      int64
	publishedSequence int64
	tracked           bool
	registered        bool
}

func (span *AlertSpan) publish(ctx context.Context, event AlertHealthEvent) {
	EmitAlertHealth(ctx, event)
	span.mu.Lock()
	span.publishedSequence = max(span.publishedSequence, event.Sequence)
	if span.publishedSequence >= span.event.Sequence && span.flushTimer == nil {
		span.pendingSince = 0
	}
	if span.registered && span.ended && span.pendingSince == 0 {
		alertPublications.Lock()
		if span.tracked {
			delete(alertPublications.spans, span)
		} else {
			alertPublications.overflow--
		}
		alertPublications.Unlock()
		// Only the publication that first completes the terminal sequence may
		// unregister; delayed older publications can still return afterwards.
		span.registered = false
	}
	span.mu.Unlock()
}

func BeginAlertSpan(ctx context.Context, event AlertHealthEvent) (context.Context, *AlertSpan) {
	if !AlertingEnabled {
		return ctx, nil
	}
	alertPublications.Lock()
	now := time.Now()
	event.SourceID = GetUUID()
	event.AttributionCovered = event.UserID > 0
	event.Kind, event.Sequence, event.StartedAt, event.OccurredAt = "started", 1, now.UnixMilli(), now.UnixMilli()
	span := &AlertSpan{event: event, started: now, lastPublished: now, contributes: true, pendingSince: now.UnixMilli(), registered: true}
	if len(alertPublications.spans) < 10000 {
		span.tracked = true
		alertPublications.spans[span] = struct{}{}
	} else {
		alertPublications.overflow++
	}
	alertPublications.Unlock()
	if event.Layer == "channel_attempt" {
		span.parent = AlertSpanFromContext(ctx)
	}
	span.publish(ctx, event)
	return context.WithValue(ctx, alertSpanKey{}, span), span
}

func (span *AlertSpan) Bind(event AlertHealthEvent) {
	if span == nil {
		return
	}
	span.mu.Lock()
	if span.ended {
		span.mu.Unlock()
		return
	}
	span.event.UserID, span.event.TokenID = event.UserID, event.TokenID
	span.event.Model, span.event.Protocol, span.event.Stream = event.Model, event.Protocol, event.Stream
	span.event.ChannelID, span.event.ChannelName = event.ChannelID, event.ChannelName
	span.event.OutputCovered = span.event.OutputCovered || event.OutputCovered
	span.event.AttributionCovered = event.UserID > 0
	span.event.ConfigVersion = event.ConfigVersion
	if event.SampleClass != "" {
		span.event.SampleClass = event.SampleClass
	}
	span.event.Sequence++
	span.event.Kind = "metadata"
	span.event.OccurredAt = span.event.StartedAt + time.Since(span.started).Milliseconds()
	if span.pendingSince == 0 {
		span.pendingSince = span.event.OccurredAt
	}
	updated := span.event
	span.mu.Unlock()
	span.publish(context.Background(), updated)
}

func (span *AlertSpan) SetRaceParticipant() {
	if span != nil {
		span.mu.Lock()
		span.contributes = false
		span.mu.Unlock()
	}
}

// ConfirmRaceWinner starts request-level progress only after the race has
// committed a winner. Loser output must never hide a stalled winning response.
func (span *AlertSpan) ConfirmRaceWinner(ctx context.Context) {
	if span == nil {
		return
	}
	span.mu.Lock()
	span.contributes = true
	parent, hasOutput := span.parent, span.event.FirstOutputAt != 0
	span.mu.Unlock()
	if parent != nil && hasOutput {
		ObserveAlertOutput(context.WithValue(ctx, alertSpanKey{}, parent))
	}
}

func AlertSpanFromContext(ctx context.Context) *AlertSpan {
	if ctx == nil {
		return nil
	}
	span, _ := ctx.Value(alertSpanKey{}).(*AlertSpan)
	return span
}

// ObserveAlertOutput is called only after protocol decoding confirms real model
// output. A response header or SSE heartbeat never calls this method.
func ObserveAlertOutput(ctx context.Context) {
	span := AlertSpanFromContext(ctx)
	if span == nil {
		return
	}
	span.mu.Lock()
	if span.ended {
		span.mu.Unlock()
		return
	}
	now := time.Now()
	first := span.event.FirstOutputAt == 0
	if first {
		span.event.FirstOutputAt = span.event.StartedAt + time.Since(span.started).Milliseconds()
	}
	span.event.LastOutputAt = span.event.StartedAt + time.Since(span.started).Milliseconds()
	if span.pendingSince == 0 {
		span.pendingSince = span.event.LastOutputAt
	}
	parent, contributes := span.parent, span.contributes
	if !first && now.Sub(span.lastPublished) < time.Second {
		// Flush the latest real output even if the next chunk never arrives.
		// Otherwise a stream that stalls just after a chunk reports an old idle
		// deadline despite healthy producer heartbeats.
		if span.flushTimer == nil {
			span.flushTimer = time.AfterFunc(time.Second-now.Sub(span.lastPublished), span.flushProgress)
		}
		span.mu.Unlock()
		if parent != nil && contributes {
			ObserveAlertOutput(context.WithValue(ctx, alertSpanKey{}, parent))
		}
		return
	}
	span.event.Sequence++
	span.event.Kind = "progress"
	if first {
		span.event.Kind = "first_output"
	}
	span.event.OccurredAt = span.event.LastOutputAt
	span.lastPublished = now
	event := span.event
	span.mu.Unlock()
	span.publish(ctx, event)
	if parent != nil && contributes {
		ObserveAlertOutput(context.WithValue(ctx, alertSpanKey{}, parent))
	}
}

func (span *AlertSpan) flushProgress() {
	span.mu.Lock()
	span.flushTimer = nil
	if span.ended {
		span.mu.Unlock()
		return
	}
	span.event.Sequence++
	span.event.Kind, span.event.OccurredAt = "progress", span.event.LastOutputAt
	span.lastPublished = time.Now()
	event := span.event
	span.mu.Unlock()
	span.publish(context.Background(), event)
}

func (span *AlertSpan) End(ctx context.Context, outcome string) {
	if span == nil {
		return
	}
	span.mu.Lock()
	if span.ended {
		span.mu.Unlock()
		return
	}
	span.ended = true
	if span.flushTimer != nil {
		span.flushTimer.Stop()
		span.flushTimer = nil
	}
	duration := max(time.Since(span.started).Milliseconds(), 0)
	span.event.Sequence++
	span.event.Kind, span.event.Outcome = "finished", outcome
	span.event.OccurredAt = span.event.StartedAt + duration
	if span.pendingSince == 0 {
		span.pendingSince = span.event.OccurredAt
	}
	span.event.DurationMS = &duration
	if span.event.FirstOutputAt > 0 {
		ttft := span.event.FirstOutputAt - span.event.StartedAt
		span.event.TTFTMS = &ttft
	}
	event := span.event
	span.mu.Unlock()
	span.publish(ctx, event)
}

// MarkAlertCoverageUnverified preserves evidence for a protocol operation that
// cannot be correlated safely. It must not contribute a misleading healthy rate.
func (span *AlertSpan) MarkAlertCoverageUnverified(ctx context.Context) {
	if span == nil {
		return
	}
	span.mu.Lock()
	if span.ended {
		span.mu.Unlock()
		return
	}
	span.event.SampleClass, span.event.OutputCovered = "unsupported", false
	span.event.Sequence++
	span.event.Kind = "metadata"
	span.event.OccurredAt = span.event.StartedAt + time.Since(span.started).Milliseconds()
	if span.pendingSince == 0 {
		span.pendingSince = span.event.OccurredAt
	}
	event := span.event
	span.mu.Unlock()
	span.publish(ctx, event)
}
