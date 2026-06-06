package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// GetOrgs 返回当前可分配的组织清单(enabled),供 root 在用户编辑下拉里选择。
//
// 只读接口,无 CRUD。详见 org.md:组织清单是 Go 常量(constant/org.go),不入 DB 表。
// 新增 / 启停组织 = 改代码 + 重新部署。
//
// Response:
//
//	{
//	  "success": true,
//	  "data": [{"code":"mt","name":"MT","enabled":true}, ...]
//	}
func GetOrgs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    constant.EnabledOrgs(),
	})
}
