package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiChatThoughtSignaturesRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name               string
		stream             bool
		parts              []string
		wantSignature      string
		wantToolSignatures []string
		wantCities         []string
	}{
		{
			name:               "JSON parallel tools preserve signed and unsigned calls",
			parts:              []string{`[{"functionCall":{"name":"get_weather","args":{"city":"上海"}},"thoughtSignature":"sig+/Shanghai=="},{"functionCall":{"name":"get_weather","args":{"city":"杭州"}}}]`},
			wantToolSignatures: []string{"sig+/Shanghai==", ""},
			wantCities:         []string{"上海", "杭州"},
		},
		{
			name:          "JSON text signature",
			parts:         []string{`[{"text":"晴天","thoughtSignature":"sig+/text=="}]`},
			wantSignature: "sig+/text==",
		},
		{
			name:               "SSE first frame tool signature",
			stream:             true,
			parts:              []string{`[{"functionCall":{"name":"get_weather","args":{"city":"上海"}},"thoughtSignature":"sig+/first=="}]`},
			wantToolSignatures: []string{"sig+/first=="},
			wantCities:         []string{"上海"},
		},
		{
			name:   "SSE successive tool calls keep separate indexes and signatures",
			stream: true,
			parts: []string{
				`[{"thought":true,"text":"正在查询"}]`,
				`[{"functionCall":{"id":"weather-sh","name":"get_weather","args":{"city":"上海"}},"thoughtSignature":"sig+/Shanghai=="}]`,
				`[{"functionCall":{"id":"weather-hz","name":"get_weather","args":{"city":"杭州"}},"thoughtSignature":"sig+/Hangzhou=="}]`,
			},
			wantToolSignatures: []string{"sig+/Shanghai==", "sig+/Hangzhou=="},
			wantCities:         []string{"上海", "杭州"},
		},
		{
			name:               "SSE parallel calls do not copy a signature onto the unsigned call",
			stream:             true,
			parts:              []string{`[{"functionCall":{"name":"get_weather","args":{"city":"上海"}},"thoughtSignature":"sig+/Shanghai=="},{"functionCall":{"name":"get_weather","args":{"city":"杭州"}}}]`},
			wantToolSignatures: []string{"sig+/Shanghai==", ""},
			wantCities:         []string{"上海", "杭州"},
		},
		{
			name:   "SSE signature-only part stays separate from the preceding tool",
			stream: true,
			parts: []string{
				`[{"functionCall":{"name":"get_weather","args":{"city":"上海"}}}]`,
				`[{"thoughtSignature":"sig+/late=="}]`,
			},
			wantSignature:      "sig+/late==",
			wantToolSignatures: []string{""},
			wantCities:         []string{"上海"},
		},
		{
			name:   "SSE partial arguments retain the first signature",
			stream: true,
			parts: []string{
				`[{"functionCall":{"id":"weather-sh","name":"get_weather","partialArgs":[{"jsonPath":"$.city","stringValue":"上"}],"willContinue":true},"thoughtSignature":"sig+/partial=="}]`,
				`[{"functionCall":{"id":"weather-sh","partialArgs":[{"jsonPath":"$.city","stringValue":"海"}],"willContinue":false}}]`,
			},
			wantToolSignatures: []string{"sig+/partial=="},
			wantCities:         []string{"上海"},
		},
		{
			name:   "SSE final partial argument frame carries its signature",
			stream: true,
			parts: []string{
				`[{"functionCall":{"name":"get_weather","partialArgs":[{"jsonPath":"$.city","stringValue":"上"}],"willContinue":true}}]`,
				`[{"functionCall":{"partialArgs":[{"jsonPath":"$.city","stringValue":"海"}],"willContinue":false},"thoughtSignature":"sig+/pending=="}]`,
			},
			wantToolSignatures: []string{"sig+/pending=="},
			wantCities:         []string{"上海"},
		},
		{
			name:          "SSE text signature-only final part",
			stream:        true,
			parts:         []string{`[{"text":"晴天"}]`, `[{"thoughtSignature":"sig+/text=="}]`},
			wantSignature: "sig+/text==",
		},
		{
			name:  "unsigned text omits provider metadata",
			parts: []string{`[{"text":"晴天"}]`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set(common.RequestIdKey, "gemini-signature-test")
			info := &relaycommon.RelayInfo{
				RelayFormat:     types.RelayFormatOpenAI,
				OriginModelName: "gemini-4-argon",
				IsStream:        tt.stream,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeGemini,
					UpstreamModelName: "gemini-4-argon",
				},
			}
			info.SetEstimatePromptTokens(99)
			var body strings.Builder
			for _, parts := range tt.parts {
				var upstream dto.GeminiChatResponse
				require.NoError(t, common.UnmarshalJsonStr(`{"candidates":[{"content":{"role":"model","parts":`+parts+`}}]}`, &upstream))
				upstream.UsageMetadata = dto.GeminiUsageMetadata{PromptTokenCount: 2, CandidatesTokenCount: 3, ThoughtsTokenCount: 1, TotalTokenCount: 6}
				data, err := common.Marshal(upstream)
				require.NoError(t, err)
				if tt.stream {
					fmt.Fprintf(&body, "data: %s\n\n", data)
				} else {
					body.Write(data)
					// Exercise the standalone converter as well as the host's
					// separate HTTP JSON response conversion below.
					converted, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &upstream)
					require.NoError(t, err)
					message := converted.Value.(*dto.OpenAITextResponse).Choices[0].Message
					assertGeminiSignatureExtraContent(t, tt.wantSignature, message.ExtraContent)
					calls := message.ParseToolCalls()
					require.Len(t, calls, len(tt.wantToolSignatures))
					for i, call := range calls {
						assertGeminiSignatureExtraContent(t, tt.wantToolSignatures[i], call.ExtraContent)
					}
				}
			}
			if tt.stream {
				body.WriteString("data: {\"candidates\":[{\"finishReason\":\"STOP\",\"content\":{\"role\":\"model\",\"parts\":[]}}]}\n\ndata: [DONE]\n\n")
			}
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body.String()))}
			var apiErr *types.NewAPIError
			if tt.stream {
				_, apiErr = GeminiChatStreamHandler(c, info, resp)
			} else {
				_, apiErr = GeminiChatHandler(c, info, resp)
			}
			require.Nil(t, apiErr)
			if tt.wantSignature != "" || slices.ContainsFunc(tt.wantToolSignatures, func(signature string) bool { return signature != "" }) {
				assert.Contains(t, recorder.Body.String(), `"extra_content":{"google":{"thought_signature":`)
			} else {
				assert.NotContains(t, recorder.Body.String(), `"extra_content"`)
			}
			message := dto.Message{Role: "assistant"}
			if tt.stream {
				calls := make(map[int]dto.ToolCallResponse)
				text := ""
				finishes := 0
				for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
					data, ok := strings.CutPrefix(line, "data: ")
					if !ok || data == "[DONE]" {
						continue
					}
					var chunk dto.ChatCompletionsStreamResponse
					require.NoError(t, common.UnmarshalJsonStr(data, &chunk))
					if chunk.Usage != nil {
						assert.Equal(t, 2, chunk.Usage.PromptTokens)
						assert.Equal(t, 4, chunk.Usage.CompletionTokens)
						assert.Equal(t, 6, chunk.Usage.TotalTokens)
					}
					for _, choice := range chunk.Choices {
						if choice.FinishReason != nil && *choice.FinishReason != "" {
							finishes++
						}
						text += choice.Delta.GetContentString()
						if choice.Delta.ExtraContent != nil {
							message.ExtraContent = choice.Delta.ExtraContent
						}
						for _, delta := range choice.Delta.ToolCalls {
							require.NotNil(t, delta.Index)
							call := calls[*delta.Index]
							call.ID += delta.ID
							call.Type = "function"
							call.Function.Name += delta.Function.Name
							call.Function.Arguments += delta.Function.Arguments
							if delta.ExtraContent != nil {
								call.ExtraContent = delta.ExtraContent
							}
							calls[*delta.Index] = call
						}
					}
				}
				assert.Equal(t, 1, finishes)
				message.Content = text
				ordered := make([]dto.ToolCallResponse, len(calls))
				for i := range ordered {
					ordered[i] = calls[i]
				}
				message.SetToolCalls(ordered)
			} else {
				var response dto.OpenAITextResponse
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				require.Len(t, response.Choices, 1)
				message = response.Choices[0].Message
			}

			calls := message.ParseToolCalls()
			require.Len(t, calls, len(tt.wantToolSignatures))
			for i, call := range calls {
				require.NotEmpty(t, call.ID)
				assert.Equal(t, "get_weather", call.Function.Name)
				assert.JSONEq(t, fmt.Sprintf(`{"city":%q}`, tt.wantCities[i]), call.Function.Arguments)
				assertGeminiSignatureExtraContent(t, tt.wantToolSignatures[i], call.ExtraContent)
			}
			assertGeminiSignatureExtraContent(t, tt.wantSignature, message.ExtraContent)

			// Replay the actual JSON/SSE result, including its provider fields,
			// with both settings for the legacy signature fallback.
			for _, fallbackEnabled := range []bool{false, true} {
				request := dto.GeneralOpenAIRequest{
					Model:    "gemini-4-argon",
					Messages: []dto.Message{{Role: "user", Content: "查询天气"}, message},
				}
				for _, call := range calls {
					request.Messages = append(request.Messages, dto.Message{Role: "tool", ToolCallId: call.ID, Content: `{"weather":"晴天"}`})
				}
				wire, err := common.Marshal(request)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(wire, &request))
				converted, err := relayconvert.ConvertRequest(c, &convmeta.Values{
					ChannelMetaAttached: true,
					UpstreamModelName:   "gemini-4-argon",
					Options:             &convmeta.Options{Gemini: convmeta.GeminiOptions{FunctionCallThoughtSignatureEnabled: fallbackEnabled}},
				}, types.RelayFormatGemini, &request)
				require.NoError(t, err)
				gemini := converted.Value.(*dto.GeminiChatRequest)
				require.GreaterOrEqual(t, len(gemini.Contents), 2)
				parts := gemini.Contents[1].Parts
				for i, signature := range tt.wantToolSignatures {
					if signature == "" {
						assert.Empty(t, parts[i].ThoughtSignature)
					} else {
						assert.JSONEq(t, fmt.Sprintf(`%q`, signature), string(parts[i].ThoughtSignature))
					}
				}
				if tt.wantSignature != "" {
					assert.JSONEq(t, fmt.Sprintf(`%q`, tt.wantSignature), string(parts[len(parts)-1].ThoughtSignature))
				}
			}
		})
	}
}

// Assert the public Google metadata contract, including omission when upstream
// did not sign this message or call. Do not infer success from DTO fields alone.
func assertGeminiSignatureExtraContent(t *testing.T, signature string, content *dto.OpenAIExtraContent) {
	t.Helper()
	if signature == "" {
		assert.Nil(t, content)
		return
	}
	data, err := common.Marshal(content)
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{"google":{"thought_signature":%q}}`, signature), string(data))
}
