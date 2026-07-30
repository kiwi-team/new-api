package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// SyncChannelsRequest 同步渠道请求
type SyncChannelsRequest struct {
	ChannelIds     []int               `json:"channel_ids" binding:"required"`
	EnvironmentIds []int               `json:"environment_ids" binding:"required"`
	ChannelMapping map[int]map[int]int `json:"channel_mapping,omitempty"` // environment_id -> source_channel_id -> target_channel_id
}

// PreviewSyncChannelsRequest 预览同步渠道请求
type PreviewSyncChannelsRequest struct {
	ChannelIds     []int `json:"channel_ids" binding:"required"`
	EnvironmentIds []int `json:"environment_ids" binding:"required"`
}

// SyncChannels 同步渠道到指定环境
func SyncChannels(c *gin.Context) {
	var req SyncChannelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	if len(req.ChannelIds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请选择要同步的渠道",
		})
		return
	}

	if len(req.EnvironmentIds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请选择目标环境",
		})
		return
	}

	// 获取渠道数据
	channels, err := model.GetChannelsByIds(req.ChannelIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取渠道数据失败: " + err.Error(),
		})
		return
	}

	if len(channels) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未找到指定的渠道",
		})
		return
	}

	// 获取目标环境
	environments, err := model.GetSyncEnvironmentsByIds(req.EnvironmentIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取环境配置失败: " + err.Error(),
		})
		return
	}

	if len(environments) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未找到指定的环境",
		})
		return
	}

	// 获取操作人ID
	operatorId := c.GetInt("id")

	// 同步到每个环境
	results := make([]*service.SyncResult, 0)
	syncLogs := make([]*model.SyncLog, 0)

	for _, env := range environments {
		// 检查环境是否启用
		if env.Status != 1 {
			results = append(results, &service.SyncResult{
				EnvironmentId:   env.Id,
				EnvironmentName: env.Name,
				Success:         false,
				Error:           "环境已禁用",
			})
			continue
		}

		// 构建该环境的渠道映射
		var envMapping service.ChannelMapping
		if req.ChannelMapping != nil {
			if m, ok := req.ChannelMapping[env.Id]; ok {
				envMapping = service.ChannelMapping(m)
			}
		}

		// 执行同步
		result := service.SyncChannelsToEnvironment(channels, env, envMapping)
		results = append(results, result)

		// 记录同步日志
		status := service.SyncStatusSuccess
		errorMsg := ""
		if !result.Success {
			status = service.SyncStatusFailed
			errorMsg = result.Error
		}

		// 生成数据摘要
		channelNames := make([]string, 0)
		for _, ch := range channels {
			channelNames = append(channelNames, ch.Name)
		}
		dataSummary := fmt.Sprintf("同步%d个渠道: %v", len(channels), channelNames)

		syncLogs = append(syncLogs, &model.SyncLog{
			SyncType:        service.SyncTypeChannel,
			EnvironmentId:   env.Id,
			EnvironmentName: env.Name,
			DataSummary:     dataSummary,
			Status:          status,
			ErrorMessage:    errorMsg,
			OperatorId:      operatorId,
		})
	}

	// 批量保存同步日志
	if len(syncLogs) > 0 {
		_ = model.BatchCreateSyncLogs(syncLogs)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"results": results,
	})
}

// PreviewSyncChannels 预览渠道同步匹配情况
func PreviewSyncChannels(c *gin.Context) {
	var req PreviewSyncChannelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	if len(req.ChannelIds) == 0 || len(req.EnvironmentIds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请选择渠道和目标环境",
		})
		return
	}

	channels, err := model.GetChannelsByIds(req.ChannelIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取渠道数据失败: " + err.Error(),
		})
		return
	}

	environments, err := model.GetSyncEnvironmentsByIds(req.EnvironmentIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取环境配置失败: " + err.Error(),
		})
		return
	}

	previews := make([]*service.EnvironmentPreview, 0, len(environments))
	for _, env := range environments {
		if env.Status != 1 {
			previews = append(previews, &service.EnvironmentPreview{
				EnvironmentId:   env.Id,
				EnvironmentName: env.Name,
				Error:           "环境已禁用",
			})
			continue
		}
		preview := service.PreviewChannelSync(channels, env)
		previews = append(previews, preview)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    previews,
	})
}
