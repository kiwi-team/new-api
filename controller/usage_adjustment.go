package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createUsageAdjustmentRequest struct {
	Date                         string   `json:"date"`
	TokenId                      int      `json:"token_id"`
	ModelName                    string   `json:"model_name"`
	CorrectPromptTokens          *int64   `json:"correct_input_tokens"`
	CorrectCompletionTokens      *int64   `json:"correct_output_tokens"`
	CorrectCachedTokens          *int64   `json:"correct_cache_read_tokens"`
	CorrectClaudeCacheCreation5m *int64   `json:"correct_cache_write_5m_tokens"`
	CorrectClaudeCacheCreation1h *int64   `json:"correct_cache_write_1h_tokens"`
	CorrectCostUsd               *float64 `json:"correct_cost_usd"`
	Reason                       string   `json:"reason"`
	Ticket                       string   `json:"ticket"`
}

type revertUsageAdjustmentRequest struct {
	Reason string `json:"reason"`
}

func GetUsageDayAdjustmentSnapshot(c *gin.Context) {
	tokenId, err := strconv.Atoi(c.Query("token_id"))
	if err != nil {
		usageAdjustmentError(c, http.StatusBadRequest, errors.New("token_id parameter is invalid"))
		return
	}
	snapshot, err := model.GetUsageDaySnapshot(c.Query("date"), tokenId, c.Query("model_name"))
	if err != nil {
		usageAdjustmentError(c, usageAdjustmentStatus(err), err)
		return
	}
	if !canManageUsageAdjustment(c) {
		usageAdjustmentError(c, http.StatusForbidden, errors.New("not authorized to adjust this usage record"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": snapshot})
}

func CreateUsageAdjustment(c *gin.Context) {
	var req createUsageAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		usageAdjustmentError(c, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if req.CorrectPromptTokens == nil || req.CorrectCompletionTokens == nil || req.CorrectCachedTokens == nil ||
		req.CorrectClaudeCacheCreation5m == nil || req.CorrectClaudeCacheCreation1h == nil || req.CorrectCostUsd == nil {
		usageAdjustmentError(c, http.StatusBadRequest, errors.New("all corrected usage values are required"))
		return
	}
	snapshot, err := model.GetUsageDaySnapshot(req.Date, req.TokenId, req.ModelName)
	if err != nil {
		usageAdjustmentError(c, usageAdjustmentStatus(err), err)
		return
	}
	if !canManageUsageAdjustment(c) {
		usageAdjustmentError(c, http.StatusForbidden, errors.New("not authorized to adjust this usage record"))
		return
	}

	adjustment, err := model.CreateUsageAdjustment(
		req.Date,
		req.TokenId,
		req.ModelName,
		model.UsageCorrectionTarget{
			PromptTokens:                *req.CorrectPromptTokens,
			CompletionTokens:            *req.CorrectCompletionTokens,
			CachedTokens:                *req.CorrectCachedTokens,
			ClaudeCacheCreation5mTokens: *req.CorrectClaudeCacheCreation5m,
			ClaudeCacheCreation1hTokens: *req.CorrectClaudeCacheCreation1h,
			CostUsd:                     *req.CorrectCostUsd,
		},
		req.Reason,
		req.Ticket,
		c.GetInt("id"),
		c.GetString("username"),
	)
	if err != nil {
		usageAdjustmentError(c, usageAdjustmentStatus(err), err)
		return
	}
	recordManageAuditFor(c, snapshot.UserId, "usage.adjustment_create", map[string]interface{}{
		"adjustment_id": adjustment.Id,
		"date":          snapshot.Date,
		"token_id":      adjustment.TokenId,
		"model_name":    adjustment.ModelName,
		"quota_delta":   adjustment.QuotaDelta,
		"reason":        adjustment.Reason,
		"ticket":        adjustment.Ticket,
	})
	updated, err := model.GetUsageDaySnapshot(req.Date, req.TokenId, req.ModelName)
	if err != nil {
		usageAdjustmentError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": updated})
}

func RevertUsageAdjustment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		usageAdjustmentError(c, http.StatusBadRequest, errors.New("adjustment id is invalid"))
		return
	}
	var req revertUsageAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		usageAdjustmentError(c, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	adjustment, err := model.GetUsageAdjustmentById(id)
	if err != nil {
		usageAdjustmentError(c, usageAdjustmentStatus(err), err)
		return
	}
	if !canManageUsageAdjustment(c) {
		usageAdjustmentError(c, http.StatusForbidden, errors.New("not authorized to revert this adjustment"))
		return
	}
	if err := model.RevertUsageAdjustment(id, c.GetInt("id"), c.GetString("username"), req.Reason); err != nil {
		usageAdjustmentError(c, usageAdjustmentStatus(err), err)
		return
	}
	recordManageAuditFor(c, adjustment.UserId, "usage.adjustment_revert", map[string]interface{}{
		"adjustment_id": id,
		"date":          time.Unix(adjustment.HourStart, 0).In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02"),
		"token_id":      adjustment.TokenId,
		"model_name":    adjustment.ModelName,
		"reason":        req.Reason,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func canManageUsageAdjustment(c *gin.Context) bool {
	return c.GetInt("role") >= common.RoleAdminUser
}

func usageAdjustmentStatus(err error) int {
	if errors.Is(err, model.ErrUsageDayNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func usageAdjustmentError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}
