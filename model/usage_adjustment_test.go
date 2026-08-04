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
