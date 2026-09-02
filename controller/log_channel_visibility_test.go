/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSanitizeLogChannelInfoForRoleRemovesRetryChainBelowRoot(t *testing.T) {
	logs := []*model.Log{{
		ChannelId:   1577,
		ChannelName: "primary-deepseek",
		Other:       `{"admin_info":{"use_channel":[1578,1577],"use_channel_time":[343],"quota_saturation":{"kind":"overflow"}},"group_ratio":1}`,
	}}

	sanitizeLogChannelInfoForRole(logs, common.RoleAdminUser)

	require.Zero(t, logs[0].ChannelId)
	require.Empty(t, logs[0].ChannelName)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, adminInfo, "use_channel")
	require.NotContains(t, adminInfo, "use_channel_time")
	require.Contains(t, adminInfo, "quota_saturation")
	require.Equal(t, float64(1), other["group_ratio"])
}

func TestSanitizeLogChannelInfoForRolePreservesRetryChainForRoot(t *testing.T) {
	const otherJSON = `{"admin_info":{"use_channel":[1578,1577],"use_channel_time":[343]}}`
	logs := []*model.Log{{
		ChannelId:   1577,
		ChannelName: "primary-deepseek",
		Other:       otherJSON,
	}}

	sanitizeLogChannelInfoForRole(logs, common.RoleRootUser)

	require.Equal(t, 1577, logs[0].ChannelId)
	require.Equal(t, "primary-deepseek", logs[0].ChannelName)
	require.JSONEq(t, otherJSON, logs[0].Other)
}
