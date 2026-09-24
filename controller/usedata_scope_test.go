package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func usageScopeTestContext(role int, userId int, query string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/data/model-usage-analysis"+query, nil)
	c.Set("role", role)
	c.Set("id", userId)
	return c
}

func TestModelUsageAnalysisScope(t *testing.T) {
	t.Run("regular user cannot override own scope", func(t *testing.T) {
		scope, err := modelUsageAnalysisScope(usageScopeTestContext(common.RoleCommonUser, 7, "?user_id=8"))
		require.NoError(t, err)
		assert.Equal(t, 7, scope)
	})

	t.Run("admin can view all users", func(t *testing.T) {
		scope, err := modelUsageAnalysisScope(usageScopeTestContext(common.RoleAdminUser, 10, ""))
		require.NoError(t, err)
		assert.Zero(t, scope)
	})

	t.Run("admin can select another user", func(t *testing.T) {
		scope, err := modelUsageAnalysisScope(usageScopeTestContext(common.RoleAdminUser, 10, "?user_id=8"))
		require.NoError(t, err)
		assert.Equal(t, 8, scope)
	})

	t.Run("admin rejects an invalid user filter", func(t *testing.T) {
		_, err := modelUsageAnalysisScope(usageScopeTestContext(common.RoleAdminUser, 10, "?user_id=invalid"))
		assert.Error(t, err)
	})
}

func TestAdminCanManageAnotherUsersUsageAdjustment(t *testing.T) {
	admin := usageScopeTestContext(common.RoleAdminUser, 10, "")
	assert.True(t, canManageUsageAdjustment(admin))

	regularUser := usageScopeTestContext(common.RoleCommonUser, 7, "")
	assert.False(t, canManageUsageAdjustment(regularUser))
}

func TestQuotaStatisticsChannelIdIsRootOnly(t *testing.T) {
	tests := []struct {
		name  string
		role  int
		query string
		want  int
	}{
		{name: "root channel filter", role: common.RoleRootUser, query: "?channel_id=42", want: 42},
		{name: "admin filter ignored", role: common.RoleAdminUser, query: "?channel_id=42", want: 0},
		{name: "regular user filter ignored", role: common.RoleCommonUser, query: "?channel_id=42", want: 0},
		{name: "invalid root filter ignored", role: common.RoleRootUser, query: "?channel_id=invalid", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := usageScopeTestContext(tc.role, 1, tc.query)
			assert.Equal(t, tc.want, quotaStatisticsChannelId(c))
		})
	}
}

func TestQuotaStatisticsChannelListRejectsNonRoot(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)

	GetQuotaStatisticsChannelList(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "root permission required")
}
