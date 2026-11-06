package middleware

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// AlertLifecycle starts before body parsing on inference HTTP routes. WebSocket
// messages are separate operations and must not be measured as one connection.
func AlertLifecycle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.AlertingEnabled || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		ctx, span := common.BeginAlertSpan(c.Request.Context(), common.AlertHealthEvent{Layer: "request", RequestID: c.GetString(common.RequestIdKey), SampleClass: "security"})
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			if class := c.GetString("alert_sample_class"); class != "" {
				span.Bind(common.AlertHealthEvent{SampleClass: class})
			}
			if c.GetInt("id") == 0 {
				span.Bind(common.AlertHealthEvent{SampleClass: "security"})
			}
			span.Bind(common.AlertHealthEvent{UserID: c.GetInt("id"), TokenID: c.GetInt("token_id"), Model: c.GetString("original_model"), Protocol: c.GetString("alert_protocol"), Stream: c.GetBool("alert_stream")})
			outcome := c.GetString("alert_outcome")
			if outcome == "" {
				outcome = "ignored"
				if c.GetInt("id") > 0 && c.Writer.Status() >= 500 {
					outcome = "failure"
				}
			}
			if c.Request.Context().Err() == context.Canceled && outcome != "success" {
				outcome = "ignored"
			}
			span.End(c.Request.Context(), outcome)
		}()
		c.Next()
	}
}

// AlertAuthenticatedRequest runs after successful authentication and before
// request parsing/selection. Unauthenticated uploads never become production
// overdue requests, and no credential contents enter lifecycle facts.
func AlertAuthenticatedRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil {
			class := "production"
			switch c.FullPath() {
			case "/mj/task/list-by-condition", "/mj-:mode/task/list-by-condition":
				class = "maintenance"
			}
			common.AlertSpanFromContext(c.Request.Context()).Bind(common.AlertHealthEvent{UserID: c.GetInt("id"), TokenID: c.GetInt("token_id"), SampleClass: class})
		}
		c.Next()
	}
}
