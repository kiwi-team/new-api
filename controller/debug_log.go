package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/service"

	"github.com/QuantumNous/new-api/model"

	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetDebugLogs 获取日志列表
func GetDebugLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	// 筛选条件
	configIds := c.Query("config_ids")     // JSON array
	templateIds := c.Query("template_ids") // JSON array
	modelName := c.Query("model_name")
	status := c.Query("status")        // success, failed, error
	isStream := c.Query("is_stream")   // true, false, all
	startTime := c.Query("start_time") // RFC3339 format
	endTime := c.Query("end_time")

	db := model.DB.Model(&model.DebugRequestLog{})

	// 配置ID筛选
	if configIds != "" {
		var ids []int
		if err := json.Unmarshal([]byte(configIds), &ids); err == nil && len(ids) > 0 {
			db = db.Where("configuration_id IN ?", ids)
		}
	}

	// 模板ID筛选
	if templateIds != "" {
		var ids []int
		if err := json.Unmarshal([]byte(templateIds), &ids); err == nil && len(ids) > 0 {
			db = db.Where("template_id IN ?", ids)
		}
	}

	// 模型名称筛选
	if modelName != "" {
		db = db.Where("model_name LIKE ?", "%"+modelName+"%")
	}

	// 状态筛选
	if status != "" {
		statuses := strings.Split(status, ",")
		db = db.Where("status IN ?", statuses)
	}

	// 流式筛选
	if isStream == "true" {
		db = db.Where("is_stream = ?", true)
	} else if isStream == "false" {
		db = db.Where("is_stream = ?", false)
	}

	// 时间范围筛选
	if startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			db = db.Where("created_at >= ?", t)
		}
	}
	if endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			db = db.Where("created_at <= ?", t)
		}
	}

	// 获取总数
	var total int64
	db.Count(&total)

	// 分页查询
	var logs []model.DebugRequestLog
	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs).Error
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
			"list":      logs,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetDebugLog 获取日志详情
func GetDebugLog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	log := &model.DebugRequestLog{}
	err = model.DB.First(log, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Log not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    log,
	})
}

// DeleteDebugLog 删除日志
func DeleteDebugLog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	err = model.DB.Delete(&model.DebugRequestLog{}, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Log deleted successfully",
	})
}

// GenerateCURL 生成 cURL 命令
func GenerateCURL(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	log := &model.DebugRequestLog{}
	err = model.DB.First(log, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Log not found",
		})
		return
	}

	curl := generateCURLCommand(log)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    curl,
	})
}

// generateCURLCommand 生成 cURL 命令
func generateCURLCommand(log *model.DebugRequestLog) string {
	var builder strings.Builder

	builder.WriteString("curl -X ")
	builder.WriteString(log.RequestMethod)
	builder.WriteString(" '")
	builder.WriteString(log.RequestUrl)
	builder.WriteString("'")

	// 添加 headers
	headers, _ := log.GetRequestHeaders()
	for key, value := range headers {
		builder.WriteString(" \\\n  -H '")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(escapeShellString(value))
		builder.WriteString("'")
	}

	// 添加 body
	if log.RequestBody != "" {
		builder.WriteString(" \\\n  -d '")
		builder.WriteString(escapeShellString(log.RequestBody))
		builder.WriteString("'")
	}

	return builder.String()
}

// escapeShellString 转义 shell 字符串
func escapeShellString(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// RerunDebugLog 重新执行日志
func RerunDebugLog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid ID",
		})
		return
	}

	// 获取原始日志
	originalLog := &model.DebugRequestLog{}
	err = model.DB.First(originalLog, id).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Log not found",
		})
		return
	}

	// 获取配置和模板
	if originalLog.ConfigurationId == nil || originalLog.TemplateId == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Cannot rerun: missing configuration or template",
		})
		return
	}

	config := &model.DebugConfiguration{}
	err = model.DB.First(config, *originalLog.ConfigurationId).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Configuration not found",
		})
		return
	}

	template := &model.DebugRequestTemplate{}
	err = model.DB.First(template, *originalLog.TemplateId).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Template not found",
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

	// 创建执行器并执行
	executor := service.NewDebugExecutor(config, channel)
	newLog, err := executor.ExecuteTemplate(template)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Execution failed: " + err.Error(),
		})
		return
	}

	// 保存新日志
	if err := model.DB.Create(newLog).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to save log: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    newLog,
	})
}

// GetDebugLogStats 获取日志统计
func GetDebugLogStats(c *gin.Context) {
	// 时间范围
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	db := model.DB.Model(&model.DebugRequestLog{})

	if startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			db = db.Where("created_at >= ?", t)
		}
	}
	if endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			db = db.Where("created_at <= ?", t)
		}
	}

	// 统计各状态数量
	var stats []struct {
		Status string
		Count  int64
	}
	db.Select("status, COUNT(*) as count").Group("status").Scan(&stats)

	// 统计平均耗时
	var avgDuration float64
	db.Select("AVG(duration_ms) as avg_duration").Scan(&avgDuration)

	// 统计总数
	var total int64
	db.Count(&total)

	// 按模型统计
	var modelStats []struct {
		ModelName string
		Count     int64
	}
	db.Select("model_name, COUNT(*) as count").
		Where("model_name != ''").
		Group("model_name").
		Order("count DESC").
		Limit(10).
		Scan(&modelStats)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total":        total,
			"status_stats": stats,
			"avg_duration": fmt.Sprintf("%.2f", avgDuration),
			"model_stats":  modelStats,
		},
	})
}

// BatchDeleteDebugLogs 批量删除日志
func BatchDeleteDebugLogs(c *gin.Context) {
	var req struct {
		Ids []int `json:"ids"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No IDs provided",
		})
		return
	}

	err = model.DB.Delete(&model.DebugRequestLog{}, req.Ids).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Deleted %d logs successfully", len(req.Ids)),
	})
}

// CleanOldDebugLogs 清理旧日志
func CleanOldDebugLogs(c *gin.Context) {
	days, _ := strconv.Atoi(c.Query("days"))
	if days <= 0 {
		days = 30 // 默认清理30天前的日志
	}

	cutoffTime := time.Now().AddDate(0, 0, -days)

	result := model.DB.Where("created_at < ?", cutoffTime).Delete(&model.DebugRequestLog{})
	if result.Error != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Cleaned %d old logs", result.RowsAffected),
	})
}

// BatchTagDebugLogs 批量给日志添加标签
func BatchTagDebugLogs(c *gin.Context) {
	var req struct {
		Ids  []int    `json:"ids"`
		Tags []string `json:"tags"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	if len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No IDs provided",
		})
		return
	}

	// 将标签转换为 JSON 字符串
	tagsJSON, err := json.Marshal(req.Tags)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to serialize tags: " + err.Error(),
		})
		return
	}

	// 批量更新标签
	err = model.DB.Model(&model.DebugRequestLog{}).
		Where("id IN ?", req.Ids).
		Update("tags", string(tagsJSON)).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Updated tags for %d logs successfully", len(req.Ids)),
	})
}
