package moonshot

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKimiChatMediaURLsBecomeBase64(t *testing.T) {
	previousMax := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	t.Cleanup(func() { constant.MaxFileDownloadMB = previousMax })
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	imageData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a0p8AAAAASUVORK5CYII=")
	require.NoError(t, err)
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		switch r.URL.Path {
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageData)
		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() { *fetch = previousFetch })
	*fetch = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowPrivateIp:       true,
		IpFilterMode:         true,
		IpList:               []string{"127.0.0.1/32"},
		AllowedPorts:         []string{strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port)},
	}
	service.InitHttpClient()

	imageURL := server.URL + "/image.png"
	inlineImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageData)
	for _, tc := range []struct {
		name     string
		adaptor  channel.Adaptor
		channel  int
		model    string
		upstream string
		convert  bool
	}{
		{name: "moonshot", adaptor: &Adaptor{}, channel: constant.ChannelTypeMoonshot, model: "kimi-k3", upstream: "kimi-k3", convert: true},
		{name: "openai compatible", adaptor: &openai.Adaptor{}, channel: constant.ChannelTypeOpenAI, model: "kimi-k3", upstream: "kimi-k3", convert: true},
		{name: "namespaced upstream", adaptor: &openai.Adaptor{}, channel: constant.ChannelTypeOpenAI, model: "vision", upstream: "moonshot/kimi-k3", convert: true},
		{name: "mapped upstream", adaptor: &openai.Adaptor{}, channel: constant.ChannelTypeOpenAI, model: "vision", upstream: "kimi-k3", convert: true},
		{name: "upstream omitted", adaptor: &openai.Adaptor{}, channel: constant.ChannelTypeOpenAI, model: "kimi-k3", convert: true},
		{name: "non kimi upstream", adaptor: &openai.Adaptor{}, channel: constant.ChannelTypeOpenAI, model: "kimi-k3", upstream: "gpt-4o"},
		{name: "bailian", adaptor: &ali.Adaptor{}, channel: constant.ChannelTypeAli, model: "kimi-k3", upstream: "kimi-k3", convert: true},
		{name: "bailian mapped upstream", adaptor: &ali.Adaptor{}, channel: constant.ChannelTypeAli, model: "vision", upstream: "moonshot/kimi-k3", convert: true},
		{name: "bailian non kimi", adaptor: &ali.Adaptor{}, channel: constant.ChannelTypeAli, model: "deepseek-v3", upstream: "deepseek-v3"},
		{name: "ark", adaptor: &volcengine.Adaptor{}, channel: constant.ChannelTypeVolcEngine, model: "kimi-k3", upstream: "kimi-k3", convert: true},
		{name: "ark endpoint mapping", adaptor: &volcengine.Adaptor{}, channel: constant.ChannelTypeVolcEngine, model: "kimi-k3", upstream: "ep-20261028000000-example", convert: true},
		{name: "ark non kimi endpoint", adaptor: &volcengine.Adaptor{}, channel: constant.ChannelTypeVolcEngine, model: "doubao-seed-2-0", upstream: "ep-20261028000000-example"},
		{name: "ark mapped non kimi", adaptor: &volcengine.Adaptor{}, channel: constant.ChannelTypeVolcEngine, model: "kimi-k3", upstream: "doubao-seed-2-0"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				downloads.Store(0)
				model := tc.upstream
				if model == "" {
					model = tc.model
				}
				body := fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"system","content":"Keep the content"},{"role":"user","content":[{"type":"image_url","image_url":{"url":%q,"detail":"high"},"cache_control":{"type":"ephemeral"}},{"type":"text","text":"请描述图片的内容。"},{"type":"image_url","image_url":{"url":%q}},{"type":"image_url","image_url":{"url":%q}},{"type":"video_url","video_url":{"url":%q}}]}],"thinking":{"type":"enabled"},"temperature":1.0}`, model, stream, imageURL, imageURL, inlineImage, server.URL+"/video.mp4")
				var request dto.GeneralOpenAIRequest
				require.NoError(t, common.UnmarshalJsonStr(body, &request))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				t.Cleanup(func() { service.CleanupFileSources(c) })
				info := &relaycommon.RelayInfo{
					RelayMode:       relayconstant.RelayModeChatCompletions,
					OriginModelName: tc.model,
					IsStream:        stream,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType:       tc.channel,
						UpstreamModelName: tc.upstream,
					},
				}
				converted, err := tc.adaptor.ConvertOpenAIRequest(c, info, &request)
				require.NoError(t, err)
				wire, err := common.Marshal(converted)
				require.NoError(t, err)
				expected := body
				if tc.convert {
					expected = strings.ReplaceAll(expected, imageURL, inlineImage)
					expected = strings.ReplaceAll(expected, server.URL+"/video.mp4", "data:video/mp4;base64,dmlkZW8=")
					assert.Equal(t, int32(2), downloads.Load(), "repeated media URLs should reuse the request cache")
				} else {
					assert.Zero(t, downloads.Load())
				}
				assert.JSONEq(t, expected, string(wire))
			})
		}
	}

	for _, adaptor := range []channel.Adaptor{&Adaptor{}, &openai.Adaptor{}, &ali.Adaptor{}, &volcengine.Adaptor{}} {
		for _, tc := range []struct {
			name      string
			url       string
			private   bool
			maxSizeMB int
			wantError string
		}{
			{name: "download failure", url: server.URL + "/missing.png", private: true, maxSizeMB: constant.MaxFileDownloadMB, wantError: "status code: 404"},
			{name: "private address rejected", url: imageURL, maxSizeMB: constant.MaxFileDownloadMB, wantError: "private IP address not allowed"},
			{name: "oversized download", url: imageURL, private: true, maxSizeMB: 0, wantError: "file size exceeds maximum allowed size"},
		} {
			t.Run(fmt.Sprintf("%T/%s", adaptor, tc.name), func(t *testing.T) {
				previousMax := constant.MaxFileDownloadMB
				t.Cleanup(func() { constant.MaxFileDownloadMB = previousMax })
				constant.MaxFileDownloadMB = tc.maxSizeMB
				fetch.AllowPrivateIp = tc.private
				var request dto.GeneralOpenAIRequest
				require.NoError(t, common.UnmarshalJsonStr(fmt.Sprintf(`{"model":"kimi-k3","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":%q}}]}]}`, tc.url), &request))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				t.Cleanup(func() { service.CleanupFileSources(c) })
				converted, err := adaptor.ConvertOpenAIRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "kimi-k3"}}, &request)
				require.ErrorContains(t, err, "convert image url to base64 failed")
				assert.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, converted)
			})
		}
	}
}

func TestConvertOpenAIRequestKimiK26UsesOnlyAllowedTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

func TestConvertOpenAIRequestKimiK26KeepsOmittedTemperatureOmitted(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "kimi-k2.6",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Nil(t, convertedRequest.Temperature)
}

func TestConvertOpenAIRequestOtherMoonshotModelKeepsTemperature(t *testing.T) {
	// 用没有专属参数改写规则的模型，避免与 kimi-k2.5 的参数适配互相干扰。
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2-0905",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2-0905",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 0.7, *convertedRequest.Temperature)
}

// kimi-k2.5 的采样参数由上游文档规定：top_p 固定 0.95，temperature 随 thinking 开关
// 取 1.0 / 0.6，客户端自定义值会被覆盖。
// https://platform.moonshot.cn/docs/guide/kimi-k2-5-quickstart
func TestConvertOpenAIRequestKimiK25AppliesDocumentedSampling(t *testing.T) {
	cases := []struct {
		name                string
		thinking            json.RawMessage
		expectedTemperature float64
		expectedThinking    string
	}{
		{name: "thinking omitted", thinking: nil, expectedTemperature: 1.0},
		{name: "thinking enabled", thinking: json.RawMessage(`{"type":"enabled","budget_tokens":1024}`), expectedTemperature: 1.0, expectedThinking: `{"type":"enabled"}`},
		{name: "thinking disabled", thinking: json.RawMessage(`{"type":"disabled"}`), expectedTemperature: 0.6, expectedThinking: `{"type":"disabled"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{
				Model:       "kimi-k2.5",
				Temperature: common.GetPointer[float64](0.7),
				TopP:        common.GetPointer[float64](0.1),
				THINKING:    tc.thinking,
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "kimi-k2.5",
				},
			}

			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

			require.NoError(t, err)
			convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			require.NotNil(t, convertedRequest.Temperature)
			assert.Equal(t, tc.expectedTemperature, *convertedRequest.Temperature)
			require.NotNil(t, convertedRequest.TopP)
			assert.Equal(t, 0.95, *convertedRequest.TopP)
			if tc.expectedThinking != "" {
				assert.JSONEq(t, tc.expectedThinking, string(convertedRequest.THINKING))
			}
		})
	}
}

// Kimi serves OpenAI Responses natively (kimi-k3); the body is forwarded
// unchanged and other models are rejected by the upstream.
func TestResponsesRequestPassesThroughToV1Responses(t *testing.T) {
	const body = `{"model":"kimi-k3","input":"Latest Go release?","tools":[{"type":"web_search"}],"stream":false}`
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.moonshot.invalid", UpstreamModelName: "kimi-k3"},
	}
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.moonshot.invalid/v1/responses", url)

	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	wire, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(wire))
}
