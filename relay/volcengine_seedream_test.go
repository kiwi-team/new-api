package relay

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedream55PluginSwitch(t *testing.T) {
	mode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(mode) })
	previous := jsplugin.DefaultRegistry
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	t.Cleanup(func() { jsplugin.DefaultRegistry = previous })
	source, err := builtinplugins.Source("doubao")
	require.NoError(t, err)
	_, err = registry.RegisterFactory(source, jsplugin.Options{Key: "doubao"})
	require.NoError(t, err)
	for _, mode := range []string{"enabled", "doubao disabled", "master disabled", "re-enabled"} {
		t.Run(mode, func(t *testing.T) {
			registry.SetEnabled(mode != "master disabled")
			disabled := []string{}
			if mode == "doubao disabled" {
				disabled = []string{"doubao"}
			}
			registry.SetDisabledFactoryKeys(disabled)
			for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
				engine := gin.New()
				engine.POST(path, middleware.PinTaskPluginEndpoint(), func(c *gin.Context) {
					_, pinned := c.Get(jsplugin.ContextKeyPinnedEndpoint)
					assert.Equal(t, mode == "enabled" || mode == "re-enabled", pinned)
					c.Status(http.StatusNoContent)
				})
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"doubao-seedream-5-5-261028","prompt":"a cat","image":"https://example.com/input.png"}`))
				request.Header.Set("Content-Type", gin.MIMEJSON)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSeedream55NativeFallbackDatabaseMatrix(t *testing.T) {
	mode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(mode) })
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
	registry := jsplugin.DefaultRegistry
	enabled := registry.Enabled()
	registry.SetEnabled(false)
	t.Cleanup(func() { registry.SetEnabled(enabled) })
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	saved := *settings
	*settings = billing_setting.BillingSetting{}
	t.Cleanup(func() { *settings = saved })
	global := model_setting.GetGlobalSettings()
	savedGlobal := *global
	global.PassThroughRequestEnabled = false
	t.Cleanup(func() { *global = savedGlobal })
	service.InitHttpClient()
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db, logDB := setupVolcengineMessagesDatabases(t, dialect)
			metricsModels := []any{&model.SettlementConfig{}, &model.QuotaData{}, &model.QuotaDataEventRecord{}, &model.QuotaDataAppliedEvent{}}
			require.NoError(t, db.AutoMigrate(metricsModels...))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(metricsModels...)) })
			for index, tc := range []struct {
				name, path, fields, response string
				multipart, passthrough       bool
				cny                          float64
			}{
				{"generation", "/v1/images/generations", `"prompt":"a cat","size":"2K"`, `{"created":1,"data":[{"url":"https://example.com/out.png","size":"2048x2048"}],"usage":{"generated_images":1,"output_tokens":100}}`, false, false, .23},
				{"reference edit", "/v1/images/edits", `"prompt":"edit","image":["https://example.com/a.png","https://example.com/b.png"],"size":"1K","metadata":{"watermark":false}`, `{"created":1,"data":[{"url":"https://example.com/out.png","size":"1024x1024"}],"usage":{"input_images":2,"generated_images":1}}`, false, false, .23},
				{"layers and OCR", "/v1/images/edits", `"image":"https://example.com/a.png","metadata":{"layer_decomposition":true,"layer_decomposition_options":{"max_images":100,"text_recognition":true}}`, `{"created":1,"data":[{"url":"https://example.com/base.png","size":"2048x2048"},{"url":"https://example.com/layer.png","size":"1024x1024","text_recognition":{"text":"hello"}}],"usage":{"input_images":1,"generated_images":2}}`, false, false, .41},
				{"Base64 output", "/v1/images/generations", `"prompt":"a cat","response_format":"b64_json","size":"1K"`, `{"created":1,"data":[{"b64_json":"aW1hZ2U=","size":"1024x1024"}],"usage":{"generated_images":1}}`, false, false, .13},
				{"multipart edit", "/v1/images/edits", ``, `{"created":1,"data":[{"url":"https://example.com/out.png","size":"1024x1024"}],"usage":{"input_images":1,"generated_images":1}}`, true, false, .18},
				{"passthrough retains validation and metadata", "/v1/images/generations", `"prompt":"sticker","background":"transparent","output_format":"png","metadata":{"watermark":false}`, `{"created":1,"data":[{"url":"https://example.com/out.png","size":"1024x1024"}],"usage":{"generated_images":1}}`, false, true, .13},
			} {
				t.Run(tc.name, func(t *testing.T) {
					captured := make(chan map[string]any, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, "/api/v3/images/generations", r.URL.Path)
						assert.Equal(t, gin.MIMEJSON, r.Header.Get("Content-Type"))
						var body map[string]any
						assert.NoError(t, common.DecodeJson(r.Body, &body))
						captured <- body
						w.Header().Set("Content-Type", gin.MIMEJSON)
						_, _ = io.WriteString(w, tc.response)
					}))
					t.Cleanup(upstream.Close)
					body := []byte(`{"model":"doubao-seedream-5-5-261028",` + tc.fields + `}`)
					contentType := gin.MIMEJSON
					if tc.multipart {
						var buffer bytes.Buffer
						writer := multipart.NewWriter(&buffer)
						for key, value := range map[string]string{"model": relaycommon.Seedream55Model, "prompt": "edit", "size": "1K", "metadata": `{"watermark":false}`} {
							require.NoError(t, writer.WriteField(key, value))
						}
						file, err := writer.CreateFormFile("image", "input.png")
						require.NoError(t, err)
						_, err = file.Write([]byte("image"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						body, contentType = buffer.Bytes(), writer.FormDataContentType()
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", contentType)
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					mode := relayconstant.RelayModeImagesGenerations
					if tc.path == "/v1/images/edits" {
						mode = relayconstant.RelayModeImagesEdits
					}
					request, err := helper.GetAndValidOpenAIImageRequest(c, mode)
					require.NoError(t, err)
					common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeVolcEngine)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
					common.SetContextKey(c, constant.ContextKeyChannelKey, "channel-key")
					common.SetContextKey(c, constant.ContextKeyOriginalModel, relaycommon.Seedream55Model)
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passthrough})
					user := model.User{Username: fmt.Sprintf("seedream-%d", index), AffCode: fmt.Sprintf("seedream-%d", index), Quota: 10_000_000, Status: common.UserStatusEnabled}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("seedream-token-%d", index), Status: common.TokenStatusEnabled, RemainQuota: 10_000_000}
					require.NoError(t, db.Create(&token).Error)
					channel := model.Channel{Name: fmt.Sprintf("seedream-channel-%d", index), Type: constant.ChannelTypeVolcEngine}
					require.NoError(t, db.Create(&channel).Error)
					common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
					info := relaycommon.GenRelayInfoImage(c, request)
					info.UserId, info.TokenId, info.TokenKey, info.UserQuota = user.Id, token.Id, token.Key, 10_000_000
					info.StartTime = time.Now().Add(-5 * time.Second)
					info.UserSetting.BillingPreference = "wallet_only"
					info.ForcePreConsume = true
					info.UsingGroup, info.UserGroup, info.RelayMode = "default", "default", mode
					info.InitChannelMeta(c)
					require.Nil(t, PrepareRequestBilling(c, info))
					require.True(t, info.TieredBillingSnapshot.TaskUsageBilling)
					require.Nil(t, ImageHelper(c, info))
					assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					wire := <-captured
					assert.Equal(t, relaycommon.Seedream55Model, wire["model"])
					assert.NotContains(t, wire, "metadata")
					assert.NotContains(t, wire, "n")
					if tc.multipart {
						references := wire["image"].([]any)
						require.Len(t, references, 1)
						assert.True(t, strings.HasPrefix(references[0].(string), "data:image/png;base64,"))
					}
					var stored model.Log
					require.NoError(t, logDB.Where("user_id = ?", user.Id).Take(&stored).Error)
					wantQuota := common.QuotaRound(tc.cny / 6.8 * common.QuotaPerUnit)
					assert.Equal(t, wantQuota, stored.Quota)
					assert.Zero(t, stored.PromptTokens, "input images must not invent an input token")
					assert.GreaterOrEqual(t, stored.UseTime, 5)
					assert.Contains(t, stored.Other, `"usage_facts"`)
					assert.NotEmpty(t, stored.Response)
					require.NoError(t, db.First(&user, user.Id).Error)
					require.NoError(t, db.First(&token, token.Id).Error)
					assert.Equal(t, 10_000_000-wantQuota, user.Quota)
					assert.Equal(t, wantQuota, user.UsedQuota)
					assert.Equal(t, 10_000_000-wantQuota, token.RemainQuota)
					var rendered map[string]any
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &rendered))
					assert.Contains(t, rendered, "data")
					assert.Contains(t, rendered, "metadata")
				})
			}
			for index, tc := range []struct {
				name, usage                string
				record, discard, streaming bool
				prompt, completion, cached int
				known                      bool
			}{
				{"discarded task still records response and tokens", `"output_tokens":16224,"total_tokens":16224`, true, true, false, 0, 16224, 0, true},
				{"retained task records response and tokens", `"output_tokens":16224,"total_tokens":16224`, true, false, false, 0, 16224, 0, true},
				{"recording disabled keeps token metrics", `"output_tokens":16224,"total_tokens":16224`, false, true, false, 0, 16224, 0, true},
				{"SSE records assembled JSON and tokens before compaction", `"output_tokens":16224,"total_tokens":16224`, true, true, true, 0, 16224, 0, true},
				{"input output meters retain cache details", `"input_tokens":7,"output_tokens":11,"total_tokens":18,"input_tokens_details":{"cached_tokens":3}`, true, true, false, 4, 11, 3, true},
				{"prompt completion meters", `"prompt_tokens":12,"completion_tokens":8,"total_tokens":20`, true, true, false, 12, 8, 0, true},
				{"explicit zero tokens", `"input_tokens":0,"output_tokens":0,"total_tokens":0`, true, true, false, 0, 0, 0, true},
				{"missing token usage clears prior attempt", `"credits":5`, true, true, false, 0, 0, 0, false},
				{"negative tokens are not recorded", `"output_tokens":-1`, true, true, false, 0, 0, 0, false},
				{"fractional tokens are not recorded", `"output_tokens":1.5`, true, true, false, 0, 0, 0, false},
				{"oversized tokens are not recorded", `"output_tokens":2147483648`, true, true, false, 0, 0, 0, false},
				{"total alone does not invent input or output", `"total_tokens":99`, true, true, false, 0, 0, 0, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv("SAVE_REQUEST_RESPONSE", fmt.Sprint(tc.record))
					oldExport, oldAlerting := common.DataExportEnabled, common.AlertingEnabled
					common.DataExportEnabled, common.AlertingEnabled = true, false
					t.Cleanup(func() { common.DataExportEnabled, common.AlertingEnabled = oldExport, oldAlerting })
					user := model.User{Username: fmt.Sprintf("seedream-log-%d", index), AffCode: fmt.Sprintf("seedream-log-%d", index), Quota: 10_000_000}
					require.NoError(t, db.Create(&user).Error)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"doubao-seedream-5-5-261028","prompt":"cat"}`))
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					c.Set("task_request", map[string]any{"model": relaycommon.Seedream55Model, "prompt": "cat"})
					c.Set(common.RequestIdKey, fmt.Sprintf("seedream-log-%s-%d", dialect, index))
					info := &relaycommon.RelayInfo{
						UserId: user.Id, OriginModelName: relaycommon.Seedream55Model, StartTime: time.Now().Add(-7 * time.Second),
						ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: relaycommon.Seedream55Model, ChannelBaseUrl: "https://example.com"},
						TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "log-task", Action: "text_to_image"},
					}
					source, err := builtinplugins.Source("doubao")
					require.NoError(t, err)
					key := "doubao"
					if tc.streaming {
						key = "log-stream-fixture"
						source = `
export const meta = {apiVersion:1,key:"log-stream-fixture",name:"Log stream fixture",version:"1.0.0",author:{name:"Test"},models:["doubao-seedream-5-5-261028"],fetchMode:"per_task",submitResponseTypes:["json","sse"]};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/generate",responseType:"sse"};}
export function parseSubmitEvent(ctx,event,state){return event.data==="[DONE]"?{state:state,done:true}:{state:JSON.parse(event.data),done:false};}
export function parseSubmitResponse(ctx,resp){return {taskId:"upstream",taskData:{compacted:true},immediate:{status:"SUCCESS"}};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function parseTaskResult(){return {status:"SUCCESS"};}`
					}
					plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: key})
					require.NoError(t, err)
					adaptor := taskplugin.New(plugin)
					adaptor.Init(info)
					originalResponse := `{"created":1,"data":[{"url":"https://example.com/out.png","size":"1024x1024","text_recognition":{"text":"hello"}}],"usage":{"generated_images":1,` + tc.usage + `},"provider_extension":"preserved"}`
					body, contentType := originalResponse, gin.MIMEJSON
					if tc.name == "missing token usage clears prior attempt" {
						_, taskErr := adaptor.ParseResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {gin.MIMEJSON}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"url":"https://example.com/prior.png"}],"usage":{"input_tokens":9,"output_tokens":99}}`))}, info)
						require.Nil(t, taskErr)
					}
					if tc.streaming {
						_, err := adaptor.BuildRequestBody(c, info)
						require.NoError(t, err)
						body, contentType = "data: "+originalResponse+"\n\ndata: [DONE]\n\n", "text/event-stream"
					}
					parsed, taskErr := adaptor.ParseResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, info)
					require.Nil(t, taskErr)
					task := &model.Task{TaskID: "log-task", Data: parsed.TaskData, PrivateData: model.TaskPrivateData{ResultDiscarded: tc.discard}}
					if tc.discard {
						task.Data = nil
					}
					info.PriceData.Quota = 4321
					service.LogTaskConsumption(c, info, task)
					var stored model.Log
					require.NoError(t, logDB.Where("user_id = ?", user.Id).Take(&stored).Error)
					assert.GreaterOrEqual(t, stored.UseTime, 7)
					assert.Equal(t, tc.prompt, stored.PromptTokens)
					assert.Equal(t, tc.completion, stored.CompletionTokens)
					assert.Equal(t, 4321, stored.Quota, "token observation must not recalculate the image price")
					if tc.known {
						var usage dto.Usage
						require.NoError(t, common.UnmarshalJsonStr(stored.Usage, &usage))
						assert.Equal(t, tc.prompt+tc.cached, usage.PromptTokens)
						assert.Equal(t, tc.prompt+tc.cached+tc.completion, usage.TotalTokens)
						assert.Equal(t, tc.cached, usage.PromptTokensDetails.CachedTokens)
					} else {
						assert.Empty(t, stored.Usage)
					}
					require.NoError(t, model.SaveQuotaDataCache())
					var stats model.QuotaData
					require.NoError(t, db.Where("user_id = ?", user.Id).Take(&stats).Error)
					assert.Equal(t, tc.prompt, stats.PromptTokens)
					assert.Equal(t, tc.completion, stats.CompletionTokens)
					assert.Equal(t, tc.prompt+tc.completion, stats.TokenUsed)
					assert.Equal(t, tc.cached, stats.CachedTokens)
					assert.Equal(t, 1, stats.Count)
					assert.Equal(t, 4321, stats.Quota)
					assert.GreaterOrEqual(t, stats.RequestTimeSum, 7000)
					if tc.record {
						assert.JSONEq(t, originalResponse, stored.Response)
						assert.NotEmpty(t, stored.Request)
					} else {
						assert.Empty(t, stored.Response)
						assert.Empty(t, stored.Request)
					}
				})
			}
		})
	}
}

func TestSeedream55NativeRequestValidation(t *testing.T) {
	for _, fields := range []string{
		`"n":2`, `"n":18446744073686646784`, `"stream":true`, `"mask":"https://example.com/mask.png"`,
		`"metadata":[]`, `"size":"4K"`, `"size":"2200x2200"`,
		`"background":"transparent","output_format":"jpeg"`,
		`"metadata":{"sequential_image_generation":"auto"}`,
		`"metadata":{"optimize_prompt_options":{"mode":"fast"}}`,
		`"metadata":{"layer_decomposition":true,"layer_decomposition_options":{"max_images":101}}`,
		`"metadata":{"layer_decomposition":true,"layer_decomposition_options":{"max_images":-1}}`,
		`"metadata":{"layer_decomposition":true,"layer_decomposition_options":{"max_images":2.5}}`,
		`"metadata":{"layer_decomposition":true,"layer_decomposition_options":{"max_images":18446744073686646784}}`,
	} {
		t.Run(fields, func(t *testing.T) {
			var request dto.ImageRequest
			err := common.UnmarshalJsonStr(`{"model":"doubao-seedream-5-5-261028","prompt":"cat","image":"https://example.com/input.png",`+fields+`}`, &request)
			if err == nil {
				_, _, err = relaycommon.PrepareSeedream55Request(nil, request)
			}
			require.Error(t, err)
		})
	}
}

func TestSeedream55NativeMeasuredUsage(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		count      int
		facts      map[string]any
	}{
		{"object", `{"data":{"url":"a","size":"2610x1000"}}`, 1, map[string]any{"images_up_to_1_5k": float64(1), "images_above_1_5k": float64(0)}},
		{"split payload", `{"data":[{"url":"a","size":"1024x1024"},{"b64_json":"aW1hZ2U=","size":"1024x1024"},{"revised_prompt":"cat"}]}`, 1, map[string]any{"images_up_to_1_5k": float64(1), "images_above_1_5k": float64(0)}},
		{"missing size keeps estimated tiers", `{"data":[{"url":"a"}],"usage":{"input_images":0}}`, 1, map[string]any{"input_images": float64(0)}},
		{"invalid count keeps reservation", `{"data":[{"url":"a","size":"1024x1024"}],"usage":{"generated_images":101}}`, 0, nil},
		{"invalid inputs keep estimated references", `{"data":[{"url":"a","size":"2611x1000"}],"usage":{"input_images":11}}`, 1, map[string]any{"images_up_to_1_5k": float64(0), "images_above_1_5k": float64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &body))
			facts, count := relaycommon.Seedream55ResponseUsage(body)
			assert.Equal(t, tc.count, count)
			assert.Equal(t, tc.facts, facts)
		})
	}
}
