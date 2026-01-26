package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/service"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetDebugConfigurations 获取配置列表
func GetDebugConfigurations(c *gin.Context) {
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

	db := model.DB.Model(&model.DebugConfiguration{})

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

	var configs []model.DebugConfiguration
	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&configs).Error
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
			"list":      configs,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetDebugConfiguration 获取配置详情
func GetDebugConfiguration(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	config := &model.DebugConfiguration{}
	err = model.DB.First(config, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Configuration not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// CreateDebugConfiguration 创建配置
func CreateDebugConfiguration(c *gin.Context) {
	config := &model.DebugConfiguration{}
	err := c.ShouldBindJSON(config)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if config.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Name is required",
		})
		return
	}

	// 验证至少有一个测试内容
	if config.TemplateId == nil && config.TestSuiteId == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Either template_id or test_suite_id is required",
		})
		return
	}

	// 验证至少有一个 API 配置
	if config.ChannelId == nil && config.BaseUrl == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Either channel_id or base_url is required",
		})
		return
	}

	err = model.DB.Create(config).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateDebugConfiguration 更新配置
func UpdateDebugConfiguration(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	config := &model.DebugConfiguration{}
	err = model.DB.First(config, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Configuration not found",
		})
		return
	}

	updates := &model.DebugConfiguration{}
	err = c.ShouldBindJSON(updates)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if updates.Name != "" {
		config.Name = updates.Name
	}
	if updates.Description != "" {
		config.Description = updates.Description
	}
	config.ChannelId = updates.ChannelId
	config.BaseUrl = updates.BaseUrl
	config.ApiKey = updates.ApiKey
	config.TestSuiteId = updates.TestSuiteId
	config.TemplateId = updates.TemplateId
	config.VariableValues = updates.VariableValues
	config.Tags = updates.Tags

	err = model.DB.Save(config).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// DeleteDebugConfiguration 删除配置
func DeleteDebugConfiguration(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	err = model.DB.Delete(&model.DebugConfiguration{}, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration deleted successfully",
	})
}

// ExecuteDebugConfiguration 执行调试配置
func ExecuteDebugConfiguration(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	// 获取配置
	config := &model.DebugConfiguration{}
	err = model.DB.First(config, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Configuration not found",
		})
		return
	}

	// 获取渠道信息（如果有）
	var channel *model.Channel
	if config.ChannelId != nil {
		channel = &model.Channel{}
		err = model.DB.First(channel, *config.ChannelId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Channel not found",
			})
			return
		}
	}

	// 创建执行器
	executor := service.NewDebugExecutor(config, channel)

	var logs []*model.DebugRequestLog

	// 判断执行类型
	if config.TestSuiteId != nil {
		// 执行测试集
		suite := &model.DebugTestSuite{}
		err = model.DB.First(suite, *config.TestSuiteId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Test suite not found",
			})
			return
		}

		logs, err = executor.ExecuteTestSuite(suite)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Execution failed: " + err.Error(),
			})
			return
		}
	} else if config.TemplateId != nil {
		// 执行单个模板
		template := &model.DebugRequestTemplate{}
		err = model.DB.First(template, *config.TemplateId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Template not found",
			})
			return
		}

		log, err := executor.ExecuteTemplate(template)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Execution failed: " + err.Error(),
			})
			return
		}
		logs = []*model.DebugRequestLog{log}
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No template or test suite specified",
		})
		return
	}

	// 保存日志到数据库
	for _, log := range logs {
		if err := model.DB.Create(log).Error; err != nil {
			common.SysError("Failed to save debug log: " + err.Error())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}

// ExecuteDebugRequest 直接执行调试请求（不保存配置）
// 支持两种模式：
// 1. 使用 template_id 或 test_suite_id 执行预定义模板
// 2. 直接传入 body, path, headers 等参数执行
func ExecuteDebugRequest(c *gin.Context) {
	var req struct {
		// 模式和目标
		Mode     string `json:"mode"`      // channel 或 key
		TargetId *int   `json:"target_id"` // 渠道ID或KeyID
		Format   string `json:"format"`    // openai, claude, gemini

		// API 配置
		BaseUrl string            `json:"base_url"`
		ApiKey  string            `json:"api_key"`
		Path    string            `json:"path"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`

		// 请求体
		Body interface{} `json:"body"`

		// 模板模式（可选）
		TemplateId  *int `json:"template_id"`
		TestSuiteId *int `json:"test_suite_id"`

		// 变量值（模板模式使用）
		VariableValues map[string]interface{} `json:"variable_values"`

		// 标签和备注
		Tags   []string `json:"tags"`
		Remark string   `json:"remark"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 如果有 template_id 或 test_suite_id，使用模板模式
	if req.TemplateId != nil || req.TestSuiteId != nil {
		executeWithTemplate(c, &req)
		return
	}

	// 直接执行模式
	executeDirectRequest(c, &req)
}

// executeWithTemplate 使用模板执行
func executeWithTemplate(c *gin.Context, req *struct {
	Mode           string                 `json:"mode"`
	TargetId       *int                   `json:"target_id"`
	Format         string                 `json:"format"`
	BaseUrl        string                 `json:"base_url"`
	ApiKey         string                 `json:"api_key"`
	Path           string                 `json:"path"`
	Method         string                 `json:"method"`
	Headers        map[string]string      `json:"headers"`
	Body           interface{}            `json:"body"`
	TemplateId     *int                   `json:"template_id"`
	TestSuiteId    *int                   `json:"test_suite_id"`
	VariableValues map[string]interface{} `json:"variable_values"`
	Tags           []string               `json:"tags"`
	Remark         string                 `json:"remark"`
}) {
	// 创建临时配置
	config := &model.DebugConfiguration{
		BaseUrl:     req.BaseUrl,
		ApiKey:      req.ApiKey,
		TemplateId:  req.TemplateId,
		TestSuiteId: req.TestSuiteId,
	}
	config.SetVariableValues(req.VariableValues)

	// 获取渠道信息（如果有）
	var channel *model.Channel
	if req.TargetId != nil && req.Mode == "channel" {
		channel = &model.Channel{}
		err := model.DB.First(channel, *req.TargetId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Channel not found",
			})
			return
		}
		config.ChannelId = req.TargetId
	}

	// 创建执行器
	executor := service.NewDebugExecutor(config, channel)

	var logs []*model.DebugRequestLog
	var err error

	// 判断执行类型
	if config.TestSuiteId != nil {
		suite := &model.DebugTestSuite{}
		err = model.DB.First(suite, *config.TestSuiteId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Test suite not found",
			})
			return
		}

		logs, err = executor.ExecuteTestSuite(suite)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Execution failed: " + err.Error(),
			})
			return
		}
	} else if config.TemplateId != nil {
		template := &model.DebugRequestTemplate{}
		err = model.DB.First(template, *config.TemplateId).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Template not found",
			})
			return
		}

		log, err := executor.ExecuteTemplate(template)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Execution failed: " + err.Error(),
			})
			return
		}
		logs = []*model.DebugRequestLog{log}
	}

	// 保存日志到数据库
	for _, log := range logs {
		if err := model.DB.Create(log).Error; err != nil {
			common.SysError("Failed to save debug log: " + err.Error())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}

// ExecuteDebugRequestStream 流式执行调试请求（SSE）
func ExecuteDebugRequestStream(c *gin.Context) {
	var req struct {
		// 模式和目标
		Mode     string `json:"mode"`      // channel 或 key
		TargetId *int   `json:"target_id"` // 渠道ID或KeyID
		Format   string `json:"format"`    // openai, claude, gemini

		// API 配置
		BaseUrl string            `json:"base_url"`
		ApiKey  string            `json:"api_key"`
		Path    string            `json:"path"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`

		// 请求体
		Body interface{} `json:"body"`

		// 标签和备注
		Tags   []string `json:"tags"`
		Remark string   `json:"remark"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 验证必要参数
	if req.BaseUrl == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "base_url is required",
		})
		return
	}
	if req.Path == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "path is required",
		})
		return
	}

	// 设置默认方法
	method := req.Method
	if method == "" {
		method = "POST"
	}

	// 序列化请求体
	var bodyStr string
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to serialize body: " + err.Error(),
			})
			return
		}
		bodyStr = string(bodyBytes)
	}

	// 序列化 headers
	headersStr := ""
	if req.Headers != nil {
		headersBytes, _ := json.Marshal(req.Headers)
		headersStr = string(headersBytes)
	}

	// 序列化 tags
	tagsStr := ""
	if req.Tags != nil {
		tagsBytes, _ := json.Marshal(req.Tags)
		tagsStr = string(tagsBytes)
	}

	// 提取模型名称
	modelName := ""
	if bodyMap, ok := req.Body.(map[string]interface{}); ok {
		if m, ok := bodyMap["model"].(string); ok {
			modelName = m
		}
	}

	// 确保 headers 不为 nil，并添加 Authorization header（如果缺失）
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	if _, exists := req.Headers["Authorization"]; !exists && req.ApiKey != "" {
		req.Headers["Authorization"] = "Bearer " + req.ApiKey
	}
	// 确保 Content-Type 存在
	if _, exists := req.Headers["Content-Type"]; !exists {
		req.Headers["Content-Type"] = "application/json"
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	// 执行流式请求
	executor := service.NewDirectDebugExecutor(req.BaseUrl, req.ApiKey)
	result, err := executor.ExecuteStream(method, req.Path, req.Headers, bodyStr, c.Writer)

	if err != nil {
		common.SysError("Stream execution error: " + err.Error())
	}

	// 如果 result 为 nil，创建一个默认的
	if result == nil {
		result = &service.DirectExecuteResult{
			Status:       "error",
			ErrorMessage: "Unknown error",
		}
		if err != nil {
			result.ErrorMessage = err.Error()
		}
	}

	// 创建日志记录（流式请求完成后保存）
	log := &model.DebugRequestLog{
		RequestMethod:   method,
		RequestUrl:      req.BaseUrl + req.Path,
		RequestHeaders:  headersStr,
		RequestBody:     bodyStr,
		ModelName:       modelName,
		IsStream:        true,
		ResponseStatus:  result.StatusCode,
		ResponseHeaders: result.ResponseHeaders,
		ResponseBody:    result.ResponseBody,
		ResponseChunks:  result.ResponseChunks,
		DurationMs:      result.DurationMs,
		TtfbMs:          result.TtfbMs,
		ErrorMessage:    result.ErrorMessage,
		Status:          result.Status,
		Tags:            tagsStr,
		Remark:          req.Remark,
	}

	// 保存日志
	if err := model.DB.Create(log).Error; err != nil {
		common.SysError("Failed to save debug log: " + err.Error())
	}
}

// executeDirectRequest 直接执行请求（不使用模板）
func executeDirectRequest(c *gin.Context, req *struct {
	Mode           string                 `json:"mode"`
	TargetId       *int                   `json:"target_id"`
	Format         string                 `json:"format"`
	BaseUrl        string                 `json:"base_url"`
	ApiKey         string                 `json:"api_key"`
	Path           string                 `json:"path"`
	Method         string                 `json:"method"`
	Headers        map[string]string      `json:"headers"`
	Body           interface{}            `json:"body"`
	TemplateId     *int                   `json:"template_id"`
	TestSuiteId    *int                   `json:"test_suite_id"`
	VariableValues map[string]interface{} `json:"variable_values"`
	Tags           []string               `json:"tags"`
	Remark         string                 `json:"remark"`
}) {
	// 验证必要参数
	if req.BaseUrl == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "base_url is required",
		})
		return
	}
	if req.Path == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "path is required",
		})
		return
	}

	// 设置默认方法
	method := req.Method
	if method == "" {
		method = "POST"
	}

	// 序列化请求体
	var bodyStr string
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to serialize body: " + err.Error(),
			})
			return
		}
		bodyStr = string(bodyBytes)
	}

	// 序列化 headers
	headersStr := ""
	if req.Headers != nil {
		headersBytes, _ := json.Marshal(req.Headers)
		headersStr = string(headersBytes)
	}

	// 序列化 tags
	tagsStr := ""
	if req.Tags != nil {
		tagsBytes, _ := json.Marshal(req.Tags)
		tagsStr = string(tagsBytes)
	}

	// 提取模型名称
	modelName := ""
	if bodyMap, ok := req.Body.(map[string]interface{}); ok {
		if m, ok := bodyMap["model"].(string); ok {
			modelName = m
		}
	}

	// 检查是否是流式请求
	isStream := false
	if bodyMap, ok := req.Body.(map[string]interface{}); ok {
		if s, ok := bodyMap["stream"].(bool); ok {
			isStream = s
		}
	}

	// 确保 headers 不为 nil，并添加 Authorization header（如果缺失）
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	if _, exists := req.Headers["Authorization"]; !exists && req.ApiKey != "" {
		req.Headers["Authorization"] = "Bearer " + req.ApiKey
	}
	// 确保 Content-Type 存在
	if _, exists := req.Headers["Content-Type"]; !exists {
		req.Headers["Content-Type"] = "application/json"
	}

	// 执行请求
	executor := service.NewDirectDebugExecutor(req.BaseUrl, req.ApiKey)
	result, err := executor.Execute(method, req.Path, req.Headers, bodyStr, isStream)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Execution failed: " + err.Error(),
		})
		return
	}

	// 创建日志记录
	log := &model.DebugRequestLog{
		RequestMethod:   method,
		RequestUrl:      req.BaseUrl + req.Path,
		RequestHeaders:  headersStr,
		RequestBody:     bodyStr,
		ModelName:       modelName,
		IsStream:        isStream,
		ResponseStatus:  result.StatusCode,
		ResponseHeaders: result.ResponseHeaders,
		ResponseBody:    result.ResponseBody,
		ResponseChunks:  result.ResponseChunks,
		DurationMs:      result.DurationMs,
		TtfbMs:          result.TtfbMs,
		ErrorMessage:    result.ErrorMessage,
		Status:          result.Status,
		Tags:            tagsStr,
		Remark:          req.Remark,
	}

	// 保存日志
	if err := model.DB.Create(log).Error; err != nil {
		common.SysError("Failed to save debug log: " + err.Error())
		// Still return the log data even if save failed
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    log,
			"warning": "Log saved to response but failed to persist to database: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    log,
	})
}
