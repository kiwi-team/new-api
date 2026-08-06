package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBillingSettingWildcardUsesLongestPrefix(t *testing.T) {
	previousModes := billingSetting.BillingMode
	previousExprs := billingSetting.BillingExpr
	t.Cleanup(func() {
		billingSetting.BillingMode = previousModes
		billingSetting.BillingExpr = previousExprs
	})
	billingSetting.BillingMode = map[string]string{
		"model-*":     BillingModeTieredExpr,
		"model-pro-*": BillingModeTieredExpr,
	}
	billingSetting.BillingExpr = map[string]string{
		"model-*":     "p * 1",
		"model-pro-*": "p * 2",
	}

	assert.Equal(t, BillingModeTieredExpr, GetBillingMode("model-pro-latest"))
	expr, ok := GetBillingExpr("model-pro-latest")
	assert.True(t, ok)
	assert.Equal(t, "p * 2", expr)
	assert.Equal(t, BillingModeRatio, GetBillingMode("other"))
}
