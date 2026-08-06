package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestLimitUserLogStartTimestamp(t *testing.T) {
	previous := common.UserLogQueryLimitDays
	t.Cleanup(func() { common.UserLogQueryLimitDays = previous })
	common.UserLogQueryLimitDays = 14
	now := int64(2_000_000_000)
	cutoff := now - 14*24*60*60

	assert.Equal(t, cutoff, limitUserLogStartTimestamp(common.RoleCommonUser, 0, now))
	assert.Equal(t, cutoff, limitUserLogStartTimestamp(common.RoleCommonUser, cutoff-1, now))
	assert.Equal(t, cutoff+1, limitUserLogStartTimestamp(common.RoleCommonUser, cutoff+1, now))
	assert.Equal(t, int64(0), limitUserLogStartTimestamp(common.RoleAdminUser, 0, now))
	assert.Equal(t, int64(123), limitUserLogStartTimestamp(common.RoleRootUser, 123, now))
}

func TestLimitUserLogStartTimestampFallsBackToDefault(t *testing.T) {
	previous := common.UserLogQueryLimitDays
	t.Cleanup(func() { common.UserLogQueryLimitDays = previous })
	common.UserLogQueryLimitDays = 0
	now := int64(2_000_000_000)

	assert.Equal(t, now-int64(common.DefaultUserLogQueryLimitDays)*24*60*60,
		limitUserLogStartTimestamp(common.RoleCommonUser, 0, now))
}
