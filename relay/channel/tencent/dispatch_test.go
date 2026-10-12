package tencent

import (
	"bytes"
	"encoding/binary"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDispatchAdaptorInit(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		baseURL     string
		wantTC3     bool
		wantBaseURL string
	}{
		{
			name:        "legacy three-segment key selects TC3 adaptor and keeps base url",
			apiKey:      "1300000000|AKIDxxxxxxxx|secretxxxxxxxx",
			baseURL:     constant.ChannelBaseURLs[constant.ChannelTypeTencent],
			wantTC3:     true,
			wantBaseURL: constant.ChannelBaseURLs[constant.ChannelTypeTencent],
		},
		{
			name:        "tokenhub key with default base url rewrites to tokenhub",
			apiKey:      "sk-xxxxxxxxxxxxxxxx",
			baseURL:     constant.ChannelBaseURLs[constant.ChannelTypeTencent],
			wantTC3:     false,
			wantBaseURL: tokenHubBaseURL,
		},
		{
			name:        "tokenhub key with empty base url rewrites to tokenhub",
			apiKey:      "sk-xxxxxxxxxxxxxxxx",
			baseURL:     "",
			wantTC3:     false,
			wantBaseURL: tokenHubBaseURL,
		},
		{
			name:        "tokenhub key with custom base url is preserved",
			apiKey:      "sk-xxxxxxxxxxxxxxxx",
			baseURL:     "https://proxy.example.com",
			wantTC3:     false,
			wantBaseURL: "https://proxy.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeTencent,
				ApiKey:         tt.apiKey,
				ChannelBaseUrl: tt.baseURL,
			}}

			dispatch := &DispatchAdaptor{}
			dispatch.Init(info)

			require.NotNil(t, dispatch.Adaptor)
			if tt.wantTC3 {
				assert.IsType(t, &Adaptor{}, dispatch.Adaptor)
			} else {
				assert.IsType(t, &TokenHubAdaptor{}, dispatch.Adaptor)
			}
			assert.Equal(t, tt.wantBaseURL, info.ChannelBaseUrl)
		})
	}
}

func TestTokenHubASRRequestAndResponse(t *testing.T) {
	t.Run("uploaded file becomes TokenHub JSON", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", hyASRModel))
		require.NoError(t, writer.WriteField("language", "zh"))
		part, err := writer.CreateFormFile("file", "speech.mp3")
		require.NoError(t, err)
		_, err = part.Write([]byte("audio"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
		ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
		info := tokenHubASRRelayInfo(&dto.AudioRequest{Model: hyASRModel, ResponseFormat: "json"})
		adaptor := &TokenHubAdaptor{}
		converted, err := adaptor.ConvertAudioRequest(ctx, info, *info.Request.(*dto.AudioRequest))
		require.NoError(t, err)
		encoded, err := io.ReadAll(converted)
		require.NoError(t, err)
		assert.Equal(t, hyASRModel, gjson.GetBytes(encoded, "model").String())
		assert.Equal(t, "zh", gjson.GetBytes(encoded, "source").String())
		assert.Equal(t, "mp3", gjson.GetBytes(encoded, "voice_encode_format").String())
		assert.Equal(t, "YXVkaW8=", gjson.GetBytes(encoded, "data").String())
		assert.False(t, gjson.GetBytes(encoded, "input_url").Exists())
	})

	t.Run("audio URL and request metadata", func(t *testing.T) {
		fetchSetting := system_setting.GetFetchSetting()
		savedFetchSetting := *fetchSetting
		fetchSetting.EnableSSRFProtection = false
		t.Cleanup(func() { *fetchSetting = savedFetchSetting })
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", hyASRModel))
		require.NoError(t, writer.WriteField("audio_url", "https://media.example.com/a/test.mp4?signature=secret"))
		require.NoError(t, writer.Close())
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
		ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
		info := tokenHubASRRelayInfo(&dto.AudioRequest{Model: hyASRModel})
		info.ChannelBaseUrl = "https://proxy.example.com/"
		adaptor := &TokenHubAdaptor{}
		converted, err := adaptor.ConvertAudioRequest(ctx, info, *info.Request.(*dto.AudioRequest))
		require.NoError(t, err)
		encoded, err := io.ReadAll(converted)
		require.NoError(t, err)
		assert.Equal(t, "https://media.example.com/a/test.mp4?signature=secret", gjson.GetBytes(encoded, "input_url").String())
		assert.Equal(t, "mp4", gjson.GetBytes(encoded, "voice_encode_format").String())
		requestURL, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://proxy.example.com"+tokenHubSyncASRPath, requestURL)
		header := http.Header{}
		require.NoError(t, adaptor.SetupRequestHeader(ctx, &header, info))
		assert.Equal(t, "application/json", header.Get("Content-Type"))
		assert.Equal(t, "Bearer tokenhub-key", header.Get("Authorization"))
	})

	t.Run("verbose response carries usage", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
		info := tokenHubASRRelayInfo(&dto.AudioRequest{Model: hyASRModel, ResponseFormat: "verbose_json"})
		response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
			"status":"completed","request_id":"request-id",
			"output":{"source":"zh","duration_ms":15230,"text":"你好","sentences":[{"begin_ms":500,"end_ms":3200,"text":"你好"}]},
			"usage":{"total_token":335}}`))}
		usageValue, apiErr := (&TokenHubAdaptor{}).DoResponse(ctx, response, info)
		require.Nil(t, apiErr)
		usage := usageValue.(*dto.Usage)
		assert.Equal(t, 335, usage.PromptTokens)
		assert.Equal(t, 335, usage.PromptTokensDetails.AudioTokens)
		assert.Equal(t, 16, usage.Seconds)
		assert.Equal(t, "transcribe", gjson.Get(recorder.Body.String(), "task").String())
		assert.Equal(t, "你好", gjson.Get(recorder.Body.String(), "segments.0.text").String())
	})

	t.Run("non-completed response is rejected", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
		response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"failed","request_id":"request-id"}`))}
		usage, apiErr := (&TokenHubAdaptor{}).DoResponse(ctx, response, tokenHubASRRelayInfo(&dto.AudioRequest{Model: hyASRModel}))
		assert.Nil(t, usage)
		require.NotNil(t, apiErr)
		assert.Contains(t, apiErr.Error(), "failed")
	})
}

func TestTokenHubHyImageV35RequestAndResponse(t *testing.T) {
	t.Run("native chat request is sent to the image endpoint", func(t *testing.T) {
		body := `{
			"model":"hy-image-v3.5-preview",
			"session":"demo-session-001",
			"size":"4096x4096",
			"seed":0,
			"generate_max_pixels":4194304,
			"resize_max_pixels":1048576,
			"footnote":"new-api",
			"use_search_tool":{"value":false},
			"messages":[
				{"role":"assistant","reasoning":"keep this","tool_calls":[{"id":"generate@call_0","type":"function","function":{"name":"generate","arguments":"{}"}}]},
				{"role":"tool","tool_call_id":"generate@call_0","content":[{"type":"image_url","image_url":{"url":"https://example.com/previous.png"},"is_generate":true}]},
				{"role":"user","content":[{"type":"text","text":"draw a cat"}]}
			]
		}`
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		request := &dto.GeneralOpenAIRequest{}
		require.NoError(t, common.UnmarshalBodyReusable(ctx, request))
		info := tokenHubImageRelayInfo()
		adaptor := &TokenHubAdaptor{}

		converted, err := adaptor.ConvertOpenAIRequest(ctx, info, request)
		require.NoError(t, err)
		encoded, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.Equal(t, hyImageV35Model, gjson.GetBytes(encoded, "model").String())
		assert.Equal(t, "demo-session-001", gjson.GetBytes(encoded, "session").String())
		assert.True(t, gjson.GetBytes(encoded, "seed").Exists())
		assert.Equal(t, int64(0), gjson.GetBytes(encoded, "seed").Int())
		assert.True(t, gjson.GetBytes(encoded, "use_search_tool.value").Exists())
		assert.False(t, gjson.GetBytes(encoded, "use_search_tool.value").Bool())
		assert.True(t, gjson.GetBytes(encoded, "messages.1.content.0.is_generate").Bool())
		assert.Equal(t, "keep this", gjson.GetBytes(encoded, "messages.0.reasoning").String())

		requestURL, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, tokenHubBaseURL+tokenHubHyImageV35Path, requestURL)
		header := http.Header{}
		require.NoError(t, adaptor.SetupRequestHeader(ctx, &header, info))
		assert.Equal(t, "Bearer tokenhub-key", header.Get("Authorization"))
		assert.Equal(t, "application/json", header.Get("Content-Type"))
	})

	t.Run("request validation rejects unsupported values", func(t *testing.T) {
		tests := []struct {
			name string
			body string
		}{
			{"streaming", `{"model":"hy-image-v3.5-preview","stream":true,"messages":[{"role":"user","content":"cat"}]}`},
			{"invalid size", `{"model":"hy-image-v3.5-preview","size":"8192x8192","messages":[{"role":"user","content":"cat"}]}`},
			{"negative seed", `{"model":"hy-image-v3.5-preview","seed":-1,"messages":[{"role":"user","content":"cat"}]}`},
			{"oversized generate pixels", `{"model":"hy-image-v3.5-preview","generate_max_pixels":4194305,"messages":[{"role":"user","content":"cat"}]}`},
			{"long footnote", `{"model":"hy-image-v3.5-preview","footnote":"12345678901234567","messages":[{"role":"user","content":"cat"}]}`},
			{"missing search value", `{"model":"hy-image-v3.5-preview","use_search_tool":{},"messages":[{"role":"user","content":"cat"}]}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tt.body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				request := &dto.GeneralOpenAIRequest{}
				require.NoError(t, common.UnmarshalBodyReusable(ctx, request))
				_, err := (&TokenHubAdaptor{}).ConvertOpenAIRequest(ctx, tokenHubImageRelayInfo(), request)
				require.Error(t, err)
			})
		}
	})

	t.Run("successful response is preserved and TokenHub usage is billed", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		responseBody := `{
			"id":"image-id","object":"image.chat.completion.chunk","model":"HY-Image-3.5-preview-4090-Tob-v1.2",
			"choices":[{"index":0,"delta":{"type":"image","image":{"url":"https://example.com/result.png","width":4096,"height":4096}},"finish_reason":null}],
			"usage":{"total_tokens":19000},"tokenhub_usage":{"total_tokens":20000},
			"assembled_history":[{"role":"assistant","reasoning":"preserved"}]
		}`
		response := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}
		usageValue, apiErr := (&TokenHubAdaptor{}).DoResponse(ctx, response, tokenHubImageRelayInfo())
		require.Nil(t, apiErr)
		usage := usageValue.(*dto.Usage)
		assert.Equal(t, 20000, usage.PromptTokens)
		assert.Equal(t, 20000, usage.TotalTokens)
		assert.JSONEq(t, responseBody, recorder.Body.String())
	})
}

func TestHyASRTokenEstimateUsesProviderTokenRate(t *testing.T) {
	wav := oneSecondPCM16WAV()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", hyASRModel))
	part, err := writer.CreateFormFile("file", "speech.wav")
	require.NoError(t, err)
	_, err = part.Write(wav)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info := tokenHubASRRelayInfo(&dto.AudioRequest{Model: hyASRModel})
	tokens, err := service.CountRequestToken(ctx, &types.TokenCountMeta{}, info)
	require.NoError(t, err)
	assert.Equal(t, 22, tokens)
}

func tokenHubASRRelayInfo(request *dto.AudioRequest) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		Request:         request,
		OriginModelName: hyASRModel,
		RelayMode:       relayconstant.RelayModeAudioTranscription,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeTencent,
			ApiKey:            "tokenhub-key",
			ChannelBaseUrl:    tokenHubBaseURL,
			UpstreamModelName: hyASRModel,
		},
	}
}

func tokenHubImageRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: hyImageV35Model,
		RelayMode:       relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeTencent,
			ApiKey:            "tokenhub-key",
			ChannelBaseUrl:    tokenHubBaseURL,
			UpstreamModelName: hyImageV35Model,
		},
	}
}

func oneSecondPCM16WAV() []byte {
	const sampleRate = uint32(16000)
	const dataSize = uint32(32000)
	buffer := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buffer.WriteString("RIFF")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(36)+dataSize)
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, sampleRate)
	_ = binary.Write(buffer, binary.LittleEndian, sampleRate*2)
	_ = binary.Write(buffer, binary.LittleEndian, uint16(2))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	_ = binary.Write(buffer, binary.LittleEndian, dataSize)
	buffer.Write(make([]byte, dataSize))
	return buffer.Bytes()
}
