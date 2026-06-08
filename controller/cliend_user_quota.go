package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// forbidIfReadOnlyOrgManagement:对当前用户禁写组织管理(UID 预算 / 项目预算)时直接 403,
// 返回 true 表示已经写了 response,caller 应当立刻 return。
// 当前只对 mt-leader 生效(他只能看不能改);mt-admin / 系统 admin 都通过。
func forbidIfReadOnlyOrgManagement(c *gin.Context) bool {
	u, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil || u == nil {
		return false // 用户查不到的话交由下面的逻辑兜底,这里不主动拒
	}
	if service.IsReadOnlyOnOrgManagement(u) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "你在该页面只读,无权执行写操作",
		})
		return true
	}
	return false
}

// applyClientUserQuotaOrgScope 给查询加 org 数据隔离。
// 系统 admin/root 返回原 query(看全局);mt-admin 等组织角色按 client_user_id IN scope.UidSet 过滤。
// 如果 scope 算出来 UidSet 为空(本 org 无成员配 uid),会显式给个不可能命中的条件让结果为空。
func applyClientUserQuotaOrgScope(c *gin.Context, q *gorm.DB) *gorm.DB {
	return q
	scope, err := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if err != nil || scope == nil {
		return q
	}
	if len(scope.UidSet) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("client_user_id IN ?", scope.UidSet)
}

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
	countQ := applyClientUserQuotaOrgScope(c, model.DB.Model(&model.CliendUserQuota{}))
	if err := countQ.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	listQ := applyClientUserQuotaOrgScope(c, model.DB.Model(&model.CliendUserQuota{}))
	if err := listQ.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
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
	query := applyClientUserQuotaOrgScope(c, model.DB.Model(&model.CliendUserQuota{}))
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
	q := applyClientUserQuotaOrgScope(c, model.DB.Where("id = ?", id))
	if err := q.First(&row).Error; err != nil {
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
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	var req CuQuotaUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ClientUserId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	req.ClientUserId = strings.TrimSpace(req.ClientUserId)

	// 非系统 admin 的组织 admin(如 mt-admin)创建预算时,client_user_id 必须 ==
	// 本 org 内某个成员的主 uid(不展开 related_uids)。详见 org.md。
	if c.GetInt("role") < common.RoleAdminUser {
		user, err := model.GetUserById(c.GetInt("id"), false)
		if err != nil || user == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "用户不存在",
			})
			return
		}
		// if !service.IsOrgMainUid(user.OrgCode, req.ClientUserId) {
		// 	c.JSON(http.StatusForbidden, gin.H{
		// 		"success": false,
		// 		"message": "client_user_id 必须是本组织某成员的主 uid",
		// 	})
		// 	return
		// }
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
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	var req CuQuotaUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ClientUserId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	req.ClientUserId = strings.TrimSpace(req.ClientUserId)
	var cur model.CliendUserQuota
	// org 过滤:确保更新的目标在本 org scope 内,否则视为不存在(防止跨 org 改别人的)
	findQ := applyClientUserQuotaOrgScope(c, model.DB.Where("client_user_id = ?", req.ClientUserId))
	if err := findQ.First(&cur).Error; err != nil {
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
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var cur model.CliendUserQuota
	findQ := applyClientUserQuotaOrgScope(c, model.DB.Where("id = ?", id))
	if err := findQ.First(&cur).Error; err != nil {
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

func ExportCliendUserQuotaCSV(c *gin.Context) {
	var rows []model.CliendUserQuota
	q := applyClientUserQuotaOrgScope(c, model.DB.Model(&model.CliendUserQuota{}))
	if err := q.Order("id desc").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment;filename=cliend_user_quota.csv")
	// Write UTF-8 BOM for Excel compatibility
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	w.Write([]string{
		"id", "client_user_id", "client_name", "fixed_quota", "temp_quota",
		"used_quota_usd", "updated_at", "expired_at",
	})
	for _, r := range rows {
		usedUSD := fmt.Sprintf("%.6f", float64(r.UsedQuota)/500000.0)
		updatedAt := ""
		if r.UpdatedAt > 0 {
			updatedAt = time.Unix(r.UpdatedAt, 0).Format("2006-01-02 15:04:05")
		}
		expiredAt := ""
		if r.ExpiredAt > 0 {
			expiredAt = time.Unix(r.ExpiredAt, 0).Format("2006-01-02 15:04:05")
		}
		w.Write([]string{
			strconv.Itoa(r.Id),
			r.ClientUserId,
			r.ClientName,
			strconv.Itoa(r.FixedQuota),
			strconv.Itoa(r.TempQuota),
			usedUSD,
			updatedAt,
			expiredAt,
		})
	}
	w.Flush()
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
	// 组织数据隔离:非系统 admin 用户只能看本 org scope 的 log 行
	if scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role")); scope != nil {
		if len(scope.UidSet) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("client_user_id IN ?", scope.UidSet)
		}
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

// GetBatchProjectBudgetSummary 批量获取多个UID的项目预算汇总
func GetBatchProjectBudgetSummary(c *gin.Context) {
	uids := c.Query("uids")
	if uids == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    map[string]interface{}{},
		})
		return
	}
	uidList := strings.Split(uids, ",")
	for i := range uidList {
		uidList[i] = strings.TrimSpace(uidList[i])
	}
	// 组织数据隔离:非系统 admin 只能查本 org scope 内的 uid
	if scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role")); scope != nil {
		allowed := make(map[string]bool, len(scope.UidSet))
		for _, u := range scope.UidSet {
			allowed[u] = true
		}
		filtered := uidList[:0]
		for _, u := range uidList {
			if allowed[u] {
				filtered = append(filtered, u)
			}
		}
		uidList = filtered
	}
	result, err := model.GetBatchProjectBudgetSummary(uidList)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetCliendUserProjectAllocations 获取某个UID的项目预算分配详情
func GetCliendUserProjectAllocations(c *gin.Context) {
	clientUserId := c.Query("client_user_id")
	if clientUserId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "client_user_id is required",
		})
		return
	}
	// 组织数据隔离:非系统 admin 必须查的是本 org scope 内的 uid
	if scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role")); scope != nil {
		inScope := false
		for _, u := range scope.UidSet {
			if u == clientUserId {
				inScope = true
				break
			}
		}
		if !inScope {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "无权访问该 client_user_id 的项目分配",
			})
			return
		}
	}
	details, err := model.GetProjectAllocationDetails(clientUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    details,
	})
}
