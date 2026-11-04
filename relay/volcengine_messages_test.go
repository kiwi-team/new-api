package relay

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestVolcengineMessagesRouting(t *testing.T) {
	for _, tc := range []struct {
		name, base, model, path string
		format                  types.RelayFormat
		mode                    int
		nativeHeader, nativeDTO bool
	}{
		{name: "default host", model: "doubao-seed-2-1-pro", format: types.RelayFormatClaude, path: "https://ark.cn-beijing.volces.com/api/compatible/v1/messages", nativeHeader: true, nativeDTO: true},
		{name: "DeepSeek", base: "https://ark.example/", model: "deepseek-v4-chat", format: types.RelayFormatClaude, path: "https://ark.example/api/compatible/v1/messages", nativeHeader: true, nativeDTO: true},
		{name: "mapped endpoint", base: "https://proxy.example/prefix/", model: "ep-model", format: types.RelayFormatClaude, path: "https://proxy.example/prefix/api/compatible/v1/messages", nativeHeader: true, nativeDTO: true},
		{name: "coding plan before bot", base: "doubao-coding-plan", model: "bot-model", format: types.RelayFormatClaude, path: "https://ark.cn-beijing.volces.com/api/coding/v1/messages", nativeDTO: true},
		{name: "bots conversion", base: "https://ark.example", model: "bot-model", format: types.RelayFormatClaude, path: "https://ark.example/api/v3/bots/chat/completions"},
		{name: "Responses policy", base: "https://ark.example", model: "doubao-seed-2-1-pro", format: types.RelayFormatClaude, mode: relayconstant.RelayModeResponses, path: "https://ark.example/api/v3/responses"},
		{name: "chat", base: "https://ark.example", model: "doubao-seed-2-1-pro", format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "https://ark.example/api/v3/chat/completions"},
		{name: "chat coding plan", base: "doubao-coding-plan", model: "model", format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, path: "https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions"},
		{name: "embedding", base: "https://ark.example", model: "embedding-model", format: types.RelayFormatEmbedding, mode: relayconstant.RelayModeEmbeddings, path: "https://ark.example/api/v3/embeddings"},
		{name: "image", base: "https://ark.example", model: "seedream-model", format: types.RelayFormatOpenAIImage, mode: relayconstant.RelayModeImagesGenerations, path: "https://ark.example/api/v3/images/generations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, info, _ := newVolcengineMessagesContext(t, tc.base, `{"model":"model","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`)
			info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVolcEngine, ChannelBaseUrl: tc.base, UpstreamModelName: tc.model, ApiKey: "channel-key"}
			info.RelayFormat, info.RelayMode = tc.format, tc.mode
			adaptor := &volcengine.Adaptor{}
			url, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tc.path, url)
			headers := http.Header{}
			require.NoError(t, adaptor.SetupRequestHeader(c, &headers, info))
			if tc.nativeHeader {
				assert.Equal(t, "channel-key", headers.Get("x-api-key"))
				assert.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
				assert.Empty(t, headers.Get("Authorization"))
			} else {
				assert.Equal(t, "Bearer channel-key", headers.Get("Authorization"))
				assert.Empty(t, headers.Get("x-api-key"))
			}
			if tc.format == types.RelayFormatClaude && tc.mode == relayconstant.RelayModeUnknown {
				request := info.Request.(*dto.ClaudeRequest)
				request.Model = tc.model
				converted, err := adaptor.ConvertClaudeRequest(c, info, request)
				require.NoError(t, err)
				_, native := converted.(*dto.ClaudeRequest)
				assert.Equal(t, tc.nativeDTO, native)
			}
		})
	}
}

func TestVolcengineMessagesNativeParameters(t *testing.T) {
	for _, tc := range []struct{ name, controls string }{
		{name: "omitted"},
		{name: "explicit zero", controls: `,"max_tokens":0,"temperature":0,"top_p":0,"top_k":0,"stream":false`},
		{name: "native thinking", controls: `,"max_tokens":64,"thinking":{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"},"future_field":true},"output_config":{"effort":"high","format":{"type":"json_schema","schema":{"type":"object"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"deepseek-v4-chat","messages":[{"role":"user","content":"hello"}]` + tc.controls + `}`
			c, info, _ := newVolcengineMessagesContext(t, "https://ark.example", body)
			info.InitChannelMeta(c)
			converted, err := (&volcengine.Adaptor{}).ConvertClaudeRequest(c, info, info.Request.(*dto.ClaudeRequest))
			require.NoError(t, err)
			wire, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.JSONEq(t, body, string(wire))
		})
	}
	for _, tc := range []struct{ modifier, want string }{
		{modifier: "@thinking:off", want: "disabled"},
		{modifier: "@thinking:on", want: "enabled"},
	} {
		t.Run(tc.modifier, func(t *testing.T) {
			body := `{"model":"deepseek-v4-chat` + tc.modifier + `","max_tokens":4096,"messages":[{"role":"user","content":"hello"}]}`
			c, info, _ := newVolcengineMessagesContext(t, "https://ark.example", body)
			info.InitChannelMeta(c)
			outbound, err := common.DeepCopy(info.Request.(*dto.ClaudeRequest))
			require.NoError(t, err)
			require.NoError(t, helper.ApplyReasoningModelSuffix(c, info, outbound))
			converted, err := (&volcengine.Adaptor{}).ConvertClaudeRequest(c, info, outbound)
			require.NoError(t, err)
			request := converted.(*dto.ClaudeRequest)
			assert.Equal(t, "deepseek-v4-chat", request.Model)
			require.NotNil(t, request.Thinking)
			assert.Equal(t, tc.want, request.Thinking.Type)
		})
	}
}

func TestVolcengineMessagesDatabaseMatrix(t *testing.T) {
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	settings := model_setting.GetGlobalSettings()
	previousSettings := *settings
	settings.PassThroughRequestEnabled = false
	settings.ChatCompletionsToResponsesPolicy.Enabled = false
	t.Cleanup(func() { *settings = previousSettings })
	service.InitHttpClient()
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db, logDB := setupVolcengineMessagesDatabases(t, dialect)
			for index, tc := range []struct {
				name                        string
				stream, cached, passThrough bool
				override, responses, bot    bool
			}{
				{name: "JSON"},
				{name: "SSE", stream: true},
				{name: "fully cached JSON", cached: true},
				{name: "fully cached SSE", stream: true, cached: true},
				{name: "raw JSON", passThrough: true},
				{name: "raw SSE", stream: true, passThrough: true},
				{name: "parameter and header overrides", override: true},
				{name: "Responses policy", responses: true},
				{name: "Bots conversion", bot: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					input, expectedQuota := 10, 28 // 10 input + 4 output * 2 + 20 cache reads * 0.5.
					if tc.cached {
						input, expectedQuota = 0, 18
					}
					response := fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"ep-upstream","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","provider_extension":{"preserved":true},"usage":{"input_tokens":%d,"output_tokens":4,"cache_read_input_tokens":20,"cache_creation_input_tokens":0}}`, input)
					contentType := gin.MIMEJSON
					if tc.stream {
						contentType = "text/event-stream"
						response = volcengineMessagesStream(input)
					}
					if tc.responses {
						response = `{"id":"resp_1","object":"response","status":"completed","model":"ep-upstream","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":30,"output_tokens":4,"total_tokens":34,"input_tokens_details":{"cached_tokens":20}}}`
					}
					if tc.bot {
						response = `{"id":"chatcmpl_1","object":"chat.completion","model":"bot-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":4,"total_tokens":34,"prompt_tokens_details":{"cached_tokens":20}}}`
					}
					captured := make(chan *http.Request, 1)
					capturedBody := make(chan string, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						captured <- r.Clone(r.Context())
						capturedBody <- string(body)
						w.Header().Set("Content-Type", contentType)
						w.Header().Set("X-Tt-Logid", "volc-request-id")
						_, _ = io.WriteString(w, response)
					}))
					t.Cleanup(upstream.Close)
					requestModel, mappedModel := "public-model", "ep-upstream"
					if tc.bot {
						mappedModel = "bot-upstream"
					}
					if tc.passThrough {
						requestModel = mappedModel
					}
					body := fmt.Sprintf(`{"model":%q,"max_tokens":64,"stream":%t,"temperature":0,"top_p":0,"top_k":0,"system":"client system","thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"service_tier":"flex","tools":[{"name":"lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1hZ2U="}},{"type":"text","text":"hello"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"previous","signature":"original-signature"},{"type":"tool_use","id":"tool_previous","name":"lookup","input":{"q":"test"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_previous","content":"found","is_error":false}]}]}`, requestModel, tc.stream)
					if tc.passThrough {
						body = strings.TrimSuffix(body, "}") + `,"future_native_option":{"keep":true}}`
					}
					c, info, recorder := newVolcengineMessagesContext(t, upstream.URL+"/", body)
					c.Request.Header.Set("anthropic-version", "2023-06-01")
					c.Request.Header.Add("anthropic-beta", "beta-one")
					c.Request.Header.Add("anthropic-beta", "beta-two")
					c.Set("model_mapping", fmt.Sprintf(`{%q:%q}`, requestModel, mappedModel))
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passThrough, SystemPrompt: "channel system", SystemPromptOverride: true})
					if tc.override {
						common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"max_tokens": 128})
						common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, map[string]any{"x-api-key": "override-key"})
					}
					settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: tc.responses, AllChannels: true, ModelPatterns: []string{".*"}}
					const initialQuota, reserved = 1_000_000, 10
					user := model.User{Username: fmt.Sprintf("volc-user-%d", index), AffCode: fmt.Sprintf("volc-%d", index), Quota: initialQuota - reserved, Status: common.UserStatusEnabled}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("volc-token-%d", index), Status: common.TokenStatusEnabled, RemainQuota: initialQuota - reserved, UsedQuota: reserved}
					require.NoError(t, db.Create(&token).Error)
					channel := model.Channel{Name: fmt.Sprintf("volc-channel-%d", index), Type: constant.ChannelTypeVolcEngine, Status: common.ChannelStatusEnabled}
					require.NoError(t, db.Create(&channel).Error)
					common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
					info.UserId, info.TokenId, info.TokenKey, info.UserQuota = user.Id, token.Id, token.Key, initialQuota
					info.FinalPreConsumedQuota = reserved
					info.PriceData = hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.5, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
					apiErr := ClaudeHelper(c, info)
					require.Nil(t, apiErr)
					request, wire := <-captured, <-capturedBody
					wantPath := "/api/compatible/v1/messages"
					if tc.responses {
						wantPath = "/api/v3/responses"
					} else if tc.bot {
						wantPath = "/api/v3/bots/chat/completions"
					}
					assert.Equal(t, wantPath, request.URL.Path)
					assert.Empty(t, request.Header.Get("Cookie"))
					if tc.bot || tc.responses {
						assert.Equal(t, "Bearer channel-key", request.Header.Get("Authorization"))
						var result dto.ClaudeResponse
						require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
						require.NotEmpty(t, result.Content)
						assert.Equal(t, "ok", result.Content[0].GetText())
					} else {
						key := "channel-key"
						if tc.override {
							key = "override-key"
						}
						assert.Equal(t, key, request.Header.Get("x-api-key"))
						assert.Empty(t, request.Header.Get("Authorization"))
						assert.Equal(t, "2023-06-01", request.Header.Get("anthropic-version"))
						assert.Equal(t, "beta-one,beta-two", request.Header.Get("anthropic-beta"))
						if tc.passThrough {
							assert.Equal(t, body, wire)
						} else {
							var expected map[string]any
							require.NoError(t, common.UnmarshalJsonStr(body, &expected))
							expected["model"], expected["system"] = mappedModel, "channel system\nclient system"
							delete(expected, "service_tier")
							if tc.override {
								expected["max_tokens"] = 128
							}
							expectedJSON, err := common.Marshal(expected)
							require.NoError(t, err)
							assert.JSONEq(t, string(expectedJSON), wire)
						}
						if tc.stream {
							assert.Contains(t, recorder.Body.String(), `"signature":"opaque-signature"`)
							assert.Contains(t, recorder.Body.String(), `"type":"input_json_delta"`)
							assert.Contains(t, recorder.Body.String(), "event: message_stop")
							assert.NotContains(t, recorder.Body.String(), "[DONE]")
							require.NotNil(t, info.StreamStatus)
							assert.Equal(t, relaycommon.ResponseOutcomeCompleted, info.StreamStatus.OutcomeSnapshot().Response)
						} else {
							assert.JSONEq(t, response, recorder.Body.String())
						}
						assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
					}
					var stored model.Log
					require.NoError(t, logDB.Where("user_id = ?", user.Id).Take(&stored).Error)
					if !tc.responses {
						assert.Equal(t, wire, stored.Request)
						assert.Equal(t, strings.TrimSpace(response), stored.Response)
					}
					assert.Equal(t, expectedQuota, stored.Quota)
					assert.Equal(t, tc.stream, stored.IsStream)
					assert.Equal(t, "volc-request-id", stored.UpstreamRequestId)
					if !tc.bot && !tc.responses {
						assert.Equal(t, input, stored.PromptTokens)
						assert.Equal(t, 4, stored.CompletionTokens)
						var usage dto.Usage
						require.NoError(t, common.UnmarshalJsonStr(stored.Usage, &usage))
						assert.Equal(t, "anthropic", usage.UsageSemantic)
						require.NotNil(t, usage.BillingUsage)
						require.NotNil(t, usage.BillingUsage.ClaudeUsage)
						assert.Equal(t, input, usage.BillingUsage.ClaudeUsage.InputTokens)
						assert.Equal(t, 20, usage.BillingUsage.ClaudeUsage.CacheReadInputTokens)
					}
					require.NoError(t, db.First(&user, user.Id).Error)
					require.NoError(t, db.First(&token, token.Id).Error)
					assert.Equal(t, initialQuota-expectedQuota, user.Quota)
					assert.Equal(t, expectedQuota, user.UsedQuota)
					assert.Equal(t, 1, user.RequestCount)
					assert.Equal(t, initialQuota-expectedQuota, token.RemainQuota)
					assert.Equal(t, expectedQuota, token.UsedQuota)
				})
			}
		})
	}
}

func TestVolcengineMessagesRejectsOverriddenTokenLimit(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
	}{
		{name: "max_tokens", field: "max_tokens", value: uint(math.MaxInt32/2 + 1)},
		{name: "max_tokens_to_sample", field: "max_tokens_to_sample", value: uint(math.MaxInt32/2 + 1)},
		{name: "negative", field: "max_tokens", value: -1},
		{name: "beyond uint range", field: "max_tokens", value: math.MaxFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, info, _ := newVolcengineMessagesContext(t, "http://127.0.0.1:1", `{"model":"doubao-seed-2-1-pro","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{tc.field: tc.value, "stop_sequences": "end"})
			apiErr := ClaudeHelper(c, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Contains(t, apiErr.Error(), "max_tokens is invalid")
		})
	}
}

func TestVolcengineMessagesUpstreamErrors(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	service.InitHttpClient()
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			stream := status == http.StatusOK
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{"type":"error","error":{"type":"overloaded_error","message":"provider unavailable"}}`
				w.Header().Set("Content-Type", gin.MIMEJSON)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					body = "data: " + body + "\n\n"
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(upstream.Close)
			c, info, _ := newVolcengineMessagesContext(t, upstream.URL, fmt.Sprintf(`{"model":"doubao-seed-2-1-pro","stream":%t,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`, stream))
			apiErr := ClaudeHelper(c, info)
			require.NotNil(t, apiErr)
			wantStatus := status
			if stream {
				wantStatus = http.StatusInternalServerError
				require.NotNil(t, info.StreamStatus)
				assert.True(t, info.StreamStatus.HasErrors())
				assert.NotEqual(t, relaycommon.ResponseOutcomeCompleted, info.StreamStatus.OutcomeSnapshot().Response)
			}
			assert.Equal(t, wantStatus, apiErr.StatusCode)
			assert.Contains(t, apiErr.Error(), "provider unavailable")
		})
	}
	t.Run("missing terminal event", func(t *testing.T) {
		c, info, _ := newVolcengineMessagesContext(t, "https://ark.example", `{"model":"doubao-seed-2-1-pro","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
		info.InitChannelMeta(c)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"doubao-seed-2-1-pro\",\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"cache_read_input_tokens\":20}}}\n\n"))}
		result, apiErr := (&volcengine.Adaptor{}).DoResponse(c, resp, info)
		require.Nil(t, apiErr)
		require.NotNil(t, info.StreamStatus)
		outcome := info.StreamStatus.OutcomeSnapshot()
		assert.True(t, outcome.ExpectsTerminal)
		assert.Equal(t, relaycommon.ResponseOutcomeUnknown, outcome.Response)
		usage, ok := result.(*dto.Usage)
		require.True(t, ok)
		assert.Zero(t, usage.PromptTokens)
		assert.Equal(t, 20, usage.PromptTokensDetails.CachedTokens)
	})
}

func newVolcengineMessagesContext(t *testing.T, base, body string) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	t.Helper()
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	c.Request.Header.Set("Authorization", "Bearer client-token")
	c.Request.Header.Set("x-api-key", "client-token")
	c.Request.Header.Set("Cookie", "session=client-session")
	request, err := helper.GetAndValidateClaudeRequest(c)
	require.NoError(t, err)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeVolcEngine)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, base)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "channel-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, request.Model)
	info := relaycommon.GenRelayInfoClaude(c, request)
	info.StartTime = time.Now()
	info.UsingGroup, info.UserGroup, info.DisablePing = "default", "default", true
	return c, info, recorder
}

func volcengineMessagesStream(input int) string {
	events := []string{
		fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"ep-upstream","content":[],"usage":{"input_tokens":%d,"output_tokens":0,"cache_read_input_tokens":20}}}`, input),
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reason"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque-signature"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"test\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":%d,"output_tokens":4,"cache_read_input_tokens":20}}`, input),
		`{"type":"message_stop"}`,
	}
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

func setupVolcengineMessagesDatabases(t *testing.T, dialect string) (*gorm.DB, *gorm.DB) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldMemory, oldBatch, oldExport, oldConsume := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.DataExportEnabled, common.LogConsumeEnabled
	var databases [2]*gorm.DB
	for index, suffix := range []string{"_DSN", "_LOG_DSN"} {
		var driver gorm.Dialector = sqlite.Open(":memory:")
		if dialect != "sqlite" {
			env := "TEST_VOLCENGINE_" + strings.ToUpper(dialect) + suffix
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip(env + " is not configured")
			}
			if dialect == "mysql" {
				driver = mysql.Open(dsn)
			} else {
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
		}
		db, err := gorm.Open(driver, &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		models := []any{&model.User{}, &model.Token{}, &model.Channel{}}
		if index == 1 {
			models = []any{&model.Log{}}
		}
		for _, value := range models {
			require.False(t, db.Migrator().HasTable(value), "use isolated empty databases for this test")
		}
		t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
		require.NoError(t, db.AutoMigrate(models...))
		versionSQL := "SELECT version()"
		if dialect == "sqlite" {
			versionSQL = "SELECT sqlite_version()"
		}
		var version string
		require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
		t.Logf("%s database %d: %s", dialect, index, version)
		databases[index] = db
	}
	model.DB, model.LOG_DB = databases[0], databases[1]
	common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.DataExportEnabled, common.LogConsumeEnabled = false, false, false, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.DataExportEnabled, common.LogConsumeEnabled = oldRedis, oldMemory, oldBatch, oldExport, oldConsume
	})
	return databases[0], databases[1]
}
