package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUIDBudgetAlertListsAllPoolsWhenProjectBudgetTriggers(t *testing.T) {
	previousMonthState := uidMonthThresholdMap
	previousProjectState := uidProjectThresholdMap
	uidMonthThresholdMap = make(map[string]map[string]int64)
	uidProjectThresholdMap = make(map[string]map[string]int64)
	t.Cleanup(func() {
		uidMonthThresholdMap = previousMonthState
		uidProjectThresholdMap = previousProjectState
	})

	pools := []uidBudgetAlertPool{
		{
			Name:      "非项目预算（本月）",
			StateKey:  "uid-1",
			PeriodKey: "month_start",
			PeriodID:  100,
			TotalUSD:  2000,
			UsedUSD:   500,
		},
		{
			Name:      "项目「alpha」/ 方案「2026-Q3」（20260701-20260930）",
			StateKey:  "uid-1|1",
			PeriodKey: "plan_id",
			PeriodID:  10,
			TotalUSD:  6000,
			UsedUSD:   5100,
			IsProject: true,
		},
		{
			Name:      "项目「beta」/ 方案「2026-H2」（20260701-20261231）",
			StateKey:  "uid-1|2",
			PeriodKey: "plan_id",
			PeriodID:  20,
			TotalUSD:  1000,
			UsedUSD:   100,
			IsProject: true,
		},
	}

	content, markers := buildUIDBudgetAlert("uid-1", pools)
	require.Len(t, markers, 2, "15% remaining crosses the 50% and 20% thresholds in one notification")
	assert.Contains(t, content, "UID预算预警：UID=uid-1")
	assert.Contains(t, content, "项目「alpha」/ 方案「2026-Q3」")
	assert.Contains(t, content, "达到 <=20%")
	assert.Contains(t, content, "非项目预算（本月）：已用 $500.00 / $2000.00")
	assert.Contains(t, content, "项目「beta」/ 方案「2026-H2」")

	nonProjectDirty, projectDirty := markUIDBudgetAlerted(markers)
	assert.False(t, nonProjectDirty)
	assert.True(t, projectDirty)

	content, markers = buildUIDBudgetAlert("uid-1", pools)
	assert.Empty(t, content)
	assert.Empty(t, markers, "all crossed thresholds were recorded after the first notification")
}

func TestBuildUIDBudgetAlertResetsDeduplicationForNewMonthAndPlan(t *testing.T) {
	previousMonthState := uidMonthThresholdMap
	previousProjectState := uidProjectThresholdMap
	uidMonthThresholdMap = map[string]map[string]int64{
		"uid-2": {
			"month_start": 100,
			"50":          100,
		},
	}
	uidProjectThresholdMap = map[string]map[string]int64{
		"uid-2|7": {
			"plan_id": 30,
			"50":      30,
		},
	}
	t.Cleanup(func() {
		uidMonthThresholdMap = previousMonthState
		uidProjectThresholdMap = previousProjectState
	})

	pools := []uidBudgetAlertPool{
		{
			Name:      "非项目预算（本月）",
			StateKey:  "uid-2",
			PeriodKey: "month_start",
			PeriodID:  200,
			TotalUSD:  100,
			UsedUSD:   60,
		},
		{
			Name:      "项目「alpha」/ 方案「new-plan」（20261001-20261231）",
			StateKey:  "uid-2|7",
			PeriodKey: "plan_id",
			PeriodID:  31,
			TotalUSD:  100,
			UsedUSD:   60,
			IsProject: true,
		},
	}

	content, markers := buildUIDBudgetAlert("uid-2", pools)
	require.Len(t, markers, 2)
	assert.Contains(t, content, "非项目预算（本月） 剩余 40.0%")
	assert.Contains(t, content, "方案「new-plan」")
}

func TestPendingPlatformQuotaMilestone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		deltaQuota       int64
		accumulatedQuota int64
		alertedUSD       int64
		initialized      bool
		expected         int64
	}{
		{name: "uninitialized state does not alert on historical usage", deltaQuota: 20_000, expected: 0},
		{name: "below first threshold", initialized: true, deltaQuota: 999, expected: 0},
		{name: "first threshold", initialized: true, deltaQuota: 1_000, expected: 1_000},
		{name: "does not repeat current milestone", initialized: true, deltaQuota: 1_999, alertedUSD: 1_000, expected: 0},
		{name: "next thousand milestone", initialized: true, deltaQuota: 2_000, alertedUSD: 1_000, expected: 2_000},
		{name: "compacted usage contributes to threshold", initialized: true, deltaQuota: 100, accumulatedQuota: 1_900, alertedUSD: 1_000, expected: 2_000},
		{name: "highest crossed milestone only", initialized: true, deltaQuota: 12_500, expected: 12_000},
		{name: "does not repeat highest milestone", initialized: true, deltaQuota: 12_999, alertedUSD: 12_000, expected: 0},
		{name: "repeats at next thousand", initialized: true, deltaQuota: 13_000, alertedUSD: 12_000, expected: 13_000},
		{name: "baseline is excluded", initialized: true, deltaQuota: 21_000, alertedUSD: 20_000, expected: 21_000},
		{name: "refund below baseline does not alert", initialized: true, deltaQuota: -500, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := platformQuotaMilestoneState{
				Initialized:      tt.initialized,
				WindowQuota:      1_000,
				AccumulatedQuota: tt.accumulatedQuota,
				MilestoneUSD:     tt.alertedUSD,
			}
			actual := pendingPlatformQuotaMilestone(tt.deltaQuota+1_000, 1, state)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestPendingPlatformQuotaMilestoneRejectsInvalidQuotaPerUnit(t *testing.T) {
	t.Parallel()

	state := platformQuotaMilestoneState{Initialized: true}
	assert.Zero(t, pendingPlatformQuotaMilestone(10_000, 0, state))
}

func TestPendingPlatformQuotaMilestoneUsesConfiguredQuotaPerUnit(t *testing.T) {
	t.Parallel()

	state := platformQuotaMilestoneState{Initialized: true}
	assert.Zero(t, pendingPlatformQuotaMilestone(499_999_999, 500_000, state))
	assert.Equal(t, int64(1_000), pendingPlatformQuotaMilestone(500_000_000, 500_000, state))
}
