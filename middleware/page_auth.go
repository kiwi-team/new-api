package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// PageAuth 检查当前用户能否访问 pageKey 对应的页面/接口。
//
// 调用约定:必须在 UserAuth/AdminAuth/等基础认证之后挂载,依赖 c.Get("id")
// 拿到用户 ID。
//
// 行为:
//   - 系统 admin / root (role >= RoleAdminUser) 直接放行(他们看完整菜单);
//   - 其他用户查 service.HasPage 判定;
//   - 无权限返回 403。
//
// 用例:
//
//	logRoute.GET("/error-logs", middleware.UserAuth(), middleware.PageAuth(service.PageErrorLog), controller.GetAllErrorLogs)
func PageAuth(pageKey string) func(c *gin.Context) {
	return func(c *gin.Context) {
		// 系统级 bypass:全局 admin/root 始终通过
		role := c.GetInt("role")
		if role >= common.RoleAdminUser {
			c.Next()
			return
		}

		userID := c.GetInt("id")
		if userID == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录",
			})
			c.Abort()
			return
		}

		user, err := model.GetUserById(userID, false)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，用户不存在",
			})
			c.Abort()
			return
		}

		if !service.HasPage(user, pageKey) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "无权进行此操作，当前组织/角色无权访问该页面",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
