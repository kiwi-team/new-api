package model

import (
	"errors"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 消耗统计页(/quota-statistics)与飞书告警都按 quota_data 的 SUM(quota) 聚合。
// 异步任务提交时预扣、完成后补扣或退款，三者必须净额落在同一行上，且请求数只
// 记提交那一次——否则总消耗只增不减，请求数也会被结算过程放大。
func TestQuotaDataSettlementAdjustmentsNetOut(t *testing.T) {
	const (
		userId    = 4101
		username  = "settle-user"
		modelName = "test-video-model"
		uid       = "uid-settle"
	)

	cases := []struct {
		name         string
		preConsumed  int
		adjustType   int
		adjustQuota  int
		wantNetQuota int
	}{
		{
			name:         "task fails and is fully refunded",
			preConsumed:  3000,
			adjustType:   LogTypeRefund,
			adjustQuota:  3000,
			wantNetQuota: 0,
		},
		{
			name:         "settlement charges the difference",
			preConsumed:  3000,
			adjustType:   LogTypeConsume,
			adjustQuota:  1200,
			wantNetQuota: 4200,
		},
		{
			name:         "settlement refunds the difference",
			preConsumed:  3000,
			adjustType:   LogTypeRefund,
			adjustQuota:  800,
			wantNetQuota: 2200,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetQuotaDataState(t)

			createdAt := common.GetTimestamp()
			// 提交：一次真实请求，计一次数。
			LogQuotaData(&LogQuotaDataCache{
				UserId:       userId,
				Username:     username,
				ModelName:    modelName,
				Quota:        tc.preConsumed,
				CreatedAt:    createdAt,
				ClientUserId: uid,
			})
			// 结算：只修正金额，不是新请求。
			LogQuotaData(&LogQuotaDataCache{
				UserId:                 userId,
				Username:               username,
				ModelName:              modelName,
				Quota:                  signedQuotaForLogType(tc.adjustType, tc.adjustQuota),
				CreatedAt:              createdAt,
				ClientUserId:           uid,
				IsSettlementAdjustment: true,
			})
			SaveQuotaDataCache()

			var row QuotaData
			require.NoError(t, DB.Table("quota_data").Where("user_id = ?", userId).First(&row).Error)
			assert.Equal(t, tc.wantNetQuota, row.Quota, "统计页的总消耗应为净额")
			assert.Equal(t, 1, row.Count, "请求数只应记提交那一次")
		})
	}
}

// 结算调整落在提交记录之后到达（跨小时、或缓存已刷盘）时，走的是
// 的 UPDATE 分支还是 Create 分支取决于时序，两条路径都不能把请求数记成 1。
func TestQuotaDataSettlementAdjustmentAloneAddsNoRequest(t *testing.T) {
	const userId = 4102

	resetQuotaDataState(t)

	LogQuotaData(&LogQuotaDataCache{
		UserId:                 userId,
		Username:               "adjust-only",
		ModelName:              "test-video-model",
		Quota:                  -500,
		CreatedAt:              common.GetTimestamp(),
		ClientUserId:           "uid-adjust",
		IsSettlementAdjustment: true,
	})
	SaveQuotaDataCache()

	var row QuotaData
	require.NoError(t, DB.Table("quota_data").Where("user_id = ?", userId).First(&row).Error)
	assert.Equal(t, -500, row.Quota)
	assert.Equal(t, 0, row.Count)
}

func TestGetQuotaDataStatisticsFiltersByChannel(t *testing.T) {
	resetQuotaDataState(t)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:       1,
		ModelName:    "channel-filter-model",
		ClientUserId: "channel-filter-user",
		ChannelId:    11,
		CreatedAt:    1000,
		Count:        1,
		Quota:        100,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:       1,
		ModelName:    "channel-filter-model",
		ClientUserId: "channel-filter-user",
		ChannelId:    22,
		CreatedAt:    1000,
		Count:        2,
		Quota:        200,
	}).Error)

	rows, err := GetQuotaDataStatistics(900, 1100, "", "", "", false, false, false, 0, "", nil, 22, 0, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].TotalCount)
	assert.Equal(t, float64(200)/common.QuotaPerUnit, rows[0].TotalQuota)
}

func resetQuotaDataState(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM quota_data").Error)
	require.NoError(t, DB.Exec("DELETE FROM cliend_user_quota").Error)
	CacheQuotaDataLock.Lock()
	clear(CacheQuotaData)
	CacheQuotaDataLock.Unlock()
}

func TestQuotaDataPersistenceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
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
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			// These models use production table names; require an empty test database.
			require.False(t, db.Migrator().HasTable(&QuotaData{}))
			require.False(t, db.Migrator().HasTable(&CliendUserQuota{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&QuotaData{}, &CliendUserQuota{})) })
			require.NoError(t, db.AutoMigrate(&QuotaData{}, &CliendUserQuota{}))
			previousDB, previousType, previousCache := DB, common.MainDatabaseType(), CacheQuotaData
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			CacheQuotaData = make(map[quotaDataKey]*QuotaData)
			t.Cleanup(func() {
				DB, CacheQuotaData = previousDB, previousCache
				common.SetMainDatabaseType(previousType)
			})
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			t.Run("counters and refunds", testQuotaDataCounters)
			t.Run("dimensions and cache keys", testQuotaDataDimensions)
			t.Run("failed batches", testQuotaDataRetry)
			t.Run("partial success", testQuotaDataPartialFlush)
			t.Run("overlapping flushes", func(t *testing.T) {
				resetQuotaDataState(t)
				event := quotaDataTestEvent()
				entered, release := make(chan struct{}), make(chan struct{})
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("quota_test_overlap", func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_data" {
						close(entered)
						<-release
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Create().Remove("quota_test_overlap") })
				LogQuotaData(&event)
				results := make(chan error, 2)
				go func() { results <- SaveQuotaDataCache() }()
				select {
				case <-entered:
				case err := <-results:
					t.Fatalf("flush ended before creating the row: %v", err)
				}
				event.Quota = 50
				LogQuotaData(&event)
				go func() { results <- SaveQuotaDataCache() }()
				close(release)
				firstErr, secondErr := <-results, <-results
				require.NoError(t, firstErr)
				require.NoError(t, secondErr)
				var rows []QuotaData
				require.NoError(t, db.Find(&rows).Error)
				require.Len(t, rows, 1)
				assert.Equal(t, 150, rows[0].Quota)
				assert.Equal(t, 2, rows[0].Count)
				assert.Equal(t, 20, rows[0].PromptTokens)
			})
			t.Run("existing duplicate rows", func(t *testing.T) {
				resetQuotaDataState(t)
				event := quotaDataTestEvent()
				LogQuotaData(&event)
				require.NoError(t, SaveQuotaDataCache())
				var duplicate QuotaData
				require.NoError(t, db.First(&duplicate).Error)
				duplicate.Id, duplicate.Quota = 0, 200
				require.NoError(t, db.Create(&duplicate).Error)
				event.Quota = 50
				LogQuotaData(&event)
				require.NoError(t, SaveQuotaDataCache())
				var total int
				require.NoError(t, db.Model(&QuotaData{}).Select("SUM(quota)").Scan(&total).Error)
				assert.Equal(t, 350, total, "the new increment must be applied only once")
			})
		})
	}
}

func quotaDataTestEvent() LogQuotaDataCache {
	return LogQuotaDataCache{
		UserId: 1, Username: "quota-test", ModelName: "test-model", CreatedAt: 3661,
		TokenId: 11, TokenName: "test-token", ChannelId: 22, UseGroup: "default", NodeName: "node-a",
		ClientUserId: "uid-a", ClientScenairo: "scenario-a", ProjectName: "project-a", PlanId: 1,
		Quota: 100, TokenUsed: 15, PromptTokens: 10, CompletionTokens: 5, CachedTokens: 4,
		ClaudeCacheCreation5mTokens: 2, ClaudeCacheCreation1hTokens: 3, FirstTokenMs: 100, UseTimeSeconds: 2,
	}
}

func TestRecordConsumeLogReadyForQuotaFlush(t *testing.T) {
	truncateTables(t)
	resetQuotaDataState(t)
	previousExport, previousConsume := common.DataExportEnabled, common.LogConsumeEnabled
	common.DataExportEnabled, common.LogConsumeEnabled = true, true
	t.Cleanup(func() {
		common.DataExportEnabled, common.LogConsumeEnabled = previousExport, previousConsume
	})
	const userID = 11002233
	require.NoError(t, DB.Create(&User{Id: userID, Username: "quota-flush-user", Setting: "{}"}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("username", "quota-flush-user")
	RecordConsumeLog(c, userID, RecordConsumeLogParams{
		ModelName: "quota-flush-model", Quota: 100, PromptTokens: 10, CompletionTokens: 5,
		Group: "default", Other: NewLogOther(),
	})
	// Shutdown can flush immediately after a request finishes without waiting on a pool.
	require.NoError(t, SaveQuotaDataCache())
	var rows []QuotaData
	require.NoError(t, DB.Where("user_id = ?", userID).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 100, rows[0].Quota)
	assert.Equal(t, 1, rows[0].Count)
	assert.Equal(t, 15, rows[0].TokenUsed)
	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ?", userID).First(&log).Error)
	assert.Equal(t, log.CreatedAt-log.CreatedAt%3600, rows[0].CreatedAt)
}

func testQuotaDataCounters(t *testing.T) {
	resetQuotaDataState(t)
	event := quotaDataTestEvent()
	for range 2 {
		LogQuotaData(&event)
		require.NoError(t, SaveQuotaDataCache())
	}
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	want := QuotaData{
		Id: row.Id, UserID: 1, Username: "quota-test", ModelName: "test-model", CreatedAt: 3600,
		TokenId: 11, TokenName: "test-token", ChannelId: 22, UseGroup: "default", NodeName: "node-a",
		ClientUserId: "uid-a", ClientScenairo: "scenario-a", ProjectName: "project-a", PlanId: 1,
		Quota: 200, Count: 2, TokenUsed: 30, PromptTokens: 20, CompletionTokens: 10, CachedTokens: 8,
		ClaudeCacheCreation5mTokens: 4, ClaudeCacheCreation1hTokens: 6,
		CacheWrite5mRequestCount: 2, CacheWrite1hRequestCount: 2, CacheReadRequestCount: 2,
		StreamRequestCount: 2, FrtSum: 200, RequestTimeSum: 4000,
	}
	assert.Equal(t, want, row)
	event.Quota, event.IsSettlementAdjustment = -20, true
	event.TokenUsed, event.PromptTokens, event.CompletionTokens, event.CachedTokens = 0, 0, 0, 0
	event.ClaudeCacheCreation5mTokens, event.ClaudeCacheCreation1hTokens = 0, 0
	event.FirstTokenMs, event.UseTimeSeconds = 0, 0
	LogQuotaData(&event)
	require.NoError(t, SaveQuotaDataCache())
	require.NoError(t, DB.First(&row).Error)
	want.Quota = 180
	assert.Equal(t, want, row, "refunds change only quota")
	var uid CliendUserQuota
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 180, uid.UsedQuota)
}

func testQuotaDataDimensions(t *testing.T) {
	for _, dimension := range []string{"uid", "scenario", "project", "plan", "hyphens"} {
		t.Run(dimension, func(t *testing.T) {
			resetQuotaDataState(t)
			a, b := quotaDataTestEvent(), quotaDataTestEvent()
			a.TokenName, b.TokenName, b.Quota = "a", "b", 200
			switch dimension {
			case "uid":
				b.ClientUserId = "uid-b"
			case "scenario":
				b.ClientScenairo = "scenario-b"
			case "project":
				b.ProjectName = "project-b"
			case "plan":
				b.PlanId = 2
			case "hyphens":
				a.ClientUserId, a.ClientScenairo = "a-b", "c"
				b.ClientUserId, b.ClientScenairo = "a", "b-c"
			}
			LogQuotaData(&a)
			LogQuotaData(&b)
			require.NoError(t, SaveQuotaDataCache())
			a.Quota = 50
			LogQuotaData(&a)
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Order("token_name").Find(&rows).Error)
			require.Len(t, rows, 2)
			assert.Equal(t, 150, rows[0].Quota)
			assert.Equal(t, 200, rows[1].Quota)
			assert.Equal(t, 2, rows[0].Count)
			assert.Equal(t, 1, rows[1].Count)
			assert.Equal(t, a.ClientUserId, rows[0].ClientUserId)
			assert.Equal(t, b.ClientUserId, rows[1].ClientUserId)
		})
	}
}

func testQuotaDataRetry(t *testing.T) {
	for _, tc := range []struct {
		operation string
		table     string
	}{
		{"query", "quota_data"}, {"create", "quota_data"}, {"update", "quota_data"},
		{"query", "cliend_user_quota"}, {"create", "cliend_user_quota"}, {"update", "cliend_user_quota"},
	} {
		t.Run(tc.operation+" "+tc.table, func(t *testing.T) {
			resetQuotaDataState(t)
			event := quotaDataTestEvent()
			initialQuota, initialCount := 0, 0
			if tc.operation == "update" {
				LogQuotaData(&event)
				require.NoError(t, SaveQuotaDataCache())
				initialQuota, initialCount = 100, 1
			}
			injected := errors.New("quota data test write failure")
			failed := false
			callback := func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					failed = true
					// A request arriving during a failed flush must survive with the retry.
					extra := event
					extra.Quota = 50
					LogQuotaData(&extra)
					tx.AddError(injected)
				}
			}
			processor := DB.Callback().Query()
			switch tc.operation {
			case "create":
				processor = DB.Callback().Create()
			case "update":
				processor = DB.Callback().Update()
			}
			require.NoError(t, processor.Before("gorm:"+tc.operation).Register("quota_test_failure", callback))
			t.Cleanup(func() { _ = processor.Remove("quota_test_failure") })
			LogQuotaData(&event)
			assert.ErrorIs(t, SaveQuotaDataCache(), injected)
			require.NoError(t, processor.Remove("quota_test_failure"))
			require.True(t, failed)
			for _, table := range []string{"quota_data", "cliend_user_quota"} {
				column := "quota"
				if table == "cliend_user_quota" {
					column = "used_quota"
				}
				var total int
				require.NoError(t, DB.Table(table).Select("COALESCE(SUM("+column+"), 0)").Scan(&total).Error)
				assert.Equal(t, initialQuota, total, "the transaction must roll back both tables")
			}
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, initialQuota+150, rows[0].Quota)
			assert.Equal(t, initialCount+2, rows[0].Count)
			assert.Equal(t, (initialCount+2)*10, rows[0].PromptTokens)
			var uid CliendUserQuota
			require.NoError(t, DB.First(&uid).Error)
			assert.Equal(t, initialQuota+150, uid.UsedQuota)
		})
	}
}

func testQuotaDataPartialFlush(t *testing.T) {
	resetQuotaDataState(t)
	a, b := quotaDataTestEvent(), quotaDataTestEvent()
	a.ModelName, a.ClientUserId = "failed", "failed"
	b.ModelName, b.ClientUserId, b.Quota = "saved", "saved", 200
	injected := errors.New("quota data partial flush failure")
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("quota_test_partial", func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*QuotaData)
		if !ok {
			return
		}
		if row.ModelName == "failed" {
			tx.AddError(injected)
			return
		}
		// A successful flush must also retain arrivals for the next batch.
		extra := b
		extra.Quota = 50
		LogQuotaData(&extra)
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove("quota_test_partial") })
	LogQuotaData(&a)
	LogQuotaData(&b)
	assert.ErrorIs(t, SaveQuotaDataCache(), injected)
	require.NoError(t, DB.Callback().Create().Remove("quota_test_partial"))
	var rows []QuotaData
	require.NoError(t, DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 200, rows[0].Quota)
	assert.Equal(t, 150, GetCachedQuotaByUser(a.UserId, 3600, 7200))
	require.NoError(t, SaveQuotaDataCache())
	require.NoError(t, SaveQuotaDataCache())
	assert.Zero(t, GetCachedQuotaByUser(a.UserId, 3600, 7200))
	require.NoError(t, DB.Order("model_name").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, 100, rows[0].Quota)
	assert.Equal(t, 250, rows[1].Quota, "the committed batch must not be replayed")
	var uids []CliendUserQuota
	require.NoError(t, DB.Order("client_user_id").Find(&uids).Error)
	require.Len(t, uids, 2)
	assert.Equal(t, 100, uids[0].UsedQuota)
	assert.Equal(t, 250, uids[1].UsedQuota)
}
