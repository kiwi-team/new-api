package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

// 退款日志(type=6)与消费日志同表同列，正值 quota 会被任何 SUM(quota) 当成又一笔
// 消费，导致退款反而推高统计。落库必须是负值，聚合才能自然抵消。
func TestCreateLogNormalizesRefundQuotaSign(t *testing.T) {
	cases := []struct {
		name      string
		logType   int
		quota     int
		wantQuota int
	}{
		{name: "refund is stored negative", logType: LogTypeRefund, quota: 500, wantQuota: -500},
		// 已经是负值说明调用方自己取过反，再取一次会把退款翻回正数。
		{name: "already negative refund is left alone", logType: LogTypeRefund, quota: -500, wantQuota: -500},
		{name: "zero refund stays zero", logType: LogTypeRefund, quota: 0, wantQuota: 0},
		{name: "consume keeps positive quota", logType: LogTypeConsume, quota: 500, wantQuota: 500},
		{name: "topup keeps positive quota", logType: LogTypeTopup, quota: 500, wantQuota: 500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, LOG_DB.Where("1 = 1").Delete(&Log{}).Error)

			require.NoError(t, createLog(&Log{
				UserId:    1,
				Type:      tc.logType,
				Quota:     tc.quota,
				CreatedAt: common.GetTimestamp(),
			}))

			var stored Log
			require.NoError(t, LOG_DB.Order("id desc").First(&stored).Error)
			assert.Equal(t, tc.wantQuota, stored.Quota)
		})
	}
}

// SUM(quota) 是消费统计、项目预算、UID 预警共用的口径；退款存负值的意义就在于
// 让「消费 + 退款」的净额直接等于聚合结果。
func TestRefundQuotaOffsetsConsumeInAggregate(t *testing.T) {
	require.NoError(t, LOG_DB.Where("1 = 1").Delete(&Log{}).Error)

	now := common.GetTimestamp()
	require.NoError(t, createLog(&Log{UserId: 7, Type: LogTypeConsume, Quota: 1200, CreatedAt: now}))
	require.NoError(t, createLog(&Log{UserId: 7, Type: LogTypeRefund, Quota: 500, CreatedAt: now}))

	var net int64
	require.NoError(t, LOG_DB.Model(&Log{}).
		Where("user_id = ?", 7).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&net).Error)
	assert.EqualValues(t, 700, net)
}
