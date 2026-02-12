package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// SyncModelPricesRequest 同步模型价格请求
type SyncModelPricesRequest struct {
	EnvironmentIds []int    `json:"environment_ids" binding:"required"`
	SelectedModels []string `json:"selected_models"` // 可选，为空则同步全部
}

// SyncModelPrices 同步模型价格到指定环境
func SyncModelPrices(c *gin.Context) {
	var req SyncModelPricesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
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

	// 获取当前的模型价格配置
	var modelRatio, modelPrice, completionRatio string

	if len(req.SelectedModels) == 0 {
		// 同步全部
		modelRatio = ratio_setting.ModelRatio2JSONString()
		modelPrice = ratio_setting.ModelPrice2JSONString()
		completionRatio = ratio_setting.CompletionRatio2JSONString()
	} else {
		// 只同步选中的模型
		selectedSet := make(map[string]bool)
		for _, m := range req.SelectedModels {
			selectedSet[m] = true
		}

		// 过滤 ModelRatio
		allModelRatio := ratio_setting.GetModelRatioMap()
		filteredModelRatio := make(map[string]float64)
		for k, v := range allModelRatio {
			if selectedSet[k] {
				filteredModelRatio[k] = v
			}
		}
		if len(filteredModelRatio) > 0 {
			bytes, _ := json.Marshal(filteredModelRatio)
			modelRatio = string(bytes)
		}

		// 过滤 ModelPrice
		allModelPrice := ratio_setting.GetModelPriceMap()
		filteredModelPrice := make(map[string]float64)
		for k, v := range allModelPrice {
			if selectedSet[k] {
				filteredModelPrice[k] = v
			}
		}
		if len(filteredModelPrice) > 0 {
			bytes, _ := json.Marshal(filteredModelPrice)
			modelPrice = string(bytes)
		}

		// 过滤 CompletionRatio
		allCompletionRatio := ratio_setting.GetCompletionRatioMap()
		filteredCompletionRatio := make(map[string]float64)
		for k, v := range allCompletionRatio {
			if selectedSet[k] {
				filteredCompletionRatio[k] = v
			}
		}
		if len(filteredCompletionRatio) > 0 {
			bytes, _ := json.Marshal(filteredCompletionRatio)
			completionRatio = string(bytes)
		}
	}

	// 获取操作人ID
	operatorId := c.GetInt("id")

	// 构建同步摘要
	dataSummary := "同步模型价格配置(ModelRatio, ModelPrice, CompletionRatio)"
	if len(req.SelectedModels) > 0 {
		dataSummary = fmt.Sprintf("同步选中模型价格配置，共 %d 个模型", len(req.SelectedModels))
	}

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

		// 执行同步
		var result *service.SyncResult
		if len(req.SelectedModels) == 0 {
			// 全量同步
			result = service.SyncModelPricesToEnvironment(modelRatio, modelPrice, completionRatio, env)
		} else {
			// 增量同步（合并模式）
			result = service.SyncModelPricesIncrementalToEnvironment(modelRatio, modelPrice, completionRatio, env)
		}
		results = append(results, result)

		// 记录同步日志
		status := service.SyncStatusSuccess
		errorMsg := ""
		if !result.Success {
			status = service.SyncStatusFailed
			errorMsg = result.Error
		}

		syncLogs = append(syncLogs, &model.SyncLog{
			SyncType:        service.SyncTypeModelPrice,
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
