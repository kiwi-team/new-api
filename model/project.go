package model

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Project status constants
const (
	ProjectStatusEnabled = 1
	ProjectStatusPaused  = 2
)

// Project represents a project entity for budget management
type Project struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectName  string `json:"project_name" gorm:"uniqueIndex;size:100;not null"`
	TotalBudget  int    `json:"total_budget" gorm:"type:int;default:0"`
	ActivePlanId int    `json:"active_plan_id" gorm:"type:int;default:0"` // currently active plan, 0=none
	Status       int    `json:"status" gorm:"type:int;default:1"`         // 1=enabled, 2=paused
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// SetActivePlanId sets the active plan for a project
func SetActivePlanId(projectId int, planId int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}
	return DB.Model(&Project{}).Where("id = ?", projectId).Update("active_plan_id", planId).Error
}

// TableName returns the table name for Project
func (Project) TableName() string {
	return "projects"
}

// ProjectAllocationPlan represents a time-bound allocation plan within a project.
// When the plan expires (end_date passes), all quotas under this plan become invalid.
type ProjectAllocationPlan struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectId int    `json:"project_id" gorm:"index;not null"`
	PlanName  string `json:"plan_name" gorm:"size:200;not null"`
	StartDate string `json:"start_date" gorm:"size:10;not null"` // format: 20260101
	EndDate   string `json:"end_date" gorm:"size:10;not null"`   // format: 20260430
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// TableName returns the table name for ProjectAllocationPlan
func (ProjectAllocationPlan) TableName() string {
	return "project_allocation_plans"
}

// IsExpired checks if the plan has passed its end_date
func (p *ProjectAllocationPlan) IsExpired() bool {
	now := time.Now().Format("20060102")
	return now > p.EndDate
}

// ProjectAllocation represents budget allocation for a user within a project
type ProjectAllocation struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectId      int    `json:"project_id" gorm:"index;not null"`
	PlanId         int    `json:"plan_id" gorm:"index;default:0"` // FK to ProjectAllocationPlan
	ClientUserId   string `json:"client_user_id" gorm:"index;size:200;not null"`
	AllocatedQuota int    `json:"allocated_quota" gorm:"type:int;default:0"`
	UsedQuota      int    `json:"used_quota" gorm:"type:int;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// TableName returns the table name for ProjectAllocation
func (ProjectAllocation) TableName() string {
	return "project_allocations"
}

// MigrateProjectAllocationPlans migrates existing allocations into a default plan per project.
// Called during DB migration. Creates plans with date range 20260101-20260415.
func MigrateProjectAllocationPlans() {
	// Check if any allocations exist without a plan
	var count int64
	DB.Model(&ProjectAllocation{}).Where("plan_id = 0 OR plan_id IS NULL").Count(&count)
	if count == 0 {
		return
	}

	// Get distinct project IDs that have allocations without plans
	var projectIds []int
	DB.Model(&ProjectAllocation{}).
		Where("plan_id = 0 OR plan_id IS NULL").
		Distinct("project_id").
		Pluck("project_id", &projectIds)

	for _, projectId := range projectIds {
		// Create a default plan for this project
		plan := &ProjectAllocationPlan{
			ProjectId: projectId,
			PlanName:  "历史数据迁移",
			StartDate: "20260101",
			EndDate:   "20260415",
		}
		if err := DB.Create(plan).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to create default allocation plan for project %d: %s", projectId, err.Error()))
			continue
		}
		// Update all allocations without a plan to use this plan
		DB.Model(&ProjectAllocation{}).
			Where("project_id = ? AND (plan_id = 0 OR plan_id IS NULL)", projectId).
			Update("plan_id", plan.Id)
		// Set this plan as the active plan for the project
		DB.Model(&Project{}).Where("id = ? AND (active_plan_id = 0 OR active_plan_id IS NULL)", projectId).
			Update("active_plan_id", plan.Id)
	}
	common.SysLog("Migrated existing allocations to default plans")
}

// ==================== Project CRUD Operations ====================

// CreateProject creates a new project with initial status set to enabled
func CreateProject(project *Project) error {
	project.Status = ProjectStatusEnabled
	return DB.Create(project).Error
}

// GetProjectById retrieves a project by its ID
func GetProjectById(id int) (*Project, error) {
	if id == 0 {
		return nil, errors.New("project id is empty")
	}
	var project Project
	err := DB.First(&project, "id = ?", id).Error
	return &project, err
}

// GetProjectByName retrieves a project by its name
func GetProjectByName(name string) (*Project, error) {
	if name == "" {
		return nil, errors.New("project name is empty")
	}
	var project Project
	err := DB.Where("project_name = ?", name).First(&project).Error
	return &project, err
}

// GetProjectList retrieves a paginated list of projects
func GetProjectList(startIdx int, num int) ([]*Project, error) {
	var projects []*Project
	err := DB.Order("id desc").Limit(num).Offset(startIdx).Find(&projects).Error
	return projects, err
}

// GetProjectListWithTotal retrieves a paginated list of projects with total count
func GetProjectListWithTotal(startIdx int, num int, keyword string) ([]*Project, int64, error) {
	var projects []*Project
	var total int64

	// Start transaction for consistent count and list
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Project{})
	if keyword != "" {
		query = query.Where("project_name LIKE ?", "%"+keyword+"%")
	}

	// Get total count
	err := query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated projects
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&projects).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

// UpdateProject updates project information
func UpdateProject(project *Project) error {
	if project.Id == 0 {
		return errors.New("project id is empty")
	}
	return DB.Model(project).Select("project_name", "total_budget").Updates(project).Error
}

// UpdateProjectStatus updates the status of a project
func UpdateProjectStatus(id int, status int) error {
	if id == 0 {
		return errors.New("project id is empty")
	}
	if status != ProjectStatusEnabled && status != ProjectStatusPaused {
		return errors.New("invalid project status")
	}
	return DB.Model(&Project{}).Where("id = ?", id).Update("status", status).Error
}

// GetProjectCount returns the total number of projects
func GetProjectCount() (int64, error) {
	var count int64
	err := DB.Model(&Project{}).Count(&count).Error
	return count, err
}

// ==================== ProjectAllocationPlan CRUD Operations ====================

// CreateAllocationPlan creates a new allocation plan
func CreateAllocationPlan(plan *ProjectAllocationPlan) error {
	if plan.ProjectId == 0 {
		return errors.New("project id is empty")
	}
	if plan.PlanName == "" {
		return errors.New("plan name is empty")
	}
	if plan.StartDate == "" || plan.EndDate == "" {
		return errors.New("start date and end date are required")
	}
	if plan.StartDate > plan.EndDate {
		return errors.New("start date must be before end date")
	}
	return DB.Create(plan).Error
}

// GetAllocationPlanById retrieves an allocation plan by ID
func GetAllocationPlanById(id int) (*ProjectAllocationPlan, error) {
	if id == 0 {
		return nil, errors.New("plan id is empty")
	}
	var plan ProjectAllocationPlan
	err := DB.First(&plan, "id = ?", id).Error
	return &plan, err
}

// GetAllocationPlansByProjectId retrieves all allocation plans for a project
func GetAllocationPlansByProjectId(projectId int) ([]*ProjectAllocationPlan, error) {
	if projectId == 0 {
		return nil, errors.New("project id is empty")
	}
	var plans []*ProjectAllocationPlan
	err := DB.Where("project_id = ?", projectId).Order("id desc").Find(&plans).Error
	return plans, err
}

// UpdateAllocationPlan updates plan name, start_date, end_date
func UpdateAllocationPlan(plan *ProjectAllocationPlan) error {
	if plan.Id == 0 {
		return errors.New("plan id is empty")
	}
	if plan.StartDate > plan.EndDate {
		return errors.New("start date must be before end date")
	}
	return DB.Model(plan).Select("plan_name", "start_date", "end_date").Updates(plan).Error
}

// DeleteAllocationPlan deletes a plan and all its allocations
func DeleteAllocationPlan(planId int) error {
	if planId == 0 {
		return errors.New("plan id is empty")
	}
	// Delete allocations under this plan first
	if err := DB.Where("plan_id = ?", planId).Delete(&ProjectAllocation{}).Error; err != nil {
		return err
	}
	return DB.Delete(&ProjectAllocationPlan{}, planId).Error
}

// GetActivePlanIdForProject returns the active plan ID for a project (from project.active_plan_id)
func GetActivePlanIdForProject(projectId int) (int, error) {
	project, err := GetProjectById(projectId)
	if err != nil {
		return 0, err
	}
	return project.ActivePlanId, nil
}

// ==================== ProjectAllocation CRUD Operations ====================

// CreateOrUpdateAllocation creates a new allocation or updates an existing one
// If an allocation for the given plan, project and user already exists, it updates the allocated_quota
// Otherwise, it creates a new allocation record
func CreateOrUpdateAllocation(allocation *ProjectAllocation) error {
	if allocation.ProjectId == 0 {
		return errors.New("project id is empty")
	}
	if allocation.PlanId == 0 {
		return errors.New("plan id is empty")
	}
	if allocation.ClientUserId == "" {
		return errors.New("client user id is empty")
	}
	if allocation.AllocatedQuota < 0 {
		return errors.New("allocated quota cannot be negative")
	}

	// Check if allocation already exists for this plan + user
	var existing ProjectAllocation
	err := DB.Where("plan_id = ? AND project_id = ? AND client_user_id = ?", allocation.PlanId, allocation.ProjectId, allocation.ClientUserId).First(&existing).Error
	if err == nil {
		// Update existing allocation
		existing.AllocatedQuota = allocation.AllocatedQuota
		err = DB.Model(&existing).Update("allocated_quota", allocation.AllocatedQuota).Error
		if err != nil {
			return err
		}
		// Copy the ID back to the input allocation
		allocation.Id = existing.Id
		allocation.UsedQuota = existing.UsedQuota
		allocation.CreatedAt = existing.CreatedAt
		allocation.UpdatedAt = existing.UpdatedAt
		return nil
	}

	// Create new allocation
	allocation.UsedQuota = 0
	return DB.Create(allocation).Error
}

// ClearAllocationUsedQuota resets used_quota to 0 for a specific allocation (clear remaining budget)
func ClearAllocationUsedQuota(allocationId int) error {
	if allocationId == 0 {
		return errors.New("allocation id is empty")
	}
	return DB.Model(&ProjectAllocation{}).Where("id = ?", allocationId).
		Update("allocated_quota", 0).Error
}

// GetAllocationByProjectAndUser retrieves the allocation for the active plan by project ID and client user ID.
// Only looks at the project's ActivePlanId. UsedQuota is plan-scoped from quota_data.
func GetAllocationByProjectAndUser(projectId int, clientUserId string) (*ProjectAllocation, error) {
	if projectId == 0 {
		return nil, errors.New("project id is empty")
	}
	if clientUserId == "" {
		return nil, errors.New("client user id is empty")
	}

	activePlanId, err := GetActivePlanIdForProject(projectId)
	if err != nil {
		return nil, err
	}
	if activePlanId == 0 {
		return nil, errors.New("no active allocation plan")
	}

	// Check if the active plan has expired
	plan, err := GetAllocationPlanById(activePlanId)
	if err != nil {
		return nil, errors.New("no active allocation plan")
	}
	if plan.IsExpired() {
		return nil, errors.New("active allocation plan has expired")
	}

	var allocation ProjectAllocation
	err = DB.Where("project_id = ? AND client_user_id = ? AND plan_id = ?", projectId, clientUserId, activePlanId).First(&allocation).Error
	if err != nil {
		return nil, err
	}
	// UsedQuota scoped to this plan
	quota, _ := GetQuotaByPlanIdUid(activePlanId, clientUserId)
	allocation.UsedQuota = quota
	return &allocation, nil
}

// GetAllocationsByPlanId retrieves all allocations for a plan with pagination
func GetAllocationsByPlanId(planId int, startIdx int, num int) ([]*ProjectAllocation, error) {
	if planId == 0 {
		return nil, errors.New("plan id is empty")
	}

	var allocations []*ProjectAllocation
	err := DB.Where("plan_id = ?", planId).Order("id desc").Limit(num).Offset(startIdx).Find(&allocations).Error
	for _, item := range allocations {
		item.UsedQuota, _ = GetQuotaByPlanIdUid(planId, item.ClientUserId)
	}
	return allocations, err
}

// GetAllocationsByPlanIdWithTotal retrieves all allocations for a plan with pagination and total count
func GetAllocationsByPlanIdWithTotal(planId int, startIdx int, num int) ([]*ProjectAllocation, int64, error) {
	if planId == 0 {
		return nil, 0, errors.New("plan id is empty")
	}

	var allocations []*ProjectAllocation
	var total int64

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	err := tx.Model(&ProjectAllocation{}).Where("plan_id = ?", planId).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = tx.Where("plan_id = ?", planId).Order("id desc").Limit(num).Offset(startIdx).Find(&allocations).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	for _, item := range allocations {
		item.UsedQuota, _ = GetQuotaByPlanIdUid(planId, item.ClientUserId)
	}

	return allocations, total, nil
}

// GetAllocationsByProjectId retrieves all allocations for a project with pagination
func GetAllocationsByProjectId(projectId int, startIdx int, num int) ([]*ProjectAllocation, error) {
	if projectId == 0 {
		return nil, errors.New("project id is empty")
	}

	var allocations []*ProjectAllocation
	err := DB.Where("project_id = ?", projectId).Order("id desc").Limit(num).Offset(startIdx).Find(&allocations).Error
	for _, item := range allocations {
		item.UsedQuota, _ = GetQuotaByProjectIdUid(item.ProjectId, item.ClientUserId)
	}
	return allocations, err
}

// GetAllocationsByProjectIdWithTotal retrieves all allocations for a project with pagination and total count
func GetAllocationsByProjectIdWithTotal(projectId int, startIdx int, num int) ([]*ProjectAllocation, int64, error) {
	if projectId == 0 {
		return nil, 0, errors.New("project id is empty")
	}

	var allocations []*ProjectAllocation
	var total int64

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	err := tx.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = tx.Where("project_id = ?", projectId).Order("id desc").Limit(num).Offset(startIdx).Find(&allocations).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	for _, item := range allocations {
		item.UsedQuota, _ = GetQuotaByProjectIdUid(item.ProjectId, item.ClientUserId)
	}

	return allocations, total, nil
}

// GetAllocationsByClientUserId retrieves all allocations for a user
func GetAllocationsByClientUserId(clientUserId string) ([]*ProjectAllocation, error) {
	if clientUserId == "" {
		return nil, errors.New("client user id is empty")
	}

	var allocations []*ProjectAllocation
	err := DB.Where("client_user_id = ?", clientUserId).Order("id desc").Find(&allocations).Error
	return allocations, err
}

// GetProjectAllocatedTotal returns the sum of all allocated quotas for a project
func GetProjectAllocatedTotal(projectId int) (int, error) {
	if projectId == 0 {
		return 0, errors.New("project id is empty")
	}

	var total int64
	err := DB.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Select("COALESCE(SUM(allocated_quota), 0)").Scan(&total).Error
	return int(total), err
}

// GetPlanAllocatedTotal returns the sum of all allocated quotas for a specific plan
func GetPlanAllocatedTotal(planId int) (int, error) {
	if planId == 0 {
		return 0, errors.New("plan id is empty")
	}

	var total int64
	err := DB.Model(&ProjectAllocation{}).Where("plan_id = ?", planId).Select("COALESCE(SUM(allocated_quota), 0)").Scan(&total).Error
	return int(total), err
}

// GetProjectUsedTotal returns the sum of all used quotas for a project
func GetProjectUsedTotal(projectId int) (int, error) {
	if projectId == 0 {
		return 0, errors.New("project id is empty")
	}

	var total int64
	err := DB.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Select("COALESCE(SUM(used_quota), 0)").Scan(&total).Error
	return int(total), err
}

// IncreaseUsedQuota atomically increases the used quota for an allocation
// It uses database-level atomic operations to prevent race conditions
// Returns an error if the update would cause used_quota to exceed allocated_quota
func IncreaseUsedQuota(allocationId int, delta int) error {
	if allocationId == 0 {
		return errors.New("allocation id is empty")
	}
	if delta < 0 {
		return errors.New("delta cannot be negative")
	}
	if delta == 0 {
		return nil // No-op for zero delta
	}

	// Use GORM's UpdateColumn with condition to ensure atomicity
	// This ensures used_quota + delta <= allocated_quota
	result := DB.Model(&ProjectAllocation{}).
		Where("id = ? AND used_quota + ? <= allocated_quota", allocationId, delta).
		UpdateColumn("used_quota", DB.Raw("used_quota + ?", delta))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("project quota exceeded or concurrent update conflict")
	}

	return nil
}

// IncreaseUsedQuotaByProjectAndUser atomically increases the used quota for an allocation
// identified by project ID and client user ID
func IncreaseUsedQuotaByProjectAndUser(projectId int, clientUserId string, delta int) error {
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

	// Use GORM's UpdateColumn with condition to ensure atomicity
	result := DB.Model(&ProjectAllocation{}).
		Where("project_id = ? AND client_user_id = ? AND used_quota + ? <= allocated_quota", projectId, clientUserId, delta).
		UpdateColumn("used_quota", DB.Raw("used_quota + ?", delta))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("project quota exceeded or concurrent update conflict")
	}

	return nil
}

// DecreaseUsedQuotaByProjectAndUser atomically returns quota to an allocation
// identified by project ID and client user ID. Async tasks pre-consume at submit
// time and settle later, so a refund or a below-estimate settlement has to give
// the project budget back.
//
// used_quota is clamped at 0: a duplicate refund or a settlement racing a manual
// budget reset would otherwise drive it negative, which reads as free budget.
// Clamping is reported so the accounting anomaly stays visible.
func DecreaseUsedQuotaByProjectAndUser(projectId int, clientUserId string, delta int) error {
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

	result := DB.Model(&ProjectAllocation{}).
		Where("project_id = ? AND client_user_id = ? AND used_quota >= ?", projectId, clientUserId, delta).
		UpdateColumn("used_quota", DB.Raw("used_quota - ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	// 到这里说明 used_quota < delta（或分配记录不存在）。退不回去会永久占住项目预算，
	// 所以退到 0 为止，并把这次异常写进系统日志。
	clamped := DB.Model(&ProjectAllocation{}).
		Where("project_id = ? AND client_user_id = ? AND used_quota > 0", projectId, clientUserId).
		UpdateColumn("used_quota", 0)
	if clamped.Error != nil {
		return clamped.Error
	}
	if clamped.RowsAffected > 0 {
		common.SysError(fmt.Sprintf(
			"project allocation used_quota clamped to 0 on refund: project_id=%d client_user_id=%s delta=%d",
			projectId, clientUserId, delta))
	}
	return nil
}

// GetAllocationCount returns the total number of allocations for a project
func GetAllocationCount(projectId int) (int64, error) {
	if projectId == 0 {
		return 0, errors.New("project id is empty")
	}

	var count int64
	err := DB.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Count(&count).Error
	return count, err
}

// GetUserProjectRemainingQuota calculates the remaining quota for a user across all their project allocations.
// Only counts allocations under the currently active plan per project.
// remaining = allocated_quota - plan-scoped consumption from quota_data
func GetUserProjectRemainingQuota(clientUserId string) (int, error) {
	if clientUserId == "" {
		return 0, errors.New("client user id is empty")
	}

	// Get allocations that are under an active plan (join projects on active_plan_id)
	var allocations []ProjectAllocation
	err := DB.Model(&ProjectAllocation{}).
		Joins("JOIN projects ON projects.id = project_allocations.project_id AND projects.active_plan_id = project_allocations.plan_id").
		Where("project_allocations.client_user_id = ? AND projects.active_plan_id > 0", clientUserId).
		Find(&allocations).Error
	if err != nil {
		return 0, err
	}

	totalRemaining := 0
	for _, a := range allocations {
		used, _ := GetQuotaByPlanIdUid(a.PlanId, clientUserId)
		remaining := a.AllocatedQuota - used/500000
		if remaining > 0 {
			totalRemaining += remaining
		}
	}
	return totalRemaining, err
}

// DeleteAllocationsByProjectId deletes all allocations for a project
// This is typically used when cleaning up test data
func DeleteAllocationsByProjectId(projectId int) error {
	if projectId == 0 {
		return errors.New("project id is empty")
	}

	return DB.Where("project_id = ?", projectId).Delete(&ProjectAllocation{}).Error
}

func GetQuotaByProjectName(projectName string) (int64, error) {
	var quota int64
	err := DB.Model(&QuotaData{}).Where("project_name = ?", projectName).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&quota).Error
	return quota, err
}

func GetQuotaByProjectIdUid(projectId int, uid string) (int, error) {
	var project Project
	err := DB.First(&project, projectId).Error
	if err != nil {
		return 0, err
	}
	var quota int64
	err = DB.Model(&QuotaData{}).Where("project_name = ? AND client_user_id = ?", project.ProjectName, uid).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&quota).Error
	return int(quota), err
}

// GetQuotaByPlanIdUid returns consumption for a specific plan + uid
func GetQuotaByPlanIdUid(planId int, uid string) (int, error) {
	var quota int64
	err := DB.Model(&QuotaData{}).Where("plan_id = ? AND client_user_id = ?", planId, uid).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&quota).Error
	return int(quota), err
}

// GetQuotaByPlanId returns plan-scoped consumption for all users.
func GetQuotaByPlanId(planId int) (int64, error) {
	if planId == 0 {
		return 0, nil
	}
	var quota int64
	err := DB.Model(&QuotaData{}).Where("plan_id = ?", planId).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&quota).Error
	return quota, err
}

// ProjectAllocationDetail 项目预算分配详情（含项目名称和已消耗金额）
type ProjectAllocationDetail struct {
	AllocationId       int     `json:"allocation_id"`
	ProjectId          int     `json:"project_id"`
	ProjectName        string  `json:"project_name"`
	ProjectStatus      int     `json:"project_status"`
	PlanId             int     `json:"plan_id"`
	PlanName           string  `json:"plan_name"`
	StartDate          string  `json:"start_date"`
	EndDate            string  `json:"end_date"`
	AllocatedQuota     int     `json:"allocated_quota"`
	UsedQuotaUSD       float64 `json:"used_quota_usd"`
	RemainingQuotaUSD  float64 `json:"remaining_quota_usd"`
	IsActivePlan       bool    `json:"is_active_plan"`
	IsInDateRange      bool    `json:"is_in_date_range"`
	IsCurrentEffective bool    `json:"is_current_effective"`
}

type projectAllocationUsage struct {
	ClientUserId string `gorm:"column:client_user_id"`
	PlanId       int    `gorm:"column:plan_id"`
	ProjectName  string `gorm:"column:project_name"`
	UsedQuota    int64  `gorm:"column:used_quota"`
}

func projectAllocationUsageKey(clientUserId string, planId int, projectName string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", clientUserId, planId, projectName)
}

// getProjectAllocationDetailsForUsers builds plan-scoped allocation details.
// Consumption must be grouped by plan_id; grouping only by project would repeat
// the same historical project consumption on every plan row.
func getProjectAllocationDetailsForUsers(clientUserIds []string, currentOnly bool) (map[string][]*ProjectAllocationDetail, error) {
	result := make(map[string][]*ProjectAllocationDetail, len(clientUserIds))
	if len(clientUserIds) == 0 {
		return result, nil
	}

	var allocations []*ProjectAllocation
	if err := DB.Where("client_user_id IN ?", clientUserIds).Find(&allocations).Error; err != nil {
		return nil, err
	}
	if len(allocations) == 0 {
		return result, nil
	}

	projectIdSet := make(map[int]struct{})
	planIdSet := make(map[int]struct{})
	for _, allocation := range allocations {
		projectIdSet[allocation.ProjectId] = struct{}{}
		if allocation.PlanId > 0 {
			planIdSet[allocation.PlanId] = struct{}{}
		}
	}
	projectIds := make([]int, 0, len(projectIdSet))
	for id := range projectIdSet {
		projectIds = append(projectIds, id)
	}
	planIds := make([]int, 0, len(planIdSet))
	for id := range planIdSet {
		planIds = append(planIds, id)
	}

	var projects []*Project
	if err := DB.Where("id IN ?", projectIds).Find(&projects).Error; err != nil {
		return nil, err
	}
	projectMap := make(map[int]*Project, len(projects))
	for _, project := range projects {
		projectMap[project.Id] = project
	}

	planMap := make(map[int]*ProjectAllocationPlan, len(planIds))
	if len(planIds) > 0 {
		var plans []*ProjectAllocationPlan
		if err := DB.Where("id IN ?", planIds).Find(&plans).Error; err != nil {
			return nil, err
		}
		for _, plan := range plans {
			planMap[plan.Id] = plan
		}
	}

	var usageRows []projectAllocationUsage
	if err := DB.Model(&QuotaData{}).
		Where("client_user_id IN ?", clientUserIds).
		Select("client_user_id, plan_id, project_name, COALESCE(SUM(quota), 0) AS used_quota").
		Group("client_user_id, plan_id, project_name").
		Scan(&usageRows).Error; err != nil {
		return nil, err
	}
	usageMap := make(map[string]int64, len(usageRows))
	for _, usage := range usageRows {
		usageMap[projectAllocationUsageKey(usage.ClientUserId, usage.PlanId, usage.ProjectName)] = usage.UsedQuota
	}

	today := time.Now().Format("20060102")
	for _, allocation := range allocations {
		project, ok := projectMap[allocation.ProjectId]
		if !ok {
			continue
		}
		plan := planMap[allocation.PlanId]
		isActivePlan := plan != nil && project.ActivePlanId == plan.Id
		isInDateRange := plan != nil && plan.StartDate <= today && today <= plan.EndDate
		isCurrentEffective := project.Status == ProjectStatusEnabled && isActivePlan && isInDateRange
		if currentOnly && !isCurrentEffective {
			continue
		}

		usedQuota := usageMap[projectAllocationUsageKey(allocation.ClientUserId, allocation.PlanId, project.ProjectName)]
		// Usage written before allocation plans were introduced has plan_id=0.
		// The migration plan owns that historical usage, scoped by project name to
		// avoid mixing it with non-project consumption for the same UID.
		if plan != nil && plan.PlanName == "历史数据迁移" {
			usedQuota += usageMap[projectAllocationUsageKey(allocation.ClientUserId, 0, project.ProjectName)]
		}
		usedQuotaUSD := float64(usedQuota) / common.QuotaPerUnit
		remainingQuotaUSD := float64(allocation.AllocatedQuota) - usedQuotaUSD
		if remainingQuotaUSD < 0 {
			remainingQuotaUSD = 0
		}
		detail := &ProjectAllocationDetail{
			AllocationId:       allocation.Id,
			ProjectId:          allocation.ProjectId,
			ProjectName:        project.ProjectName,
			ProjectStatus:      project.Status,
			PlanId:             allocation.PlanId,
			AllocatedQuota:     allocation.AllocatedQuota,
			UsedQuotaUSD:       usedQuotaUSD,
			RemainingQuotaUSD:  remainingQuotaUSD,
			IsActivePlan:       isActivePlan,
			IsInDateRange:      isInDateRange,
			IsCurrentEffective: isCurrentEffective,
		}
		if plan != nil {
			detail.PlanName = plan.PlanName
			detail.StartDate = plan.StartDate
			detail.EndDate = plan.EndDate
		}
		result[allocation.ClientUserId] = append(result[allocation.ClientUserId], detail)
	}

	for clientUserId := range result {
		sort.SliceStable(result[clientUserId], func(i, j int) bool {
			left, right := result[clientUserId][i], result[clientUserId][j]
			if left.IsCurrentEffective != right.IsCurrentEffective {
				return left.IsCurrentEffective
			}
			if left.StartDate != right.StartDate {
				return left.StartDate > right.StartDate
			}
			return left.AllocationId > right.AllocationId
		})
	}

	return result, nil
}

// GetProjectAllocationDetails 获取某个UID的所有项目预算分配详情。
// 历史方案会返回，但消耗和剩余额度均按 plan_id 独立计算。
func GetProjectAllocationDetails(clientUserId string) ([]*ProjectAllocationDetail, error) {
	if clientUserId == "" {
		return nil, errors.New("client user id is empty")
	}
	detailsByUser, err := getProjectAllocationDetailsForUsers([]string{clientUserId}, false)
	if err != nil {
		return nil, err
	}
	details := detailsByUser[clientUserId]
	if details == nil {
		details = make([]*ProjectAllocationDetail, 0)
	}
	return details, nil
}

// ProjectBudgetSummary 某个UID的项目预算汇总
type ProjectBudgetSummary struct {
	ClientUserId             string                     `json:"client_user_id"`
	TotalAllocated           int                        `json:"total_allocated"`
	TotalUsedUSD             float64                    `json:"total_used_usd"`
	TotalRemainingUSD        float64                    `json:"total_remaining_usd"`
	MonthlyProjectUsedUSD    float64                    `json:"monthly_project_used_usd"`
	MonthlyNonProjectUsedUSD float64                    `json:"monthly_non_project_used_usd"`
	ProjectCount             int                        `json:"project_count"`
	Projects                 []*ProjectAllocationDetail `json:"projects"`
}

// GetBatchProjectBudgetSummary 批量获取多个UID当前生效的项目预算汇总。
// 历史、未启用、暂停、未开始和已过期方案不计入列表汇总。
func GetBatchProjectBudgetSummary(clientUserIds []string) (map[string]*ProjectBudgetSummary, error) {
	if len(clientUserIds) == 0 {
		return map[string]*ProjectBudgetSummary{}, nil
	}
	detailsByUser, err := getProjectAllocationDetailsForUsers(clientUserIds, true)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*ProjectBudgetSummary, len(clientUserIds))
	for _, clientUserId := range clientUserIds {
		result[clientUserId] = &ProjectBudgetSummary{
			ClientUserId: clientUserId,
			Projects:     make([]*ProjectAllocationDetail, 0),
		}
	}
	for clientUserId, details := range detailsByUser {
		summary := result[clientUserId]
		summary.Projects = details
		projectIds := make(map[int]struct{})
		for _, detail := range details {
			summary.TotalAllocated += detail.AllocatedQuota
			summary.TotalUsedUSD += detail.UsedQuotaUSD
			summary.TotalRemainingUSD += detail.RemainingQuotaUSD
			projectIds[detail.ProjectId] = struct{}{}
		}
		summary.ProjectCount = len(projectIds)
	}

	// 项目消耗与非项目消耗按同一个计费月分别聚合，与准入闸门口径一致：
	// 非项目消耗受 fixed+temp 约束，项目消耗受各自的项目额度约束，两者互不透支。
	// 这里必须用 common.BillingMonthStartUnix，否则看板与闸门会用不同的月起点。
	type monthlyUsage struct {
		ClientUserId string `gorm:"column:client_user_id"`
		UsedQuota    int64  `gorm:"column:used_quota"`
	}
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())
	var monthlyUsageRows []monthlyUsage
	if err := DB.Model(&QuotaData{}).
		Where("client_user_id IN ? AND created_at >= ? AND project_name <> ?", clientUserIds, monthStart, "").
		Select("client_user_id, COALESCE(SUM(quota), 0) AS used_quota").
		Group("client_user_id").
		Scan(&monthlyUsageRows).Error; err != nil {
		return nil, err
	}
	for _, usage := range monthlyUsageRows {
		if summary := result[usage.ClientUserId]; summary != nil && usage.UsedQuota > 0 {
			summary.MonthlyProjectUsedUSD = float64(usage.UsedQuota) / common.QuotaPerUnit
		}
	}

	var monthlyNonProjectRows []monthlyUsage
	if err := DB.Model(&QuotaData{}).
		Where("client_user_id IN ? AND created_at >= ? AND (project_name = '' OR project_name IS NULL)", clientUserIds, monthStart).
		Select("client_user_id, COALESCE(SUM(quota), 0) AS used_quota").
		Group("client_user_id").
		Scan(&monthlyNonProjectRows).Error; err != nil {
		return nil, err
	}
	for _, usage := range monthlyNonProjectRows {
		if summary := result[usage.ClientUserId]; summary != nil && usage.UsedQuota > 0 {
			summary.MonthlyNonProjectUsedUSD = float64(usage.UsedQuota) / common.QuotaPerUnit
		}
	}
	return result, nil
}
