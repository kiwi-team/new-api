package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func geminiFileTestContext(t *testing.T, path, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() {
		if plan := GetGeminiFilePlan(c); plan != nil {
			plan.Close(c)
		}
		common.CleanupBodyStorage(c)
	})
	return c
}

func configureGeminiFileTest(t *testing.T, config string) {
	t.Helper()
	configureSSRFTestFetchSetting(t)
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{model_setting.GeminiFileRoutingKey: config}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() { common.OptionMapRWMutex.Lock(); common.OptionMap = previous; common.OptionMapRWMutex.Unlock() })
}

func TestGeminiFilePlanChatContent(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"chat_model_patterns":["^gemini-"]}`)
	for _, tc := range []struct {
		name, body, wantError string
		wantPositions         []string
	}{
		{
			name: "string content",
			body: `{"model":"gemini-3-flash-preview","messages":[{"role":"user","content":"你是谁"}],"temperature":0.01,"max_tokens":100000,"chat_template_kwargs":{"enable_thinking":false},"thinking":{"type":"disabled"}}`,
		},
		{
			name: "text array",
			body: `{"messages":[{"role":"user","content":[{"type":"text","text":"你是谁"}]}]}`,
		},
		{
			name:          "string message before attachments",
			body:          `{"messages":[{"role":"system","content":"Describe the image"},{"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}]}`,
			wantPositions: []string{"messages[1].content[1]"},
		},
		{
			name:      "embedded markdown image",
			body:      `{"messages":[{"role":"user","content":"![image](data:image/png;base64,iVBORw0KGgo=)"}]}`,
			wantError: "messages[0]: use image_url for embedded markdown images",
		},
		{
			name:      "unsupported array content",
			body:      `{"messages":[{"role":"user","content":[{"type":"unknown"}]}]}`,
			wantError: "messages[0].content[0]: unsupported content type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := geminiFileTestContext(t, "/v1/chat/completions", tc.body)
			plan, apiErr := PlanGeminiFiles(c, "gemini-3-flash-preview")
			if tc.wantError != "" {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Equal(t, types.ErrorCode("unsupported_file_input"), apiErr.GetErrorCode())
				assert.EqualError(t, apiErr, tc.wantError)
				return
			}
			require.Nil(t, apiErr)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			body, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tc.body, string(body), "file planning must preserve the request body")
			if len(tc.wantPositions) == 0 {
				assert.Nil(t, plan)
				return
			}
			require.NotNil(t, plan)
			require.Len(t, plan.Files, len(tc.wantPositions))
			for i, position := range tc.wantPositions {
				assert.Equal(t, position, plan.Files[i].Position)
			}
		})
	}
}

func TestGeminiFilePlanPreservesAttachmentsBeyondSixteen(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"chat_model_patterns":["^gemini-"]}`)
	for _, tc := range []struct {
		name  string
		count int
		kinds []string
	}{
		{"images across messages", 17, []string{"image_url"}},
		{"Developer API image limit", 3600, []string{"image_url"}},
		{"audio without a file count limit", 17, []string{"audio_url"}},
		{"PDFs across messages", 17, []string{"pdf_url"}},
		{"mixed attachments", 18, []string{"image_url", "audio_url", "pdf_url"}},
	} {
		for _, native := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/native=%t", tc.name, native), func(t *testing.T) {
				parts := make([]map[string]any, tc.count)
				expected := make([]dto.GeminiInlineData, tc.count)
				for i := range parts {
					kind := tc.kinds[i%len(tc.kinds)]
					mimeType, data := "image/png", "iVBORw0KGgo="
					switch kind {
					case "audio_url":
						mimeType, data = "audio/wav", "UklGRgAAAABXQVZF"
					case "pdf_url":
						mimeType, data = "application/pdf", "JVBERi0xLjcK"
					}
					expected[i] = dto.GeminiInlineData{MimeType: mimeType, Data: data}
					parts[i] = map[string]any{"type": kind, kind: map[string]string{"url": "data:" + mimeType + ";base64," + data}}
					if native {
						parts[i] = map[string]any{"inlineData": map[string]string{"mimeType": mimeType, "data": data}}
					}
				}
				path := "/v1/chat/completions"
				body := map[string]any{
					"model": "gemini-2.5-flash",
					"messages": []map[string]any{
						{"role": "user", "content": parts[:tc.count-1]},
						{"role": "user", "content": parts[tc.count-1:]},
					},
				}
				if native {
					path = "/v1beta/models/gemini-2.5-flash:generateContent"
					body = map[string]any{"contents": []map[string]any{
						{"role": "user", "parts": parts[:tc.count-1]},
						{"role": "user", "parts": parts[tc.count-1:]},
					}}
				}
				raw, err := common.Marshal(body)
				require.NoError(t, err)
				c := geminiFileTestContext(t, path, string(raw))
				plan, apiErr := PlanGeminiFiles(c, "gemini-2.5-flash")
				require.Nil(t, apiErr)
				require.NotNil(t, plan)
				require.Len(t, plan.Files, tc.count)
				assert.False(t, plan.RequiresStorage)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, UpstreamModelName: "gemini-2.5-flash"}, RelayFormat: types.RelayFormatOpenAI}
				require.Nil(t, PrepareGeminiFileAttempt(c, info))
				var result *dto.GeminiChatRequest
				if native {
					var request dto.GeminiChatRequest
					require.NoError(t, common.Unmarshal(raw, &request))
					require.NoError(t, ApplyGeminiFilePlan(c, &request))
					result = &request
				} else {
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.Unmarshal(raw, &request))
					converted, err := ConvertRequest(c, info, types.RelayFormatGemini, &request)
					require.NoError(t, err)
					result = converted.Value.(*dto.GeminiChatRequest)
				}
				require.Len(t, result.Contents, 2)
				require.Len(t, result.Contents[0].Parts, tc.count-1)
				require.Len(t, result.Contents[1].Parts, 1)
				i := 0
				for _, content := range result.Contents {
					for _, part := range content.Parts {
						assert.Equal(t, &expected[i], part.InlineData)
						i++
					}
				}
			})
		}
	}
}

func TestGeminiInlineFilesStayInlineAndPreserveChatConversion(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"mode":"forward","chat_model_patterns":["^gemini-"],"inline_threshold_bytes":64}`)
	previousCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = previousCountToken })
	fixtures := []struct {
		kind, mime string
		prefix     []byte
	}{
		{"image_url", "image/png", []byte("\x89PNG\r\n\x1a\n")},
		{"audio_url", "audio/wav", []byte("RIFF\x00\x00\x00\x00WAVE")},
		{"video_url", "video/mp4", []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")},
		{"pdf_url", "application/pdf", []byte("%PDF-1.7\n")},
		{"input_audio", "audio/wav", []byte("RIFF\x00\x00\x00\x00WAVE")},
		{"file", "application/pdf", []byte("%PDF-1.7\n")},
	}
	for _, fixture := range fixtures {
		for _, size := range []int{63, 64, 65} {
			for _, native := range []bool{false, true} {
				if native && (fixture.kind == "input_audio" || fixture.kind == "file") {
					continue
				}
				t.Run(fmt.Sprintf("%s/%d/native=%t", fixture.kind, size, native), func(t *testing.T) {
					data := append(bytes.Clone(fixture.prefix), bytes.Repeat([]byte{'x'}, size-len(fixture.prefix))...)
					encoded := base64.StdEncoding.EncodeToString(data)
					path := "/v1/chat/completions"
					body := fmt.Sprintf(`{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":%q,%q:{"url":%q}}]}]}`, fixture.kind, fixture.kind, "data:"+fixture.mime+";base64,"+encoded)
					if fixture.kind == "input_audio" {
						body = fmt.Sprintf(`{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"input_audio","input_audio":{"data":%q,"format":"wav"}}]}]}`, encoded)
					} else if fixture.kind == "file" {
						body = fmt.Sprintf(`{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"file","file":{"file_data":%q}}]}]}`, "data:application/pdf;base64,"+encoded)
					}
					if native {
						path = "/v1beta/models/gemini-test:generateContent"
						body = fmt.Sprintf(`{"contents":[{"role":"user","parts":[{"text":"inspect"},{"inline_data":{"mime_type":%q,"data":%q},"videoMetadata":{"fps":1}}]}]}`, fixture.mime, encoded)
					}
					c := geminiFileTestContext(t, path, body)
					plan, apiErr := PlanGeminiFiles(c, "gemini-test")
					require.Nil(t, apiErr)
					require.NotNil(t, plan)
					require.Len(t, plan.Files, 1)
					assert.False(t, plan.RequiresStorage, "base64 attachments must not trigger large file routing")
					assert.False(t, plan.Files[0].Large)
					assert.Equal(t, int64(size), plan.Files[0].Size)
					info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, UpstreamModelName: "gemini-test"}, RelayFormat: types.RelayFormatOpenAI}
					require.Nil(t, PrepareGeminiFileAttempt(c, info))
					var result *dto.GeminiChatRequest
					var meta *types.TokenCountMeta
					if native {
						var request dto.GeminiChatRequest
						require.NoError(t, common.UnmarshalJsonStr(body, &request))
						meta = request.GetTokenCountMeta()
						require.NoError(t, ApplyGeminiFilePlan(c, &request))
						result = &request
						assert.JSONEq(t, `{"fps":1}`, string(result.Contents[0].Parts[1].VideoMetadata))
					} else {
						var request dto.GeneralOpenAIRequest
						require.NoError(t, common.UnmarshalJsonStr(body, &request))
						meta = request.GetTokenCountMeta()
						converted, err := ConvertRequest(c, info, types.RelayFormatGemini, &request)
						require.NoError(t, err)
						result = converted.Value.(*dto.GeminiChatRequest)
					}
					require.Len(t, result.Contents, 1)
					require.Len(t, result.Contents[0].Parts, 2)
					require.NotNil(t, result.Contents[0].Parts[1].InlineData)
					assert.Equal(t, encoded, result.Contents[0].Parts[1].InlineData.Data)
					require.Len(t, meta.Files, 1)
					tokens, err := EstimateRequestToken(c, meta, info)
					require.NoError(t, err)
					assert.Positive(t, tokens)
					redacted := RedactGeminiFileRequest(c, body)
					assert.NotContains(t, redacted, encoded)
					plan.Close(c)
				})
			}
		}
	}
}

func TestGeminiInlineFilesStillRespectBodyLimit(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"mode":"process","chat_model_patterns":["gemini-test"],"inline_threshold_bytes":8,"max_inline_request_bytes":160}`)
	encoded := base64.StdEncoding.EncodeToString(append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{'x'}, 40)...))
	part := fmt.Sprintf(`{"inlineData":{"mimeType":"application/pdf","data":%q}}`, encoded)
	body := `{"contents":[{"parts":[` + part + `,` + part + `]}]}`
	c := geminiFileTestContext(t, "/v1/models/gemini-test:streamGenerateContent", body)
	plan, err := PlanGeminiFiles(c, "gemini-test")
	require.Nil(t, err)
	require.NotNil(t, plan)
	assert.False(t, plan.RequiresStorage, "aggregate size and base64 length must not trigger GCS")
	_, err = FinalizeGeminiFileBody(c, []byte(body), nil, false)
	require.NotNil(t, err)
	assert.Equal(t, 413, err.StatusCode)
	assert.False(t, plan.RequiresStorage)
	assert.Empty(t, plan.objects)
	plan.RequiresStorage = true
	plan.Config.Mode = "forward"
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{"Authorization": "{client_header:Authorization}"}}}
	require.NotNil(t, ValidateGeminiFileForwardingHeaders(c, info), "runtime overrides cannot substitute the caller credential")
}

func TestGeminiFileMIMEPreservesDeclaredType(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"chat_model_patterns":["gemini-test"]}`)
	for _, tc := range []struct {
		name, declared, data, want string
	}{
		{"different image signature", "image/jpeg", "\x89PNG\r\n\x1a\n", "image/jpeg"},
		{"AAC signature detected as MPEG", "audio/x-aac", "\xff\xf1\x50\x80", "audio/x-aac"},
		{"unrecognized signature", "application/pdf", "\x00\x01\x02\x03", "application/pdf"},
		{"MIME alias", "audio/x-wav", "\x00\x01\x02\x03", "audio/wav"},
		{"generic MIME falls back to signature", "application/octet-stream", "%PDF-1.7\n", "application/pdf"},
		{"missing MIME falls back to signature", "", "\x89PNG\r\n\x1a\n", "image/png"},
	} {
		for _, native := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/native=%t", tc.name, native), func(t *testing.T) {
				encoded := base64.StdEncoding.EncodeToString([]byte(tc.data))
				path := "/v1/models/gemini-test:generateContent"
				body := fmt.Sprintf(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":%q,"data":%q}}]}]}`, tc.declared, encoded)
				if !native {
					path = "/v1/chat/completions"
					body = fmt.Sprintf(`{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"file","file":{"file_data":%q}}]}]}`, "data:"+tc.declared+";base64,"+encoded)
				}
				c := geminiFileTestContext(t, path, body)
				plan, apiErr := PlanGeminiFiles(c, "gemini-test")
				require.Nil(t, apiErr)
				require.NotNil(t, plan)
				assert.False(t, plan.RequiresStorage)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, UpstreamModelName: "gemini-test"}, RelayFormat: types.RelayFormatOpenAI}
				require.Nil(t, PrepareGeminiFileAttempt(c, info))
				var result dto.GeminiChatRequest
				if native {
					require.NoError(t, common.UnmarshalJsonStr(body, &result))
					require.NoError(t, ApplyGeminiFilePlan(c, &result))
				} else {
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					converted, err := ConvertRequest(c, info, types.RelayFormatGemini, &request)
					require.NoError(t, err)
					result = *converted.Value.(*dto.GeminiChatRequest)
				}
				require.Len(t, result.Contents, 1)
				require.Len(t, result.Contents[0].Parts, 1)
				require.NotNil(t, result.Contents[0].Parts[0].InlineData)
				assert.Equal(t, tc.want, result.Contents[0].Parts[0].InlineData.MimeType)
				assert.Equal(t, encoded, result.Contents[0].Parts[0].InlineData.Data)
			})
		}
	}
}

func TestGeminiFileURLMIMEFallback(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().AllowPrivateIp = true
	for _, tc := range []struct {
		name, declared, received, data, want string
	}{
		{"declaration takes precedence", "image/jpeg", "image/png", "\x89PNG\r\n\x1a\n", "image/jpeg"},
		{"response MIME takes precedence over signature", "", "audio/x-aac", "\xff\xf1\x50\x80", "audio/x-aac"},
		{"generic declaration uses response MIME", "application/octet-stream", "audio/x-wav", "\x00\x01\x02\x03", "audio/wav"},
		{"generic response falls back to signature", "", "application/octet-stream", "%PDF-1.7\n", "application/pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.received)
				_, _ = w.Write([]byte(tc.data))
			}))
			defer source.Close()
			parsed, err := url.Parse(source.URL)
			require.NoError(t, err)
			system_setting.GetFetchSetting().AllowedPorts = []string{parsed.Port()}
			plan := &GeminiFilePlan{Config: model_setting.GeminiFileRouting{InlineThresholdBytes: 64}, client: source.Client()}
			file := &GeminiPlannedFile{Source: types.NewURLFileSource(source.URL), MIMEType: tc.declared}
			require.Nil(t, plan.probe(context.Background(), file))
			require.NotNil(t, file.resolved)
			assert.Equal(t, tc.want, file.resolved.MIMEType)
			assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(tc.data)), file.resolved.Base64)
		})
	}
}

func TestGeminiFilePlanRejectsInvalidOrUnsupportedAttachments(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"chat_model_patterns":["gemini-test"]}`)
	cases := []struct{ name, path, body, model string }{
		{"unsupported declared MIME", "/v1/models/gemini-test:generateContent", `{"contents":[{"parts":[{"inlineData":{"mimeType":"application/zip","data":"JVBERi0xLjcK"}}]}]}`, "gemini-test"},
		{"unrecognized file without MIME", "/v1/models/gemini-test:generateContent", `{"contents":[{"parts":[{"inlineData":{"data":"AAECAw=="}}]}]}`, "gemini-test"},
		{"invalid PDF structure", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":"https://example.com/file"}]}]}`, "gemini-test"},
		{"disabled model PDF", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"https://example.com/file"}}]}]}`, "other"},
		{"role would drop attachment", "/v1/chat/completions", `{"messages":[{"role":"system","content":[{"type":"pdf_url","pdf_url":{"url":"https://example.com/file"}}]}]}`, "gemini-test"},
		{"PDF disguised as image", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":"data:application/pdf;base64,JVBERi0xLjcK"}]}]}`, "gemini-test"},
		{"conflicting data URI MIME", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}]}`, "gemini-test"},
		{"private source", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"https://127.0.0.1/private?signature=secret"}}]}]}`, "gemini-test"},
		{"unsafe URL scheme", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"http://example.com/file"}}]}]}`, "gemini-test"},
		{"conflicting URI aliases", "/v1beta/models/gemini-test:generateContent", `{"contents":[{"parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"","file_uri":"https://example.com/file"}}]}]}`, "gemini-test"},
		{"cross site GCS identity", "/v1beta/models/gemini-test:generateContent", `{"contents":[{"parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"gs://bucket/object"}}]}]}`, "gemini-test"},
		{"message field casing bypass", "/v1/chat/completions", `{"messages":[{"role":"user","Content":[{"type":"pdf_url","pdf_url":{"url":"https://example.com/file"}}]}]}`, "gemini-test"},
		{"root field casing bypass", "/v1/chat/completions", `{"messages":[],"Messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"https://example.com/file"}}]}]}`, "gemini-test"},
		{"inline data cannot reuse a URL probe", "/v1/models/gemini-test:generateContent", `{"contents":[{"parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"https://generativelanguage.googleapis.com/v1beta/files/test"}},{"inlineData":{"mimeType":"application/pdf","data":"https://generativelanguage.googleapis.com/v1beta/files/test"}}]}]}`, "gemini-test"},
	}
	cases = append(cases, struct{ name, path, body, model string }{"duplicate JSON attachments", "/v1/models/gemini-test:generateContent", `{"contents":[{"parts":[{"fileData":{"fileUri":"first","fileUri":"second","mimeType":"application/pdf"}}]}]}`, "gemini-test"})
	invalidTail := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\n"+strings.Repeat("x", 640))) + "!"
	cases = append(cases, struct{ name, path, body, model string }{"invalid base64 after MIME prefix", "/v1/models/gemini-test:generateContent", fmt.Sprintf(`{"contents":[{"parts":[{"inlineData":{"mimeType":"application/pdf","data":%q}}]}]}`, invalidTail), "gemini-test"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := geminiFileTestContext(t, tc.path, tc.body)
			plan, err := PlanGeminiFiles(c, tc.model)
			require.NotNil(t, err)
			assert.Contains(t, []int{400, 403}, err.StatusCode)
			assert.NotContains(t, err.Error(), "signature=secret")
			if plan != nil {
				plan.Close(c)
			}
		})
	}
}

func TestGeminiFileURLThreshold(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().AllowPrivateIp = true
	for _, fixture := range []struct{ mime, prefix string }{
		{"image/png", "\x89PNG\r\n\x1a\n"},
		{"audio/wav", "RIFF\x00\x00\x00\x00WAVE"},
		{"video/mp4", "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"},
		{"application/pdf", "%PDF-1.7\n"},
	} {
		for _, size := range []int{63, 64, 65} {
			t.Run(fmt.Sprintf("%s/%d", fixture.mime, size), func(t *testing.T) {
				data := []byte(fixture.prefix + strings.Repeat("x", size-len(fixture.prefix)))
				source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", fixture.mime)
					_, _ = w.Write(data)
				}))
				defer source.Close()
				parsed, err := url.Parse(source.URL)
				require.NoError(t, err)
				system_setting.GetFetchSetting().AllowedPorts = []string{parsed.Port()}
				plan := &GeminiFilePlan{Config: model_setting.GeminiFileRouting{InlineThresholdBytes: 64}, client: source.Client()}
				file := &GeminiPlannedFile{Source: types.NewURLFileSource(source.URL), MIMEType: fixture.mime}
				require.Nil(t, plan.probe(context.Background(), file))
				assert.Equal(t, size >= 64, file.Large)
				if size < 64 {
					require.NotNil(t, file.resolved)
					assert.Equal(t, base64.StdEncoding.EncodeToString(data), file.resolved.Base64)
				} else {
					assert.Nil(t, file.resolved, "large URLs must await forwarding or upload")
				}
			})
		}
	}
}

func TestGeminiFileURLProbeIsBoundedAndDetectsChanges(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().AllowPrivateIp = true
	var requests int
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("ETag", fmt.Sprintf("version%d", requests))
		_, _ = w.Write(append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{'x'}, 1024)...))
	}))
	defer source.Close()
	parsed, err := url.Parse(source.URL)
	require.NoError(t, err)
	system_setting.GetFetchSetting().AllowedPorts = []string{parsed.Port()}
	config, err := model_setting.ParseGeminiFileRouting(`{"inline_threshold_bytes":64}`)
	require.NoError(t, err)
	plan := &GeminiFilePlan{Config: config, client: source.Client()}
	file := &GeminiPlannedFile{Source: types.NewURLFileSource(source.URL + "?signature=secret"), MIMEType: "application/pdf", Position: "contents[0].parts[0]"}
	require.Nil(t, plan.probe(context.Background(), file))
	assert.True(t, file.Large)
	assert.Equal(t, int64(64), plan.probeBytes)
	assert.Nil(t, file.resolved)
	assert.Equal(t, 1, requests)
	plan.Files = []*GeminiPlannedFile{file}
	c := geminiFileTestContext(t, "/v1/chat/completions", `{}`)
	c.Set(geminiFilePlanKey, plan)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, UpstreamModelName: "gemini-test"}}
	plan.RequiresStorage = true
	require.Nil(t, PrepareGeminiFileAttempt(c, info))
	resolved, err := plan.MediaForPosition(file.Source, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, file.Source.GetRawData(), resolved.URI)
	assert.Equal(t, 1, requests, "forward preparation must not download the remaining file")
	_, transferErr := plan.upload(context.Background(), c, nil, info, file, maxGeminiFileBytes)
	require.NotNil(t, transferErr)
	assert.Equal(t, 400, transferErr.StatusCode)
	assert.Contains(t, transferErr.Error(), "changed after planning")
}

func TestGeminiFileGCSLifecycleAndPermissionDomain(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"mode":"process","inline_threshold_bytes":8}`)
	var mu sync.Mutex
	uploads := map[string][]byte{}
	deleted := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if !assert.NoError(t, err) {
				w.WriteHeader(500)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			metadata, err := reader.NextPart()
			if !assert.NoError(t, err) {
				w.WriteHeader(500)
				return
			}
			raw, err := io.ReadAll(metadata)
			assert.NoError(t, err)
			part, err := reader.NextPart()
			if !assert.NoError(t, err) {
				w.WriteHeader(500)
				return
			}
			data, err := io.ReadAll(part)
			assert.NoError(t, err)
			name := gjson.GetBytes(raw, "name").String()
			assert.Equal(t, "42", gjson.GetBytes(raw, "metadata.new_api_user_id").String())
			uploads[name] = data
			_, _ = fmt.Fprintf(w, `{"name":%q,"bucket":"test-bucket","generation":"1","size":%q}`, name, strconv.Itoa(len(data)))
		case http.MethodDelete:
			name := strings.SplitN(r.URL.Path, "/o/", 2)[1]
			deleted[name] = true
			w.WriteHeader(204)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"size":"20","contentType":"application/pdf","metadata":{"new_api_user_id":"999"}}`)
		default:
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL)
	c := geminiFileTestContext(t, "/v1beta/models/gemini-test:generateContent", `{"contents":[{"parts":[{"inlineData":{"mimeType":"application/pdf","data":"JVBERi0xLjcK"}}]}]}`)
	plan, apiErr := PlanGeminiFiles(c, "gemini-test")
	require.Nil(t, apiErr)
	info := &relaycommon.RelayInfo{UserId: 42, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVertexAi, UpstreamModelName: "gemini-test", ApiKey: `{"type":"service_account","project_id":"test"}`, ChannelSetting: dto.ChannelSettings{GoogleFileBucket: "test-bucket"}}}
	require.Nil(t, PrepareGeminiFileAttempt(c, info))
	assert.False(t, plan.RequiresStorage)
	assert.Empty(t, uploads, "base64 above the URL threshold must remain inline in process mode")
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7\nURL attachment")
	}))
	defer source.Close()
	parsed, err := url.Parse(source.URL)
	require.NoError(t, err)
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{parsed.Port()}
	plan.client.CloseIdleConnections()
	plan.client = source.Client()
	file := &GeminiPlannedFile{PartIndex: 1, Position: "contents[0].parts[1]", Source: types.NewURLFileSource(source.URL), MIMEType: "application/pdf"}
	require.Nil(t, plan.probe(c.Request.Context(), file))
	require.True(t, file.Large)
	plan.Files = append(plan.Files, file)
	plan.RequiresStorage = file.Large
	require.Nil(t, PrepareGeminiFileAttempt(c, info))
	require.Nil(t, PrepareGeminiFileAttempt(c, info))
	assert.Len(t, uploads, 1, "retry with the same selected key must reuse the object")
	info.ApiKey = `{"type":"service_account","project_id":"another"}`
	require.Nil(t, PrepareGeminiFileAttempt(c, info))
	assert.Len(t, uploads, 2, "another credential must upload its own object")
	for _, data := range uploads {
		assert.Equal(t, "%PDF-1.7\nURL attachment", string(data), "only the URL attachment is uploaded")
	}
	request := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{
		{InlineData: &dto.GeminiInlineData{MimeType: "application/pdf", Data: "JVBERi0xLjcK"}},
		{FileData: &dto.GeminiFileData{MimeType: "application/pdf", FileUri: source.URL}},
	}}}}
	require.NoError(t, ApplyGeminiFilePlan(c, request))
	require.NotNil(t, request.Contents[0].Parts[0].InlineData)
	assert.Equal(t, "JVBERi0xLjcK", request.Contents[0].Parts[0].InlineData.Data)
	require.NotNil(t, request.Contents[0].Parts[1].FileData)
	assert.True(t, strings.HasPrefix(request.Contents[0].Parts[1].FileData.FileUri, "gs://test-bucket/"))
	plan.Close(c)
	assert.Len(t, deleted, 2)
	plan.Close(c)
	assert.Len(t, deleted, 2)
	existing := geminiFileTestContext(t, "/v1/models/gemini-test:generateContent", `{"contents":[{"parts":[{"file_data":{"mime_type":"application/pdf","file_uri":"gs://test-bucket/private"}}]}]}`)
	_, apiErr = PlanGeminiFiles(existing, "gemini-test")
	require.Nil(t, apiErr)
	apiErr = PrepareGeminiFileAttempt(existing, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 403, apiErr.StatusCode)
	GetGeminiFilePlan(existing).Close(existing)
	assert.Len(t, deleted, 2, "never delete a client-owned reference")
}

func TestGeminiFilePassthroughAndOverrides(t *testing.T) {
	configureGeminiFileTest(t, `{"enabled":true,"mode":"forward"}`)
	body := `{"contents":[{"parts":[{"inline_data":{"mime_type":"application/pdf","data":"JVBERi0xLjcK"},"videoMetadata":{"fps":1}}]}],"customProviderField":{"keep":true}}`
	c := geminiFileTestContext(t, "/v1/models/gemini-test:generateContent", body)
	_, err := PlanGeminiFiles(c, "gemini-test")
	require.Nil(t, err)
	var request dto.GeminiChatRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	require.NoError(t, ApplyGeminiFilePlan(c, &request))
	serialized, marshalErr := common.Marshal(request)
	require.NoError(t, marshalErr)
	var reordered map[string]any
	require.NoError(t, common.Unmarshal(serialized, &reordered))
	reformatted, marshalErr := common.Marshal(reordered)
	require.NoError(t, marshalErr)
	_, err = FinalizeGeminiFileBody(c, reformatted, serialized, false)
	require.Nil(t, err, "JSON object key order must not invalidate an unchanged attachment")
	result, err := FinalizeGeminiFileBody(c, serialized, serialized, true)
	require.Nil(t, err)
	assert.True(t, gjson.GetBytes(result, "customProviderField.keep").Bool())
	assert.True(t, gjson.GetBytes(result, "contents.0.parts.0.inlineData").Exists())
	assert.False(t, gjson.GetBytes(result, "contents.0.parts.0.inline_data").Exists())
	changed, mutationErr := sjson.SetBytes(serialized, "contents.0.parts.0.fileData", map[string]string{"fileUri": "https://example.com/unplanned"})
	require.NoError(t, mutationErr)
	_, err = FinalizeGeminiFileBody(c, changed, serialized, false)
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
}

func TestGeminiFileSubmittedAttemptStopsRetryAndRecordsFailure(t *testing.T) {
	c := geminiFileTestContext(t, "/v1/chat/completions", `{}`)
	plan := &GeminiFilePlan{}
	c.Set(geminiFilePlanKey, plan)
	apiErr := types.NewError(fmt.Errorf("no available key"), types.ErrorCodeChannelNoAvailableKey)
	assert.True(t, ShouldRetryRelayError(c, apiErr, 1), "an unsubmitted attempt retains normal retry behavior")
	plan.Submitted = true
	decision := DecideRelayRetry(c, apiErr, 1)
	assert.Equal(t, PolicyDecision{Action: "stop", Reason: "file_inference_submitted", Source: "file_routing"}, decision)
	assert.False(t, ShouldRetryRelayError(c, apiErr, 1))
	RecordPolicyFailure(c, 7, apiErr, decision)
	events := RequestPolicy(c).Events()
	require.Len(t, events, 2)
	assert.Equal(t, "failure", events[0].Decision.Action)
	assert.Equal(t, decision, events[1].Decision)
}
