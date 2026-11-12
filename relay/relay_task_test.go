package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGeminiOmniOfficialUsageExpression(t *testing.T) {
	const modelName = "gemini-omni-1.2-flash"
	expression, ok := billing_setting.GetBuiltinBillingExpr(modelName)
	require.True(t, ok)
	require.NoError(t, model.ValidateModelPricing(modelName, model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": expression,
	}))
	for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi, constant.ChannelTypeOpenAI, constant.ChannelTypeSora} {
		for _, tc := range []struct {
			resolution string
			tokens     float64
			cost       float64
		}{
			{"360p", 28965, 0.35094},
			{"720p", 86880, 1.04592},
			{"1080p", 130320, 1.5672},
			{"4k", 260640, 3.13104},
		} {
			t.Run(fmt.Sprintf("%d/%s", channelType, tc.resolution), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Set("task_request", relaycommon.TaskSubmitReq{Model: modelName, Duration: 15, Resolution: tc.resolution, Images: []string{"one", "two"}})
				info := &relaycommon.RelayInfo{OriginModelName: "video-alias", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelName}}
				platform := constant.TaskPlatform(strconv.Itoa(channelType))
				_, adaptor := getTaskAdaptorForRequest(c, platform, modelName)
				provider, supported := adaptor.(channel.TaskValidatedUsageFactsProvider)
				require.True(t, supported)
				facts, err := provider.ExtractUsageFactsValidated(c, info)
				require.NoError(t, err)
				assert.Equal(t, float64(2240), facts["input_tokens"])
				assert.Equal(t, tc.tokens, facts["video_output_tokens"])
				cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
				require.NoError(t, err)
				assert.InDelta(t, tc.cost, cost, 1e-12)
			})
		}
	}

	for _, tc := range []struct {
		name, model, seconds string
		metadata             map[string]any
	}{
		{name: "duration exceeds new model limit", model: modelName, seconds: "16"},
		{name: "old model retains ten second limit", model: "gemini-omni-1.1-flash-preview", seconds: "15"},
		{name: "metadata duration cannot bypass limit", model: modelName, metadata: map[string]any{"response_format": map[string]any{"duration": "999999s"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{Model: tc.model, Seconds: tc.seconds, Metadata: tc.metadata})
			_, adaptor := getTaskAdaptorForRequest(c, constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)), tc.model)
			provider := adaptor.(channel.TaskValidatedUsageFactsProvider)
			_, err := provider.ExtractUsageFactsValidated(c, &relaycommon.RelayInfo{OriginModelName: tc.model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}})
			require.Error(t, err)
		})
	}
}

func TestGeminiOmniOfficialUsageSettlement(t *testing.T) {
	const modelName = "gemini-omni-1.2-flash"
	expression, ok := billing_setting.GetBuiltinBillingExpr(modelName)
	require.True(t, ok)
	for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi, constant.ChannelTypeOpenAI, constant.ChannelTypeSora} {
		for _, tc := range []struct {
			name, usage string
			cost        float64
			pending     bool
		}{
			{"mixed output and separate thinking", `{"total_input_tokens":1220,"total_output_tokens":5800,"total_thought_tokens":20,"output_tokens_by_modality":[{"modality":"text","tokens":8},{"modality":"video","tokens":5792}]}`, 0.071586, false},
			{"modality totals without aggregates", `{"input_tokens_by_modality":[{"modality":"text","tokens":100},{"modality":"video","tokens":200}],"output_tokens_by_modality":[{"modality":"text","tokens":8},{"modality":"video","tokens":5792}]}`, 0.070026, false},
			{"text remainder from explicit video", `{"total_input_tokens":0,"total_output_tokens":5800,"output_tokens_by_modality":[{"modality":"video","tokens":5792}]}`, 0.069576, false},
			{"video remainder from explicit text", `{"total_input_tokens":0,"total_output_tokens":5800,"output_tokens_by_modality":[{"modality":"text","tokens":8}]}`, 0.069576, false},
			{"explicit zero refunds reservation", `{"total_input_tokens":0,"total_output_tokens":0,"total_thought_tokens":0}`, 0, false},
			{"zero unused modality", `{"total_input_tokens":0,"total_output_tokens":5792,"output_tokens_by_modality":[{"modality":"video","tokens":5792},{"modality":"audio","tokens":0}]}`, 0.069504, false},
			{"missing usage", `null`, 0, true},
			{"missing modality count is not zero", `{"input_tokens_by_modality":[{"modality":"text"}],"total_output_tokens":0}`, 0, true},
			{"missing input usage", `{"total_output_tokens":5792,"output_tokens_by_modality":[{"modality":"video","tokens":5792}]}`, 0, true},
			{"unspecified output modality", `{"total_input_tokens":10,"total_output_tokens":5792}`, 0, true},
			{"inconsistent output totals", `{"total_input_tokens":10,"total_output_tokens":6000,"output_tokens_by_modality":[{"modality":"text","tokens":8},{"modality":"video","tokens":5792}]}`, 0, true},
			{"negative count", `{"total_input_tokens":-1,"total_output_tokens":0}`, 0, true},
			{"unknown nonzero output modality", `{"total_input_tokens":0,"output_tokens_by_modality":[{"modality":"audio","tokens":100}]}`, 0, true},
			{"fractional count", `{"total_input_tokens":1.5,"total_output_tokens":0}`, 0, true},
			{"oversized count", `{"total_input_tokens":2147483648,"total_output_tokens":0}`, 0, true},
			{"overflowing JSON count", `{"total_input_tokens":18446744073709551616,"total_output_tokens":0}`, 0, true},
		} {
			t.Run(fmt.Sprintf("%d/%s", channelType, tc.name), func(t *testing.T) {
				platform := constant.TaskPlatform(strconv.Itoa(channelType))
				_, submitter := getTaskAdaptorForRequest(nil, platform, modelName)
				// Obtain a real tagged upstream ID using the native submit parser.
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"interaction-1","status":"in_progress"}`))}
				info := &relaycommon.RelayInfo{OriginModelName: modelName, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_omni_official"}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelName, ApiKey: `{"project_id":"test-project"}`, ApiVersion: "global"}}
				submitter.Init(info)
				parsed, taskErr := submitter.ParseResponse(c, response, info)
				require.Nil(t, taskErr)
				snapshot := &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), TaskUsageBilling: true, QuotaPerUnit: 500000, GroupRatio: 1.25,
					UsageFacts: map[string]any{"input_tokens": float64(1000), "text_output_tokens": float64(0), "thought_tokens": float64(0), "video_output_tokens": float64(57920)}}
				task := &model.Task{TaskID: "task_omni_official", Platform: platform, Status: model.TaskStatusSuccess, Quota: 500000,
					Properties: model.Properties{OriginModelName: modelName, UpstreamModelName: modelName}, PrivateData: model.TaskPrivateData{UpstreamTaskID: parsed.UpstreamTaskID, BillingContext: &model.TaskBillingContext{TieredSnapshot: snapshot}}}
				poller := GetTaskAdaptor(platform)
				result, err := poller.ParseTaskResult(task, nil, []byte(`{"id":"interaction-1","object":"interaction","status":"completed","usage":`+tc.usage+`}`))
				require.NoError(t, err)
				assert.Equal(t, model.TaskStatusSuccess, result.Status)
				assert.Equal(t, tc.pending, result.BillingPending)
				if tc.pending {
					assert.Empty(t, result.UsageFacts)
					assert.True(t, service.SettleTaskBillingOnComplete(context.Background(), poller, task, result))
					assert.Equal(t, 500000, task.Quota)
					return
				}
				settled, actual, err := service.EvaluateTaskCompletionUsage(snapshot, result.UsageFacts)
				require.NoError(t, err)
				assert.InDelta(t, tc.cost*500000, settled.ActualQuotaBeforeGroup, 1e-8)
				assert.Equal(t, common.QuotaRound(tc.cost*500000*1.25), settled.ActualQuotaAfterGroup)
				assert.Equal(t, result.UsageFacts, actual)
				assert.Equal(t, float64(57920), snapshot.UsageFacts["video_output_tokens"], "evaluating actual usage must not mutate the reservation")
			})
		}
	}
}

func TestGeminiOmniCompatibleSubmitAndUsageRoundTrip(t *testing.T) {
	const modelName = "gemini-omni-1.2-flash"
	const usage = `{"total_input_tokens":1220,"total_output_tokens":46344,"total_thought_tokens":20,"output_tokens_by_modality":[{"modality":"text","tokens":8},{"modality":"video","tokens":46336}]}`
	saveBillingConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{}`, "billing_setting.billing_expr": `{}`}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos", r.URL.Path)
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		assert.Equal(t, modelName, payload["model"])
		assert.Equal(t, "8", payload["seconds"])
		assert.Equal(t, "720p", payload["resolution"])
		assert.Equal(t, "16:9", payload["aspect_ratio"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"upstream-video","status":"queued","model":"gemini-omni-1.2-flash"}`)
	}))
	defer server.Close()
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeSora} {
		t.Run(strconv.Itoa(channelType), func(t *testing.T) {
			c, info := newTaskSubmitContext(t, modelName, "")
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"prompt":"A man and a woman danced in the rain, it was incredibly romantic.","model":"gemini-omni-1.2-flash","seconds":"8","resolution":"720p","aspect_ratio":"16:9"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			c.Set("group", "default")
			info.UserGroup, info.UsingGroup, info.OriginModelName = "default", "default", modelName
			info.Billing = &imageReservation{limit: 1 << 30}
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			result, taskErr := RelayTaskSubmit(c, info)
			require.Nil(t, taskErr, "submission error: %+v", taskErr)
			require.NotNil(t, result)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, float64(46336), info.TieredBillingSnapshot.UsageFacts["video_output_tokens"])
			assert.Empty(t, info.PriceData.OtherRatios())
			task := model.InitTask(result.Platform, info)
			task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
			task.Status = model.TaskStatusSuccess
			task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: info.TieredBillingSnapshot}
			poller := GetTaskAdaptor(result.Platform)
			actual, err := poller.ParseTaskResult(task, nil, []byte(`{"id":"upstream-video","status":"completed","usage":`+usage+`}`))
			require.NoError(t, err)
			assert.False(t, actual.BillingPending)
			settled, _, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, actual.UsageFacts)
			require.NoError(t, err)
			assert.InDelta(t, 0.558114*common.QuotaPerUnit, settled.ActualQuotaBeforeGroup, 1e-8)
		})
	}
	// Native Gemini and Vertex expose the same official usage in their public
	// video response, which an OpenAI-compatible downstream can bill again
	// using its own prices without relying on private upstream logs.
	for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi} {
		t.Run("native response/"+strconv.Itoa(channelType), func(t *testing.T) {
			platform := constant.TaskPlatform(strconv.Itoa(channelType))
			task := &model.Task{TaskID: "task-public", Platform: platform, Status: model.TaskStatusSuccess,
				Properties:  model.Properties{OriginModelName: modelName, UpstreamModelName: modelName},
				PrivateData: model.TaskPrivateData{UpstreamTaskID: "b21uaTppbnRlcmFjdGlvbi0x"},
				Data:        []byte(`{"id":"interaction-1","status":"completed","usage":` + usage + `}`)}
			body, err := GetTaskAdaptor(platform).(channel.OpenAIVideoConverter).ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			var video struct {
				ID    string
				Usage map[string]any
			}
			require.NoError(t, common.Unmarshal(body, &video))
			assert.Equal(t, "task-public", video.ID)
			require.NotEmpty(t, video.Usage)
			_, downstream := getTaskAdaptorForRequest(nil, constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOpenAI)), modelName)
			parsed, err := downstream.ParseTaskResult(task, nil, body)
			require.NoError(t, err)
			assert.Equal(t, float64(46336), parsed.UsageFacts["video_output_tokens"])
			assert.Equal(t, float64(20), parsed.UsageFacts["thought_tokens"])
		})
	}
}

func TestGeminiOmniNativePricingConfiguration(t *testing.T) {
	const modelName = "gemini-omni-1.2-flash"
	saveBillingConfig(t)
	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		model.InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{}`, "billing_setting.billing_expr": `{}`}))
	db := setupRelayChannelDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Ability{}, &model.Model{}, &model.Vendor{}))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Type: constant.ChannelTypeGemini, Key: "test", Name: "omni", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Ability{ChannelId: 1, Model: modelName, Group: "default", Enabled: true}).Error)
	model.InvalidatePricingCache()
	prices := model.GetPricing()
	require.Len(t, prices, 1)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, prices[0].BillingMode)
	assert.Equal(t, "token", prices[0].BillingUsageSchema["video_output_tokens"].Unit)
	require.Error(t, model.ValidateModelPricing(modelName, model.PricingValues{"billing_setting.billing_expr": `tier("bad", u("unknown") * 12 / 1000000)`}))
	require.Error(t, model.ValidateModelPricing(modelName, model.PricingValues{"billing_setting.billing_expr": `tier("bad", fixed(0.1))`}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gemini-omni-1.2-flash":0.2}`))
	assert.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode(modelName), "existing administrator prices must remain explicit overrides")
}

func TestGeminiOmniNativeSubmitFreezesOfficialExpression(t *testing.T) {
	const modelName = "gemini-omni-1.2-flash"
	saveBillingConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{}`, "billing_setting.billing_expr": `{}`}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/interactions", r.URL.Path)
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		assert.Equal(t, modelName, payload["model"])
		assert.Equal(t, map[string]any{"type": "video", "duration": "15s", "resolution": "720p"}, payload["response_format"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"omni-official-submit","status":"in_progress"}`)
	}))
	defer server.Close()
	c, info := newTaskSubmitContext(t, "video-alias", `{"video-alias":"gemini-omni-1.2-flash"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"video-alias","prompt":"a cat","duration":15,"resolution":"720p"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	c.Set("group", "default")
	info.UserGroup, info.UsingGroup, info.OriginModelName = "default", "default", "video-alias"
	info.Billing = &imageReservation{limit: 1 << 30}
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	result, taskErr := RelayTaskSubmit(c, info)
	require.Nil(t, taskErr, "submission error: %+v", taskErr)
	require.NotNil(t, result)
	require.NotNil(t, info.TieredBillingSnapshot)
	assert.Empty(t, info.PriceData.OtherRatios(), "expression prices must not also receive legacy duration or resolution multipliers")
	assert.Equal(t, "video-alias", info.TieredBillingSnapshot.ModelName)
	assert.Equal(t, float64(86880), info.TieredBillingSnapshot.UsageFacts["video_output_tokens"])
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"gemini-omni-1.2-flash":"tier(\"changed\", u(\"video_output_tokens\") * 99 / 1000000)"}`}))
	settled, _, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, map[string]any{
		"input_tokens": float64(0), "text_output_tokens": float64(0), "thought_tokens": float64(0), "video_output_tokens": float64(57920),
	})
	require.NoError(t, err)
	assert.Equal(t, "standard", settled.MatchedTier)
	assert.InDelta(t, 0.69504*common.QuotaPerUnit, settled.ActualQuotaBeforeGroup, 1e-8)
}

func setupRelayChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestApplyChannelPinPreservesOriginTasksAndRetryMode(t *testing.T) {
	database := setupRelayChannelDB(t)
	channel := &model.Channel{Name: "origin-channel", Key: "sk-test", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeDoubaoVideo}
	require.NoError(t, database.Create(channel).Error)
	originTask := &model.Task{
		TaskID: "task-lock", ChannelId: channel.Id, Action: "text_to_video", Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-task-lock"},
		Data:        []byte(`{"id":"upstream-task-lock"}`),
	}

	for _, tc := range []struct {
		name     string
		tokenPin bool
		apply    func(*gin.Context, *relaycommon.RelayInfo) *dto.TaskError
	}{
		{name: "origin affinity", apply: ApplyOriginTaskAffinity},
		{name: "same channel retry", apply: ApplyChannelPin},
		{name: "token pin suppresses channel lock", tokenPin: true, apply: ApplyChannelPin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyOriginTasks, []*model.Task{originTask})
			constraints := service.GetChannelConstraints(c)
			constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceOriginTask, Rank: dto.PinRankOriginTask, RetryMode: dto.PinRetrySameChannel})
			if tc.tokenPin {
				constraints.AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt})
			}
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			require.Nil(t, tc.apply(c, info))
			if tc.tokenPin {
				assert.Nil(t, info.LockedChannel)
			} else {
				locked, ok := info.LockedChannel.(*model.Channel)
				require.True(t, ok)
				require.NotNil(t, locked)
				assert.Equal(t, channel.Id, locked.Id)
			}
			require.Len(t, info.OriginTasks, 1)
			assert.Equal(t, "task-lock", info.OriginTasks[0].TaskID)
			assert.Equal(t, "upstream-task-lock", info.OriginTasks[0].UpstreamTaskID)
			assert.Equal(t, "text_to_video", info.OriginTasks[0].Action)
			assert.Equal(t, string(model.TaskStatusSuccess), info.OriginTasks[0].Status)
			assert.Equal(t, []byte(originTask.Data), info.OriginTasks[0].Data)
		})
	}
}

func TestTaskModel2DtoNormalizesLegacyAction(t *testing.T) {
	task := &model.Task{Action: "firstTailGenerate"}

	dtoTask := TaskModel2Dto(task)

	assert.Equal(t, constant.TaskActionFirstTailToVideo, dtoTask.Action)
	assert.Equal(t, "firstTailGenerate", task.Action)
}

const mappingOrderSubmitPlugin = `
export const meta = {apiVersion:1,key:"maporder",name:"Map Order",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

const mappingOrderRewritePlugin = `
export const meta = {apiVersion:1,key:"maporder-rw",name:"Map Order RW",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel}, rewriteModel:"rewritten"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func pinMappingOrderPlugin(t *testing.T, c *gin.Context, source string) {
	t.Helper()
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
}

func newTaskSubmitContext(t *testing.T, originalModel, mapping string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, originalModel)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
	if mapping != "" {
		c.Set("model_mapping", mapping)
	}
	c.Set("task_request", map[string]any{"prompt": "p"})
	return c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet(t *testing.T) {
	const mapping = `{"alias-model":"mid-model","mid-model":"declared-model"}`

	c, info := newTaskSubmitContext(t, "alias-model", mapping)
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "alias-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDeclaredNameWithoutMappingIsUnchanged(t *testing.T) {
	c, info := newTaskSubmitContext(t, "declared-model", "")
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "declared-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "declared-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.False(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDoesNotApplyMappingTwice(t *testing.T) {
	c, info := newTaskSubmitContext(t, "alias-model", `{"alias-model":"declared-model"}`)
	pinMappingOrderPlugin(t, c, mappingOrderRewritePlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "rewritten", info.UpstreamModelName, "late mapping would overwrite rewriteModel with the chain tail")
	assert.Equal(t, "alias-model", info.OriginModelName)
}

func TestRelayTaskSubmitEmptyOriginKeepsLateMapping(t *testing.T) {
	plugin, err := pluginruntime.NewRegistry().Register(mappingOrderSubmitPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	synthesized := service.CoverTaskActionToModelName(constant.TaskPlatform(plugin.Meta.Key), "text_to_video")
	c, info := newTaskSubmitContext(t, "pre-validate-upstream",
		`{"pre-validate-upstream":"should-not-apply-early","`+synthesized+`":"legacy-tail"}`)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	info.OriginModelName = ""

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, synthesized, info.OriginModelName)
	assert.Equal(t, "legacy-tail", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

const billingFallbackPlugin = `
export const meta = {apiVersion:1,key:"bill-fallback",name:"Bill Fallback",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func saveBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func TestRelayTaskSubmitAliasBillingIdentityAndExprFallback(t *testing.T) {
	const mapping = `{"alias-model":"declared-model"}`
	const aliasExpr = `tier("alias", 2)`
	const tailExpr = `tier("tail", 3)`
	const legacyExpr = `tier("legacy", u("old_units") * 2)`

	tests := []struct {
		name       string
		modes      map[string]string
		exprs      map[string]string
		wantTiered bool
		wantExpr   string
		source     string
	}{
		{
			name:       "alias own tiered wins",
			modes:      map[string]string{"alias-model": "tiered_expr", "declared-model": "tiered_expr"},
			exprs:      map[string]string{"alias-model": aliasExpr, "declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   aliasExpr,
		},
		{
			name:       "fallback uses tail expr",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   tailExpr,
		},
		{
			name:       "stored expression keeps running after schema narrows",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": legacyExpr},
			wantTiered: true,
			wantExpr:   legacyExpr,
			source: strings.Replace(billingFallbackPlugin, `fetchMode:"per_task"`, `fetchMode:"per_task", usageSchema:{old_units:{type:"number",unit:"count"}}, usageProfiles:[{models:["declared-model"],schema:{seconds:{type:"number",unit:"second"}}}]`, 1) + `
export function extractUsage(){return {old_units:2};}
`,
		},
		{
			name:       "neither tiered uses ordinary pricing",
			wantTiered: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			saveBillingConfig(t)
			if len(testCase.modes) > 0 {
				modeJSON, marshalErr := common.Marshal(testCase.modes)
				require.NoError(t, marshalErr)
				exprJSON, marshalErr := common.Marshal(testCase.exprs)
				require.NoError(t, marshalErr)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"billing_setting.billing_mode": string(modeJSON),
					"billing_setting.billing_expr": string(exprJSON),
				}))
				if testCase.wantExpr == aliasExpr {
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("alias-model"))
				} else {
					require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("alias-model"))
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("declared-model"))
				}
			}

			c, info := newTaskSubmitContext(t, "alias-model", mapping)
			c.Set("group", "default")
			info.UserGroup = "default"
			info.UsingGroup = "default"
			source := testCase.source
			if source == "" {
				source = billingFallbackPlugin
			}
			pinMappingOrderPlugin(t, c, source)
			info.OriginModelName = "alias-model"

			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, "alias-model", info.OriginModelName)
			assert.Equal(t, "declared-model", info.UpstreamModelName)
			assert.True(t, info.IsModelMapped)

			task := model.InitTask(constant.TaskPlatform("bill-fallback"), info)
			assert.Equal(t, "alias-model", task.Properties.OriginModelName)
			assert.Equal(t, "declared-model", task.Properties.UpstreamModelName)

			if testCase.wantTiered {
				require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
				assert.Equal(t, "alias-model", info.TieredBillingSnapshot.ModelName)
				assert.Equal(t, testCase.wantExpr, info.TieredBillingSnapshot.ExprString)
				assert.Equal(t, billingexpr.ExprHashString(testCase.wantExpr), info.TieredBillingSnapshot.ExprHash)
				assert.NotEqual(t, "model_price_error", taskErr.Code)
				if testCase.wantExpr == legacyExpr {
					assert.Equal(t, 4*common.QuotaPerUnit, info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup)
				}
			} else {
				assert.Nil(t, info.TieredBillingSnapshot)
				assert.Equal(t, "model_price_error", taskErr.Code)
			}
		})
	}
}

func TestSharedTaskBillingExpressionSelectionAndFrozenSettlement(t *testing.T) {
	const baseExpr = `tier("base", u("seconds") * 2)`
	const alphaExpr = `tier("alpha", u("seconds") * 3)`
	const betaExpr = `tier("beta", u("credits") * 5)`
	const aliasExpr = `tier("alias", u("credits") * 7)`
	for _, tc := range []struct {
		name, plugin, model, mapping, modelExpr, mode, wantExpr string
		variants                                                map[string]string
		wantPriceError, profiled                                bool
	}{
		{name: "executing plugin override", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-alpha::declared-model": alphaExpr, "billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "override ignores model mode", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "ratio", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "model expression fallback", plugin: "billing-alpha", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantExpr: baseExpr},
		{name: "alias override precedes mapped override", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr, "billing-beta::alias-model": aliasExpr}, wantExpr: aliasExpr},
		{name: "mapped override precedes model fallback", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "unconfigured plugin cannot use another schema", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantPriceError: true},
		{name: "missing usage in skipped branch remains incompatible", plugin: "billing-beta", model: "declared-model", modelExpr: `true ? tier("free", 0) : tier("missing", u("seconds"))`, mode: "tiered_expr", wantPriceError: true},
		{name: "fixed pricing is still rejected", plugin: "billing-beta", model: "declared-model", variants: map[string]string{"billing-beta::declared-model": `tier("fixed", fixed(1))`}, wantPriceError: true},
		{name: "endpoint mapping keeps the declared profile", plugin: "billing-beta", model: "declared-model", mapping: `{"declared-model":"ep-endpoint"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr, profiled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			registry := pluginruntime.NewRegistry()
			for _, spec := range []struct{ key, field, unit string }{{"billing-alpha", "seconds", "second"}, {"billing-beta", "credits", "credit"}} {
				source := strings.ReplaceAll(billingFallbackPlugin, "bill-fallback", spec.key)
				schema := `usageSchema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}`
				if tc.profiled && spec.key == "billing-beta" {
					// The endpoint ID is undeclared, so only the declared model's profile carries this schema.
					schema = `usageSchema:{seconds:{type:"number",unit:"second"}},usageProfiles:[{models:["declared-model"],schema:{` + spec.field + `:{type:"number",unit:"` + spec.unit + `"}}}]`
				}
				source = strings.Replace(source, `fetchMode:"per_task"`, `fetchMode:"per_task",`+schema, 1)
				source += `export function extractUsage(){return {` + spec.field + `:2};}`
				_, err := registry.Register(source, pluginruntime.Options{})
				require.NoError(t, err)
			}
			variants := tc.variants
			if variants == nil {
				variants = map[string]string{}
			}
			rawVariants, err := common.Marshal(variants)
			require.NoError(t, err)
			modes, err := common.Marshal(map[string]string{"declared-model": tc.mode})
			require.NoError(t, err)
			expressions, err := common.Marshal(map[string]string{"declared-model": tc.modelExpr})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: string(rawVariants), "billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(expressions),
			}))
			c, info := newTaskSubmitContext(t, tc.model, tc.mapping)
			c.Set("group", "default")
			c.Set("task_plugin_key", tc.plugin)
			info.UserGroup = "default"
			info.UsingGroup = "default"
			info.OriginModelName = tc.model
			plugin, ok := registry.Generation().Get(tc.plugin)
			require.True(t, ok)
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr) // This fixture stops at reservation, before upstream submission.
			if tc.wantPriceError {
				assert.Equal(t, "model_price_error", taskErr.Code)
				assert.Nil(t, info.TieredBillingSnapshot)
				return
			}
			require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
			assert.NotEqual(t, "model_price_error", taskErr.Code)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`, "billing_setting.billing_expr": `{}`}))
			field := "seconds"
			if tc.plugin == "billing-beta" {
				field = "credits"
			}
			result, usage, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, map[string]any{field: float64(4)})
			require.NoError(t, err)
			assert.Equal(t, float64(4), usage[field])
			assert.Equal(t, float64(2), info.TieredBillingSnapshot.UsageFacts[field])
			assert.Equal(t, 2*info.TieredBillingSnapshot.EstimatedQuotaAfterGroup, result.ActualQuotaAfterGroup)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
		})
	}
}

// Issue #7478: task APIs answer 201 Created or 202 Accepted on submission.
// Every 2xx must reach parseSubmitResponse; only other statuses are upstream
// failures that keep the upstream status and body.
func TestRelayTaskSubmitAcceptsAnySuccessfulUpstreamStatus(t *testing.T) {
	service.InitHttpClient()
	const source = `
export const meta = {apiVersion:1,key:"status-echo",name:"Status Echo",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl+"/submit", method:"POST", body:{model: ctx.model}, action:"text_to_video"}; }
export function parseSubmitResponse(ctx, response) { return {taskId: response.body.id, taskData: {status: response.statusCode}}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
`
	for _, tc := range []struct {
		status   int
		wantCode string
	}{
		{status: http.StatusOK},
		{status: http.StatusCreated},
		{status: http.StatusAccepted},
		{status: http.StatusBadRequest, wantCode: "fail_to_fetch_task"},
		{status: http.StatusBadGateway, wantCode: "fail_to_fetch_task"},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			saveBillingConfig(t)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": `{"declared-model":"tiered_expr"}`,
				"billing_setting.billing_expr": `{"declared-model":"tier(\"flat\", 3)"}`,
			}))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"id":"job-42","message":"upstream body"}`))
			}))
			defer server.Close()

			c, info := newTaskSubmitContext(t, "declared-model", "")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			c.Set("group", "default")
			info.UserGroup, info.UsingGroup = "default", "default"
			info.OriginModelName = "declared-model"
			// An existing reservation skips pre-consume so the fixture reaches the upstream call.
			info.Billing = &imageReservation{limit: 1 << 30}
			pinMappingOrderPlugin(t, c, source)

			result, taskErr := RelayTaskSubmit(c, info)
			if tc.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tc.wantCode, taskErr.Code)
				assert.Equal(t, tc.status, taskErr.StatusCode)
				assert.Contains(t, taskErr.Message, "upstream body")
				assert.Nil(t, result)
				return
			}
			require.Nil(t, taskErr, "submission error: %+v", taskErr)
			require.NotNil(t, result)
			assert.Equal(t, "job-42", result.UpstreamTaskID)
			assert.JSONEq(t, `{"status":`+strconv.Itoa(tc.status)+`}`, string(result.TaskData))
		})
	}
}
