package gemini

import (
	"bytes"
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这两个用例是打真实网络的集成测试：需要一个可用的 Gemini API Key，
// 且被测 URL 必须可访问，因此默认跳过，仅在显式配置后手动运行。
//
//	GEMINI_TEST_API_KEY=xxx go test ./relay/channel/gemini/ -run Integration
const (
	geminiTestAPIKeyEnv  = "GEMINI_TEST_API_KEY"
	geminiTestBaseURLEnv = "GEMINI_TEST_BASE_URL"
	geminiTestFileURLEnv = "GEMINI_TEST_FILE_URL"
)

const defaultGeminiTestFileURL = "https://ark-project.tos-cn-beijing.volces.com/images/view.jpeg"

func TestUploadFileToGemini_Integration(t *testing.T) {
	apiKey := os.Getenv(geminiTestAPIKeyEnv)
	if apiKey == "" {
		t.Skipf("%s not set, skipping Gemini upload integration test", geminiTestAPIKeyEnv)
	}
	fileUri := os.Getenv(geminiTestFileURLEnv)
	if fileUri == "" {
		fileUri = defaultGeminiTestFileURL
	}

	uploadedFile, err := UploadFileToGemini(context.Background(), fileUri, apiKey, os.Getenv(geminiTestBaseURLEnv))
	require.NoError(t, err, "UploadFileToGemini failed")
	require.NotNil(t, uploadedFile, "expected a file from Gemini")
	assert.NotEmpty(t, uploadedFile.URI, "expected uploaded file to have a URI")
}

func TestGetFileMimeType_Integration(t *testing.T) {
	fileUri := os.Getenv(geminiTestFileURLEnv)
	if fileUri == "" {
		t.Skipf("%s not set, skipping Gemini mime type integration test", geminiTestFileURLEnv)
	}

	mimeType, err := GetFileMimeType(fileUri)
	require.NoError(t, err, "GetFileMimeType failed")
	assert.NotEmpty(t, mimeType)
}

// Exercise the actual adaptor and response handlers, including the Chat/SSE
// contract. Source/GCS transport boundaries are covered by the service tests.
func TestGeminiFilePlanActualAdaptorJSONAndSSE(t *testing.T) {
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{"GeminiFileRouting": `{"enabled":true,"mode":"forward","chat_model_patterns":["gemini-test"]}`}
	protection := system_setting.GetFetchSetting()
	previousProtection := *protection
	protection.EnableSSRFProtection = true
	protection.ApplyIPFilterForDomain = true
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		common.OptionMap = previousOptions
		*protection = previousProtection
		constant.StreamingTimeout = previousTimeout
	})
	for _, native := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%t/stream=%t", native, stream), func(t *testing.T) {
				payload := `{"candidates":[{"content":{"role":"model","parts":[{"text":"file received"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":321,"candidatesTokenCount":7,"totalTokenCount":328}}`
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Equal(t, "selected-service-key", r.Header.Get("x-goog-api-key"))
					assert.Equal(t, "Bearer selected-service-key", r.Header.Get("Authorization"))
					assert.Empty(t, r.Header.Get("Cookie"))
					assert.Equal(t, "application/pdf", gjson.GetBytes(raw, "contents.0.parts.0.inlineData.mimeType").String())
					assert.Equal(t, "JVBERi0xLjcK", gjson.GetBytes(raw, "contents.0.parts.0.inlineData.data").String())
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+payload+"\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, payload)
					}
				}))
				defer upstream.Close()
				path := "/v1/chat/completions"
				body := `{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"data:application/pdf;base64,JVBERi0xLjcK"}}]}]}`
				if native {
					path = "/v1beta/models/gemini-test:generateContent"
					if stream {
						path = "/v1beta/models/gemini-test:streamGenerateContent"
					}
					body = `{"contents":[{"parts":[{"inlineData":{"mimeType":"application/pdf","data":"JVBERi0xLjcK"}}]}]}`
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set("Authorization", "Bearer caller-key")
				c.Request.Header.Set("Cookie", "session=caller-secret")
				defer common.CleanupBodyStorage(c)
				plan, apiErr := service.PlanGeminiFiles(c, "gemini-test")
				require.Nil(t, apiErr)
				require.NotNil(t, plan)
				defer plan.Close(c)
				info := &relaycommon.RelayInfo{OriginModelName: "gemini-test", RequestURLPath: path, RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, IsStream: stream, ShouldIncludeUsage: true, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, ChannelBaseUrl: upstream.URL, UpstreamModelName: "gemini-test", ApiKey: "selected-service-key"}}
				if native {
					info.RelayFormat = types.RelayFormatGemini
					info.RelayMode = relayconstant.RelayModeGemini
				}
				adaptor := &Adaptor{}
				adaptor.Init(info)
				require.Nil(t, service.PrepareGeminiFileAttempt(c, info))
				var converted any
				var err error
				if native {
					var request dto.GeminiChatRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					require.NoError(t, service.ApplyGeminiFilePlan(c, &request))
					converted, err = adaptor.ConvertGeminiRequest(c, info, &request)
				} else {
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					converted, err = adaptor.ConvertOpenAIRequest(c, info, &request)
				}
				require.NoError(t, err)
				raw, err := common.Marshal(converted)
				require.NoError(t, err)
				raw, apiErr = service.FinalizeGeminiFileBody(c, raw, raw, false)
				require.Nil(t, apiErr)
				response, err := adaptor.DoRequest(c, info, bytes.NewReader(raw))
				require.NoError(t, err)
				usage, apiErr := adaptor.DoResponse(c, response.(*http.Response), info)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 321, usage.(*dto.Usage).PromptTokens)
				assert.Equal(t, 7, usage.(*dto.Usage).CompletionTokens)
				result := recorder.Body.String()
				assert.Contains(t, result, "file received")
				if native {
					assert.Contains(t, result, "candidates")
					assert.NotContains(t, result, "chat.completion")
				} else {
					assert.Contains(t, result, "chat.completion")
					assert.NotContains(t, result, `"candidates":`)
				}
				if stream {
					assert.Contains(t, result, "data: ")
				} else {
					assert.True(t, gjson.Valid(result))
				}
				cancelled, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(cancelled)
				_, err = adaptor.DoRequest(c, info, bytes.NewReader(raw))
				require.Error(t, err, "cancelled file requests must not outlive their object lifetime")
			})
		}
	}
}
