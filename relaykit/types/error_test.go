package types

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetMessageUpdatesOpenAIRelayPayload(t *testing.T) {
	err := WithOpenAIError(OpenAIError{
		Message: "quota exceeded (request id: upstream)",
		Type:    "upstream_error",
		Param:   "model",
		Code:    "insufficient_quota",
	}, http.StatusForbidden)

	err.SetMessage("请求失败，请稍后再尝试 (request id: local)")

	response := err.ToOpenAIError()
	assert.Equal(t, "请求失败，请稍后再尝试 (request id: local)", err.Error())
	assert.Equal(t, "请求失败，请稍后再尝试 (request id: local)", response.Message)
	assert.Equal(t, "model", response.Param)
	assert.Equal(t, "insufficient_quota", response.Code)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestSetMessageUpdatesClaudeRelayPayload(t *testing.T) {
	err := WithClaudeError(ClaudeError{
		Message: "original upstream message",
		Type:    "overloaded_error",
	}, http.StatusServiceUnavailable)

	err.SetMessage("rewritten message (request id: local)")

	response := err.ToClaudeError()
	assert.Equal(t, "rewritten message (request id: local)", err.Error())
	assert.Equal(t, "rewritten message (request id: local)", response.Message)
	assert.Equal(t, "overloaded_error", response.Type)
}
