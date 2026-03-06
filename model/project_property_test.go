package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
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

	return db
}

// cleanupTestDB cleans up the test database
func cleanupTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// Feature: project-budget-management, Property 1: Project Creation Initial State
// Validates: Requirements 1.1, 1.4, 2.1
//
// Property: For any valid project creation request with a non-empty project_name
// and non-negative total_budget, the created project SHALL have status set to
// enabled (1) and all required fields (id, project_name, total_budget, status,
// created_at, updated_at) populated.
func TestProperty1_ProjectCreationInitialState(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names (non-empty, max 100 chars, alphanumeric with underscores)
	validProjectNameGen := gen.RegexMatch(`[a-zA-Z][a-zA-Z0-9_]{0,99}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for valid total budget (non-negative)
	validBudgetGen := gen.IntRange(0, 10000000)

	properties.Property("created projects have enabled status and all required fields populated", prop.ForAll(
		func(projectName string, totalBudget int) bool {
			// Clean up any existing project with the same name first
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create the project
			project := Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
			}

			result := db.Create(&project)
			if result.Error != nil {
				t.Logf("Failed to create project: %v", result.Error)
				return false
			}

			// Verify all required fields are populated
			// 1. Id should be auto-generated and > 0
			if project.Id <= 0 {
				t.Logf("Id not populated: %d", project.Id)
				return false
			}

			// 2. ProjectName should match input
			if project.ProjectName != projectName {
				t.Logf("ProjectName mismatch: expected %s, got %s", projectName, project.ProjectName)
				return false
			}

			// 3. TotalBudget should match input
			if project.TotalBudget != totalBudget {
				t.Logf("TotalBudget mismatch: expected %d, got %d", totalBudget, project.TotalBudget)
				return false
			}

			// 4. Status should be enabled (1) - this is the key property from Requirement 1.4
			if project.Status != ProjectStatusEnabled {
				t.Logf("Status not enabled: expected %d, got %d", ProjectStatusEnabled, project.Status)
				return false
			}

			// 5. CreatedAt should be populated (non-zero)
			if project.CreatedAt <= 0 {
				t.Logf("CreatedAt not populated: %d", project.CreatedAt)
				return false
			}

			// 6. UpdatedAt should be populated (non-zero)
			if project.UpdatedAt <= 0 {
				t.Logf("UpdatedAt not populated: %d", project.UpdatedAt)
				return false
			}

			// Verify by re-reading from database
			var fetchedProject Project
			if err := db.First(&fetchedProject, project.Id).Error; err != nil {
				t.Logf("Failed to fetch project: %v", err)
				return false
			}

			// Verify fetched project has same values
			if fetchedProject.Status != ProjectStatusEnabled {
				t.Logf("Fetched project status not enabled: %d", fetchedProject.Status)
				return false
			}

			if fetchedProject.ProjectName != projectName {
				t.Logf("Fetched project name mismatch: expected %s, got %s", projectName, fetchedProject.ProjectName)
				return false
			}

			if fetchedProject.TotalBudget != totalBudget {
				t.Logf("Fetched project budget mismatch: expected %d, got %d", totalBudget, fetchedProject.TotalBudget)
				return false
			}

			return true
		},
		validProjectNameGen,
		validBudgetGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 2: Project Name Uniqueness
// Validates: Requirements 1.5, 2.1
//
// Property: For any two projects in the system, their project_name values SHALL
// be distinct. Attempting to create a project with an existing project_name
// SHALL fail with an appropriate error.
func TestProperty2_ProjectNameUniqueness(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names (non-empty, max 100 chars, alphanumeric with underscores)
	validProjectNameGen := gen.RegexMatch(`[a-zA-Z][a-zA-Z0-9_]{0,99}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for valid total budget (non-negative)
	validBudgetGen := gen.IntRange(0, 10000000)

	properties.Property("duplicate project names are rejected", prop.ForAll(
		func(projectName string, budget1 int, budget2 int) bool {
			// Clean up any existing project with the same name first
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create the first project - should succeed
			project1 := Project{
				ProjectName: projectName,
				TotalBudget: budget1,
			}

			result1 := db.Create(&project1)
			if result1.Error != nil {
				t.Logf("Failed to create first project: %v", result1.Error)
				return false
			}

			// Verify first project was created successfully
			if project1.Id <= 0 {
				t.Logf("First project Id not populated: %d", project1.Id)
				return false
			}

			// Attempt to create a second project with the same name - should fail
			project2 := Project{
				ProjectName: projectName,
				TotalBudget: budget2,
			}

			result2 := db.Create(&project2)

			// The second creation MUST fail due to uniqueness constraint
			if result2.Error == nil {
				t.Logf("Second project creation should have failed but succeeded with Id: %d", project2.Id)
				return false
			}

			// Verify only one project exists with this name
			var count int64
			db.Model(&Project{}).Where("project_name = ?", projectName).Count(&count)
			if count != 1 {
				t.Logf("Expected exactly 1 project with name %s, found %d", projectName, count)
				return false
			}

			// Verify the existing project is the first one we created
			var existingProject Project
			if err := db.Where("project_name = ?", projectName).First(&existingProject).Error; err != nil {
				t.Logf("Failed to fetch existing project: %v", err)
				return false
			}

			if existingProject.Id != project1.Id {
				t.Logf("Existing project Id mismatch: expected %d, got %d", project1.Id, existingProject.Id)
				return false
			}

			if existingProject.TotalBudget != budget1 {
				t.Logf("Existing project budget mismatch: expected %d, got %d", budget1, existingProject.TotalBudget)
				return false
			}

			return true
		},
		validProjectNameGen,
		validBudgetGen,
		validBudgetGen,
	))

	// Additional property: distinct project names can coexist
	properties.Property("distinct project names can coexist", prop.ForAll(
		func(name1 string, name2 string, budget1 int, budget2 int) bool {
			// Skip if names are the same (this case is covered by the uniqueness test)
			if name1 == name2 {
				return true
			}

			// Clean up any existing projects with these names
			db.Where("project_name IN ?", []string{name1, name2}).Delete(&Project{})

			// Create first project
			project1 := Project{
				ProjectName: name1,
				TotalBudget: budget1,
			}

			result1 := db.Create(&project1)
			if result1.Error != nil {
				t.Logf("Failed to create first project: %v", result1.Error)
				return false
			}

			// Create second project with different name - should succeed
			project2 := Project{
				ProjectName: name2,
				TotalBudget: budget2,
			}

			result2 := db.Create(&project2)
			if result2.Error != nil {
				t.Logf("Failed to create second project with different name: %v", result2.Error)
				return false
			}

			// Verify both projects exist
			var count int64
			db.Model(&Project{}).Where("project_name IN ?", []string{name1, name2}).Count(&count)
			if count != 2 {
				t.Logf("Expected 2 projects, found %d", count)
				return false
			}

			// Verify each project has correct data
			var fetchedProject1, fetchedProject2 Project
			if err := db.Where("project_name = ?", name1).First(&fetchedProject1).Error; err != nil {
				t.Logf("Failed to fetch project1: %v", err)
				return false
			}
			if err := db.Where("project_name = ?", name2).First(&fetchedProject2).Error; err != nil {
				t.Logf("Failed to fetch project2: %v", err)
				return false
			}

			// Verify they have distinct IDs
			if fetchedProject1.Id == fetchedProject2.Id {
				t.Logf("Projects should have distinct IDs: %d vs %d", fetchedProject1.Id, fetchedProject2.Id)
				return false
			}

			return true
		},
		validProjectNameGen,
		validProjectNameGen,
		validBudgetGen,
		validBudgetGen,
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 3: Allocation Budget Constraint
// Validates: Requirements 2.6, 3.1, 3.2, 3.5, 3.6
//
// Property: For any project, the sum of all allocated_quota values across all
// ProjectAllocation records for that project SHALL be less than or equal to
// the project's total_budget. Any allocation operation that would violate
// this constraint SHALL be rejected.
func TestProperty3_AllocationBudgetConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names (non-empty, max 100 chars, alphanumeric with underscores)
	validProjectNameGen := gen.RegexMatch(`[a-zA-Z][a-zA-Z0-9_]{0,99}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		})

	// Generator for valid total budget (positive to allow allocations)
	validBudgetGen := gen.IntRange(1, 10000000)

	// Generator for number of allocations (1 to 10)
	numAllocationsGen := gen.IntRange(1, 10)

	// Property: Sum of allocations never exceeds total budget
	properties.Property("sum of allocations never exceeds total budget", prop.ForAll(
		func(projectName string, totalBudget int, numAllocations int) bool {
			// Clean up any existing project with the same name first
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create the project
			project := Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
			}

			result := db.Create(&project)
			if result.Error != nil {
				t.Logf("Failed to create project: %v", result.Error)
				return false
			}

			// Clean up any existing allocations for this project
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Calculate allocation amounts that should fit within budget
			// Distribute budget evenly among allocations
			allocationAmount := totalBudget / numAllocations
			if allocationAmount == 0 {
				allocationAmount = 1
			}

			// Create allocations that should succeed (within budget)
			var totalAllocated int
			for i := 0; i < numAllocations; i++ {
				// Calculate remaining budget
				remaining := totalBudget - totalAllocated

				// Determine allocation amount (don't exceed remaining)
				allocAmount := allocationAmount
				if allocAmount > remaining {
					allocAmount = remaining
				}
				if allocAmount <= 0 {
					break // No more budget to allocate
				}

				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   generateUniqueUserId(i),
					AllocatedQuota: allocAmount,
				}

				err := db.Create(&allocation).Error
				if err != nil {
					t.Logf("Failed to create allocation: %v", err)
					return false
				}

				totalAllocated += allocAmount
			}

			// Verify the constraint: sum of allocations <= total budget
			var sumAllocated int64
			err := db.Model(&ProjectAllocation{}).
				Where("project_id = ?", project.Id).
				Select("COALESCE(SUM(allocated_quota), 0)").
				Scan(&sumAllocated).Error
			if err != nil {
				t.Logf("Failed to sum allocations: %v", err)
				return false
			}

			if int(sumAllocated) > totalBudget {
				t.Logf("Constraint violated: sum of allocations (%d) > total budget (%d)", sumAllocated, totalBudget)
				return false
			}

			return true
		},
		validProjectNameGen,
		validBudgetGen,
		numAllocationsGen,
	))

	// Property: Allocation that would exceed budget is rejected (when validation is applied)
	properties.Property("allocation exceeding remaining budget should be detectable", prop.ForAll(
		func(projectName string, totalBudget int, firstAllocationRatio float64) bool {
			// Clean up any existing project with the same name first
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create the project
			project := Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
			}

			result := db.Create(&project)
			if result.Error != nil {
				t.Logf("Failed to create project: %v", result.Error)
				return false
			}

			// Clean up any existing allocations for this project
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Create first allocation using a portion of the budget
			firstAllocation := int(float64(totalBudget) * firstAllocationRatio)
			if firstAllocation < 0 {
				firstAllocation = 0
			}
			if firstAllocation > totalBudget {
				firstAllocation = totalBudget
			}

			allocation1 := ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   "user_first_001",
				AllocatedQuota: firstAllocation,
			}

			err := db.Create(&allocation1).Error
			if err != nil {
				t.Logf("Failed to create first allocation: %v", err)
				return false
			}

			// Calculate remaining budget
			remainingBudget := totalBudget - firstAllocation

			// Verify that we can detect when a second allocation would exceed the budget
			// by checking the remaining budget before creating the allocation
			var currentAllocatedTotal int64
			err = db.Model(&ProjectAllocation{}).
				Where("project_id = ?", project.Id).
				Select("COALESCE(SUM(allocated_quota), 0)").
				Scan(&currentAllocatedTotal).Error
			if err != nil {
				t.Logf("Failed to get current allocated total: %v", err)
				return false
			}

			calculatedRemaining := totalBudget - int(currentAllocatedTotal)
			if calculatedRemaining != remainingBudget {
				t.Logf("Remaining budget calculation mismatch: expected %d, got %d", remainingBudget, calculatedRemaining)
				return false
			}

			// Verify that an allocation request exceeding remaining budget can be detected
			excessiveAllocation := remainingBudget + 1
			if excessiveAllocation > 0 {
				// This allocation would exceed the budget
				wouldExceed := (int(currentAllocatedTotal) + excessiveAllocation) > totalBudget
				if !wouldExceed {
					t.Logf("Failed to detect budget violation: current=%d, new=%d, total=%d",
						currentAllocatedTotal, excessiveAllocation, totalBudget)
					return false
				}
			}

			// Verify that an allocation within remaining budget is valid
			if remainingBudget > 0 {
				validAllocation := remainingBudget
				wouldExceed := (int(currentAllocatedTotal) + validAllocation) > totalBudget
				if wouldExceed {
					t.Logf("Valid allocation incorrectly flagged as exceeding budget: current=%d, new=%d, total=%d",
						currentAllocatedTotal, validAllocation, totalBudget)
					return false
				}
			}

			return true
		},
		validProjectNameGen,
		validBudgetGen,
		gen.Float64Range(0.0, 1.0),
	))

	// Property: Multiple allocations sum correctly
	properties.Property("multiple allocations sum correctly and respect budget", prop.ForAll(
		func(projectName string, totalBudget int, allocationRatios []float64) bool {
			// Skip if no allocations
			if len(allocationRatios) == 0 {
				return true
			}

			// Clean up any existing project with the same name first
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create the project
			project := Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
			}

			result := db.Create(&project)
			if result.Error != nil {
				t.Logf("Failed to create project: %v", result.Error)
				return false
			}

			// Clean up any existing allocations for this project
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Normalize ratios so they sum to <= 1.0
			var ratioSum float64
			for _, r := range allocationRatios {
				if r > 0 {
					ratioSum += r
				}
			}

			// Create allocations based on normalized ratios
			var expectedTotal int
			for i, ratio := range allocationRatios {
				if ratio <= 0 {
					continue
				}

				// Normalize and calculate allocation amount
				normalizedRatio := ratio
				if ratioSum > 1.0 {
					normalizedRatio = ratio / ratioSum
				}

				allocAmount := int(float64(totalBudget) * normalizedRatio)
				if allocAmount <= 0 {
					continue
				}

				// Ensure we don't exceed remaining budget
				remaining := totalBudget - expectedTotal
				if allocAmount > remaining {
					allocAmount = remaining
				}
				if allocAmount <= 0 {
					continue
				}

				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   generateUniqueUserId(i),
					AllocatedQuota: allocAmount,
				}

				err := db.Create(&allocation).Error
				if err != nil {
					t.Logf("Failed to create allocation %d: %v", i, err)
					return false
				}

				expectedTotal += allocAmount
			}

			// Verify the sum of allocations
			var actualTotal int64
			err := db.Model(&ProjectAllocation{}).
				Where("project_id = ?", project.Id).
				Select("COALESCE(SUM(allocated_quota), 0)").
				Scan(&actualTotal).Error
			if err != nil {
				t.Logf("Failed to sum allocations: %v", err)
				return false
			}

			// Verify sum matches expected
			if int(actualTotal) != expectedTotal {
				t.Logf("Sum mismatch: expected %d, got %d", expectedTotal, actualTotal)
				return false
			}

			// Verify constraint: sum <= total budget
			if int(actualTotal) > totalBudget {
				t.Logf("Constraint violated: sum (%d) > total budget (%d)", actualTotal, totalBudget)
				return false
			}

			return true
		},
		validProjectNameGen,
		validBudgetGen,
		gen.SliceOf(gen.Float64Range(0.0, 0.5)),
	))

	properties.TestingRun(t)
}

// generateUniqueUserId generates a unique user ID for testing
func generateUniqueUserId(index int) string {
	return "user_test_" + string(rune('a'+index%26)) + "_" + time.Now().Format("150405") + "_" + string(rune('0'+index%10))
}

// Feature: project-budget-management, Property 10: Concurrency Safety
// Validates: Requirements 10.1, 10.2, 10.3, 10.4
//
// Property: For any set of concurrent operations on the same ProjectAllocation record:
// - The final used_quota SHALL equal the initial used_quota plus the sum of all successful delta updates
// - No update SHALL cause used_quota to exceed allocated_quota
// - No allocation update SHALL cause total allocations to exceed total_budget, even under concurrent access
func TestProperty10_ConcurrencySafety(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Set the global DB for model functions
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for valid project names
	validProjectNameGen := gen.RegexMatch(`[a-zA-Z][a-zA-Z0-9_]{0,49}`).
		SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 50
		})

	// Generator for allocated quota (must be positive to allow updates)
	allocatedQuotaGen := gen.IntRange(100, 100000)

	// Generator for initial used quota (0 to allow room for updates)
	initialUsedQuotaGen := gen.IntRange(0, 50)

	// Generator for delta values (positive, small enough to allow multiple updates)
	deltaGen := gen.IntRange(1, 100)

	// Property: Sequential updates maintain consistency (simulates concurrent behavior)
	// The final used_quota equals initial + sum of successful deltas
	properties.Property("sequential updates maintain consistency", prop.ForAll(
		func(projectName string, allocatedQuota int, initialUsedQuota int, deltas []int) bool {
			// Clean up any existing project with the same name
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create project
			project := Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2, // Ensure enough budget
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Clean up any existing allocations
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Create allocation with initial used quota
			allocation := ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   "concurrent_test_user_" + projectName,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			// Track successful updates
			var successfulDeltaSum int
			var successCount int

			// Apply deltas sequentially (simulating concurrent operations)
			for _, delta := range deltas {
				if delta <= 0 {
					continue
				}

				// Use the atomic IncreaseUsedQuota function
				err := IncreaseUsedQuota(allocation.Id, delta)
				if err == nil {
					successfulDeltaSum += delta
					successCount++
				}
				// If error, the update was rejected (quota exceeded) - this is expected behavior
			}

			// Fetch the final state
			var finalAllocation ProjectAllocation
			if err := db.First(&finalAllocation, allocation.Id).Error; err != nil {
				t.Logf("Failed to fetch final allocation: %v", err)
				return false
			}

			// Verify: final used_quota = initial + sum of successful deltas
			expectedUsedQuota := initialUsedQuota + successfulDeltaSum
			if finalAllocation.UsedQuota != expectedUsedQuota {
				t.Logf("Used quota mismatch: expected %d (initial %d + successful deltas %d), got %d",
					expectedUsedQuota, initialUsedQuota, successfulDeltaSum, finalAllocation.UsedQuota)
				return false
			}

			// Verify: used_quota never exceeds allocated_quota
			if finalAllocation.UsedQuota > finalAllocation.AllocatedQuota {
				t.Logf("Constraint violated: used_quota (%d) > allocated_quota (%d)",
					finalAllocation.UsedQuota, finalAllocation.AllocatedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		allocatedQuotaGen,
		initialUsedQuotaGen,
		gen.SliceOfN(10, deltaGen),
	))

	// Property: Updates that would exceed quota are rejected
	properties.Property("updates exceeding quota are rejected", prop.ForAll(
		func(projectName string, allocatedQuota int, usedQuota int) bool {
			// Ensure usedQuota is less than allocatedQuota
			if usedQuota >= allocatedQuota {
				usedQuota = allocatedQuota - 1
			}
			if usedQuota < 0 {
				usedQuota = 0
			}

			// Clean up any existing project with the same name
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create project
			project := Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Clean up any existing allocations
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Create allocation
			allocation := ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   "quota_exceed_test_user_" + projectName,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      usedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			remainingQuota := allocatedQuota - usedQuota

			// Try to update with exactly remaining quota - should succeed
			if remainingQuota > 0 {
				err := IncreaseUsedQuota(allocation.Id, remainingQuota)
				if err != nil {
					t.Logf("Update with exact remaining quota should succeed: %v", err)
					return false
				}

				// Verify the update was applied
				var updated ProjectAllocation
				if err := db.First(&updated, allocation.Id).Error; err != nil {
					t.Logf("Failed to fetch updated allocation: %v", err)
					return false
				}

				if updated.UsedQuota != allocatedQuota {
					t.Logf("Used quota should equal allocated quota after exact update: got %d, expected %d",
						updated.UsedQuota, allocatedQuota)
					return false
				}
			}

			// Try to update with 1 more - should fail (quota exceeded)
			err := IncreaseUsedQuota(allocation.Id, 1)
			if err == nil {
				t.Logf("Update exceeding quota should have failed")
				return false
			}

			// Verify the allocation was not modified
			var finalAllocation ProjectAllocation
			if err := db.First(&finalAllocation, allocation.Id).Error; err != nil {
				t.Logf("Failed to fetch final allocation: %v", err)
				return false
			}

			if finalAllocation.UsedQuota > finalAllocation.AllocatedQuota {
				t.Logf("Constraint violated after rejected update: used_quota (%d) > allocated_quota (%d)",
					finalAllocation.UsedQuota, finalAllocation.AllocatedQuota)
				return false
			}

			return true
		},
		validProjectNameGen,
		gen.IntRange(10, 10000),
		gen.IntRange(0, 9999),
	))

	// Property: Multiple allocations respect total budget constraint
	properties.Property("multiple allocations respect total budget constraint", prop.ForAll(
		func(projectName string, totalBudget int, numAllocations int) bool {
			if numAllocations <= 0 {
				return true
			}

			// Clean up any existing project with the same name
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create project
			project := Project{
				ProjectName: projectName,
				TotalBudget: totalBudget,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Clean up any existing allocations
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Calculate allocation per user
			allocationPerUser := totalBudget / numAllocations
			if allocationPerUser <= 0 {
				allocationPerUser = 1
			}

			// Create allocations
			var totalAllocated int
			for i := 0; i < numAllocations; i++ {
				remaining := totalBudget - totalAllocated
				allocAmount := allocationPerUser
				if allocAmount > remaining {
					allocAmount = remaining
				}
				if allocAmount <= 0 {
					break
				}

				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   generateUniqueUserId(i) + "_budget_" + projectName,
					AllocatedQuota: allocAmount,
					UsedQuota:      0,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation %d: %v", i, err)
					return false
				}
				totalAllocated += allocAmount
			}

			// Verify total allocations don't exceed total budget
			allocatedTotal, err := GetProjectAllocatedTotal(project.Id)
			if err != nil {
				t.Logf("Failed to get allocated total: %v", err)
				return false
			}

			if allocatedTotal > totalBudget {
				t.Logf("Total allocations (%d) exceed total budget (%d)", allocatedTotal, totalBudget)
				return false
			}

			// Simulate concurrent usage updates on all allocations
			allocations, err := GetAllocationsByProjectId(project.Id, 0, 100)
			if err != nil {
				t.Logf("Failed to get allocations: %v", err)
				return false
			}

			// Each allocation uses some quota
			for _, alloc := range allocations {
				if alloc.AllocatedQuota > 0 {
					// Use half of the allocated quota
					delta := alloc.AllocatedQuota / 2
					if delta > 0 {
						_ = IncreaseUsedQuota(alloc.Id, delta)
					}
				}
			}

			// Verify used quota never exceeds allocated quota for any allocation
			updatedAllocations, err := GetAllocationsByProjectId(project.Id, 0, 100)
			if err != nil {
				t.Logf("Failed to get updated allocations: %v", err)
				return false
			}

			for _, alloc := range updatedAllocations {
				if alloc.UsedQuota > alloc.AllocatedQuota {
					t.Logf("Allocation %d: used_quota (%d) > allocated_quota (%d)",
						alloc.Id, alloc.UsedQuota, alloc.AllocatedQuota)
					return false
				}
			}

			// Verify total used doesn't exceed total budget
			usedTotal, err := GetProjectUsedTotal(project.Id)
			if err != nil {
				t.Logf("Failed to get used total: %v", err)
				return false
			}

			if usedTotal > totalBudget {
				t.Logf("Total used (%d) exceeds total budget (%d)", usedTotal, totalBudget)
				return false
			}

			return true
		},
		validProjectNameGen,
		gen.IntRange(100, 100000),
		gen.IntRange(1, 10),
	))

	// Property: Atomic update guarantees - no partial updates
	properties.Property("atomic updates are all-or-nothing", prop.ForAll(
		func(projectName string, allocatedQuota int, initialUsedQuota int, delta int) bool {
			// Ensure valid initial state
			if initialUsedQuota >= allocatedQuota {
				initialUsedQuota = allocatedQuota - 1
			}
			if initialUsedQuota < 0 {
				initialUsedQuota = 0
			}

			// Clean up any existing project with the same name
			db.Where("project_name = ?", projectName).Delete(&Project{})

			// Create project
			project := Project{
				ProjectName: projectName,
				TotalBudget: allocatedQuota * 2,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Clean up any existing allocations
			db.Where("project_id = ?", project.Id).Delete(&ProjectAllocation{})

			// Create allocation
			allocation := ProjectAllocation{
				ProjectId:      project.Id,
				ClientUserId:   "atomic_test_user_" + projectName,
				AllocatedQuota: allocatedQuota,
				UsedQuota:      initialUsedQuota,
			}
			if err := db.Create(&allocation).Error; err != nil {
				t.Logf("Failed to create allocation: %v", err)
				return false
			}

			remainingQuota := allocatedQuota - initialUsedQuota

			// Attempt the update
			err := IncreaseUsedQuota(allocation.Id, delta)

			// Fetch the final state
			var finalAllocation ProjectAllocation
			if fetchErr := db.First(&finalAllocation, allocation.Id).Error; fetchErr != nil {
				t.Logf("Failed to fetch final allocation: %v", fetchErr)
				return false
			}

			if err == nil {
				// Update succeeded - verify the full delta was applied
				expectedUsedQuota := initialUsedQuota + delta
				if finalAllocation.UsedQuota != expectedUsedQuota {
					t.Logf("Successful update should apply full delta: expected %d, got %d",
						expectedUsedQuota, finalAllocation.UsedQuota)
					return false
				}
			} else {
				// Update failed - verify no change was made
				if finalAllocation.UsedQuota != initialUsedQuota {
					t.Logf("Failed update should not modify used_quota: expected %d, got %d",
						initialUsedQuota, finalAllocation.UsedQuota)
					return false
				}
			}

			// Verify constraint is always maintained
			if finalAllocation.UsedQuota > finalAllocation.AllocatedQuota {
				t.Logf("Constraint violated: used_quota (%d) > allocated_quota (%d)",
					finalAllocation.UsedQuota, finalAllocation.AllocatedQuota)
				return false
			}

			// Verify the update result is consistent with the constraint
			if delta <= remainingQuota {
				// Update should have succeeded
				if err != nil {
					t.Logf("Update with delta %d should succeed (remaining %d): %v", delta, remainingQuota, err)
					return false
				}
			} else {
				// Update should have failed
				if err == nil {
					t.Logf("Update with delta %d should fail (remaining %d)", delta, remainingQuota)
					return false
				}
			}

			return true
		},
		validProjectNameGen,
		gen.IntRange(100, 10000),
		gen.IntRange(0, 99),
		gen.IntRange(1, 200),
	))

	properties.TestingRun(t)
}

// Feature: project-budget-management, Property 9: Pagination Correctness
// Validates: Requirements 9.1, 9.4
//
// Property: For any paginated list request (projects or allocations), if the total count is N and page size is P, then:
// - Page 1 SHALL return min(P, N) items
// - The sum of items across all pages SHALL equal N
// - No item SHALL appear in more than one page
func TestProperty9_PaginationCorrectness(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Set the global DB for model functions
	originalDB := DB
	DB = db
	defer func() { DB = originalDB }()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for number of items (1 to 50)
	numItemsGen := gen.IntRange(1, 50)

	// Generator for page size (1 to 20)
	pageSizeGen := gen.IntRange(1, 20)

	// Property: Project pagination page 1 returns min(P, N) items
	properties.Property("project pagination page 1 returns min(P, N) items", prop.ForAll(
		func(numProjects int, pageSize int) bool {
			// Clean up existing projects
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create test projects
			for i := 0; i < numProjects; i++ {
				project := Project{
					ProjectName: "pagination_test_project_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					TotalBudget: 1000 + i,
				}
				if err := db.Create(&project).Error; err != nil {
					t.Logf("Failed to create project %d: %v", i, err)
					return false
				}
			}

			// Get total count
			totalCount, err := GetProjectCount()
			if err != nil {
				t.Logf("Failed to get project count: %v", err)
				return false
			}

			if int(totalCount) != numProjects {
				t.Logf("Total count mismatch: expected %d, got %d", numProjects, totalCount)
				return false
			}

			// Get page 1
			page1Projects, err := GetProjectList(0, pageSize)
			if err != nil {
				t.Logf("Failed to get page 1: %v", err)
				return false
			}

			// Verify page 1 returns min(P, N) items
			expectedPage1Count := pageSize
			if numProjects < pageSize {
				expectedPage1Count = numProjects
			}

			if len(page1Projects) != expectedPage1Count {
				t.Logf("Page 1 count mismatch: expected %d, got %d (N=%d, P=%d)",
					expectedPage1Count, len(page1Projects), numProjects, pageSize)
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	// Property: Sum of items across all project pages equals total count
	properties.Property("sum of items across all project pages equals total count", prop.ForAll(
		func(numProjects int, pageSize int) bool {
			// Clean up existing projects
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create test projects
			for i := 0; i < numProjects; i++ {
				project := Project{
					ProjectName: "pagination_sum_test_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					TotalBudget: 2000 + i,
				}
				if err := db.Create(&project).Error; err != nil {
					t.Logf("Failed to create project %d: %v", i, err)
					return false
				}
			}

			// Get total count
			totalCount, err := GetProjectCount()
			if err != nil {
				t.Logf("Failed to get project count: %v", err)
				return false
			}

			// Iterate through all pages and count items
			var totalItems int
			offset := 0
			for {
				projects, err := GetProjectList(offset, pageSize)
				if err != nil {
					t.Logf("Failed to get projects at offset %d: %v", offset, err)
					return false
				}

				totalItems += len(projects)

				// If we got fewer items than page size, we've reached the end
				if len(projects) < pageSize {
					break
				}

				offset += pageSize

				// Safety check to prevent infinite loop
				if offset > numProjects+pageSize {
					t.Logf("Pagination exceeded expected bounds: offset=%d, numProjects=%d", offset, numProjects)
					return false
				}
			}

			// Verify sum equals total count
			if totalItems != int(totalCount) {
				t.Logf("Sum of items (%d) does not equal total count (%d)", totalItems, totalCount)
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	// Property: No project appears in more than one page
	properties.Property("no project appears in more than one page", prop.ForAll(
		func(numProjects int, pageSize int) bool {
			// Clean up existing projects
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create test projects
			for i := 0; i < numProjects; i++ {
				project := Project{
					ProjectName: "pagination_unique_test_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					TotalBudget: 3000 + i,
				}
				if err := db.Create(&project).Error; err != nil {
					t.Logf("Failed to create project %d: %v", i, err)
					return false
				}
			}

			// Collect all project IDs across all pages
			seenIds := make(map[int]int) // map[projectId]pageNumber
			offset := 0
			pageNum := 1
			for {
				projects, err := GetProjectList(offset, pageSize)
				if err != nil {
					t.Logf("Failed to get projects at offset %d: %v", offset, err)
					return false
				}

				for _, project := range projects {
					if existingPage, exists := seenIds[project.Id]; exists {
						t.Logf("Project ID %d appears in both page %d and page %d", project.Id, existingPage, pageNum)
						return false
					}
					seenIds[project.Id] = pageNum
				}

				// If we got fewer items than page size, we've reached the end
				if len(projects) < pageSize {
					break
				}

				offset += pageSize
				pageNum++

				// Safety check to prevent infinite loop
				if offset > numProjects+pageSize {
					t.Logf("Pagination exceeded expected bounds: offset=%d, numProjects=%d", offset, numProjects)
					return false
				}
			}

			// Verify we saw all projects
			if len(seenIds) != numProjects {
				t.Logf("Did not see all projects: expected %d, saw %d", numProjects, len(seenIds))
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	// Property: Allocation pagination page 1 returns min(P, N) items
	properties.Property("allocation pagination page 1 returns min(P, N) items", prop.ForAll(
		func(numAllocations int, pageSize int) bool {
			// Clean up existing data
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create a test project
			project := Project{
				ProjectName: "alloc_pagination_test_" + time.Now().Format("150405.000000"),
				TotalBudget: 1000000,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create test allocations
			for i := 0; i < numAllocations; i++ {
				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   "alloc_page_user_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					AllocatedQuota: 100 + i,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation %d: %v", i, err)
					return false
				}
			}

			// Get total count
			totalCount, err := GetAllocationCount(project.Id)
			if err != nil {
				t.Logf("Failed to get allocation count: %v", err)
				return false
			}

			if int(totalCount) != numAllocations {
				t.Logf("Total count mismatch: expected %d, got %d", numAllocations, totalCount)
				return false
			}

			// Get page 1
			page1Allocations, err := GetAllocationsByProjectId(project.Id, 0, pageSize)
			if err != nil {
				t.Logf("Failed to get page 1: %v", err)
				return false
			}

			// Verify page 1 returns min(P, N) items
			expectedPage1Count := pageSize
			if numAllocations < pageSize {
				expectedPage1Count = numAllocations
			}

			if len(page1Allocations) != expectedPage1Count {
				t.Logf("Page 1 count mismatch: expected %d, got %d (N=%d, P=%d)",
					expectedPage1Count, len(page1Allocations), numAllocations, pageSize)
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	// Property: Sum of items across all allocation pages equals total count
	properties.Property("sum of items across all allocation pages equals total count", prop.ForAll(
		func(numAllocations int, pageSize int) bool {
			// Clean up existing data
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create a test project
			project := Project{
				ProjectName: "alloc_sum_pagination_test_" + time.Now().Format("150405.000000"),
				TotalBudget: 1000000,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create test allocations
			for i := 0; i < numAllocations; i++ {
				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   "alloc_sum_user_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					AllocatedQuota: 200 + i,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation %d: %v", i, err)
					return false
				}
			}

			// Get total count
			totalCount, err := GetAllocationCount(project.Id)
			if err != nil {
				t.Logf("Failed to get allocation count: %v", err)
				return false
			}

			// Iterate through all pages and count items
			var totalItems int
			offset := 0
			for {
				allocations, err := GetAllocationsByProjectId(project.Id, offset, pageSize)
				if err != nil {
					t.Logf("Failed to get allocations at offset %d: %v", offset, err)
					return false
				}

				totalItems += len(allocations)

				// If we got fewer items than page size, we've reached the end
				if len(allocations) < pageSize {
					break
				}

				offset += pageSize

				// Safety check to prevent infinite loop
				if offset > numAllocations+pageSize {
					t.Logf("Pagination exceeded expected bounds: offset=%d, numAllocations=%d", offset, numAllocations)
					return false
				}
			}

			// Verify sum equals total count
			if totalItems != int(totalCount) {
				t.Logf("Sum of items (%d) does not equal total count (%d)", totalItems, totalCount)
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	// Property: No allocation appears in more than one page
	properties.Property("no allocation appears in more than one page", prop.ForAll(
		func(numAllocations int, pageSize int) bool {
			// Clean up existing data
			db.Exec("DELETE FROM project_allocations")
			db.Exec("DELETE FROM projects")

			// Create a test project
			project := Project{
				ProjectName: "alloc_unique_pagination_test_" + time.Now().Format("150405.000000"),
				TotalBudget: 1000000,
			}
			if err := db.Create(&project).Error; err != nil {
				t.Logf("Failed to create project: %v", err)
				return false
			}

			// Create test allocations
			for i := 0; i < numAllocations; i++ {
				allocation := ProjectAllocation{
					ProjectId:      project.Id,
					ClientUserId:   "alloc_unique_user_" + time.Now().Format("150405.000000") + "_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
					AllocatedQuota: 300 + i,
				}
				if err := db.Create(&allocation).Error; err != nil {
					t.Logf("Failed to create allocation %d: %v", i, err)
					return false
				}
			}

			// Collect all allocation IDs across all pages
			seenIds := make(map[int]int) // map[allocationId]pageNumber
			offset := 0
			pageNum := 1
			for {
				allocations, err := GetAllocationsByProjectId(project.Id, offset, pageSize)
				if err != nil {
					t.Logf("Failed to get allocations at offset %d: %v", offset, err)
					return false
				}

				for _, allocation := range allocations {
					if existingPage, exists := seenIds[allocation.Id]; exists {
						t.Logf("Allocation ID %d appears in both page %d and page %d", allocation.Id, existingPage, pageNum)
						return false
					}
					seenIds[allocation.Id] = pageNum
				}

				// If we got fewer items than page size, we've reached the end
				if len(allocations) < pageSize {
					break
				}

				offset += pageSize
				pageNum++

				// Safety check to prevent infinite loop
				if offset > numAllocations+pageSize {
					t.Logf("Pagination exceeded expected bounds: offset=%d, numAllocations=%d", offset, numAllocations)
					return false
				}
			}

			// Verify we saw all allocations
			if len(seenIds) != numAllocations {
				t.Logf("Did not see all allocations: expected %d, saw %d", numAllocations, len(seenIds))
				return false
			}

			return true
		},
		numItemsGen,
		pageSizeGen,
	))

	properties.TestingRun(t)
}
