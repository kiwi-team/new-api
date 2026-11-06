package volcengine

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seeduplexNativeUsage = `{"input_text_tokens":100,"input_audio_tokens":200,"cached_text_tokens":300,"cached_audio_tokens":400,"output_text_tokens":50,"output_audio_tokens":60}`

func TestSeeduplexUpstreamCredentials(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, tc := range []struct {
		name, key string
		legacy    bool
		invalid   bool
	}{
		{name: "modern API key", key: "test-api-key"},
		{name: "legacy app ID and access token", key: "app-id|access-token", legacy: true},
		{name: "empty key", invalid: true},
		{name: "empty legacy component", key: "app-id|", invalid: true},
		{name: "extra credential components", key: "app|secret|extra", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: seeduplexModel, ApiKey: tc.key}}
			header := http.Header{}
			err := (&RealtimeAdaptor{}).SetupRequestHeader(ctx, &header, info)
			if tc.invalid {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "secret")
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, header.Get("X-Api-Request-Id"))
			assert.Empty(t, header.Get("Authorization"))
			if tc.legacy {
				assert.Empty(t, header.Get("X-Api-Key"))
				assert.Equal(t, "app-id", header.Get("X-Api-App-Id"))
				assert.Equal(t, "access-token", header.Get("X-Api-Access-Key"))
				assert.Equal(t, doubaoRealtimeResourceID, header.Get("X-Api-Resource-Id"))
				assert.Equal(t, doubaoRealtimeAppKey, header.Get("X-Api-App-Key"))
			} else {
				assert.Equal(t, tc.key, header.Get("X-Api-Key"))
				assert.Empty(t, header.Get("X-Api-Access-Key"))
				assert.Empty(t, header.Get("X-Api-App-Id"))
			}
		})
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: seeduplexModel, ApiKey: "test-api-key"}}
	info.ChannelSetting.TLSInsecureSkipVerify = true
	header := http.Header{}
	require.ErrorContains(t, (&RealtimeAdaptor{}).SetupRequestHeader(ctx, &header, info), "certificate verification")
	assert.Empty(t, header.Get("X-Api-Key"))
}

func TestSeeduplexUsageValidation(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		invalid       bool
	}{
		{name: "native disjoint counters", payload: seeduplexNativeUsage},
		{name: "inclusive realtime counters", payload: `{"input_tokens":1000,"output_tokens":110,"total_tokens":1110,"input_token_details":{"text_tokens":400,"audio_tokens":600,"cached_tokens":700,"cached_tokens_details":{"text_tokens":300,"audio_tokens":400}},"output_token_details":{"text_tokens":50,"audio_tokens":60}}`},
		{name: "missing usage", payload: `null`, invalid: true},
		{name: "no modality breakdown", payload: `{"input_tokens":1000,"output_tokens":110}`, invalid: true},
		{name: "missing native cache counts", payload: `{"input_text_tokens":100,"input_audio_tokens":200,"output_text_tokens":50,"output_audio_tokens":60}`, invalid: true},
		{name: "negative count", payload: strings.Replace(seeduplexNativeUsage, `"input_text_tokens":100`, `"input_text_tokens":-1`, 1), invalid: true},
		{name: "fractional count", payload: strings.Replace(seeduplexNativeUsage, `"input_text_tokens":100`, `"input_text_tokens":1.5`, 1), invalid: true},
		{name: "overflowing counters", payload: strings.Replace(seeduplexNativeUsage, `"input_text_tokens":100`, `"input_text_tokens":2147483647`, 1), invalid: true},
		{name: "inconsistent total", payload: strings.Replace(seeduplexNativeUsage, "}", `,"total_tokens":1000}`, 1), invalid: true},
		{name: "unknown cached audio share", payload: `{"input_tokens":1000,"output_tokens":110,"input_token_details":{"text_tokens":400,"audio_tokens":600,"cached_tokens":700},"output_token_details":{"text_tokens":50,"audio_tokens":60}}`, invalid: true},
		{name: "cache exceeds modality", payload: `{"input_tokens":1000,"output_tokens":110,"input_token_details":{"text_tokens":400,"audio_tokens":600,"cached_tokens":700,"cached_tokens_details":{"text_tokens":500,"audio_tokens":200}},"output_token_details":{"text_tokens":50,"audio_tokens":60}}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage, err := parseSeeduplexUsage([]byte(tc.payload))
			if tc.invalid {
				require.Error(t, err)
				assert.Nil(t, usage)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1000, usage.InputTokens)
			assert.Equal(t, 110, usage.OutputTokens)
			assert.Equal(t, 1110, usage.TotalTokens)
			assert.Equal(t, 700, usage.InputTokenDetails.CachedTokens)
			assert.Zero(t, usage.InputTokenDetails.CacheWriteTokens)
			assert.Equal(t, 400, *usage.InputTokenDetails.CachedTokensDetails.AudioTokens)
		})
	}
}

func TestSeeduplexUsagePerTurnDeduplication(t *testing.T) {
	var tracker seeduplexUsageTracker
	event := seeduplexEvent{Type: "response.done", EventID: "usage-1", Usage: []byte(seeduplexNativeUsage)}
	changed, err := tracker.consume(event)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = tracker.consume(event)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, 1110, tracker.total.TotalTokens)
	event.Usage = []byte(strings.Replace(seeduplexNativeUsage, `"input_text_tokens":100`, `"input_text_tokens":101`, 1))
	_, err = tracker.consume(event)
	require.ErrorContains(t, err, "conflicting")
	assert.Equal(t, 1110, tracker.total.TotalTokens)
	event.Usage, event.EventID = []byte(seeduplexNativeUsage), "usage-2"
	changed, err = tracker.consume(event)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, 2220, tracker.total.TotalTokens)
	assert.Equal(t, 1400, tracker.total.InputTokenDetails.CachedTokens)
	assert.Equal(t, 800, *tracker.total.InputTokenDetails.CachedTokensDetails.AudioTokens)
}

func TestSeeduplexRejectsInvalidClientSession(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		binary        bool
	}{
		{name: "binary protocol", message: "audio", binary: true},
		{name: "missing event type", message: `{}`},
		{name: "audio before session", message: `{"type":"input_audio_buffer.append","audio":"AAAA"}`},
		{name: "update before create", message: `{"type":"session.update","session":{"model":"1.2.6.1"}}`},
		{name: "different model", message: `{"type":"session.create","session":{"model":"other-model"}}`},
		{name: "missing model", message: `{"type":"session.create","session":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway, client := seeduplexSocketPair(t)
			session := &seeduplexRelay{info: &relaycommon.RelayInfo{ClientWs: gateway}}
			done := make(chan error, 1)
			go func() { done <- session.readClient() }()
			kind := websocket.TextMessage
			if tc.binary {
				kind = websocket.BinaryMessage
			}
			require.NoError(t, client.WriteMessage(kind, []byte(tc.message)))
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(10 * time.Second):
				t.Fatal("invalid Seeduplex input was not rejected")
			}
		})
	}
}

type seeduplexTestBilling struct {
	reserved int
	targets  []int
	reject   bool
}

func (*seeduplexTestBilling) Settle(int) error           { return nil }
func (*seeduplexTestBilling) Refund(*gin.Context)        {}
func (*seeduplexTestBilling) NeedsRefund() bool          { return false }
func (b *seeduplexTestBilling) GetPreConsumedQuota() int { return b.reserved }
func (b *seeduplexTestBilling) Reserve(quota int) error {
	b.targets = append(b.targets, quota)
	if b.reject {
		return errors.New("insufficient quota")
	}
	b.reserved = max(b.reserved, quota)
	return nil
}

func seeduplexSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if assert.NoError(t, err) {
			accepted <- conn
		}
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	conn := <-accepted
	t.Cleanup(func() { _ = conn.Close(); _ = client.Close() })
	require.NoError(t, client.SetReadDeadline(time.Now().Add(10*time.Second)))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	return conn, client
}

func TestSeeduplexRelayPassthroughAndBillingLifecycle(t *testing.T) {
	for _, ending := range []string{"normal close", "client disconnected", "provider error", "quota exhausted", "malformed usage"} {
		t.Run(ending, func(t *testing.T) {
			oldEnabled, oldControls := common.AlertingEnabled, common.AlertingControlsEnabled
			common.AlertingEnabled, common.AlertingControlsEnabled = false, false
			t.Cleanup(func() { common.AlertingEnabled, common.AlertingControlsEnabled = oldEnabled, oldControls })
			gatewayClient, client := seeduplexSocketPair(t)
			upstream, gatewayTarget := seeduplexSocketPair(t)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model="+seeduplexModel, nil)
			expression, ok := billing_setting.GetBuiltinBillingExpr(seeduplexModel)
			require.True(t, ok)
			billing := &seeduplexTestBilling{reject: ending == "quota exhausted"}
			info := relaycommon.GenRelayInfoWs(ctx, gatewayClient)
			info.TargetWs, info.Billing, info.OriginModelName = gatewayTarget, billing, seeduplexModel
			info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: seeduplexModel}
			info.PriceData = hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000}
			type result struct {
				usage *dto.RealtimeUsage
				err   *types.NewAPIError
			}
			done := make(chan result, 1)
			go func() {
				usage, apiErr := (&RealtimeAdaptor{}).DoResponse(ctx, nil, info)
				done <- result{usage.(*dto.RealtimeUsage), apiErr}
			}()
			create := `{"type":"session.create","session":{"model":"1.2.6.1","audio":{"output":{"speed":0}},"future_field":false},"extension":{"dialog":{"extra":{"volc_websearch_api_key":"do-not-log"}}}}`
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(create)))
			_, message, err := upstream.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, create, string(message), "unknown fields, extension and explicit zero/false must survive")
			created := `{"type":"session.created","session":{"id":"session-1"}}`
			require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(created)))
			_, message, err = client.ReadMessage()
			require.NoError(t, err)
			assert.JSONEq(t, created, string(message))
			if ending == "normal close" {
				for _, control := range []string{
					`{"type":"session.update","session":{"model":"1.2.6.1","tools":[],"audio":{"output":{"speed":0,"loudness":0}}},"extension":{"asr":{"extra":{"enable_asr_twopass":false}}}}`,
					`{"type":"input_audio_mute.commit"}`,
					`{"type":"input_audio_unmute.commit"}`,
					`{"type":"input_audio_buffer.append","audio":"AAAA"}`,
					`{"type":"conversation.item.create","items":[{"role":"tool","call_id":"call-1","content":[{"type":"input_text","text":"result-1"}]},{"role":"tool","call_id":"call-2","content":[{"type":"input_text","text":"result-2"}]}]}`,
					`{"type":"response.cancel"}`,
				} {
					require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(control)))
					_, message, err = upstream.ReadMessage()
					require.NoError(t, err)
					assert.Equal(t, control, string(message))
				}
				for _, output := range []string{
					`{"type":"response.function_call_arguments.done","items":[{"call_id":"call-1","name":"weather","arguments":"{}"},{"call_id":"call-2","name":"clock","arguments":"{}"}]}`,
					`{"type":"response.output_audio.delta","delta":"AAAA","future_field":false}`,
					`{"type":"response.canceled","event_id":"cancel-1"}`,
				} {
					require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(output)))
					_, message, err = client.ReadMessage()
					require.NoError(t, err)
					assert.JSONEq(t, output, string(message))
				}
			}
			usageEvent := `{"type":"response.done","event_id":"usage-1","usage":` + seeduplexNativeUsage + `,"extension":{"key":"do-not-log"}}`
			require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(usageEvent)))
			_, message, err = client.ReadMessage()
			require.NoError(t, err)
			assert.JSONEq(t, usageEvent, string(message))
			switch ending {
			case "quota exhausted":
				_, message, err = client.ReadMessage()
				require.NoError(t, err)
				assert.Contains(t, string(message), "insufficient quota")
			case "provider error":
				require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","error":{"code":"upstream_error","message":"failed"}}`)))
				_, message, err = client.ReadMessage()
				require.NoError(t, err)
				assert.Contains(t, string(message), "upstream_error")
			case "malformed usage":
				require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.done","response":null}`)))
				_, _, err = client.ReadMessage()
				require.NoError(t, err)
				_, message, err = client.ReadMessage()
				require.NoError(t, err)
				assert.Contains(t, string(message), "invalid Seeduplex usage")
			default:
				if ending == "normal close" {
					// A duplicate usage is still forwarded, but must not reserve twice.
					require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(usageEvent)))
					_, _, err = client.ReadMessage()
					require.NoError(t, err)
					require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.close"}`)))
				} else {
					require.NoError(t, client.Close())
				}
				_, message, err = upstream.ReadMessage()
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"session.close"}`, string(message))
				require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(strings.Replace(usageEvent, "usage-1", "usage-2", 1))))
				require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.closed"}`)))
			}
			var completed result
			select {
			case completed = <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("Seeduplex relay did not finish")
			}
			require.Nil(t, completed.err, "confirmed usage must reach WssHelper settlement even on errors")
			assert.Equal(t, ending != "normal close" && ending != "client disconnected", info.RealtimeUsageIncomplete)
			assert.Equal(t, ending == "normal close", info.HasSendResponse(), "only non-empty model output establishes a first response")
			if ending == "normal close" || ending == "client disconnected" {
				assert.Equal(t, 2220, completed.usage.TotalTokens)
				assert.Equal(t, []int{3125, 6250}, billing.targets)
			} else {
				assert.Equal(t, 1110, completed.usage.TotalTokens)
				assert.Equal(t, []int{3125}, billing.targets)
			}
			assert.NotContains(t, strings.Join(info.WsRequestMessages, "\n"), "do-not-log")
			assert.NotContains(t, strings.Join(info.WsResponseMessages, "\n"), "do-not-log")
			assert.NotEmpty(t, info.WsResponseMessages)
		})
	}
}
