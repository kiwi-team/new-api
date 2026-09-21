package typesafe

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoNativeResponsePreservesSystemOnePayloadAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := []byte(`{"model":"jev-1.13.0","answers":{"urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":296,"output_tokens":20},"future_field":{"kept":true}}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	usage, responseBody, apiErr := (&Adaptor{}).DoNativeResponse(ctx, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, upstreamBody, responseBody)
	assert.Equal(t, string(upstreamBody), recorder.Body.String())
	assert.Equal(t, 296, usage.PromptTokens)
	assert.Equal(t, 20, usage.CompletionTokens)
	assert.Equal(t, 316, usage.TotalTokens)
	assert.Equal(t, dto.BillingUsageSourceTypeSafe, usage.UsageSource)
}

func TestDoNativeResponseRejectsNegativeUsageBeforeWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"model":"jev-1.13.0","answers":{},"usage":{"input_tokens":-1,"output_tokens":0}}`)),
		Header:     make(http.Header),
	}

	usage, _, apiErr := (&Adaptor{}).DoNativeResponse(ctx, resp)
	require.NotNil(t, apiErr)
	assert.Nil(t, usage)
	assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
	assert.Empty(t, recorder.Body.String())
}

func TestDoNativeResponseRejectsMissingUsageBeforeWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"model":"jev-1.13.0","answers":{}}`)),
		Header:     make(http.Header),
	}

	usage, _, apiErr := (&Adaptor{}).DoNativeResponse(ctx, resp)
	require.NotNil(t, apiErr)
	assert.Nil(t, usage)
	assert.Contains(t, apiErr.Error(), "missing usage")
	assert.Empty(t, recorder.Body.String())
}
