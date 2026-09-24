package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelRouteConfigMatchesEveryConfiguredRequestDimension(t *testing.T) {
	config := &ModelRouteConfig{
		ModelPatterns: `["^gpt-4"]`,
		BodyPatterns:  `["premium"]`,
		UrlPatterns:   `["/v1/responses"]`,
	}

	assert.True(t, config.MatchesRequest("gpt-4.1", `{"tier":"premium"}`, "/v1/responses?trace=1"))
	assert.False(t, config.MatchesRequest("claude-3", `{"tier":"premium"}`, "/v1/responses"))
	assert.False(t, config.MatchesRequest("gpt-4.1", `{"tier":"standard"}`, "/v1/responses"))
	assert.False(t, config.MatchesRequest("gpt-4.1", `{"tier":"premium"}`, "/v1/chat/completions"))
}

func TestModelRouteConfigTreatsEmptyBodyAndURLRulesAsWildcards(t *testing.T) {
	config := &ModelRouteConfig{ModelPatterns: `["^gpt-4"]`}

	assert.True(t, config.MatchesRequest("gpt-4.1", "", "/v1/chat/completions"))
}

func TestModelRouteConfigStructuredBodySubset(t *testing.T) {
	tests := []struct {
		name      string
		bodyMatch string
		body      string
		want      bool
	}{
		{
			name:      "nested object ignores formatting order and extra fields",
			bodyMatch: `{"thinking":{"type":"adaptive"}}`,
			body:      `{"thinking": {"budget_tokens":1024, "type":"adaptive"}, "model":"claude-sonnet"}`,
			want:      true,
		},
		{
			name:      "different nested value does not match",
			bodyMatch: `{"thinking":{"type":"disabled"}}`,
			body:      `{"thinking":{"type":"adaptive"}}`,
		},
		{
			name:      "missing and null remain distinct",
			bodyMatch: `{"thinking":null}`,
			body:      `{}`,
		},
		{
			name:      "numeric representations compare by value",
			bodyMatch: `{"temperature":1.0,"large":9007199254740993}`,
			body:      `{"large":9007199254740993,"temperature":1}`,
			want:      true,
		},
		{
			name:      "arrays require the same order",
			bodyMatch: `{"tools":["search","code"]}`,
			body:      `{"tools":["code","search"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ModelRouteConfig{BodyMatch: tt.bodyMatch}
			assert.Equal(t, tt.want, config.MatchBodyJSON(tt.body))
		})
	}
}

func TestValidateModelRouteBodyMatch(t *testing.T) {
	require.NoError(t, ValidateModelRouteBodyMatch(`{"thinking":{"type":"adaptive"}}`))
	assert.Error(t, ValidateModelRouteBodyMatch(`{}`))
	assert.Error(t, ValidateModelRouteBodyMatch(`[]`))
	assert.Error(t, ValidateModelRouteBodyMatch(`{"thinking":`))
}

func TestModelRouteConfigPriorityAndApplyMode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelRouteConfig{}))

	originalDB := DB
	DB = db
	t.Cleanup(func() {
		DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	configs := []ModelRouteConfig{
		{
			Name:          "lower adaptive",
			ModelPatterns: `["^claude"]`,
			BodyMatch:     `{"thinking":{"type":"adaptive"}}`,
			ChannelGroups: `[[11]]`,
			RandomType:    "order",
			Priority:      10,
			ApplyMode:     ModelRouteApplyModeEnforce,
			Enabled:       1,
		},
		{
			Name:          "higher adaptive",
			ModelPatterns: `["^claude"]`,
			BodyMatch:     `{"thinking":{"type":"adaptive"}}`,
			ChannelGroups: `[[21]]`,
			RandomType:    "order",
			Priority:      20,
			ApplyMode:     ModelRouteApplyModeEnforce,
			Enabled:       1,
		},
		{
			Name:          "legacy fallback",
			ModelPatterns: `["^claude"]`,
			ChannelGroups: `[[31]]`,
			RandomType:    "order",
			Priority:      100,
			Enabled:       1,
		},
	}
	require.NoError(t, db.Create(&configs).Error)

	body := `{"model":"claude-sonnet","thinking":{"type":"adaptive"}}`
	enforced, err := GetChannelRouteByModel("claude-sonnet", body, "/v1/messages", ModelRouteApplyModeEnforce)
	require.NoError(t, err)
	require.True(t, enforced.Matched)
	assert.Equal(t, configs[1].Id, enforced.ConfigID)
	assert.Equal(t, []int{21}, enforced.ChannelIDs)

	fallbackIds, _, err := GetChannelIdsByModel("claude-sonnet", body, "/v1/messages")
	require.NoError(t, err)
	assert.Equal(t, []int{31}, fallbackIds)
}

func TestModelRouteConfigUpdatePreservesCreatedTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelRouteConfig{}))

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	config := &ModelRouteConfig{
		Name:          "original",
		ModelPatterns: `["^claude"]`,
		ChannelGroups: `[[1]]`,
		RandomType:    "order",
		Enabled:       1,
	}
	require.NoError(t, config.Insert())
	createdTime := config.CreatedTime

	updated := &ModelRouteConfig{
		Id:            config.Id,
		Name:          "updated",
		ModelPatterns: config.ModelPatterns,
		ChannelGroups: config.ChannelGroups,
		RandomType:    config.RandomType,
		Enabled:       config.Enabled,
	}
	require.NoError(t, updated.Update())
	require.NoError(t, updated.Update(), "an unchanged update must not be mistaken for a missing row")

	saved, err := GetModelRouteConfigById(config.Id)
	require.NoError(t, err)
	assert.Equal(t, createdTime, saved.CreatedTime)
	assert.Equal(t, "updated", saved.Name)
	assert.Equal(t, ModelRouteApplyModeFallback, saved.GetApplyMode())

	encoded, err := common.Marshal(saved)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"body_match":null`)
	assert.Contains(t, string(encoded), `"apply_mode":"fallback"`)
}
