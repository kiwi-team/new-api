package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupProjectTestDB creates an in-memory SQLite database for unit testing
func setupProjectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Use in-memory SQLite for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to connect to test database")

	// Set the global flags for SQLite
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	// Run migrations for Project and ProjectAllocation tables
	err = db.AutoMigrate(&Project{}, &ProjectAllocation{})
	require.NoError(t, err, "failed to migrate test database")

	// Set the global DB for model functions
	DB = db

	return db
}

// cleanupProjectTestDB cleans up the test database
func cleanupProjectTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// ==================== CreateProject Tests ====================

func TestCreateProject_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}

	err := CreateProject(project)
	require.NoError(t, err)

	// Verify project was created with correct values
	assert.Greater(t, project.Id, 0)
	assert.Equal(t, "test-project", project.ProjectName)
	assert.Equal(t, 10000, project.TotalBudget)
	assert.Equal(t, ProjectStatusEnabled, project.Status)
	assert.Greater(t, project.CreatedAt, int64(0))
	assert.Greater(t, project.UpdatedAt, int64(0))
}

func TestCreateProject_InitialStatusEnabled(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Even if status is set to paused, CreateProject should set it to enabled
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 5000,
		Status:      ProjectStatusPaused, // This should be overridden
	}

	err := CreateProject(project)
	require.NoError(t, err)

	// Verify status is enabled regardless of input
	assert.Equal(t, ProjectStatusEnabled, project.Status)

	// Verify by fetching from database
	var fetched Project
	err = db.First(&fetched, project.Id).Error
	require.NoError(t, err)
	assert.Equal(t, ProjectStatusEnabled, fetched.Status)
}

func TestCreateProject_ZeroBudget(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "zero-budget-project",
		TotalBudget: 0,
	}

	err := CreateProject(project)
	require.NoError(t, err)

	assert.Greater(t, project.Id, 0)
	assert.Equal(t, 0, project.TotalBudget)
}

func TestCreateProject_MaxBudget(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Test with a large budget value
	project := &Project{
		ProjectName: "max-budget-project",
		TotalBudget: 2147483647, // Max int32 value
	}

	err := CreateProject(project)
	require.NoError(t, err)

	assert.Equal(t, 2147483647, project.TotalBudget)
}

func TestCreateProject_DuplicateName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create first project
	project1 := &Project{
		ProjectName: "duplicate-name",
		TotalBudget: 1000,
	}
	err := CreateProject(project1)
	require.NoError(t, err)

	// Attempt to create second project with same name
	project2 := &Project{
		ProjectName: "duplicate-name",
		TotalBudget: 2000,
	}
	err = CreateProject(project2)
	assert.Error(t, err, "should fail due to unique constraint")
}

func TestCreateProject_EmptyName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "",
		TotalBudget: 1000,
	}

	// Note: SQLite allows empty strings even with NOT NULL constraint
	// The uniqueness constraint will prevent multiple empty names
	err := CreateProject(project)
	if err != nil {
		// Some databases may reject empty strings
		return
	}

	// If first empty name succeeds, second should fail due to unique constraint
	project2 := &Project{
		ProjectName: "",
		TotalBudget: 2000,
	}
	err = CreateProject(project2)
	assert.Error(t, err, "duplicate empty name should fail")
}

// ==================== GetProjectById Tests ====================

func TestGetProjectById_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project first
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 5000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Fetch by ID
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)

	assert.Equal(t, project.Id, fetched.Id)
	assert.Equal(t, "test-project", fetched.ProjectName)
	assert.Equal(t, 5000, fetched.TotalBudget)
	assert.Equal(t, ProjectStatusEnabled, fetched.Status)
}

func TestGetProjectById_NotFound(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetProjectById(99999)
	assert.Error(t, err)
}

func TestGetProjectById_ZeroId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetProjectById(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

// ==================== GetProjectByName Tests ====================

func TestGetProjectByName_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project first
	project := &Project{
		ProjectName: "named-project",
		TotalBudget: 3000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Fetch by name
	fetched, err := GetProjectByName("named-project")
	require.NoError(t, err)

	assert.Equal(t, project.Id, fetched.Id)
	assert.Equal(t, "named-project", fetched.ProjectName)
	assert.Equal(t, 3000, fetched.TotalBudget)
}

func TestGetProjectByName_NotFound(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetProjectByName("nonexistent-project")
	assert.Error(t, err)
}

func TestGetProjectByName_EmptyName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetProjectByName("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project name is empty")
}

// ==================== GetProjectList Tests ====================

func TestGetProjectList_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create multiple projects
	for i := 1; i <= 5; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: i * 1000,
		}
		err := CreateProject(project)
		require.NoError(t, err)
	}

	// Fetch all projects
	projects, err := GetProjectList(0, 10)
	require.NoError(t, err)

	assert.Len(t, projects, 5)
}

func TestGetProjectList_Pagination(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create 10 projects
	for i := 1; i <= 10; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: i * 100,
		}
		err := CreateProject(project)
		require.NoError(t, err)
	}

	// Fetch first page (5 items)
	page1, err := GetProjectList(0, 5)
	require.NoError(t, err)
	assert.Len(t, page1, 5)

	// Fetch second page (5 items)
	page2, err := GetProjectList(5, 5)
	require.NoError(t, err)
	assert.Len(t, page2, 5)

	// Verify no overlap between pages
	page1Ids := make(map[int]bool)
	for _, p := range page1 {
		page1Ids[p.Id] = true
	}
	for _, p := range page2 {
		assert.False(t, page1Ids[p.Id], "pages should not overlap")
	}
}

func TestGetProjectList_Empty(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	projects, err := GetProjectList(0, 10)
	require.NoError(t, err)

	assert.Len(t, projects, 0)
}

func TestGetProjectList_OrderByIdDesc(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create projects
	for i := 1; i <= 3; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: i * 100,
		}
		err := CreateProject(project)
		require.NoError(t, err)
	}

	projects, err := GetProjectList(0, 10)
	require.NoError(t, err)

	// Verify descending order by ID
	for i := 0; i < len(projects)-1; i++ {
		assert.Greater(t, projects[i].Id, projects[i+1].Id)
	}
}

// ==================== UpdateProject Tests ====================

func TestUpdateProject_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "original-name",
		TotalBudget: 1000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Update the project
	project.ProjectName = "updated-name"
	project.TotalBudget = 2000
	err = UpdateProject(project)
	require.NoError(t, err)

	// Verify update
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)
	assert.Equal(t, "updated-name", fetched.ProjectName)
	assert.Equal(t, 2000, fetched.TotalBudget)
}

func TestUpdateProject_ZeroId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		Id:          0,
		ProjectName: "test",
		TotalBudget: 1000,
	}

	err := UpdateProject(project)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

func TestUpdateProject_OnlyNameAndBudget(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "original-name",
		TotalBudget: 1000,
	}
	err := CreateProject(project)
	require.NoError(t, err)
	originalStatus := project.Status

	// Try to update status through UpdateProject (should not change)
	project.ProjectName = "new-name"
	project.TotalBudget = 2000
	project.Status = ProjectStatusPaused
	err = UpdateProject(project)
	require.NoError(t, err)

	// Verify status was not changed
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)
	assert.Equal(t, originalStatus, fetched.Status)
	assert.Equal(t, "new-name", fetched.ProjectName)
	assert.Equal(t, 2000, fetched.TotalBudget)
}

func TestUpdateProject_ZeroBudget(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project with non-zero budget
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 5000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Update to zero budget
	project.TotalBudget = 0
	err = UpdateProject(project)
	require.NoError(t, err)

	// Verify update
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, fetched.TotalBudget)
}

// ==================== UpdateProjectStatus Tests ====================

func TestUpdateProjectStatus_ToEnabled(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 1000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// First pause it
	err = UpdateProjectStatus(project.Id, ProjectStatusPaused)
	require.NoError(t, err)

	// Then enable it
	err = UpdateProjectStatus(project.Id, ProjectStatusEnabled)
	require.NoError(t, err)

	// Verify
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)
	assert.Equal(t, ProjectStatusEnabled, fetched.Status)
}

func TestUpdateProjectStatus_ToPaused(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 1000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Pause it
	err = UpdateProjectStatus(project.Id, ProjectStatusPaused)
	require.NoError(t, err)

	// Verify
	fetched, err := GetProjectById(project.Id)
	require.NoError(t, err)
	assert.Equal(t, ProjectStatusPaused, fetched.Status)
}

func TestUpdateProjectStatus_ZeroId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	err := UpdateProjectStatus(0, ProjectStatusEnabled)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

func TestUpdateProjectStatus_InvalidStatus(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 1000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Try invalid status values
	err = UpdateProjectStatus(project.Id, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project status")

	err = UpdateProjectStatus(project.Id, 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project status")

	err = UpdateProjectStatus(project.Id, -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project status")
}

// ==================== GetProjectCount Tests ====================

func TestGetProjectCount_Empty(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	count, err := GetProjectCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGetProjectCount_WithProjects(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create multiple projects
	for i := 1; i <= 5; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: i * 100,
		}
		err := CreateProject(project)
		require.NoError(t, err)
	}

	count, err := GetProjectCount()
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

// ==================== GetProjectListWithTotal Tests ====================

func TestGetProjectListWithTotal_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create 10 projects
	for i := 1; i <= 10; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: i * 100,
		}
		err := CreateProject(project)
		require.NoError(t, err)
	}

	// Fetch first page with total
	projects, total, err := GetProjectListWithTotal(0, 5)
	require.NoError(t, err)

	assert.Len(t, projects, 5)
	assert.Equal(t, int64(10), total)
}

func TestGetProjectListWithTotal_Empty(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	projects, total, err := GetProjectListWithTotal(0, 10)
	require.NoError(t, err)

	assert.Len(t, projects, 0)
	assert.Equal(t, int64(0), total)
}

// ==================== Edge Cases ====================

func TestProject_MaxLengthName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project with max length name (100 chars)
	maxName := ""
	for i := 0; i < 100; i++ {
		maxName += "a"
	}

	project := &Project{
		ProjectName: maxName,
		TotalBudget: 1000,
	}

	err := CreateProject(project)
	require.NoError(t, err)

	// Verify
	fetched, err := GetProjectByName(maxName)
	require.NoError(t, err)
	assert.Equal(t, maxName, fetched.ProjectName)
}

func TestProject_SpecialCharactersInName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Test with various special characters
	specialNames := []string{
		"project-with-dash",
		"project_with_underscore",
		"project.with.dot",
		"project123",
		"Project-Mixed_Case.123",
	}

	for _, name := range specialNames {
		project := &Project{
			ProjectName: name,
			TotalBudget: 1000,
		}

		err := CreateProject(project)
		require.NoError(t, err, "should create project with name: %s", name)

		fetched, err := GetProjectByName(name)
		require.NoError(t, err)
		assert.Equal(t, name, fetched.ProjectName)
	}
}

func TestProject_UnicodeInName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Test with Unicode characters
	project := &Project{
		ProjectName: "项目-测试-プロジェクト",
		TotalBudget: 1000,
	}

	err := CreateProject(project)
	require.NoError(t, err)

	fetched, err := GetProjectByName("项目-测试-プロジェクト")
	require.NoError(t, err)
	assert.Equal(t, "项目-测试-プロジェクト", fetched.ProjectName)
}

func TestProject_ConcurrentCreation(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Note: In-memory SQLite has limitations with concurrent access
	// This test creates projects sequentially but verifies the count
	// For true concurrent testing, use a file-based SQLite or other DB

	// Create multiple projects sequentially
	for i := 0; i < 10; i++ {
		project := &Project{
			ProjectName: "concurrent-project-" + string(rune('a'+i)),
			TotalBudget: i * 100,
		}
		err := CreateProject(project)
		require.NoError(t, err, "failed to create project %d", i)
	}

	// Verify all projects were created
	count, err := GetProjectCount()
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

// ==================== CreateOrUpdateAllocation Tests ====================

func TestCreateOrUpdateAllocation_CreateNew(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project first
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create allocation
	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 5000,
	}

	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	assert.Greater(t, allocation.Id, 0)
	assert.Equal(t, project.Id, allocation.ProjectId)
	assert.Equal(t, "user-123", allocation.ClientUserId)
	assert.Equal(t, 5000, allocation.AllocatedQuota)
	assert.Equal(t, 0, allocation.UsedQuota)
}

func TestCreateOrUpdateAllocation_UpdateExisting(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create a project
	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create initial allocation
	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 3000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)
	originalId := allocation.Id

	// Update allocation
	allocation2 := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 5000,
	}
	err = CreateOrUpdateAllocation(allocation2)
	require.NoError(t, err)

	// Should have same ID (updated, not created new)
	assert.Equal(t, originalId, allocation2.Id)
	assert.Equal(t, 5000, allocation2.AllocatedQuota)

	// Verify in database
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, originalId, fetched.Id)
	assert.Equal(t, 5000, fetched.AllocatedQuota)
}

func TestCreateOrUpdateAllocation_ZeroProjectId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	allocation := &ProjectAllocation{
		ProjectId:      0,
		ClientUserId:   "user-123",
		AllocatedQuota: 1000,
	}

	err := CreateOrUpdateAllocation(allocation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

func TestCreateOrUpdateAllocation_EmptyClientUserId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	allocation := &ProjectAllocation{
		ProjectId:      1,
		ClientUserId:   "",
		AllocatedQuota: 1000,
	}

	err := CreateOrUpdateAllocation(allocation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client user id is empty")
}

func TestCreateOrUpdateAllocation_NegativeQuota(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	allocation := &ProjectAllocation{
		ProjectId:      1,
		ClientUserId:   "user-123",
		AllocatedQuota: -100,
	}

	err := CreateOrUpdateAllocation(allocation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "allocated quota cannot be negative")
}

func TestCreateOrUpdateAllocation_ZeroQuota(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 0,
	}

	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)
	assert.Equal(t, 0, allocation.AllocatedQuota)
}

// ==================== GetAllocationByProjectAndUser Tests ====================

func TestGetAllocationByProjectAndUser_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-456",
		AllocatedQuota: 3000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-456")
	require.NoError(t, err)

	assert.Equal(t, allocation.Id, fetched.Id)
	assert.Equal(t, project.Id, fetched.ProjectId)
	assert.Equal(t, "user-456", fetched.ClientUserId)
	assert.Equal(t, 3000, fetched.AllocatedQuota)
}

func TestGetAllocationByProjectAndUser_NotFound(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationByProjectAndUser(999, "nonexistent-user")
	assert.Error(t, err)
}

func TestGetAllocationByProjectAndUser_ZeroProjectId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationByProjectAndUser(0, "user-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

func TestGetAllocationByProjectAndUser_EmptyUserId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationByProjectAndUser(1, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client user id is empty")
}

// ==================== GetAllocationsByProjectId Tests ====================

func TestGetAllocationsByProjectId_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create multiple allocations
	for i := 1; i <= 5; i++ {
		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-" + string(rune('a'+i-1)),
			AllocatedQuota: i * 100,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	allocations, err := GetAllocationsByProjectId(project.Id, 0, 10)
	require.NoError(t, err)
	assert.Len(t, allocations, 5)
}

func TestGetAllocationsByProjectId_Pagination(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 100000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create 10 allocations
	for i := 1; i <= 10; i++ {
		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-" + string(rune('a'+i-1)),
			AllocatedQuota: i * 100,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	// First page
	page1, err := GetAllocationsByProjectId(project.Id, 0, 5)
	require.NoError(t, err)
	assert.Len(t, page1, 5)

	// Second page
	page2, err := GetAllocationsByProjectId(project.Id, 5, 5)
	require.NoError(t, err)
	assert.Len(t, page2, 5)

	// Verify no overlap
	page1Ids := make(map[int]bool)
	for _, a := range page1 {
		page1Ids[a.Id] = true
	}
	for _, a := range page2 {
		assert.False(t, page1Ids[a.Id], "pages should not overlap")
	}
}

func TestGetAllocationsByProjectId_ZeroProjectId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationsByProjectId(0, 0, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

// ==================== GetAllocationsByClientUserId Tests ====================

func TestGetAllocationsByClientUserId_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create multiple projects
	for i := 1; i <= 3; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: 10000,
		}
		err := CreateProject(project)
		require.NoError(t, err)

		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-123",
			AllocatedQuota: i * 1000,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	allocations, err := GetAllocationsByClientUserId("user-123")
	require.NoError(t, err)
	assert.Len(t, allocations, 3)
}

func TestGetAllocationsByClientUserId_Empty(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	allocations, err := GetAllocationsByClientUserId("nonexistent-user")
	require.NoError(t, err)
	assert.Len(t, allocations, 0)
}

func TestGetAllocationsByClientUserId_EmptyUserId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationsByClientUserId("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client user id is empty")
}

// ==================== GetProjectAllocatedTotal Tests ====================

func TestGetProjectAllocatedTotal_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create allocations with known quotas
	quotas := []int{1000, 2000, 3000}
	for i, quota := range quotas {
		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-" + string(rune('a'+i)),
			AllocatedQuota: quota,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	total, err := GetProjectAllocatedTotal(project.Id)
	require.NoError(t, err)
	assert.Equal(t, 6000, total) // 1000 + 2000 + 3000
}

func TestGetProjectAllocatedTotal_NoAllocations(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	total, err := GetProjectAllocatedTotal(project.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestGetProjectAllocatedTotal_ZeroProjectId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetProjectAllocatedTotal(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}

// ==================== IncreaseUsedQuota Tests ====================

func TestIncreaseUsedQuota_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 5000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	// Increase used quota
	err = IncreaseUsedQuota(allocation.Id, 1000)
	require.NoError(t, err)

	// Verify
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 1000, fetched.UsedQuota)
}

func TestIncreaseUsedQuota_MultipleIncrements(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 5000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	// Multiple increments
	err = IncreaseUsedQuota(allocation.Id, 1000)
	require.NoError(t, err)
	err = IncreaseUsedQuota(allocation.Id, 500)
	require.NoError(t, err)
	err = IncreaseUsedQuota(allocation.Id, 200)
	require.NoError(t, err)

	// Verify total
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 1700, fetched.UsedQuota)
}

func TestIncreaseUsedQuota_ExceedsAllocated(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 1000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	// Try to exceed allocated quota
	err = IncreaseUsedQuota(allocation.Id, 1500)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project quota exceeded")

	// Verify used_quota unchanged
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 0, fetched.UsedQuota)
}

func TestIncreaseUsedQuota_ExactlyAllocated(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 1000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	// Use exactly the allocated amount
	err = IncreaseUsedQuota(allocation.Id, 1000)
	require.NoError(t, err)

	// Verify
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 1000, fetched.UsedQuota)
}

func TestIncreaseUsedQuota_ZeroAllocationId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	err := IncreaseUsedQuota(0, 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "allocation id is empty")
}

func TestIncreaseUsedQuota_NegativeDelta(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	err := IncreaseUsedQuota(1, -100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delta cannot be negative")
}

func TestIncreaseUsedQuota_ZeroDelta(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 1000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	// Zero delta should be a no-op
	err = IncreaseUsedQuota(allocation.Id, 0)
	require.NoError(t, err)

	// Verify unchanged
	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 0, fetched.UsedQuota)
}

// ==================== IncreaseUsedQuotaByProjectAndUser Tests ====================

func TestIncreaseUsedQuotaByProjectAndUser_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 5000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	err = IncreaseUsedQuotaByProjectAndUser(project.Id, "user-123", 1000)
	require.NoError(t, err)

	fetched, err := GetAllocationByProjectAndUser(project.Id, "user-123")
	require.NoError(t, err)
	assert.Equal(t, 1000, fetched.UsedQuota)
}

func TestIncreaseUsedQuotaByProjectAndUser_ExceedsAllocated(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	allocation := &ProjectAllocation{
		ProjectId:      project.Id,
		ClientUserId:   "user-123",
		AllocatedQuota: 1000,
	}
	err = CreateOrUpdateAllocation(allocation)
	require.NoError(t, err)

	err = IncreaseUsedQuotaByProjectAndUser(project.Id, "user-123", 1500)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project quota exceeded")
}

// ==================== GetUserProjectRemainingQuota Tests ====================

func TestGetUserProjectRemainingQuota_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	// Create multiple projects with allocations for the same user
	for i := 1; i <= 3; i++ {
		project := &Project{
			ProjectName: "project-" + string(rune('a'+i-1)),
			TotalBudget: 10000,
		}
		err := CreateProject(project)
		require.NoError(t, err)

		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-123",
			AllocatedQuota: 1000 * i, // 1000, 2000, 3000
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)

		// Use some quota
		err = IncreaseUsedQuota(allocation.Id, 100*i) // 100, 200, 300
		require.NoError(t, err)
	}

	// Total allocated: 6000, Total used: 600, Remaining: 5400
	remaining, err := GetUserProjectRemainingQuota("user-123")
	require.NoError(t, err)
	assert.Equal(t, 5400, remaining)
}

func TestGetUserProjectRemainingQuota_NoAllocations(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	remaining, err := GetUserProjectRemainingQuota("nonexistent-user")
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)
}

func TestGetUserProjectRemainingQuota_EmptyUserId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetUserProjectRemainingQuota("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client user id is empty")
}

// ==================== GetAllocationsByProjectIdWithTotal Tests ====================

func TestGetAllocationsByProjectIdWithTotal_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 100000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	// Create 10 allocations
	for i := 1; i <= 10; i++ {
		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-" + string(rune('a'+i-1)),
			AllocatedQuota: i * 100,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	allocations, total, err := GetAllocationsByProjectIdWithTotal(project.Id, 0, 5)
	require.NoError(t, err)

	assert.Len(t, allocations, 5)
	assert.Equal(t, int64(10), total)
}

// ==================== GetAllocationCount Tests ====================

func TestGetAllocationCount_Success(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	project := &Project{
		ProjectName: "test-project",
		TotalBudget: 10000,
	}
	err := CreateProject(project)
	require.NoError(t, err)

	for i := 1; i <= 5; i++ {
		allocation := &ProjectAllocation{
			ProjectId:      project.Id,
			ClientUserId:   "user-" + string(rune('a'+i-1)),
			AllocatedQuota: 100,
		}
		err = CreateOrUpdateAllocation(allocation)
		require.NoError(t, err)
	}

	count, err := GetAllocationCount(project.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestGetAllocationCount_ZeroProjectId(t *testing.T) {
	db := setupProjectTestDB(t)
	defer cleanupProjectTestDB(t, db)

	_, err := GetAllocationCount(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project id is empty")
}
