package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func bridgeSSE(events ...string) string {
	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	return body.String()
}

func bridgeContext(t *testing.T, format types.RelayFormat) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	require.NoError(t, i18n.Init())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	path := "/v1/chat/completions"
	if format == types.RelayFormatClaude {
		path = "/v1/messages"
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "m1")
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set(common.RequestIdKey, common.NewRequestId())
	common.SetContextKey(c, constant.ContextKeyChannelModels, "m1,m2,m3")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ModelBehaviors: map[string]dto.ChannelModelBehavior{"m1": {ForceUpstreamStreamForNonStream: true}}})
	var request dto.Request = &dto.GeneralOpenAIRequest{Model: "m1", Messages: []dto.Message{{Role: "user", Content: "hello"}}}
	if format == types.RelayFormatClaude {
		request = &dto.ClaudeRequest{Model: "m1", Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
	}
	info, err := relaycommon.GenRelayInfo(c, format, request, nil)
	require.NoError(t, err)
	info.InitChannelMeta(c)
	return c, recorder, info
}

func TestBufferedChatAggregation(t *testing.T) {
	c, recorder, info := bridgeContext(t, types.RelayFormatOpenAI)
	info.ChannelSetting.ModelOutputMapping = `{"provider-m1":"public-m1","public-m1":"must-not-map-twice"}`
	info.ChannelSetting.ForceFormat = true
	info.UpstreamModelName = "provider-m1"
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(bridgeSSE(
		`{"id":"chat-1","created":123,"model":"provider-m1","system_fingerprint":"fp","choices":[{"index":1,"delta":{"role":"assistant","content":"second"}},{"index":0,"delta":{"reasoning_content":"think","signature":"sig","tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"two","arguments":"{\"b\":"}},{"index":0,"id":"call_a","type":"function","function":{"name":"one","arguments":"{\"a\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hello ","reasoning_content":" more","tool_calls":[{"index":0,"function":{"arguments":"1}"}},{"index":1,"function":{"arguments":"2}"}}],"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com"}}]}},{"index":1,"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"delta":{"content":"world"},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`[DONE]`,
	)))}
	usage, logged, apiErr := doBufferedStreamResponse(c, resp, info, &openai.Adaptor{})
	require.Nil(t, apiErr)
	assert.Equal(t, 18, usage.(*dto.Usage).TotalTokens)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, recorder.Body.String(), logged)
	assert.Equal(t, "chat.completion", gjson.Get(logged, "object").String())
	assert.Equal(t, "public-m1", gjson.Get(logged, "model").String())
	assert.Equal(t, "fp", gjson.Get(logged, "system_fingerprint").String())
	assert.Equal(t, "hello world", gjson.Get(logged, "choices.0.message.content").String())
	assert.Equal(t, "think more", gjson.Get(logged, "choices.0.message.reasoning_content").String())
	assert.Equal(t, "sig", gjson.Get(logged, "choices.0.message.signature").String())
	assert.Equal(t, "https://example.com", gjson.Get(logged, "choices.0.message.annotations.0.url_citation.url").String())
	assert.Equal(t, "{\"a\":1}", gjson.Get(logged, "choices.0.message.tool_calls.0.function.arguments").String())
	assert.Equal(t, "{\"b\":2}", gjson.Get(logged, "choices.0.message.tool_calls.1.function.arguments").String())
	assert.Equal(t, "second", gjson.Get(logged, "choices.1.message.content").String())
	assert.False(t, gjson.Get(logged, "choices.0.message.tool_calls.0.index").Exists())
	assert.True(t, info.IsStream)
	assert.False(t, info.ClientIsStream)
	assert.Equal(t, "completed", info.StreamStatus.ResponseOutcome())
}

func bridgeMessages() string {
	return bridgeSSE(
		`{"type":"message_start","message":{"id":"msg-1","type":"message","role":"assistant","model":"provider-m1","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":12,"ephemeral_1h_input_tokens":23}}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call-1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"hello\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":"Hello"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	)
}

func TestBufferedMessagesAndClaudeBillingUsage(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			c, recorder, info := bridgeContext(t, format)
			info.FinalRequestRelayFormat = types.RelayFormatClaude
			info.UpstreamModelName = "provider-m1"
			info.ChannelSetting.ModelOutputMapping = `{"provider-m1":"public-m1"}`
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(bridgeMessages()))}
			usageValue, _, apiErr := doBufferedStreamResponse(c, resp, info, &claude.Adaptor{})
			require.Nil(t, apiErr)
			usage := usageValue.(*dto.Usage)
			assert.Equal(t, 10, usage.PromptTokens, "billing keeps native ordinary input")
			assert.Equal(t, 20, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 12, usage.ClaudeCacheCreation5mTokens)
			assert.Equal(t, 23, usage.ClaudeCacheCreation1hTokens)
			require.NotNil(t, usage.BillingUsage)
			require.NotNil(t, usage.BillingUsage.ClaudeUsage)
			assert.Equal(t, 10, usage.BillingUsage.ClaudeUsage.InputTokens)
			assert.Equal(t, 30, usage.BillingUsage.ClaudeUsage.CacheCreationInputTokens)
			body := recorder.Body.String()
			assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "public-m1", gjson.Get(body, "model").String())
			if format == types.RelayFormatClaude {
				assert.Equal(t, "sig", gjson.Get(body, "content.0.signature").String())
				assert.Equal(t, "hello", gjson.Get(body, "content.1.input.q").String())
				assert.Equal(t, "Hello world", gjson.Get(body, "content.2.text").String())
				assert.Equal(t, int64(10), gjson.Get(body, "usage.input_tokens").Int())
			} else {
				assert.Equal(t, "sig", gjson.Get(body, "choices.0.message.signature").String())
				assert.Equal(t, int64(65), gjson.Get(body, "usage.prompt_tokens").Int())
				assert.Equal(t, int64(65), gjson.Get(body, "usage.input_tokens").Int())
				assert.Equal(t, int64(72), gjson.Get(body, "usage.total_tokens").Int())
				assert.Equal(t, "openai", gjson.Get(body, "usage.usage_semantic").String())
				assert.Equal(t, "anthropic", gjson.Get(body, "usage.usage_source").String())
			}
		})
	}
}

type bridgeFailingReader struct{ io.Reader }

func (r bridgeFailingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, errors.New("connection reset")
	}
	return n, err
}

func TestBufferedStreamFailuresNeverCommitResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		format     types.RelayFormat
		broken     bool
		limit      int
	}{
		{"malformed JSON", bridgeSSE(`{"choices":`), types.RelayFormatOpenAI, false, 4096},
		{"error event", bridgeSSE(`{"error":{"type":"overloaded_error","message":"busy"}}`), types.RelayFormatOpenAI, false, 4096},
		{"missing finish", bridgeSSE(`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`, `[DONE]`), types.RelayFormatOpenAI, false, 4096},
		{"empty stream", bridgeSSE(`[DONE]`), types.RelayFormatOpenAI, false, 4096},
		{"scanner failure after finish", bridgeSSE(`{"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`), types.RelayFormatOpenAI, true, 4096},
		{"bounded response", bridgeSSE(`{"choices":[{"index":0,"delta":{"content":"too long"},"finish_reason":"stop"}]}`), types.RelayFormatOpenAI, false, 32},
		{"missing message stop", strings.ReplaceAll(bridgeMessages(), "data: {\"type\":\"message_stop\"}\n\n", ""), types.RelayFormatClaude, false, 65536},
		{"invalid tool JSON", strings.Replace(bridgeMessages(), `\"hello\"}`, `broken}`, 1), types.RelayFormatClaude, false, 65536},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder, info := bridgeContext(t, tc.format)
			var reader io.Reader = strings.NewReader(tc.body)
			if tc.broken {
				reader = bridgeFailingReader{reader}
			}
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(reader)}
			_, err := aggregateBufferedStream(c, resp, info, tc.limit)
			require.Error(t, err)
			assert.False(t, c.Writer.Written())
			assert.Empty(t, recorder.Body.String())
			assert.Empty(t, recorder.Header().Get("Content-Type"))
		})
	}
}

func TestBufferedStreamCancellationClosesBody(t *testing.T) {
	c, _, info := bridgeContext(t, types.RelayFormatOpenAI)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	cancel()
	resp := &http.Response{Body: reader}
	_, err := aggregateBufferedStream(c, resp, info, 4096)
	require.ErrorIs(t, err, context.Canceled)
	_, err = writer.Write([]byte("data: {}\n\n"))
	require.Error(t, err, "cancellation closes upstream body")
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
}

func TestBufferedStreamAttemptPolicyAndRetry(t *testing.T) {
	c, _, info := bridgeContext(t, types.RelayFormatOpenAI)
	assert.True(t, info.BufferStreamResponse)
	assert.True(t, info.IsStream)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	info.InitChannelMeta(c)
	assert.False(t, info.BufferStreamResponse)
	assert.False(t, info.IsStream)
	assert.False(t, info.Request.IsStream(c.Request))
	// Responses WebSocket forces an upstream stream independently of the
	// optional stream member on each response.create frame.
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.IsStream = true
	info.InitChannelMeta(c)
	assert.True(t, info.IsStream)
	assert.False(t, info.BufferStreamResponse)

	for _, tc := range []struct {
		name, model  string
		stream, test bool
		path         string
	}{
		{"different model", "m2", false, false, "/v1/chat/completions"},
		{"case sensitive model", "M1", false, false, "/v1/chat/completions"},
		{"orphaned model", "orphan", false, false, "/v1/chat/completions"},
		{"client streaming", "m1", true, false, "/v1/chat/completions"},
		{"channel test", "m1", false, true, "/v1/chat/completions"},
		{"Responses entry", "m1", false, false, "/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, info := bridgeContext(t, types.RelayFormatOpenAI)
			info.OriginModelName, info.IsChannelTest = tc.model, tc.test
			info.ClientIsStream = tc.stream
			info.Request.(*dto.GeneralOpenAIRequest).Stream = common.GetPointer(tc.stream)
			if tc.path == "/v1/responses" {
				info.RelayFormat = types.RelayFormatOpenAIResponses
				info.IsStream = tc.stream
			}
			info.InitChannelMeta(c)
			assert.False(t, info.BufferStreamResponse)
			assert.Equal(t, tc.stream, info.IsStream)
		})
	}
}

func TestBufferedStreamForcesFinalUpstreamBody(t *testing.T) {
	database := bridgeAccountingDatabase(t)
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	for _, passthrough := range []bool{false, true} {
		for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", format, passthrough), func(t *testing.T) {
				captured := make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "read failed", 500)
						return
					}
					captured <- string(data)
					assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
					w.Header().Set("Content-Type", "text/event-stream")
					if format == types.RelayFormatClaude {
						_, _ = io.WriteString(w, bridgeMessages())
						return
					}
					_, _ = io.WriteString(w, bridgeSSE(`{"id":"chat-1","model":"provider-m1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, `[DONE]`))
				}))
				defer server.Close()
				c, recorder, info := bridgeContext(t, format)
				if format == types.RelayFormatClaude {
					common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
				}
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
				common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
				common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{"m1":"provider-m1"}`)
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"stream": false})
				settings, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
				settings.PassThroughBodyEnabled = passthrough
				common.SetContextKey(c, constant.ContextKeyChannelSetting, settings)
				c.Request.Header.Set("Accept", "application/json")
				oldSetting := model_setting.GetGlobalSettings().PassThroughRequestEnabled
				model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
				t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldSetting })
				info.StartTime = time.Now()
				var apiErr *types.NewAPIError
				if format == types.RelayFormatOpenAI {
					apiErr = TextHelper(c, info)
				} else {
					apiErr = ClaudeHelper(c, info)
				}
				require.Nil(t, apiErr)
				received := <-captured
				assert.True(t, gjson.Get(received, "stream").Bool())
				if !passthrough {
					assert.Equal(t, "provider-m1", gjson.Get(received, "model").String())
				}
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
				assert.False(t, info.Request.IsStream(c.Request))
				var logs []model.Log
				require.NoError(t, database.Where("request_id = ?", c.GetString(common.RequestIdKey)).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.False(t, logs[0].IsStream)
				assert.Zero(t, logs[0].FirstTokenMs)
				assert.Equal(t, recorder.Body.String(), logs[0].Response)
				assert.False(t, gjson.Get(logs[0].Request, "stream").Bool())

			})
		}
	}
}

func TestBufferedStreamLogMetadataUsesClientContract(t *testing.T) {
	c, _, info := bridgeContext(t, types.RelayFormatOpenAI)
	info.StartTime = time.Now()
	info.FirstResponseTime = info.StartTime.Add(time.Second)
	info.UpstreamFirstChunkTime = info.StartTime.Add(100 * time.Millisecond)
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	other := service.GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
	encoded, err := common.Marshal(other)
	require.NoError(t, err)
	assert.Zero(t, gjson.GetBytes(encoded, "frt").Float())
	assert.True(t, gjson.GetBytes(encoded, "admin_info.stream_bridge.buffered").Bool())
	assert.False(t, gjson.GetBytes(encoded, "admin_info.stream_bridge.client_stream").Bool())
	assert.Equal(t, int64(100), gjson.GetBytes(encoded, "admin_info.stream_bridge.upstream_first_chunk_ms").Int())
}

func bridgeAccountingDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousBatch, previousRedis := common.BatchUpdateEnabled, common.RedisEnabled
	previousExport, previousAlerts, previousLogs := common.DataExportEnabled, common.AlertingEnabled, common.LogConsumeEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Channel{}, &model.Log{}))
	require.NoError(t, database.Create(&model.User{Id: 1, Username: "bridge-user", Group: "default"}).Error)
	require.NoError(t, database.Create(&model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Name: "bridge", Key: "test-key"}).Error)
	model.DB, model.LOG_DB = database, database
	common.BatchUpdateEnabled, common.RedisEnabled = false, false
	common.DataExportEnabled, common.AlertingEnabled, common.LogConsumeEnabled = false, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.BatchUpdateEnabled, common.RedisEnabled = previousBatch, previousRedis
		common.DataExportEnabled, common.AlertingEnabled, common.LogConsumeEnabled = previousExport, previousAlerts, previousLogs
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestBufferedStreamRetryRestoresNonStreamAndSettlesOnce(t *testing.T) {
	database := bridgeAccountingDatabase(t)
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	modes := make(chan bool, 2)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", 500)
			return
		}
		modes <- gjson.GetBytes(body, "stream").Bool()
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, bridgeSSE(`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`, `[DONE]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"recovered","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"recovered"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
	}))
	defer server.Close()
	c, recorder, info := bridgeContext(t, types.RelayFormatOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	apiErr := TextHelper(c, info)
	require.NotNil(t, apiErr)
	assert.False(t, c.Writer.Written())
	assert.Empty(t, recorder.Body.String())
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	apiErr = TextHelper(c, info)
	require.Nil(t, apiErr)
	assert.Equal(t, []bool{true, false}, []bool{<-modes, <-modes})
	assert.True(t, service.RequestPolicy(c).Successful, "the failed buffered attempt must not mark the recovered request as unsuccessful")
	assert.Equal(t, "recovered", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
	var user model.User
	require.NoError(t, database.First(&user, 1).Error)
	assert.Equal(t, 1, user.RequestCount)
	var logs []model.Log
	require.NoError(t, database.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.False(t, logs[0].IsStream)
}

func TestBufferedStreamPassthroughOnlyChangesStream(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprintf("global=%t", global), func(t *testing.T) {
			_, _, info := bridgeContext(t, types.RelayFormatOpenAI)
			settings := model_setting.GetGlobalSettings()
			previous := settings.PassThroughRequestEnabled
			settings.PassThroughRequestEnabled = global
			t.Cleanup(func() { settings.PassThroughRequestEnabled = previous })
			info.ChannelSetting.PassThroughBodyEnabled = !global
			body := []byte(`{"model":"m1","stream":false,"stream_options":{"include_usage":false},"extra":{"keep":0},"temperature":0}`)
			actual, err := forceBufferedStreamRequest(info, body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"m1","stream":true,"stream_options":{"include_usage":false},"extra":{"keep":0},"temperature":0}`, string(actual))
		})
	}
}

func TestBufferedStreamCrossProtocolAndMissingUsage(t *testing.T) {
	c, recorder, info := bridgeContext(t, types.RelayFormatClaude)
	info.FinalRequestRelayFormat = types.RelayFormatOpenAI
	info.SetEstimatePromptTokens(4)
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(bridgeSSE(`{"id":"chat-1","model":"m1","choices":[{"index":0,"delta":{"content":"hello world"},"finish_reason":"stop"}]}`, `[DONE]`)))}
	usage, _, apiErr := doBufferedStreamResponse(c, resp, info, &openai.Adaptor{})
	require.Nil(t, apiErr)
	assert.Positive(t, usage.(*dto.Usage).CompletionTokens)
	assert.Equal(t, 4, usage.(*dto.Usage).PromptTokens)
	assert.Equal(t, "message", gjson.Get(recorder.Body.String(), "type").String())
	assert.Equal(t, "hello world", gjson.Get(recorder.Body.String(), "content.0.text").String())
}

func TestBufferedResponsesRequiresTerminalAndReturnsClientProtocol(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(fmt.Sprintf("complete=%t", complete), func(t *testing.T) {
			c, recorder, info := bridgeContext(t, types.RelayFormatOpenAI)
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
			events := []string{`{"type":"response.output_text.delta","output_index":0,"item_id":"item-1","delta":"hello"}`}
			if complete {
				events = append(events, `{"type":"response.completed","response":{"id":"resp-1","model":"m1","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}`)
			}
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(bridgeSSE(events...)))}
			usage, _, apiErr := doBufferedStreamResponse(c, resp, info, &openai.Adaptor{})
			if !complete {
				require.NotNil(t, apiErr)
				assert.False(t, c.Writer.Written())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, 6, usage.(*dto.Usage).TotalTokens)
			assert.Equal(t, "chat.completion", gjson.Get(recorder.Body.String(), "object").String())
			assert.Equal(t, "hello", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
		})
	}
}

func TestBufferedMessagesEstimatesUnreportedOutput(t *testing.T) {
	for _, reported := range []bool{false, true} {
		t.Run(fmt.Sprintf("reported=%t", reported), func(t *testing.T) {
			c, recorder, info := bridgeContext(t, types.RelayFormatClaude)
			info.SetEstimatePromptTokens(5)
			end := `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`
			if reported {
				end = `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`
			}
			body := bridgeSSE(`{"type":"message_start","message":{"id":"msg-1","role":"assistant","model":"m1","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello world"}}`, `{"type":"content_block_stop","index":0}`, end, `{"type":"message_stop"}`)
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			value, _, apiErr := doBufferedStreamResponse(c, resp, info, &claude.Adaptor{})
			require.Nil(t, apiErr)
			assert.Equal(t, 5, value.(*dto.Usage).PromptTokens)
			if reported {
				assert.Zero(t, value.(*dto.Usage).CompletionTokens)
			} else {
				assert.Positive(t, value.(*dto.Usage).CompletionTokens)
			}
			assert.Equal(t, int64(value.(*dto.Usage).CompletionTokens), gjson.Get(recorder.Body.String(), "usage.output_tokens").Int())
		})
	}
}

func TestBufferedGlobalResponsesRoutePreservesLogsAndOverrides(t *testing.T) {
	database := bridgeAccountingDatabase(t)
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	settings := model_setting.GetGlobalSettings()
	previousPolicy := settings.ChatCompletionsToResponsesPolicy
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, AllChannels: true, ModelPatterns: []string{"^m1$"}}
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = previousPolicy })
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			captured := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", 500)
					return
				}
				captured <- string(body)
				assert.Equal(t, "/v1/responses", r.URL.Path)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, bridgeSSE(`{"type":"response.completed","response":{"id":"resp-1","model":"m1","status":"completed","output":[{"id":"item-1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}`))
			}))
			defer server.Close()
			c, recorder, info := bridgeContext(t, format)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"stream": false})
			var apiErr *types.NewAPIError
			if format == types.RelayFormatClaude {
				apiErr = ClaudeHelper(c, info)
			} else {
				apiErr = TextHelper(c, info)
			}
			require.Nil(t, apiErr)
			assert.True(t, gjson.Get(<-captured, "stream").Bool())
			var log model.Log
			require.NoError(t, database.Where("request_id = ?", c.GetString(common.RequestIdKey)).Take(&log).Error)
			assert.False(t, log.IsStream)
			assert.False(t, gjson.Get(log.Request, "stream").Bool())
			assert.JSONEq(t, `{"model":"m1","messages":[{"role":"user","content":"hello"}]}`, log.Request)
			assert.Equal(t, recorder.Body.String(), log.Response)
			assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		})
	}
}

func TestBufferedStreamErrorsUseClientLanguage(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN", "zh-TW"} {
		t.Run(lang, func(t *testing.T) {
			c, _, info := bridgeContext(t, types.RelayFormatOpenAI)
			c.Request.Header.Set("Accept-Language", lang)
			apiErr := newBufferedStreamError(c, info, errBufferedStreamTooLarge)
			require.NotNil(t, apiErr)
			assert.Contains(t, apiErr.Error(), "64 MiB")
			assert.Equal(t, i18n.Translate(lang, i18n.MsgRelayBufferedStreamTooLarge), apiErr.Error())
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}

func TestBufferedResponsesErrorsPreserveStatusMapping(t *testing.T) {
	c, recorder, info := bridgeContext(t, types.RelayFormatOpenAI)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, bridgeSSE(`{"type":"response.output_text.delta","delta":"unfinished"}`))
	}))
	defer server.Close()
	info.ChannelBaseUrl = server.URL
	c.Set("status_code_mapping", `{"502":"503"}`)
	adaptor := &openai.Adaptor{}
	adaptor.Init(info)
	_, apiErr := relayResponsesRequest(c, info, adaptor, &dto.OpenAIResponsesRequest{Model: "m1", Input: []byte(`"hello"`), Stream: common.GetPointer(true)}, false)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.False(t, c.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}
