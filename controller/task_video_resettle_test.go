package controller

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withModelRatio installs a temporary model ratio table and restores the
// original one afterwards, so the test does not leak into other packages.
func withModelRatio(t *testing.T, ratios map[string]float64) {
	t.Helper()
	saved := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(saved))
	})
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))
}

func videoTask(quota int) *model.Task {
	task := &model.Task{
		TaskID: "task_test",
		UserId: 1,
		Group:  "default",
		Quota:  quota,
	}
	task.Properties.OriginModelName = "resettle-test-model"
	return task
}

// total_tokens comes straight from the upstream response. A value large enough
// to overflow the 32-bit quota columns must not be turned into a charge: the
// saturated number is not the real consumption, and applying it would either
// bankrupt the user (huge extra debit) or — if the conversion wrapped negative —
// silently credit them. Either way the pre-consumed quota must stay untouched.
func TestResettleVideoTaskByTokensSkipsOnQuotaSaturation(t *testing.T) {
	withModelRatio(t, map[string]float64{"resettle-test-model": 1000})

	const preConsumed = 500
	task := videoTask(preConsumed)
	resettleVideoTaskByTokens(context.Background(), task, &relaycommon.TaskInfo{
		// 1e18 * 1000 is far beyond MaxQuota (2^31-1)
		TotalTokens: 1_000_000_000_000_000_000,
	})

	assert.Equal(t, preConsumed, task.Quota,
		"a saturated settlement must leave the pre-consumed quota untouched")
}

// Guards the branch that decides debit vs credit: only a genuinely smaller
// actual consumption may refund. The inputs here stay well inside int32.
func TestResettleVideoTaskByTokensNoOpCases(t *testing.T) {
	withModelRatio(t, map[string]float64{"resettle-test-model": 2})

	cases := []struct {
		name        string
		totalTokens int
		model       string
	}{
		{name: "no tokens reported", totalTokens: 0, model: "resettle-test-model"},
		{name: "negative tokens rejected", totalTokens: -100, model: "resettle-test-model"},
		{name: "model without ratio setting", totalTokens: 100, model: "unpriced-model"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := videoTask(500)
			task.Properties.OriginModelName = tc.model
			resettleVideoTaskByTokens(context.Background(), task,
				&relaycommon.TaskInfo{TotalTokens: tc.totalTokens})
			assert.Equal(t, 500, task.Quota)
		})
	}
}

// The saturation policy itself lives in common/quota_math.go; this pins the
// contract the settlement path relies on — an overflowing product reports a
// clamp instead of wrapping into a negative (i.e. a credit).
func TestQuotaFromFloatCheckedReportsOverflowInsteadOfWrapping(t *testing.T) {
	quota, clamp := common.QuotaFromFloatChecked(1e18 * 1000)

	require.NotNil(t, clamp)
	assert.Equal(t, common.QuotaClampOverflow, clamp.Kind)
	assert.Equal(t, common.MaxQuota, quota)
	assert.Positive(t, quota, "a clamped charge must never come out negative")
}
