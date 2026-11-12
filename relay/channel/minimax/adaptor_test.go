package minimax

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLForImageGeneration(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.minimax.chat",
		},
	}

	got, err := GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://api.minimax.chat/v1/image_generation"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestConvertImageRequest(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "image-01",
	}
	request := dto.ImageRequest{
		Model:          "image-01",
		Prompt:         "a red fox in snowfall",
		Size:           "1536x1024",
		ResponseFormat: "url",
		N:              uintPtr(2),
	}

	got, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["model"] != "image-01" {
		t.Fatalf("model = %#v, want %q", payload["model"], "image-01")
	}
	if payload["prompt"] != request.Prompt {
		t.Fatalf("prompt = %#v, want %q", payload["prompt"], request.Prompt)
	}
	if payload["n"] != float64(2) {
		t.Fatalf("n = %#v, want 2", payload["n"])
	}
	if payload["aspect_ratio"] != "3:2" {
		t.Fatalf("aspect_ratio = %#v, want %q", payload["aspect_ratio"], "3:2")
	}
	if payload["response_format"] != "url" {
		t.Fatalf("response_format = %#v, want %q", payload["response_format"], "url")
	}
}

func TestDoResponseForImageGeneration(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Unix(1700000000, 0),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       httptest.NewRecorder().Result().Body,
	}
	resp.Body = ioNopCloser(`{"data":{"image_urls":["https://example.com/minimax.png"]}}`)

	adaptor := &Adaptor{}
	usage, err := adaptor.DoResponse(c, resp, info)
	if err != nil {
		t.Fatalf("DoResponse returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("DoResponse returned nil usage")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"url":"https://example.com/minimax.png"`) {
		t.Fatalf("response body = %s, want OpenAI image response with image URL", body)
	}
	if strings.Contains(body, `"image_urls"`) {
		t.Fatalf("response body = %s, should not expose raw MiniMax image_urls payload", body)
	}
}

type nopReadCloser struct {
	*strings.Reader
}

func (n nopReadCloser) Close() error {
	return nil
}

func ioNopCloser(body string) nopReadCloser {
	return nopReadCloser{Reader: strings.NewReader(body)}
}

func uintPtr(v uint) *uint {
	return &v
}

func TestH3ImageRoutes(t *testing.T) {
	for _, model := range []string{"MiniMax-H3-Image", "MiniMax-H3-Image-Highspeed"} {
		assert.Contains(t, (&Adaptor{}).GetModelList(), model)
		assert.Contains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeMiniMax, model), constant.EndpointTypeImageGeneration)
		for _, tc := range []struct {
			mode int
			path string
		}{
			{relayconstant.RelayModeImagesGenerations, "/v1/images/generations"},
			{relayconstant.RelayModeImagesEdits, "/v1/images/edits"},
		} {
			info := &relaycommon.RelayInfo{
				RelayMode: tc.mode, OriginModelName: "public-image-alias",
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.minimax.cn/", UpstreamModelName: model},
			}
			got, err := GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.minimax.cn"+tc.path, got)
			info.ChannelBaseUrl = ""
			got, err = GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.minimax.cn"+tc.path, got)
		}
	}
}

func TestH3ImageJSONConversion(t *testing.T) {
	for _, model := range []string{"MiniMax-H3-Image", "MiniMax-H3-Image-Highspeed"} {
		for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"`+model+`","prompt":"draw a cat","n":10,"size":"auto","aspect_ratio":"16:9","area":"2K","disable_prompt_enhanced":false,"output_format":"webp","output_compression":0,"aigc_watermark":false,"response_format":"b64_json","stream":false,"images":[{"image_url":"mm_file://reference"}]}`), &request))
			converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: mode}, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, model, payload["model"])
			assert.Equal(t, "auto", payload["size"])
			assert.Equal(t, "16:9", payload["aspect_ratio"])
			assert.Equal(t, "2K", payload["area"])
			assert.Equal(t, "webp", payload["output_format"])
			assert.Equal(t, "b64_json", payload["response_format"])
			assert.Equal(t, float64(10), payload["n"])
			assert.Equal(t, float64(0), payload["output_compression"])
			assert.Equal(t, false, payload["aigc_watermark"])
			assert.Equal(t, false, payload["stream"])
			if mode == relayconstant.RelayModeImagesEdits {
				assert.NotContains(t, payload, "disable_prompt_enhanced")
				assert.Equal(t, []any{map[string]any{"image_url": "mm_file://reference"}}, payload["images"])
			} else {
				assert.Equal(t, false, payload["disable_prompt_enhanced"])
				assert.NotContains(t, payload, "images")
			}
		}
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}, dto.ImageRequest{
		Model: "MiniMax-H3-Image", Prompt: "cat", Size: "2720x1536",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"MiniMax-H3-Image","prompt":"cat","size":"2720x1536"}`, string(body), "explicit dimensions must not gain an aspect_ratio or area")

	for _, n := range []uint{0, 11, dto.MaxImageN + 1, ^uint(0)} {
		_, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}, dto.ImageRequest{Model: "MiniMax-H3-Image", N: &n})
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.True(t, types.IsSkipRetryError(apiErr))
	}
	for _, input := range []string{`{"output_compression":"0"}`, `{"aigc_watermark":0}`, `{"disable_prompt_enhanced":"false"}`} {
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(input), &request))
		request.Model = "MiniMax-H3-Image"
		_, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}, request)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	}
	for _, input := range []string{`{"image":"data:image/png;base64,Y2F0"}`, `{"images":["data:image/png;base64,Y2F0"]}`} {
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(input), &request))
		request.Model = "MiniMax-H3-Image"
		converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}, request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"images":[{"image_url":"data:image/png;base64,Y2F0"}]`)
	}
}

func TestH3ImageMultipartConversion(t *testing.T) {
	for _, n := range []string{"2", "0", "11", "18446744073709551615", "2,2"} {
		t.Run(n, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			fields := map[string]string{
				"model": "public-alias", "prompt": "keep the cat", "n": n,
				"size": "auto", "area": "4K", "aspect_ratio": "16:9", "output_format": "webp",
				"output_compression": "0", "aigc_watermark": "false", "stream": "true",
			}
			for key, value := range fields {
				for item := range strings.SplitSeq(value, ",") {
					require.NoError(t, writer.WriteField(key, item))
				}
			}
			for range 2 {
				part, err := writer.CreateFormFile("image[]", "cat.png")
				require.NoError(t, err)
				_, err = part.Write([]byte("reference image"))
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			require.NoError(t, c.Request.ParseMultipartForm(1<<20))
			t.Cleanup(func() { require.NoError(t, c.Request.MultipartForm.RemoveAll()) })
			// As at ingress, the DTO has a normalized count. The original
			// form must still be checked before being sent to the provider.
			request := dto.ImageRequest{Model: "MiniMax-H3-Image-Highspeed", Prompt: fields["prompt"], N: uintPtr(2)}
			converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}, request)
			if n != "2" {
				var apiErr *types.NewAPIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				return
			}
			require.NoError(t, err)
			replayed := httptest.NewRequest(http.MethodPost, "/v1/images/edits", converted.(*bytes.Buffer))
			replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
			require.NoError(t, replayed.ParseMultipartForm(1<<20))
			t.Cleanup(func() { require.NoError(t, replayed.MultipartForm.RemoveAll()) })
			assert.Equal(t, request.Model, replayed.FormValue("model"))
			for key, value := range fields {
				if key != "model" {
					assert.Equal(t, value, replayed.FormValue(key), key)
				}
			}
			require.Len(t, replayed.MultipartForm.File["image[]"], 2)
			for _, header := range replayed.MultipartForm.File["image[]"] {
				file, err := header.Open()
				require.NoError(t, err)
				image, err := io.ReadAll(file)
				require.NoError(t, err)
				require.NoError(t, file.Close())
				assert.Equal(t, "reference image", string(image))
			}
		})
	}
}

func TestH3ImageResponsesAndBilling(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	const usageJSON = `{"generated_images":2,"input_tokens":1500,"output_tokens":300,"total_tokens":1800,"input_tokens_details":{"text_tokens":1000,"image_tokens":500},"output_tokens_details":{"image_tokens":300}}`
	for _, model := range []string{"MiniMax-H3-Image", "MiniMax-H3-Image-Highspeed"} {
		for _, mode := range []string{"image_generation", "image_edit"} {
			for _, format := range []string{"json", "sse"} {
				t.Run(model+"/"+mode+"/"+format, func(t *testing.T) {
					body := `{"created":1792308678,"data":[{"url":"https://example.com/1.jpg","size":"2720x1536","revised_prompt":"cat"},{"error":{"code":"1027","message":"content safety"}},{"url":"https://example.com/3.jpg"}],"output_format":"jpeg","usage":` + usageJSON + `,"request_id":"trace"}`
					contentType := "application/json"
					if format == "sse" {
						contentType = "text/event-stream"
						body = "data: {\"type\":\"" + mode + ".partial_succeeded\",\"image_index\":0,\"url\":\"https://example.com/1.jpg\"}\n\n" +
							"data: {\"type\":\"" + mode + ".partial_failed\",\"image_index\":1,\"code\":\"1027\",\"message\":\"content safety\"}\n\n" +
							"data: {\"type\":\"" + mode + ".partial_succeeded\",\"image_index\":2,\"url\":\"https://example.com/3.jpg\"}\n\n" +
							"data: {\"type\":\"" + mode + ".completed\",\"usage\":" + usageJSON + ",\"request_id\":\"trace\"}\n\n"
					}
					c, recorder, response, info := newH3ImageResponseTest(body, contentType, model)
					if mode == "image_edit" {
						info.RelayMode = relayconstant.RelayModeImagesEdits
					}
					value, apiErr := (&Adaptor{}).DoResponse(c, response, info)
					require.Nil(t, apiErr)
					usage := value.(*dto.Usage)
					assert.Equal(t, 1500, usage.PromptTokens)
					assert.Equal(t, 300, usage.CompletionTokens)
					assert.Equal(t, 1800, usage.TotalTokens)
					assert.Equal(t, 500, usage.PromptTokensDetails.ImageTokens)
					assert.Equal(t, 300, usage.CompletionTokenDetails.ImageTokens)
					require.NotNil(t, info.BillingImageCount)
					assert.Equal(t, 2, *info.BillingImageCount, "the batch completion must not count as an image")
					assert.Contains(t, recorder.Body.String(), "content safety")
					assert.Contains(t, recorder.Body.String(), `"request_id":"trace"`)
					if format == "json" {
						assert.JSONEq(t, body, recorder.Body.String())
					} else {
						assert.Contains(t, recorder.Body.String(), "event: "+mode+".partial_succeeded")
						assert.Contains(t, recorder.Body.String(), "event: "+mode+".completed")
					}
					expression, ok := billing_setting.GetBuiltinBillingExpr(model)
					require.True(t, ok)
					result, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{
						ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000,
					}, service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expression)), billingexpr.RequestInput{ImageCount: info.BillingImageCount})
					require.NoError(t, err)
					want := 2022
					if model == "MiniMax-H3-Image-Highspeed" {
						want = 2941
					}
					assert.Equal(t, want, result.ActualQuotaAfterGroup, "actual tokens already include all successful images")
				})
			}
		}
	}
}

func newH3ImageResponseTest(body, contentType, model string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations, OriginModelName: "public-alias",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}, IsStream: contentType == "text/event-stream",
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{EstimatedImageCount: common.GetPointer(3)},
	}
	return c, recorder, response, info
}

func TestH3ImageStreamFailuresAndDisconnect(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	t.Run("all failed propagates error for refund", func(t *testing.T) {
		body := "data: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"message\":\"generation failed\"},\"usage\":{\"generated_images\":0,\"input_tokens\":0,\"output_tokens\":0}}\n\n"
		c, recorder, response, info := newH3ImageResponseTest(body, "text/event-stream", "MiniMax-H3-Image")
		_, apiErr := (&Adaptor{}).DoResponse(c, response, info)
		require.NotNil(t, apiErr)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Contains(t, recorder.Body.String(), "generation failed")
		assert.True(t, info.StreamStatus.HasErrors())
	})
	t.Run("duplicate success and payload-less event do not add images", func(t *testing.T) {
		body := "data: {\"type\":\"image_generation.partial_succeeded\",\"image_index\":0,\"url\":\"https://example.com/1.jpg\"}\n\n"
		body += body + "data: {\"type\":\"image_generation.partial_succeeded\",\"image_index\":1}\n\n"
		c, _, response, info := newH3ImageResponseTest(body, "text/event-stream", "MiniMax-H3-Image")
		_, apiErr := (&Adaptor{}).DoResponse(c, response, info)
		require.Nil(t, apiErr)
		require.NotNil(t, info.BillingImageCount)
		assert.Equal(t, 1, *info.BillingImageCount)
		assert.Equal(t, "missing_usage", info.UsageBillingFallback)
	})
	t.Run("client abort keeps reserved image count", func(t *testing.T) {
		c, recorder, response, info := newH3ImageResponseTest("", "text/event-stream", "MiniMax-H3-Image")
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Writer = &h3DisconnectWriter{ResponseWriter: c.Writer, cancel: cancel}
		reader, writer := io.Pipe()
		response.Body = reader
		t.Cleanup(func() { _ = writer.Close() })
		go func() {
			_, _ = io.WriteString(writer, "data: {\"type\":\"image_generation.partial_succeeded\",\"image_index\":0,\"url\":\"https://example.com/1.jpg\"}\n\n")
		}()
		_, apiErr := (&Adaptor{}).DoResponse(c, response, info)
		require.Nil(t, apiErr)
		assert.Contains(t, recorder.Body.String(), "partial_succeeded")
		assert.Nil(t, info.BillingImageCount)
		assert.Equal(t, 3, info.RequestedImageCount())
		assert.Equal(t, "missing_usage", info.UsageBillingFallback)
	})
}

type h3DisconnectWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

func (w *h3DisconnectWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if bytes.Contains(data, []byte("partial_succeeded")) {
		w.cancel()
	}
	return n, err
}
