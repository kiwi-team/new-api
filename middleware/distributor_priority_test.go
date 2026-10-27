package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelRoutingPriority(t *testing.T) {
	require.NoError(t, appI18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelRouteConfig{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previousDB, previousCache, previousRedis := model.DB, common.MemoryCacheEnabled, common.RedisEnabled
	previousAffinity := *operation_setting.GetChannelAffinitySetting()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	model.DB, common.MemoryCacheEnabled, common.RedisEnabled = db, true, false
	t.Cleanup(func() {
		// Empty this fixture's channel cache before restoring the shared state.
		require.NoError(t, db.Where("id > ?", 0).Delete(&model.Channel{}).Error)
		require.NoError(t, db.Where("channel_id > ?", 0).Delete(&model.Ability{}).Error)
		model.InitChannelCache()
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = previousDB, previousCache, previousRedis
		*operation_setting.GetChannelAffinitySetting() = previousAffinity
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		if previousCache && previousDB != nil {
			model.InitChannelCache()
		}
	})
	*operation_setting.GetChannelAffinitySetting() = operation_setting.ChannelAffinitySetting{
		Enabled: true, SessionMode: "strict", DefaultTTLSeconds: 60,
		Rules: []operation_setting.ChannelAffinityRule{{
			Name: t.Name(), ModelRegex: []string{".*"}, SessionMode: "inherit", IncludeRuleName: true,
			KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
		}},
	}
	for _, channel := range []model.Channel{
		{Id: 1, Group: "vip", Models: "route-model", Status: common.ChannelStatusEnabled},
		{Id: 2, Group: "default", Models: "route-model", Status: common.ChannelStatusEnabled},
		{Id: 3, Group: "default", Models: "route-model", Status: common.ChannelStatusEnabled},
		{Id: 4, Group: "default", Models: "route-model", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(100))},
		{Id: 5, Group: "default", Models: "route-model", Status: common.ChannelStatusAutoDisabled},
	} {
		channel.Type, channel.Key = constant.ChannelTypeOpenAI, "test-key"
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&model.Ability{ChannelId: channel.Id, Group: channel.Group, Model: "route-model", Enabled: channel.Status == common.ChannelStatusEnabled, Priority: channel.Priority}).Error)
	}
	model.InitChannelCache()
	ordered := []dto.ChannelItem{{Ids: []int{1, 2}, RaceMode: dto.ChannelRaceGroupModeOrder}, {Ids: []int{3}, RaceMode: dto.ChannelRaceGroupModeOrder}}
	for _, tc := range []struct {
		name           string
		keyChannels    []dto.ChannelItem
		keyMode        string
		keyRetry       int
		keyPattern     string
		globalMode     string
		globalChannel  int
		bound          int
		wantCandidates []int
		wantSelected   int
		wantRetries    int
	}{
		{name: "key precedes enforce and disabled session", keyChannels: ordered, keyRetry: 2, globalMode: "enforce", globalChannel: 4, bound: 5, wantCandidates: []int{1, 2, 3}, wantSelected: 1, wantRetries: 2},
		{name: "session cannot move a later group ahead", keyChannels: ordered, keyRetry: 1, bound: 3, wantCandidates: []int{1, 2, 3}, wantSelected: 1, wantRetries: 1},
		{name: "session outside key list is ignored", keyChannels: ordered, bound: 4, wantCandidates: []int{1, 2, 3}, wantSelected: 1},
		{name: "explicit key channel need not use ordinary group lookup", keyChannels: ordered, bound: 1, wantCandidates: []int{1, 2, 3}, wantSelected: 1},
		{name: "key precedes unavailable enforce route", keyChannels: ordered, keyPattern: "^route-", globalMode: "enforce", globalChannel: 999, wantCandidates: []int{1, 2, 3}, wantSelected: 1},
		{name: "random key mode preserves group members", keyChannels: ordered, keyMode: "random", keyRetry: 2, globalMode: "enforce", globalChannel: 4, bound: 5, wantCandidates: []int{1, 2, 3}, wantRetries: 2},
		{name: "race keeps key groups and serial fallback order", keyChannels: ordered, keyMode: "race", globalMode: "enforce", globalChannel: 4, bound: 5, wantCandidates: []int{1, 2, 3}, wantSelected: 1, wantRetries: 2},
		{name: "unavailable key candidates fall back to global", keyChannels: []dto.ChannelItem{{Id: 5}}, keyRetry: 5, globalMode: "fallback", globalChannel: 3, wantCandidates: []int{3}, wantSelected: 3},
		{name: "unmatched key model falls back to global", keyChannels: ordered, keyPattern: "other-model", globalMode: "fallback", globalChannel: 3, wantCandidates: []int{3}, wantSelected: 3},
		{name: "global enforce precedes session and defaults", globalMode: "enforce", globalChannel: 3, bound: 5, wantCandidates: []int{3}, wantSelected: 3},
		{name: "global fallback precedes session and defaults", globalMode: "fallback", globalChannel: 3, bound: 5, wantCandidates: []int{3}, wantSelected: 3},
		{name: "default routing without explicit plan", wantSelected: 4, wantRetries: common.RetryTimes},
		{name: "unavailable global fallback uses defaults", globalMode: "fallback", globalChannel: 999, wantSelected: 4, wantRetries: common.RetryTimes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("id > ?", 0).Delete(&model.ModelRouteConfig{}).Error)
			if tc.globalMode != "" {
				require.NoError(t, db.Create(&model.ModelRouteConfig{
					Name: tc.name, ModelPatterns: `["^route-model$"]`, BodyMatch: `{"routing":"match"}`, UrlPatterns: `["route-check=1"]`,
					ChannelGroups: fmt.Sprintf("[[%d]]", tc.globalChannel), ApplyMode: tc.globalMode, RandomType: "order", Enabled: 1,
				}).Error)
			}
			if tc.bound != 0 {
				seed, _ := gin.CreateTestContext(httptest.NewRecorder())
				seed.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				seed.Request.Header.Set("X-Session", t.Name())
				_, found := service.GetPreferredChannelByAffinity(seed, "route-model", "default")
				require.False(t, found)
				service.RecordChannelAffinity(seed, tc.bound)
				t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(seed) })
			}
			mode, pattern := tc.keyMode, tc.keyPattern
			if mode == "" {
				mode = "order"
			}
			if pattern == "" {
				pattern = "route-model"
			}
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				if tc.keyChannels != nil {
					c.Set("token_channel_rules", map[string]dto.ChannelRulesItem{pattern: {Channels: tc.keyChannels, RandomType: mode, Retry: tc.keyRetry, RaceTimeout: 17}})
				}
				c.Next()
			}, Distribute())
			called := false
			engine.POST("/v1/responses", func(c *gin.Context) {
				called = true
				selected := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
				plan := service.GetChannelRetryPlan(c)
				assert.Equal(t, tc.wantRetries, plan.MaxRetries)
				if mode == "random" {
					assert.Contains(t, [][]int{{1, 2, 3}, {3, 1, 2}}, plan.ChannelIds)
					assert.Equal(t, plan.ChannelIds[0], selected)
				} else {
					assert.Equal(t, tc.wantCandidates, plan.ChannelIds)
					assert.Equal(t, tc.wantSelected, selected)
				}
				if mode == "race" {
					raw, exists := c.Get(common.KeyChannelRacePlan)
					require.True(t, exists)
					race, ok := raw.(dto.ChannelRacePlan)
					require.True(t, ok)
					assert.Equal(t, 17, race.TimeoutSeconds)
					require.Len(t, race.Groups, 2)
					assert.Equal(t, []int{1, 2}, race.Groups[0].ChannelIds)
					assert.Equal(t, []int{3}, race.Groups[1].ChannelIds)
				}
				if len(plan.ChannelIds) > 0 {
					assert.False(t, service.ShouldSkipRetryAfterChannelAffinityFailure(c))
					for attempt := range plan.MaxRetries + 1 {
						channel, _, selectErr := service.SelectChannelForRequest(c, "route-model", &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: "route-model", ChannelIds: plan.ChannelIds, Retry: common.GetPointer(attempt)})
						require.Nil(t, selectErr)
						require.NotNil(t, channel)
						assert.Equal(t, plan.ChannelIds[attempt], channel.Id)
					}
					_, _, selectErr := service.SelectChannelForRequest(c, "route-model", &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: "route-model", ChannelIds: plan.ChannelIds, Retry: common.GetPointer(len(plan.ChannelIds))})
					assert.NotNil(t, selectErr, "an exhausted plan must not fall through to defaults")
				}
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/responses?route-check=1", strings.NewReader(`{"model":"route-model","input":"hello","routing":"match"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Session", t.Name())
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
			assert.True(t, called)
		})
	}
}

func TestEnforcedModelRouteFailsClosedWhenNoConfiguredChannelIsAvailable(t *testing.T) {
	require.NoError(t, appI18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelRouteConfig{}, &model.Channel{}))

	originalDB := model.DB
	originalMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCache
	})

	route := &model.ModelRouteConfig{
		Name:          "adaptive only",
		ModelPatterns: `["^claude"]`,
		BodyMatch:     `{"thinking":{"type":"adaptive"}}`,
		ChannelGroups: `[[999]]`,
		RandomType:    "order",
		MaxRetry:      2,
		Priority:      100,
		ApplyMode:     model.ModelRouteApplyModeEnforce,
		Enabled:       1,
	}
	require.NoError(t, db.Create(route).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	called := false
	engine.Use(Distribute())
	engine.POST("/v1/messages", func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet","thinking":{"type":"adaptive"}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	assert.False(t, called)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "model_route_no_available_channel")
}

func TestFilterExplicitChannelCandidatesRequiresRequestedModelWhenEnabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	originalDB := model.DB
	originalMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCache
	})

	channels := []model.Channel{
		{Id: 1, Name: "supports-request", Models: "deepseek-chat,gpt-4o", Status: common.ChannelStatusEnabled},
		{Id: 2, Name: "same-keyword-only", Models: "deepseek-reasoner", Status: common.ChannelStatusEnabled},
		{Id: 3, Name: "unrelated", Models: "claude-3-5", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)

	constraints := &dto.ChannelConstraints{}
	assert.Equal(t, []int{1}, filterExplicitChannelCandidates(
		[]int{1, 2, 3}, "deepseek-chat", true, nil, constraints,
	))
	assert.Equal(t, []int{1, 2, 3}, filterExplicitChannelCandidates(
		[]int{1, 2, 3}, "deepseek-chat", false, nil, constraints,
	))
}

func TestGeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules(t *testing.T) {
	require.NoError(t, appI18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ModelRouteConfig{}))
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	previousFetch := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().ApplyIPFilterForDomain = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
		*system_setting.GetFetchSetting() = previousFetch
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	channels := []model.Channel{
		{Id: 1, Name: "ordinary", Models: "gemini-test", Key: "test", Type: constant.ChannelTypeGemini, Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer("https://ordinary.example")},
		{Id: 2, Name: "file-process", Models: "gemini-test", Key: "test", Type: constant.ChannelTypeVertexAi, Status: common.ChannelStatusEnabled},
		{Id: 3, Name: "file-process-backup", Models: "gemini-test", Key: "test", Type: constant.ChannelTypeVertexAi, Status: common.ChannelStatusEnabled},
	}
	channels[1].Setting = common.GetPointer(`{"google_file_bucket":"test-bucket"}`)
	channels[2].Setting = common.GetPointer(`{"google_file_bucket":"test-bucket"}`)
	require.NoError(t, db.Create(&channels).Error)
	for _, tc := range []struct {
		name                      string
		enabled                   bool
		threshold, pin            int
		keyChannel, globalChannel int
		deny                      bool
		reference, emptyRules     bool
		wantStatus, wantChannel   int
	}{
		{name: "pinned channel outside file rule", enabled: true, threshold: 8, pin: 1, reference: true, wantStatus: 400},
		{name: "compatible pinned channel", enabled: true, threshold: 8, pin: 2, reference: true, wantStatus: 204, wantChannel: 2},
		{name: "small PDF uses ordinary pinned route", enabled: true, threshold: 64, pin: 1, wantStatus: 204, wantChannel: 1},
		{name: "base64 above threshold uses ordinary pinned route without file rules", enabled: true, threshold: 8, pin: 1, emptyRules: true, wantStatus: 204, wantChannel: 1},
		{name: "base64 above threshold preserves key priority", enabled: true, threshold: 8, wantStatus: 204, wantChannel: 1},
		{name: "disabled ignores legacy rules", threshold: 8, pin: 1, wantStatus: 204, wantChannel: 1},
		{name: "key priority cannot escape file rule", enabled: true, threshold: 8, reference: true, wantStatus: 204, wantChannel: 2},
		{name: "file group order precedes compatible key preference", enabled: true, threshold: 8, reference: true, keyChannel: 3, wantStatus: 204, wantChannel: 2},
		{name: "file routing precedes incompatible enforce", enabled: true, threshold: 8, reference: true, globalChannel: 1, wantStatus: 204, wantChannel: 2},
		{name: "file group order precedes compatible enforce", enabled: true, threshold: 8, reference: true, globalChannel: 3, wantStatus: 204, wantChannel: 2},
		{name: "file routing precedes unavailable enforce", enabled: true, threshold: 8, reference: true, globalChannel: 999, wantStatus: 204, wantChannel: 2},
		{name: "missing file candidates cannot fall back to key", enabled: true, threshold: 8, reference: true, emptyRules: true, wantStatus: 503},
		{name: "pinned model access checked before probing", enabled: true, threshold: 8, pin: 2, deny: true, wantStatus: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("id > ?", 0).Delete(&model.ModelRouteConfig{}).Error)
			if tc.globalChannel > 0 {
				require.NoError(t, db.Create(&model.ModelRouteConfig{Name: tc.name, ModelPatterns: `["gemini-test"]`, ApplyMode: model.ModelRouteApplyModeEnforce, Enabled: 1, ChannelGroups: fmt.Sprintf("[[%d]]", tc.globalChannel)}).Error)
			}
			common.OptionMapRWMutex.Lock()
			common.OptionMap["GeminiFileRouting"] = fmt.Sprintf(`{"enabled":%t,"mode":"process","chat_model_patterns":["gemini-test"],"inline_threshold_bytes":%d}`, tc.enabled, tc.threshold)
			common.OptionMap["LargeFileChannels"] = `[{"model_name":"gemini-test","channel_ids":[[2],[3]],"random_type":"order"}]`
			if tc.emptyRules {
				common.OptionMap["LargeFileChannels"] = `[]`
			}
			for _, key := range []string{"VideoChannels", "OnlyTextChannels", "OnlyImageChannels", "NoVideoChannels"} {
				common.OptionMap[key] = "invalid legacy JSON"
			}
			common.OptionMapRWMutex.Unlock()
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				if tc.pin > 0 {
					service.GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: tc.pin, Source: dto.PinSourceToken})
				}
				if tc.deny {
					common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"other": true})
				}
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenChannelRulesHighPriority, true)
				keyChannel := tc.keyChannel
				if keyChannel == 0 {
					keyChannel = 1
				}
				c.Set("token_channel_rules", map[string]dto.ChannelRulesItem{"gemini-test": {RandomType: "order", Channels: []dto.ChannelItem{{Id: keyChannel}}}})
				c.Next()
			}, Distribute())
			selected := 0
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				selected = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
				if tc.enabled && tc.reference && tc.pin == 0 {
					plan := service.GetChannelRetryPlan(c)
					assert.Equal(t, []int{2, 3}, plan.ChannelIds)
					assert.Equal(t, 1, plan.MaxRetries)
				}
				c.Status(204)
			})
			body := `{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"file","file":{"file_data":"data:application/pdf;base64,JVBERi0xLjcK"}}]}]}`
			if tc.reference {
				body = `{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"pdf_url","pdf_url":{"url":"gs://test-bucket/document"}}]}]}`
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			assert.Equal(t, tc.wantChannel, selected)
		})
	}
}
