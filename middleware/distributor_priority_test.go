package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
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
