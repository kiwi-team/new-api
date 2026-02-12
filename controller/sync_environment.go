package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetSyncEnvironments 获取所有同步环境
func GetSyncEnvironments(c *gin.Context) {
	environments, err := model.GetAllSyncEnvironments()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 隐藏敏感信息
	for _, env := range environments {
		env.RootToken = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    environments,
	})
}

// GetEnabledSyncEnvironments 获取已启用的同步环境
func GetEnabledSyncEnvironments(c *gin.Context) {
	environments, err := model.GetEnabledSyncEnvironments()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 隐藏敏感信息
	for _, env := range environments {
		env.RootToken = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    environments,
	})
}

// AddSyncEnvironment 添加同步环境
func AddSyncEnvironment(c *gin.Context) {
	var env model.SyncEnvironment
	if err := c.ShouldBindJSON(&env); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	if err := env.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "添加成功",
	})
}

// UpdateSyncEnvironment 更新同步环境
func UpdateSyncEnvironment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的环境ID",
		})
		return
	}

	var env model.SyncEnvironment
	if err := c.ShouldBindJSON(&env); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	env.Id = id

	// 如果没有传token，保留原有token
	if env.RootToken == "" {
		existingEnv, err := model.GetSyncEnvironmentById(id)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "环境不存在",
			})
			return
		}
		env.RootToken = existingEnv.RootToken
	}

	if err := env.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}

// DeleteSyncEnvironment 删除同步环境
func DeleteSyncEnvironment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的环境ID",
		})
		return
	}

	env := &model.SyncEnvironment{Id: id}
	if err := env.Delete(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// TestSyncEnvironment 测试同步环境连接
func TestSyncEnvironment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的环境ID",
		})
		return
	}

	env, err := model.GetSyncEnvironmentById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "环境不存在",
		})
		return
	}

	client := service.NewSyncClient(env)
	if err := client.TestConnection(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "连接成功",
	})
}
