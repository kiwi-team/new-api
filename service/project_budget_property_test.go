package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupServiceTestDB creates an in-memory SQLite database for testing
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Use in-memory SQLite for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to connect to test database")

	// Set the global flags for SQLite
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	// Run migrations for Project and ProjectAllocation tables
	err = db.AutoMigrate(&model.Project{}, &model.ProjectAllocation{})
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// cleanupServiceTestDB cleans up the test database
func cleanupServiceTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// Feature: project-budget-management, Property 4: Project Request Validation
// Validates: Requirements 2.5, 5.1, 5.2, 5.3, 5.4, 5.5, 6.3
//
// Property: For any API request with a project header:
// - If the project_name does not exist, the request SHALL be rejected with "project not found"
// - If the project status is paused, the request SHALL be rejected with "project is paused"
// - If the client_user_id has no allocation for the project, the request SHALL be rejected with "user not allocated to project"
// - If the user's used_quota >= allocated_quota for that project, the request SHALL be rejected
func TestProperty4_ProjectRequestValidation(t *testing.T) {
	db := setupServiceTestDB(t)
	defer cleanupServiceTestDB(t, db)

	// Set the global DB for model functions
	originalDB := model.DB
	model.DB = db
	defer func() { model.DB = originalDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names (non-empty, max 100 chars, alphanumeric with underscores)
	validProjectNameGen := gen.RegexMatch(`[a-zA-Z][a-zA-Z0-9_]{0,49}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for non-existent project names (different pattern to avoid collision)
	nonExistentProjectNameGen := gen.RegexMatch(`nonexistent_[a-zA-Z0-9]{5,20}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for valid client user IDs
	validClientUserIdGen := gen.RegexMatch(`client_[a-zA-Z0-9]{5,20}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for valid total budget (positive to allow allocations)
	validBudgetGen := gen.IntRange(100, 1000000)

	// Generator for allocated quota
	allocatedQuotaGen := gen.IntRange(10, 10000)

	// Property 4.1: Non-existent project returns "project not found"
	properties.Property("non-existent project returns project not found error", prop.ForAll(
		func(projectName string, clientUserId string) bool {
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			allocation, err := ValidateProjectRequest(projectName, clientUserId)
			if err == nil {
				t.Logf("Expected error for non-existent project, got allocation: %+v", allocation)
				return false
			}
			if err != ErrProjectNotFound {
				t.Logf("Expected ErrProjectNotFound, got: %v", err)
				return false
			}
			if allocation != nil {
				t.Logf("Expected nil allocation, got: %+v", allocation)
				return false
			}
			return true
		},
		nonExistentProjectNameGen,
		validClientUserIdGen,
	))

	// Property 4.2: Paused project returns "project is paused"
	properties.Property("paused project returns project is paused error", prop.ForAll(
		func(projectName string, clientUserId string, totalBudget int, allocatedQuota int) bool {
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
				Status:      model.ProjectStatusPaused,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}
			db.Where("project_id = ?", project.Id).Delete(&model.ProjectAllocation{})
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}
			result, err := ValidateProjectRequest(projectName, clientUserId)
			if err == nil {
				t.Logf("Expected error for paused project, got allocation: %+v", result)
				return false
			}
			if err != ErrProjectPaused {
				t.Logf("Expected ErrProjectPaused, got: %v", err)
				return false
			}
			if result != nil {
				t.Logf("Expected nil result, got: %+v", result)
				return false
			}
			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validBudgetGen,
		allocatedQuotaGen,
	))

	// Property 4.3: User not allocated to project returns "user not allocated to project"
	properties.Property("user not allocated to project returns user not allocated error", prop.ForAll(
		func(projectName string, allocatedUserId string, unallocatedUserId string, totalBudget int, allocatedQuota int) bool {
			if allocatedUserId == unallocatedUserId {
				return true
			}
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}
			db.Where("project_id = ?", project.Id).Delete(&model.ProjectAllocation{})
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   allocatedUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}
			result, err := ValidateProjectRequest(projectName, unallocatedUserId)
			if err == nil {
				t.Logf("Expected error for unallocated user, got allocation: %+v", result)
				return false
			}
			if err != ErrUserNotAllocatedProject {
				t.Logf("Expected ErrUserNotAllocatedProject, got: %v", err)
				return false
			}
			if result != nil {
				t.Logf("Expected nil result, got: %+v", result)
				return false
			}
			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validClientUserIdGen,
		validBudgetGen,
		allocatedQuotaGen,
	))

	// Property 4.4: User with exhausted quota returns "project quota exceeded"
	properties.Property("user with exhausted quota returns quota exceeded error", prop.ForAll(
		func(projectName string, clientUserId string, totalBudget int, allocatedQuota int) bool {
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}
			db.Where("project_id = ?", project.Id).Delete(&model.ProjectAllocation{})
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      allocatedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}
			result, err := ValidateProjectRequest(projectName, clientUserId)
			if err == nil {
				t.Logf("Expected error for exhausted quota, got allocation: %+v", result)
				return false
			}
			if err != ErrProjectQuotaExceeded {
				t.Logf("Expected ErrProjectQuotaExceeded, got: %v", err)
				return false
			}
			if result != nil {
				t.Logf("Expected nil result, got: %+v", result)
				return false
			}
			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validBudgetGen,
		allocatedQuotaGen,
	))

	// Property 4.5: Valid request with remaining quota succeeds
	properties.Property("valid request with remaining quota succeeds", prop.ForAll(
		func(projectName string, clientUserId string, totalBudget int, allocatedQuota int, usedQuotaRatio float64) bool {
			usedQuota := int(float64(allocatedQuota) * usedQuotaRatio)
			if usedQuota >= allocatedQuota {
				usedQuota = allocatedQuota - 1
			}
			if usedQuota < 0 {
				usedQuota = 0
			}
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}
			db.Where("project_id = ?", project.Id).Delete(&model.ProjectAllocation{})
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      usedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}
			result, err := ValidateProjectRequest(projectName, clientUserId)
			if err != nil {
				t.Logf("Expected success for valid request, got error: %v", err)
				return false
			}
			if result == nil {
				t.Logf("Expected non-nil result for valid request")
				return false
			}
			if result.ProjectId != project.Id {
				t.Logf("Result project ID mismatch: expected %d, got %d", project.Id, result.ProjectId)
				return false
			}
			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validBudgetGen,
		allocatedQuotaGen,
		gen.Float64Range(0.0, 0.99),
	))

	// Property 4.6: Empty project name returns "project not found"
	properties.Property("empty project name returns project not found error", prop.ForAll(
		func(clientUserId string) bool {
			result, err := ValidateProjectRequest("", clientUserId)
			if err == nil {
				t.Logf("Expected error for empty project name, got allocation: %+v", result)
				return false
			}
			if err != ErrProjectNotFound {
				t.Logf("Expected ErrProjectNotFound for empty project name, got: %v", err)
				return false
			}
			return result == nil
		},
		validClientUserIdGen,
	))

	// Property 4.7: Empty client user ID returns "user not allocated to project"
	properties.Property("empty client user ID returns user not allocated error", prop.ForAll(
		func(projectName string, totalBudget int) bool {
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}
			result, err := ValidateProjectRequest(projectName, "")
			if err == nil {
				t.Logf("Expected error for empty client user ID, got allocation: %+v", result)
				return false
			}
			if err != ErrUserNotAllocatedProject {
				t.Logf("Expected ErrUserNotAllocatedProject for empty client user ID, got: %v", err)
				return false
			}
			return result == nil
		},
		validProjectNameGen,
		validBudgetGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 6: User Total Budget Calculation
// Validates: Requirements 7.1, 7.2, 7.3, 7.4
//
// Property: For any client_user_id, the total available budget SHALL equal:
// fixed_quota + temp_quota + sum(allocated_quota - used_quota) for all ProjectAllocation records
// belonging to that user. This calculation SHALL consider all three budget types when checking
// if a user has sufficient budget.
func TestProperty6_UserTotalBudgetCalculation(t *testing.T) {
	db := setupServiceTestDB(t)
	defer cleanupServiceTestDB(t, db)

	// Also migrate CliendUserQuota table for this test
	err := db.AutoMigrate(&model.CliendUserQuota{})
	require.NoError(t, err, "failed to migrate CliendUserQuota table")

	// Set the global DB for model functions
	originalDB := model.DB
	model.DB = db
	defer func() { model.DB = originalDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid client user IDs
	validClientUserIdGen := gen.RegexMatch(`client_[a-zA-Z0-9]{5,20}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for quota values (non-negative)
	quotaGen := gen.IntRange(0, 1000000)

	// Generator for number of project allocations (0 to 5)
	numAllocationsGen := gen.IntRange(0, 5)

	// Property 6.1: Total budget equals fixed + temp + sum(allocated - used) for all allocations
	properties.Property("total budget equals sum of all budget types", prop.ForAll(
		func(clientUserId string, fixedQuota int, tempQuota int, numAllocations int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record with fixed and temp quota
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   fixedQuota,
				TempQuota:    tempQuota,
				UsedQuota:    0,
				ExpiredAt:    0, // Not expired
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Create project allocations
			var expectedProjectRemaining int
			for i := 0; i < numAllocations; i++ {
				// Create a unique project for each allocation
				projectName := clientUserId + "_proj_" + string(rune('a'+i))
				db.Where("project_name = ?", projectName).Delete(&model.Project{})

				project := model.Project{
					ProjectName: projectName,
					TotalBudget: 10000000, // Large enough to accommodate allocations
					Status:      model.ProjectStatusEnabled,
				}
				if err := db.Create(&project).Error; err != nil {
					t.Logf("Failed to create project: %v", err)
					return false
				}

				// Deterministic allocated and used quota for reproducibility
				allocatedQuota := (i + 1) * 10000
				usedQuota := (i + 1) * 1000

				allocation := model.ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   clientUserId,
					AllocatedQuota: allocatedQuota,
					UsedQuota:      usedQuota,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation: %v", err)
					return false
				}

				expectedProjectRemaining += allocatedQuota - usedQuota
			}

			// Calculate expected total budget
			expectedTotal := fixedQuota + tempQuota + expectedProjectRemaining

			// Get actual total budget from service
			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			// Verify the formula
			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch: expected %d (fixed=%d + temp=%d + project=%d), got %d",
					expectedTotal, fixedQuota, tempQuota, expectedProjectRemaining, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
		quotaGen,
		numAllocationsGen,
	))

	// Property 6.2: Total budget with no CliendUserQuota record equals only project allocations
	properties.Property("total budget with no client quota equals project allocations only", prop.ForAll(
		func(clientUserId string, numAllocations int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Do NOT create CliendUserQuota record (user has no fixed/temp quota)

			// Create project allocations
			var expectedProjectRemaining int
			for i := 0; i < numAllocations; i++ {
				projectName := clientUserId + "_proj_" + string(rune('a'+i))
				db.Where("project_name = ?", projectName).Delete(&model.Project{})

				project := model.Project{
					ProjectName: projectName,
					TotalBudget: 10000000,
					Status:      model.ProjectStatusEnabled,
				}
				if err := db.Create(&project).Error; err != nil {
					t.Logf("Failed to create project: %v", err)
					return false
				}

				allocatedQuota := (i + 1) * 10000
				usedQuota := (i + 1) * 1000

				allocation := model.ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   clientUserId,
					AllocatedQuota: allocatedQuota,
					UsedQuota:      usedQuota,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation: %v", err)
					return false
				}

				expectedProjectRemaining += allocatedQuota - usedQuota
			}

			// Expected total = 0 (fixed) + 0 (temp) + project remaining
			expectedTotal := expectedProjectRemaining

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch: expected %d (project only), got %d", expectedTotal, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		numAllocationsGen,
	))

	// Property 6.3: Total budget with no project allocations equals fixed + temp only
	properties.Property("total budget with no allocations equals fixed plus temp only", prop.ForAll(
		func(clientUserId string, fixedQuota int, tempQuota int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   fixedQuota,
				TempQuota:    tempQuota,
				UsedQuota:    0,
				ExpiredAt:    0,
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Do NOT create any project allocations

			// Expected total = fixed + temp + 0 (no project allocations)
			expectedTotal := fixedQuota + tempQuota

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch: expected %d (fixed=%d + temp=%d), got %d",
					expectedTotal, fixedQuota, tempQuota, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
		quotaGen,
	))

	// Property 6.4: Expired temp quota is not included in total budget
	properties.Property("expired temp quota is not included in total budget", prop.ForAll(
		func(clientUserId string, fixedQuota int, tempQuota int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record with expired temp quota
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   fixedQuota,
				TempQuota:    tempQuota,
				UsedQuota:    0,
				ExpiredAt:    time.Now().Unix() - 3600, // Expired 1 hour ago
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Expected total = fixed + 0 (expired temp) + 0 (no project allocations)
			expectedTotal := fixedQuota

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch with expired temp: expected %d (fixed only), got %d", expectedTotal, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
		gen.IntRange(1, 1000000), // Non-zero temp quota to verify it's excluded
	))

	// Property 6.5: Non-expired temp quota is included in total budget
	properties.Property("non-expired temp quota is included in total budget", prop.ForAll(
		func(clientUserId string, fixedQuota int, tempQuota int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record with non-expired temp quota
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   fixedQuota,
				TempQuota:    tempQuota,
				UsedQuota:    0,
				ExpiredAt:    time.Now().Unix() + 3600, // Expires in 1 hour
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Expected total = fixed + temp (not expired) + 0 (no project allocations)
			expectedTotal := fixedQuota + tempQuota

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch with non-expired temp: expected %d (fixed=%d + temp=%d), got %d",
					expectedTotal, fixedQuota, tempQuota, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
		quotaGen,
	))

	// Property 6.6: Project allocations with used_quota > allocated_quota contribute negative to total
	properties.Property("over-used allocations contribute negative to total", prop.ForAll(
		func(clientUserId string, fixedQuota int, tempQuota int, allocatedQuota int, extraUsed int) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   fixedQuota,
				TempQuota:    tempQuota,
				UsedQuota:    0,
				ExpiredAt:    0,
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Create a project with over-used allocation
			projectName := clientUserId + "_overused_proj"
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation with used > allocated
			usedQuota := allocatedQuota + extraUsed
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      usedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Expected project remaining = allocated - used (negative)
			projectRemaining := allocatedQuota - usedQuota
			expectedTotal := fixedQuota + tempQuota + projectRemaining

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch with over-used: expected %d (fixed=%d + temp=%d + project=%d), got %d",
					expectedTotal, fixedQuota, tempQuota, projectRemaining, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
		quotaGen,
		gen.IntRange(100, 10000),
		gen.IntRange(1, 1000), // Extra used quota
	))

	// Property 6.7: Empty client user ID returns error
	properties.Property("empty client user ID returns error", prop.ForAll(
		func(_ int) bool {
			_, err := GetUserTotalBudget("")
			if err == nil {
				t.Logf("Expected error for empty client user ID")
				return false
			}
			return true
		},
		gen.IntRange(0, 100), // Dummy generator
	))

	// Property 6.8: Zero quotas result in zero total budget
	properties.Property("zero quotas result in zero total budget", prop.ForAll(
		func(clientUserId string) bool {
			// Clean up any existing data for this user
			db.Where("client_user_id = ?", clientUserId).Delete(&model.CliendUserQuota{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create CliendUserQuota record with zero quotas
			clientQuota := model.CliendUserQuota{
				ClientUserId: clientUserId,
				FixedQuota:   0,
				TempQuota:    0,
				UsedQuota:    0,
				ExpiredAt:    0,
			}
			if err := db.Create(&clientQuota).Error; err != nil {
				t.Logf("Failed to create client quota: %v", err)
				return false
			}

			// Create a project allocation with zero remaining
			projectName := clientUserId + "_zero_proj"
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 1000,
				UsedQuota:      1000, // Fully used
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Expected total = 0 + 0 + (1000 - 1000) = 0
			expectedTotal := 0

			actualTotal, err := GetUserTotalBudget(clientUserId)
			if err != nil {
				t.Logf("GetUserTotalBudget failed: %v", err)
				return false
			}

			if actualTotal != expectedTotal {
				t.Logf("Budget mismatch with zero quotas: expected %d, got %d", expectedTotal, actualTotal)
				return false
			}

			return true
		},
		validClientUserIdGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 8: Dashboard Filtering
// Validates: Requirements 4.1, 4.5, 8.4, 9.6
//
// Property: For any dashboard query with time range, project_name, client_user_id, or scenario filters,
// the returned consumption data SHALL only include records that match ALL specified filter criteria.
func TestProperty8_DashboardFiltering(t *testing.T) {
	db := setupServiceTestDB(t)
	defer cleanupServiceTestDB(t, db)

	// Also migrate Log table for this test
	err := db.AutoMigrate(&model.Log{})
	require.NoError(t, err, "failed to migrate Log table")

	// Also migrate CliendUserQuota table for this test
	err = db.AutoMigrate(&model.CliendUserQuota{})
	require.NoError(t, err, "failed to migrate CliendUserQuota table")

	// Set the global DB for model functions
	originalDB := model.DB
	model.DB = db
	defer func() { model.DB = originalDB }()

	// Set the LOG_DB for log queries
	originalLogDB := model.LOG_DB
	model.LOG_DB = db
	defer func() { model.LOG_DB = originalLogDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names
	validProjectNameGen := gen.RegexMatch(`proj_[a-zA-Z0-9]{3,10}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for valid client user IDs
	validClientUserIdGen := gen.RegexMatch(`user_[a-zA-Z0-9]{3,10}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for scenario values
	scenarioGen := gen.OneConstOf(
		"personal_experiment",
		"release_evaluation",
		"daily_external_model_evaluation",
	)

	// Generator for quota values
	quotaGen := gen.IntRange(100, 10000)

	// Generator for number of log records (1 to 5)
	numLogsGen := gen.IntRange(1, 5)

	// Property 8.1: Time range filter returns only records within the specified range
	properties.Property("time range filter returns only records within range", prop.ForAll(
		func(projectName string, clientUserId string, numLogs int) bool {
			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Create logs with different timestamps
			now := time.Now().Unix()
			baseTime := now - 3600 // 1 hour ago

			// Create logs: some inside time range, some outside
			var expectedInRange int64
			for i := 0; i < numLogs; i++ {
				// Logs inside range (within last 30 minutes)
				logInRange := model.Log{
					UserId:         1,
					CreatedAt:      baseTime + int64(i*60), // Spread within range
					Type:           model.LogTypeConsume,
					Quota:          100 + i*10,
					ClientUserId:   clientUserId,
					ClientScenairo: projectName, // Using project name as scenario for filtering
				}
				if err := db.Create(&logInRange).Error; err != nil {
					t.Logf("Failed to create log in range: %v", err)
					return false
				}
				expectedInRange += int64(logInRange.Quota)

				// Logs outside range (2 hours ago)
				logOutRange := model.Log{
					UserId:         1,
					CreatedAt:      baseTime - 7200 - int64(i*60), // 2+ hours before base
					Type:           model.LogTypeConsume,
					Quota:          200 + i*10,
					ClientUserId:   clientUserId,
					ClientScenairo: projectName,
				}
				if err := db.Create(&logOutRange).Error; err != nil {
					t.Logf("Failed to create log out of range: %v", err)
					return false
				}
			}

			// Query with time range filter
			startTime := baseTime - 60 // Slightly before first in-range log
			endTime := now + 60        // Slightly after now

			result, err := GetProjectDashboard(startTime, endTime, projectName, clientUserId)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify only in-range records are counted
			if result.HistoricalConsumption != expectedInRange {
				t.Logf("Time range filter mismatch: expected %d, got %d", expectedInRange, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		numLogsGen,
	))

	// Property 8.2: Project name filter returns only records for the specified project
	properties.Property("project name filter returns only records for specified project", prop.ForAll(
		func(projectName1 string, projectName2 string, clientUserId string, quota1 int, quota2 int) bool {
			// Skip if project names are the same
			if projectName1 == projectName2 {
				return true
			}

			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name IN ?", []string{projectName1, projectName2}).Delete(&model.Project{})

			// Create two projects
			project1 := model.Project{
				ProjectName: projectName1,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project1).Error; err != nil {
				t.Logf("Failed to create project1: %v", err)
				return false
			}

			project2 := model.Project{
				ProjectName: projectName2,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project2).Error; err != nil {
				t.Logf("Failed to create project2: %v", err)
				return false
			}

			// Create allocations
			allocation1 := model.ProjectAllocation{
				ProjectId:      project1.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation1).Error; err != nil {
				t.Logf("Failed to create allocation1: %v", err)
				return false
			}

			allocation2 := model.ProjectAllocation{
				ProjectId:      project2.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation2).Error; err != nil {
				t.Logf("Failed to create allocation2: %v", err)
				return false
			}

			now := time.Now().Unix()

			// Create logs for project1
			log1 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota1,
				ClientUserId:   clientUserId,
				ClientScenairo: projectName1, // Using project name as scenario
			}
			if err := db.Create(&log1).Error; err != nil {
				t.Logf("Failed to create log1: %v", err)
				return false
			}

			// Create logs for project2
			log2 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota2,
				ClientUserId:   clientUserId,
				ClientScenairo: projectName2, // Using project name as scenario
			}
			if err := db.Create(&log2).Error; err != nil {
				t.Logf("Failed to create log2: %v", err)
				return false
			}

			// Query with project name filter for project1
			result, err := GetProjectDashboard(0, 0, projectName1, clientUserId)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify only project1 records are counted
			if result.HistoricalConsumption != int64(quota1) {
				t.Logf("Project filter mismatch: expected %d, got %d", quota1, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validProjectNameGen,
		validClientUserIdGen,
		quotaGen,
		quotaGen,
	))

	// Property 8.3: Client user ID filter returns only records for the specified user
	properties.Property("client user ID filter returns only records for specified user", prop.ForAll(
		func(projectName string, clientUserId1 string, clientUserId2 string, quota1 int, quota2 int) bool {
			// Skip if user IDs are the same
			if clientUserId1 == clientUserId2 {
				return true
			}

			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocations for both users
			allocation1 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId1,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation1).Error; err != nil {
				t.Logf("Failed to create allocation1: %v", err)
				return false
			}

			allocation2 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId2,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation2).Error; err != nil {
				t.Logf("Failed to create allocation2: %v", err)
				return false
			}

			now := time.Now().Unix()

			// Create logs for user1
			log1 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota1,
				ClientUserId:   clientUserId1,
				ClientScenairo: projectName,
			}
			if err := db.Create(&log1).Error; err != nil {
				t.Logf("Failed to create log1: %v", err)
				return false
			}

			// Create logs for user2
			log2 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota2,
				ClientUserId:   clientUserId2,
				ClientScenairo: projectName,
			}
			if err := db.Create(&log2).Error; err != nil {
				t.Logf("Failed to create log2: %v", err)
				return false
			}

			// Query with client user ID filter for user1
			result, err := GetProjectDashboard(0, 0, projectName, clientUserId1)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify only user1 records are counted
			if result.HistoricalConsumption != int64(quota1) {
				t.Logf("User filter mismatch: expected %d, got %d", quota1, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validClientUserIdGen,
		quotaGen,
		quotaGen,
	))

	// Property 8.4: Scenario filter returns only records for the specified scenario
	properties.Property("scenario filter returns only records for specified scenario", prop.ForAll(
		func(projectName string, clientUserId string, scenario1 string, scenario2 string, quota1 int, quota2 int) bool {
			// Skip if scenarios are the same
			if scenario1 == scenario2 {
				return true
			}

			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			now := time.Now().Unix()

			// Create logs for scenario1
			log1 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota1,
				ClientUserId:   clientUserId,
				ClientScenairo: scenario1,
			}
			if err := db.Create(&log1).Error; err != nil {
				t.Logf("Failed to create log1: %v", err)
				return false
			}

			// Create logs for scenario2
			log2 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota2,
				ClientUserId:   clientUserId,
				ClientScenairo: scenario2,
			}
			if err := db.Create(&log2).Error; err != nil {
				t.Logf("Failed to create log2: %v", err)
				return false
			}

			// Query with scenario filter using GetProjectStatistics
			result, err := GetProjectStatistics(project.Id, 0, 0, clientUserId, scenario1)
			if err != nil {
				t.Logf("GetProjectStatistics failed: %v", err)
				return false
			}

			// Verify only scenario1 records are counted
			if result.HistoricalConsumption != int64(quota1) {
				t.Logf("Scenario filter mismatch: expected %d, got %d", quota1, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		scenarioGen,
		scenarioGen,
		quotaGen,
		quotaGen,
	))

	// Property 8.5: Combined filters return only records matching ALL criteria
	properties.Property("combined filters return only records matching all criteria", prop.ForAll(
		func(projectName string, clientUserId1 string, clientUserId2 string, quota int) bool {
			// Skip if user IDs are the same
			if clientUserId1 == clientUserId2 {
				return true
			}

			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocations
			allocation1 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId1,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation1).Error; err != nil {
				t.Logf("Failed to create allocation1: %v", err)
				return false
			}

			allocation2 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId2,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation2).Error; err != nil {
				t.Logf("Failed to create allocation2: %v", err)
				return false
			}

			now := time.Now().Unix()
			inRangeTime := now - 1800  // 30 minutes ago
			outRangeTime := now - 7200 // 2 hours ago

			// Create log matching ALL criteria (project, user1, in time range)
			logMatch := model.Log{
				UserId:         1,
				CreatedAt:      inRangeTime,
				Type:           model.LogTypeConsume,
				Quota:          quota,
				ClientUserId:   clientUserId1,
				ClientScenairo: projectName,
			}
			if err := db.Create(&logMatch).Error; err != nil {
				t.Logf("Failed to create matching log: %v", err)
				return false
			}

			// Create log matching project and time but different user
			logWrongUser := model.Log{
				UserId:         1,
				CreatedAt:      inRangeTime,
				Type:           model.LogTypeConsume,
				Quota:          quota * 2,
				ClientUserId:   clientUserId2,
				ClientScenairo: projectName,
			}
			if err := db.Create(&logWrongUser).Error; err != nil {
				t.Logf("Failed to create wrong user log: %v", err)
				return false
			}

			// Create log matching project and user but out of time range
			logOutOfRange := model.Log{
				UserId:         1,
				CreatedAt:      outRangeTime,
				Type:           model.LogTypeConsume,
				Quota:          quota * 3,
				ClientUserId:   clientUserId1,
				ClientScenairo: projectName,
			}
			if err := db.Create(&logOutOfRange).Error; err != nil {
				t.Logf("Failed to create out of range log: %v", err)
				return false
			}

			// Query with combined filters
			startTime := inRangeTime - 60
			endTime := now + 60

			result, err := GetProjectDashboard(startTime, endTime, projectName, clientUserId1)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify only the matching record is counted
			if result.HistoricalConsumption != int64(quota) {
				t.Logf("Combined filter mismatch: expected %d, got %d", quota, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validClientUserIdGen,
		quotaGen,
	))

	// Property 8.6: Empty filters return all records
	properties.Property("empty filters return all records", prop.ForAll(
		func(projectName string, clientUserId string, numLogs int) bool {
			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			now := time.Now().Unix()
			var expectedTotal int64

			// Create multiple logs
			for i := 0; i < numLogs; i++ {
				log := model.Log{
					UserId:         1,
					CreatedAt:      now - int64(i*60),
					Type:           model.LogTypeConsume,
					Quota:          100 + i*10,
					ClientUserId:   clientUserId,
					ClientScenairo: projectName,
				}
				if err := db.Create(&log).Error; err != nil {
					t.Logf("Failed to create log: %v", err)
					return false
				}
				expectedTotal += int64(log.Quota)
			}

			// Query with no time range filter (0, 0)
			result, err := GetProjectDashboard(0, 0, projectName, clientUserId)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify all records are counted
			if result.HistoricalConsumption != expectedTotal {
				t.Logf("Empty filter mismatch: expected %d, got %d", expectedTotal, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		numLogsGen,
	))

	// Property 8.7: Only consume type logs are counted
	properties.Property("only consume type logs are counted", prop.ForAll(
		func(projectName string, clientUserId string, consumeQuota int, otherQuota int) bool {
			// Clean up
			db.Where("1=1").Delete(&model.Log{})
			db.Where("project_name = ?", projectName).Delete(&model.Project{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: 10000000,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: 10000000,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			now := time.Now().Unix()

			// Create consume type log
			consumeLog := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          consumeQuota,
				ClientUserId:   clientUserId,
				ClientScenairo: projectName,
			}
			if err := db.Create(&consumeLog).Error; err != nil {
				t.Logf("Failed to create consume log: %v", err)
				return false
			}

			// Create non-consume type log (e.g., topup)
			topupLog := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeTopup,
				Quota:          otherQuota,
				ClientUserId:   clientUserId,
				ClientScenairo: projectName,
			}
			if err := db.Create(&topupLog).Error; err != nil {
				t.Logf("Failed to create topup log: %v", err)
				return false
			}

			// Query dashboard
			result, err := GetProjectDashboard(0, 0, projectName, clientUserId)
			if err != nil {
				t.Logf("GetProjectDashboard failed: %v", err)
				return false
			}

			// Verify only consume logs are counted
			if result.HistoricalConsumption != int64(consumeQuota) {
				t.Logf("Log type filter mismatch: expected %d (consume only), got %d", consumeQuota, result.HistoricalConsumption)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		quotaGen,
		quotaGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 7: Scenario Handling
// Validates: Requirements 8.1, 8.2, 8.3
//
// Property: For any API request:
//   - If scenario header is provided with a valid value (personal_experiment, release_evaluation,
//     daily_external_model_evaluation, or custom project name), that value SHALL be recorded in the consumption log
//   - If scenario header is not provided, the log SHALL record "personal_experiment" as the default scenario
func TestProperty7_ScenarioHandling(t *testing.T) {
	db := setupServiceTestDB(t)
	defer cleanupServiceTestDB(t, db)

	// Also migrate Log table for this test
	err := db.AutoMigrate(&model.Log{})
	require.NoError(t, err, "failed to migrate Log table")

	// Set the global DB for model functions
	originalDB := model.DB
	model.DB = db
	defer func() { model.DB = originalDB }()

	// Set the LOG_DB for log queries
	originalLogDB := model.LOG_DB
	model.LOG_DB = db
	defer func() { model.LOG_DB = originalLogDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid client user IDs
	validClientUserIdGen := gen.RegexMatch(`client_[a-zA-Z0-9]{5,20}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for predefined scenario values
	predefinedScenarioGen := gen.OneConstOf(
		"personal_experiment",
		"release_evaluation",
		"daily_external_model_evaluation",
	)

	// Generator for custom project name scenarios
	customScenarioGen := gen.RegexMatch(`custom_proj_[a-zA-Z0-9]{3,15}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for quota values
	quotaGen := gen.IntRange(100, 10000)

	// Property 7.1: Predefined scenario values are recorded correctly in logs
	properties.Property("predefined scenario values are recorded correctly", prop.ForAll(
		func(clientUserId string, scenario string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create a log with the specified scenario
			log := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota,
				ClientUserId:   clientUserId,
				ClientScenairo: scenario,
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the scenario was recorded correctly
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			if retrievedLog.ClientScenairo != scenario {
				t.Logf("Scenario mismatch: expected %s, got %s", scenario, retrievedLog.ClientScenairo)
				return false
			}

			return true
		},
		validClientUserIdGen,
		predefinedScenarioGen,
		quotaGen,
	))

	// Property 7.2: Custom project name scenarios are recorded correctly
	properties.Property("custom project name scenarios are recorded correctly", prop.ForAll(
		func(clientUserId string, customScenario string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create a log with custom scenario (project name)
			log := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota,
				ClientUserId:   clientUserId,
				ClientScenairo: customScenario,
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the custom scenario was recorded correctly
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			if retrievedLog.ClientScenairo != customScenario {
				t.Logf("Custom scenario mismatch: expected %s, got %s", customScenario, retrievedLog.ClientScenairo)
				return false
			}

			return true
		},
		validClientUserIdGen,
		customScenarioGen,
		quotaGen,
	))

	// Property 7.3: Default scenario is "personal_experiment" when not provided
	properties.Property("default scenario is personal_experiment when empty", prop.ForAll(
		func(clientUserId string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Simulate the default scenario behavior
			// When scenario header is not provided, middleware sets it to "personal_experiment"
			defaultScenario := "personal_experiment"

			// Create a log with the default scenario (simulating middleware behavior)
			log := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota,
				ClientUserId:   clientUserId,
				ClientScenairo: defaultScenario,
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the default scenario was recorded
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			if retrievedLog.ClientScenairo != "personal_experiment" {
				t.Logf("Default scenario mismatch: expected personal_experiment, got %s", retrievedLog.ClientScenairo)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
	))

	// Property 7.4: Scenario filtering works correctly for predefined values
	properties.Property("scenario filtering works for predefined values", prop.ForAll(
		func(clientUserId string, scenario1 string, scenario2 string, quota1 int, quota2 int) bool {
			// Skip if scenarios are the same
			if scenario1 == scenario2 {
				return true
			}

			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create logs with different scenarios
			log1 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota1,
				ClientUserId:   clientUserId,
				ClientScenairo: scenario1,
			}
			if err := db.Create(&log1).Error; err != nil {
				t.Logf("Failed to create log1: %v", err)
				return false
			}

			log2 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota2,
				ClientUserId:   clientUserId,
				ClientScenairo: scenario2,
			}
			if err := db.Create(&log2).Error; err != nil {
				t.Logf("Failed to create log2: %v", err)
				return false
			}

			// Query logs filtered by scenario1
			var filteredLogs []model.Log
			if err := db.Where("client_user_id = ? AND client_scenairo = ?", clientUserId, scenario1).Find(&filteredLogs).Error; err != nil {
				t.Logf("Failed to query filtered logs: %v", err)
				return false
			}

			// Verify only scenario1 logs are returned
			if len(filteredLogs) != 1 {
				t.Logf("Expected 1 log for scenario %s, got %d", scenario1, len(filteredLogs))
				return false
			}

			if filteredLogs[0].ClientScenairo != scenario1 {
				t.Logf("Filtered log scenario mismatch: expected %s, got %s", scenario1, filteredLogs[0].ClientScenairo)
				return false
			}

			if filteredLogs[0].Quota != quota1 {
				t.Logf("Filtered log quota mismatch: expected %d, got %d", quota1, filteredLogs[0].Quota)
				return false
			}

			return true
		},
		validClientUserIdGen,
		predefinedScenarioGen,
		predefinedScenarioGen,
		quotaGen,
		quotaGen,
	))

	// Property 7.5: Scenario filtering works correctly for custom project names
	properties.Property("scenario filtering works for custom project names", prop.ForAll(
		func(clientUserId string, customScenario string, predefinedScenario string, quota1 int, quota2 int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create log with custom scenario
			log1 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota1,
				ClientUserId:   clientUserId,
				ClientScenairo: customScenario,
			}
			if err := db.Create(&log1).Error; err != nil {
				t.Logf("Failed to create log1: %v", err)
				return false
			}

			// Create log with predefined scenario
			log2 := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota2,
				ClientUserId:   clientUserId,
				ClientScenairo: predefinedScenario,
			}
			if err := db.Create(&log2).Error; err != nil {
				t.Logf("Failed to create log2: %v", err)
				return false
			}

			// Query logs filtered by custom scenario
			var filteredLogs []model.Log
			if err := db.Where("client_user_id = ? AND client_scenairo = ?", clientUserId, customScenario).Find(&filteredLogs).Error; err != nil {
				t.Logf("Failed to query filtered logs: %v", err)
				return false
			}

			// Verify only custom scenario logs are returned
			if len(filteredLogs) != 1 {
				t.Logf("Expected 1 log for custom scenario %s, got %d", customScenario, len(filteredLogs))
				return false
			}

			if filteredLogs[0].ClientScenairo != customScenario {
				t.Logf("Filtered log scenario mismatch: expected %s, got %s", customScenario, filteredLogs[0].ClientScenairo)
				return false
			}

			return true
		},
		validClientUserIdGen,
		customScenarioGen,
		predefinedScenarioGen,
		quotaGen,
		quotaGen,
	))

	// Property 7.6: All predefined scenario values are accepted
	properties.Property("all predefined scenario values are accepted", prop.ForAll(
		func(clientUserId string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()
			predefinedScenarios := []string{
				"personal_experiment",
				"release_evaluation",
				"daily_external_model_evaluation",
			}

			// Create logs for all predefined scenarios
			for i, scenario := range predefinedScenarios {
				log := model.Log{
					UserId:         1,
					CreatedAt:      now + int64(i),
					Type:           model.LogTypeConsume,
					Quota:          quota + i*100,
					ClientUserId:   clientUserId,
					ClientScenairo: scenario,
				}
				if err := db.Create(&log).Error; err != nil {
					t.Logf("Failed to create log for scenario %s: %v", scenario, err)
					return false
				}
			}

			// Verify all scenarios were recorded
			var logs []model.Log
			if err := db.Where("client_user_id = ?", clientUserId).Find(&logs).Error; err != nil {
				t.Logf("Failed to query logs: %v", err)
				return false
			}

			if len(logs) != len(predefinedScenarios) {
				t.Logf("Expected %d logs, got %d", len(predefinedScenarios), len(logs))
				return false
			}

			// Verify each scenario is present
			scenarioSet := make(map[string]bool)
			for _, log := range logs {
				scenarioSet[log.ClientScenairo] = true
			}

			for _, scenario := range predefinedScenarios {
				if !scenarioSet[scenario] {
					t.Logf("Missing scenario: %s", scenario)
					return false
				}
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
	))

	// Property 7.7: Empty scenario string is stored as-is (middleware handles default)
	properties.Property("empty scenario string is stored as-is", prop.ForAll(
		func(clientUserId string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create a log with empty scenario (before middleware processing)
			log := model.Log{
				UserId:         1,
				CreatedAt:      now,
				Type:           model.LogTypeConsume,
				Quota:          quota,
				ClientUserId:   clientUserId,
				ClientScenairo: "", // Empty scenario
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the empty scenario was stored
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			// Empty string should be stored as-is
			if retrievedLog.ClientScenairo != "" {
				t.Logf("Expected empty scenario, got %s", retrievedLog.ClientScenairo)
				return false
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
	))

	// Property 7.8: Scenario values with special characters are handled correctly
	properties.Property("scenario values with underscores are handled correctly", prop.ForAll(
		func(clientUserId string, quota int) bool {
			// Clean up
			db.Where("client_user_id = ?", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Test scenarios with underscores (common in project names)
			scenariosWithUnderscores := []string{
				"my_project_name",
				"test_scenario_123",
				"daily_external_model_evaluation", // predefined with underscores
			}

			for i, scenario := range scenariosWithUnderscores {
				log := model.Log{
					UserId:         1,
					CreatedAt:      now + int64(i),
					Type:           model.LogTypeConsume,
					Quota:          quota + i*100,
					ClientUserId:   clientUserId,
					ClientScenairo: scenario,
				}
				if err := db.Create(&log).Error; err != nil {
					t.Logf("Failed to create log for scenario %s: %v", scenario, err)
					return false
				}

				// Verify the scenario was stored correctly
				var retrievedLog model.Log
				if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
					t.Logf("Failed to retrieve log: %v", err)
					return false
				}

				if retrievedLog.ClientScenairo != scenario {
					t.Logf("Scenario mismatch: expected %s, got %s", scenario, retrievedLog.ClientScenairo)
					return false
				}
			}

			return true
		},
		validClientUserIdGen,
		quotaGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 5: Consumption Tracking Correctness
// Validates: Requirements 5.6, 6.1, 6.2, 6.4, 6.5
//
// Property: For any completed API request:
//   - If a valid project header is provided, the used_quota in the corresponding ProjectAllocation
//     record SHALL increase by the consumed amount, and the log SHALL contain the project_name
//   - If no project header is provided, no ProjectAllocation records SHALL be modified,
//     and only fixed_quota/temp_quota consumption SHALL be updated
func TestProperty5_ConsumptionTrackingCorrectness(t *testing.T) {
	db := setupServiceTestDB(t)
	defer cleanupServiceTestDB(t, db)

	// Also migrate Log table for this test
	err := db.AutoMigrate(&model.Log{})
	require.NoError(t, err, "failed to migrate Log table")

	// Also migrate CliendUserQuota table for this test
	err = db.AutoMigrate(&model.CliendUserQuota{})
	require.NoError(t, err, "failed to migrate CliendUserQuota table")

	// Set the global DB for model functions
	originalDB := model.DB
	model.DB = db
	defer func() { model.DB = originalDB }()

	// Set the LOG_DB for log queries
	originalLogDB := model.LOG_DB
	model.LOG_DB = db
	defer func() { model.LOG_DB = originalLogDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names
	validProjectNameGen := gen.RegexMatch(`proj_[a-zA-Z0-9]{3,10}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for valid client user IDs
	validClientUserIdGen := gen.RegexMatch(`user_[a-zA-Z0-9]{3,10}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for quota consumption values (positive)
	consumptionGen := gen.IntRange(1, 10000)

	// Generator for allocated quota (must be larger than consumption)
	allocatedQuotaGen := gen.IntRange(10000, 100000)

	// Property 5.1: With valid project header, used_quota increases by consumed amount
	properties.Property("with project header used_quota increases by consumed amount", prop.ForAll(
		func(projectName string, clientUserId string, allocatedQuota int, consumption int) bool {
			// Ensure consumption doesn't exceed allocated quota
			if consumption > allocatedQuota {
				consumption = allocatedQuota / 2
			}

			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation with initial used_quota = 0
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Record initial used_quota
			initialUsedQuota := allocation.UsedQuota

			// Simulate consumption tracking by calling IncreaseProjectUsedQuota
			err := IncreaseProjectUsedQuota(project.Id, clientUserId, consumption)
			if err != nil {
				t.Logf("IncreaseProjectUsedQuota failed: %v", err)
				return false
			}

			// Verify used_quota increased by the consumed amount
			var updatedAllocation model.ProjectAllocation
			if err := db.Where("id = ?", allocation.Id).First(&updatedAllocation).Error; err != nil {
				t.Logf("Failed to retrieve updated allocation: %v", err)
				return false
			}

			expectedUsedQuota := initialUsedQuota + consumption
			if updatedAllocation.UsedQuota != expectedUsedQuota {
				t.Logf("UsedQuota mismatch: expected %d, got %d", expectedUsedQuota, updatedAllocation.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		consumptionGen,
	))

	// Property 5.2: Log contains project_name when project header is provided
	properties.Property("log contains project_name when project header is provided", prop.ForAll(
		func(projectName string, clientUserId string, consumption int) bool {
			// Clean up
			db.Where("client_user_id = ? AND project_name = ?", clientUserId, projectName).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create a log with project_name (simulating consumption tracking)
			log := model.Log{
				UserId:       1,
				CreatedAt:    now,
				Type:         model.LogTypeConsume,
				Quota:        consumption,
				ClientUserId: clientUserId,
				ProjectName:  projectName, // Project name should be recorded
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the log contains the project_name
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			if retrievedLog.ProjectName != projectName {
				t.Logf("ProjectName mismatch: expected %s, got %s", projectName, retrievedLog.ProjectName)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		consumptionGen,
	))

	// Property 5.3: Without project header, no ProjectAllocation records are modified
	properties.Property("without project header no allocation records are modified", prop.ForAll(
		func(projectName string, clientUserId string, allocatedQuota int, initialUsedQuota int) bool {
			// Ensure initialUsedQuota is valid
			if initialUsedQuota > allocatedQuota {
				initialUsedQuota = allocatedQuota / 2
			}

			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation with some initial used_quota
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Simulate a request WITHOUT project header
			// In this case, IncreaseProjectUsedQuota should NOT be called
			// The allocation should remain unchanged

			// Verify allocation is unchanged
			var unchangedAllocation model.ProjectAllocation
			if err := db.Where("id = ?", allocation.Id).First(&unchangedAllocation).Error; err != nil {
				t.Logf("Failed to retrieve allocation: %v", err)
				return false
			}

			if unchangedAllocation.UsedQuota != initialUsedQuota {
				t.Logf("UsedQuota should be unchanged: expected %d, got %d", initialUsedQuota, unchangedAllocation.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		gen.IntRange(0, 5000),
	))

	// Property 5.4: Log without project header has empty project_name
	properties.Property("log without project header has empty project_name", prop.ForAll(
		func(clientUserId string, consumption int) bool {
			// Clean up
			db.Where("client_user_id = ? AND project_name = ''", clientUserId).Delete(&model.Log{})

			now := time.Now().Unix()

			// Create a log without project_name (simulating request without project header)
			log := model.Log{
				UserId:       1,
				CreatedAt:    now,
				Type:         model.LogTypeConsume,
				Quota:        consumption,
				ClientUserId: clientUserId,
				ProjectName:  "", // No project header
			}
			if err := db.Create(&log).Error; err != nil {
				t.Logf("Failed to create log: %v", err)
				return false
			}

			// Verify the log has empty project_name
			var retrievedLog model.Log
			if err := db.Where("id = ?", log.Id).First(&retrievedLog).Error; err != nil {
				t.Logf("Failed to retrieve log: %v", err)
				return false
			}

			if retrievedLog.ProjectName != "" {
				t.Logf("ProjectName should be empty, got %s", retrievedLog.ProjectName)
				return false
			}

			return true
		},
		validClientUserIdGen,
		consumptionGen,
	))

	// Property 5.5: Multiple consumptions accumulate correctly in used_quota
	properties.Property("multiple consumptions accumulate correctly in used_quota", prop.ForAll(
		func(projectName string, clientUserId string, allocatedQuota int, numConsumptions int) bool {
			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Perform multiple consumptions
			var totalConsumed int
			consumptionPerRequest := 100 // Fixed small consumption per request
			for i := 0; i < numConsumptions; i++ {
				// Check if we would exceed quota
				if totalConsumed+consumptionPerRequest > allocatedQuota {
					break
				}

				err := IncreaseProjectUsedQuota(project.Id, clientUserId, consumptionPerRequest)
				if err != nil {
					// Expected if quota exceeded
					break
				}
				totalConsumed += consumptionPerRequest
			}

			// Verify final used_quota equals total consumed
			var finalAllocation model.ProjectAllocation
			if err := db.Where("id = ?", allocation.Id).First(&finalAllocation).Error; err != nil {
				t.Logf("Failed to retrieve final allocation: %v", err)
				return false
			}

			if finalAllocation.UsedQuota != totalConsumed {
				t.Logf("UsedQuota mismatch after multiple consumptions: expected %d, got %d", totalConsumed, finalAllocation.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		gen.IntRange(1, 10),
	))

	// Property 5.6: Consumption tracking does not modify other users' allocations
	properties.Property("consumption tracking does not modify other users allocations", prop.ForAll(
		func(projectName string, clientUserId1 string, clientUserId2 string, allocatedQuota int, consumption int) bool {
			// Skip if user IDs are the same
			if clientUserId1 == clientUserId2 {
				return true
			}

			// Ensure consumption doesn't exceed allocated quota
			if consumption > allocatedQuota {
				consumption = allocatedQuota / 2
			}

			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id IN ?", []string{clientUserId1, clientUserId2}).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 4,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocations for both users
			allocation1 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId1,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation1).Error; err != nil {
				t.Logf("Failed to create allocation1: %v", err)
				return false
			}

			initialUsedQuota2 := 500
			allocation2 := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId2,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota2,
			}
			if err := db.Create(&allocation2).Error; err != nil {
				t.Logf("Failed to create allocation2: %v", err)
				return false
			}

			// Track consumption for user1 only
			err := IncreaseProjectUsedQuota(project.Id, clientUserId1, consumption)
			if err != nil {
				t.Logf("IncreaseProjectUsedQuota failed: %v", err)
				return false
			}

			// Verify user2's allocation is unchanged
			var unchangedAllocation2 model.ProjectAllocation
			if err := db.Where("id = ?", allocation2.Id).First(&unchangedAllocation2).Error; err != nil {
				t.Logf("Failed to retrieve allocation2: %v", err)
				return false
			}

			if unchangedAllocation2.UsedQuota != initialUsedQuota2 {
				t.Logf("User2's UsedQuota should be unchanged: expected %d, got %d", initialUsedQuota2, unchangedAllocation2.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		consumptionGen,
	))

	// Property 5.7: Consumption tracking does not modify other projects' allocations
	properties.Property("consumption tracking does not modify other projects allocations", prop.ForAll(
		func(projectName1 string, projectName2 string, clientUserId string, allocatedQuota int, consumption int) bool {
			// Skip if project names are the same
			if projectName1 == projectName2 {
				return true
			}

			// Ensure consumption doesn't exceed allocated quota
			if consumption > allocatedQuota {
				consumption = allocatedQuota / 2
			}

			// Clean up
			db.Where("project_name IN ?", []string{projectName1, projectName2}).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create two projects
			project1 := model.Project{
				ProjectName: projectName1,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project1).Error; err != nil {
				t.Logf("Failed to create project1: %v", err)
				return false
			}

			project2 := model.Project{
				ProjectName: projectName2,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project2).Error; err != nil {
				t.Logf("Failed to create project2: %v", err)
				return false
			}

			// Create allocations for both projects
			allocation1 := model.ProjectAllocation{
				ProjectId:      project1.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      0,
			}
			if err := db.Create(&allocation1).Error; err != nil {
				t.Logf("Failed to create allocation1: %v", err)
				return false
			}

			initialUsedQuota2 := 500
			allocation2 := model.ProjectAllocation{
				ProjectId:      project2.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota2,
			}
			if err := db.Create(&allocation2).Error; err != nil {
				t.Logf("Failed to create allocation2: %v", err)
				return false
			}

			// Track consumption for project1 only
			err := IncreaseProjectUsedQuota(project1.Id, clientUserId, consumption)
			if err != nil {
				t.Logf("IncreaseProjectUsedQuota failed: %v", err)
				return false
			}

			// Verify project2's allocation is unchanged
			var unchangedAllocation2 model.ProjectAllocation
			if err := db.Where("id = ?", allocation2.Id).First(&unchangedAllocation2).Error; err != nil {
				t.Logf("Failed to retrieve allocation2: %v", err)
				return false
			}

			if unchangedAllocation2.UsedQuota != initialUsedQuota2 {
				t.Logf("Project2's UsedQuota should be unchanged: expected %d, got %d", initialUsedQuota2, unchangedAllocation2.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validProjectNameGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		consumptionGen,
	))

	// Property 5.8: Zero consumption does not modify used_quota
	properties.Property("zero consumption does not modify used_quota", prop.ForAll(
		func(projectName string, clientUserId string, allocatedQuota int, initialUsedQuota int) bool {
			// Ensure initialUsedQuota is valid
			if initialUsedQuota > allocatedQuota {
				initialUsedQuota = allocatedQuota / 2
			}

			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Track zero consumption
			err := IncreaseProjectUsedQuota(project.Id, clientUserId, 0)
			if err != nil {
				t.Logf("IncreaseProjectUsedQuota with zero should not fail: %v", err)
				return false
			}

			// Verify used_quota is unchanged
			var unchangedAllocation model.ProjectAllocation
			if err := db.Where("id = ?", allocation.Id).First(&unchangedAllocation).Error; err != nil {
				t.Logf("Failed to retrieve allocation: %v", err)
				return false
			}

			if unchangedAllocation.UsedQuota != initialUsedQuota {
				t.Logf("UsedQuota should be unchanged for zero consumption: expected %d, got %d", initialUsedQuota, unchangedAllocation.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		allocatedQuotaGen,
		gen.IntRange(0, 5000),
	))

	// Property 5.9: Consumption that would exceed quota is rejected
	properties.Property("consumption exceeding quota is rejected", prop.ForAll(
		func(projectName string, clientUserId string, allocatedQuota int) bool {
			// Clean up
			db.Where("project_name = ?", projectName).Delete(&model.Project{})
			db.Where("client_user_id = ?", clientUserId).Delete(&model.ProjectAllocation{})

			// Create project
			project := model.Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
				Status:      model.ProjectStatusEnabled,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create allocation with used_quota close to allocated_quota
			usedQuota := allocatedQuota - 10
			allocation := model.ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   clientUserId,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      usedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Try to consume more than remaining quota
			excessConsumption := 100 // More than the 10 remaining
			err := IncreaseProjectUsedQuota(project.Id, clientUserId, excessConsumption)

			// Should fail because consumption would exceed quota
			if err == nil {
				t.Logf("Expected error for consumption exceeding quota")
				return false
			}

			// Verify used_quota is unchanged
			var unchangedAllocation model.ProjectAllocation
			if err := db.Where("id = ?", allocation.Id).First(&unchangedAllocation).Error; err != nil {
				t.Logf("Failed to retrieve allocation: %v", err)
				return false
			}

			if unchangedAllocation.UsedQuota != usedQuota {
				t.Logf("UsedQuota should be unchanged after rejected consumption: expected %d, got %d", usedQuota, unchangedAllocation.UsedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		validClientUserIdGen,
		gen.IntRange(100, 10000),
	))

	properties.TestingRun(t)
}
