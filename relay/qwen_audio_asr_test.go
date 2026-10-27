package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
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
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/ali_dashscope"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func qwenASRWAV(seconds int) []byte {
	data := make([]byte, 44+seconds*16000)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return data
}

func qwenASRContext(t *testing.T, fields map[string]string, files ...[]byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	require.NoError(t, i18n.Init())
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	for _, data := range files {
		file, err := writer.CreateFormFile("file", "untrusted-name.mp3")
		require.NoError(t, err)
		_, err = file.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
	return ctx, recorder
}

func TestQwenAudioASRRequestContract(t *testing.T) {
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	wav := qwenASRWAV(1)
	ctx, _ := qwenASRContext(t, map[string]string{
		"model": ali_dashscope.QwenAudioASRModel, "language": "zh",
		"parameters":       `{"sample_rate":"8000","speaker_diarization_enabled":false,"keep_dialect":false,"vocabulary":{"百炼":50}}`,
		"context_messages": `[{"role":"user","content":[{"type":"input_text","text":"这是音频背景"}]},{"role":"assistant","content":[{"type":"text","text":"已知内容"}]}]`,
	}, wav)
	request, err := ali_dashscope.PrepareQwenAudioASRRequest(ctx)
	require.NoError(t, err)
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"speaker_diarization_enabled":false`)
	assert.Contains(t, string(encoded), `"keep_dialect":false`)
	assert.Contains(t, string(encoded), `"format":"wav"`)
	assert.Contains(t, string(encoded), `"language_hints":["zh"]`)
	require.Len(t, request.Input.Messages, 3)
	uri := request.Input.Messages[2].Content[0].InputAudio.Data
	_, audio, ok := strings.Cut(uri, ";base64,")
	require.True(t, ok)
	decoded, err := base64.StdEncoding.DecodeString(audio)
	require.NoError(t, err)
	assert.Equal(t, wav, decoded)
	assert.Contains(t, ctx.GetString(ali_dashscope.QwenASRRequestLogKey), "[audio omitted]")
	assert.NotContains(t, ctx.GetString(ali_dashscope.QwenASRRequestLogKey), audio)
	cached, err := ali_dashscope.PrepareQwenAudioASRRequest(ctx)
	require.NoError(t, err)
	assert.Same(t, request, cached)

	oldCount := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = oldCount })
	for _, tc := range []struct {
		name   string
		fields map[string]string
		files  [][]byte
		status int
	}{
		{"missing file", nil, nil, 400},
		{"two files", nil, [][]byte{wav, wav}, 400},
		{"invalid audio", nil, [][]byte{[]byte("not audio")}, 400},
		{"truncated WAV", nil, [][]byte{wav[:100]}, 400},
		{"zero duration", nil, [][]byte{qwenASRWAV(0)}, 400},
		{"over duration", nil, [][]byte{qwenASRWAV(301)}, 400},
		{"over encoded size", nil, [][]byte{make([]byte, 7_500_001)}, 413},
		{"stream", map[string]string{"stream": "true"}, [][]byte{wav}, 400},
		{"SSE", map[string]string{"stream_format": "sse"}, [][]byte{wav}, 400},
		{"subtitles", map[string]string{"response_format": "srt"}, [][]byte{wav}, 400},
		{"remote input", map[string]string{"audio_url": "https://example.com/a.wav"}, [][]byte{wav}, 400},
		{"invalid parameters", map[string]string{"parameters": "[]"}, [][]byte{wav}, 400},
		{"unknown parameter", map[string]string{"parameters": `{"anything":true}`}, [][]byte{wav}, 400},
		{"null parameter", map[string]string{"parameters": `{"keep_dialect":null}`}, [][]byte{wav}, 400},
		{"numeric sample rate", map[string]string{"parameters": `{"sample_rate":8000}`}, [][]byte{wav}, 400},
		{"mismatched sample rate", map[string]string{"parameters": `{"sample_rate":"16000"}`}, [][]byte{wav}, 400},
		{"mismatched format", map[string]string{"parameters": `{"format":"mp3"}`}, [][]byte{wav}, 400},
		{"invalid weight", map[string]string{"parameters": `{"vocabulary":{"word":6}}`}, [][]byte{wav}, 400},
		{"language conflict", map[string]string{"language": "en", "parameters": `{"language_hints":["zh"]}`}, [][]byte{wav}, 400},
		{"too many languages", map[string]string{"parameters": `{"language_hints":["zh","en","ja","ko","fr"]}`}, [][]byte{wav}, 400},
		{"long context", map[string]string{"prompt": strings.Repeat("中", 401)}, [][]byte{wav}, 400},
		{"two context forms", map[string]string{"prompt": "hi", "context_messages": "[]"}, [][]byte{wav}, 400},
		{"audio context", map[string]string{"context_messages": `[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"x"}}]}]`}, [][]byte{wav}, 400},
		{"assistant without user", map[string]string{"context_messages": `[{"role":"assistant","content":[{"type":"text","text":"hi"}]}]`}, [][]byte{wav}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := qwenASRContext(t, tc.fields, tc.files...)
			info := &relaycommon.RelayInfo{OriginModelName: ali_dashscope.QwenAudioASRModel, RelayMode: relayconstant.RelayModeAudioTranscription, Request: &dto.AudioRequest{Model: ali_dashscope.QwenAudioASRModel}}
			apiErr := PrepareRequestBilling(c, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)
			assert.Nil(t, info.Billing, "invalid input must fail before reservation even with token estimation disabled")
		})
	}
	c, _ := qwenASRContext(t, map[string]string{"prompt": strings.Repeat("中", 400)}, wav)
	_, err = ali_dashscope.PrepareQwenAudioASRRequest(c)
	require.NoError(t, err)
}

func TestQwenAudioASRResponseContract(t *testing.T) {
	const output = `"request_id":"asr-id","output":{"text":"完整文本。","sentences":[{"sentence_id":0,"channel_id":0,"text":"partial"},{"sentence_id":0,"channel_id":0,"sentence_end":true,"text":"完整文本。","begin_time":0,"end_time":1234,"speaker_id":0,"words":[{"text":"完整","punctuation":"。","begin_time":0,"end_time":1234,"speaker_id":null,"fixed":false}]},{"sentence_id":1,"channel_id":0,"text":"第二句","speaker_id":null}]}`
	const validUsage = `{"input_tokens":2000,"output_tokens":200,"total_tokens":2200,"duration":1.234}`
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	for _, format := range []string{"json", "text", "verbose_json", "dashscope"} {
		t.Run(format, func(t *testing.T) {
			body := "{" + output + `,"usage":` + validUsage + "}"
			c, rec := qwenASRContext(t, nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ali_dashscope.QwenAudioASRModel}, Request: &dto.AudioRequest{ResponseFormat: format}}
			apiErr, usage := ali_dashscope.ASRHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(body))}, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 2000, usage.PromptTokens)
			assert.Equal(t, 200, usage.CompletionTokens)
			assert.Equal(t, 2200, usage.TotalTokens)
			assert.Zero(t, usage.PromptTokensDetails.AudioTokens)
			assert.Equal(t, "dashscope", usage.UsageSource)
			assert.Equal(t, "asr-id", c.GetString(common.UpstreamRequestIdKey))
			assert.JSONEq(t, body, common.GetContextKeyString(c, constant.ContextKeySdkResponseStr))
			switch format {
			case "json":
				assert.JSONEq(t, `{"text":"完整文本。"}`, rec.Body.String())
			case "text":
				assert.Equal(t, "完整文本。", rec.Body.String())
			case "dashscope":
				assert.JSONEq(t, body, rec.Body.String())
			case "verbose_json":
				var result struct {
					Duration float64
					Segments []struct {
						SpeakerID  *int `json:"speaker_id"`
						Start, End float64
						Words      []struct {
							SpeakerID *int `json:"speaker_id"`
							Fixed     *bool
						}
					}
				}
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &result))
				require.Len(t, result.Segments, 2)
				assert.Nil(t, result.Segments[1].SpeakerID)
				require.NotNil(t, result.Segments[0].SpeakerID)
				assert.Zero(t, *result.Segments[0].SpeakerID)
				assert.Equal(t, 1.234, result.Duration)
				assert.Equal(t, 1.234, result.Segments[0].End)
				require.Len(t, result.Segments[0].Words, 1)
				assert.Nil(t, result.Segments[0].Words[0].SpeakerID)
				require.NotNil(t, result.Segments[0].Words[0].Fixed)
				assert.False(t, *result.Segments[0].Words[0].Fixed)
				assert.NotContains(t, rec.Body.String(), "avg_logprob")
			}
		})
	}
	for _, tc := range []struct{ name, usage, fallback string }{
		{"missing", "null", "missing_usage"}, {"partial", `{"duration":1}`, "missing_usage"},
		{"negative", `{"input_tokens":-1,"output_tokens":2}`, "invalid_usage"},
		{"wrong type", `{"input_tokens":"2","output_tokens":2}`, "invalid_usage"},
		{"overflow", `{"input_tokens":9223372036854775807,"output_tokens":1}`, "invalid_usage"},
		{"zero", `{"input_tokens":0,"output_tokens":0}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := qwenASRContext(t, nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ali_dashscope.QwenAudioASRModel}}
			apiErr, usage := ali_dashscope.ASRHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader("{" + output + `,"usage":` + tc.usage + "}"))}, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Zero(t, usage.TotalTokens)
			assert.Equal(t, tc.fallback, info.UsageBillingFallback)
		})
	}
	t.Run("total mismatch uses input and output", func(t *testing.T) {
		c, _ := qwenASRContext(t, nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ali_dashscope.QwenAudioASRModel}}
		body := `{"output":{"text":"text"},"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":999}}`
		apiErr, usage := ali_dashscope.ASRHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(body))}, info)
		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		assert.Equal(t, 2, usage.PromptTokens)
		assert.Equal(t, 3, usage.CompletionTokens)
		assert.Equal(t, 5, usage.TotalTokens)
		assert.Empty(t, info.UsageBillingFallback)
	})
	for _, body := range []string{`{}`, `<html>upstream error</html>`, `{"code":"InvalidParameter","message":"bad audio"}`, `{"output":{}}`} {
		c, _ := qwenASRContext(t, nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ali_dashscope.QwenAudioASRModel}}
		apiErr, _ := ali_dashscope.ASRHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(body))}, info)
		require.NotNil(t, apiErr)
		assert.False(t, c.Writer.Written())
	}
}

func TestQwenAudioASRRelayBillingDatabaseMatrix(t *testing.T) {
	require.NoError(t, i18n.Init())
	t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupRelayBillingDatabase(t, dialect)
			extraModels := []any{&model.QuotaData{}, &model.QuotaDataEventRecord{}, &model.QuotaDataAppliedEvent{}, &model.CliendUserQuota{}, &model.ProjectAllocation{}}
			require.NoError(t, db.AutoMigrate(extraModels...))
			oldExport, oldCount := common.DataExportEnabled, constant.CountToken
			common.DataExportEnabled, constant.CountToken = true, true
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"double":2,"free":0}`))
			t.Cleanup(func() {
				require.NoError(t, model.SaveQuotaDataCache())
				common.DataExportEnabled, constant.CountToken = oldExport, oldCount
				require.NoError(t, db.Migrator().DropTable(extraModels...))
			})
			for index, tc := range []struct {
				name                           string
				channel                        int
				usage                          string
				want, p, c                     int
				alias, disabled                bool
				lateAlias, project, paidToFree bool
				noLogs, insufficient           bool
				group                          string
				status                         int
			}{
				{name: "Ali actual usage", channel: constant.ChannelTypeAli, usage: `{"input_tokens":2000,"output_tokens":200,"duration":1}`, want: 151, p: 2000, c: 200},
				{name: "DashScope actual usage", channel: constant.ChannelTypeAliDashScope, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 151, p: 2000, c: 200},
				{name: "explicit zero refunds", channel: constant.ChannelTypeAliDashScope, usage: `{"input_tokens":0,"output_tokens":0}`},
				{name: "missing retains reservation", channel: constant.ChannelTypeAli, usage: `null`, want: 405},
				{name: "invalid retains reservation", channel: constant.ChannelTypeAliDashScope, usage: `{"input_tokens":-1,"output_tokens":0}`, want: 405},
				{name: "alias configured price", channel: constant.ChannelTypeAli, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 1200, p: 2000, c: 200, alias: true},
				{name: "estimation disabled", channel: constant.ChannelTypeAliDashScope, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 151, p: 2000, c: 200, disabled: true},
				{name: "group multiplier", channel: constant.ChannelTypeAli, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 302, p: 2000, c: 200, group: "double"},
				{name: "paid to free missing usage", channel: constant.ChannelTypeAliDashScope, usage: `null`, paidToFree: true},
				{name: "free missing usage", channel: constant.ChannelTypeAliDashScope, usage: `null`, group: "free"},
				{name: "project attribution", channel: constant.ChannelTypeAliDashScope, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 151, p: 2000, c: 200, project: true},
				{name: "late channel alias", channel: constant.ChannelTypeAli, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 1200, p: 2000, c: 200, alias: true, lateAlias: true},
				{name: "logging disabled", channel: constant.ChannelTypeAli, usage: `{"input_tokens":2000,"output_tokens":200}`, want: 151, p: 2000, c: 200, noLogs: true},
				{name: "insufficient wallet", channel: constant.ChannelTypeAli, insufficient: true},
				{name: "failure refunds", channel: constant.ChannelTypeAliDashScope, status: 503},
			} {
				t.Run(tc.name, func(t *testing.T) {
					constant.CountToken = !tc.disabled
					if tc.noLogs {
						t.Setenv("SAVE_REQUEST_RESPONSE", "false")
					}
					user := model.User{Username: fmt.Sprintf("asr%d", index), AffCode: fmt.Sprintf("a%d", index), Quota: 100000, Group: "default", Status: common.UserStatusEnabled}
					if tc.insufficient {
						user.Quota = 1
					}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("asr-key-%d", index), RemainQuota: 100000, Status: common.TokenStatusEnabled}
					require.NoError(t, db.Create(&token).Error)
					channel := model.Channel{Type: tc.channel, Name: tc.name}
					require.NoError(t, db.Create(&channel).Error)
					if tc.project {
						require.NoError(t, db.Create(&model.ProjectAllocation{ProjectId: index + 10, PlanId: 0, ClientUserId: "asr-user", AllocatedQuota: 10}).Error)
						require.NoError(t, db.Create(&model.ProjectAllocation{ProjectId: index + 10, PlanId: 2, ClientUserId: "asr-user", AllocatedQuota: 10, UsedQuota: 50}).Error)
					}
					upstreamBody := `{"request_id":"asr-request","output":{"text":"已识别"},"usage":` + tc.usage + `}`
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						assert.Equal(t, "/api/v1/services/aigc/multimodal-generation/generation", r.URL.Path)
						assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
						assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
						assert.Equal(t, "disable", r.Header.Get("X-DashScope-SSE"))
						assert.Empty(t, r.Header.Get("X-DashScope-Async"))
						assert.Empty(t, r.Header.Get("X-DashScope-OssResourceResolve"))
						var request ali_dashscope.QwenAudioASRRequest
						assert.NoError(t, common.DecodeJson(r.Body, &request))
						assert.Equal(t, ali_dashscope.QwenAudioASRModel, request.Model)
						assert.Len(t, request.Input.Messages, 1)
						if tc.status != 0 {
							w.WriteHeader(tc.status)
							_, _ = io.WriteString(w, `{"error":{"message":"unavailable","code":503}}`)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, upstreamBody)
					}))
					defer server.Close()
					modelName := ali_dashscope.QwenAudioASRModel
					if tc.alias {
						modelName = "asr-alias"
						settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
						settings.BillingMode[modelName] = "tiered_expr"
						settings.BillingExpr[modelName] = `tier("custom", p * 1 + c * 2)`
					}
					// Two batches must accumulate counters once per request, including zero
					// usage and reservation fallback; settlement must not add a second row.
					for pass := range 2 {
						ctx, rec := qwenASRContext(t, map[string]string{"model": modelName}, qwenASRWAV(1))
						common.SetContextKey(ctx, constant.ContextKeyOriginalModel, modelName)
						common.SetContextKey(ctx, constant.ContextKeyChannelType, tc.channel)
						common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
						common.SetContextKey(ctx, constant.ContextKeyChannelKey, "upstream-key")
						common.SetContextKey(ctx, constant.ContextKeyChannelId, channel.Id)
						common.SetContextKey(ctx, constant.ContextKeyClientUserId, "asr-user")
						common.SetContextKey(ctx, constant.ContextKeyClientScenairo, "transcription")
						ctx.Set(common.RequestIdKey, fmt.Sprintf("%s-%d-%d", dialect, index, pass))
						ctx.Set("username", user.Username)
						if tc.project {
							common.SetContextKey(ctx, constant.ContextKeyProjectName, "asr-project")
							common.SetContextKey(ctx, constant.ContextKeyProjectId, index+10)
							common.SetContextKey(ctx, constant.ContextKeyProjectPlanId, 0)
						}
						if tc.alias && !tc.lateAlias {
							ctx.Set("model_mapping", `{"asr-alias":"qwen-audio-3.1-asr-flash"}`)
						}
						request, err := helper.GetAndValidateRequest(ctx, types.RelayFormatOpenAIAudio)
						require.NoError(t, err)
						info, err := relaycommon.GenRelayInfo(ctx, types.RelayFormatOpenAIAudio, request, nil)
						require.NoError(t, err)
						info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, token.Key
						info.UserGroup, info.UsingGroup = "default", "default"
						if tc.group != "" {
							info.UsingGroup = tc.group
						}
						info.UserSetting.BillingPreference = "wallet_only"
						info.ForcePreConsume = true
						info.StartTime = time.Now()
						prepareErr := PrepareRequestBilling(ctx, info)
						if tc.insufficient {
							require.NotNil(t, prepareErr)
							assert.Zero(t, calls.Load())
							require.NoError(t, db.First(&user, user.Id).Error)
							require.NoError(t, db.First(&token, token.Id).Error)
							assert.Equal(t, 1, user.Quota)
							assert.Equal(t, 100000, token.RemainQuota)
							continue
						}
						require.Nil(t, prepareErr)
						estimate := ali_dashscope.QwenAudioASRInputEstimate
						if tc.disabled {
							estimate = 0
						}
						if !tc.lateAlias {
							assert.Equal(t, estimate, info.GetEstimatePromptTokens())
						}
						if tc.lateAlias {
							ctx.Set("model_mapping", `{"asr-alias":"qwen-audio-3.1-asr-flash"}`)
						}
						if tc.paidToFree {
							info.UsingGroup = "free"
							info.PriceData.GroupRatioInfo.GroupRatio = 0
							require.Nil(t, service.PrepareTieredBillingForSelectedGroup(ctx, info))
						}
						apiErr := AudioHelper(ctx, info)
						if tc.status != 0 {
							require.NotNil(t, apiErr)
							RefundFailedRequestBilling(ctx, info, apiErr)
							require.Eventually(t, func() bool {
								var u model.User
								var tok model.Token
								return db.First(&u, user.Id).Error == nil && db.First(&tok, token.Id).Error == nil && u.Quota == 100000 && tok.RemainQuota == 100000
							}, 5*time.Second, 10*time.Millisecond)
							assert.False(t, ctx.Writer.Written())
							continue
						}
						require.Nil(t, apiErr)
						assert.Equal(t, http.StatusOK, rec.Code)
						assert.JSONEq(t, `{"text":"已识别"}`, rec.Body.String())
						assert.Equal(t, modelName, request.(*dto.AudioRequest).Model)
						if info.Billing != nil {
							require.NoError(t, info.Billing.Settle(tc.want))
						}
						assert.Equal(t, estimate, info.GetEstimatePromptTokens())
						require.NoError(t, db.First(&user, user.Id).Error)
						require.NoError(t, db.First(&token, token.Id).Error)
						assert.Equal(t, 100000-(pass+1)*tc.want, user.Quota)
						assert.Equal(t, user.Quota, token.RemainQuota)
						assert.Equal(t, (pass+1)*tc.want, user.UsedQuota)
						assert.Equal(t, pass+1, user.RequestCount)
						var log model.Log
						require.NoError(t, model.LOG_DB.Where("request_id = ?", ctx.GetString(common.RequestIdKey)).Take(&log).Error)
						assert.Equal(t, tc.want, log.Quota)
						assert.Equal(t, tc.p, log.PromptTokens)
						assert.Equal(t, tc.c, log.CompletionTokens)
						assert.Equal(t, "asr-user", log.ClientUserId)
						assert.Equal(t, "transcription", log.ClientScenairo)
						if tc.project {
							assert.Equal(t, "asr-project", log.ProjectName)
							assert.Zero(t, log.PlanId)
							var allocations []model.ProjectAllocation
							require.NoError(t, db.Where("project_id = ?", index+10).Order("plan_id").Find(&allocations).Error)
							require.Len(t, allocations, 2)
							assert.Equal(t, (pass+1)*tc.want, allocations[0].UsedQuota)
							assert.Equal(t, 50, allocations[1].UsedQuota)
						}
						assert.Equal(t, "asr-request", log.UpstreamRequestId)
						if tc.noLogs {
							assert.Empty(t, log.Request)
							assert.Empty(t, log.Response)
						} else {
							assert.Contains(t, log.Request, "[audio omitted]")
							assert.NotContains(t, log.Request, "base64,")
							assert.JSONEq(t, upstreamBody, log.Response)
						}
						if tc.want == 405 {
							assert.Contains(t, log.Other, `"usage_estimated":true`)
						}
						require.NoError(t, model.SaveQuotaDataCache())
						var buckets []model.QuotaData
						require.NoError(t, db.Where("user_id = ?", user.Id).Find(&buckets).Error)
						require.Len(t, buckets, 1)
						assert.EqualValues(t, pass+1, buckets[0].Count)
						assert.EqualValues(t, (pass+1)*tc.want, buckets[0].Quota)
						assert.EqualValues(t, (pass+1)*tc.p, buckets[0].PromptTokens)
						assert.EqualValues(t, (pass+1)*tc.c, buckets[0].CompletionTokens)
						assert.Equal(t, "asr-user", buckets[0].ClientUserId)
						assert.Equal(t, "transcription", buckets[0].ClientScenairo)
						if tc.project {
							assert.Equal(t, "asr-project", buckets[0].ProjectName)
							assert.Zero(t, buckets[0].PlanId)
						}
					}
				})
			}
		})
	}
}

func TestQwenAudioASRRejectsChat(t *testing.T) {
	require.NoError(t, i18n.Init())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ali_dashscope.QwenAudioASRModel}}
	for _, adaptor := range []interface {
		ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error)
	}{&ali.Adaptor{}, &ali_dashscope.Adaptor{}} {
		_, err := adaptor.ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{Model: ali_dashscope.QwenAudioASRModel})
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 400, apiErr.StatusCode)
	}
}
