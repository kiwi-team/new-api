package controller

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/model"

	"strconv"

	"github.com/gin-gonic/gin"
)

// GetDebugTestSuites 获取测试集列表
func GetDebugTestSuites(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	keyword := c.Query("keyword")
	tags := c.Query("tags")

	db := model.DB.Model(&model.DebugTestSuite{})

	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if tags != "" {
		var tagList []string
		if err := json.Unmarshal([]byte(tags), &tagList); err == nil && len(tagList) > 0 {
			for _, tag := range tagList {
				db = db.Where("JSON_CONTAINS(tags, ?)", `"`+tag+`"`)
			}
		}
	}

	var total int64
	db.Count(&total)

	var suites []model.DebugTestSuite
	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&suites).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":      suites,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetDebugTestSuite 获取测试集详情
func GetDebugTestSuite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	suite := &model.DebugTestSuite{}
	err = model.DB.First(suite, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Test suite not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    suite,
	})
}

// CreateDebugTestSuite 创建测试集
func CreateDebugTestSuite(c *gin.Context) {
	suite := &model.DebugTestSuite{}
	err := c.ShouldBindJSON(suite)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if suite.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Name is required",
		})
		return
	}

	if suite.TemplateIds == "" || suite.TemplateIds == "[]" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "At least one template is required",
		})
		return
	}

	err = model.DB.Create(suite).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    suite,
	})
}

// UpdateDebugTestSuite 更新测试集
func UpdateDebugTestSuite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	suite := &model.DebugTestSuite{}
	err = model.DB.First(suite, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Test suite not found",
		})
		return
	}

	updates := &model.DebugTestSuite{}
	err = c.ShouldBindJSON(updates)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if updates.Name != "" {
		suite.Name = updates.Name
	}
	if updates.Description != "" {
		suite.Description = updates.Description
	}
	if updates.TemplateIds != "" {
		suite.TemplateIds = updates.TemplateIds
	}
	if updates.VariableMappings != "" {
		suite.VariableMappings = updates.VariableMappings
	}
	if updates.Tags != "" {
		suite.Tags = updates.Tags
	}

	err = model.DB.Save(suite).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    suite,
	})
}

// DeleteDebugTestSuite 删除测试集
func DeleteDebugTestSuite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	err = model.DB.Delete(&model.DebugTestSuite{}, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Test suite deleted successfully",
	})
}
