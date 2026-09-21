package ali_dashscope

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestASRUploadPolicyResponseAcceptsMaxFileSizeMBFormats(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int64
	}{
		{name: "number", body: `{"data":{"max_file_size_mb":2048}}`, want: 2048},
		{name: "numeric string", body: `{"data":{"max_file_size_mb":"2048"}}`, want: 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response asrUploadPolicyResponse
			require.NoError(t, common.Unmarshal([]byte(tt.body), &response))
			assert.Equal(t, tt.want, int64(response.Data.MaxFileSizeMB))
		})
	}
}

func TestConvertASRFlashRequestUsesSynchronousMultimodalPayload(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/uploads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"policy":"policy","signature":"signature","upload_dir":"dir","upload_host":"`+server.URL+`","max_file_size_mb":10,"oss_access_key_id":"key","x_oss_object_acl":"private","x_oss_forbid_overwrite":"true"}}`)
		case r.Method == http.MethodPost:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := file.Close(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	filePart, err := writer.CreateFormFile("file", "sample.wav")
	require.NoError(t, err)
	_, err = filePart.Write([]byte("audio"))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("language", "zh"))
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &requestBody)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "secret"}}

	converted, err := ConvertASRRequest(c, info, dto.AudioRequest{Model: "fun-asr-flash-2026-06-15"})
	require.NoError(t, err)
	body, err := io.ReadAll(converted)
	require.NoError(t, err)
	assert.Equal(t, "fun-asr-flash-2026-06-15", gjson.GetBytes(body, "model").String())
	assert.Equal(t, "input_audio", gjson.GetBytes(body, "input.messages.0.content.0.type").String())
	assert.Equal(t, "oss://dir/sample.wav", gjson.GetBytes(body, "input.messages.0.content.0.input_audio.data").String())
	assert.Equal(t, "wav", gjson.GetBytes(body, "parameters.format").String())
	assert.Equal(t, "zh", gjson.GetBytes(body, "parameters.language_hints.0").String())
}

func TestASRFlashHandlerReturnsOpenAIResponseAndDurationUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
	info := &relaycommon.RelayInfo{
		Request: &dto.AudioRequest{ResponseFormat: "json"},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "fun-asr-flash-2026-06-15",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"output":{"text":"欢迎使用阿里云。","sentence":{"begin_time":0,"end_time":3800,"sentence_end":true,"text":"欢迎使用阿里云。"}},
			"usage":{"duration":4},"request_id":"request"
		}`)),
	}

	newAPIError, usage := ASRHandler(c, resp, info)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 67, usage.PromptTokensDetails.AudioTokens)
	assert.JSONEq(t, `{"text":"欢迎使用阿里云。"}`, recorder.Body.String())
}

func TestNormalizeQwen3ASRUsageUsesReportedSeconds(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	usage := &dto.Usage{
		PromptTokens:     75,
		CompletionTokens: 12,
		TotalTokens:      87,
		Seconds:          3,
		PromptTokensDetails: dto.InputTokenDetails{
			AudioTokens: 75,
		},
	}

	NormalizeQwen3ASRUsage(info, usage)

	assert.Equal(t, 50, usage.PromptTokens)
	assert.Equal(t, 50, usage.PromptTokensDetails.AudioTokens)
	assert.Equal(t, 62, usage.TotalTokens)
	assert.Nil(t, info.QuotaClamp)
}

func TestQwen3ASRFlashUsesCompatibleChatEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://dashscope.aliyuncs.com",
			UpstreamModelName: "qwen3-asr-flash",
		},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", requestURL)
}
