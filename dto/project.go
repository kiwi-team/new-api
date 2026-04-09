package dto

// CreateProjectRequest represents the request body for creating a new project
type CreateProjectRequest struct {
	ProjectName string `json:"project_name" binding:"required,max=100"`
	TotalBudget int    `json:"total_budget" binding:"gte=0"`
}

// UpdateProjectRequest represents the request body for updating a project
type UpdateProjectRequest struct {
	ProjectName string `json:"project_name" binding:"max=100"`
	TotalBudget int    `json:"total_budget" binding:"gte=0"`
}

// UpdateProjectStatusRequest represents the request body for updating project status
type UpdateProjectStatusRequest struct {
	Status int `json:"status" binding:"oneof=1 2"`
}

// CreateAllocationPlanRequest represents the request for creating an allocation plan
type CreateAllocationPlanRequest struct {
	PlanName  string `json:"plan_name" binding:"required,max=200"`
	StartDate string `json:"start_date" binding:"required"` // format: 20260101
	EndDate   string `json:"end_date" binding:"required"`   // format: 20260430
}

// UpdateAllocationPlanRequest represents the request for updating an allocation plan
type UpdateAllocationPlanRequest struct {
	PlanName  string `json:"plan_name" binding:"max=200"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// AllocationRequest represents the request body for creating or updating a budget allocation
type AllocationRequest struct {
	ClientUserId   string `json:"client_user_id" binding:"required"`
	AllocatedQuota int    `json:"allocated_quota" binding:"gte=0"`
}

// ClearAllocationRequest represents the request to clear a user's remaining budget
type ClearAllocationRequest struct {
	AllocationId int `json:"allocation_id" binding:"required"`
}

// ProjectDashboardResponse represents the response for the project budget dashboard
type ProjectDashboardResponse struct {
	HistoricalConsumption int64            `json:"historical_consumption"`
	CurrentMonthFixed     int              `json:"current_month_fixed"`
	ValidTempQuota        int              `json:"valid_temp_quota"`
	ProjectConsumption    int              `json:"project_consumption"`
	MaxRemainingBudget    int              `json:"max_remaining_budget"`
	Projects              []ProjectSummary `json:"projects"`
}

// ProjectSummary represents a summary of a project for the dashboard
type ProjectSummary struct {
	Id             int    `json:"id"`
	ProjectName    string `json:"project_name"`
	TotalBudget    int    `json:"total_budget"`
	AllocatedTotal int    `json:"allocated_total"`
	UsedTotal      int    `json:"used_total"`
	Status         int    `json:"status"`
}

// ProjectStatisticsResponse represents the response for project consumption statistics
type ProjectStatisticsResponse struct {
	ProjectId             int    `json:"project_id"`
	ProjectName           string `json:"project_name"`
	TotalBudget           int    `json:"total_budget"`
	AllocatedTotal        int    `json:"allocated_total"`
	UsedTotal             int    `json:"used_total"`
	HistoricalConsumption int64  `json:"historical_consumption"`
	Status                int    `json:"status"`
}
