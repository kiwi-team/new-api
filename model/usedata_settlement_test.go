package model

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 消耗统计页(/quota-statistics)与飞书告警都按 quota_data 的 SUM(quota) 聚合。
// 异步任务提交时预扣、完成后补扣或退款，三者必须净额落在同一行上，且请求数只
// 记提交那一次——否则总消耗只增不减，请求数也会被结算过程放大。
func TestQuotaDataSettlementAdjustmentsNetOut(t *testing.T) {
	const (
		userId    = 4101
		username  = "settle-user"
		modelName = "test-video-model"
		uid       = "uid-settle"
	)

	cases := []struct {
		name         string
		preConsumed  int
		adjustType   int
		adjustQuota  int
		wantNetQuota int
	}{
		{
			name:         "task fails and is fully refunded",
			preConsumed:  3000,
			adjustType:   LogTypeRefund,
			adjustQuota:  3000,
			wantNetQuota: 0,
		},
		{
			name:         "settlement charges the difference",
			preConsumed:  3000,
			adjustType:   LogTypeConsume,
			adjustQuota:  1200,
			wantNetQuota: 4200,
		},
		{
			name:         "settlement refunds the difference",
			preConsumed:  3000,
			adjustType:   LogTypeRefund,
			adjustQuota:  800,
			wantNetQuota: 2200,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetQuotaDataState(t)

			createdAt := common.GetTimestamp()
			// 提交：一次真实请求，计一次数。
			LogQuotaData(&LogQuotaDataCache{
				UserId:       userId,
				Username:     username,
				ModelName:    modelName,
				Quota:        tc.preConsumed,
				CreatedAt:    createdAt,
				ClientUserId: uid,
			})
			// 结算：只修正金额，不是新请求。
			LogQuotaData(&LogQuotaDataCache{
				UserId:                 userId,
				Username:               username,
				ModelName:              modelName,
				Quota:                  signedQuotaForLogType(tc.adjustType, tc.adjustQuota),
				CreatedAt:              createdAt,
				ClientUserId:           uid,
				IsSettlementAdjustment: true,
			})
			SaveQuotaDataCache()

			var row QuotaData
			require.NoError(t, DB.Table("quota_data").Where("user_id = ?", userId).First(&row).Error)
			assert.Equal(t, tc.wantNetQuota, row.Quota, "统计页的总消耗应为净额")
			assert.Equal(t, 1, row.Count, "请求数只应记提交那一次")
		})
	}
}

// 结算调整落在提交记录之后到达（跨小时、或缓存已刷盘）时，走的是
// 的 UPDATE 分支还是 Create 分支取决于时序，两条路径都不能把请求数记成 1。
func TestQuotaDataSettlementAdjustmentAloneAddsNoRequest(t *testing.T) {
	const userId = 4102

	resetQuotaDataState(t)

	LogQuotaData(&LogQuotaDataCache{
		UserId:                 userId,
		Username:               "adjust-only",
		ModelName:              "test-video-model",
		Quota:                  -500,
		CreatedAt:              common.GetTimestamp(),
		ClientUserId:           "uid-adjust",
		IsSettlementAdjustment: true,
	})
	SaveQuotaDataCache()

	var row QuotaData
	require.NoError(t, DB.Table("quota_data").Where("user_id = ?", userId).First(&row).Error)
	assert.Equal(t, -500, row.Quota)
	assert.Equal(t, 0, row.Count)
}

func TestGetQuotaDataStatisticsFiltersByChannel(t *testing.T) {
	resetQuotaDataState(t)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:       1,
		ModelName:    "channel-filter-model",
		ClientUserId: "channel-filter-user",
		ChannelId:    11,
		CreatedAt:    1000,
		Count:        1,
		Quota:        100,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:       1,
		ModelName:    "channel-filter-model",
		ClientUserId: "channel-filter-user",
		ChannelId:    22,
		CreatedAt:    1000,
		Count:        2,
		Quota:        200,
	}).Error)

	rows, err := GetQuotaDataStatistics(900, 1100, "", "", "", false, false, false, 0, "", nil, 22, 0, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].TotalCount)
	assert.Equal(t, float64(200)/common.QuotaPerUnit, rows[0].TotalQuota)
}

func resetQuotaDataState(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM quota_data").Error)
	require.NoError(t, DB.Exec("DELETE FROM cliend_user_quota").Error)
	require.NoError(t, DB.Exec("DELETE FROM quota_data_events").Error)
	require.NoError(t, DB.Exec("DELETE FROM quota_data_applied_events").Error)
}

func quotaDataTestEvent() LogQuotaDataCache {
	return LogQuotaDataCache{
		UserId: 1, Username: "quota-test", ModelName: "test-model", CreatedAt: 3661,
		TokenId: 11, TokenName: "test-token", ChannelId: 22, UseGroup: "default", NodeName: "node-a",
		ClientUserId: "uid-a", ClientScenairo: "scenario-a", ProjectName: "project-a", PlanId: 1,
		Quota: 100, TokenUsed: 15, PromptTokens: 10, CompletionTokens: 5, CachedTokens: 4,
		ClaudeCacheCreation5mTokens: 2, ClaudeCacheCreation1hTokens: 3, FirstTokenMs: 100, UseTimeSeconds: 2,
	}
}

func TestRecordConsumeLogReadyForQuotaFlush(t *testing.T) {
	for _, saveLog := range []bool{true, false} {
		t.Run(fmt.Sprintf("consume_log=%t", saveLog), func(t *testing.T) {
			truncateTables(t)
			resetQuotaDataState(t)
			previousExport, previousConsume := common.DataExportEnabled, common.LogConsumeEnabled
			common.DataExportEnabled, common.LogConsumeEnabled = true, saveLog
			t.Cleanup(func() {
				common.DataExportEnabled, common.LogConsumeEnabled = previousExport, previousConsume
			})
			const userID = 11002233
			require.NoError(t, DB.Create(&User{Id: userID, Username: "quota-flush-user", Setting: "{}"}).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set("username", "quota-flush-user")
			RecordConsumeLog(c, userID, RecordConsumeLogParams{
				ModelName: "quota-flush-model", Quota: 100, PromptTokens: 10, CompletionTokens: 5,
				Group: "default", Other: NewLogOther(),
			})
			// Shutdown can flush immediately after a request finishes without waiting on a pool.
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Where("user_id = ?", userID).Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, 100, rows[0].Quota)
			assert.Equal(t, 1, rows[0].Count)
			assert.Equal(t, 15, rows[0].TokenUsed)
			var logCount int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ?", userID).Count(&logCount).Error)
			if saveLog {
				assert.EqualValues(t, 1, logCount)
			} else {
				assert.Zero(t, logCount)
			}
		})
	}
}

func testQuotaDataCounters(t *testing.T) {
	resetQuotaDataState(t)
	event := quotaDataTestEvent()
	for range 2 {
		LogQuotaData(&event)
		require.NoError(t, SaveQuotaDataCache())
	}
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	want := QuotaData{
		Id: row.Id, BucketKey: row.BucketKey, UserID: 1, Username: "quota-test", ModelName: "test-model", CreatedAt: 3600,
		TokenId: 11, TokenName: "test-token", ChannelId: 22, UseGroup: "default", NodeName: "node-a",
		ClientUserId: "uid-a", ClientScenairo: "scenario-a", ProjectName: "project-a", PlanId: 1,
		Quota: 200, Count: 2, TokenUsed: 30, PromptTokens: 20, CompletionTokens: 10, CachedTokens: 8,
		ClaudeCacheCreation5mTokens: 4, ClaudeCacheCreation1hTokens: 6,
		CacheWrite5mRequestCount: 2, CacheWrite1hRequestCount: 2, CacheReadRequestCount: 2,
		StreamRequestCount: 2, FrtSum: 200, RequestTimeSum: 4000,
	}
	assert.Equal(t, want, row)
	event.Quota, event.IsSettlementAdjustment = -20, true
	event.TokenUsed, event.PromptTokens, event.CompletionTokens, event.CachedTokens = 0, 0, 0, 0
	event.ClaudeCacheCreation5mTokens, event.ClaudeCacheCreation1hTokens = 0, 0
	event.FirstTokenMs, event.UseTimeSeconds = 0, 0
	LogQuotaData(&event)
	require.NoError(t, SaveQuotaDataCache())
	require.NoError(t, DB.First(&row).Error)
	want.Quota = 180
	assert.Equal(t, want, row, "refunds change only quota")
	var uid CliendUserQuota
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 180, uid.UsedQuota)
}

func testQuotaDataDimensions(t *testing.T) {
	for _, dimension := range []string{"uid", "scenario", "project", "plan", "hyphens"} {
		t.Run(dimension, func(t *testing.T) {
			resetQuotaDataState(t)
			a, b := quotaDataTestEvent(), quotaDataTestEvent()
			a.TokenName, b.TokenName, b.Quota = "a", "b", 200
			switch dimension {
			case "uid":
				b.ClientUserId = "uid-b"
			case "scenario":
				b.ClientScenairo = "scenario-b"
			case "project":
				b.ProjectName = "project-b"
			case "plan":
				b.PlanId = 2
			case "hyphens":
				a.ClientUserId, a.ClientScenairo = "a-b", "c"
				b.ClientUserId, b.ClientScenairo = "a", "b-c"
			}
			LogQuotaData(&a)
			LogQuotaData(&b)
			require.NoError(t, SaveQuotaDataCache())
			a.Quota = 50
			LogQuotaData(&a)
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Order("token_name").Find(&rows).Error)
			require.Len(t, rows, 2)
			assert.Equal(t, 150, rows[0].Quota)
			assert.Equal(t, 200, rows[1].Quota)
			assert.Equal(t, 2, rows[0].Count)
			assert.Equal(t, 1, rows[1].Count)
			assert.Equal(t, a.ClientUserId, rows[0].ClientUserId)
			assert.Equal(t, b.ClientUserId, rows[1].ClientUserId)
		})
	}
}
