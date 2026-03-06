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

// GetProjects handles GET /api/projects - retrieves paginated list of projects
func GetProjects(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := c.Query("keyword")
	projects, total, err := model.GetProjectListWithTotal(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Enrich each project with allocations, allocated_total and used_total
	type AllocationInfo struct {
		ClientUserId   string `json:"client_user_id"`
		AllocatedQuota int    `json:"allocated_quota"`
		UsedQuota      int    `json:"used_quota"`
	}
	type ProjectWithBudget struct {
		*model.Project
		AllocatedTotal int              `json:"allocated_total"`
		UsedTotal      int              `json:"used_total"`
		Allocations    []AllocationInfo `json:"allocations"`
		Quota          int64            `json:"quota"`
	}
	enriched := make([]ProjectWithBudget, 0, len(projects))
	for _, p := range projects {
		allocated, _ := model.GetProjectAllocatedTotal(p.Id)
		used, _ := model.GetProjectUsedTotal(p.Id)
		quota, _ := model.GetQuotaByProjectName(p.ProjectName)
		// Fetch all allocations for this project
		allocs, _ := model.GetAllocationsByProjectId(p.Id, 0, 1000)
		allocInfos := make([]AllocationInfo, 0, len(allocs))
		for _, a := range allocs {
			allocInfos = append(allocInfos, AllocationInfo{
				ClientUserId:   a.ClientUserId,
				AllocatedQuota: a.AllocatedQuota,
				UsedQuota:      a.UsedQuota,
			})
		}
		enriched = append(enriched, ProjectWithBudget{
			Project:        p,
			Quota:          quota, // 这个项目已消耗的总额
			AllocatedTotal: allocated,
			UsedTotal:      used,
			Allocations:    allocInfos,
		})
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(enriched)
	common.ApiSuccess(c, pageInfo)
}

// CreateProject handles POST /api/project - creates a new project
func CreateProject(c *gin.Context) {
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
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

// GetProjectAllocations handles GET /api/project/:id/allocations - retrieves allocations for a project
func GetProjectAllocations(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}

	// Check if project exists
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid project id")
		return
	}

	var req dto.AllocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Check if project exists
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

	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	clientUserId := c.Query("client_user_id")
	scenario := c.Query("scenario")

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
