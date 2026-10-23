package model

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func testSettlementDiscountConsistency(t *testing.T) {
	require.False(t, DB.Migrator().HasTable(&SettlementConfig{}))
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		require.NoError(t, ensureSettlementConfigTableSQLite())
	} else {
		require.NoError(t, DB.AutoMigrate(&SettlementConfig{}))
	}
	t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(&SettlementConfig{})) })
	useUserCacheMiniRedis(t)
	cfg := SettlementConfig{UserId: 97001, ModelName: "discount-model", Discount: 1}
	require.NoError(t, CreateSettlementConfig(&cfg))

	// An old instance may have left a still-valid cache entry. A partitioned
	// writer cannot remove it, but a healthy reader must still use the new price.
	stale, err := common.Marshal([]*SettlementConfig{&cfg})
	require.NoError(t, err)
	key := "new-api:settlement_config:v1:" + strconv.Itoa(cfg.UserId)
	require.NoError(t, common.RDB.Set(t.Context(), key, stale, 5*time.Minute).Err())
	connected := common.RDB
	failed := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, failed.Close())
	common.RDB = failed
	cfg.Discount = 0.5
	err = UpdateSettlementConfig(&cfg)
	common.RDB = connected
	require.NoError(t, err)
	assert.Equal(t, 0.5, GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName))

	// Release an old SELECT after an update. Its result must not contaminate
	// any later pricing lookup, whether Redis is enabled or disabled.
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("late_read_redis_%t", enabled), func(t *testing.T) {
			oldEnabled := common.RedisEnabled
			common.RedisEnabled = enabled
			t.Cleanup(func() { common.RedisEnabled = oldEnabled })
			cfg.Discount = 0.5
			require.NoError(t, UpdateSettlementConfig(&cfg))
			captured, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var blocked atomic.Bool
			const callback = "test:delay_discount_snapshot"
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "settlement_configs" && blocked.CompareAndSwap(false, true) {
					close(captured)
					<-release
				}
			}))
			t.Cleanup(func() { require.NoError(t, DB.Callback().Query().Remove(callback)) })
			result := make(chan float64, 1)
			go func() { result <- GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName) }()
			<-captured
			cfg.Discount = 0.25
			require.NoError(t, UpdateSettlementConfig(&cfg))
			unblock()
			assert.Equal(t, 0.5, <-result, "the overlapping request retains its original snapshot")
			assert.Equal(t, 0.25, GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName))
		})
	}
	// Moving a configuration must stop applying the old user's discount too.
	previousUser := cfg.UserId
	cfg.UserId++
	require.NoError(t, UpdateSettlementConfig(&cfg))
	assert.Equal(t, 1.0, GetUserModelSettlementDiscount(previousUser, cfg.ModelName))
	assert.Equal(t, 0.25, GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName))
	require.NoError(t, BatchCreateSettlementConfigs([]*SettlementConfig{
		{UserId: cfg.UserId, ModelName: "discount-*", Discount: 0.8},
		{UserId: cfg.UserId, ModelName: "discount-model*", Discount: 0.6},
		{UserId: cfg.UserId, ModelName: cfg.ModelName, Discount: 0.2},
	}))
	assert.Equal(t, 0.2, GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName))
	require.NoError(t, DeleteSettlementConfig(cfg.Id))
	assert.Equal(t, 0.6, GetUserModelSettlementDiscount(cfg.UserId, cfg.ModelName))
	assert.Equal(t, 0.8, GetUserModelSettlementDiscount(cfg.UserId, "discount-other"))
}

func failQuotaDataInboxWrites(t *testing.T) func() {
	t.Helper()
	const callback = "test:quota_inbox_unavailable"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data_events" {
			tx.AddError(errors.New("injected quota inbox outage"))
		}
	}))
	var once sync.Once
	restore := func() { once.Do(func() { require.NoError(t, DB.Callback().Create().Remove(callback)) }) }
	t.Cleanup(restore)
	return restore
}

func testQuotaDataSpoolRecovery(t *testing.T) {
	for _, redisEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("redis_%t", redisEnabled), func(t *testing.T) {
			resetQuotaDataState(t)
			t.Setenv("QUOTA_DATA_SPOOL_DIR", filepath.Join(t.TempDir(), "spool"))
			useUserCacheMiniRedis(t)
			common.RedisEnabled = redisEnabled
			require.NoError(t, common.RDB.Close())
			restore := failQuotaDataInboxWrites(t)
			params := quotaDataTestEvent()
			var events []QuotaDataEvent
			for _, amount := range []int{100, 100, -20} {
				params.EventID, params.Quota = uuid.NewString(), amount
				params.IsSettlementAdjustment = amount < 0
				event, err := newQuotaDataEvent(&params)
				require.NoError(t, err)
				events = append(events, event)
				require.NoError(t, LogQuotaData(&params))
			}
			files, err := os.ReadDir(quotaDataSpoolDir())
			require.NoError(t, err)
			require.Len(t, files, 3)
			assert.Error(t, SaveQuotaDataCache(), "failed replay must retain every file")
			afterFailure, err := os.ReadDir(quotaDataSpoolDir())
			require.NoError(t, err)
			assert.Len(t, afterFailure, 3)
			restore()
			// Model an XADD whose successful reply was lost: this same event has
			// already been consumed by a different instance before local recovery.
			require.NoError(t, applyQuotaDataEvents(t.Context(), events[:1]))
			for range 2 {
				require.NoError(t, SaveQuotaDataCache())
			}
			var row QuotaData
			require.NoError(t, DB.First(&row).Error)
			want := QuotaData{
				Id: row.Id, BucketKey: row.BucketKey, UserID: 1, Username: "quota-test", ModelName: "test-model", CreatedAt: 3600,
				TokenId: 11, TokenName: "test-token", ChannelId: 22, UseGroup: "default", NodeName: "node-a",
				ClientUserId: "uid-a", ClientScenairo: "scenario-a", ProjectName: "project-a", PlanId: 1,
				Quota: 180, Count: 2, TokenUsed: 30, PromptTokens: 20, CompletionTokens: 10, CachedTokens: 8,
				ClaudeCacheCreation5mTokens: 4, ClaudeCacheCreation1hTokens: 6,
				CacheWrite5mRequestCount: 2, CacheWrite1hRequestCount: 2, CacheReadRequestCount: 2,
				StreamRequestCount: 2, FrtSum: 200, RequestTimeSum: 4000,
			}
			assert.Equal(t, want, row)
			var uid CliendUserQuota
			require.NoError(t, DB.First(&uid).Error)
			assert.Equal(t, 180, uid.UsedQuota)
			files, err = os.ReadDir(quotaDataSpoolDir())
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}

func TestQuotaDataSpoolFailureBoundaries(t *testing.T) {
	resetQuotaDataState(t)
	dir := t.TempDir()
	t.Setenv("QUOTA_DATA_SPOOL_DIR", dir)
	params := quotaDataTestEvent()
	params.EventID = uuid.NewString()
	event, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	encoded, err := common.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, persistQuotaDataEvent(t.Context(), event, string(encoded)))
	params.Quota++
	conflict, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	assert.ErrorIs(t, deliverQuotaDataEvent(conflict, nil, ""), errInvalidQuotaDataEvent)
	encoded, err = common.Marshal(conflict)
	require.NoError(t, err)
	require.NoError(t, spoolQuotaDataEvent(encoded))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "malformed.json"), []byte("broken"), 0600))
	params.EventID = uuid.NewString()
	valid, err := newQuotaDataEvent(&params)
	require.NoError(t, err)
	encoded, err = common.Marshal(valid)
	require.NoError(t, err)
	require.NoError(t, spoolQuotaDataEvent(encoded))
	require.NoError(t, SaveQuotaDataCache())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2, "invalid payloads must survive in quarantine")
	for _, file := range files {
		assert.True(t, strings.HasSuffix(file.Name(), ".quarantined"))
	}
	var row QuotaData
	require.NoError(t, DB.First(&row).Error)
	assert.Equal(t, 201, row.Quota)
	assert.Equal(t, 2, row.Count)

	// Disk rejection must be surfaced, never reported as durable acceptance.
	blocked := filepath.Join(t.TempDir(), "file-not-directory")
	require.NoError(t, os.WriteFile(blocked, nil, 0600))
	t.Setenv("QUOTA_DATA_SPOOL_DIR", blocked)
	failQuotaDataInboxWrites(t)
	params.EventID = uuid.NewString()
	err = LogQuotaData(&params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected quota inbox outage")
	assert.Contains(t, err.Error(), "quota data spool")
}

func TestQuotaDataSpoolProcessRecovery(t *testing.T) {
	// The child exits with SQL unavailable, after the real consumption/refund
	// entry points return. Only files survive into the parent/replacement process.
	if dir := os.Getenv("TEST_QUOTA_SPOOL_CHILD"); dir != "" {
		t.Setenv("QUOTA_DATA_SPOOL_DIR", dir)
		common.DataExportEnabled, common.LogConsumeEnabled = true, false
		user := createReserveTestUser(t, 100)
		require.NoError(t, DecreaseUserQuota(user.Id, 20, true))
		useUserCacheMiniRedis(t)
		require.NoError(t, common.RDB.Close())
		failQuotaDataInboxWrites(t)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		c.Set("username", user.Username)
		RecordConsumeLog(c, user.Id, RecordConsumeLogParams{
			ModelName: "replay-model", Quota: 20, PromptTokens: 3, CompletionTokens: 2,
			Group: "default", ClientUserId: "replay-uid", ClientScenairo: "replay-scenario", ProjectName: "replay-project", PlanId: 42,
		})
		require.NoError(t, IncreaseUserQuota(user.Id, 5, true))
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			EventID: uuid.NewString(), UserId: user.Id, LogType: LogTypeRefund, ModelName: "replay-model", Quota: 5,
			Group: "default", ClientUserId: "replay-uid", ClientScenairo: "replay-scenario", ProjectName: "replay-project", PlanId: 42,
		})
		assert.Equal(t, 85, getUserQuotaFromDB(t, user.Id), "logging must not refund funds a second time")
		return
	}
	resetQuotaDataState(t)
	dir := t.TempDir()
	t.Setenv("QUOTA_DATA_SPOOL_DIR", dir)
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestQuotaDataSpoolProcessRecovery$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "TEST_QUOTA_SPOOL_CHILD="+dir)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	// Replay must also run when new statistics production has been disabled.
	oldExport := common.DataExportEnabled
	common.DataExportEnabled = false
	t.Cleanup(func() { common.DataExportEnabled = oldExport })
	ctx, cancel := context.WithCancel(t.Context())
	done := StartQuotaDataWorkers(ctx)
	defer func() { cancel(); <-done }()
	require.Eventually(t, func() bool {
		var applied int64
		return DB.Model(&QuotaDataAppliedEvent{}).Count(&applied).Error == nil && applied == 2
	}, 5*time.Second, 10*time.Millisecond, "startup workers must recover the previous process's files")
	cancel()
	<-done
	for range 2 {
		require.NoError(t, SaveQuotaDataCache())
	}
	var rows []QuotaData
	require.NoError(t, DB.Find(&rows).Error)
	var quota, count, tokens int
	for _, row := range rows {
		quota += row.Quota
		count += row.Count
		tokens += row.TokenUsed
		assert.Equal(t, "replay-uid", row.ClientUserId)
		assert.Equal(t, "replay-scenario", row.ClientScenairo)
		assert.Equal(t, "replay-project", row.ProjectName)
		assert.Equal(t, 42, row.PlanId)
	}
	assert.Equal(t, 15, quota)
	assert.Equal(t, 1, count)
	assert.Equal(t, 5, tokens)
}
