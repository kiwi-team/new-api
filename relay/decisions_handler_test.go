package relay

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	"github.com/QuantumNous/new-api/relay/channel/typesafe"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const decisionsRequestJSON = `{"model":"gpt-6-luna","input":"A broken screen","questions":[{"type":"predicate","instructions":"Is it damaged?"}]}`

func TestDecisionsOpenRouterConversion(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", nil)
	defer service.CleanupFileSources(ctx)
	var request dto.DecisionsRequest
	require.NoError(t, common.UnmarshalJsonStr(`{
		"model":"openai/gpt-6-luna-decisions","safety_identifier":"end-user",
		"input":[{"role":"user","content":[{"type":"input_text","text":"Check this"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8=","detail":"low"}]}],
		"questions":[
			{"type":"predicate","instructions":"Damaged?"},
			{"type":"choice","name":"same","instructions":"Choose","choices":[{"value":false},{"value":"false","description":"The string"},{"value":true}]},
			{"type":"score","name":"same","instructions":"Severity","levels":[{"label":"low"},{"label":"high","description":"Broken"}]},
			{"type":"predicate","name":"","instructions":"Refuse this"}
		]}`, &request))
	require.NoError(t, helper.ValidateDecisionsRequest(&request))
	converted, err := openrouter.ConvertDecisionsRequest(ctx, &request)
	require.NoError(t, err)
	assert.Equal(t, "end-user", *converted.User)
	assert.JSONEq(t, `["Check this",{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8=","detail":"low"}}]`, string(converted.State))
	assert.JSONEq(t, `{"type":"noul","instructions":"Damaged?"}`, string(converted.Questions["0"]))
	assert.JSONEq(t, `{"type":"choice","instructions":"Choose","criteria":{"0":{"value":false},"1":{"value":"false","description":"The string"},"2":{"value":true}}}`, string(converted.Questions["1"]))
	assert.JSONEq(t, `{"type":"score","instructions":"Severity","criteria":[{"label":"low"},{"label":"high","description":"Broken"}]}`, string(converted.Questions["2"]))

	upstream := `{"model":"openai/gpt-6-luna-decisions","answers":{
		"3":{"type":"refusal"},
		"2":{"type":"score","score":0,"confidence":0,"probabilities":{"1":0,"0":1}},
		"1":{"type":"choice","choice":"0","confidence":1,"probabilities":{"2":0,"1":0,"0":1}},
		"0":{"type":"noul","noul":0}},"usage":{"input_tokens":42,"output_tokens":0,"cost":0.0000042}}`
	response, err := openrouter.ConvertDecisionsResponse([]byte(upstream), &request, "gpt-6-luna")
	require.NoError(t, err)
	_, err = decisionsBillingUsage(response.Usage)
	require.NoError(t, err)
	encoded, err := common.Marshal(response)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"gpt-6-luna","answers":[
		{"type":"predicate","name":null,"probability":0},
		{"type":"choice","name":"same","choice":false,"confidence":1,"probabilities":[{"value":false,"probability":1},{"value":"false","probability":0},{"value":true,"probability":0}]},
		{"type":"score","name":"same","score":0,"confidence":0,"probabilities":[{"value":0,"label":"low","probability":1},{"value":1,"label":"high","probability":0}]},
		{"type":"refusal","name":""}],
		"usage":{"input_tokens":42,"output_tokens":0,"total_tokens":42,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, string(encoded))
	meta := request.GetTokenCountMeta()
	assert.Contains(t, meta.CombineText, "Check this")
	assert.Contains(t, meta.CombineText, "Damaged?")
	assert.NotContains(t, meta.CombineText, "aGVsbG8=")
	require.Len(t, meta.Files, 1)
	assert.Equal(t, "low", meta.Files[0].Detail)
	for index, expected := range []any{false, "false", true} {
		body := strings.Replace(upstream, `"choice":"0"`, fmt.Sprintf(`"choice":"%d"`, index), 1)
		response, err := openrouter.ConvertDecisionsResponse([]byte(body), &request, "gpt-6-luna")
		require.NoError(t, err)
		assert.Equal(t, expected, response.Answers[1].Choice)
	}

	for _, replacement := range []struct{ old, value string }{
		{`"noul":0`, `"noul":-0.1`},
		{`"choice":"0"`, `"choice":"false"`},
		{`"confidence":1`, `"confidence":null`},
		{`"2":0,"1":0,"0":1`, `"2":0,"0":1`},
		{`"type":"noul"`, `"type":"choice"`},
		{`"3":{"type":"refusal"}`, `"4":{"type":"refusal"}`},
	} {
		_, err := openrouter.ConvertDecisionsResponse([]byte(strings.Replace(upstream, replacement.old, replacement.value, 1)), &request, "gpt-6-luna")
		assert.Error(t, err, replacement.value)
	}
}

func TestDecisionsValidationAndUnsupportedOpenRouterFields(t *testing.T) {
	var base dto.DecisionsRequest
	require.NoError(t, common.UnmarshalJsonStr(decisionsRequestJSON, &base))
	for _, tc := range []struct {
		name string
		edit func(*dto.DecisionsRequest)
	}{
		{"missing model", func(r *dto.DecisionsRequest) { r.Model = "" }},
		{"missing input", func(r *dto.DecisionsRequest) { r.Input = nil }},
		{"null input", func(r *dto.DecisionsRequest) { r.Input = []byte(`null`) }},
		{"object input", func(r *dto.DecisionsRequest) { r.Input = []byte(`{}`) }},
		{"missing questions", func(r *dto.DecisionsRequest) { r.Questions = nil }},
		{"too many questions", func(r *dto.DecisionsRequest) { r.Questions = make([]dto.DecisionQuestion, 201) }},
		{"missing instructions", func(r *dto.DecisionsRequest) { r.Questions[0].Instructions = nil }},
		{"unknown question type", func(r *dto.DecisionsRequest) { r.Questions[0].Type = "noul" }},
		{"too few choices", func(r *dto.DecisionsRequest) {
			r.Questions[0].Type = "choice"
			r.Questions[0].Choices = []dto.DecisionChoice{{Value: false}}
		}},
		{"too many choices", func(r *dto.DecisionsRequest) {
			r.Questions[0].Type = "choice"
			r.Questions[0].Choices = make([]dto.DecisionChoice, 256)
		}},
		{"duplicate choices", func(r *dto.DecisionsRequest) {
			r.Questions[0].Type = "choice"
			r.Questions[0].Choices = []dto.DecisionChoice{{Value: false}, {Value: false}}
		}},
		{"non scalar choice", func(r *dto.DecisionsRequest) {
			r.Questions[0].Type = "choice"
			r.Questions[0].Choices = []dto.DecisionChoice{{Value: []any{}}, {Value: true}}
		}},
		{"missing level label", func(r *dto.DecisionsRequest) {
			r.Questions[0].Type = "score"
			r.Questions[0].Levels = []dto.DecisionLevel{{}}
		}},
		{"too many images", func(r *dto.DecisionsRequest) {
			r.Input = []byte(`[{"role":"user","content":[` + strings.Repeat(`{"type":"input_image","image_url":"data:image/png;base64,aA=="},`, 128) + `{"type":"input_image","image_url":"data:image/png;base64,aA=="}]}]`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := common.DeepCopy(&base)
			require.NoError(t, err)
			tc.edit(request)
			assert.Error(t, helper.ValidateDecisionsRequest(request))
		})
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, raw := range []string{
		`{"role":"system","content":"instruction"}`,
		`{"type":"function_call","name":"f","call_id":"a","arguments":"{}"}`,
		`{"role":"user","content":[{"type":"input_text","text":"cached","prompt_cache_breakpoint":{"mode":"explicit"}}]}`,
		`{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aA==","detail":"original"}]}`,
	} {
		request, err := common.DeepCopy(&base)
		require.NoError(t, err)
		request.Input = []byte("[" + raw + "]")
		_, err = openrouter.ConvertDecisionsRequest(ctx, request)
		assert.Error(t, err)
	}
	base.PromptCacheKey = common.GetPointer("")
	_, err := openrouter.ConvertDecisionsRequest(ctx, &base)
	assert.ErrorContains(t, err, "prompt_cache_key")
	base.PromptCacheKey = nil
	base.PromptCacheOptions = []byte(`{"mode":"explicit"}`)
	_, err = openrouter.ConvertDecisionsRequest(ctx, &base)
	assert.ErrorContains(t, err, "prompt_cache_options")
}

func TestDecisionsUsageSafety(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `{"input_tokens":1}`, `{"input_tokens":-1,"output_tokens":0}`,
		`{"input_tokens":1,"output_tokens":-1}`, `{"input_tokens":2147483647,"output_tokens":1}`,
		`{"input_tokens":100,"output_tokens":0,"input_tokens_details":{"cached_tokens":101}}`,
		`{"input_tokens":100,"output_tokens":0,"input_tokens_details":{"cache_write_tokens":-1}}`,
		`{"input_tokens":100,"output_tokens":0,"input_tokens_details":{"cached_tokens":60,"cache_write_tokens":2147483647}}`,
		`{"input_tokens":100,"output_tokens":0,"output_tokens_details":{"reasoning_tokens":1}}`,
	} {
		var usage *dto.DecisionUsage
		require.NoError(t, common.UnmarshalJsonStr(raw, &usage))
		_, err := decisionsBillingUsage(usage)
		assert.Error(t, err, raw)
	}
	for _, count := range []int{0, math.MaxInt32} {
		usage, err := decisionsBillingUsage(&dto.DecisionUsage{InputTokens: &count, OutputTokens: common.GetPointer(0)})
		require.NoError(t, err)
		assert.Equal(t, count, usage.TotalTokens)
	}
	var overlapping dto.DecisionUsage
	require.NoError(t, common.UnmarshalJsonStr(`{"input_tokens":100,"output_tokens":0,"input_tokens_details":{"cached_tokens":60,"cache_write_tokens":50}}`, &overlapping))
	_, err := decisionsBillingUsage(&overlapping)
	require.NoError(t, err, "OpenAI cache writes can overlap cached prefixes")
}

func TestDecisionsRelayBillingAndLogs(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			database := setupRelayBillingDatabase(t, dialect)
			for caseIndex, tc := range []struct {
				name          string
				channel       int
				response      string
				status, quota int
			}{
				{"openrouter", constant.ChannelTypeOpenRouter, `{"model":"openai/gpt-6-luna-decisions","answers":{"0":{"type":"noul","noul":0.95}},"usage":{"input_tokens":1000,"output_tokens":0,"cost":0.0001}}`, 200, 50},
				{"native cache", constant.ChannelTypeOpenAI, `{"model":"gpt-6-luna","answers":[{"type":"predicate","name":null,"probability":0.95}],"usage":{"input_tokens":1000,"output_tokens":0,"total_tokens":1000,"input_tokens_details":{"cached_tokens":200,"cache_write_tokens":100},"output_tokens_details":{"reasoning_tokens":0}}}`, 200, 35},
				{"free cache", constant.ChannelTypeOpenRouter, `{"answers":{"0":{"type":"noul","noul":0}},"usage":{"input_tokens":1000,"output_tokens":0,"input_tokens_details":{"cached_tokens":1000}}}`, 200, 0},
				{"missing usage", constant.ChannelTypeOpenRouter, `{"answers":{"0":{"type":"noul","noul":0.5}}}`, 200, -1},
				{"negative usage", constant.ChannelTypeOpenRouter, `{"answers":{"0":{"type":"noul","noul":0.5}},"usage":{"input_tokens":-1,"output_tokens":0}}`, 200, -1},
				{"upstream error", constant.ChannelTypeOpenRouter, `{"error":{"message":"unavailable","code":503}}`, 503, -1},
				{"retry", constant.ChannelTypeOpenRouter, `{"model":"openai/gpt-6-luna-decisions","answers":{"0":{"type":"noul","noul":1}},"usage":{"input_tokens":1000,"output_tokens":0}}`, 200, 50},
				{"logging disabled", constant.ChannelTypeOpenRouter, `{"answers":{"0":{"type":"noul","noul":1}},"usage":{"input_tokens":1000,"output_tokens":0}}`, 200, 50},
			} {
				t.Run(tc.name, func(t *testing.T) {
					captured := make(chan []byte, 2)
					var attempts atomic.Int32
					if tc.name == "logging disabled" {
						t.Setenv("SAVE_REQUEST_RESPONSE", "false")
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						expectedPath := "/v1/decisions"
						if tc.channel == constant.ChannelTypeOpenRouter {
							expectedPath = "/api/alpha/decisions"
						}
						assert.Equal(t, expectedPath, r.URL.Path)
						assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
						body, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						captured <- body
						if attempts.Add(1) == 1 && tc.name == "retry" {
							w.WriteHeader(http.StatusServiceUnavailable)
							_, _ = io.WriteString(w, `{"error":{"message":"retry","code":503}}`)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.status)
						_, _ = io.WriteString(w, tc.response)
					}))
					defer server.Close()
					user := model.User{Username: tc.name, AffCode: fmt.Sprintf("d%d", caseIndex), Quota: 10000, Group: "default", Status: common.UserStatusEnabled}
					require.NoError(t, database.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: "decisions-" + tc.name, RemainQuota: 10000, Status: common.TokenStatusEnabled}
					require.NoError(t, database.Create(&token).Error)
					channel := model.Channel{Type: tc.channel, Name: tc.name}
					require.NoError(t, database.Create(&channel).Error)
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					requestJSON := decisionsRequestJSON
					if tc.channel == constant.ChannelTypeOpenAI {
						requestJSON = `{"model":"gpt-6-luna","input":[{"role":"developer","content":"Classify the tool output"},{"type":"function_call","name":"inspect","call_id":"c1","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":[{"type":"input_text","text":"A broken screen","prompt_cache_breakpoint":{"mode":"explicit"}}]}],"questions":[{"type":"predicate","instructions":"Is it damaged?"}],"safety_identifier":"","prompt_cache_key":"","prompt_cache_options":{"mode":"explicit","ttl":"30m"}}`
					}
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(requestJSON))
					ctx.Request.Header.Set("Content-Type", "application/json")
					baseURL := server.URL
					if tc.channel == constant.ChannelTypeOpenRouter {
						baseURL += "/api/v1"
					}
					common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-6-luna")
					common.SetContextKey(ctx, constant.ContextKeyChannelType, tc.channel)
					common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, baseURL)
					common.SetContextKey(ctx, constant.ContextKeyChannelKey, "upstream-key")
					common.SetContextKey(ctx, constant.ContextKeyChannelId, channel.Id)
					if tc.channel == constant.ChannelTypeOpenRouter {
						ctx.Set("model_mapping", `{"gpt-6-luna":"openai/gpt-6-luna-decisions"}`)
					}
					request, err := helper.GetAndValidateRequest(ctx, types.RelayFormatOpenAIDecisions)
					require.NoError(t, err)
					info, err := relaycommon.GenRelayInfo(ctx, types.RelayFormatOpenAIDecisions, request, nil)
					require.NoError(t, err)
					assert.Equal(t, relayconstant.RelayModeDecisions, info.RelayMode)
					info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, token.Key
					info.UsingGroup, info.UserGroup = "default", "default"
					info.UserSetting.BillingPreference = "wallet_only"
					info.StartTime = time.Now()
					price, err := helper.ModelPriceHelper(ctx, info, 2000, request.GetTokenCountMeta())
					require.NoError(t, err)
					assert.Equal(t, "gpt-6-luna-decisions", info.GetBillingModelName())
					assert.Equal(t, 100, price.QuotaToPreConsume)
					require.Nil(t, service.PreConsumeBilling(ctx, price.QuotaToPreConsume, info))
					reserved, err := model.GetUserQuota(user.Id, true)
					require.NoError(t, err)
					assert.Equal(t, 9900, reserved)
					apiErr := DecisionsHelper(ctx, info)
					capturedBody := <-captured
					if tc.name == "retry" {
						require.NotNil(t, apiErr)
						assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
						assert.False(t, ctx.Writer.Written())
						apiErr = DecisionsHelper(ctx, info)
						capturedBody = <-captured
						assert.Equal(t, int32(2), attempts.Load())
					}
					assert.Equal(t, "gpt-6-luna", request.(*dto.DecisionsRequest).Model)
					if tc.quota < 0 {
						require.NotNil(t, apiErr)
						assert.False(t, ctx.Writer.Written())
						RefundFailedRequestBilling(ctx, info, apiErr)
						require.Eventually(t, func() bool {
							var currentUser model.User
							var currentToken model.Token
							return database.First(&currentUser, user.Id).Error == nil && database.First(&currentToken, token.Id).Error == nil && currentUser.Quota == 10000 && currentToken.RemainQuota == 10000
						}, time.Second*5, time.Millisecond*10)
						var count int64
						require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ?", user.Id).Count(&count).Error)
						assert.Zero(t, count)
						return
					}
					require.Nil(t, apiErr)
					require.Equal(t, 200, recorder.Code)
					var result dto.DecisionsResponse
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
					assert.Equal(t, "gpt-6-luna", result.Model)
					require.Len(t, result.Answers, 1)
					assert.Equal(t, "predicate", result.Answers[0].Type)
					if tc.channel == constant.ChannelTypeOpenRouter {
						assert.Contains(t, string(capturedBody), `"model":"openai/gpt-6-luna-decisions"`)
					} else {
						assert.JSONEq(t, requestJSON, string(capturedBody))
						assert.Equal(t, tc.response, recorder.Body.String())
					}
					var stored model.Log
					require.NoError(t, model.LOG_DB.Where("user_id = ?", user.Id).Take(&stored).Error)
					if tc.name == "logging disabled" {
						assert.Empty(t, stored.Request)
						assert.Empty(t, stored.Response)
					} else {
						assert.JSONEq(t, string(capturedBody), stored.Request)
						assert.JSONEq(t, tc.response, stored.Response)
					}
					assert.Equal(t, tc.quota, stored.Quota)
					inputTokens := 1000 - result.Usage.InputTokensDetails.CachedTokens - result.Usage.InputTokensDetails.CacheWriteTokens
					assert.Equal(t, inputTokens, stored.PromptTokens)
					assert.Contains(t, stored.Other, `"input_tokens_total":1000`)
					assert.Zero(t, stored.CompletionTokens)
					assert.Contains(t, stored.Other, "OpenAI Decisions")
					require.NoError(t, database.First(&user, user.Id).Error)
					require.NoError(t, database.First(&token, token.Id).Error)
					assert.Equal(t, 10000-tc.quota, user.Quota)
					assert.Equal(t, 10000-tc.quota, token.RemainQuota)
					assert.Equal(t, tc.quota, user.UsedQuota)
					assert.False(t, info.Billing.NeedsRefund())
				})
			}
		})
	}
}

func setupRelayBillingDatabase(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	dbType := common.DatabaseTypeSQLite
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		dialector, dbType = mysql.Open(dsn), common.DatabaseTypeMySQL
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		dialector, dbType = postgres.Open(dsn), common.DatabaseTypePostgreSQL
	default:
		dialector = sqlite.Open(":memory:")
	}
	database, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("decisions_%d_", time.Now().UnixNano())}})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	versionQuery := "SELECT VERSION()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, database.Raw(versionQuery).Scan(&version).Error)
	t.Logf("%s version: %s", dialect, version)
	models := []any{&model.User{}, &model.Token{}, &model.Channel{}, &model.SettlementConfig{}}
	require.NoError(t, database.AutoMigrate(models...))
	logDatabase, logType := database, dbType
	if dialect == "mysql" {
		// The existing Log model hard-codes PostgreSQL jsonb for extra/header.
		// Exercise a supported separate log database without changing that
		// unrelated migration contract or masking it with a fake MySQL schema.
		t.Log("MySQL main database with separate SQLite log database: existing Log jsonb tags prevent MySQL log AutoMigrate")
		logDatabase, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		logSQL, err := logDatabase.DB()
		require.NoError(t, err)
		logSQL.SetMaxOpenConns(1)
		logType = common.DatabaseTypeSQLite
		t.Cleanup(func() { require.NoError(t, logSQL.Close()) })
	}
	require.NoError(t, logDatabase.AutoMigrate(&model.Log{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldBatch, oldLog := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	oldExport := common.DataExportEnabled
	oldQuota := *operation_setting.GetQuotaSetting()
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	oldSettings := *settings
	oldRatios, oldPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldGroupRatios := ratio_setting.GroupRatio2JSONString()
	oldQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldRedis, oldBatch, oldLog
		common.DataExportEnabled = oldExport
		*operation_setting.GetQuotaSetting(), *settings = oldQuota, oldSettings
		common.QuotaPerUnit = oldQuotaPerUnit
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, logDatabase.Migrator().DropTable(&model.Log{}))
		require.NoError(t, database.Migrator().DropTable(models...))
		require.NoError(t, sqlDB.Close())
	})
	model.DB, model.LOG_DB = database, logDatabase
	common.SetDatabaseTypes(dbType, logType)
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, true
	common.DataExportEnabled = false
	common.QuotaPerUnit = 500000
	operation_setting.GetQuotaSetting().TrustQuotaUSD = 0
	operation_setting.GetQuotaSetting().PreConsumeMultiplier = 1
	*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}}
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Setenv("SAVE_REQUEST_RESPONSE", "true")
	return database
}

func TestDecisionsOpenRouterURL(t *testing.T) {
	for _, base := range []string{"https://openrouter.ai/api", "https://openrouter.ai/api/v1/", "https://openrouter.ai/api/alpha/decisions"} {
		url, err := (&typesafe.Adaptor{}).GetRequestURL(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, ChannelBaseUrl: base}})
		require.NoError(t, err)
		assert.Equal(t, "https://openrouter.ai/api/alpha/decisions", url)
	}
}

func TestDecisionsPricing(t *testing.T) {
	setupRelayBillingDatabase(t, "sqlite")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(decisionsRequestJSON))
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-6-luna", UsingGroup: "default", UserGroup: "default", RelayFormat: types.RelayFormatOpenAIDecisions}
	price, err := helper.ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, 100, price.QuotaToPreConsume)
	for _, tc := range []struct{ input, read, write, output, expected int }{
		{1000, 200, 100, 99, 35},
		{272000, 200000, 20000, 1000, 2600},
		{272001, 200000, 20000, 1000, 5200},
		{1000, 1000, 0, 0, 0},
		{3619, 2921, 3616, 36, 0},
	} {
		usage := &dto.Usage{PromptTokens: tc.input, CompletionTokens: tc.output, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: tc.read, CacheWriteTokens: tc.write}}
		params := service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString))
		result, err := billingexpr.ComputeTieredQuotaWithRequest(info.TieredBillingSnapshot, params, billingexpr.RequestInput{})
		require.NoError(t, err)
		assert.Equal(t, tc.expected, result.ActualQuotaAfterGroup)
	}
	// The fallback does not introduce Decisions prices for ordinary chat.
	assert.False(t, helper.HasPriceOrRatioEntry("gpt-6-luna"))
	for _, modelName := range []string{"gpt-6-luna", "gpt-6-luna-decisions"} {
		settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
		settings.BillingMode = map[string]string{modelName: billing_setting.BillingModeTieredExpr}
		settings.BillingExpr = map[string]string{modelName: `tier("custom", p * 3)`}
		info.BillingModelName = ""
		price, err := helper.ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})
		require.NoError(t, err)
		assert.Equal(t, 3000, price.QuotaToPreConsume)
	}
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	settings.BillingMode, settings.BillingExpr = map[string]string{}, map[string]string{}
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-6-luna":0}`))
	info.BillingModelName = ""
	info.TieredBillingSnapshot = nil
	price, err = helper.ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Zero(t, price.QuotaToPreConsume)
	assert.Nil(t, info.TieredBillingSnapshot)
}
