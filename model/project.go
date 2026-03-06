package model

import (
	"errors"
)

// Project status constants
const (
	ProjectStatusEnabled = 1
	ProjectStatusPaused  = 2
)

// Project represents a project entity for budget management
type Project struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectName string `json:"project_name" gorm:"uniqueIndex;size:100;not null"`
	TotalBudget int    `json:"total_budget" gorm:"type:int;default:0"`
	Status      int    `json:"status" gorm:"type:int;default:1"` // 1=enabled, 2=paused
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// TableName returns the table name for Project
func (Project) TableName() string {
	return "projects"
}

// ProjectAllocation represents budget allocation for a user within a project
type ProjectAllocation struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectId      int    `json:"project_id" gorm:"index;not null"`
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

// ==================== ProjectAllocation CRUD Operations ====================

// CreateOrUpdateAllocation creates a new allocation or updates an existing one
// If an allocation for the given project and user already exists, it updates the allocated_quota
// Otherwise, it creates a new allocation record
func CreateOrUpdateAllocation(allocation *ProjectAllocation) error {
	if allocation.ProjectId == 0 {
		return errors.New("project id is empty")
	}
	if allocation.ClientUserId == "" {
		return errors.New("client user id is empty")
	}
	if allocation.AllocatedQuota < 0 {
		return errors.New("allocated quota cannot be negative")
	}

	// Check if allocation already exists
	var existing ProjectAllocation
	err := DB.Where("project_id = ? AND client_user_id = ?", allocation.ProjectId, allocation.ClientUserId).First(&existing).Error
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

// GetAllocationByProjectAndUser retrieves an allocation by project ID and client user ID
func GetAllocationByProjectAndUser(projectId int, clientUserId string) (*ProjectAllocation, error) {
	if projectId == 0 {
		return nil, errors.New("project id is empty")
	}
	if clientUserId == "" {
		return nil, errors.New("client user id is empty")
	}

	var allocation ProjectAllocation
	err := DB.Where("project_id = ? AND client_user_id = ?", projectId, clientUserId).First(&allocation).Error
	quota, _ := GetQuotaByProjectIdUid(projectId, clientUserId)
	allocation.UsedQuota = quota
	return &allocation, err
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

	// Get total count
	err := tx.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated allocations
	err = tx.Where("project_id = ?", projectId).Order("id desc").Limit(num).Offset(startIdx).Find(&allocations).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
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

// GetAllocationCount returns the total number of allocations for a project
func GetAllocationCount(projectId int) (int64, error) {
	if projectId == 0 {
		return 0, errors.New("project id is empty")
	}

	var count int64
	err := DB.Model(&ProjectAllocation{}).Where("project_id = ?", projectId).Count(&count).Error
	return count, err
}

// GetUserProjectRemainingQuota calculates the remaining quota for a user across all their project allocations
// Returns the sum of (allocated_quota - used_quota) for all allocations belonging to the user
func GetUserProjectRemainingQuota(clientUserId string) (int, error) {
	if clientUserId == "" {
		return 0, errors.New("client user id is empty")
	}

	var total int64
	err := DB.Model(&ProjectAllocation{}).
		Where("client_user_id = ?", clientUserId).
		Select("COALESCE(SUM(allocated_quota - used_quota), 0)").
		Scan(&total).Error
	return int(total), err
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

// ProjectAllocationDetail 项目预算分配详情（含项目名称和已消耗金额）
type ProjectAllocationDetail struct {
	ProjectId      int     `json:"project_id"`
	ProjectName    string  `json:"project_name"`
	AllocatedQuota int     `json:"allocated_quota"`
	UsedQuotaUSD   float64 `json:"used_quota_usd"`
}

// GetProjectAllocationDetails 获取某个UID的所有项目预算分配详情
func GetProjectAllocationDetails(clientUserId string) ([]*ProjectAllocationDetail, error) {
	if clientUserId == "" {
		return nil, errors.New("client user id is empty")
	}

	var allocations []*ProjectAllocation
	err := DB.Where("client_user_id = ?", clientUserId).Find(&allocations).Error
	if err != nil {
		return nil, err
	}

	details := make([]*ProjectAllocationDetail, 0, len(allocations))
	for _, a := range allocations {
		var project Project
		if err := DB.First(&project, a.ProjectId).Error; err != nil {
			continue
		}
		usedQuota, _ := GetQuotaByProjectIdUid(a.ProjectId, clientUserId)
		details = append(details, &ProjectAllocationDetail{
			ProjectId:      a.ProjectId,
			ProjectName:    project.ProjectName,
			AllocatedQuota: a.AllocatedQuota,
			UsedQuotaUSD:   float64(usedQuota) / 500000.0,
		})
	}
	return details, nil
}
