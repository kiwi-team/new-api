package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ========== Task 4.1: Admin config endpoints ==========

// GetSettlementConfigs GET /api/settlement/config?user_id=
// Query settlement configs by user_id query param (admin only)
func GetSettlementConfigs(c *gin.Context) {
	userIdStr := c.Query("user_id")
	if userIdStr == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 user_id 参数",
		})
		return
	}
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "user_id 参数格式错误",
		})
		return
	}
	configs, err := model.GetSettlementConfigsByUserId(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    configs,
	})
}

// CreateSettlementConfigHandler POST /api/settlement/config
// Create a settlement config. Validates non-negative prices, user exists, model_name not empty.
func CreateSettlementConfigHandler(c *gin.Context) {
	var config model.SettlementConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	// Validate model_name not empty
	if strings.TrimSpace(config.ModelName) == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "模型名称不能为空",
		})
		return
	}
	// Validate non-negative prices
	if config.InputPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入价格不能为负数",
		})
		return
	}
	if config.OutputPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输出价格不能为负数",
		})
		return
	}
	if config.RequestPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "次数价格不能为负数",
		})
		return
	}
	// Validate user exists
	_, err := model.GetUserById(config.UserId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	// Create config
	if err := model.CreateSettlementConfig(&config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// UpdateSettlementConfigHandler PUT /api/settlement/config
// Update a settlement config. Validates non-negative prices.
func UpdateSettlementConfigHandler(c *gin.Context) {
	var config model.SettlementConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if config.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供配置 ID",
		})
		return
	}
	// Validate non-negative prices
	if config.InputPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入价格不能为负数",
		})
		return
	}
	if config.OutputPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输出价格不能为负数",
		})
		return
	}
	if config.RequestPrice < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "次数价格不能为负数",
		})
		return
	}
	if err := model.UpdateSettlementConfig(&config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteSettlementConfigHandler DELETE /api/settlement/config/:id
// Delete a settlement config by id path param.
func DeleteSettlementConfigHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 ID",
		})
		return
	}
	if err := model.DeleteSettlementConfig(id); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// batchImportRequest is the JSON format for batch import
type batchImportRequest struct {
	UserId  int                     `json:"user_id"`
	Configs []batchImportConfigItem `json:"configs"`
}

type batchImportConfigItem struct {
	ModelName    string  `json:"model_name"`
	InputPrice   float64 `json:"input_price"`
	OutputPrice  float64 `json:"output_price"`
	RequestPrice float64 `json:"request_price"`
}

// BatchImportSettlementConfigs POST /api/settlement/config/batch
// JSON batch import with transaction, rollback on any failure.
func BatchImportSettlementConfigs(c *gin.Context) {
	var req batchImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 JSON 格式",
		})
		return
	}
	if req.UserId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 user_id",
		})
		return
	}
	// Validate user exists
	_, err := model.GetUserById(req.UserId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	if len(req.Configs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
		})
		return
	}
	// Validate each config and build model slice
	configs := make([]*model.SettlementConfig, 0, len(req.Configs))
	for i, item := range req.Configs {
		if strings.TrimSpace(item.ModelName) == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("第 %d 条配置的模型名称不能为空", i+1),
			})
			return
		}
		if item.InputPrice < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("第 %d 条配置的输入价格不能为负数", i+1),
			})
			return
		}
		if item.OutputPrice < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("第 %d 条配置的输出价格不能为负数", i+1),
			})
			return
		}
		if item.RequestPrice < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("第 %d 条配置的次数价格不能为负数", i+1),
			})
			return
		}
		configs = append(configs, &model.SettlementConfig{
			UserId:       req.UserId,
			ModelName:    item.ModelName,
			InputPrice:   item.InputPrice,
			OutputPrice:  item.OutputPrice,
			RequestPrice: item.RequestPrice,
		})
	}
	if err := model.BatchCreateSettlementConfigs(configs); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("批量导入失败: %s", err.Error()),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ========== Task 4.2: User-side endpoints ==========

// GetSelfSettlementConfigs GET /api/settlement/config/self
// Get current user's settlement configs. Uses c.GetInt("id") for data isolation.
func GetSelfSettlementConfigs(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}
	configs, err := model.GetSettlementConfigsByUserId(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    configs,
	})
}

// GetSelfSettlementBill GET /api/settlement/bill/self?start_timestamp=&end_timestamp=
// Get current user's settlement bill. Forces use of logged-in user ID for data isolation.
func GetSelfSettlementBill(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}
	startTimestampStr := c.Query("start_timestamp")
	endTimestampStr := c.Query("end_timestamp")
	if startTimestampStr == "" || endTimestampStr == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 start_timestamp 和 end_timestamp 参数",
		})
		return
	}
	startTimestamp, err := strconv.ParseInt(startTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "start_timestamp 参数格式错误",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(endTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "end_timestamp 参数格式错误",
		})
		return
	}
	// Validate time range
	if err := service.ValidateTimeRange(startTimestamp, endTimestamp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	bill, err := service.CalculateSettlementBill(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    bill,
	})
}

// ========== Task 4.3: Admin bill query endpoints ==========

// AdminGetSettlementBill GET /api/settlement/bill/admin?user_id=&start_timestamp=&end_timestamp=
// Admin endpoint to query any user's settlement bill.
func AdminGetSettlementBill(c *gin.Context) {
	userIdStr := c.Query("user_id")
	startTimestampStr := c.Query("start_timestamp")
	endTimestampStr := c.Query("end_timestamp")
	if userIdStr == "" || startTimestampStr == "" || endTimestampStr == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 user_id、start_timestamp 和 end_timestamp 参数",
		})
		return
	}
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "user_id 参数格式错误",
		})
		return
	}
	startTimestamp, err := strconv.ParseInt(startTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "start_timestamp 参数格式错误",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(endTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "end_timestamp 参数格式错误",
		})
		return
	}
	// Validate time range
	if err := service.ValidateTimeRange(startTimestamp, endTimestamp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	bill, err := service.CalculateSettlementBill(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    bill,
	})
}

// AdminExportSettlementBillCSV GET /api/settlement/bill/admin/export?user_id=&start_timestamp=&end_timestamp=
// Admin endpoint to export a user's settlement bill as CSV.
func AdminExportSettlementBillCSV(c *gin.Context) {
	userIdStr := c.Query("user_id")
	startTimestampStr := c.Query("start_timestamp")
	endTimestampStr := c.Query("end_timestamp")
	if userIdStr == "" || startTimestampStr == "" || endTimestampStr == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 user_id、start_timestamp 和 end_timestamp 参数",
		})
		return
	}
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "user_id 参数格式错误",
		})
		return
	}
	startTimestamp, err := strconv.ParseInt(startTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "start_timestamp 参数格式错误",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(endTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "end_timestamp 参数格式错误",
		})
		return
	}
	// Validate time range
	if err := service.ValidateTimeRange(startTimestamp, endTimestamp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	bill, err := service.CalculateSettlementBill(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	csvData, err := service.ExportSettlementBillCSV(bill)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	filename := fmt.Sprintf("settlement_bill_user_%d.csv", userId)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment;filename=%s", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", csvData)
}
