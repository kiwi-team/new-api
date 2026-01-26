package controller

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/model"

	"strconv"

	"github.com/gin-gonic/gin"
)

// GetDebugTemplates 获取模板列表
func GetDebugTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	keyword := c.Query("keyword")
	tags := c.Query("tags") // JSON array string

	db := model.DB.Model(&model.DebugRequestTemplate{})

	// 关键词搜索
	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 标签筛选
	if tags != "" {
		var tagList []string
		if err := json.Unmarshal([]byte(tags), &tagList); err == nil && len(tagList) > 0 {
			for _, tag := range tagList {
				db = db.Where("JSON_CONTAINS(tags, ?)", `"`+tag+`"`)
			}
		}
	}

	// 获取总数
	var total int64
	db.Count(&total)

	// 分页查询
	var templates []model.DebugRequestTemplate
	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&templates).Error
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
			"list":      templates,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetDebugTemplate 获取模板详情
func GetDebugTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	template := &model.DebugRequestTemplate{}
	err = model.DB.First(template, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Template not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    template,
	})
}

// CreateDebugTemplate 创建模板
func CreateDebugTemplate(c *gin.Context) {
	template := &model.DebugRequestTemplate{}
	err := c.ShouldBindJSON(template)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if template.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Name is required",
		})
		return
	}

	if template.Path == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Path is required",
		})
		return
	}

	err = model.DB.Create(template).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    template,
	})
}

// UpdateDebugTemplate 更新模板
func UpdateDebugTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	template := &model.DebugRequestTemplate{}
	err = model.DB.First(template, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Template not found",
		})
		return
	}

	updates := &model.DebugRequestTemplate{}
	err = c.ShouldBindJSON(updates)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 更新字段
	if updates.Name != "" {
		template.Name = updates.Name
	}
	template.Description = updates.Description
	if updates.Method != "" {
		template.Method = updates.Method
	}
	if updates.Path != "" {
		template.Path = updates.Path
	}
	if updates.ContentType != "" {
		template.ContentType = updates.ContentType
	}
	template.BodyTemplate = updates.BodyTemplate
	template.Headers = updates.Headers
	template.Variables = updates.Variables
	template.ResponseSchema = updates.ResponseSchema
	template.StreamValidation = updates.StreamValidation
	template.Tags = updates.Tags

	err = model.DB.Save(template).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    template,
	})
}

// DeleteDebugTemplate 删除模板
func DeleteDebugTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	err = model.DB.Delete(&model.DebugRequestTemplate{}, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Template deleted successfully",
	})
}

// CopyDebugTemplate 复制模板
func CopyDebugTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	original := &model.DebugRequestTemplate{}
	err = model.DB.First(original, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Template not found",
		})
		return
	}

	// 创建副本
	copy := &model.DebugRequestTemplate{
		Name:             original.Name + " (Copy)",
		Description:      original.Description,
		Method:           original.Method,
		Path:             original.Path,
		ContentType:      original.ContentType,
		BodyTemplate:     original.BodyTemplate,
		Headers:          original.Headers,
		Variables:        original.Variables,
		ResponseSchema:   original.ResponseSchema,
		StreamValidation: original.StreamValidation,
		Tags:             original.Tags,
	}

	err = model.DB.Create(copy).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    copy,
	})
}
