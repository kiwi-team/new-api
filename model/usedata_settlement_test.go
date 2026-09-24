package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

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

// 结算调整落在提交记录之后到达（跨小时、或缓存已刷盘）时，走的是 increaseQuotaData
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
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
}
