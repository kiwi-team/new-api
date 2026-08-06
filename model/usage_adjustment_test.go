package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAdjustmentCorrectsWholeDayWithoutMutatingSource(t *testing.T) {
	truncateTables(t)
	chinaTime := time.FixedZone("UTC+8", 8*60*60)
	date := "2026-08-01"
	dayStart := time.Date(2026, 8, 1, 0, 0, 0, 0, chinaTime).Unix()
	require.NoError(t, DB.Create(&QuotaData{
		UserID:                      7,
		TokenId:                     11,
		TokenName:                   "billing-key",
		ModelName:                   "claude-test",
		CreatedAt:                   dayStart + 3*3600 + 60,
		PromptTokens:                1000,
		CompletionTokens:            200,
		CachedTokens:                300,
		ClaudeCacheCreation5mTokens: 400,
		ClaudeCacheCreation1hTokens: 500,
		Quota:                       500000,
		Count:                       2,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:           7,
		TokenId:          11,
		TokenName:        "billing-key",
		ModelName:        "claude-test",
		CreatedAt:        dayStart + 15*3600 + 60,
		PromptTokens:     500,
		CompletionTokens: 100,
		Quota:            100000,
		Count:            1,
	}).Error)

	before, err := GetUsageDaySnapshot(date, 11, "claude-test")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), before.Effective.PromptTokens)
	assert.Equal(t, 1.2, before.Effective.CostUsd)

	adjustment, err := CreateUsageAdjustment(
		date,
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
	assert.Equal(t, int64(-700), adjustment.PromptTokensDelta)
	assert.Equal(t, int64(-200000), adjustment.QuotaDelta)

	rows, err := GetModelUsageAnalysis(7, dayStart, dayStart+24*3600-1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(800), rows[0].InputTokens)
	assert.Equal(t, int64(180), rows[0].OutputTokens)
	assert.Equal(t, int64(250), rows[0].CacheReadTokens)
	assert.Equal(t, int64(800), rows[0].CacheWriteTokens)
	assert.InDelta(t, 0.8, rows[0].CostUsd, 0.000001)

	var sources []QuotaData
	require.NoError(t, DB.Order("created_at").Find(&sources).Error)
	require.Len(t, sources, 2)
	assert.Equal(t, 1000, sources[0].PromptTokens)
	assert.Equal(t, 500, sources[1].PromptTokens)

	require.NoError(t, RevertUsageAdjustment(adjustment.Id, 100, "root", "correction was entered by mistake"))
	rows, err = GetModelUsageAnalysis(7, dayStart, dayStart+24*3600-1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1500), rows[0].InputTokens)
	assert.Equal(t, 1.2, rows[0].CostUsd)
}

func TestUsageAdjustmentRejectsMissingSourceAndNegativeCorrection(t *testing.T) {
	truncateTables(t)
	date := "2026-08-01"
	dayStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)).Unix()
	_, err := GetUsageDaySnapshot(date, 99, "missing")
	assert.ErrorIs(t, err, ErrUsageDayNotFound)

	require.NoError(t, DB.Create(&QuotaData{
		UserID:    7,
		TokenId:   11,
		TokenName: "key",
		ModelName: "model",
		CreatedAt: dayStart + 1,
	}).Error)
	_, err = CreateUsageAdjustment(
		date,
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
