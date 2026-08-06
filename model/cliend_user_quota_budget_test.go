package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usd 把美元金额换算成数据库里存的原始 quota 单位。
func usd(t *testing.T, amount float64) int {
	t.Helper()
	return int(amount * common.QuotaPerUnit)
}

func insertQuotaDataRow(t *testing.T, clientUserId string, projectName string, createdAt int64, quota int) {
	t.Helper()
	require.NoError(t, DB.Create(&QuotaData{
		ClientUserId: clientUserId,
		ProjectName:  projectName,
		CreatedAt:    createdAt,
		Quota:        quota,
		Count:        1,
	}).Error)
}

func insertClientUserQuota(t *testing.T, row *CliendUserQuota) {
	t.Helper()
	row.UpdatedAt = common.GetTimestamp()
	require.NoError(t, DB.Create(row).Error)
}

// 缓存按 (uid, 计费月) 建 key，同一个 uid 在多个用例间会互相命中。
// 每个用例用独立 uid 隔离，这个函数保证 uid 不重复。
func uniqueClientUserId(t *testing.T) string {
	t.Helper()
	return "uid-" + t.Name()
}

// 本工单的直接回归：项目消耗不得吃掉非项目预算。
// 数字取自线上实际案例——月度固定预算 $2000、本月总消耗 $2004.64（其中项目 $1028.94、
// 非项目 $975.70）。改动前该 uid 的非项目请求已被 403，改动后应放行。
func TestCheckClientUserNonProjectBudgetIgnoresProjectSpend(t *testing.T) {
	truncateTables(t)
	clientUserId := uniqueClientUserId(t)
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())

	insertClientUserQuota(t, &CliendUserQuota{
		ClientUserId: clientUserId,
		FixedQuota:   2000,
		TempQuota:    0,
		UsedQuota:    usd(t, 2004.636724),
	})
	insertQuotaDataRow(t, clientUserId, "some-project", monthStart+60, usd(t, 1028.939196))
	insertQuotaDataRow(t, clientUserId, "", monthStart+120, usd(t, 975.697528))

	allowed, err := CheckClientUserNonProjectBudget(clientUserId)
	require.NoError(t, err)
	assert.True(t, allowed, "非项目消耗 $975.70 未超 $2000 预算，应放行")
}

func TestCheckClientUserNonProjectBudgetDeniesWhenNonProjectSpendExceedsBudget(t *testing.T) {
	truncateTables(t)
	clientUserId := uniqueClientUserId(t)
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())

	insertClientUserQuota(t, &CliendUserQuota{
		ClientUserId: clientUserId,
		FixedQuota:   1000,
	})
	insertQuotaDataRow(t, clientUserId, "", monthStart+60, usd(t, 1000.01))

	allowed, err := CheckClientUserNonProjectBudget(clientUserId)
	require.NoError(t, err)
	require.False(t, allowed)

	// 追加一大笔项目消耗不应改变非项目判定——它走的是项目额度闸门。
	insertQuotaDataRow(t, clientUserId, "some-project", monthStart+120, usd(t, 9999))
	allowed, err = CheckClientUserNonProjectBudget(clientUserId)
	require.NoError(t, err)
	assert.False(t, allowed)
}

// 计费月起点用 UTC+8。月初前 8 小时是最容易因时区判错的窗口，这里直接锁住边界。
func TestCheckClientUserNonProjectBudgetCountsOnlyCurrentBillingMonth(t *testing.T) {
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())

	cases := []struct {
		name        string
		createdAt   int64
		wantAllowed bool
	}{
		{name: "上月最后一秒不计入", createdAt: monthStart - 1, wantAllowed: true},
		{name: "本月第一秒计入", createdAt: monthStart, wantAllowed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			clientUserId := uniqueClientUserId(t)
			insertClientUserQuota(t, &CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   100,
			})
			insertQuotaDataRow(t, clientUserId, "", tc.createdAt, usd(t, 500))

			allowed, err := CheckClientUserNonProjectBudget(clientUserId)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAllowed, allowed)
		})
	}
}

// 临时额度必须在读时判过期：后台任务每分钟才清零，中间的窗口不能继续放行。
func TestCheckClientUserNonProjectBudgetTreatsExpiredTempQuotaAsZero(t *testing.T) {
	now := common.GetTimestamp()
	monthStart := common.BillingMonthStartUnix(now)

	cases := []struct {
		name        string
		expiredAt   int64
		wantAllowed bool
	}{
		{name: "永不过期", expiredAt: 0, wantAllowed: true},
		{name: "尚未过期", expiredAt: now + 3600, wantAllowed: true},
		{name: "已过期", expiredAt: now - 1, wantAllowed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			clientUserId := uniqueClientUserId(t)
			// 固定预算 $100 不足以覆盖 $300 消耗，只有临时额度仍然有效时才放行。
			insertClientUserQuota(t, &CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   100,
				TempQuota:    500,
				ExpiredAt:    tc.expiredAt,
			})
			insertQuotaDataRow(t, clientUserId, "", monthStart+60, usd(t, 300))

			allowed, err := CheckClientUserNonProjectBudget(clientUserId)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAllowed, allowed)
		})
	}
}

// 没有预算记录 = 没有额度，是业务结论而非查询失败；零预算同样必拒。
func TestCheckClientUserNonProjectBudgetDeniesUnconfiguredClientUser(t *testing.T) {
	truncateTables(t)

	allowed, err := CheckClientUserNonProjectBudget("uid-never-configured")
	require.NoError(t, err)
	assert.False(t, allowed)

	zeroBudgetUser := uniqueClientUserId(t)
	insertClientUserQuota(t, &CliendUserQuota{ClientUserId: zeroBudgetUser})
	allowed, err = CheckClientUserNonProjectBudget(zeroBudgetUser)
	require.NoError(t, err)
	assert.False(t, allowed)
}

// project_name 为 NULL 的历史行（手工 ALTER TABLE 加列留下的）同样属于非项目消耗。
func TestGetMonthlyNonProjectQuotaTreatsNullProjectNameAsNonProject(t *testing.T) {
	truncateTables(t)
	clientUserId := uniqueClientUserId(t)
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())

	insertQuotaDataRow(t, clientUserId, "", monthStart+60, usd(t, 10))
	insertQuotaDataRow(t, clientUserId, "some-project", monthStart+120, usd(t, 20))
	insertQuotaDataRow(t, clientUserId, "to-be-nulled", monthStart+180, usd(t, 30))
	require.NoError(t, DB.Exec("UPDATE quota_data SET project_name = NULL WHERE project_name = ?", "to-be-nulled").Error)

	used, err := GetMonthlyNonProjectQuota(clientUserId, monthStart)
	require.NoError(t, err)
	assert.Equal(t, int64(usd(t, 40)), used, "空串与 NULL 都算非项目，$10 + $30")
}

// 重启会重复执行迁移，索引已存在时必须静默通过而不是让进程起不来。
func TestCreateQuotaDataClientUserCreatedAtIndexIsIdempotent(t *testing.T) {
	require.NoError(t, createQuotaDataClientUserCreatedAtIndex())
	require.True(t, DB.Migrator().HasIndex(&QuotaData{}, "idx_quota_data_client_user_created_at"))
	require.NoError(t, createQuotaDataClientUserCreatedAtIndex())
	assert.True(t, DB.Migrator().HasIndex(&QuotaData{}, "idx_quota_data_client_user_created_at"))
}

func TestBillingMonthStartUnixUsesUTC8(t *testing.T) {
	beijing := time.FixedZone("CST", 8*60*60)
	// 2026-03-01 07:59:59 +08:00 仍属于 2 月的 UTC 视角，但计费月已经是 3 月。
	ts := time.Date(2026, 3, 1, 7, 59, 59, 0, beijing).Unix()
	assert.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, beijing).Unix(), common.BillingMonthStartUnix(ts))

	// 2026-02-28 23:59:59 +08:00 属于 2 月。
	ts = time.Date(2026, 2, 28, 23, 59, 59, 0, beijing).Unix()
	assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, beijing).Unix(), common.BillingMonthStartUnix(ts))
}
