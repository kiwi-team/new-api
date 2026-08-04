package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAdjustmentChangesReportsWithoutMutatingHourlySource(t *testing.T) {
	truncateTables(t)
	chinaTime := time.FixedZone("UTC+8", 8*60*60)
	hourStart := time.Date(2026, 8, 1, 3, 0, 0, 0, chinaTime).Unix()
	require.NoError(t, DB.Create(&QuotaData{
		UserID:                      7,
		TokenId:                     11,
		TokenName:                   "billing-key",
		ModelName:                   "claude-test",
		CreatedAt:                   hourStart + 60,
		PromptTokens:                1000,
		CompletionTokens:            200,
		CachedTokens:                300,
		ClaudeCacheCreation5mTokens: 400,
		ClaudeCacheCreation1hTokens: 500,
		Quota:                       500000,
		Count:                       2,
	}).Error)

	before, err := GetUsageHourSnapshot(hourStart, 11, "claude-test")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), before.Effective.PromptTokens)
	assert.Equal(t, 1.0, before.Effective.CostUsd)

	adjustment, err := CreateUsageAdjustment(
		hourStart,
		11,
		"claude-test",
		UsageCorrectionTarget{
			PromptTokens:                800,
			CompletionTokens:            180,
			CachedTokens:                250,
			ClaudeCacheCreation5mTokens: 350,
			ClaudeCacheCreation1hTokens: 450,
			CostUsd:                     0.8,
		},
		"incorrect upstream price",
		"BILL-42",
		100,
		"root",
	)
	require.NoError(t, err)
	assert.Equal(t, int64(-200), adjustment.PromptTokensDelta)
	assert.Equal(t, int64(-100000), adjustment.QuotaDelta)

	rows, err := GetModelUsageAnalysis(7, hourStart-3*3600, hourStart+21*3600-1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(800), rows[0].InputTokens)
	assert.Equal(t, int64(180), rows[0].OutputTokens)
	assert.Equal(t, int64(250), rows[0].CacheReadTokens)
	assert.Equal(t, int64(800), rows[0].CacheWriteTokens)
	assert.InDelta(t, 0.8, rows[0].CostUsd, 0.000001)

	var source QuotaData
	require.NoError(t, DB.First(&source).Error)
	assert.Equal(t, 1000, source.PromptTokens)
	assert.Equal(t, 500000, source.Quota)

	require.NoError(t, RevertUsageAdjustment(adjustment.Id, 100, "root", "correction was entered by mistake"))
	rows, err = GetModelUsageAnalysis(7, hourStart-3*3600, hourStart+21*3600-1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1000), rows[0].InputTokens)
	assert.Equal(t, 1.0, rows[0].CostUsd)
}

func TestUsageAdjustmentRejectsMissingSourceAndNegativeCorrection(t *testing.T) {
	truncateTables(t)
	hourStart := int64(1785510000)
	_, err := GetUsageHourSnapshot(hourStart, 99, "missing")
	assert.ErrorIs(t, err, ErrUsageHourNotFound)

	require.NoError(t, DB.Create(&QuotaData{
		UserID:    7,
		TokenId:   11,
		TokenName: "key",
		ModelName: "model",
		CreatedAt: hourStart + 1,
	}).Error)
	_, err = CreateUsageAdjustment(
		hourStart,
		11,
		"model",
		UsageCorrectionTarget{PromptTokens: -1},
		"invalid correction",
		"",
		100,
		"root",
	)
	assert.ErrorContains(t, err, "non-negative")
}

// 修正弹窗只把 ActiveHours 里的小时列给管理员，所以这个列表必须精确等于当天真正
// 落有 quota_data 的小时；多列一个小时用户点进去就会撞上 ErrUsageHourNotFound。
func TestModelUsageAnalysisReportsOnlyCorrectableHours(t *testing.T) {
	truncateTables(t)
	chinaTime := time.FixedZone("UTC+8", 8*60*60)
	dayStart := time.Date(2026, 8, 4, 0, 0, 0, 0, chinaTime)

	for _, hour := range []int{3, 11, 11} {
		require.NoError(t, DB.Create(&QuotaData{
			UserID:       7,
			TokenId:      11,
			TokenName:    "billing-key",
			ModelName:    "claude-test",
			CreatedAt:    dayStart.Add(time.Duration(hour)*time.Hour).Unix() + 60,
			PromptTokens: 100,
			Quota:        1000,
			Count:        1,
		}).Error)
	}

	rows, err := GetModelUsageAnalysis(7, dayStart.Unix(), dayStart.Add(24*time.Hour).Unix()-1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "2026-08-04", rows[0].Date)
	assert.Equal(t, []int{3, 11}, rows[0].ActiveHours)

	for _, hour := range rows[0].ActiveHours {
		_, err := GetUsageHourSnapshot(dayStart.Add(time.Duration(hour)*time.Hour).Unix(), 11, "claude-test")
		assert.NoError(t, err, "hour %d is offered by the dialog so it must be correctable", hour)
	}
	_, err = GetUsageHourSnapshot(dayStart.Unix(), 11, "claude-test")
	assert.ErrorIs(t, err, ErrUsageHourNotFound)
}

func TestModelUsageAnalysisCanFilterAndIdentifyUsers(t *testing.T) {
	truncateTables(t)
	chinaTime := time.FixedZone("UTC+8", 8*60*60)
	hourStart := time.Date(2026, 8, 4, 9, 0, 0, 0, chinaTime).Unix()
	for _, usage := range []QuotaData{
		{UserID: 7, Username: "alice", TokenId: 11, TokenName: "alice-key", ModelName: "model", CreatedAt: hourStart + 1, PromptTokens: 100},
		{UserID: 8, Username: "bob", TokenId: 12, TokenName: "bob-key", ModelName: "model", CreatedAt: hourStart + 2, PromptTokens: 200},
	} {
		record := usage
		require.NoError(t, DB.Create(&record).Error)
	}

	allRows, err := GetModelUsageAnalysis(0, hourStart, hourStart+3599)
	require.NoError(t, err)
	require.Len(t, allRows, 2)
	assert.Equal(t, []int{7, 8}, []int{allRows[0].UserId, allRows[1].UserId})
	assert.Equal(t, []string{"alice", "bob"}, []string{allRows[0].Username, allRows[1].Username})

	bobRows, err := GetModelUsageAnalysis(8, hourStart, hourStart+3599)
	require.NoError(t, err)
	require.Len(t, bobRows, 1)
	assert.Equal(t, 8, bobRows[0].UserId)
	assert.Equal(t, int64(200), bobRows[0].InputTokens)
}
