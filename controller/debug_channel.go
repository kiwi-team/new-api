package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// DebugChannelInfo 调试模块使用的渠道信息
type DebugChannelInfo struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	BaseURL string `json:"base_url"`
	Type    int    `json:"type"`
	Models  string `json:"models"`
	Status  int    `json:"status"`
}

// GetDebugChannels 获取所有渠道信息（包含 key 和 base_url）
// 仅限 root 用户访问
func GetDebugChannels(c *gin.Context) {
	var channels []model.Channel
	err := model.DB.Select("id, name, key, base_url, type, models, status").
		Order("id desc").
		Find(&channels).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取渠道列表失败: " + err.Error(),
		})
		return
	}

	// 转换为调试模块使用的格式
	result := make([]DebugChannelInfo, len(channels))
	for i, ch := range channels {
		baseURL := ""
		if ch.BaseURL != nil {
			baseURL = *ch.BaseURL
		}
		result[i] = DebugChannelInfo{
			Id:      ch.Id,
			Name:    ch.Name,
			Key:     ch.Key,
			BaseURL: baseURL,
			Type:    ch.Type,
			Models:  ch.Models,
			Status:  ch.Status,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// DebugKeyInfo 调试模块使用的 Key 信息
type DebugKeyInfo struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Key    string `json:"key"`
	Status int    `json:"status"`
}

// GetDebugKeys 获取所有 Token/Key 信息
// 仅限 root 用户访问
func GetDebugKeys(c *gin.Context) {
	userId := c.GetInt("id")
	var tokens []model.Token
	err := model.DB.Select("id, name, key, status").
		Where("user_id = ?", userId).
		Order("id desc").
		Find(&tokens).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取 Key 列表失败: " + err.Error(),
		})
		return
	}

	// 转换为调试模块使用的格式
	result := make([]DebugKeyInfo, len(tokens))
	for i, t := range tokens {
		result[i] = DebugKeyInfo{
			Id:     t.Id,
			Name:   t.Name,
			Key:    t.Key,
			Status: t.Status,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetDebugTags 获取所有调试日志中使用过的标签
// 用于标签建议
func GetDebugTags(c *gin.Context) {
	// 预设标签
	presetTags := []string{
		"功能验证", "bug复现", "性能测试", "回归测试",
		"多模态", "tools调用", "thinking", "流式",
	}

	// 从数据库获取所有日志的标签字段
	var logs []model.DebugRequestLog
	err := model.DB.Select("tags").Where("tags != '' AND tags IS NOT NULL").Find(&logs).Error
	if err != nil {
		// 如果查询失败，返回预设标签
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    presetTags,
		})
		return
	}

	// 收集所有标签并去重
	tagMap := make(map[string]bool)
	for _, tag := range presetTags {
		tagMap[tag] = true
	}

	for _, log := range logs {
		tags, err := log.GetTags()
		if err == nil {
			for _, tag := range tags {
				if tag != "" {
					tagMap[tag] = true
				}
			}
		}
	}

	// 转换为切片
	uniqueTags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		uniqueTags = append(uniqueTags, tag)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    uniqueTags,
	})
}
