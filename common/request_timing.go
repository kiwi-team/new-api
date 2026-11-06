package common

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// RequestTimingEnabled is configured at startup. Disabled requests allocate no collector.
var RequestTimingEnabled bool

const timingLimit = 32

// Version 2 reserves request_parse for business validation; decoding has its own phases.
const requestTimingSchemaVersion = 2

type timingContextKey struct{}
type timingAttemptKey struct{}

// RequestTiming owns bounded metadata only: it never retains a body or Gin context.
type RequestTiming struct {
	mu            sync.Mutex
	started       time.Time
	requestID     string
	offsets       map[string]int64
	metrics       map[string]int64
	bodyComplete  *bool
	coverage      map[string]string
	attempts      []*TimingAttempt
	calls         []*TimingCall
	spans         []map[string]any
	spanCount     int
	callCount     int
	attemptCount  int
	finished      bool
	emit          func(map[string]any)
	traceRoot     string
	operationID   string
	operationName string
	taskID        string
	autoFinish    bool
}

type TimingAttempt struct {
	owner    *RequestTiming
	id       string
	channel  int
	offsets  map[string]int64
	callIDs  []string
	ended    bool
	retained bool
	metadata map[string]string
}

type TimingCall struct {
	owner       *RequestTiming
	id          string
	attempt     *TimingAttempt
	purpose     string
	primary     bool
	kind        string
	offsets     map[string]int64
	metrics     map[string]int64
	state       map[string]any
	retained    bool
	ended       bool
	exchanges   []map[string]any
	writeEvents []map[string]any
}

type TimingPhase struct {
	owner   *RequestTiming
	name    string
	started int64
	first   bool
	ended   bool
	span    map[string]any
}

// RequestTimingLogEmitter gates service-log output independently of collection.
// A nil emitter avoids building log-only snapshots when DEBUG is off.
func RequestTimingLogEmitter(write func(string)) func(map[string]any) {
	if !DebugEnabled {
		return nil
	}
	return func(snapshot map[string]any) {
		if !DebugEnabled {
			return
		}
		if data, err := Marshal(snapshot); err == nil {
			write(string(data))
		}
	}
}

func NewRequestTiming(ctx context.Context, requestID, traceRoot string, emit func(map[string]any)) (context.Context, *RequestTiming) {
	t := &RequestTiming{started: time.Now(), requestID: requestID, traceRoot: traceRoot, emit: emit,
		offsets: map[string]int64{"gateway_entry_at_ms": 0}, metrics: make(map[string]int64),
		coverage: map[string]string{"ingress": "full", "preprocessing": "partial", "upstream": "unsupported", "protocol": "unsupported", "downstream": "full"}}
	return context.WithValue(ctx, timingContextKey{}, t), t
}

// BackgroundTimingContext creates an independent boundary; it never revives a finished HTTP request.
func BackgroundTimingContext(operation, taskID string) context.Context {
	ctx := context.Background()
	if !RequestTimingEnabled {
		return ctx
	}
	ctx, t := NewRequestTiming(ctx, "", "", RequestTimingLogEmitter(SysLog))
	t.operationID, t.operationName, t.taskID = GetUUID(), operation, taskID
	t.coverage["ingress"], t.coverage["downstream"] = "not_applicable", "not_applicable"
	t.autoFinish = true
	return ctx
}

// TimingScope is embedded in task adaptors so read-only polls need not retain a Gin context.
type TimingScope struct{ timingContext context.Context }

func (s *TimingScope) SetTimingContext(ctx context.Context) { s.timingContext = ctx }
func (s *TimingScope) RequestTimingContext() context.Context {
	if s == nil || s.timingContext == nil {
		return context.Background()
	}
	return s.timingContext
}
func AttachTimingScope(adaptor any, ctx context.Context) {
	if TimingFromContext(ctx) == nil {
		return
	}
	if target, ok := adaptor.(interface{ SetTimingContext(context.Context) }); ok {
		target.SetTimingContext(ctx)
	}
}

func BeginTimingOperationScope(ctx context.Context, operation, taskID string) (context.Context, *RequestTiming) {
	if !RequestTimingEnabled || TimingFromContext(ctx) != nil {
		return ctx, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, t := NewRequestTiming(ctx, "", "", RequestTimingLogEmitter(SysLog))
	t.operationID, t.operationName, t.taskID = GetUUID(), operation, taskID
	t.coverage["ingress"], t.coverage["downstream"] = "not_applicable", "not_applicable"
	return ctx, t
}
func (t *RequestTiming) EndOperation() {
	if t == nil {
		return
	}
	t.Finish()
	if t.emit != nil {
		t.emit(t.Snapshot("operation_end"))
	}
}

// OutboundTimingContext distinguishes a continuing attempt from a new background poll.
func OutboundTimingContext(source context.Context, operation string) context.Context {
	if source == nil {
		source = context.Background()
	}
	if t := TimingFromContext(source); t != nil {
		t.mu.Lock()
		active := !t.finished
		if a, ok := source.Value(timingAttemptKey{}).(*TimingAttempt); ok {
			active = active || !a.ended
		}
		t.mu.Unlock()
		if active {
			return source
		}
	}
	if !RequestTimingEnabled {
		return source
	}
	return BackgroundTimingContext(operation, "")
}

func TimingFromContext(ctx context.Context) *RequestTiming {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(timingContextKey{}).(*RequestTiming)
	return t
}

// WithTimingContext copies diagnostic values without changing cancellation/deadlines.
func WithTimingContext(target, source context.Context) context.Context {
	if t := TimingFromContext(source); t != nil {
		target = context.WithValue(target, timingContextKey{}, t)
		if attempt, ok := source.Value(timingAttemptKey{}).(*TimingAttempt); ok {
			target = context.WithValue(target, timingAttemptKey{}, attempt)
		}
	}
	return target
}

func (t *RequestTiming) offset() int64 { return time.Since(t.started).Milliseconds() }

func MarkTiming(ctx context.Context, name string) {
	if t := TimingFromContext(ctx); t != nil {
		t.mu.Lock()
		if _, exists := t.offsets[name]; !exists {
			t.offsets[name] = t.offset()
		}
		t.mu.Unlock()
	}
}

func SetTimingRequestID(ctx context.Context, requestID string) {
	if t := TimingFromContext(ctx); t != nil {
		t.mu.Lock()
		t.requestID = requestID
		t.mu.Unlock()
	}
}

func TimingMetric(ctx context.Context, name string, value int64) {
	if t := TimingFromContext(ctx); t != nil {
		t.mu.Lock()
		t.metrics[name] = value
		t.mu.Unlock()
	}
}

func StartTimingPhase(ctx context.Context, name string) *TimingPhase {
	t := TimingFromContext(ctx)
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, exists := t.offsets[name+"_started_at_ms"]
	p := &TimingPhase{owner: t, name: name, started: t.offset(), first: !exists}
	if p.first {
		t.offsets[name+"_started_at_ms"] = p.started
	}
	t.spanCount++
	if len(t.spans) < timingLimit {
		p.span = map[string]any{"span_id": fmt.Sprintf("span-%d", t.spanCount), "phase": name,
			"started_at_ms": p.started, "ended_at_ms": nil, "duration_ms": nil, "status": "in_progress"}
		t.spans = append(t.spans, p.span)
	}
	return p
}

func (p *TimingPhase) End() {
	if p == nil {
		return
	}
	t := p.owner
	t.mu.Lock()
	defer t.mu.Unlock()
	if p.ended {
		return
	}
	p.ended = true
	ended := t.offset()
	if p.first {
		t.offsets[p.name+"_ended_at_ms"] = ended
	}
	if p.span != nil {
		p.span["ended_at_ms"], p.span["duration_ms"], p.span["status"] = ended, ended-p.started, "ended"
	}
}

func BeginTimingAttempt(ctx context.Context, channelID int) (context.Context, *TimingAttempt) {
	t := TimingFromContext(ctx)
	if t == nil {
		return ctx, nil
	}
	t.mu.Lock()
	t.attemptCount++
	a := &TimingAttempt{owner: t, id: fmt.Sprintf("attempt-%d", t.attemptCount), channel: channelID,
		offsets: map[string]int64{"attempt_started_at_ms": t.offset()}, metadata: make(map[string]string)}
	if len(t.attempts) < 16 {
		a.retained = true
		t.attempts = append(t.attempts, a)
	}
	if t.coverage["upstream"] == "not_applicable" {
		t.coverage["upstream"] = "unsupported"
	}
	t.mu.Unlock()
	return context.WithValue(ctx, timingAttemptKey{}, a), a
}

func AnnotateTimingAttempt(ctx context.Context, upstreamRequestID, raceResult string) {
	if ctx == nil {
		return
	}
	a, _ := ctx.Value(timingAttemptKey{}).(*TimingAttempt)
	if a == nil {
		return
	}
	a.owner.mu.Lock()
	// These are existing bounded correlation fields, never response content.
	a.metadata["upstream_request_id"] = upstreamRequestID[:min(len(upstreamRequestID), 256)]
	a.metadata["race_result"] = raceResult
	a.owner.mu.Unlock()
}

func (a *TimingAttempt) End() {
	if a == nil {
		return
	}
	t := a.owner
	t.mu.Lock()
	if a.ended {
		t.mu.Unlock()
		return
	}
	a.ended = true
	a.offsets["attempt_ended_at_ms"] = t.offset()
	late := t.finished || !a.retained
	var snapshot map[string]any
	if late && t.emit != nil {
		snapshot = a.snapshotLocked()
	}
	requestID := t.requestID
	t.mu.Unlock()
	if late && t.emit != nil {
		t.emit(map[string]any{"event": "request_timing", "scope": "attempt", "phase": "attempt_end", "schema_version": requestTimingSchemaVersion,
			"request_id": requestID, "attempt": snapshot})
	}
}

func ObserveTimingProtocol(ctx context.Context, event string, output bool, terminal bool, terminalKind ...string) {
	if output {
		ObserveAlertOutput(ctx)
	}
	t := TimingFromContext(ctx)
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, _ := ctx.Value(timingAttemptKey{}).(*TimingAttempt)
	if a == nil {
		t.coverage["protocol"] = "partial"
		return
	}
	t.coverage["protocol"] = "partial"
	names := []string{event}
	if event != "first_sse_data_at_ms" {
		names = append(names, "first_protocol_event_at_ms")
	}
	for _, name := range names {
		if name != "" {
			if _, ok := a.offsets[name]; !ok {
				a.offsets[name] = t.offset()
			}
		}
	}
	if output {
		if _, ok := a.offsets["first_output_at_ms"]; !ok {
			a.offsets["first_output_at_ms"] = t.offset()
			switch event {
			case "first_text_at_ms":
				a.metadata["first_output_kind"] = "text"
			case "first_reasoning_at_ms":
				a.metadata["first_output_kind"] = "reasoning"
			case "first_tool_output_at_ms":
				a.metadata["first_output_kind"] = "tool"
			}
		}
	}
	if terminal {
		a.offsets["protocol_terminal_at_ms"] = t.offset()
		if len(terminalKind) > 0 {
			a.metadata["protocol_terminal_kind"] = terminalKind[0][:min(len(terminalKind[0]), 64)]
		}
	}
}

func TimingSnapshot(ctx context.Context, phase string) map[string]any {
	if t := TimingFromContext(ctx); t != nil {
		return t.Snapshot(phase, ctx)
	}
	return nil
}

func (a *TimingAttempt) snapshotLocked() map[string]any {
	m := map[string]any{"attempt_id": a.id, "channel_id": a.channel, "outbound_call_ids": slices.Clone(a.callIDs)}
	for key, value := range a.metadata {
		m[key] = value
	}
	m["status"] = "ended"
	if !a.ended {
		m["status"] = "pending_at_finalize"
	}
	for _, name := range []string{"attempt_started_at_ms", "attempt_ended_at_ms", "upstream_call_started_at_ms", "first_protocol_event_at_ms", "first_sse_data_at_ms", "first_output_at_ms", "protocol_terminal_at_ms", "response_handling_ended_at_ms", "response_parsed_at_ms", "first_text_at_ms", "first_reasoning_at_ms", "first_tool_output_at_ms"} {
		m[name] = timingValue(a.offsets, name)
	}
	return m
}

func timingValue(values map[string]int64, name string) any {
	if value, ok := values[name]; ok {
		return value
	}
	return nil
}

func timingDuration(values map[string]int64, end, start string) any {
	e, endOK := values[end]
	s, startOK := values[start]
	if !endOK || !startOK || e < s {
		return nil
	}
	return e - s
}

func (t *RequestTiming) Snapshot(phase string, source ...context.Context) map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	values := maps.Clone(t.offsets)
	if len(source) > 0 && source[0] != nil {
		if a, ok := source[0].Value(timingAttemptKey{}).(*TimingAttempt); ok {
			delete(values, "response_handling_ended_at_ms")
			if at, exists := a.offsets["response_handling_ended_at_ms"]; exists {
				values["response_handling_ended_at_ms"] = at
			}
		}
	}
	offsets := make(map[string]any)
	for _, name := range []string{"gateway_entry_at_ms", "auth_started_at_ms", "auth_ended_at_ms", "body_read_started_at_ms", "body_first_byte_at_ms", "body_read_ended_at_ms", "body_storage_ready_at_ms", "model_extract_started_at_ms", "model_extract_ended_at_ms", "route_started_at_ms", "route_ended_at_ms", "route_ready_at_ms", "request_parse_started_at_ms", "request_parse_ended_at_ms", "first_outbound_call_started_at_ms", "first_upstream_call_started_at_ms", "response_handling_ended_at_ms", "handler_ended_at_ms", "context_done_observed_at_ms", "host_first_output_at_ms"} {
		offsets[name] = timingValue(values, name)
	}
	r := map[string]any{"offsets_ms": offsets, "body_complete": t.bodyComplete, "body_state": "not_read"}
	if _, ok := values["body_read_started_at_ms"]; ok {
		r["body_state"] = "in_progress"
	}
	if t.bodyComplete != nil {
		r["body_state"] = "interrupted"
		if *t.bodyComplete {
			r["body_state"] = "complete"
		}
	}
	if t.metrics["has_body"] == 0 {
		r["body_state"] = "no_body"
	}
	r["cancel_source"], r["context_error"] = nil, nil
	if len(source) > 0 && source[0] != nil && source[0].Err() != nil {
		r["context_error"] = source[0].Err().Error()
		r["cancel_source"] = "unknown"
	}
	for k, v := range t.metrics {
		if !strings.HasPrefix(k, "downstream_") && k != "has_body" && k != "flush_error_observable" {
			r[k] = v
		}
	}
	for _, name := range []string{"auth", "model_extract", "request_parse"} {
		r[name+"_ms"] = timingDuration(values, name+"_ended_at_ms", name+"_started_at_ms")
	}
	r["route_ms"] = timingDuration(values, "route_ended_at_ms", "route_started_at_ms")
	r["before_body_read_ms"] = timingDuration(values, "body_read_started_at_ms", "gateway_entry_at_ms")
	r["body_read_ms"] = timingDuration(values, "body_read_ended_at_ms", "body_read_started_at_ms")
	r["pre_upstream_ms"] = timingDuration(values, "first_upstream_call_started_at_ms", "gateway_entry_at_ms")
	r["after_body_ready_to_upstream_ms"] = timingDuration(values, "first_upstream_call_started_at_ms", "body_storage_ready_at_ms")
	r["post_response_ms"] = timingDuration(values, "handler_ended_at_ms", "response_handling_ended_at_ms")
	r["gateway_total_ms"] = timingDuration(values, "handler_ended_at_ms", "gateway_entry_at_ms")
	down := map[string]any{"body_bytes": t.metrics["downstream_bytes"], "client_received_complete": nil}
	for _, key := range []string{"first_write_at_ms", "last_write_at_ms", "last_flush_at_ms"} {
		down[key] = timingValue(values, key)
	}
	for _, op := range []string{"write", "flush"} {
		down[op+"_call_count"] = t.metrics["downstream_"+op+"_calls"]
		down[op+"_error_count"] = t.metrics["downstream_"+op+"_errors"]
		down[op+"_call_sum_ms"], down[op+"_call_max_ms"] = nil, nil
		if t.metrics["downstream_"+op+"_calls"] > 0 {
			down[op+"_call_sum_ms"] = float64(t.metrics["downstream_"+op+"_sum_ns"]) / 1e6
			down[op+"_call_max_ms"] = float64(t.metrics["downstream_"+op+"_max_ns"]) / 1e6
		}
	}
	down["flush_error_observable"] = t.metrics["flush_error_observable"] > 0
	down["write_span_ms"] = timingDuration(values, "last_write_at_ms", "first_write_at_ms")
	attempts := make([]map[string]any, 0, len(t.attempts))
	for _, a := range t.attempts {
		attempts = append(attempts, a.snapshotLocked())
	}
	calls := make([]map[string]any, 0, len(t.calls))
	for _, call := range t.calls {
		calls = append(calls, call.snapshotLocked())
	}
	spans := make([]map[string]any, 0, len(t.spans))
	for _, span := range t.spans {
		copy := maps.Clone(span)
		if copy["ended_at_ms"] == nil {
			copy["status"] = "pending_at_finalize"
		}
		spans = append(spans, copy)
	}
	result := map[string]any{"event": "request_timing", "schema_version": requestTimingSchemaVersion, "scope": "request", "phase": phase,
		"request_id": t.requestID, "started_at": t.started.UTC().Format(time.RFC3339Nano), "snapshot_at_ms": t.offset(),
		"coverage": maps.Clone(t.coverage), "request": r, "downstream": down, "attempts": attempts, "outbound_calls": calls,
		"business_spans": spans, "attempt_count": t.attemptCount, "outbound_call_count": t.callCount, "business_span_count": t.spanCount,
		"truncated": t.attemptCount > len(attempts) || t.callCount > len(calls) || t.spanCount > len(spans)}
	if t.traceRoot != "" {
		result["alb_trace_root"] = t.traceRoot
	}
	if t.operationID != "" {
		delete(result, "request_id")
		result["scope"], result["operation_id"], result["operation"] = "background_operation", t.operationID, t.operationName
		if t.taskID != "" {
			result["task_id"] = t.taskID
		}
	}
	missing := make(map[string]string)
	for name, value := range offsets {
		if value == nil {
			reason := "not_observed"
			if name == "handler_ended_at_ms" && !t.finished {
				reason = "pending_at_finalize"
			}
			missing["request."+name] = reason
		}
	}
	for _, a := range attempts {
		for _, key := range []string{"first_output_at_ms", "protocol_terminal_at_ms"} {
			if a[key] == nil {
				missing[a["attempt_id"].(string)+"."+key] = "not_observed"
			}
		}
	}
	missing["downstream.client_received_complete"] = "not_observable"
	result["missing_reasons"] = missing
	return result
}

func (t *RequestTiming) Finish() {
	t.mu.Lock()
	if !t.finished {
		t.finished = true
		t.offsets["handler_ended_at_ms"] = t.offset()
	}
	t.mu.Unlock()
}

func (t *RequestTiming) ObserveBody(started time.Time, n int, complete bool, ended bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.offsets["body_read_started_at_ms"]; !ok {
		t.offsets["body_read_started_at_ms"] = started.Sub(t.started).Milliseconds()
	}
	if n > 0 {
		if _, ok := t.offsets["body_first_byte_at_ms"]; !ok {
			t.offsets["body_first_byte_at_ms"] = t.offset()
		}
		t.metrics["body_bytes"] += int64(n)
	}
	if ended {
		if _, ok := t.offsets["body_read_ended_at_ms"]; !ok {
			t.offsets["body_read_ended_at_ms"] = t.offset()
		}
		value := complete || t.bodyComplete != nil && *t.bodyComplete
		t.bodyComplete = &value
	}
}

func (t *RequestTiming) ObserveDownstream(op string, elapsed time.Duration, n int, succeeded, errorObservable bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metrics["downstream_"+op+"_sum_ns"] += elapsed.Nanoseconds()
	t.metrics["downstream_"+op+"_calls"]++
	if op == "flush" && errorObservable {
		t.metrics["flush_error_observable"] = 1
	}
	if !succeeded {
		t.metrics["downstream_"+op+"_errors"]++
	}
	t.metrics["downstream_"+op+"_max_ns"] = max(t.metrics["downstream_"+op+"_max_ns"], elapsed.Nanoseconds())
	if op == "write" && n > 0 {
		t.metrics["downstream_bytes"] += int64(n)
		if _, ok := t.offsets["first_write_at_ms"]; !ok {
			t.offsets["first_write_at_ms"] = t.offset()
		}
		t.offsets["last_write_at_ms"] = t.offset()
	}
	if op == "flush" && succeeded {
		t.offsets["last_flush_at_ms"] = t.offset()
	}
}
