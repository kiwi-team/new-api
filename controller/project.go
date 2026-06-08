package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// checkClientUserInOrgScope 校验 client_user_id 是否在当前用户的 org scope 内。
// 系统 admin/root 直接放行(看全局)。
// 非 admin 且 client_user_id 不在 scope.UidSet 时返回 false——controller 应当 403。
// 详见 org.md (mt-admin 等组织角色只能操作本 org 的 uid)。
func checkClientUserInOrgScope(c *gin.Context, clientUserId string) bool {
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		return true // 系统 admin/root bypass
	}
	for _, u := range scope.UidSet {
		if u == clientUserId {
			return true
		}
	}
	return false
}

// projectInOrgScope 校验项目是否"属于"当前用户的 org scope:
//   - 系统 admin/root 直接放行
//   - 非 admin:项目必须至少有一条 allocation 命中 scope.UidSet 才算属于本 org;
//     空项目(没有任何 allocation)对组织 admin 不可见,只有系统 admin 能管。
//
// 这是按 org.md 既定原则(不加 projects.org_code 列)的妥协做法:
// 用 ProjectAllocation 反查归属。
func projectInOrgScope(c *gin.Context, projectId int) bool {
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		return true
	}
	if len(scope.UidSet) == 0 {
		return false
	}
	var count int64
	if err := model.DB.Model(&model.ProjectAllocation{}).
		Where("project_id = ? AND client_user_id IN ?", projectId, scope.UidSet).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// planInOrgScope 校验 plan 是否属于本 org(通过 plan -> project -> allocations 反查)。
func planInOrgScope(c *gin.Context, planId int) bool {
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		return true
	}
	plan, err := model.GetAllocationPlanById(planId)
	if err != nil || plan == nil {
		return false
	}
	return projectInOrgScope(c, plan.ProjectId)
}

// allocationInOrgScope 校验 allocation 是否属于本 org(直接看其 client_user_id)。
func allocationInOrgScope(c *gin.Context, allocationId int) bool {
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		return true
	}
	var alloc model.ProjectAllocation
	if err := model.DB.First(&alloc, allocationId).Error; err != nil {
		return false
	}
	return checkClientUserInOrgScope(c, alloc.ClientUserId)
}

// forbidIfOutOfScope 是 controller 入口的通用 403 兜底
//
// 返回值约定:
//   - 返回 true  → 已经写了 403 response,caller 应当立即 return
//   - 返回 false → 资源在 scope 内,caller 继续往下走
//
// 当前为了让 mt-admin 拥有项目预算管理全部权限,**整个 scope 检查暂时关闭**——直接返回
// false 表示永远不拦。如果未来需要重新启用 org 隔离,把下面 return false 那行删掉、
// 取消下面真正逻辑的注释即可。
func forbidIfOutOfScope(c *gin.Context, ok bool) bool {
	return false
	// ---- 下面是原 scope 检查逻辑,暂停启用 ----
	// if ok {
	// 	return false
	// }
	// c.JSON(http.StatusForbidden, gin.H{
	// 	"success": false,
	// 	"message": "该资源不在你所属组织的范围内",
	// })
	// return true
}

// 注:forbidIfReadOnlyOrgManagement 在 controller/cliend_user_quota.go 里定义,同 package 复用。
// mt-leader 等"只读组织管理页面"角色调写接口时 403。系统 admin / mt-admin 等通过。

// requireSystemAdmin 要求当前用户必须是系统 admin/root,否则 403 中止。
// 项目预算管理里所有写操作(创建项目/方案/预算)和 dashboard 统计接口都用它锁起来。
// 组织 admin(mt-admin 等)在该页面**只读**:能看自己 org 范围内的项目+预算,但不能创建/编辑/删除。
// 详见 org.md。
func requireSystemAdmin(c *gin.Context) bool {
	if c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "无权进行此操作，仅系统管理员可执行",
	})
	return false
}

// GetProjects handles GET /api/projects - retrieves paginated list of projects
func GetProjects(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := c.Query("keyword")

	var (
		projects []*model.Project
		total    int64
		err      error
	)
	scope, _ := service.CurrentUserOrgScope(c.GetInt("id"), c.GetInt("role"))
	if scope == nil {
		// 系统 admin/root:看全局
		projects, total, err = model.GetProjectListWithTotal(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), keyword)
	} else if len(scope.UidSet) == 0 {
		// 组织成员但本 org 没人配 uid:列表为空
		projects, total, err = nil, 0, nil
	} else {
		// 组织 admin(mt-admin 等):只列出至少有一条 allocation 命中 scope uid 的项目
		q := model.DB.Model(&model.Project{})
		//Where("id IN (SELECT DISTINCT project_id FROM project_allocations WHERE client_user_id IN ?)", scope.UidSet)
		if keyword != "" {
			q = q.Where("project_name LIKE ?", "%"+keyword+"%")
		}
		if err = q.Count(&total).Error; err == nil {
			err = q.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&projects).Error
		}
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Enrich each project with plan-grouped allocations
	type AllocationInfo struct {
		ClientUserId   string `json:"client_user_id"`
		AllocatedQuota int    `json:"allocated_quota"`
		UsedQuota      int    `json:"used_quota"`
	}
	type PlanAllocationInfo struct {
		PlanId      int              `json:"plan_id"`
		PlanName    string           `json:"plan_name"`
		StartDate   string           `json:"start_date"`
		EndDate     string           `json:"end_date"`
		IsActive    bool             `json:"is_active"`
		Allocations []AllocationInfo `json:"allocations"`
	}
	type ProjectWithBudget struct {
		*model.Project
		AllocatedTotal int                  `json:"allocated_total"`
		UsedTotal      int                  `json:"used_total"`
		Plans          []PlanAllocationInfo `json:"plans"`
		Quota          int64                `json:"quota"`
	}
	enriched := make([]ProjectWithBudget, 0, len(projects))
	for _, p := range projects {
		allocated, _ := model.GetProjectAllocatedTotal(p.Id)
		used, _ := model.GetProjectUsedTotal(p.Id)
		quota, _ := model.GetQuotaByProjectName(p.ProjectName)

		// Fetch plans for this project
		plans, _ := model.GetAllocationPlansByProjectId(p.Id)
		planInfos := make([]PlanAllocationInfo, 0, len(plans))
		for _, plan := range plans {
			allocs, _ := model.GetAllocationsByPlanId(plan.Id, 0, 1000)
			allocInfos := make([]AllocationInfo, 0, len(allocs))
			for _, a := range allocs {
				allocInfos = append(allocInfos, AllocationInfo{
					ClientUserId:   a.ClientUserId,
					AllocatedQuota: a.AllocatedQuota,
					UsedQuota:      a.UsedQuota,
				})
			}
			planInfos = append(planInfos, PlanAllocationInfo{
				PlanId:      plan.Id,
				PlanName:    plan.PlanName,
				StartDate:   plan.StartDate,
				EndDate:     plan.EndDate,
				IsActive:    p.ActivePlanId == plan.Id,
				Allocations: allocInfos,
			})
		}

		enriched = append(enriched, ProjectWithBudget{
			Project:        p,
			Quota:          quota,
			AllocatedTotal: allocated,
			UsedTotal:      used,
			Plans:          planInfos,
		})
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(enriched)
	common.ApiSuccess(c, pageInfo)
}

// CreateProject handles POST /api/project - creates a new project
// 组织 admin(mt-admin)也能新建项目;新建后需 immediately 配 allocation 给本 org 成员,
// 否则空项目对自己不可见(GetProjects 按 allocation 反查归属)。
// mt-leader 是只读,被 forbidIfReadOnlyOrgManagement 拦掉。
func CreateProject(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	var req dto.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate project name is not empty
	if req.ProjectName == "" {
		common.ApiErrorMsg(c, "invalid project name")
		return
	}

	// Check if project name already exists
	existingProject, _ := model.GetProjectByName(req.ProjectName)
	if existingProject != nil && existingProject.Id > 0 {
		common.ApiErrorMsg(c, "project name already exists")
		return
	}

	project := &model.Project{
		ProjectName: req.ProjectName,
		TotalBudget: req.TotalBudget,
	}

	err := model.CreateProject(project)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    project,
	})
}

// UpdateProject handles PUT /api/project/:id - updates project information
func UpdateProject(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	var req dto.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Get existing project
	project, err := model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	// If updating total_budget, validate it's >= sum of all allocations
	if req.TotalBudget > 0 || req.TotalBudget == 0 {
		allocatedTotal, err := model.GetProjectAllocatedTotal(id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if req.TotalBudget < allocatedTotal {
			common.ApiErrorMsg(c, "budget reduction not allowed")
			return
		}
		project.TotalBudget = req.TotalBudget
	}

	// Update project name if provided
	if req.ProjectName != "" {
		// Check if new name conflicts with existing project
		if req.ProjectName != project.ProjectName {
			existingProject, _ := model.GetProjectByName(req.ProjectName)
			if existingProject != nil && existingProject.Id > 0 {
				common.ApiErrorMsg(c, "project name already exists")
				return
			}
		}
		project.ProjectName = req.ProjectName
	}

	err = model.UpdateProject(project)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    project,
	})
}

// UpdateProjectStatus handles PUT /api/project/:id/status - updates project status
func UpdateProjectStatus(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	var req dto.UpdateProjectStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate status value
	if req.Status != model.ProjectStatusEnabled && req.Status != model.ProjectStatusPaused {
		common.ApiErrorMsg(c, "invalid project status")
		return
	}

	// Check if project exists
	_, err = model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	err = model.UpdateProjectStatus(id, req.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// SetActivePlan handles PUT /api/project/:id/active-plan - sets the active plan for a project
func SetActivePlan(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	var req struct {
		PlanId int `json:"plan_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	_, err = model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	// plan_id=0 means deactivate
	if req.PlanId > 0 {
		plan, err := model.GetAllocationPlanById(req.PlanId)
		if err != nil {
			common.ApiErrorMsg(c, "plan not found")
			return
		}
		if plan.ProjectId != id {
			common.ApiErrorMsg(c, "plan does not belong to this project")
			return
		}
		if plan.IsExpired() {
			common.ApiErrorMsg(c, "cannot activate an expired plan")
			return
		}
	}

	err = model.SetActivePlanId(id, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ==================== Allocation Plan Endpoints ====================

// GetProjectPlans handles GET /api/project/:id/plans - retrieves allocation plans for a project
func GetProjectPlans(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	project, err := model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	plans, err := model.GetAllocationPlansByProjectId(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Enrich plans with allocation details
	type AllocationInfo struct {
		ClientUserId   string `json:"client_user_id"`
		AllocatedQuota int    `json:"allocated_quota"`
		UsedQuota      int    `json:"used_quota"`
	}
	type PlanWithStats struct {
		*model.ProjectAllocationPlan
		AllocationCount int              `json:"allocation_count"`
		AllocatedTotal  int              `json:"allocated_total"`
		IsActive        bool             `json:"is_active"`
		IsExpired       bool             `json:"is_expired"`
		Allocations     []AllocationInfo `json:"allocations"`
	}
	enriched := make([]PlanWithStats, 0, len(plans))
	for _, p := range plans {
		allocs, _ := model.GetAllocationsByPlanId(p.Id, 0, 1000)
		allocInfos := make([]AllocationInfo, 0, len(allocs))
		allocatedTotal := 0
		for _, a := range allocs {
			allocatedTotal += a.AllocatedQuota
			allocInfos = append(allocInfos, AllocationInfo{
				ClientUserId:   a.ClientUserId,
				AllocatedQuota: a.AllocatedQuota,
				UsedQuota:      a.UsedQuota,
			})
		}
		enriched = append(enriched, PlanWithStats{
			ProjectAllocationPlan: p,
			AllocationCount:       len(allocs),
			AllocatedTotal:        allocatedTotal,
			IsActive:              project.ActivePlanId == p.Id,
			IsExpired:             p.IsExpired(),
			Allocations:           allocInfos,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    enriched,
	})
}

// CreateProjectPlan handles POST /api/project/:id/plan - creates a new allocation plan
func CreateProjectPlan(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	var req dto.CreateAllocationPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	_, err = model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	plan := &model.ProjectAllocationPlan{
		ProjectId: id,
		PlanName:  req.PlanName,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	err = model.CreateAllocationPlan(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

// UpdateProjectPlan handles PUT /api/project/plan/:planId - updates a plan
func UpdateProjectPlan(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	planId, err := strconv.Atoi(c.Param("planId"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid plan id")
		return
	}
	if forbidIfOutOfScope(c, planInOrgScope(c, planId)) {
		return
	}

	var req dto.UpdateAllocationPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	plan, err := model.GetAllocationPlanById(planId)
	if err != nil {
		common.ApiErrorMsg(c, "plan not found")
		return
	}

	if req.PlanName != "" {
		plan.PlanName = req.PlanName
	}
	if req.StartDate != "" {
		plan.StartDate = req.StartDate
	}
	if req.EndDate != "" {
		plan.EndDate = req.EndDate
	}

	err = model.UpdateAllocationPlan(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

// DeleteProjectPlan handles DELETE /api/project/plan/:planId - deletes a plan and its allocations
func DeleteProjectPlan(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	planId, err := strconv.Atoi(c.Param("planId"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid plan id")
		return
	}
	if forbidIfOutOfScope(c, planInOrgScope(c, planId)) {
		return
	}

	_, err = model.GetAllocationPlanById(planId)
	if err != nil {
		common.ApiErrorMsg(c, "plan not found")
		return
	}

	err = model.DeleteAllocationPlan(planId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ==================== Allocation Endpoints (Plan-scoped) ====================

// GetPlanAllocations handles GET /api/project/plan/:planId/allocations - retrieves allocations for a plan
func GetPlanAllocations(c *gin.Context) {
	planId, err := strconv.Atoi(c.Param("planId"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid plan id")
		return
	}
	if forbidIfOutOfScope(c, planInOrgScope(c, planId)) {
		return
	}

	_, err = model.GetAllocationPlanById(planId)
	if err != nil {
		common.ApiErrorMsg(c, "plan not found")
		return
	}

	pageInfo := common.GetPageQuery(c)

	allocations, total, err := model.GetAllocationsByPlanIdWithTotal(planId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(allocations)
	common.ApiSuccess(c, pageInfo)
}

// CreateOrUpdatePlanAllocation handles POST /api/project/plan/:planId/allocation
func CreateOrUpdatePlanAllocation(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	planId, err := strconv.Atoi(c.Param("planId"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid plan id")
		return
	}

	var req dto.AllocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 注:原先有 checkClientUserInOrgScope 校验 client_user_id 必须在本 org scope 内,
	// 已去掉——业务场景是 admin 经常提前给"尚未注册"的 uid 配预算,严格校验会阻塞这个流程。
	// 副作用:admin 输错 uid(比如填了别 org 的 uid)时不会被前端拦截,只能事后纠正。

	plan, err := model.GetAllocationPlanById(planId)
	if err != nil {
		common.ApiErrorMsg(c, "plan not found")
		return
	}

	// Validate allocation doesn't exceed project budget
	err = service.ValidateAllocationForPlan(plan.ProjectId, planId, req.ClientUserId, req.AllocatedQuota)
	if err != nil {
		if err == service.ErrAllocationExceedsBudget {
			common.ApiErrorMsg(c, "allocation exceeds project budget")
			return
		}
		common.ApiError(c, err)
		return
	}

	allocation := &model.ProjectAllocation{
		ProjectId:      plan.ProjectId,
		PlanId:         planId,
		ClientUserId:   req.ClientUserId,
		AllocatedQuota: req.AllocatedQuota,
	}

	err = model.CreateOrUpdateAllocation(allocation)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    allocation,
	})
}

// ClearAllocationBudget handles POST /api/project/allocation/:allocationId/clear - clears remaining budget
func ClearAllocationBudget(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	allocationId, err := strconv.Atoi(c.Param("allocationId"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid allocation id")
		return
	}
	if forbidIfOutOfScope(c, allocationInOrgScope(c, allocationId)) {
		return
	}

	err = model.ClearAllocationUsedQuota(allocationId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ==================== Legacy Allocation Endpoints (kept for backward compatibility) ====================

// GetProjectAllocations handles GET /api/project/:id/allocations - retrieves allocations for a project
func GetProjectAllocations(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	_, err = model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	pageInfo := common.GetPageQuery(c)

	allocations, total, err := model.GetAllocationsByProjectIdWithTotal(id, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(allocations)
	common.ApiSuccess(c, pageInfo)
}

// CreateOrUpdateAllocation handles POST /api/project/:id/allocation - creates or updates a budget allocation
func CreateOrUpdateAllocation(c *gin.Context) {
	if forbidIfReadOnlyOrgManagement(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	var req dto.AllocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 注:原先有 checkClientUserInOrgScope,允许 pre-registration 创建,理由同 CreateOrUpdatePlanAllocation。

	_, err = model.GetProjectById(id)
	if err != nil {
		common.ApiErrorMsg(c, "project not found")
		return
	}

	// Validate allocation doesn't exceed project budget
	err = service.ValidateAllocationForUser(id, req.ClientUserId, req.AllocatedQuota)
	if err != nil {
		if err == service.ErrAllocationExceedsBudget {
			common.ApiErrorMsg(c, "allocation exceeds project budget")
			return
		}
		common.ApiError(c, err)
		return
	}

	allocation := &model.ProjectAllocation{
		ProjectId:      id,
		ClientUserId:   req.ClientUserId,
		AllocatedQuota: req.AllocatedQuota,
	}

	err = model.CreateOrUpdateAllocation(allocation)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    allocation,
	})
}

// GetProjectDashboard handles GET /api/project/dashboard - retrieves budget summary dashboard data
// 组织 admin(mt-admin)也能看 dashboard;后端按 client_user_id 参数过滤,前端按需求传递。
// 注意:dashboard 当前是全局聚合,如果需要严格按 org 隔离统计数字,需要再单独改 service 层。
func GetProjectDashboard(c *gin.Context) {
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	projectName := c.Query("project_name")
	clientUserId := c.Query("client_user_id")

	dashboard, err := service.GetProjectDashboard(startTime, endTime, projectName, clientUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, dashboard)
}

// GetProjectStatistics handles GET /api/project/:id/statistics - retrieves project consumption statistics
func GetProjectStatistics(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}
	if forbidIfOutOfScope(c, projectInOrgScope(c, id)) {
		return
	}

	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	clientUserId := c.Query("client_user_id")
	scenario := c.Query("scenario")
	// 组织 admin 即便项目在 scope 内,client_user_id 查询参数也必须在 scope 内
	if clientUserId != "" && !checkClientUserInOrgScope(c, clientUserId) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "client_user_id 不在你所属组织的范围内",
		})
		return
	}

	statistics, err := service.GetProjectStatistics(id, startTime, endTime, clientUserId, scenario)
	if err != nil {
		if err.Error() == "project id is empty" || err.Error() == "record not found" {
			common.ApiErrorMsg(c, "project not found")
			return
		}
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, statistics)
}
