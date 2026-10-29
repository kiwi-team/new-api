package relay

import (
	"context"
	"io"
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
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiInlineFilesPreserveUpstreamErrors(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{model_setting.GeminiFileRoutingKey: `{"enabled":true,"mode":"forward","chat_model_patterns":["gemini-test"]}`}
	common.OptionMapRWMutex.Unlock()
	protection := system_setting.GetFetchSetting()
	previousProtection := *protection
	protection.EnableSSRFProtection = true
	protection.ApplyIPFilterForDomain = true
	previousTLS := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = false
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		*protection = previousProtection
		common.TLSInsecureSkipVerify = previousTLS
	})
	service.InitHttpClient()

	const message = "Image generation is not supported for this model"
	const upstreamError = `{"error":{"code":400,"message":"` + message + `","status":"INVALID_ARGUMENT"}}`
	for _, endpoint := range []struct {
		name           string
		native, stream bool
	}{
		{name: "native JSON", native: true},
		{name: "native SSE", native: true, stream: true},
		{name: "chat JSON"},
		{name: "chat SSE", stream: true},
	} {
		for _, tc := range []struct {
			name              string
			remote, mixed     bool
			transport, mapped bool
		}{
			{name: "inline upstream error"},
			{name: "inline status mapping", mapped: true},
			{name: "inline transport error", transport: true},
			{name: "remote reference upstream error", remote: true},
			{name: "remote reference transport error", remote: true, transport: true},
			{name: "mixed attachment upstream error", remote: true, mixed: true},
		} {
			t.Run(endpoint.name+"/"+tc.name, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, upstreamError)
				}))
				t.Cleanup(upstream.Close)
				path := "/v1/chat/completions"
				inline := `{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}`
				remote := `{"type":"image_url","image_url":{"url":"https://generativelanguage.googleapis.com/v1beta/files/test","mime_type":"image/png"}}`
				if endpoint.native {
					path = "/v1beta/models/gemini-test:generateContent"
					if endpoint.stream {
						path = "/v1beta/models/gemini-test:streamGenerateContent"
					}
					inline = `{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}`
					remote = `{"fileData":{"mimeType":"image/png","fileUri":"https://generativelanguage.googleapis.com/v1beta/files/test"}}`
				}
				attachments := inline
				if tc.remote {
					attachments = remote
					if tc.mixed {
						attachments = inline + "," + remote
					}
				}
				body := `{"model":"gemini-test","messages":[{"role":"user","content":[` + attachments + `]}]}`
				if endpoint.native {
					body = `{"contents":[{"role":"user","parts":[{"text":"Generate an image"},` + attachments + `]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"],"imageConfig":{"aspectRatio":"16:9","imageSize":"2K"}}}`
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
				common.SetContextKey(c, constant.ContextKeyChannelKey, "selected-service-key")
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-test")
				if tc.mapped {
					c.Set("status_code_mapping", `{"400":422}`)
				}
				plan, apiErr := service.PlanGeminiFiles(c, "gemini-test")
				require.Nil(t, apiErr)
				require.NotNil(t, plan)
				t.Cleanup(func() { plan.Close(c) })
				assert.False(t, plan.RequiresStorage, "even small remote references must retain their error redaction")
				if tc.transport {
					ctx, cancel := context.WithCancel(c.Request.Context())
					cancel()
					c.Request = c.Request.WithContext(ctx)
				}
				info := &relaycommon.RelayInfo{
					OriginModelName: "gemini-test", RequestURLPath: path, IsStream: endpoint.stream,
					RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
				}
				if endpoint.native {
					var request dto.GeminiChatRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					info.Request = &request
					info.RelayFormat = types.RelayFormatGemini
					info.RelayMode = relayconstant.RelayModeGemini
					apiErr = GeminiHelper(c, info)
				} else {
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					request.Stream = &endpoint.stream
					info.Request = &request
					apiErr = TextHelper(c, info)
				}
				require.NotNil(t, apiErr)
				if tc.remote {
					assert.Equal(t, types.ErrorCode("file_inference_failed"), apiErr.GetErrorCode())
					assert.NotContains(t, apiErr.Error(), message)
					if tc.transport {
						assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					} else {
						assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
					}
					return
				}
				if tc.transport {
					assert.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
					assert.Contains(t, apiErr.ToOpenAIError().Message, "do request failed")
					return
				}
				assert.Equal(t, types.ErrorCode("400"), apiErr.GetErrorCode())
				assert.EqualError(t, apiErr, message)
				assert.Equal(t, message, apiErr.ToOpenAIError().Message)
				if tc.mapped {
					assert.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
				} else {
					assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				}
			})
		}
	}
}
