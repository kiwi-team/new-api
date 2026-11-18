package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func insertUsersForPaginationTest(t *testing.T, total int) {
	t.Helper()
	for id := 1; id <= total; id++ {
		user := &User{
			Id:          id,
			Username:    fmt.Sprintf("user%02d", id),
			Password:    "password123",
			DisplayName: fmt.Sprintf("User %02d", id),
			Email:       fmt.Sprintf("user%02d@example.com", id),
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AffCode:     fmt.Sprintf("aff%02d", id),
		}
		require.NoError(t, DB.Create(user).Error)
	}
}

func collectUserIDs(users []*User) []int {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	return ids
}

func TestGetAllUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	pageOne, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, collectUserIDs(pageOne))

	pageTwo, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(pageTwo))

	pageThree, total, err := GetAllUsers(&common.PageInfo{Page: 3, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{41, 42}, collectUserIDs(pageThree))
}

func TestSearchUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	users, total, err := SearchUsers("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}

func TestSearchUsersDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupListSearchTestDB(t, dialect)
			users := make([]User, 205)
			for i := range users {
				name := fmt.Sprintf("member-%c%c", 'a'+rune(i/26), 'a'+rune(i%26))
				users[i] = User{Id: i + 1, Username: name, DisplayName: "Ordinary member", Email: name + "@example.com", AffCode: name, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
			}
			users[204].DisplayName = "Remote Alice"
			users[19].Group = "premium"
			users[203].Username = "tenant.river"
			users[202].Email = "qa.needle@example.net"
			users[201].DisplayName = "生产测试账号"
			require.NoError(t, db.Create(&users).Error)
			for _, tc := range []struct {
				name, keyword, group string
				offset, limit        int
				total                int64
				ids                  []int
			}{
				{"display name beyond first 200", "aLiCe", "", 0, 20, 1, []int{205}},
				{"username substring", "ant.RIV", "", 0, 20, 1, []int{204}},
				{"display name substring", "MoTe AL", "", 0, 20, 1, []int{205}},
				{"email substring", "NEEDle@", "", 0, 20, 1, []int{203}},
				{"Chinese display name substring", "测试", "", 0, 20, 1, []int{202}},
				{"trimmed keyword", "  aLiCe  ", "", 0, 20, 1, []int{205}},
				{"exact ID", "205", "", 0, 20, 1, []int{205}},
				{"partial ID", "20", "", 0, 20, 8, []int{20, 120, 200, 201, 202, 203, 204, 205}},
				{"partial ID pagination", "20", "", 1, 2, 8, []int{120, 200}},
				{"partial ID respects group", "20", "premium", 0, 20, 1, []int{20}},
				{"pages beyond 200", "member", "", 200, 20, 205, []int{201, 202, 203, 204, 205}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					items, total, err := SearchUsers(tc.keyword, tc.group, nil, nil, tc.offset, tc.limit, NewUserSortOptions("id", "asc"))
					require.NoError(t, err)
					assert.Equal(t, tc.total, total)
					assert.Equal(t, tc.ids, collectUserIDs(items))
				})
			}
		})
	}
}

func setupListSearchTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN not configured")
		}
		driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "owner_search_"}})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseType(dialect))
	initCol()
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(&Token{}, &User{}))
		require.NoError(t, connection.Close())
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
	})
	require.False(t, db.Migrator().HasTable(&User{}), "use an isolated empty database")
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	var version string
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database version: %s", version)
	return db
}

func TestSearchUserTokensDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupListSearchTestDB(t, dialect)
			tokens := []Token{
				{Id: 1, UserId: 1, Name: "production-service", Key: "owner-alpha-key"},
				{Id: 2, UserId: 1, Name: "staging-production", Key: "owner-beta-key"},
				{Id: 3, UserId: 1, Name: "DEV_日报!", Key: "owner-special-key"},
				{Id: 4, UserId: 2, Name: "production-foreign", Key: "foreign-gamma-key"},
				{Id: 5, UserId: 1, Name: "development", Key: "dev-delta-key"},
			}
			require.NoError(t, db.Create(&tokens).Error)
			previousMax := operation_setting.GetTokenSetting().MaxUserTokens
			operation_setting.GetTokenSetting().MaxUserTokens = 1000
			t.Cleanup(func() { operation_setting.GetTokenSetting().MaxUserTokens = previousMax })
			for _, tc := range []struct {
				name, keyword, key    string
				userID, offset, limit int
				total                 int64
				ids                   []int
			}{
				{"name substring", "duct", "", 1, 0, 20, 2, []int{2, 1}},
				{"case insensitive", "PRODuct", "", 1, 0, 20, 2, []int{2, 1}},
				{"single Chinese character", "日", "", 1, 0, 20, 1, []int{3}},
				{"literal underscore", "_", "", 1, 0, 20, 1, []int{3}},
				{"literal escape character", "报!", "", 1, 0, 20, 1, []int{3}},
				{"trimmed name", " production ", "", 1, 0, 20, 2, []int{2, 1}},
				{"explicit wildcard compatibility", "%duct%", "", 1, 0, 20, 2, []int{2, 1}},
				{"empty name", "", "", 1, 0, 20, 4, []int{5, 3, 2, 1}},
				{"blank name", "   ", "", 1, 0, 20, 4, []int{5, 3, 2, 1}},
				{"root sees every owner", "production", "", 0, 0, 20, 3, []int{4, 2, 1}},
				{"pagination and total", "production", "", 1, 1, 1, 2, []int{1}},
				{"key stays exact", "", "alpha", 1, 0, 20, 0, []int{}},
				{"name plus full key", "prod", "sk-owner-beta-key", 1, 0, 20, 1, []int{2}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					items, total, err := SearchUserTokens(tc.userID, tc.keyword, tc.key, "", "", tc.offset, tc.limit)
					require.NoError(t, err)
					ids := make([]int, 0, len(items))
					for _, item := range items {
						ids = append(ids, item.Id)
					}
					assert.Equal(t, tc.total, total)
					assert.Equal(t, tc.ids, ids)
				})
			}
			t.Run("name substring above token creation limit", func(t *testing.T) {
				operation_setting.GetTokenSetting().MaxUserTokens = 1
				items, total, err := SearchUserTokens(1, "production", "", "", "", 0, 20)
				require.NoError(t, err)
				assert.EqualValues(t, 2, total)
				require.Len(t, items, 2)
				assert.Equal(t, 2, items[0].Id)
				assert.Equal(t, 1, items[1].Id)
			})
		})
	}
}
