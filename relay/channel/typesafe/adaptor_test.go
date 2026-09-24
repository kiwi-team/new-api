package typesafe

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemOneRequestURL(t *testing.T) {
	adaptor := &Adaptor{}
	for _, tc := range []struct {
		name        string
		channelType int
		baseURL     string
		want        string
	}{
		{name: "TypeSafe", channelType: constant.ChannelTypeTypeSafe, baseURL: "https://api.typesafe.ai", want: "https://api.typesafe.ai/v1/systemone"},
		{name: "OpenRouter default", channelType: constant.ChannelTypeOpenRouter, baseURL: "https://openrouter.ai/api", want: "https://openrouter.ai/api/alpha/decisions"},
		{name: "OpenRouter v1 base", channelType: constant.ChannelTypeOpenRouter, baseURL: "https://openrouter.ai/api/v1", want: "https://openrouter.ai/api/alpha/decisions"},
		{name: "OpenRouter full endpoint", channelType: constant.ChannelTypeOpenRouter, baseURL: "https://openrouter.ai/api/alpha/decisions", want: "https://openrouter.ai/api/alpha/decisions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: tc.channelType, ChannelBaseUrl: tc.baseURL,
			}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, actual)
		})
	}
}

func TestOpenRouterSystemOneHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	header := make(http.Header)
	err := (&Adaptor{}).SetupRequestHeader(ctx, &header, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeOpenRouter,
		ApiKey:      "openrouter-key",
	}})
	require.NoError(t, err)
	assert.Equal(t, "Bearer openrouter-key", header.Get("Authorization"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
	assert.Equal(t, "https://www.newapi.ai", header.Get("HTTP-Referer"))
	assert.Equal(t, "New API", header.Get("X-OpenRouter-Title"))
}

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
	assert.Equal(t, dto.BillingUsageSourceSystemOne, usage.UsageSource)
}

func TestDoNativeResponseAcceptsOpenRouterUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := []byte(`{"model":"~typesafe/jev-latest","answers":{},"usage":{"prompt_tokens":296,"completion_tokens":20,"total_tokens":316}}`)
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
	assert.Equal(t, dto.BillingUsageSourceSystemOne, usage.UsageSource)
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
