package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
