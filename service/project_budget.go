package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Error messages for project budget validation
var (
	ErrProjectNotFound         = errors.New("project not found")
	ErrProjectPaused           = errors.New("project is paused")
	ErrUserNotAllocatedProject = errors.New("user not allocated to project")
	ErrProjectQuotaExceeded    = errors.New("project 余额不足，请联系管理员")
	ErrAllocationExceedsBudget = errors.New("allocation exceeds project budget")
	ErrInvalidAllocationQuota  = errors.New("invalid allocation quota")
	ErrNoActivePlan            = errors.New("no active allocation plan for this project")
	ErrActivePlanExpired       = errors.New("active allocation plan has expired")
)

// ValidateProjectRequest validates an API request with a project header.
// It checks:
// 1. Project exists
// 2. Project is not paused
// 3. User has an allocation for the project
// 4. User's used_quota < allocated_quota for that project
//
// Returns the ProjectAllocation if validation passes, or an error with specific message.
// Error messages:
// - "project not found" - project_name does not exist
// - "project is paused" - project status is paused
// - "user not allocated to project" - client_user_id has no allocation for project
// - "project quota exceeded" - user's used_quota >= allocated_quota
func ValidateProjectRequest(projectName string, clientUserId string) (*model.ProjectAllocation, error) {
	if projectName == "" {
		return nil, ErrProjectNotFound
	}
	if clientUserId == "" {
		return nil, ErrUserNotAllocatedProject
	}

	// Step 1: Check if project exists
	project, err := model.GetProjectByName(projectName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	// Step 2: Check if project is paused
	if project.Status == model.ProjectStatusPaused {
		return nil, ErrProjectPaused
	}

	// Step 3: Check if there's an active plan
	allocation, err := model.GetAllocationByProjectAndUser(project.Id, clientUserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotAllocatedProject
		}
		if err.Error() == "no active allocation plan" {
			return nil, ErrNoActivePlan
		}
		if err.Error() == "active allocation plan has expired" {
			return nil, ErrActivePlanExpired
		}
		return nil, err
	}

	// Step 4: Check if user has remaining quota
	if float64(allocation.UsedQuota) >= float64(allocation.AllocatedQuota)*common.QuotaPerUnit {
		return nil, ErrProjectQuotaExceeded
	}

	return allocation, nil
}

// GetUserTotalBudget calculates the total available budget for a user.
// Formula: fixed_quota + temp_quota + sum(allocated_quota - used_quota) for all project allocations
//
// This considers all three budget types:
// - Fixed quota: Monthly reset budget from CliendUserQuota
// - Temp quota: Temporary budget with expiration from CliendUserQuota
// - Project quota: Sum of remaining quota from all project allocations
//
// Returns the total budget in quota units, or an error if calculation fails.
func GetUserTotalBudget(clientUserId string) (int, error) {
	if clientUserId == "" {
		return 0, errors.New("client user id is empty")
	}

	// Get fixed and temp quota from CliendUserQuota
	var clientQuota model.CliendUserQuota
	err := model.DB.Where("client_user_id = ?", clientUserId).First(&clientQuota).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	fixedQuota := clientQuota.FixedQuota
	tempQuota := clientQuota.TempQuota

	// Check if temp quota is expired
	if clientQuota.ExpiredAt > 0 && clientQuota.ExpiredAt <= common.GetTimestamp() {
		tempQuota = 0
	}

	// Get project remaining quota: sum(allocated_quota - used_quota)
	projectRemainingQuota, err := model.GetUserProjectRemainingQuota(clientUserId)
	if err != nil {
		return 0, err
	}

	// Total budget = fixed + temp + project remaining
	totalBudget := fixedQuota + tempQuota + projectRemainingQuota

	return totalBudget, nil
}

// IncreaseProjectUsedQuota atomically increases the used quota for a project allocation.
// It uses database-level atomic operations to prevent race conditions.
//
// Parameters:
// - projectId: The ID of the project
// - clientUserId: The client user ID
// - delta: The amount to increase (must be non-negative)
//
// Returns an error if:
// - The update would cause used_quota to exceed allocated_quota
// - There's a concurrent update conflict
// - The allocation doesn't exist
func IncreaseProjectUsedQuota(projectId int, clientUserId string, delta int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}
	if clientUserId == "" {
		return errors.New("client user id is empty")
	}
	if delta < 0 {
		return errors.New("delta cannot be negative")
	}
	if delta == 0 {
		return nil // No-op for zero delta
	}

	// Use the model function for atomic update
	return model.IncreaseUsedQuotaByProjectAndUser(projectId, clientUserId, delta)
}

// TrackProjectConsumption tracks project consumption after an API request completes.
// It extracts project context from the request context and updates the project allocation's used_quota.
// This function should be called after the API request completes and quota is calculated.
//
// Parameters:
// - ctx: The Gin context containing project information (ContextKeyProjectId, ContextKeyProjectAllocationId)
// - quota: The quota consumed by the request
//
// Returns:
// - projectName: The project name from context (empty if no project)
// - error: Any error that occurred during the update
//
// The function is safe to call even if no project is specified in the request.
// If no project context is found, it returns empty string and nil error.
func TrackProjectConsumption(ctx *gin.Context, quota int) (string, int, error) {
	// Get project context from request
	projectName := common.GetContextKeyString(ctx, constant.ContextKeyProjectName)
	planId := common.GetContextKeyInt(ctx, constant.ContextKeyProjectPlanId)
	if projectName == "" {
		// No project specified, nothing to track
		return "", 0, nil
	}

	// Skip if quota is zero or negative
	if quota <= 0 {
		return projectName, planId, nil
	}

	// Get project ID and client user ID from context
	projectId := common.GetContextKeyInt(ctx, constant.ContextKeyProjectId)
	clientUserId := common.GetContextKeyString(ctx, constant.ContextKeyClientUserId)

	if projectId == 0 || clientUserId == "" {
		// Missing required context, log warning but don't fail
		common.SysLog("TrackProjectConsumption: missing project_id or client_user_id in context")
		return projectName, planId, nil
	}

	// Atomically update the project allocation's used_quota
	err := IncreaseProjectUsedQuota(projectId, clientUserId, quota)
	if err != nil {
		common.SysError("TrackProjectConsumption: failed to update project used quota: " + err.Error())
		return projectName, planId, err
	}

	return projectName, planId, nil
}

// ValidateAllocation validates that a new or updated allocation does not exceed the project's total budget.
// It checks that the sum of all allocations (including the new/updated one) does not exceed total_budget.
//
// Parameters:
// - projectId: The ID of the project
// - newAllocatedQuota: The new allocated quota for the user
// - existingAllocationId: The ID of the existing allocation (0 if creating new)
//
// Returns nil if validation passes, or an error if:
// - The allocation would exceed the project's total budget
// - The allocated quota is negative
// - The project doesn't exist
func ValidateAllocation(projectId int, newAllocatedQuota int, existingAllocationId int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}
	if newAllocatedQuota < 0 {
		return ErrInvalidAllocationQuota
	}

	// Get the project to check total budget
	project, err := model.GetProjectById(projectId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProjectNotFound
		}
		return err
	}

	// Get current total allocated quota for the project
	currentAllocatedTotal, err := model.GetProjectAllocatedTotal(projectId)
	if err != nil {
		return err
	}

	// If updating an existing allocation, subtract its current value
	if existingAllocationId > 0 {
		var existingAllocation model.ProjectAllocation
		err := model.DB.First(&existingAllocation, "id = ?", existingAllocationId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			currentAllocatedTotal -= existingAllocation.AllocatedQuota
		}
	}

	// Check if new allocation would exceed total budget
	if currentAllocatedTotal+newAllocatedQuota > project.TotalBudget {
		return ErrAllocationExceedsBudget
	}

	return nil
}

// ValidateAllocationForUser validates allocation for a specific user within a project.
// This is a convenience function that combines looking up the existing allocation
// and validating the new quota.
//
// Parameters:
// - projectId: The ID of the project
// - clientUserId: The client user ID
// - newAllocatedQuota: The new allocated quota for the user
//
// Returns nil if validation passes, or an error if validation fails.
func ValidateAllocationForUser(projectId int, clientUserId string, newAllocatedQuota int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}
	if clientUserId == "" {
		return errors.New("client user id is empty")
	}

	// Try to get existing allocation
	existingAllocationId := 0
	existingAllocation, err := model.GetAllocationByProjectAndUser(projectId, clientUserId)
	if err == nil {
		existingAllocationId = existingAllocation.Id
	} else if !errors.Is(err, gorm.ErrRecordNotFound) && err.Error() != "no active allocation plan" {
		return err
	}

	return ValidateAllocation(projectId, newAllocatedQuota, existingAllocationId)
}

// ValidateAllocationForPlan validates allocation for a user within a specific plan.
func ValidateAllocationForPlan(projectId int, planId int, clientUserId string, newAllocatedQuota int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}
	if planId == 0 {
		return errors.New("plan id is empty")
	}
	if clientUserId == "" {
		return errors.New("client user id is empty")
	}
	if newAllocatedQuota < 0 {
		return ErrInvalidAllocationQuota
	}

	// Get the project to check total budget
	project, err := model.GetProjectById(projectId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProjectNotFound
		}
		return err
	}

	// Get current total allocated quota for the project (across all plans)
	currentAllocatedTotal, err := model.GetProjectAllocatedTotal(projectId)
	if err != nil {
		return err
	}

	// Subtract existing allocation for this user in this plan if updating
	var existingAllocation model.ProjectAllocation
	err = model.DB.Where("plan_id = ? AND project_id = ? AND client_user_id = ?", planId, projectId, clientUserId).First(&existingAllocation).Error
	if err == nil {
		currentAllocatedTotal -= existingAllocation.AllocatedQuota
	}

	// Check if new allocation would exceed total budget
	if currentAllocatedTotal+newAllocatedQuota > project.TotalBudget {
		return ErrAllocationExceedsBudget
	}

	return nil
}

// CheckProjectQuotaAvailable checks if a user has available quota in a specific project.
// Returns true if the user has remaining quota (used_quota < allocated_quota).
func CheckProjectQuotaAvailable(projectId int, clientUserId string) (bool, error) {
	if projectId == 0 {
		return false, errors.New("project id is empty")
	}
	if clientUserId == "" {
		return false, errors.New("client user id is empty")
	}

	allocation, err := model.GetAllocationByProjectAndUser(projectId, clientUserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return allocation.UsedQuota < allocation.AllocatedQuota, nil
}

// GetProjectRemainingQuota returns the remaining quota for a user in a specific project.
// Returns (allocated_quota - used_quota) or 0 if no allocation exists.
func GetProjectRemainingQuota(projectId int, clientUserId string) (int, error) {
	if projectId == 0 {
		return 0, errors.New("project id is empty")
	}
	if clientUserId == "" {
		return 0, errors.New("client user id is empty")
	}

	allocation, err := model.GetAllocationByProjectAndUser(projectId, clientUserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}

	remaining := allocation.AllocatedQuota - allocation.UsedQuota
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// ==================== Dashboard Statistics Types ====================

// ProjectDashboardResponse represents the budget summary dashboard data
type ProjectDashboardResponse struct {
	HistoricalConsumption int64            `json:"historical_consumption"` // Total consumption in time range
	CurrentMonthFixed     int              `json:"current_month_fixed"`    // Current month fixed quota total
	ValidTempQuota        int              `json:"valid_temp_quota"`       // Valid (non-expired) temp quota total
	ProjectConsumption    int              `json:"project_consumption"`    // Current month project consumption total
	MaxRemainingBudget    int              `json:"max_remaining_budget"`   // Sum of (allocated - used) across all projects
	Projects              []ProjectSummary `json:"projects"`               // List of project summaries
}

// ProjectSummary represents summary data for a single project
type ProjectSummary struct {
	Id             int    `json:"id"`
	ProjectName    string `json:"project_name"`
	TotalBudget    int    `json:"total_budget"`
	AllocatedTotal int    `json:"allocated_total"` // Sum of all allocated quotas
	UsedTotal      int    `json:"used_total"`      // Sum of all used quotas
	Status         int    `json:"status"`
}

// ProjectStatisticsResponse represents consumption statistics for a project
type ProjectStatisticsResponse struct {
	ProjectId             int    `json:"project_id"`
	ProjectName           string `json:"project_name"`
	TotalBudget           int    `json:"total_budget"`
	AllocatedTotal        int    `json:"allocated_total"`
	UsedTotal             int    `json:"used_total"`
	RemainingBudget       int    `json:"remaining_budget"`
	HistoricalConsumption int64  `json:"historical_consumption"` // Consumption in time range from logs
}

// ==================== Dashboard Statistics Functions ====================

// GetProjectDashboard retrieves budget summary dashboard data.
// It supports filtering by time range, project name, and client user ID.
//
// Parameters:
// - startTime: Start timestamp for historical consumption query (0 for no filter)
// - endTime: End timestamp for historical consumption query (0 for no filter)
// - projectName: Filter by project name (empty for all projects)
// - clientUserId: Filter by client user ID (empty for all users)
//
// Returns:
// - ProjectDashboardResponse with aggregated budget data
// - Error if query fails
func GetProjectDashboard(startTime, endTime int64, projectName, clientUserId string) (*ProjectDashboardResponse, error) {
	response := &ProjectDashboardResponse{
		Projects: []ProjectSummary{},
	}

	// 1. Get historical consumption from logs
	historicalConsumption, err := getHistoricalConsumption(startTime, endTime, projectName, clientUserId, "")
	if err != nil {
		return nil, err
	}
	response.HistoricalConsumption = historicalConsumption

	// 2. Get current month fixed quota total
	currentMonthFixed, err := getCurrentMonthFixedQuotaTotal(clientUserId)
	if err != nil {
		return nil, err
	}
	response.CurrentMonthFixed = currentMonthFixed

	// 3. Get valid (non-expired) temp quota total
	validTempQuota, err := getValidTempQuotaTotal(clientUserId)
	if err != nil {
		return nil, err
	}
	response.ValidTempQuota = validTempQuota

	// 4. Get current month project consumption total
	projectConsumption, err := getCurrentMonthProjectConsumption(projectName, clientUserId)
	if err != nil {
		return nil, err
	}
	response.ProjectConsumption = projectConsumption

	// 5. Get max remaining budget (sum of allocated - used across all projects)
	maxRemainingBudget, err := getMaxRemainingBudget(projectName, clientUserId)
	if err != nil {
		return nil, err
	}
	response.MaxRemainingBudget = maxRemainingBudget

	// 6. Get project summaries
	projects, err := getProjectSummaries(projectName)
	if err != nil {
		return nil, err
	}
	response.Projects = projects

	return response, nil
}

// GetProjectStatistics retrieves consumption statistics for a specific project.
// It supports filtering by time range, client user ID, and scenario.
//
// Parameters:
// - projectId: The ID of the project
// - startTime: Start timestamp for historical consumption query (0 for no filter)
// - endTime: End timestamp for historical consumption query (0 for no filter)
// - clientUserId: Filter by client user ID (empty for all users)
// - scenario: Filter by scenario (empty for all scenarios)
//
// Returns:
// - ProjectStatisticsResponse with project consumption data
// - Error if query fails
func GetProjectStatistics(projectId int, startTime, endTime int64, clientUserId, scenario string) (*ProjectStatisticsResponse, error) {
	if projectId == 0 {
		return nil, errors.New("project id is empty")
	}

	// Get project info
	project, err := model.GetProjectById(projectId)
	if err != nil {
		return nil, err
	}

	// Get allocated total
	allocatedTotal, err := model.GetProjectAllocatedTotal(projectId)
	if err != nil {
		return nil, err
	}

	// Get used total
	usedTotal, err := model.GetProjectUsedTotal(projectId)
	if err != nil {
		return nil, err
	}

	// Get historical consumption from logs
	// When scenario is provided, use it for filtering instead of project name
	// since client_scenairo field stores the scenario value
	projectNameFilter := project.ProjectName
	if scenario != "" {
		// When filtering by scenario, don't also filter by project name
		// as they both use the same client_scenairo field
		projectNameFilter = ""
	}
	historicalConsumption, err := getHistoricalConsumption(startTime, endTime, projectNameFilter, clientUserId, scenario)
	if err != nil {
		return nil, err
	}

	response := &ProjectStatisticsResponse{
		ProjectId:             project.Id,
		ProjectName:           project.ProjectName,
		TotalBudget:           project.TotalBudget,
		AllocatedTotal:        allocatedTotal,
		UsedTotal:             usedTotal,
		RemainingBudget:       allocatedTotal - usedTotal,
		HistoricalConsumption: historicalConsumption,
	}

	if response.RemainingBudget < 0 {
		response.RemainingBudget = 0
	}

	return response, nil
}

// ==================== Helper Functions ====================

// getHistoricalConsumption queries the sum of quota from logs within the time range.
// Supports filtering by project name, client user ID, and scenario.
func getHistoricalConsumption(startTime, endTime int64, projectName, clientUserId, scenario string) (int64, error) {
	tx := model.LOG_DB.Table("logs").Select("COALESCE(SUM(quota), 0)")
	tx = tx.Where("type = ?", model.LogTypeConsume)

	if startTime > 0 {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		tx = tx.Where("created_at <= ?", endTime)
	}
	if projectName != "" {
		tx = tx.Where("client_scenairo = ?", projectName)
	}
	if clientUserId != "" {
		tx = tx.Where("client_user_id = ?", clientUserId)
	}
	if scenario != "" {
		tx = tx.Where("client_scenairo = ?", scenario)
	}

	var total int64
	err := tx.Scan(&total).Error
	return total, err
}

// getCurrentMonthFixedQuotaTotal returns the sum of fixed quotas for all users (or a specific user).
func getCurrentMonthFixedQuotaTotal(clientUserId string) (int, error) {
	tx := model.DB.Model(&model.CliendUserQuota{}).Select("COALESCE(SUM(fixed_quota), 0)")

	if clientUserId != "" {
		tx = tx.Where("client_user_id = ?", clientUserId)
	}

	var total int64
	err := tx.Scan(&total).Error
	return int(total), err
}

// getValidTempQuotaTotal returns the sum of valid (non-expired) temp quotas.
func getValidTempQuotaTotal(clientUserId string) (int, error) {
	now := common.GetTimestamp()
	tx := model.DB.Model(&model.CliendUserQuota{}).Select("COALESCE(SUM(temp_quota), 0)")

	// Only count non-expired temp quotas
	// expired_at = 0 means no expiration, or expired_at > now means not yet expired
	tx = tx.Where("(expired_at = 0 OR expired_at > ?)", now)

	if clientUserId != "" {
		tx = tx.Where("client_user_id = ?", clientUserId)
	}

	var total int64
	err := tx.Scan(&total).Error
	return int(total), err
}

// getCurrentMonthProjectConsumption returns the sum of used quotas from project allocations.
// This represents the total project consumption (not time-based, as project quotas don't reset monthly).
func getCurrentMonthProjectConsumption(projectName, clientUserId string) (int, error) {
	tx := model.DB.Model(&model.ProjectAllocation{}).Select("COALESCE(SUM(used_quota), 0)")

	// If filtering by project name, we need to join with projects table
	if projectName != "" {
		tx = tx.Joins("JOIN projects ON projects.id = project_allocations.project_id").
			Where("projects.project_name = ?", projectName)
	}

	if clientUserId != "" {
		tx = tx.Where("project_allocations.client_user_id = ?", clientUserId)
	}

	var total int64
	err := tx.Scan(&total).Error
	return int(total), err
}

// getMaxRemainingBudget returns the sum of (allocated_quota - used_quota) across all project allocations.
func getMaxRemainingBudget(projectName, clientUserId string) (int, error) {
	tx := model.DB.Model(&model.ProjectAllocation{}).Select("COALESCE(SUM(allocated_quota - used_quota), 0)")

	// If filtering by project name, we need to join with projects table
	if projectName != "" {
		tx = tx.Joins("JOIN projects ON projects.id = project_allocations.project_id").
			Where("projects.project_name = ?", projectName)
	}

	if clientUserId != "" {
		tx = tx.Where("project_allocations.client_user_id = ?", clientUserId)
	}

	var total int64
	err := tx.Scan(&total).Error
	if total < 0 {
		total = 0
	}
	return int(total), err
}

// getProjectSummaries returns a list of project summaries with aggregated allocation data.
func getProjectSummaries(projectName string) ([]ProjectSummary, error) {
	var projects []*model.Project
	var err error

	if projectName != "" {
		// Get specific project
		project, err := model.GetProjectByName(projectName)
		if err != nil {
			return []ProjectSummary{}, err
		}
		projects = []*model.Project{project}
	} else {
		// Get all projects
		projects, err = model.GetProjectList(0, 1000) // Get up to 1000 projects for dashboard
		if err != nil {
			return nil, err
		}
	}

	summaries := make([]ProjectSummary, 0, len(projects))
	for _, project := range projects {
		allocatedTotal, err := model.GetProjectAllocatedTotal(project.Id)
		if err != nil {
			return nil, err
		}

		usedTotal, err := model.GetQuotaByProjectName(project.ProjectName)
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, ProjectSummary{
			Id:             project.Id,
			ProjectName:    project.ProjectName,
			TotalBudget:    project.TotalBudget,
			AllocatedTotal: allocatedTotal,
			UsedTotal:      int(usedTotal),
			Status:         project.Status,
		})
	}

	return summaries, nil
}
