package middleware

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

var albTimingRootPattern = regexp.MustCompile(`^1-[0-9a-fA-F]{8}-[0-9a-fA-F]{24}$`)

// RequestTiming must be installed before recovery and before any routes.
func RequestTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.RequestTimingEnabled || common.TimingFromContext(c.Request.Context()) != nil {
			c.Next()
			return
		}
		traceRoot := ""
		for item := range strings.SplitSeq(c.GetHeader("X-Amzn-Trace-Id"), ";") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(item), "Root="); ok && albTimingRootPattern.MatchString(value) {
				traceRoot = value
			}
		}
		emitTiming := common.RequestTimingLogEmitter(func(data string) {
			logger.LogInfo(context.Background(), data)
		})
		ctx, timing := common.NewRequestTiming(c.Request.Context(), c.GetString(common.RequestIdKey), traceRoot, emitTiming)
		c.Request = c.Request.WithContext(ctx)
		common.TimingMetric(ctx, "has_body", 0)
		if c.Request.Body != nil && c.Request.Body != http.NoBody {
			common.TimingMetric(ctx, "has_body", 1)
			c.Request.Body = &timingRequestBody{ReadCloser: c.Request.Body, timing: timing, expected: c.Request.ContentLength}
		}
		stop := context.AfterFunc(ctx, func() { common.MarkTiming(ctx, "context_done_observed_at_ms") })
		writer := &timingResponseWriter{ResponseWriter: c.Writer, timing: timing, stop: stop}
		c.Writer = writer
		defer func() {
			stop()
			if ctx.Err() != nil {
				common.MarkTiming(ctx, "context_done_observed_at_ms")
			}
			c.Writer = writer.ResponseWriter
			if writer.hijacked {
				return
			}
			timing.Finish()
			if emitTiming == nil {
				return
			}
			snapshot := timing.Snapshot("request_end", c.Request.Context())
			snapshot["http_method"], snapshot["http_status_code"] = c.Request.Method, writer.Status()
			snapshot["route_pattern"], snapshot["request_path"] = c.FullPath(), c.FullPath()
			snapshot["request_kind"] = c.GetString(RouteTagKey)
			snapshot["node_name"], snapshot["build_version"] = common.NodeName, common.Version
			emitTiming(snapshot)
		}()
		c.Next()
	}
}

type timingRequestBody struct {
	io.ReadCloser
	timing   *common.RequestTiming
	expected int64
	bytes    atomic.Int64
	started  atomic.Bool
}

func (body *timingRequestBody) Read(p []byte) (int, error) {
	started := time.Now()
	if !body.started.Load() {
		body.timing.ObserveBody(started, 0, false, false)
	}
	body.started.Store(true)
	n, err := body.ReadCloser.Read(p)
	total := body.bytes.Add(int64(n))
	complete := err == io.EOF || body.expected >= 0 && total >= body.expected
	body.timing.ObserveBody(started, n, complete, complete || err != nil)
	return n, err
}

func (body *timingRequestBody) Close() error {
	err := body.ReadCloser.Close()
	if body.started.Load() {
		complete := body.expected >= 0 && body.bytes.Load() >= body.expected
		body.timing.ObserveBody(time.Now(), 0, complete, true)
	}
	return err
}

type timingResponseWriter struct {
	gin.ResponseWriter
	timing   *common.RequestTiming
	stop     func() bool
	hijacked bool
}

func (w *timingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *timingResponseWriter) Write(data []byte) (int, error) {
	started := time.Now()
	n, err := w.ResponseWriter.Write(data)
	w.timing.ObserveDownstream("write", time.Since(started), n, err == nil, true)
	return n, err
}
func (w *timingResponseWriter) WriteString(data string) (int, error) {
	started := time.Now()
	n, err := w.ResponseWriter.WriteString(data)
	w.timing.ObserveDownstream("write", time.Since(started), n, err == nil, true)
	return n, err
}
func (w *timingResponseWriter) Flush() { _ = w.FlushError() }
func (w *timingResponseWriter) FlushError() error {
	started := time.Now()
	var err error
	observable := false
	if flusher, ok := w.ResponseWriter.(interface{ FlushError() error }); ok {
		observable = true
		err = flusher.FlushError()
	} else {
		w.ResponseWriter.Flush()
	}
	w.timing.ObserveDownstream("flush", time.Since(started), 0, err == nil, observable)
	return err
}
func (w *timingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.Hijack()
	if err == nil {
		w.hijacked = true
		w.stop()
	}
	return conn, rw, err
}
func (w *timingResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	// Keep writes observable without counting ReaderFrom and Write twice.
	return io.Copy(struct{ io.Writer }{w}, reader)
}
