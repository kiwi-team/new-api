package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetUserMenu 返回当前用户能看到的菜单 + 顶栏模式。
//
// 前端登录后调一次缓存到 React Context;SiderBar / Headerbar / PageRoute 路由守卫
// 都消费这个响应,不再自行判断 toio / role。
//
// Response:
//
//	{
//	  "success": true,
//	  "data": {
//	    "topbar_mode": "normal" | "logout_only",
//	    "pages": ["log", "quota_statistics", ...]
//	  }
//	}
func GetUserMenu(c *gin.Context) {
	userID := c.GetInt("id")
	if userID == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	menu := service.GetUserMenu(user)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    menu,
	})
}
