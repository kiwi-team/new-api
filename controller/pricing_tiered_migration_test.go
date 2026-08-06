package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyTiersToBillingExprPreservesTierSelection(t *testing.T) {
	expr, err := legacyTiersToBillingExpr("migration-test-model", []ratio_setting.PriceTier{
		{MaxTokens: 200_000, InputPrice: 3, OutputPrice: 15, CachedInputPrice: 0.3, CacheWritePrice: 3.75},
		{MaxTokens: 1_000_000, InputPrice: 6, OutputPrice: 22.5, CachedInputPrice: 0.6, CacheWritePrice: 7.5},
	})
	require.NoError(t, err)

	low, lowTrace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{P: 10, C: 2, CR: 5, Len: 100})
	require.NoError(t, err)
	high, highTrace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{P: 10, C: 2, CR: 5, Len: 200_001})
	require.NoError(t, err)

	assert.Equal(t, float64(10*3+2*15+5*0.3), low)
	assert.Equal(t, float64(10*6+2*22.5+5*0.6), high)
	assert.Equal(t, "legacy_1", lowTrace.MatchedTier)
	assert.Equal(t, "legacy_2", highTrace.MatchedTier)
}
