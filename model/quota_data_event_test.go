package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestQuotaDataPersistenceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "quota.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
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
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(4)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			models := []any{&QuotaData{}, &QuotaDataEventRecord{}, &QuotaDataAppliedEvent{}, &CliendUserQuota{}, &Token{}, &RateLimitBucket{}, &User{}, &SystemTaskLock{}, &UsageAdjustment{}}
			for _, model := range models {
				require.False(t, db.Migrator().HasTable(model), "use an empty disposable database")
			}
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
			oldDB, oldType, oldRedis := DB, common.MainDatabaseType(), common.RedisEnabled
			DB, common.RedisEnabled = db, false
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() { DB, common.RedisEnabled = oldDB, oldRedis; common.SetMainDatabaseType(oldType); initCol() })
			testQuotaDataSchemaMigration(t, models)
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			t.Run("all counters and refunds", testQuotaDataCounters)
			t.Run("all dimensions", testQuotaDataDimensions)
			t.Run("model name length", testQuotaDataModelNameLength)
			t.Run("event deduplication", testQuotaDataEventDeduplication)
			t.Run("atomic failure and recovery", testQuotaDataEventRollback)
			t.Run("concurrent consumers", testQuotaDataConcurrentConsumers)
			t.Run("settlement discount consistency", testSettlementDiscountConsistency)
			t.Run("durable spool recovery", testQuotaDataSpoolRecovery)
			t.Run("legacy migration", func(t *testing.T) { testQuotaDataEventMigration(t, models) })
			t.Run("existing keyed buckets migration", func(t *testing.T) { testQuotaDataKeyedMigration(t, models) })
			t.Run("interrupted bucket migration", func(t *testing.T) { testQuotaDataInterruptedMigration(t, models) })
			t.Run("released rc45 migration", func(t *testing.T) {
				resetQuotaDataState(t)
				require.NoError(t, DB.Migrator().DropTable(&QuotaData{}))
				require.NoError(t, DB.Table("quota_data").AutoMigrate(&releasedQuotaDataRC45{}))
				legacyModelName := strings.Repeat("m", 64)
				var before []releasedQuotaDataRC45
				for range 2 {
					row := releasedQuotaDataRC45{UserID: 1, Username: "legacy", ModelName: legacyModelName, CreatedAt: 3600, TokenUsed: 15, Count: 1, Quota: 100}
					require.NoError(t, DB.Table("quota_data").Create(&row).Error)
					before = append(before, row)
				}
				testQuotaDataSchemaMigration(t, models)
				var after []releasedQuotaDataRC45
				require.NoError(t, DB.Table("quota_data").Order("id").Find(&after).Error)
				assert.Equal(t, before, after, "released data and row IDs must remain unchanged")
				var legacyCount int64
				require.NoError(t, DB.Model(&QuotaData{}).Where("bucket_key IS NULL").Count(&legacyCount).Error)
				assert.EqualValues(t, 2, legacyCount)
			})
			t.Run("model name length after upgrade", testQuotaDataModelNameLength)
			t.Run("funds and resource leases", func(t *testing.T) {
				user := createReserveTestUser(t, 100)
				token := createReserveTestToken(t, 100)
				for _, expected := range []bool{true, false} {
					ok, err := TryReserveUserQuota(user.Id, 80)
					require.NoError(t, err)
					assert.Equal(t, expected, ok)
					ok, err = TryReserveTokenQuota(token.Id, token.Key, 80, false)
					require.NoError(t, err)
					assert.Equal(t, expected, ok)
				}
				require.NoError(t, IncreaseUserQuota(user.Id, 10, false))
				require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 10))
				assert.Equal(t, 30, getUserQuotaFromDB(t, user.Id))
				assert.Equal(t, 30, getTokenFromDB(t, token.Id).RemainQuota)
				token = getTokenFromDB(t, token.Id)
				token.Group = "restricted"
				require.NoError(t, token.Update())
				assert.EqualValues(t, 2, getTokenFromDB(t, token.Id).AuthVersion)
				useUserCacheMiniRedis(t)
				token = getTokenFromDB(t, token.Id)
				_, err := cacheInitToken(token)
				require.NoError(t, err)
				cachedToken, err := cacheGetTokenByKey(token.Key)
				require.NoError(t, err)
				assert.Equal(t, 30, cachedToken.RemainQuota)
				assert.Equal(t, "restricted", cachedToken.Group)
				ctx := t.Context()
				until := time.Now().Add(time.Minute).Unix()
				ok, err := AcquireResourceLease(ctx, "log_export:1", "node-a", until)
				require.NoError(t, err)
				require.True(t, ok)
				ok, err = AcquireResourceLease(ctx, "log_export:1", "node-b", until)
				require.NoError(t, err)
				assert.False(t, ok)
				require.NoError(t, ReleaseResourceLease(ctx, "log_export:1", "node-b"))
				require.NoError(t, RenewResourceLease(ctx, "log_export:1", "node-a", until+10))
				require.NoError(t, ReleaseResourceLease(ctx, "log_export:1", "node-a"))
				ok, err = AcquireResourceLease(ctx, "log_export:1", "node-b", until)
				require.NoError(t, err)
				assert.True(t, ok)
				assert.ErrorIs(t, RenewResourceLease(ctx, "log_export:1", "node-a", until+20), ErrSystemTaskLockLost)
			})
			t.Run("shared security limit", func(t *testing.T) {
				key := uuid.NewString()
				for _, expected := range []bool{true, true, false} {
					allowed, ttl, err := TakeSecurityRateLimit(context.Background(), key, 2, 30)
					require.NoError(t, err)
					assert.Equal(t, expected, allowed)
					assert.Positive(t, ttl)
				}
			})
		})
	}
}

// Every upgrade is repeated to ensure a restart only checks the quota schema.
// In particular, nullable bucket keys must not trigger a recurring table rebuild.
func testQuotaDataSchemaMigration(t *testing.T, models []any) {
	t.Helper()
	recorder := &migrationSQLRecorder{}
	db := DB.Session(&gorm.Session{Logger: recorder})
	for pass := range 2 {
		recorder.reset()
		require.NoError(t, migrateQuotaDataBuckets(db))
		require.NoError(t, db.AutoMigrate(models...))
		for _, statement := range recorder.statements {
			normalized := strings.ToUpper(strings.TrimSpace(statement))
			if strings.Contains(normalized, "QUOTA_DATA") {
				assert.False(t, strings.HasPrefix(normalized, "DELETE "), "startup must not delete historical statistics: %s", statement)
				if pass == 1 {
					assert.False(t, strings.HasPrefix(normalized, "UPDATE "), "second startup must not update historical rows: %s", statement)
				}
			}
		}
		if pass == 1 {
			for _, mutation := range recorder.schemaMutations() {
				assert.NotContains(t, strings.ToLower(mutation), "quota_data", "second startup must not repeat quota DDL")
			}
		}
	}
}

func testQuotaDataModelNameLength(t *testing.T) {
	for _, tc := range []struct {
		name      string
		modelName string
		valid     bool
	}{
		{name: "64 ASCII characters", modelName: strings.Repeat("m", 64), valid: true},
		{name: "65 ASCII characters", modelName: strings.Repeat("m", 65), valid: true},
		{name: "128 ASCII characters", modelName: strings.Repeat("m", 128), valid: true},
		{name: "128 Unicode characters", modelName: strings.Repeat("模型", 64), valid: true},
		{name: "129 ASCII characters", modelName: strings.Repeat("m", 129)},
		{name: "129 Unicode characters", modelName: strings.Repeat("模", 129)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetQuotaDataState(t)
			params := quotaDataTestEvent()
			params.ModelName = tc.modelName
			err := LogQuotaData(&params)
			if !tc.valid {
				require.ErrorIs(t, err, errInvalidQuotaDataEvent)
				var pending int64
				require.NoError(t, DB.Model(&QuotaDataEventRecord{}).Count(&pending).Error)
				assert.Zero(t, pending)
				return
			}
			require.NoError(t, err)
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, LogQuotaData(&params))
			require.NoError(t, SaveQuotaDataCache())
			params.Quota, params.IsSettlementAdjustment = -20, true
			require.NoError(t, LogQuotaData(&params))
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.modelName, rows[0].ModelName)
			assert.Equal(t, 2, rows[0].Count)
			assert.Equal(t, 180, rows[0].Quota)
			assert.Equal(t, 30, rows[0].TokenUsed)
			duplicate := rows[0]
			duplicate.Id = 0
			assert.Error(t, DB.Create(&duplicate).Error, "the bucket must remain unique after migration")
		})
	}
}

func testQuotaDataEventDeduplication(t *testing.T) {
	resetQuotaDataState(t)
	params := quotaDataTestEvent()
	params.EventID = uuid.NewString()
	event, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, deliverQuotaDataEvent(event, nil, ""))
	require.NoError(t, deliverQuotaDataEvent(event, nil, ""))
	require.NoError(t, applyQuotaDataEvents(ctx, []QuotaDataEvent{event, event}))
	require.NoError(t, SaveQuotaDataCache())
	require.NoError(t, applyQuotaDataEvents(ctx, []QuotaDataEvent{event}))
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 100, row.Quota)
	assert.Equal(t, 1, row.Count)
	var receipts int64
	require.NoError(t, DB.Model(&QuotaDataAppliedEvent{}).Count(&receipts).Error)
	assert.EqualValues(t, 1, receipts)
	params.Quota++
	conflict, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	assert.ErrorIs(t, deliverQuotaDataEvent(conflict, nil, ""), errInvalidQuotaDataEvent)
	assert.ErrorIs(t, applyQuotaDataEvents(ctx, []QuotaDataEvent{conflict}), errInvalidQuotaDataEvent)
	var uid CliendUserQuota
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 100, uid.UsedQuota)
	// A different action is not suppressed just because it has the same bucket.
	params.EventID = uuid.NewString()
	params.Quota = 0
	zero, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	require.NoError(t, applyQuotaDataEvents(ctx, []QuotaDataEvent{event, zero}))
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 2, row.Count)
	assert.Equal(t, 100, row.Quota)
}

func testQuotaDataEventRollback(t *testing.T) {
	for _, table := range []string{"quota_data_applied_events", "quota_data", "cliend_user_quota", "quota_data_events"} {
		t.Run(table, func(t *testing.T) {
			resetQuotaDataState(t)
			params := quotaDataTestEvent()
			require.NoError(t, LogQuotaData(&params))
			injected := errors.New("injected quota transaction failure")
			callback := func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(injected)
				}
			}
			processor := DB.Callback().Create()
			operation := "create"
			if table == "quota_data_events" {
				processor = DB.Callback().Update()
				operation = "update"
			}
			require.NoError(t, processor.Before("gorm:"+operation).Register("quota_event_failure", callback))
			assert.ErrorIs(t, SaveQuotaDataCache(), injected)
			require.NoError(t, processor.Remove("quota_event_failure"))
			for _, model := range []any{&QuotaData{}, &CliendUserQuota{}, &QuotaDataAppliedEvent{}} {
				var count int64
				require.NoError(t, DB.Model(model).Count(&count).Error)
				assert.Zero(t, count)
			}
			var row QuotaDataEventRecord
			require.NoError(t, DB.First(&row).Error)
			assert.Equal(t, "pending", row.State)
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, SaveQuotaDataCache())
			var q QuotaData
			require.NoError(t, DB.First(&q).Error)
			assert.Equal(t, 100, q.Quota)
			var uid CliendUserQuota
			require.NoError(t, DB.First(&uid).Error)
			assert.Equal(t, 100, uid.UsedQuota)
		})
	}
}

func testQuotaDataConcurrentConsumers(t *testing.T) {
	resetQuotaDataState(t)
	params := quotaDataTestEvent()
	first, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	second, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 3)
	var wg sync.WaitGroup
	for _, batch := range [][]QuotaDataEvent{{first}, {second, first}, {second}} {
		wg.Go(func() { <-start; results <- applyQuotaDataEvents(context.Background(), batch) })
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var rows []QuotaData
	require.NoError(t, DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 200, rows[0].Quota)
	assert.Equal(t, 2, rows[0].Count)
	var uid CliendUserQuota
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 200, uid.UsedQuota)
}

func testQuotaDataEventMigration(t *testing.T, models []any) {
	resetQuotaDataState(t)
	// Recreate the released layout (no bucket key or token auth version), then
	// insert representative duplicate rows without the new creation hook.
	require.NoError(t, DB.Migrator().DropIndex(&QuotaData{}, "idx_quota_data_bucket"))
	require.NoError(t, DB.Migrator().DropColumn(&QuotaData{}, "BucketKey"))
	require.NoError(t, DB.Migrator().DropColumn(&Token{}, "AuthVersion"))
	if common.MainDatabaseType() != common.DatabaseTypeSQLite {
		require.NoError(t, DB.Table("cliend_user_quota").Migrator().AlterColumn(&releasedUIDQuota{}, "UsedQuota"))
	}
	params := quotaDataTestEvent()
	var before []QuotaData
	for range 2 {
		row := quotaDataDelta(&params, 3600)
		require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}}}).Omit("bucket_key").Create(row).Error)
		before = append(before, *row)
	}
	require.NoError(t, DB.Create(&CliendUserQuota{ClientUserId: params.ClientUserId, UsedQuota: 200}).Error)
	require.NoError(t, DB.Table("tokens").Create(map[string]any{"user_id": 1, "key": "migration-token", "name": "migration", "status": 1}).Error)
	testQuotaDataSchemaMigration(t, models)
	var rows []QuotaData
	require.NoError(t, DB.Where("bucket_key IS NULL").Order("id").Find(&rows).Error)
	assert.Equal(t, before, rows, "migration must preserve every historical row, dimension and counter")
	var uid CliendUserQuota
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 200, uid.UsedQuota, "migration must not reapply UID amounts")
	// New requests and a later refund target the same historical hour/identity.
	// They must accumulate in one new bucket without updating either legacy row.
	for _, quota := range []int{100, 100, -20} {
		params.EventID, params.Quota = uuid.NewString(), quota
		params.IsSettlementAdjustment = quota < 0
		require.NoError(t, LogQuotaData(&params))
		require.NoError(t, SaveQuotaDataCache())
		// Retry each delivery after commit to protect deduplication at the boundary.
		require.NoError(t, LogQuotaData(&params))
		require.NoError(t, SaveQuotaDataCache())
	}
	rows = nil
	require.NoError(t, DB.Where("bucket_key IS NULL").Order("id").Find(&rows).Error)
	assert.Equal(t, before, rows, "new consumption and refunds must not mutate historical rows")
	rows = nil
	require.NoError(t, DB.Where("bucket_key IS NOT NULL").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.NotEmpty(t, rows[0].BucketKey)
	assert.Equal(t, 180, rows[0].Quota)
	assert.Equal(t, 2, rows[0].Count)
	duplicate := rows[0]
	duplicate.Id = 0
	assert.Error(t, DB.Create(&duplicate).Error, "non-NULL keys must remain unique")

	// Existing reports must include both generations, including all counters.
	analysis, err := GetModelUsageAnalysis(params.UserId, 0, 7200)
	require.NoError(t, err)
	require.Len(t, analysis, 1)
	assert.Equal(t, &ModelUsageAnalysisRow{
		Date: analysis[0].Date, UserId: params.UserId, Username: params.Username, TokenId: params.TokenId, TokenName: params.TokenName, ModelName: params.ModelName,
		Quota: 380, CostUsd: 380 / common.QuotaPerUnit, TotalRequests: 4,
		CacheWriteRequests: 8, CacheWrite5mRequests: 4, CacheWrite1hRequests: 4, CacheReadRequests: 4,
		CacheWriteTokens: 20, CacheReadTokens: 16, InputTokens: 40, OutputTokens: 20,
		StreamRequests: 4, FrtSumMs: 400, RequestTimeSumMs: 8000, AvgFirstTokenMs: 100, AvgUseTimeMs: 2000,
	}, analysis[0])
	chart, err := GetQuotaDataByUserId(params.UserId, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, chart, 1)
	assert.Equal(t, 380, chart[0].Quota)
	assert.Equal(t, 4, chart[0].Count)
	assert.Equal(t, 60, chart[0].TokenUsed)
	quota, err := GetQuotaByTime(params.UserId, 0, 7200)
	require.NoError(t, err)
	assert.Equal(t, 380, quota)
	personal, err := GetMonthlyNonProjectQuota(params.ClientUserId, 0)
	require.NoError(t, err)
	assert.Zero(t, personal, "project spending must not enter the personal budget")
	require.NoError(t, DB.First(&uid).Error)
	assert.Equal(t, 380, uid.UsedQuota, "only the net new quota may change UID accounting")
	var token Token
	require.NoError(t, DB.Where(commonKeyCol+" = ?", "migration-token").First(&token).Error)
	assert.EqualValues(t, 1, token.AuthVersion)
	require.NoError(t, IncreaseCliendUserUsedQuota(params.ClientUserId, 1<<32))
	require.NoError(t, DB.First(&uid).Error)
	assert.EqualValues(t, 380+(int64(1)<<32), uid.UsedQuota, "legacy INT must migrate without truncating large totals")
}

// The first Redis implementation required a key with an empty-string default. Preserve its already
// keyed rows and event receipts while relaxing the column for untouched history.
type requiredQuotaDataBucketKey struct {
	BucketKey string `gorm:"size:64;not null;default:''"`
}

func testQuotaDataKeyedMigration(t *testing.T, models []any) {
	resetQuotaDataState(t)
	params := quotaDataTestEvent()
	params.EventID = uuid.NewString()
	require.NoError(t, LogQuotaData(&params))
	require.NoError(t, SaveQuotaDataCache())
	var before QuotaData
	require.NoError(t, DB.First(&before).Error)
	require.NoError(t, DB.Table("quota_data").Migrator().AlterColumn(&requiredQuotaDataBucketKey{}, "BucketKey"))
	require.NoError(t, DB.Exec("CREATE INDEX idx_quota_data_legacy_fixture ON quota_data (username, created_at)").Error)
	t.Cleanup(func() { require.NoError(t, DB.Migrator().DropIndex(&QuotaData{}, "idx_quota_data_legacy_fixture")) })
	testQuotaDataSchemaMigration(t, models)
	assert.True(t, DB.Migrator().HasIndex(&QuotaData{}, "idx_quota_data_legacy_fixture"), "indexes outside the current model tags must survive")
	require.NoError(t, LogQuotaData(&params))
	require.NoError(t, SaveQuotaDataCache())
	var after QuotaData
	require.NoError(t, DB.First(&after).Error)
	assert.Equal(t, before, after, "schema upgrade must preserve applied events and their buckets")
	// Omitted keys must now default to NULL, including on already upgraded DBs.
	for range 2 {
		legacy := before
		legacy.Id = 0
		require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).Omit("bucket_key").Create(&legacy).Error)
	}
	var legacyCount int64
	require.NoError(t, DB.Model(&QuotaData{}).Where("bucket_key IS NULL").Count(&legacyCount).Error)
	assert.EqualValues(t, 2, legacyCount)
	duplicate := before
	duplicate.Id = 0
	assert.Error(t, DB.Create(&duplicate).Error, "existing bucket uniqueness must survive the upgrade")
}

func testQuotaDataInterruptedMigration(t *testing.T, models []any) {
	for _, tc := range []struct {
		name         string
		requiredKey  bool
		duplicateKey bool
	}{
		{name: "empty placeholders with old default", requiredKey: true},
		{name: "empty placeholders after dropping default"},
		{name: "nonempty duplicates remain an error", duplicateKey: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetQuotaDataState(t)
			params := quotaDataTestEvent()
			params.EventID = uuid.NewString()
			require.NoError(t, LogQuotaData(&params))
			require.NoError(t, SaveQuotaDataCache())
			var keyed QuotaData
			require.NoError(t, DB.First(&keyed).Error)
			require.NoError(t, DB.Migrator().DropIndex(&QuotaData{}, "idx_quota_data_bucket"))
			if tc.requiredKey {
				require.NoError(t, DB.Table("quota_data").Migrator().AlterColumn(&requiredQuotaDataBucketKey{}, "BucketKey"))
			}
			// An interrupted old migration can leave duplicate empty strings
			// even after a subsequent startup removed NOT NULL and DEFAULT.
			for range 2 {
				legacy := keyed
				legacy.Id = 0
				if !tc.duplicateKey {
					legacy.BucketKey = ""
				}
				require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).Create(&legacy).Error)
			}
			var before []QuotaData
			require.NoError(t, DB.Order("id").Find(&before).Error)
			require.Error(t, DB.Migrator().CreateIndex(&QuotaData{}, "idx_quota_data_bucket"), "reproduce the startup uniqueness failure")
			if tc.duplicateKey {
				require.NoError(t, migrateQuotaDataBuckets(DB))
				assert.Error(t, DB.AutoMigrate(models...), "never hide duplicate valid keys by discarding their identity")
			} else {
				testQuotaDataSchemaMigration(t, models)
				var emptyCount, nullCount int64
				require.NoError(t, DB.Model(&QuotaData{}).Where("LENGTH(bucket_key) = 0").Count(&emptyCount).Error)
				require.NoError(t, DB.Model(&QuotaData{}).Where("bucket_key IS NULL").Count(&nullCount).Error)
				assert.Zero(t, emptyCount)
				assert.EqualValues(t, 2, nullCount)
				require.NoError(t, LogQuotaData(&params))
				require.NoError(t, SaveQuotaDataCache())
				duplicate := keyed
				duplicate.Id = 0
				assert.Error(t, DB.Create(&duplicate).Error, "valid keys must remain unique")
			}
			var after []QuotaData
			require.NoError(t, DB.Order("id").Find(&after).Error)
			assert.Equal(t, before, after, "row IDs, valid keys, every dimension and counter must be preserved")
			var uid CliendUserQuota
			require.NoError(t, DB.First(&uid).Error)
			assert.Equal(t, keyed.Quota, uid.UsedQuota, "migration and event redelivery must not reapply UID consumption")
			if tc.duplicateKey {
				// Clean up only these disposable conflicting fixtures so the
				// remaining matrix scenarios can start with a working schema.
				resetQuotaDataState(t)
				require.NoError(t, DB.AutoMigrate(models...))
			}
		})
	}
}

type releasedUIDQuota struct {
	UsedQuota int `gorm:"type:int;default:0"`
}

type quotaDataLostReply struct{ dropped bool }

func (h *quotaDataLostReply) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *quotaDataLostReply) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	if cmd.Name() == "xadd" && !h.dropped && cmd.Err() == nil {
		h.dropped = true
		return errors.New("simulated lost XADD reply")
	}
	return nil
}
func (h *quotaDataLostReply) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *quotaDataLostReply) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	return nil
}

func TestQuotaDataRealRedisDelivery(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not configured")
	}
	options, err := redis.ParseURL(url)
	require.NoError(t, err)
	options.MaxRetries = -1
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	info, err := client.Info(ctx, "server").Result()
	require.NoError(t, err)
	t.Log(info)
	for _, tc := range []struct {
		lostReply bool
		failSQL   bool
	}{{}, {lostReply: true}, {lostReply: true, failSQL: true}} {
		t.Run(fmt.Sprintf("lost_reply_%t_sql_failure_%t", tc.lostReply, tc.failSQL), func(t *testing.T) {
			resetQuotaDataState(t)
			t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
			stream := "test:quota-data:" + uuid.NewString()
			t.Cleanup(func() { require.NoError(t, client.Del(ctx, stream).Err()) })
			producer := client
			if tc.lostReply {
				producer = redis.NewClient(options)
				producer.AddHook(&quotaDataLostReply{})
				t.Cleanup(func() { require.NoError(t, producer.Close()) })
			}
			params := quotaDataTestEvent()
			params.ModelName = strings.Repeat("模型", 64)
			event, err := newQuotaDataEvent(&params)
			require.NoError(t, err)
			if tc.failSQL {
				restore := failQuotaDataInboxWrites(t)
				err = deliverQuotaDataEvent(event, producer, stream)
				restore()
				require.NoError(t, err)
				files, err := os.ReadDir(quotaDataSpoolDir())
				require.NoError(t, err)
				require.Len(t, files, 1)
			} else {
				require.NoError(t, deliverQuotaDataEvent(event, producer, stream))
			}
			var inboxCount int64
			require.NoError(t, DB.Model(&QuotaDataEventRecord{}).Count(&inboxCount).Error)
			if tc.lostReply && !tc.failSQL {
				assert.EqualValues(t, 1, inboxCount)
			} else {
				assert.Zero(t, inboxCount)
			}
			// Group creation after enqueue must include pre-existing entries.
			require.NoError(t, client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err())
			batches, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: quotaDataConsumerGroup, Consumer: "exited-instance", Streams: []string{stream, ">"}, Count: 100, Block: -1}).Result()
			require.NoError(t, err)
			require.Len(t, batches, 1)
			require.Len(t, batches[0].Messages, 1)
			// Simulate successful SQL commit followed by process exit before ACK.
			require.NoError(t, applyQuotaDataEvents(ctx, []QuotaDataEvent{event}))
			messages, err := client.XClaim(ctx, &redis.XClaimArgs{Stream: stream, Group: quotaDataConsumerGroup, Consumer: "replacement-instance", MinIdle: 0, Messages: []string{batches[0].Messages[0].ID}}).Result()
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.NoError(t, processQuotaDataMessages(ctx, client, stream, messages))
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, params.ModelName, rows[0].ModelName)
			assert.Equal(t, 100, rows[0].Quota)
			assert.Equal(t, 1, rows[0].Count)
			pending, err := client.XPending(ctx, stream, quotaDataConsumerGroup).Result()
			require.NoError(t, err)
			assert.Zero(t, pending.Count)
			length, err := client.XLen(ctx, stream).Result()
			require.NoError(t, err)
			assert.Zero(t, length)
		})
	}
	// A malformed message must be retained in quarantine before it is ACKed.
	t.Run("quarantine", func(t *testing.T) {
		resetQuotaDataState(t)
		stream := "test:quota-data:" + uuid.NewString()
		t.Cleanup(func() { require.NoError(t, client.Del(ctx, stream).Err()) })
		require.NoError(t, client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err())
		require.NoError(t, client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"event": "broken"}}).Err())
		batches, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: quotaDataConsumerGroup, Consumer: "test", Streams: []string{stream, ">"}, Count: 10, Block: -1}).Result()
		require.NoError(t, err)
		require.NoError(t, processQuotaDataMessages(ctx, client, stream, batches[0].Messages))
		var row QuotaDataEventRecord
		require.NoError(t, DB.First(&row).Error)
		assert.Equal(t, "quarantined", row.State)
		assert.Contains(t, row.Payload, "broken")
	})
	t.Run("identity_conflict_does_not_block_valid_events", func(t *testing.T) {
		resetQuotaDataState(t)
		stream := "test:quota-data:" + uuid.NewString()
		t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), stream).Err()) })
		params := quotaDataTestEvent()
		params.EventID = uuid.NewString()
		for i, amount := range []int{100, 200, 300} {
			params.Quota = amount
			if i == 2 {
				params.EventID = uuid.NewString()
			}
			event, err := newQuotaDataEvent(&params)
			require.NoError(t, err)
			require.NoError(t, deliverQuotaDataEvent(event, client, stream))
		}
		require.NoError(t, client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err())
		batches, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: quotaDataConsumerGroup, Consumer: "conflict-test", Streams: []string{stream, ">"}, Count: 10, Block: -1}).Result()
		require.NoError(t, err)
		require.Len(t, batches[0].Messages, 3)
		require.NoError(t, processQuotaDataMessages(ctx, client, stream, batches[0].Messages))
		var row QuotaData
		require.NoError(t, DB.First(&row).Error)
		assert.Equal(t, 400, row.Quota)
		assert.Equal(t, 2, row.Count)
		var isolated QuotaDataEventRecord
		require.NoError(t, DB.Where("state = ?", "quarantined").First(&isolated).Error)
		assert.Equal(t, stream, isolated.Source)
		assert.Equal(t, batches[0].Messages[1].ID, isolated.SourceID)
		assert.Contains(t, isolated.LastError, "identity conflict")
	})

}

func TestQuotaDataRedisBatchAndCacheIsolation(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not configured")
	}
	options, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := t.Context()
	stream := "test:quota-data:" + uuid.NewString()
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), stream).Err()) })
	resetQuotaDataState(t)
	var lateClient *redis.Client
	cache := cachex.NewHybridCache(cachex.HybridCacheConfig[int]{
		Namespace: cachex.Namespace(stream + ":cache"), RedisClient: func() *redis.Client { return lateClient }, RedisCodec: cachex.IntCodec{},
	})
	require.NoError(t, cache.SetWithTTL("value", 1, time.Minute))
	lateClient = client
	require.NoError(t, cache.SetWithTTL("value", 2, time.Minute))
	value, found, err := cache.Get("value")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, value)
	require.NoError(t, client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err())
	for range 3 {
		params := quotaDataTestEvent()
		event, err := newQuotaDataEvent(&params)
		require.NoError(t, err)
		require.NoError(t, deliverQuotaDataEvent(event, client, stream))
	}
	require.NoError(t, cache.Purge())
	length, err := client.XLen(ctx, stream).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 3, length, "cache cleanup must not delete queued events")
	messages, err := readQuotaDataBatch(ctx, client, stream, "batch-consumer")
	require.NoError(t, err)
	require.Len(t, messages, 3)
	const callback = "test:quota-batch-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data_applied_events" {
			tx.AddError(errors.New("database unavailable"))
		}
	}))
	assert.Error(t, processQuotaDataMessages(ctx, client, stream, messages))
	require.NoError(t, DB.Callback().Create().Remove(callback))
	pending, err := client.XPending(ctx, stream, quotaDataConsumerGroup).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 3, pending.Count)
	receiptsBatches := 0
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data_applied_events" {
			receiptsBatches++
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.NoError(t, processQuotaDataMessages(ctx, client, stream, messages))
	assert.Equal(t, 1, receiptsBatches, "a batch must share one receipt insert and transaction")
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 300, row.Quota)
	assert.Equal(t, 3, row.Count)
	pending, err = client.XPending(ctx, stream, quotaDataConsumerGroup).Result()
	require.NoError(t, err)
	assert.Zero(t, pending.Count)
}

// Run seed, restart a disposable Redis with AOF, then run recover with the same
// test key. Never run restart/capacity operations against an application Redis.
func TestQuotaDataRedisRestart(t *testing.T) {
	url, stream, phase := os.Getenv("TEST_REDIS_RESTART_URL"), os.Getenv("TEST_REDIS_RESTART_KEY"), os.Getenv("TEST_REDIS_RESTART_PHASE")
	if url == "" {
		t.Skip("disposable restart fixture not configured")
	}
	require.Contains(t, stream, "test:quota-data:")
	options, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := t.Context()
	require.NoError(t, client.Ping(ctx).Err())
	resetQuotaDataState(t)
	if phase == "seed" {
		exists, err := client.Exists(ctx, stream).Result()
		require.NoError(t, err)
		require.Zero(t, exists)
		params := quotaDataTestEvent()
		event, err := newQuotaDataEvent(&params)
		require.NoError(t, err)
		require.NoError(t, deliverQuotaDataEvent(event, client, stream))
		require.NoError(t, client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err())
		batches, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: quotaDataConsumerGroup, Consumer: "before-restart", Streams: []string{stream, ">"}, Count: 1, Block: -1}).Result()
		require.NoError(t, err)
		require.Len(t, batches[0].Messages, 1)
		return
	}
	require.Equal(t, "recover", phase)
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), stream).Err()) })
	pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: stream, Group: quotaDataConsumerGroup, Start: "-", End: "+", Count: 10}).Result()
	require.NoError(t, err)
	require.Len(t, pending, 1, "the pending entry must survive the Redis process restart")
	messages, err := client.XClaim(ctx, &redis.XClaimArgs{Stream: stream, Group: quotaDataConsumerGroup, Consumer: "after-restart", MinIdle: 0, Messages: []string{pending[0].ID}}).Result()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.NoError(t, processQuotaDataMessages(ctx, client, stream, messages))
	require.NoError(t, processQuotaDataMessages(ctx, client, stream, messages))
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 100, row.Quota)
	assert.Equal(t, 1, row.Count)
}

func TestQuotaDataRedisCapacityFallback(t *testing.T) {
	url := os.Getenv("TEST_REDIS_CAPACITY_URL")
	if url == "" {
		t.Skip("disposable capacity fixture not configured")
	}
	options, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := t.Context()
	stream := "test:quota-data:" + uuid.NewString()
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), stream).Err()) })
	resetQuotaDataState(t)
	params := quotaDataTestEvent()
	event, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	require.NoError(t, deliverQuotaDataEvent(event, client, stream))
	config, err := client.ConfigGet(ctx, "maxmemory").Result()
	require.NoError(t, err)
	require.Len(t, config, 2)
	t.Cleanup(func() {
		require.NoError(t, client.ConfigSet(context.Background(), "maxmemory", fmt.Sprint(config[1])).Err())
	})
	require.NoError(t, client.ConfigSet(ctx, "maxmemory", "1").Err())
	// noeviction rejects new writes but must retain the previously accepted event.
	params.EventID = uuid.NewString()
	fallback, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	require.NoError(t, deliverQuotaDataEvent(fallback, client, stream))
	length, err := client.XLen(ctx, stream).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, length)
	var inbox QuotaDataEventRecord
	require.NoError(t, DB.First(&inbox).Error)
	assert.Equal(t, fallback.EventID, inbox.EventID)
	require.NoError(t, SaveQuotaDataCache())
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 100, row.Quota)
}

// Exact quota_data model from release v1.0.0-rc.45.
type releasedQuotaDataRC45 struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `json:"use_group" gorm:"index;size:64;default:''"`
	TokenID   int    `json:"token_id" gorm:"index;default:0"`
	ChannelID int    `json:"channel_id" gorm:"index;default:0"`
	NodeName  string `json:"node_name" gorm:"index;size:64;default:''"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

func TestQuotaDataRedisConfigurationFallback(t *testing.T) {
	previousClient, previousEnabled, previousSync := common.RDB, common.RedisEnabled, common.SyncFrequency
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled, common.SyncFrequency = previousClient, previousEnabled, previousSync
	})
	for _, tc := range []struct {
		name, url  string
		configured bool
	}{
		{name: "missing"}, {name: "invalid", url: "http://invalid"}, {name: "unreachable", url: "redis://127.0.0.1:1/3", configured: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetQuotaDataState(t)
			common.RDB = nil
			t.Setenv("REDIS_CONN_STRING", tc.url)
			require.NoError(t, common.InitRedisClient())
			if common.RDB != nil {
				defer common.RDB.Close()
			}
			assert.Equal(t, tc.configured, common.RedisEnabled)
			params := quotaDataTestEvent()
			require.NoError(t, LogQuotaData(&params))
			var pending int64
			require.NoError(t, DB.Model(&QuotaDataEventRecord{}).Where("state = ?", "pending").Count(&pending).Error)
			assert.EqualValues(t, 1, pending)
			require.NoError(t, SaveQuotaDataCache())
			var row QuotaData
			require.NoError(t, DB.First(&row).Error)
			assert.Equal(t, 100, row.Quota)
		})
	}
}
