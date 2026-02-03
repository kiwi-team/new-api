package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type CuQuotaUpsertRequest struct {
	ClientUserId string `json:"client_user_id" binding:"required"`
	ClientName   string `json:"client_name"`
	FixedQuota   int    `json:"fixed_quota"`
	TempQuota    int    `json:"temp_quota"`
	Remark       string `json:"remark"`
	ExpiredAt    int64  `json:"expired_at"`
}

func GetAllCliendUserQuota(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var rows []*model.CliendUserQuota
	var total int64
	err := model.DB.Model(&model.CliendUserQuota{}).Count(&total).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = model.DB.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func SearchCliendUserQuota(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	var rows []*model.CliendUserQuota
	var total int64
	query := model.DB.Model(&model.CliendUserQuota{})
	if keyword != "" {
		query = query.Where("client_user_id LIKE ?", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func GetCliendUserQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var row model.CliendUserQuota
	if err := model.DB.First(&row, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    row,
	})
}

func CreateCliendUserQuota(c *gin.Context) {
	var req CuQuotaUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ClientUserId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	row := model.CliendUserQuota{
		ClientUserId: req.ClientUserId,
		ClientName:   req.ClientName,
		FixedQuota:   req.FixedQuota,
		TempQuota:    req.TempQuota,
		UpdatedAt:    time.Now().Unix(),
	}
	if req.ExpiredAt == 0 {
		loc, _ := time.LoadLocation("Asia/Shanghai")
		now := time.Now().In(loc)
		firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
		endOfMonth := firstOfNextMonth.Add(-time.Second)
		row.ExpiredAt = endOfMonth.Unix()
	} else {
		row.ExpiredAt = req.ExpiredAt
	}
	if err := model.DB.Create(&row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	log := model.CliendUserQuotaLog{
		ClientUserId: req.ClientUserId,
		AdminUserId:  c.GetInt("id"),
		Action:       "create",
		OldFixed:     0,
		NewFixed:     req.FixedQuota,
		OldTemp:      0,
		NewTemp:      req.TempQuota,
		Remark:       req.Remark,
		CreatedAt:    time.Now().Unix(),
	}
	_ = model.DB.Create(&log).Error
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateCliendUserQuota(c *gin.Context) {
	var req CuQuotaUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ClientUserId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	var cur model.CliendUserQuota
	if err := model.DB.First(&cur, "client_user_id = ?", req.ClientUserId).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	updates := map[string]interface{}{
		"client_name": req.ClientName,
		"fixed_quota": req.FixedQuota,
		"temp_quota":  req.TempQuota,
		"updated_at":  time.Now().Unix(),
	}
	if req.ExpiredAt != 0 {
		updates["expired_at"] = req.ExpiredAt
	}
	if err := model.DB.Model(&model.CliendUserQuota{}).Where("client_user_id = ?", req.ClientUserId).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	log := model.CliendUserQuotaLog{
		ClientUserId: req.ClientUserId,
		AdminUserId:  c.GetInt("id"),
		Action:       "update",
		OldFixed:     cur.FixedQuota,
		NewFixed:     req.FixedQuota,
		OldTemp:      cur.TempQuota,
		NewTemp:      req.TempQuota,
		Remark:       req.Remark,
		CreatedAt:    time.Now().Unix(),
	}
	_ = model.DB.Create(&log).Error
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteCliendUserQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var cur model.CliendUserQuota
	if err := model.DB.First(&cur, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Delete(&model.CliendUserQuota{}, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	log := model.CliendUserQuotaLog{
		ClientUserId: cur.ClientUserId,
		AdminUserId:  c.GetInt("id"),
		Action:       "delete",
		OldFixed:     cur.FixedQuota,
		NewFixed:     cur.FixedQuota,
		OldTemp:      cur.TempQuota,
		NewTemp:      cur.TempQuota,
		Remark:       "",
		CreatedAt:    time.Now().Unix(),
	}
	_ = model.DB.Create(&log).Error
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func GetCliendUserQuotaLogs(c *gin.Context) {
	clientUserId := c.Query("client_user_id")
	pageInfo := common.GetPageQuery(c)
	var rows []*model.CliendUserQuotaLog
	var total int64
	query := model.DB.Model(&model.CliendUserQuotaLog{})
	if clientUserId != "" {
		query = query.Where("client_user_id = ?", clientUserId)
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}
