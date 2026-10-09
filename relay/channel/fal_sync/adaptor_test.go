package fal_sync

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFlux3TestContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c
}

func TestFlux3RequestConversionAndEndpoints(t *testing.T) {
	require.True(t, slices.Contains(ModelList, Flux3Model))
	assert.False(t, slices.Contains(ModelList, "flux-3"), "the image adaptor must not claim the existing video model name")

	t.Run("text to image", func(t *testing.T) {
		adaptor := &Adaptor{}
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(`{
			"model":"flux-3-image",
			"prompt":"a lighthouse",
			"size":"1792x1024",
			"output_format":"png",
			"resolution":"4k",
			"metadata":{
				"resolution":"2k",
				"enable_prompt_expansion":false,
				"safety_tolerance":0,
				"sync_mode":false,
				"version":"latest"
			}
		}`), &request))
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: Flux3Model},
			Request:     &request,
		}

		converted, err := adaptor.ConvertImageRequest(newFlux3TestContext(), info, request)
		require.NoError(t, err)
		fluxRequest := converted.(*Flux3ImageRequest)
		assert.Empty(t, fluxRequest.ImageURLs)
		assert.Equal(t, "16:9", fluxRequest.AspectRatio)
		assert.Equal(t, "2k", fluxRequest.Resolution)
		assert.Equal(t, "png", fluxRequest.OutputFormat)
		require.NotNil(t, fluxRequest.EnablePromptExpansion)
		assert.False(t, *fluxRequest.EnablePromptExpansion)
		require.NotNil(t, fluxRequest.SafetyTolerance)
		assert.Zero(t, *fluxRequest.SafetyTolerance)
		require.NotNil(t, fluxRequest.SyncMode)
		assert.False(t, *fluxRequest.SyncMode)
		wireRequest, err := common.Marshal(fluxRequest)
		require.NoError(t, err)
		assert.Contains(t, string(wireRequest), `"enable_prompt_expansion":false`)
		assert.Contains(t, string(wireRequest), `"safety_tolerance":0`)
		assert.Contains(t, string(wireRequest), `"sync_mode":false`)
		assert.NotContains(t, string(wireRequest), `"metadata"`)
		billingInput, err := relayhelper.ResolveImageBillingRequestInput(newFlux3TestContext(), info, billingexpr.RequestInput{})
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"flux-3-image","n":1,"size":"1792x1024","quality":"","metadata":{"resolution":"2k"}}`, string(billingInput.Body))

		requestURL, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://queue.fal.run/"+flux3TextToImageEndpoint, requestURL)
		assert.Equal(t,
			"https://queue.fal.run/"+flux3TextToImageEndpoint+"/requests/request-1/status",
			buildStatusURL("https://queue.fal.run", adaptor.pollingEndpoint(info), "request-1"),
		)
	})

	t.Run("image edit", func(t *testing.T) {
		adaptor := &Adaptor{}
		request := dto.ImageRequest{
			Model:  Flux3Model,
			Prompt: "turn day into night",
			Images: []byte(`[
				"https://example.com/first.png",
				"data:image/png;base64,c2Vjb25k"
			]`),
		}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: Flux3Model},
			Request:     &request,
		}

		converted, err := adaptor.ConvertImageRequest(newFlux3TestContext(), info, request)
		require.NoError(t, err)
		fluxRequest := converted.(*Flux3ImageRequest)
		assert.Equal(t, []string{
			"https://example.com/first.png",
			"data:image/png;base64,c2Vjb25k",
		}, fluxRequest.ImageURLs)

		requestURL, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://queue.fal.run/"+flux3EditImageEndpoint, requestURL)
		assert.Equal(t,
			"https://queue.fal.run/"+flux3EditImageEndpoint+"/requests/request-2",
			buildResultURL("https://queue.fal.run", adaptor.pollingEndpoint(info), "request-2"),
		)
	})
}

func TestFlux3RejectsUnsupportedImageCounts(t *testing.T) {
	elevenImages := `[` + strings.Repeat(`"https://example.com/image.png",`, 10) + `"https://example.com/image.png"]`
	request := dto.ImageRequest{Model: Flux3Model, Prompt: "edit", Images: []byte(elevenImages)}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: Flux3Model},
		Request:     &request,
	}

	_, err := (&Adaptor{}).ConvertImageRequest(newFlux3TestContext(), info, request)
	require.ErrorContains(t, err, "at most 10 reference images")

	n := uint(2)
	request = dto.ImageRequest{Model: Flux3Model, Prompt: "generate", N: &n}
	info.Request = &request
	_, err = (&Adaptor{}).ConvertImageRequest(newFlux3TestContext(), info, request)
	require.ErrorContains(t, err, "exactly one output image")

	request = dto.ImageRequest{Model: Flux3Model, Prompt: "generate", Metadata: []byte(`{"resolution":"8k"}`)}
	info.Request = &request
	_, err = (&Adaptor{}).ConvertImageRequest(newFlux3TestContext(), info, request)
	require.ErrorContains(t, err, "metadata.resolution must be one of")
}

func TestFALResponseUpdatesActualImageCount(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"images":[
				{"url":"https://example.com/one.png"},
				{"url":"https://example.com/two.png"}
			]
		}`)),
	}
	requested := 3
	info := &relaycommon.RelayInfo{
		ChannelMeta:           &relaycommon.ChannelMeta{UpstreamModelName: Flux3Model},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{EstimatedImageCount: &requested},
	}

	_, apiErr := (&Adaptor{}).DoResponse(c, response, info)
	require.Nil(t, apiErr)
	require.NotNil(t, info.BillingImageCount)
	assert.Equal(t, 2, *info.BillingImageCount)
	assert.Contains(t, recorder.Body.String(), "https://example.com/one.png")
}

func TestFALQueueOperationURLUsesSubmitResponseSafely(t *testing.T) {
	reference := "https://queue.fal.run/blackforestlabs/flux-3/text-to-image"

	assert.Equal(t,
		"https://queue.fal.run/blackforestlabs/flux-3/requests/request-1/status",
		sameOriginQueueURL(reference, "https://queue.fal.run/blackforestlabs/flux-3/requests/request-1/status"),
	)
	assert.Equal(t,
		"https://queue.fal.run/blackforestlabs/flux-3/requests/request-1/response",
		sameOriginQueueURL(reference, "/blackforestlabs/flux-3/requests/request-1/response"),
	)
	assert.Empty(t,
		sameOriginQueueURL(reference, "https://example.com/steal-api-key"),
		"fal operation URLs must not redirect authenticated polling to another origin",
	)
}
