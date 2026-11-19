package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryHonorsPinRetryMode(t *testing.T) {
	openaiErr := types.NewOpenAIError(errors.New("upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	c := newPinRetryContext()
	assert.True(t, service.ShouldRetryRelayError(c, openaiErr, 1))

	origin := newPinRetryContext()
	service.GetChannelConstraints(origin).AddPin(dto.ChannelPin{
		ChannelId: 2,
		Source:    dto.PinSourceOriginTask,
		Rank:      dto.PinRankOriginTask,
		RetryMode: dto.PinRetrySameChannel,
	})
	assert.True(t, service.ShouldRetryRelayError(origin, openaiErr, 1), "origin pin retries on the same channel")

	token := newPinRetryContext()
	service.GetChannelConstraints(token).AddPin(dto.ChannelPin{
		ChannelId: 1,
		Source:    dto.PinSourceToken,
		Rank:      dto.PinRankToken,
		RetryMode: dto.PinRetrySingleAttempt,
	})
	assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}, service.DecideRelayRetry(token, openaiErr, 1), "token pin suppresses retry")
}

func TestShouldRetryTaskRelayHonorsPinRetryMode(t *testing.T) {
	taskErr := &dto.TaskError{StatusCode: http.StatusInternalServerError}

	c := newPinRetryContext()
	assert.Equal(t, "retry", decideTaskRetry(c, taskErr, 1).Action)

	origin := newPinRetryContext()
	service.GetChannelConstraints(origin).AddPin(dto.ChannelPin{
		ChannelId: 2,
		Source:    dto.PinSourceOriginTask,
		Rank:      dto.PinRankOriginTask,
		RetryMode: dto.PinRetrySameChannel,
	})
	assert.Equal(t, "retry", decideTaskRetry(origin, taskErr, 1).Action)

	token := newPinRetryContext()
	service.GetChannelConstraints(token).AddPin(dto.ChannelPin{
		ChannelId: 1,
		Source:    dto.PinSourceToken,
		Rank:      dto.PinRankToken,
		RetryMode: dto.PinRetrySingleAttempt,
	})
	assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}, decideTaskRetry(token, taskErr, 1))
}

func TestSameChannelPinsMergeToStricterRetryMode(t *testing.T) {
	c := newPinRetryContext()
	constraints := service.GetChannelConstraints(c)
	constraints.AddPin(dto.ChannelPin{
		ChannelId: 7,
		Source:    dto.PinSourceOriginTask,
		Rank:      dto.PinRankOriginTask,
		RetryMode: dto.PinRetrySameChannel,
	})
	constraints.AddPin(dto.ChannelPin{
		ChannelId: 7,
		Source:    dto.PinSourceToken,
		Rank:      dto.PinRankToken,
		RetryMode: dto.PinRetrySingleAttempt,
	})
	pin, found, overridden := constraints.ResolvedPin()
	require.True(t, found)
	assert.Equal(t, 7, pin.ChannelId)
	assert.Equal(t, dto.PinRetrySingleAttempt, pin.RetryMode)
	assert.Empty(t, overridden)
	assert.False(t, service.ShouldRetryRelayError(c, types.NewOpenAIError(errors.New("upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError), 1))
}

func TestExplicitChannelRoutesIgnoreRetryStatusCodes(t *testing.T) {
	previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = nil
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previousRanges })
	for _, status := range []int{400, 408, 429, 500, 504, 524} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := newPinRetryContext()
			c.Set("token_channel_ids", []int{1, 2})
			c.Set("new_retry_times", 1)
			apiErr := types.NewOpenAIError(errors.New("upstream rejected request"), types.ErrorCodeBadResponseStatusCode, status)
			want := service.PolicyDecision{Action: "retry", Reason: "channel_error", Source: "routing"}
			assert.Equal(t, want, service.DecideRelayRetry(c, apiErr, 1))
			assert.Equal(t, want, decideTaskRetry(c, &dto.TaskError{StatusCode: status}, 1))
			assert.Equal(t, "stop", service.DecideRelayRetry(c, apiErr, 0).Action, "the route cannot exceed its attempt budget")
			assert.Equal(t, "stop", decideTaskRetry(c, &dto.TaskError{StatusCode: status}, 0).Action)

			ordinary := newPinRetryContext()
			assert.Equal(t, "stop", service.DecideRelayRetry(ordinary, apiErr, 1).Action, "ordinary selection still follows the configured status rules")
		})
	}
	t.Run("client WebSocket handshake still permits upstream fallback", func(t *testing.T) {
		c := newPinRetryContext()
		c.Set("token_channel_ids", []int{1, 2})
		c.Set("new_retry_times", 1)
		c.Request.Header.Set("Connection", "Upgrade")
		c.Request.Header.Set("Upgrade", "websocket")
		c.Writer.WriteHeaderNow()
		apiErr := types.NewOpenAIError(errors.New("upstream handshake rejected"), types.ErrorCodeBadResponseStatusCode, 400)
		assert.Equal(t, "retry", service.DecideRelayRetry(c, apiErr, 1).Action)
	})

	for _, tc := range []struct {
		name    string
		apiErr  *types.NewAPIError
		taskErr *dto.TaskError
		setup   func(*gin.Context)
	}{
		{name: "successful HTTP status", apiErr: types.NewOpenAIError(errors.New("invalid response"), types.ErrorCodeBadResponseStatusCode, 200), taskErr: &dto.TaskError{StatusCode: 200}},
		{name: "local validation and accepted tasks", apiErr: types.NewErrorWithStatusCode(errors.New("invalid request"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry()), taskErr: &dto.TaskError{StatusCode: 400, NoRetry: true}},
		{name: "invalid response body and local task error", apiErr: types.NewOpenAIError(errors.New("invalid response"), types.ErrorCodeBadResponseBody, 400), taskErr: &dto.TaskError{StatusCode: 400, LocalError: true}},
		{name: "single attempt pin", setup: func(c *gin.Context) {
			service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 1, Source: dto.PinSourceToken, RetryMode: dto.PinRetrySingleAttempt})
		}},
		{name: "origin task cannot switch channels", setup: func(c *gin.Context) {
			service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: 1, Source: dto.PinSourceOriginTask, RetryMode: dto.PinRetrySameChannel})
		}},
		{name: "strict session", setup: func(c *gin.Context) { c.Set("channel_affinity_skip_retry_on_failure", true) }},
		{name: "response already started", setup: func(c *gin.Context) {
			_, err := c.Writer.WriteString("data: partial response\n\n")
			require.NoError(t, err)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newPinRetryContext()
			c.Set("token_channel_ids", []int{1, 2})
			c.Set("new_retry_times", 1)
			if tc.setup != nil {
				tc.setup(c)
			}
			apiErr, taskErr := tc.apiErr, tc.taskErr
			if apiErr == nil {
				apiErr = types.NewOpenAIError(errors.New("upstream rejected request"), types.ErrorCodeBadResponseStatusCode, 400)
			}
			if taskErr == nil {
				taskErr = &dto.TaskError{StatusCode: 400}
			}
			assert.Equal(t, "stop", service.DecideRelayRetry(c, apiErr, 1).Action)
			assert.Equal(t, "stop", decideTaskRetry(c, taskErr, 1).Action)
		})
	}
}

func TestExplicitChannelRouteRetriesUpstreamRejections(t *testing.T) {
	require.NoError(t, i18n.Init())
	t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
	db := modelManagementDB(t, "sqlite", "")
	require.NoError(t, db.AutoMigrate(&model.SettlementConfig{}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gemini-route-test":0}`))
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "route-test", Group: "default"}).Error)
	previousConsume, previousError, previousSave := common.LogConsumeEnabled, constant.ErrorLogEnabled, common.SaveErrorLog
	previousExport, previousBatch := common.DataExportEnabled, common.BatchUpdateEnabled
	previousRetry, previousCount := common.RetryTimes, constant.CountToken
	previousAffinity := *operation_setting.GetChannelAffinitySetting()
	previousFetch := *system_setting.GetFetchSetting()
	previousTLS := common.TLSInsecureSkipVerify
	previousFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	t.Cleanup(func() {
		common.LogConsumeEnabled, constant.ErrorLogEnabled, common.SaveErrorLog = previousConsume, previousError, previousSave
		common.DataExportEnabled, common.BatchUpdateEnabled = previousExport, previousBatch
		common.RetryTimes, constant.CountToken = previousRetry, previousCount
		*operation_setting.GetChannelAffinitySetting() = previousAffinity
		*system_setting.GetFetchSetting() = previousFetch
		common.TLSInsecureSkipVerify = previousTLS
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = previousFreePreConsume
	})
	common.LogConsumeEnabled, constant.ErrorLogEnabled, common.SaveErrorLog = false, false, false
	common.DataExportEnabled, common.BatchUpdateEnabled = false, false
	common.RetryTimes, constant.CountToken = 0, false
	operation_setting.GetChannelAffinitySetting().Enabled = false
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().ApplyIPFilterForDomain = true
	common.TLSInsecureSkipVerify = false
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	common.OptionMap["GeminiFileRouting"] = `{"enabled":true,"mode":"forward","chat_model_patterns":["gemini-route-test"]}`
	service.InitHttpClient()
	var firstStatus, secondStatus atomic.Int32
	var calls [3]atomic.Int32
	for i := range calls {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[i].Add(1)
			status := int(firstStatus.Load())
			if i == 1 {
				status = int(secondStatus.Load())
			}
			if status == 0 {
				connection, _, err := w.(http.Hijacker).Hijack()
				if assert.NoError(t, err) {
					assert.NoError(t, connection.Close())
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if status != http.StatusOK {
				_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"channel rejected request","status":"INVALID_ARGUMENT"}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
		}))
		t.Cleanup(upstream.Close)
		channel := &model.Channel{Id: i + 1, Name: fmt.Sprintf("route-%d", i+1), Type: constant.ChannelTypeGemini, Key: "test-only", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Group: "default", Models: "gemini-route-test"}
		require.NoError(t, db.Create(channel).Error)
	}
	for _, endpoint := range []struct {
		name   string
		path   string
		format types.RelayFormat
	}{
		{name: "chat", path: "/v1/chat/completions", format: types.RelayFormatOpenAI},
		{name: "native", path: "/v1beta/models/gemini-route-test:generateContent", format: types.RelayFormatGemini},
	} {
		for _, attachment := range []string{"none", "inline", "remote"} {
			for _, tc := range []struct {
				status, secondStatus, retries, wantStatus int
				wantChannels                              []string
			}{
				{400, 200, 9, 200, []string{"1", "2"}},
				{408, 200, 9, 200, []string{"1", "2"}},
				{504, 200, 9, 200, []string{"1", "2"}},
				{524, 200, 9, 200, []string{"1", "2"}},
				{400, 400, 9, 400, []string{"1", "2"}},
				{400, 200, 0, 400, []string{"1"}},
				{0, 200, 9, 200, []string{"1", "2"}},
			} {
				t.Run(fmt.Sprintf("%s/%s/status=%d/second=%d/retries=%d", endpoint.name, attachment, tc.status, tc.secondStatus, tc.retries), func(t *testing.T) {
					for i := range calls {
						calls[i].Store(0)
					}
					firstStatus.Store(int32(tc.status))
					secondStatus.Store(int32(tc.secondStatus))
					body := `{"model":"gemini-route-test","messages":[{"role":"user","content":"hello"}]}`
					if attachment == "remote" {
						body = `{"model":"gemini-route-test","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://generativelanguage.googleapis.com/v1beta/files/test","mime_type":"image/png"}}]}]}`
					} else if attachment == "inline" {
						body = `{"model":"gemini-route-test","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}]}`
					}
					if endpoint.format == types.RelayFormatGemini {
						body = `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
						if attachment == "remote" {
							body = `{"contents":[{"role":"user","parts":[{"fileData":{"mimeType":"image/png","fileUri":"https://generativelanguage.googleapis.com/v1beta/files/test"}}]}]}`
						} else if attachment == "inline" {
							body = `{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]}]}`
						}
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Set("id", 7)
					common.SetContextKey(c, constant.ContextKeyUserSetting, kitdto.UserSetting{BillingPreference: "wallet_only"})
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-route-test")
					c.Set("token_channel_ids", []int{1, 2})
					c.Set("new_retry_times", tc.retries)
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					filePlan, apiErr := service.PlanGeminiFiles(c, "gemini-route-test")
					require.Nil(t, apiErr)
					if filePlan != nil {
						t.Cleanup(func() { filePlan.Close(c) })
					}
					RelayWithoutRace(c, endpoint.format)
					wantStatus, wantChannels := tc.wantStatus, tc.wantChannels
					if tc.status == 0 && attachment != "none" {
						wantStatus, wantChannels = http.StatusInternalServerError, []string{"1"}
						if attachment == "remote" {
							wantStatus = http.StatusBadGateway
						}
					}
					assert.Equal(t, wantStatus, recorder.Code, recorder.Body.String())
					if wantStatus == http.StatusOK {
						assert.Contains(t, recorder.Body.String(), "ok")
					}
					assert.Equal(t, wantChannels, c.GetStringSlice("use_channel"))
					assert.Equal(t, int32(1), calls[0].Load())
					assert.Equal(t, int32(len(wantChannels)-1), calls[1].Load())
					assert.Zero(t, calls[2].Load(), "the request must stay within its explicit channel list")
				})
			}
		}
	}
}

func newPinRetryContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestRequestPolicyConfigReturnsSettingsWithoutMigration(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	GetRequestPolicy(ctx)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Contains(t, response.Data, "options")
	assert.NotContains(t, response.Data, "migration")
	assert.NotContains(t, response.Data, "differences")
}

func TestChannelAffinityRetryPriorities(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := ""
			if dialect.env != "" {
				dsn = os.Getenv(dialect.env)
				if dsn == "" {
					t.Skipf("%s not configured", dialect.env)
				}
			}
			db := modelManagementDB(t, dialect.kind, dsn)
			previousRetryTimes := common.RetryTimes
			previousGroups := setting.UserUsableGroups2JSONString()
			previousMaxGroups := setting.GetMaxTokenAutoGroups()
			affinity := operation_setting.GetChannelAffinitySetting()
			previousAffinity := *affinity
			t.Cleanup(func() {
				common.RetryTimes = previousRetryTimes
				*affinity = previousAffinity
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprint(previousMaxGroups)))
			})
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","empty":"Empty"}`))
			require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"empty":1}`))
			const modelName = "affinity-retry-test"
			for _, spec := range []struct {
				id       int
				priority int64
				group    string
			}{{24, 4, "default"}, {27, 3, "default"}, {30, 2, "default"}, {34, 4, "vip"}, {37, 3, "vip"}} {
				channel := model.Channel{
					Id: spec.id, Name: fmt.Sprintf("channel-%d", spec.id), Type: constant.ChannelTypeOpenAI,
					Key: "test-only", Status: common.ChannelStatusEnabled, Models: modelName, Group: spec.group,
					Priority: common.GetPointer(spec.priority), Weight: common.GetPointer(uint(1)),
				}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(db))
			}
			for _, cached := range []bool{false, true} {
				common.MemoryCacheEnabled = cached
				model.InitChannelCache()
				for _, tc := range []struct {
					name, mode string
					binding    int
					retries    int
					autoGroups []string
					crossGroup bool
					want       []int
				}{
					{name: "ordinary routing", mode: "off", retries: 2, want: []int{24, 27, 30}},
					{name: "affinity miss", mode: "prefer", retries: 2, want: []int{24, 27, 30}},
					{name: "affinity hit", mode: "prefer", binding: 27, retries: 2, want: []int{27, 24, 27}},
					{name: "no retry budget", mode: "prefer", binding: 27, want: []int{27}},
					{name: "one retry", mode: "prefer", binding: 27, retries: 1, want: []int{27, 24}},
					{name: "strict session", mode: "strict", binding: 27, retries: 2, want: []int{27}},
					{name: "auto group", mode: "prefer", binding: 27, retries: 1, autoGroups: []string{"default", "vip"}, want: []int{27, 24}},
					{name: "cross group", mode: "prefer", binding: 27, retries: 1, autoGroups: []string{"default", "vip"}, crossGroup: true, want: []int{27, 24, 34, 37}},
					{name: "skip empty group", mode: "prefer", binding: 27, retries: 1, autoGroups: []string{"empty", "default"}, want: []int{27, 24, 27}},
				} {
					t.Run(fmt.Sprintf("cache=%t/%s", cached, tc.name), func(t *testing.T) {
						common.RetryTimes = tc.retries
						*affinity = operation_setting.ChannelAffinitySetting{
							Enabled: true, DefaultTTLSeconds: 60, MaxEntries: 100,
							Rules: []operation_setting.ChannelAffinityRule{{
								Name: t.Name(), ModelRegex: []string{"^" + modelName + "$"}, SessionMode: tc.mode,
								KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
							}},
						}
						ctx := newPinRetryContext()
						ctx.Request.Header.Set("X-Session", t.Name())
						group := "default"
						common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
						if tc.autoGroups != nil {
							group = "auto"
							common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, tc.autoGroups)
							common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, tc.crossGroup)
						}
						if tc.binding != 0 {
							_, found := service.GetPreferredChannelByAffinity(ctx, modelName, group)
							require.False(t, found)
							service.RecordChannelAffinity(ctx, tc.binding)
							t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(ctx) })
						}
						// HTTP distribution and the relay loop use separate RetryParam values.
						channel, _, selectionErr := service.SelectChannelForRequest(ctx, modelName, &service.RetryParam{Ctx: ctx, TokenGroup: group, ModelName: modelName})
						require.Nil(t, selectionErr)
						require.NotNil(t, channel)
						retry := &service.RetryParam{Ctx: ctx, TokenGroup: group, ModelName: modelName}
						upstreamErr := types.NewOpenAIError(errors.New("upstream failure"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
						var attempts []int
						for ; retry.GetRetry() <= common.RetryTimes; retry.IncreaseRetry() {
							if len(attempts) > 0 {
								var err error
								channel, _, err = service.CacheGetRandomSatisfiedChannel(retry)
								require.NoError(t, err)
								if channel == nil {
									break
								}
							}
							service.AppendUsedChannel(ctx, channel.Id)
							attempts = append(attempts, channel.Id)
							require.LessOrEqual(t, len(attempts), len(tc.want), "retry budget must not increase")
							if service.DecideRelayRetry(ctx, upstreamErr, common.RetryTimes-retry.GetRetry()).Action != "retry" {
								break
							}
						}
						assert.Equal(t, tc.want, attempts)
					})
				}
			}
		})
	}
}

func TestRequestPolicyRoutingDatabaseMatrix(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := ""
			if dialect.env != "" {
				dsn = os.Getenv(dialect.env)
				if dsn == "" {
					t.Skipf("%s not configured", dialect.env)
				}
			}
			db := modelManagementDB(t, dialect.kind, dsn)
			require.NoError(t, db.AutoMigrate(&model.ModelRouteConfig{}))
			previousGroups := setting.UserUsableGroups2JSONString()
			previousRatios, err := common.Marshal(ratio_setting.GetGroupRatioCopy())
			require.NoError(t, err)
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			t.Cleanup(func() {
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(previousRatios)))
			})
			// Channel 1 holds the session binding but is no longer usable.
			channels := []model.Channel{
				{Id: 1, Name: "A", Type: 1, Key: "test-only", Status: common.ChannelStatusAutoDisabled, Models: "policy-test", Group: "default", Priority: common.GetPointer(int64(10))},
				{Id: 2, Name: "B", Type: 1, Key: "test-only", Status: common.ChannelStatusEnabled, Models: "policy-test", Group: "default", Priority: common.GetPointer(int64(5))},
			}
			for i := range channels {
				require.NoError(t, db.Create(&channels[i]).Error)
				require.NoError(t, channels[i].AddAbilities(db))
			}
			affinity := operation_setting.GetChannelAffinitySetting()
			previousAffinity := *affinity
			t.Cleanup(func() { *affinity = previousAffinity })
			for _, cached := range []bool{false, true} {
				for _, keep := range []bool{false, true} {
					for _, tc := range []struct {
						globalMode, ruleMode string
						blocked              bool
					}{
						{"strict", "inherit", true},
						{"prefer", "strict", true},
						{"prefer", "inherit", false},
						{"strict", "prefer", false},
					} {
						t.Run(fmt.Sprintf("cache=%t/keep=%t/global=%s/rule=%s", cached, keep, tc.globalMode, tc.ruleMode), func(t *testing.T) {
							common.MemoryCacheEnabled = cached
							model.InitChannelCache()
							snapshot, err := model.BuildRequestPolicy(map[string]string{
								"channel_affinity_setting.enabled":                  "true",
								"channel_affinity_setting.session_mode":             tc.globalMode,
								"channel_affinity_setting.keep_on_channel_disabled": fmt.Sprint(keep),
								"channel_affinity_setting.rules":                    fmt.Sprintf(`[{"name":"session","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":%q}]`, tc.ruleMode),
							})
							require.NoError(t, err)
							*affinity = snapshot.Affinity
							seed := newPinRetryContext()
							seed.Request.Header.Set("X-Session", t.Name())
							_, found := service.GetPreferredChannelByAffinity(seed, "policy-test", "default")
							require.False(t, found)
							seed.Set("channel_id", 1)
							service.RecordChannelAffinity(seed, 1)
							t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(seed) })
							bound, found := service.GetPreferredChannelByAffinity(seed, "policy-test", "default")
							require.True(t, found)
							require.Equal(t, 1, bound)

							request := newPinRetryContext()
							request.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"policy-test"}`))
							request.Request.Header.Set("Content-Type", "application/json")
							request.Request.Header.Set("X-Session", t.Name())
							common.SetContextKey(request, constant.ContextKeyUsingGroup, "default")
							middleware.Distribute()(request)
							events := service.RequestPolicy(request).Events()
							assert.True(t, slices.ContainsFunc(events, func(event service.PolicyEvent) bool { return event.Decision.Reason == "session_rule_matched" }), "decision events are recorded for every request")
							bound, found = service.GetPreferredChannelByAffinity(seed, "policy-test", "default")
							if tc.blocked {
								assert.Equal(t, http.StatusServiceUnavailable, request.Writer.Status())
								assert.True(t, request.IsAborted())
								assert.Equal(t, keep, found, "binding retention is independent of strict request handling")
								if found {
									assert.Equal(t, 1, bound)
								}
								return
							}
							assert.False(t, request.IsAborted())
							assert.Equal(t, 2, common.GetContextKeyInt(request, constant.ContextKeyChannelId), "prefer falls back to the next eligible channel")
							require.True(t, found)
							assert.Equal(t, 2, bound, "a successful fallback rebinds the session")
						})
					}
				}
			}
		})
	}
}
