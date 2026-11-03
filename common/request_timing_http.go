package common

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"
)

type timingCallKey struct{}
type timingKindKey struct{}
type timingPurposeKey struct{}
type timingPurpose struct {
	name    string
	primary bool
}

func WithTimingPurpose(ctx context.Context, purpose string, primary bool) context.Context {
	if TimingFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, timingPurposeKey{}, timingPurpose{purpose, primary})
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// TimingHTTPClient also satisfies provider SDK HTTP client interfaces.
type TimingHTTPClient struct {
	Client  HTTPDoer
	Context context.Context
	Purpose string
	Primary bool
	Kind    string
}

func (c TimingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req != nil && TimingFromContext(c.Context) == nil && TimingFromContext(req.Context()) == nil {
		return c.Client.Do(req)
	}
	if req != nil && c.Kind != "" {
		req = req.WithContext(context.WithValue(req.Context(), timingKindKey{}, c.Kind))
	}
	return DoTimedHTTP(c.Context, c.Client, req, c.Purpose, c.Primary)
}

type timingRoundTripClient struct{ transport http.RoundTripper }

func (c timingRoundTripClient) Do(req *http.Request) (*http.Response, error) {
	return c.transport.RoundTrip(req)
}

func TimedRoundTrip(transport http.RoundTripper, req *http.Request) (*http.Response, error) {
	if call, ok := req.Context().Value(timingCallKey{}).(*TimingCall); ok {
		// The outer Client.Do owns body accounting. This layer records each
		// RoundTrip (including redirect responses), without wrapping the body twice.
		started := time.Now()
		resp, err := transport.RoundTrip(req)
		t := call.owner
		t.mu.Lock()
		call.metrics["transport_exchange_count"]++
		if len(call.exchanges) < 8 {
			exchange := map[string]any{"started_at_ms": started.Sub(t.started).Milliseconds(),
				"headers_returned_at_ms": t.offset(), "status_code": nil, "error_class": timingErrorClass(err)}
			if resp != nil {
				exchange["status_code"] = resp.StatusCode
			}
			call.exchanges = append(call.exchanges, exchange)
		}
		t.mu.Unlock()
		return resp, err
	}
	return DoTimedHTTP(req.Context(), timingRoundTripClient{transport}, req, "dependency", false)
}

// BeginTimingOperation records an SDK operation that does not expose its HTTP transport.
func BeginTimingOperation(ctx context.Context, purpose string, primary bool) *TimingCall {
	if t := TimingFromContext(ctx); t != nil {
		call := t.beginCall(ctx, purpose, primary)
		t.mu.Lock()
		call.kind = "sdk_opaque"
		t.mu.Unlock()
		return call
	}
	return nil
}

func (call *TimingCall) End() {
	if call != nil {
		call.end("operation_end")
	}
}

func (t *RequestTiming) beginCall(ctx context.Context, purpose string, primary bool) *TimingCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callCount++
	a, _ := ctx.Value(timingAttemptKey{}).(*TimingAttempt)
	call := &TimingCall{owner: t, id: "call-" + strconv.Itoa(t.callCount), attempt: a, purpose: purpose, primary: primary,
		kind: "http", offsets: map[string]int64{"call_started_at_ms": t.offset()}, metrics: make(map[string]int64), state: make(map[string]any)}
	if len(t.calls) < timingLimit {
		call.retained = true
		t.calls = append(t.calls, call)
	}
	if a != nil {
		if len(a.callIDs) < timingLimit {
			a.callIDs = append(a.callIDs, call.id)
		}
		if primary {
			if _, ok := a.offsets["upstream_call_started_at_ms"]; !ok {
				a.offsets["upstream_call_started_at_ms"] = t.offset()
			}
		}
	}
	if _, ok := t.offsets["first_outbound_call_started_at_ms"]; !ok {
		t.offsets["first_outbound_call_started_at_ms"] = t.offset()
	}
	if primary {
		if _, ok := t.offsets["first_upstream_call_started_at_ms"]; !ok {
			t.offsets["first_upstream_call_started_at_ms"] = t.offset()
		}
	}
	t.coverage["upstream"] = "partial"
	return call
}

func (call *TimingCall) mark(name string) {
	t := call.owner
	t.mu.Lock()
	if _, exists := call.offsets[name]; !exists {
		call.offsets[name] = t.offset()
	}
	t.mu.Unlock()
}

func (call *TimingCall) end(kind string) {
	t := call.owner
	t.mu.Lock()
	if call.ended {
		t.mu.Unlock()
		return
	}
	call.ended = true
	call.offsets["call_ended_at_ms"] = t.offset()
	call.state["body_end_kind"] = kind
	background := t.operationID != "" && t.autoFinish
	if background {
		t.finished = true
		t.offsets["handler_ended_at_ms"] = t.offset()
	}
	late := t.finished || !call.retained
	var snapshot map[string]any
	if late && !background && t.emit != nil {
		snapshot = call.snapshotLocked()
	}
	requestID, operationID := t.requestID, t.operationID
	t.mu.Unlock()
	if background && t.emit != nil {
		t.emit(t.Snapshot("operation_end"))
		return
	}
	if late && t.emit != nil {
		event := map[string]any{"event": "request_timing", "schema_version": requestTimingSchemaVersion, "scope": "outbound_call", "phase": "call_end", "request_id": requestID, "outbound_call": snapshot}
		if operationID != "" {
			delete(event, "request_id")
			event["operation_id"] = operationID
		}
		t.emit(event)
	}
}

func (call *TimingCall) snapshotLocked() map[string]any {
	m := maps.Clone(call.state)
	m["call_id"], m["purpose"], m["is_primary"], m["transport_kind"] = call.id, call.purpose, call.primary, call.kind
	m["attempt_id"] = nil
	m["status"] = "ended"
	if !call.ended {
		m["status"] = "pending_at_finalize"
	}
	if call.attempt != nil {
		m["attempt_id"] = call.attempt.id
	}
	for _, key := range []string{"call_started_at_ms", "call_ended_at_ms", "get_conn_at_ms", "got_conn_at_ms", "dns_started_at_ms", "dns_ended_at_ms", "connect_started_at_ms", "connect_ended_at_ms", "tls_started_at_ms", "tls_ended_at_ms", "request_headers_written_at_ms", "request_written_at_ms", "first_response_header_byte_at_ms", "response_headers_ready_at_ms", "response_body_first_byte_at_ms", "response_body_last_byte_at_ms", "response_body_ended_at_ms"} {
		m[key] = timingValue(call.offsets, key)
	}
	for key, value := range call.metrics {
		m[key] = value
	}
	exchanges := make([]map[string]any, 0, len(call.exchanges))
	for _, exchange := range call.exchanges {
		exchanges = append(exchanges, maps.Clone(exchange))
	}
	m["transport_exchanges"] = exchanges
	writes := make([]map[string]any, 0, len(call.writeEvents))
	for _, event := range call.writeEvents {
		writes = append(writes, maps.Clone(event))
	}
	m["request_write_events"] = writes
	m["transport_details_truncated"] = call.metrics["transport_exchange_count"] > int64(len(exchanges)) || call.metrics["request_write_attempts"] > int64(len(writes))
	m["call_total_ms"] = timingDuration(call.offsets, "call_ended_at_ms", "call_started_at_ms")
	m["connection_wait_ms"] = timingDuration(call.offsets, "got_conn_at_ms", "get_conn_at_ms")
	m["upstream_body_write_ms"], m["upstream_wait_headers_ms"] = nil, nil
	if call.metrics["request_write_attempts"] == 1 && call.state["request_write_succeeded"] == true && call.state["early_response"] != true {
		m["upstream_body_write_ms"] = timingDuration(call.offsets, "request_written_at_ms", "request_headers_written_at_ms")
		m["upstream_wait_headers_ms"] = timingDuration(call.offsets, "response_headers_ready_at_ms", "request_written_at_ms")
	} else if call.metrics["request_write_attempts"] > 1 {
		m["missing_reason"] = "ambiguous_transport_retry"
	} else {
		m["missing_reason"] = "not_observed"
	}
	if call.kind == "sdk_opaque" {
		m["missing_reason"] = "unsupported_transport"
	}
	m["upstream_receive_span_ms"] = timingDuration(call.offsets, "response_body_last_byte_at_ms", "response_body_first_byte_at_ms")
	m["upstream_tail_wait_ms"] = timingDuration(call.offsets, "response_body_ended_at_ms", "response_body_last_byte_at_ms")
	m["upstream_read_call_sum_ms"], m["upstream_read_call_max_ms"] = nil, nil
	if call.metrics["read_calls"] > 0 {
		m["upstream_read_call_sum_ms"] = float64(call.metrics["read_sum_ns"]) / 1e6
		m["upstream_read_call_max_ms"] = float64(call.metrics["read_max_ns"]) / 1e6
		m["response_body_bytes"] = call.metrics["response_body_bytes"]
	}
	return m
}

// DoTimedHTTP observes the existing request; its cancellation, redirects and client remain unchanged.
func DoTimedHTTP(source context.Context, client HTTPDoer, req *http.Request, purpose string, primary bool) (*http.Response, error) {
	if req == nil {
		return client.Do(req)
	}
	t := TimingFromContext(source)
	if existing := TimingFromContext(req.Context()); existing != nil && existing != t {
		t = existing
		source = req.Context()
	} else if t == nil {
		t = TimingFromContext(req.Context())
		source = req.Context()
	}
	if t == nil {
		return client.Do(req)
	}
	if _, exists := req.Context().Value(timingCallKey{}).(*TimingCall); exists {
		return client.Do(req)
	}
	if hint, ok := req.Context().Value(timingPurposeKey{}).(timingPurpose); ok {
		purpose, primary = hint.name, hint.primary
	}
	call := t.beginCall(source, purpose, primary)
	if kind, ok := req.Context().Value(timingKindKey{}).(string); ok {
		t.mu.Lock()
		call.kind = kind
		t.mu.Unlock()
	}
	ctx := context.WithValue(WithTimingContext(req.Context(), source), timingCallKey{}, call)
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { call.mark("get_conn_at_ms") },
		GotConn: func(info httptrace.GotConnInfo) {
			call.mark("got_conn_at_ms")
			t.mu.Lock()
			call.state["connection_reused"] = info.Reused
			t.mu.Unlock()
		},
		DNSStart:          func(httptrace.DNSStartInfo) { call.mark("dns_started_at_ms") },
		DNSDone:           func(httptrace.DNSDoneInfo) { call.mark("dns_ended_at_ms") },
		ConnectStart:      func(string, string) { call.mark("connect_started_at_ms") },
		ConnectDone:       func(string, string, error) { call.mark("connect_ended_at_ms") },
		TLSHandshakeStart: func() { call.mark("tls_started_at_ms") },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { call.mark("tls_ended_at_ms") },
		WroteHeaders:      func() { call.mark("request_headers_written_at_ms") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.mu.Lock()
			call.metrics["request_write_attempts"]++
			call.state["request_write_succeeded"] = info.Err == nil
			call.offsets["request_written_at_ms"] = t.offset()
			if len(call.writeEvents) < 8 {
				call.writeEvents = append(call.writeEvents, map[string]any{"at_ms": t.offset(), "succeeded": info.Err == nil,
					"error_class": timingErrorClass(info.Err)})
			}
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() { call.mark("first_response_header_byte_at_ms") },
	}
	observed := req.WithContext(httptrace.WithClientTrace(ctx, trace))
	if observed.Body != nil && observed.Body != http.NoBody {
		observed.Body = &timingUploadBody{ReadCloser: observed.Body, call: call}
	}
	if getBody := observed.GetBody; getBody != nil {
		observed.GetBody = func() (io.ReadCloser, error) {
			body, err := getBody()
			if err != nil {
				return nil, err
			}
			return &timingUploadBody{ReadCloser: body, call: call}, nil
		}
	}
	resp, err := client.Do(observed)
	if resp != nil {
		call.mark("response_headers_ready_at_ms")
		t.mu.Lock()
		call.state["http_status_code"] = resp.StatusCode
		_, written := call.offsets["request_written_at_ms"]
		call.state["early_response"] = !written
		t.mu.Unlock()
		if resp.Body != nil {
			resp.Body = &timingResponseBody{ReadCloser: resp.Body, call: call}
		} else {
			call.end("no_body")
		}
	}
	if err != nil {
		t.mu.Lock()
		call.state["request_error_class"] = timingErrorClass(err)
		t.mu.Unlock()
		call.end("request_error")
	}
	return resp, err
}

type timingUploadBody struct {
	io.ReadCloser
	call *TimingCall
}

func (body *timingUploadBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if n > 0 {
		t := body.call.owner
		t.mu.Lock()
		body.call.metrics["request_body_bytes"] += int64(n)
		t.mu.Unlock()
	}
	return n, err
}

type timingResponseBody struct {
	io.ReadCloser
	call *TimingCall
}

func (body *timingResponseBody) Read(p []byte) (int, error) {
	started := time.Now()
	n, err := body.ReadCloser.Read(p)
	elapsed := time.Since(started).Nanoseconds()
	call, t := body.call, body.call.owner
	t.mu.Lock()
	call.metrics["read_sum_ns"] += elapsed
	call.metrics["read_calls"]++
	call.metrics["read_max_ns"] = max(call.metrics["read_max_ns"], elapsed)
	if n > 0 {
		if _, exists := call.offsets["response_body_first_byte_at_ms"]; !exists {
			call.offsets["response_body_first_byte_at_ms"] = t.offset()
		}
		call.offsets["response_body_last_byte_at_ms"] = t.offset()
		call.metrics["response_body_bytes"] += int64(n)
	}
	if err != nil {
		if _, ended := call.offsets["response_body_ended_at_ms"]; !ended {
			call.offsets["response_body_ended_at_ms"] = t.offset()
		}
		if err != io.EOF {
			if _, exists := call.state["response_read_error_class"]; !exists {
				call.state["response_read_error_class"] = timingErrorClass(err)
			}
		}
	}
	t.mu.Unlock()
	if err == io.EOF {
		call.end("eof")
	} else if err != nil {
		call.end("read_error")
	}
	return n, err
}
func (body *timingResponseBody) Close() error {
	err := body.ReadCloser.Close()
	body.call.mark("response_body_ended_at_ms")
	body.call.end("closed")
	return err
}

// Classify errors without retaining provider messages, URLs or credentials.
func timingErrorClass(err error) any {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return "io_error"
}
