package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// RequestErrorLog covers bare status/JSON rejections as well as panics. Relay
// and protocol handlers record their original errors explicitly before rewrite;
// this fallback never buffers successful responses or SSE traffic.
func RequestErrorLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		service.InitRequestErrorLog(c)
		writer := &requestErrorWriter{ResponseWriter: c.Writer, ctx: c}
		c.Writer = writer
		defer func() {
			c.Writer = writer.ResponseWriter
			recovered := recover()
			if isRelayErrorRequest(c) && !service.RequestErrorHandled(c) {
				if recovered != nil {
					service.RecordRequestError(c, types.NewErrorWithStatusCode(fmt.Errorf("request panic: %v", recovered), types.ErrorCodeBadResponse, http.StatusInternalServerError))
				} else if writer.Status() >= http.StatusBadRequest {
					writer.recordJSONError(writer.body.Bytes(), false)
					if !service.RequestErrorHandled(c) {
						service.RecordRequestError(c, types.NewOpenAIError(errors.New(http.StatusText(writer.Status())), types.ErrorCodeBadResponseStatusCode, writer.Status()))
					}
				}
			}
			if recovered != nil {
				panic(recovered)
			}
		}()
		c.Next()
	}
}

type requestErrorWriter struct {
	gin.ResponseWriter
	ctx  *gin.Context
	body bytes.Buffer
}

func isRelayErrorRequest(c *gin.Context) bool {
	path := c.Request.URL.Path
	return c.GetString(RouteTagKey) == "relay" || strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/") || strings.HasPrefix(path, "/v1alpha/")
}

func (w *requestErrorWriter) Write(data []byte) (int, error) {
	originalLength := len(data)
	if w.Status() >= http.StatusBadRequest && isRelayErrorRequest(w.ctx) && !service.RequestErrorHandled(w.ctx) && w.body.Len() < 64*1024 {
		// Gin renders JSON errors in one write. Capture and rewrite before the
		// first byte is sent; fragmented errors retain a bounded log fallback.
		if w.body.Len() == 0 && len(data) <= 64*1024 {
			data = w.recordJSONError(data, !w.Written())
		}
		w.body.Write(data[:min(len(data), 64*1024-w.body.Len())])
	}
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		return min(n, originalLength), err
	}
	return originalLength, nil
}

func (w *requestErrorWriter) WriteString(data string) (int, error) {
	if w.Status() < http.StatusBadRequest {
		return w.ResponseWriter.WriteString(data)
	}
	return w.Write([]byte(data))
}

func (w *requestErrorWriter) recordJSONError(data []byte, rewrite bool) []byte {
	var envelope map[string]common.RawMessage
	if common.Unmarshal(data, &envelope) != nil {
		return data
	}
	target := envelope
	var nested map[string]common.RawMessage
	if common.Unmarshal(envelope["error"], &nested) == nil && nested != nil {
		target = nested
	}
	field := "message"
	var message string
	_ = common.Unmarshal(target[field], &message)
	if message == "" {
		field = "description"
		_ = common.Unmarshal(target[field], &message)
	}
	if message == "" {
		field = "error"
		_ = common.Unmarshal(target[field], &message)
	}
	if message == "" {
		return data
	}
	code := ""
	if value := target["code"]; len(value) != 0 && string(value) != "null" {
		code = common.JsonRawMessageToString(value)
	}
	service.RecordRequestError(w.ctx, types.NewOpenAIError(errors.New(message), types.ErrorCode(code), w.Status()))
	if !rewrite || common.RewriteErrorMessage(message) == message {
		return data
	}
	target[field], _ = common.Marshal(service.ClientErrorMessage(w.ctx, message))
	if nested != nil {
		envelope["error"], _ = common.Marshal(nested)
	}
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return data
	}
	w.Header().Del("Content-Length")
	return encoded
}
