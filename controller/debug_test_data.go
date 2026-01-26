package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// DebugTestData 测试数据（简化版模板）
type DebugTestData struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Vendor      string `json:"vendor"`   // openai, claude, gemini, other
	Category    string `json:"category"` // basic, multimodal, tools, thinking, multi_turn
	RequestBody string `json:"request_body"`
	Tags        string `json:"tags"`
	Description string `json:"description"`
}

// GetDebugTestData 获取测试数据列表
func GetDebugTestData(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	keyword := c.Query("keyword")
	vendor := c.Query("vendor")
	category := c.Query("category")

	db := model.DB.Model(&model.DebugRequestTemplate{})

	// 关键词搜索
	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 按 vendor 筛选（存储在 other 字段中）
	if vendor != "" {
		db = db.Where("JSON_EXTRACT(variables, '$.vendor') = ?", vendor)
	}

	// 按 category 筛选
	if category != "" {
		db = db.Where("JSON_EXTRACT(variables, '$.category') = ?", category)
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

	// 转换为测试数据格式
	result := make([]DebugTestData, len(templates))
	for i, t := range templates {
		result[i] = templateToTestData(&t)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":      result,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// CreateDebugTestData 创建测试数据
func CreateDebugTestData(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Vendor      string `json:"vendor"`
		Category    string `json:"category"`
		RequestBody string `json:"request_body"`
		Tags        string `json:"tags"`
		Description string `json:"description"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Name is required",
		})
		return
	}

	// 根据 vendor 确定路径
	path := "/v1/chat/completions"
	contentType := "application/json"
	switch req.Vendor {
	case "claude":
		path = "/v1/messages"
	case "gemini":
		path = "/v1beta/models/{model}:generateContent"
	}

	// 存储 vendor 和 category 到 variables 字段
	variables := map[string]interface{}{
		"vendor":   req.Vendor,
		"category": req.Category,
	}
	variablesJSON, _ := json.Marshal(variables)

	// 处理 tags - 可能是字符串或数组
	var tagsJSON string
	if req.Tags != "" {
		// 尝试解析为数组
		var tagsArray []string
		if err := json.Unmarshal([]byte(req.Tags), &tagsArray); err != nil {
			// 如果不是数组，当作单个标签
			tagsArray = []string{req.Tags}
		}
		tagsBytes, _ := json.Marshal(tagsArray)
		tagsJSON = string(tagsBytes)
	} else {
		tagsJSON = "[]"
	}

	// 创建模板
	template := &model.DebugRequestTemplate{
		Name:         req.Name,
		Description:  req.Description,
		Method:       "POST",
		Path:         path,
		ContentType:  contentType,
		BodyTemplate: req.RequestBody,
		Variables:    string(variablesJSON),
		Tags:         tagsJSON,
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
		"data":    templateToTestData(template),
	})
}

// UpdateDebugTestData 更新测试数据
func UpdateDebugTestData(c *gin.Context) {
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
			"message": "Test data not found",
		})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Vendor      string `json:"vendor"`
		Category    string `json:"category"`
		RequestBody string `json:"request_body"`
		Tags        string `json:"tags"`
		Description string `json:"description"`
	}

	err = c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 更新字段
	if req.Name != "" {
		template.Name = req.Name
	}
	template.Description = req.Description
	if req.RequestBody != "" {
		template.BodyTemplate = req.RequestBody
	}

	// 更新 vendor 和 category
	if req.Vendor != "" || req.Category != "" {
		variables := map[string]interface{}{
			"vendor":   req.Vendor,
			"category": req.Category,
		}
		variablesJSON, _ := json.Marshal(variables)
		template.Variables = string(variablesJSON)

		// 更新路径
		switch req.Vendor {
		case "claude":
			template.Path = "/v1/messages"
		case "gemini":
			template.Path = "/v1beta/models/{model}:generateContent"
		default:
			template.Path = "/v1/chat/completions"
		}
	}

	// 处理 tags
	if req.Tags != "" {
		var tagsArray []string
		if err := json.Unmarshal([]byte(req.Tags), &tagsArray); err != nil {
			tagsArray = []string{req.Tags}
		}
		tagsBytes, _ := json.Marshal(tagsArray)
		template.Tags = string(tagsBytes)
	}

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
		"data":    templateToTestData(template),
	})
}

// DeleteDebugTestData 删除测试数据
func DeleteDebugTestData(c *gin.Context) {
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
		"message": "Test data deleted successfully",
	})
}

// templateToTestData 将模板转换为测试数据格式
func templateToTestData(t *model.DebugRequestTemplate) DebugTestData {
	data := DebugTestData{
		Id:          t.Id,
		Name:        t.Name,
		RequestBody: t.BodyTemplate,
		Tags:        t.Tags,
		Description: t.Description,
	}

	// 从 variables 中提取 vendor 和 category
	if t.Variables != "" {
		var variables map[string]interface{}
		if err := json.Unmarshal([]byte(t.Variables), &variables); err == nil {
			if vendor, ok := variables["vendor"].(string); ok {
				data.Vendor = vendor
			}
			if category, ok := variables["category"].(string); ok {
				data.Category = category
			}
		}
	}

	return data
}
