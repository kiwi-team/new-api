package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// checkUserIdInOrgScope 校验 user_id 是否在当前用户的 org scope 内。
// 系统 admin/root 直接放行(看全局)。非系统 admin 用户(wl-admin 等)只能查
// 本组织成员的 user_id。详见 org.md 4.2.2。
func checkUserIdInOrgScope(c *gin.Context, userId int) bool {
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		return true
	}
	for _, id := range scope.UserIdSet {
		if id == userId {
			return true
		}
	}
	return false
}

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
	// 组织数据隔离:wl-admin 等只能查本 org 成员的结算配置(详见 org.md 4.2.2 只读约束)
	if !checkUserIdInOrgScope(c, userId) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权查看该 user_id 的结算配置(不在本组织范围内)",
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

// GetSettlementBillTokenOptions GET /api/settlement/bill/tokens?keyword=
// 账单页 key 筛选下拉框数据源：root 返回所有用户的 token（带所属用户名），
// 普通用户只返回自己的 token。服务端按名称模糊匹配并限量返回。
func GetSettlementBillTokenOptions(c *gin.Context) {
	userId := c.GetInt("id")
	role := c.GetInt("role")
	keyword := c.Query("keyword")

	// root 查全部（queryUserId=0），其他用户只查自己
	queryUserId := userId
	if role >= common.RoleRootUser {
		queryUserId = 0
	}

	opts, err := model.SearchTokensForBill(queryUserId, keyword, 50)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 仅 root 需要区分 key 所属用户，普通用户下拉无需展示自己的用户名
	if role < common.RoleRootUser {
		for _, opt := range opts {
			opt.Username = ""
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    opts,
	})
}

// resolveSelfBillScope 解析账单自助接口的 token_id 参数，返回用于统计的有效 userId 与 tokenId。
//   - 未传 token_id：返回当前登录用户、tokenId=0（统计本人全部 key）。
//   - 传了 token_id：有效 userId 取该 token 的拥有者。非 root 用户只能选自己的 key，
//     否则返回 403；root 可选任意用户的 key 以跨用户审查其消耗（价格用 key 拥有者配置）。
//
// 第二个返回值为 false 时表示已写出错误响应，调用方应直接 return。
func resolveSelfBillScope(c *gin.Context) (int, int, bool) {
	currentUserId := c.GetInt("id")
	tokenIdStr := c.Query("token_id")
	if tokenIdStr == "" {
		return currentUserId, 0, true
	}
	tokenId, err := strconv.Atoi(tokenIdStr)
	if err != nil || tokenId <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "token_id 参数格式错误",
		})
		return 0, 0, false
	}
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "指定的 key 不存在",
		})
		return 0, 0, false
	}
	if c.GetInt("role") < common.RoleRootUser && token.UserId != currentUserId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权查看该 key 的账单",
		})
		return 0, 0, false
	}
	return token.UserId, tokenId, true
}

// GetSelfSettlementBill GET /api/settlement/bill/self?start_timestamp=&end_timestamp=&token_id=&expand_date=
// Get current user's settlement bill. Forces use of logged-in user ID for data isolation.
// token_id 可选（按 key 过滤，root 可跨用户）；expand_date=true 时按日期(东八区)展开。
func GetSelfSettlementBill(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}
	effectiveUserId, tokenId, ok := resolveSelfBillScope(c)
	if !ok {
		return
	}
	expandDate := c.Query("expand_date") == "true" || c.Query("expand_date") == "1"
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
	bill, err := service.CalculateSettlementBill(effectiveUserId, startTimestamp, endTimestamp, tokenId, expandDate)
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

// SelfExportSettlementBillCSV GET /api/settlement/bill/self/export?start_timestamp=&end_timestamp=&token_id=&expand_date=
// Export current user's settlement bill as CSV. 与查询接口保持完全一致的筛选口径。
func SelfExportSettlementBillCSV(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}
	effectiveUserId, tokenId, ok := resolveSelfBillScope(c)
	if !ok {
		return
	}
	expandDate := c.Query("expand_date") == "true" || c.Query("expand_date") == "1"
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
	if err := service.ValidateTimeRange(startTimestamp, endTimestamp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	bill, err := service.CalculateSettlementBill(effectiveUserId, startTimestamp, endTimestamp, tokenId, expandDate)
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

// ========== Task 4.3: Admin bill query endpoints ==========

// AdminGetSettlementBill GET /api/settlement/bill/admin?user_id=&start_timestamp=&end_timestamp=
// Admin endpoint to query any user's settlement bill.
// 组织数据隔离:wl-admin 等非系统 admin 只能查本 org 成员的账单(详见 org.md 4.2.2)。
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
	if !checkUserIdInOrgScope(c, userId) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权查看该 user_id 的账单(不在本组织范围内)",
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
	bill, err := service.CalculateSettlementBill(userId, startTimestamp, endTimestamp, 0, false)
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
// 组织数据隔离:wl-admin 等只能导出本 org 成员的账单(详见 org.md 4.2.2)。
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
	if !checkUserIdInOrgScope(c, userId) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权导出该 user_id 的账单(不在本组织范围内)",
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
	bill, err := service.CalculateSettlementBill(userId, startTimestamp, endTimestamp, 0, false)
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
