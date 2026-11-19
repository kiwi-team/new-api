package qwen_realtime

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/ali_dashscope"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASRRealtimeRequestURLAndModelLock(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://workspace.cn-beijing.maas.aliyuncs.com",
			UpstreamModelName: "fun-asr-realtime",
		},
	}
	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "wss://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference", requestURL)

	message := []byte(`{"header":{"action":"run-task","task_id":"task"},"payload":{"model":"client-model","parameters":{"format":"pcm"}}}`)
	rewritten, err := rewriteASRRunTaskModel(message, info.UpstreamModelName)
	require.NoError(t, err)
	var event map[string]any
	require.NoError(t, common.Unmarshal(rewritten, &event))
	payload, ok := event["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fun-asr-realtime", payload["model"])

	info.ChannelMeta.UpstreamModelName = "qwen3-asr-flash-realtime"
	requestURL, err = (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "wss://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/realtime?model=qwen3-asr-flash-realtime", requestURL)
	assert.True(t, isASRRealtimeModel(info.UpstreamModelName))
	assert.False(t, isFunASRRealtimeModel(info.UpstreamModelName))
}

func TestASRRealtimeUsageUsesCumulativeDurationDelta(t *testing.T) {
	accounted := make(map[string]int)
	first, clamp, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":3}}}`), accounted)
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, first)
	assert.Equal(t, 50, first.InputTokenDetails.AudioTokens)

	second, clamp, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":9}}}`), accounted)
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, second)
	assert.Equal(t, 100, second.InputTokenDetails.AudioTokens)

	duplicate, _, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":9}}}`), accounted)
	require.NoError(t, err)
	assert.Nil(t, duplicate)

	_, clamp, err = asrUsageDelta([]byte(`{"header":{"task_id":"other","event":"result-generated"},"payload":{"usage":{"duration":1e300}}}`), accounted)
	require.NoError(t, err)
	require.NotNil(t, clamp)
	assert.Equal(t, common.MaxQuota, clamp.Clamped)
}

func TestQwen3ASRUsageTracksCumulativePCMSeconds(t *testing.T) {
	tracker := newQwen3ASRUsageTracker()
	_, _, err := tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":16000}}`))
	require.NoError(t, err)

	oneSecond := base64.StdEncoding.EncodeToString(make([]byte, 16000*2))
	first, clamp, err := tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"` + oneSecond + `"}`))
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, first)
	assert.Equal(t, 17, first.InputTokens)
	assert.Equal(t, 17, first.InputTokenDetails.AudioTokens)

	second, clamp, err := tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"` + oneSecond + `"}`))
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, second)
	assert.Equal(t, 16, second.InputTokens)
	assert.Equal(t, 16, second.InputTokenDetails.AudioTokens)
}

func TestQwen3ASRUsageRejectsUnbillableAudio(t *testing.T) {
	tracker := newQwen3ASRUsageTracker()
	_, _, err := tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"opus","sample_rate":16000}}`))
	require.ErrorContains(t, err, "unsupported Qwen3 ASR input_audio_format")

	tracker = newQwen3ASRUsageTracker()
	_, _, err = tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":48000}}`))
	require.ErrorContains(t, err, "unsupported Qwen3 ASR sample_rate")

	_, _, err = tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"%%%"}`))
	require.ErrorContains(t, err, "decode Qwen3 ASR PCM audio")
}

const qwenSpeechDone = `{"event_id":"event-speech","type":"response.done","response":{"id":"speech","status":"completed","modalities":["text","audio"],"output":[{"type":"message","content":[{"type":"audio","transcript":"Hello"}]}],"usage":{"total_tokens":3150,"input_tokens":1700,"output_tokens":1450,"input_tokens_details":{"text_tokens":1000,"audio_tokens":700},"output_tokens_details":{"text_tokens":200,"audio_tokens":1250}}},"provider_extension":{"keep":true}}`
const qwenTextDone = `{"event_id":"event-text","type":"response.done","response":{"id":"text","status":"completed","modalities":["text","audio"],"usage":{"total_tokens":1200,"input_tokens":1000,"output_tokens":200,"input_tokens_details":{"text_tokens":1000},"output_tokens_details":{"text_tokens":200}}}}`

// Sanitized usage and output shape from the 2026-11-06 Audio Plus response:
// output_tokens=39, while the reported text/audio breakdown sums to 38.
const qwenResidualSpeechDone = `{"event_id":"event-residual","type":"response.done","response":{"id":"residual","status":"completed","modalities":["text","audio"],"output":[{"type":"message","content":[{"type":"audio","transcript":"hello"}]}],"usage":{"total_tokens":131,"input_tokens":92,"output_tokens":39,"input_tokens_details":{"text_tokens":30,"audio_tokens":62},"output_tokens_details":{"text_tokens":9,"audio_tokens":29}}}}`

func TestQwenRealtimeModelsAndBeijingConfiguration(t *testing.T) {
	for _, model := range []string{"qwen3.8-omni-flash-realtime", "qwen-audio-3.1-realtime-plus", "qwen-audio-3.1-realtime-flash"} {
		t.Run(model, func(t *testing.T) {
			assert.Contains(t, (&Adaptor{}).GetModelList(), model)
			assert.Contains(t, ali.ModelList, model)
			assert.Contains(t, ali_dashscope.ModelList, model)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://workspace.cn-beijing.maas.aliyuncs.com/", UpstreamModelName: model, ApiKey: "upstream-key", Organization: "workspace",
			}}
			requestURL, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "wss://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/realtime?model="+model, requestURL)
			header := http.Header{}
			require.NoError(t, (&Adaptor{}).SetupRequestHeader(nil, &header, info))
			assert.Equal(t, "Bearer upstream-key", header.Get("Authorization"))
			assert.Equal(t, "workspace", header.Get("X-DashScope-WorkSpace"))
			for _, invalidURL := range []string{"https://dashscope.aliyuncs.com", "http://workspace.cn-beijing.maas.aliyuncs.com", "https://workspace.ap-southeast-1.maas.aliyuncs.com", "https://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/realtime", "https:///missing-host"} {
				info.ChannelBaseUrl = invalidURL
				_, err := (&Adaptor{}).GetRequestURL(info)
				require.Error(t, err)
			}
			info.ChannelBaseUrl = "https://other.cn-beijing.maas.aliyuncs.com"
			_, err = (&Adaptor{}).GetRequestURL(info)
			require.ErrorContains(t, err, "workspace header")
			info.ChannelBaseUrl = "https://custom-proxy.example"
			_, err = (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err, "custom gateway endpoints remain supported")
		})
	}
}

func TestQwenRealtimeMixedTurnsAndBuiltinPrices(t *testing.T) {
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	saved := *settings
	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		*settings = saved
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
	})
	*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}}
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	for _, tc := range []struct {
		model                               string
		speechQuota, mixedQuota, textTokens int
	}{
		{"qwen3.8-omni-flash-realtime", 1526, 1696, 400},
		{"qwen-audio-3.1-realtime-plus", 15162, 16056, 200},
		{"qwen-audio-3.1-realtime-flash", 1463, 1632, 200},
	} {
		t.Run(tc.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
			var tracker qwenRealtimeUsageTracker
			raw := &dto.RealtimeUsage{}
			expression, ok := billing_setting.GetBillingExpr(tc.model)
			require.True(t, ok)
			snapshot := &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000}
			for i, payload := range []string{qwenSpeechDone, strings.Replace(qwenSpeechDone, "event-speech", "redelivery", 1), qwenTextDone} {
				var event QwenRealtimeEvent
				require.NoError(t, common.UnmarshalJsonStr(payload, &event))
				updated, err := tracker.consume(info, &event, raw)
				require.NoError(t, err)
				assert.Equal(t, i != 1, updated)
				params := service.BuildTieredRealtimeTokenParams(info.RealtimeBillingUsage, billingexpr.UsedVars(expression))
				result, err := billingexpr.ComputeTieredQuota(snapshot, params)
				require.NoError(t, err)
				wantQuota := tc.speechQuota
				if i == 2 {
					wantQuota = tc.mixedQuota
				}
				assert.Equal(t, wantQuota, result.ActualQuotaAfterGroup)
				assert.Equal(t, 200, *event.Response.Usage.OutputTokensDetails.TextTokens, "normalization must not change upstream usage")
			}
			assert.Equal(t, 4350, raw.TotalTokens)
			assert.Equal(t, 2700, raw.InputTokens, "every turn includes billable history; do not difference input totals")
			assert.Equal(t, 1650, raw.OutputTokens)
			assert.Equal(t, 400, raw.OutputTokenDetails.TextTokens)
			assert.Equal(t, tc.textTokens, info.RealtimeBillingUsage.OutputTokenDetails.TextTokens)
			assert.Equal(t, 2, info.RealtimeResponseCount)
			assert.Equal(t, 1, info.RealtimeDuplicateResponses)
			assert.False(t, info.RealtimeUsageIncomplete)

			settings.BillingMode[tc.model] = billing_setting.BillingModeTieredExpr
			settings.BillingExpr[tc.model] = `tier("custom", p * 7)`
			custom, ok := billing_setting.GetBillingExpr(tc.model)
			require.True(t, ok)
			assert.Equal(t, settings.BillingExpr[tc.model], custom)
			delete(settings.BillingMode, tc.model)
			delete(settings.BillingExpr, tc.model)
			ratios, err := common.Marshal(map[string]float64{tc.model: 0})
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratios)))
			assert.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode(tc.model))
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
		})
	}
}

func TestQwenRealtimeUsageBoundariesAndCancellation(t *testing.T) {
	for _, tc := range []struct{ name, usage string }{
		{"negative", `{"input_tokens":-1,"output_tokens":0}`},
		{"huge", `{"input_tokens":2147483648,"output_tokens":0}`},
		{"sum overflow", `{"input_tokens":2147483647,"output_tokens":1}`},
		{"missing total", `{"output_tokens":0}`},
		{"missing details", `{"input_tokens":5,"output_tokens":0}`},
		{"excess modality", `{"input_tokens":5,"output_tokens":0,"input_tokens_details":{"text_tokens":6}}`},
		{"negative modality", `{"input_tokens":5,"output_tokens":0,"input_tokens_details":{"text_tokens":-5,"audio_tokens":10}}`},
		{"inconsistent total", `{"input_tokens":5,"output_tokens":0,"total_tokens":6,"input_tokens_details":{"text_tokens":5}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage QwenRealtimeUsage
			require.NoError(t, common.UnmarshalJsonStr(tc.usage, &usage))
			_, err := usage.realtimeUsage()
			require.Error(t, err)
		})
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "qwen-audio-3.1-realtime-plus"}}
	var tracker qwenRealtimeUsageTracker
	raw := &dto.RealtimeUsage{}
	for _, payload := range []string{qwenSpeechDone, `{"type":"response.done","response":{"id":"cancelled","status":"cancelled"}}`, qwenTextDone} {
		var event QwenRealtimeEvent
		require.NoError(t, common.UnmarshalJsonStr(payload, &event))
		_, err := tracker.consume(info, &event, raw)
		require.NoError(t, err)
	}
	assert.Equal(t, 4350, raw.TotalTokens)
	assert.True(t, info.RealtimeUsageIncomplete)
	require.Len(t, info.RealtimeUsageDiagnostics, 1)
	assert.Equal(t, "cancelled", info.RealtimeUsageDiagnostics[0].Status)
	assert.Equal(t, "cancelled", info.RealtimeUsageDiagnostics[0].ResponseID)

	var event QwenRealtimeEvent
	require.NoError(t, common.UnmarshalJsonStr(strings.Replace(qwenSpeechDone, `"id":"speech"`, `"id":"overflow"`, 1), &event))
	full := &dto.RealtimeUsage{TotalTokens: common.MaxQuota}
	updated, err := tracker.consume(info, &event, full)
	require.ErrorContains(t, err, "exceeds the supported range")
	assert.False(t, updated)
	assert.Equal(t, common.MaxQuota, full.TotalTokens)
}

func TestQwenRealtimeUsageKeepsUnclassifiedTokensBillable(t *testing.T) {
	secondObservedResponse := strings.NewReplacer(`"total_tokens":131`, `"total_tokens":134`, `"output_tokens":39`, `"output_tokens":42`, `"text_tokens":9`, `"text_tokens":10`, `"audio_tokens":29`, `"audio_tokens":31`).Replace(qwenResidualSpeechDone)
	for _, tc := range []struct {
		name, model, payload                  string
		inputUnclassified, outputUnclassified int
		billableText, quota                   int
	}{
		{"observed 131 tokens", "qwen-audio-3.1-realtime-plus", qwenResidualSpeechDone, 0, 1, 1, 483},
		{"observed 134 tokens", "qwen-audio-3.1-realtime-plus", secondObservedResponse, 0, 1, 1, 503},
		{"flash output remainder", "qwen-audio-3.1-realtime-flash", qwenResidualSpeechDone, 0, 1, 1, 54},
		{"omni keeps reported text and remainder", "qwen3.8-omni-flash-realtime", qwenResidualSpeechDone, 0, 1, 10, 57},
		{"input and output remainders", "qwen-audio-3.1-realtime-plus", strings.Replace(qwenResidualSpeechDone, `"text_tokens":30`, `"text_tokens":29`, 1), 1, 1, 1, 483},
		{"text only response remainder", "qwen-audio-3.1-realtime-plus", strings.NewReplacer(`"type":"audio"`, `"type":"text"`, `"text_tokens":9`, `"text_tokens":38`, `"audio_tokens":29`, `"audio_tokens":0`).Replace(qwenResidualSpeechDone), 0, 1, 39, 288},
		{"explicit zero text is not inferred", "qwen-audio-3.1-realtime-plus", strings.Replace(qwenResidualSpeechDone, `"text_tokens":9`, `"text_tokens":0`, 1), 0, 10, 10, 507},
		{"missing text is not inferred", "qwen-audio-3.1-realtime-plus", strings.Replace(qwenResidualSpeechDone, `"text_tokens":9,`, ``, 1), 0, 10, 10, 507},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event QwenRealtimeEvent
			require.NoError(t, common.UnmarshalJsonStr(tc.payload, &event))
			original, err := common.Marshal(event.Response.Usage)
			require.NoError(t, err)
			billing := &qwenRealtimeTestBilling{}
			expression, ok := billing_setting.GetBillingExpr(tc.model)
			require.True(t, ok)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}, Billing: billing,
				TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000},
			}
			var tracker qwenRealtimeUsageTracker
			raw := &dto.RealtimeUsage{}
			updated, err := tracker.consume(info, &event, raw)
			require.NoError(t, err)
			require.True(t, updated)
			assert.Equal(t, *event.Response.Usage.TotalTokens, raw.TotalTokens)
			assert.Equal(t, *event.Response.Usage.OutputTokens, raw.OutputTokens)
			assert.Equal(t, tc.billableText, info.RealtimeBillingUsage.OutputTokenDetails.TextTokens)
			assert.Equal(t, raw.InputTokens, info.RealtimeBillingUsage.InputTokens)
			assert.True(t, info.RealtimeUsageIncomplete)
			require.Len(t, info.RealtimeUsageDiagnostics, 1)
			assert.Equal(t, tc.inputUnclassified, info.RealtimeUsageDiagnostics[0].InputUnclassifiedTokens)
			assert.Equal(t, tc.outputUnclassified, info.RealtimeUsageDiagnostics[0].OutputUnclassifiedTokens)
			assert.Equal(t, "completed", info.RealtimeUsageDiagnostics[0].Status)
			ctx, _ := gin.CreateTestContext(nil)
			require.NoError(t, service.ReserveWssUsage(ctx, info, raw))
			assert.Equal(t, []int{tc.quota}, billing.targets)
			params := service.BuildTieredRealtimeTokenParams(info.RealtimeBillingUsage, billingexpr.UsedVars(expression))
			assert.Equal(t, float64(tc.billableText), params.C)
			settled, quota, _ := service.TryTieredSettle(info, params)
			require.True(t, settled)
			assert.Equal(t, tc.quota, quota, "final settlement must use the same remainder as reservation")
			updated, err = tracker.consume(info, &event, raw)
			require.NoError(t, err)
			assert.False(t, updated)
			require.Len(t, info.RealtimeUsageDiagnostics, 1, "redelivery must not add another remainder")
			assert.Equal(t, 1, info.RealtimeResponseCount)
			assert.Equal(t, 1, info.RealtimeDuplicateResponses)
			unchanged, err := common.Marshal(event.Response.Usage)
			require.NoError(t, err)
			assert.JSONEq(t, string(original), string(unchanged), "preserve provider counts and missing-versus-zero fields")
		})
	}
}

func TestQwenRealtimeOmniModalitiesAndToolText(t *testing.T) {
	for _, tc := range []struct {
		name, model, output string
		wantText            int
		incomplete          bool
	}{
		{"speech transcript", "qwen-audio-3.1-realtime-flash", `[{"type":"message","content":[{"type":"audio","transcript":"hello"}]}]`, 0, false},
		{"tool text stays billable", "qwen-audio-3.1-realtime-flash", `[{"type":"function_call","name":"weather","arguments":"{}"}]`, 200, true},
		{"independent text stays billable", "qwen-audio-3.1-realtime-flash", `[{"type":"message","content":[{"type":"text","text":"hello"}]}]`, 200, true},
		{"Omni charges transcripts", "qwen3.8-omni-flash-realtime", `[]`, 200, false},
		{"legacy Omni", "qwen3-omni-flash-realtime", `[]`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
			var tracker qwenRealtimeUsageTracker
			var event QwenRealtimeEvent
			require.NoError(t, common.UnmarshalJsonStr(qwenSpeechDone, &event))
			require.NoError(t, common.UnmarshalJsonStr(tc.output, &event.Response.Output))
			event.Response.Usage.InputTokensDetails.ImageTokens = common.GetPointer(100)
			event.Response.Usage.InputTokensDetails.VideoTokens = common.GetPointer(200)
			event.Response.Usage.InputTokensDetails.TextTokens = common.GetPointer(700)
			raw := &dto.RealtimeUsage{}
			_, err := tracker.consume(info, &event, raw)
			require.NoError(t, err)
			assert.Equal(t, 200, raw.OutputTokenDetails.TextTokens)
			assert.Equal(t, tc.wantText, info.RealtimeBillingUsage.OutputTokenDetails.TextTokens)
			assert.Equal(t, 100, raw.InputTokenDetails.ImageTokens)
			assert.Equal(t, 200, raw.InputTokenDetails.VideoTokens)
			assert.Equal(t, tc.incomplete, info.RealtimeUsageIncomplete)
		})
	}
}

func TestQwenRealtimeClientConfiguration(t *testing.T) {
	for _, message := range []string{
		`{"type":"session.update","session":{"voice":"longanqian_v3.1","turn_detection":{"type":"smart_turn_v2","threshold":0},"enable_search":false,"max_response_output_tokens":0}}`,
		`{"type":"session.update","session":{"voice":"Tina","audio":{"output":{"voice":"other"}},"turn_detection":null,"provider_extension":false}}`,
		`{"type":"response.create","response":{"modalities":["text"],"max_response_output_tokens":"inf"}}`,
	} {
		require.NoError(t, validateQwenRealtimeClientEvent([]byte(message)))
	}
	for _, message := range []string{
		`{"type":"session.update","session":{"enable_search":true}}`,
		`{"type":"session.update","session":{"sample_rate":24000}}`,
		`{"type":"session.update","session":{"input_audio_format":"mp3"}}`,
		`{"type":"response.create","response":{"max_response_output_tokens":-1}}`,
		`{"type":"response.create","response":{"max_response_output_tokens":18446744073686646784}}`,
	} {
		require.Error(t, validateQwenRealtimeClientEvent([]byte(message)))
	}
}

// The fake records cumulative reservation targets; it never charges per event.
type qwenRealtimeTestBilling struct {
	targets []int
	fail    bool
}

func (b *qwenRealtimeTestBilling) ReserveBeforeSubmit(quota int) error { return b.Reserve(quota) }

func (b *qwenRealtimeTestBilling) Reserve(quota int) error {
	b.targets = append(b.targets, quota)
	if b.fail {
		return errors.New("insufficient quota")
	}
	return nil
}
func (*qwenRealtimeTestBilling) Settle(int) error    { return nil }
func (*qwenRealtimeTestBilling) Refund(*gin.Context) {}
func (*qwenRealtimeTestBilling) NeedsRefund() bool   { return false }
func (b *qwenRealtimeTestBilling) GetPreConsumedQuota() int {
	if b.fail || len(b.targets) == 0 {
		return 0
	}
	return b.targets[len(b.targets)-1]
}

// A real pair of WebSocket peers verifies forwarding and the handler's joined
// shutdown. No provider connection or paid model call is made by this fixture.
func TestQwenRealtimeWebSocketForwardingAndShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, model              string
		failReserve, closeClient bool
		wantText, wantRounds     int
		unfinished               bool
		usageRemainder           bool
	}{
		{"Omni mixed turns", "qwen3.8-omni-flash-realtime", false, false, 400, 2, false, false},
		{"Audio Plus mixed turns", "qwen-audio-3.1-realtime-plus", false, false, 200, 2, false, false},
		{"Audio Flash mixed turns", "qwen-audio-3.1-realtime-flash", false, false, 200, 2, false, false},
		{"reservation failure preserves consumed usage", "qwen-audio-3.1-realtime-flash", true, false, 0, 1, false, false},
		{"client disconnect preserves confirmed usage", "qwen-audio-3.1-realtime-flash", false, true, 0, 1, false, false},
		{"upstream closes before terminal usage", "qwen-audio-3.1-realtime-flash", false, false, 0, 1, true, false},
		{"Audio Plus forwards observed usage remainder", "qwen-audio-3.1-realtime-plus", false, false, 1, 1, false, true},
		{"Audio Flash forwards usage remainder", "qwen-audio-3.1-realtime-flash", false, false, 1, 1, false, true},
		{"Omni forwards usage remainder", "qwen3.8-omni-flash-realtime", false, false, 10, 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{qwenSpeechDone, strings.Replace(qwenSpeechDone, "event-speech", "duplicate", 1), qwenTextDone}
			if tc.unfinished {
				events = []string{qwenSpeechDone, `{"type":"response.created","response":{"id":"unfinished","status":"in_progress"}}`}
			}
			if tc.usageRemainder {
				events = []string{qwenResidualSpeechDone, strings.Replace(qwenResidualSpeechDone, "event-residual", "event-redelivered", 1)}
			}
			upgrader := websocket.Upgrader{}
			upstreamMessages := make(chan string, 1)
			upstreamErrors := make(chan error, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					upstreamErrors <- err
					return
				}
				defer conn.Close()
				_, payload, err := conn.ReadMessage()
				if err != nil {
					upstreamErrors <- err
					return
				}
				upstreamMessages <- string(payload)
				for _, event := range events {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
						return
					}
					// The client acknowledgement keeps the next turn after the preceding forward.
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
			}))
			defer upstream.Close()
			billing := &qwenRealtimeTestBilling{fail: tc.failReserve}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL, UpstreamModelName: tc.model}, Billing: billing}
			const expression = `tier("test", p + ai * 2 + c * 3 + ao * 4)`
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000}
			result := make(chan *dto.RealtimeUsage, 1)
			relayErrors := make(chan error, 1)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				client, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					relayErrors <- err
					return
				}
				defer client.Close()
				ctx, _ := gin.CreateTestContext(w)
				ctx.Request = r
				target, err := (&Adaptor{}).DoRequest(ctx, info, nil)
				if err != nil {
					relayErrors <- err
					return
				}
				info.ClientWs, info.TargetWs = client, target.(*websocket.Conn)
				apiErr, usage := QwenRealtimeHandler(ctx, info)
				if apiErr != nil {
					relayErrors <- apiErr
					return
				}
				result <- usage
			}))
			defer gateway.Close()
			client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http"), nil)
			require.NoError(t, err)
			defer client.Close()
			require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
			const configMessage = `{"type":"session.update","session":{"voice":"custom","audio":{"output":{"voice":"Tina"}},"turn_detection":{"type":"smart_turn_v2","threshold":0},"enable_search":false,"extension":false}}`
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(configMessage)))
			select {
			case got := <-upstreamMessages:
				assert.Equal(t, configMessage, got)
			case err := <-upstreamErrors:
				require.NoError(t, err)
			case err := <-relayErrors:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("upstream did not receive client event")
			}
			for i, want := range events {
				_, message, err := client.ReadMessage()
				require.NoError(t, err)
				if tc.failReserve {
					assert.Contains(t, string(message), "insufficient quota")
					break
				}
				assert.Equal(t, want, string(message), "provider extensions and raw usage must remain intact")
				if tc.closeClient && i == 0 {
					require.NoError(t, client.Close())
					break
				}
				require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"input_audio_buffer.clear"}`)))
			}
			select {
			case raw := <-result:
				assert.Equal(t, tc.wantRounds, info.RealtimeResponseCount)
				assert.Equal(t, tc.unfinished || tc.usageRemainder, info.RealtimeUsageIncomplete)
				if tc.unfinished {
					require.Len(t, info.RealtimeUsageDiagnostics, 1)
					assert.Equal(t, "unfinished", info.RealtimeUsageDiagnostics[0].ResponseID)
					assert.Equal(t, "in_progress", info.RealtimeUsageDiagnostics[0].Status)
				}
				assert.Equal(t, tc.wantText, info.RealtimeBillingUsage.OutputTokenDetails.TextTokens)
				wantTokens := 3150
				if tc.wantRounds == 2 {
					wantTokens = 4350
				}
				if tc.usageRemainder {
					wantTokens = 131
					require.Len(t, info.RealtimeUsageDiagnostics, 1)
					assert.Equal(t, 1, info.RealtimeUsageDiagnostics[0].OutputUnclassifiedTokens)
					assert.Equal(t, 1, info.RealtimeDuplicateResponses)
					wantQuota := 137
					if tc.model == "qwen3.8-omni-flash-realtime" {
						wantQuota = 150
					}
					assert.Equal(t, []int{wantQuota}, billing.targets)
				}
				assert.Equal(t, wantTokens, raw.TotalTokens)
				assert.Len(t, billing.targets, tc.wantRounds, "duplicate events must not reserve again")
				if tc.wantRounds == 2 {
					wantTargets := []int{3700, 4500}
					if tc.model == "qwen3.8-omni-flash-realtime" {
						wantTargets = []int{4000, 4800}
					}
					assert.Equal(t, wantTargets, billing.targets)
				}
			case err := <-relayErrors:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("realtime handler did not stop both readers")
			}
		})
	}
}
