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
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShouldSpecialChannelsOverrideKeyRules(t *testing.T) {
	tests := []struct {
		name                 string
		specialChannelIds    []int
		keyChannelIds        []int
		keyRulesHighPriority bool
		want                 bool
	}{
		{
			name:              "special rules keep their existing default priority",
			specialChannelIds: []int{10},
			keyChannelIds:     []int{20},
			want:              true,
		},
		{
			name:                 "key rules override special rules when enabled",
			specialChannelIds:    []int{10},
			keyChannelIds:        []int{20},
			keyRulesHighPriority: true,
			want:                 false,
		},
		{
			name:                 "special rules remain available when key rules yield no channels",
			specialChannelIds:    []int{10},
			keyRulesHighPriority: true,
			want:                 true,
		},
		{
			name:          "no special rules means no override",
			keyChannelIds: []int{20},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldSpecialChannelsOverrideKeyRules(
				tt.specialChannelIds,
				tt.keyChannelIds,
				tt.keyRulesHighPriority,
			))
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
	}
	channels[1].Setting = common.GetPointer(`{"google_file_bucket":"test-bucket"}`)
	require.NoError(t, db.Create(&channels).Error)
	for _, tc := range []struct {
		name                    string
		enabled                 bool
		threshold, pin          int
		deny                    bool
		reference, emptyRules   bool
		wantStatus, wantChannel int
	}{
		{name: "pinned channel outside file rule", enabled: true, threshold: 8, pin: 1, reference: true, wantStatus: 400},
		{name: "compatible pinned channel", enabled: true, threshold: 8, pin: 2, reference: true, wantStatus: 204, wantChannel: 2},
		{name: "small PDF uses ordinary pinned route", enabled: true, threshold: 64, pin: 1, wantStatus: 204, wantChannel: 1},
		{name: "base64 above threshold uses ordinary pinned route without file rules", enabled: true, threshold: 8, pin: 1, emptyRules: true, wantStatus: 204, wantChannel: 1},
		{name: "base64 above threshold preserves key priority", enabled: true, threshold: 8, wantStatus: 204, wantChannel: 1},
		{name: "disabled ignores legacy rules", threshold: 8, pin: 1, wantStatus: 204, wantChannel: 1},
		{name: "key priority cannot escape file rule", enabled: true, threshold: 8, reference: true, wantStatus: 204, wantChannel: 2},
		{name: "pinned model access checked before probing", enabled: true, threshold: 8, pin: 2, deny: true, wantStatus: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.OptionMapRWMutex.Lock()
			common.OptionMap["GeminiFileRouting"] = fmt.Sprintf(`{"enabled":%t,"mode":"process","chat_model_patterns":["gemini-test"],"inline_threshold_bytes":%d}`, tc.enabled, tc.threshold)
			common.OptionMap["LargeFileChannels"] = `[{"model_name":"gemini-test","channel_ids":[[2]],"random_type":"order"}]`
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
				c.Set("token_channel_rules", map[string]dto.ChannelRulesItem{"gemini-test": {RandomType: "order", Channels: []dto.ChannelItem{{Ids: []int{1}}}}})
				c.Next()
			}, Distribute())
			selected := 0
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				selected = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
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
