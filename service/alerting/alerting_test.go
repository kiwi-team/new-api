package alerting_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Representative released primary tables from v1.0.0-rc.47. The release uses
// the same GORM/driver versions as this branch; no release binary is executed.
type alertReleasedUser struct {
	Id                   int                        `json:"id"`
	Username             string                     `json:"username" gorm:"unique;index" validate:"max=20"`
	Password             string                     `json:"password" gorm:"not null;" validate:"min=8,max=128"`
	HasPassword          bool                       `json:"-" gorm:"-:all"`
	OriginalPassword     string                     `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName          string                     `json:"display_name" gorm:"index" validate:"max=20"`
	Role                 int                        `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status               int                        `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email                string                     `json:"email" gorm:"index" validate:"max=50"`
	GitHubId             string                     `json:"github_id" gorm:"column:github_id;index"`
	DiscordId            string                     `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId               string                     `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId             string                     `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId           string                     `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode     string                     `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken          *string                    `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
	AccessTokenCreatedAt *int64                     `json:"-" gorm:"type:bigint;column:access_token_created_at"`    // Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
	Quota                int                        `json:"quota" gorm:"type:int;default:0"`
	UsedQuota            int                        `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount         int                        `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group                string                     `json:"group" gorm:"type:varchar(64);default:'default'"`
	AffCode              string                     `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount             int                        `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota             int                        `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota      int                        `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId            int                        `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt            gorm.DeletedAt             `gorm:"index"`
	LinuxDOId            string                     `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting              string                     `json:"setting" gorm:"type:text;column:setting"`
	Remark               string                     `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer       string                     `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt            int64                      `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt          int64                      `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	AuthVersion          int64                      `json:"-" gorm:"type:bigint;not null;default:1;column:auth_version"`
	AdminPermissions     map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`
}

func (alertReleasedUser) TableName() string { return "users" }

type alertReleasedToken struct {
	Id                 int            `json:"id"`
	UserId             int            `json:"user_id" gorm:"index"`
	Key                string         `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status             int            `json:"status" gorm:"default:1"`
	Name               string         `json:"name" gorm:"index" `
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	AccessedTime       int64          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota        int            `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota     bool           `json:"unlimited_quota"`
	ModelLimitsEnabled bool           `json:"model_limits_enabled"`
	ModelLimits        string         `json:"model_limits" gorm:"type:text"`
	AllowIps           *string        `json:"allow_ips" gorm:"default:''"`
	UsedQuota          int            `json:"used_quota" gorm:"default:0"` // used quota
	Group              string         `json:"group" gorm:"default:''"`
	CrossGroupRetry    bool           `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	AutoGroups         string         `json:"-" gorm:"type:text"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
	// 允许用此令牌查询所属用户的额度；新建令牌默认开启，已有令牌升级后保持关闭
	AllowUserQuotaQuery bool `json:"allow_user_quota_query"`
}

func (alertReleasedToken) TableName() string { return "tokens" }

func TestAlertingDatabaseMatrix(t *testing.T) {
	t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
	t.Setenv("ALERTING_SPOOL_DIR", t.TempDir())
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "alerts.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
			case "mysql":
				dsn := os.Getenv("ALERTING_TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("ALERTING_TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("ALERTING_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("ALERTING_TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(4)
			old := model.DB
			oldType := common.MainDatabaseType()
			model.DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			t.Cleanup(func() { model.DB = old; common.SetMainDatabaseType(oldType); require.NoError(t, connection.Close()) })
			core := []any{&model.User{}, &model.Token{}, &model.Channel{}, &model.SystemTaskLock{}, &model.Ability{}, &model.SystemInstance{}, &model.Project{}, &model.ProjectAllocationPlan{}, &model.ProjectAllocation{}, &model.CliendUserQuota{}, &model.QuotaData{}, &model.QuotaDataEventRecord{}, &model.QuotaDataAppliedEvent{}, &model.UserSubscription{}, &model.Option{}}
			for _, table := range append(core, model.AlertModels()...) {
				require.False(t, db.Migrator().HasTable(table), "use an empty disposable test database")
			}
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(append(model.AlertModels(), core...)...)) })
			coreDB := db
			// The released core UID index already requires large-prefix row
			// format with utf8mb4 on 5.7.8. Alert tables below deliberately use
			// the server default, exercising their own 767-byte index bounds.
			if dialect == "mysql" {
				coreDB = db.Set("gorm:table_options", "ROW_FORMAT=DYNAMIC")
			}
			require.NoError(t, coreDB.AutoMigrate(&alertReleasedUser{}, &alertReleasedToken{}))
			require.NoError(t, db.Create(&alertReleasedUser{Id: 77, Username: "released-owner", Password: "invalid-fixture-password", AffCode: "released-77", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 123456, UsedQuota: 234, RequestCount: 5}).Error)
			require.NoError(t, db.Create(&alertReleasedToken{Id: 78, UserId: 77, Key: "released-fixture-key", Name: "Released business key", Status: common.TokenStatusEnabled, RemainQuota: 98765, UsedQuota: 456}).Error)
			for range 2 {
				require.NoError(t, coreDB.AutoMigrate(core...))
			}
			var upgradedUser model.User
			var upgradedToken model.Token
			require.NoError(t, db.First(&upgradedUser, 77).Error)
			require.NoError(t, db.First(&upgradedToken, 78).Error)
			assert.EqualValues(t, 123456, upgradedUser.Quota)
			assert.EqualValues(t, 234, upgradedUser.UsedQuota)
			assert.EqualValues(t, 5, upgradedUser.RequestCount)
			assert.Equal(t, "Released business key", upgradedToken.Name)
			assert.EqualValues(t, 98765, upgradedToken.RemainQuota)
			assert.EqualValues(t, 456, upgradedToken.UsedQuota)
			assert.False(t, upgradedToken.AllowUserQuotaQuery)
			assert.EqualValues(t, 1, upgradedToken.AuthVersion)
			require.Error(t, db.Create(&model.Token{UserId: 77, Key: "released-fixture-key", Name: "Duplicate released key"}).Error, "upgrading must preserve credential uniqueness")
			// The previous released schema contains no alert tables. Preserve its
			// existing users/tokens while adding and repeatedly migrating alerts.
			require.NoError(t, db.Create(&model.User{Id: 1, Username: "alert-owner", AffCode: "alert-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.User{Id: 2, Username: "other-owner", AffCode: "other-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.User{Id: 3, Username: "alert-root", AffCode: "alert-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 11, UserId: 1, Name: "business", Key: "alert-test-token", Status: common.TokenStatusEnabled}).Error)
			for range 2 {
				require.NoError(t, model.MigrateAlertModels(db))
			}
			require.NoError(t, model.InitializeAlertNamespace(context.Background()))
			now := time.Now()
			require.NoError(t, db.Create(&model.SystemInstance{NodeName: "alert-test-node", Info: `{"runtime":{"alerting_boot_id":"alert-test-boot"}}`, StartedAt: now.Add(-time.Minute).Unix(), LastSeenAt: now.Unix()}).Error)
			require.NoError(t, db.Create(&model.AlertWorkerCheckpoint{ID: "alert-test-boot", NodeID: "alert-test-node", BootID: "alert-test-boot", Kind: "producer", UsageHealthy: true, Healthy: true, UpdatedAt: now.UnixMilli(), ProtocolVersion: model.AlertSourceProtocolVersion, CapturedAt: now.Add(-time.Second).UnixMilli(), CoverageStartedAt: now.Add(-72 * time.Hour).UnixMilli()}).Error)
			var token model.Token
			require.NoError(t, db.First(&token, 11).Error)
			assert.Equal(t, "business", token.Name)
			assert.Equal(t, 1, token.UserId)
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("%s version: %s", dialect, version)
			t.Run("immutable lifecycle and terminal tombstones", testAlertLifecycle)
			t.Run("source prefix rejects future receipts masking gaps", testAlertSourcePrefix)
			t.Run("ownership and incident delivery", testAlertIncident)
			t.Run("withdrawn notification authorization", testAlertNotificationRevocation)
			t.Run("low traffic and source loss", testAlertInflight)
			t.Run("HTTP object isolation", testAlertHTTPIsolation)
			t.Run("scoped isolation and alternative guard", testAlertChannelIsolation)
			t.Run("token holds and manual edit precedence", testAlertTokenHolds)
			t.Run("net spending and milestones", testAlertMilestones)
			t.Run("future key first charge and owner isolation", testAlertFutureKey)
			t.Run("Redis ingestion reclaim and SQL fallback", testAlertRedisTransport)
			t.Run("budget pool and plan isolation", testAlertBudgetPools)
			t.Run("independent recovery latency and coverage", testAlertRecovery)
			t.Run("reports partial revision and backfill", testAlertReports)
			t.Run("batch migration is retryable and preserves legacy delivery", testAlertBatchMigration)
			t.Run("legacy baseline shadow cutover and rollback", testAlertMigration)
			t.Run("account milestones retain baseline and refund high water", testAlertUserSpendMigration)
			t.Run("daily report cutover retains accepted dates", testAlertUserReportMigration)
			t.Run("natural Key migration schedules an unambiguous boundary", testAlertKeyPeriodMigration)
			t.Run("legacy event adapters retain checkpoints and a single executor", testAlertLegacyNoticeMigration)
			t.Run("bad rule cannot starve other rules", testAlertEvaluationIsolation)
			t.Run("reserved platform delivery and atomic limits", testAlertPriority)
			t.Run("shared metric projection and late facts", testAlertMetricCache)
			t.Run("milestone action freshness and historical preview", testAlertMilestoneActions)
			t.Run("high priority fallback preserves primary receipts", testAlertFallback)
			t.Run("target ownership rotation and private allowlist", testAlertTargetManagement)
			t.Run("independent watchdog deduplicates receipts", testAlertWatchdog)
			t.Run("slow channel distribution and threshold calibration", testAlertSlowChannelCalibration)
			t.Run("usage bridge with statistics and logs disabled", testAlertUsageBridge)
		})
	}
}

func testAlertSourcePrefix(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	boot := "prefix-proof-boot"
	source := model.AlertWorkerCheckpoint{ID: boot, BootID: boot, Kind: "producer", OfferedSequence: 1, CapturedAt: now - 1000, UpdatedAt: now, UsageHealthy: true, Healthy: true, ProtocolVersion: model.AlertSourceProtocolVersion}
	require.NoError(t, model.DB.Create(&source).Error)
	o := model.AlertObservation{ID: model.AlertIdentity("prefix-second"), Namespace: "health", SourceID: model.AlertIdentity("prefix-life"), Kind: "started", Layer: "request", Sequence: 2, SourceSequence: 2, BootID: boot, StartedAt: now - 2000, OccurredAt: now - 1500}
	require.NoError(t, model.AcceptAlertObservation(ctx, o))
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, boot))
	require.NoError(t, model.DB.First(&source, "id = ?", boot).Error)
	assert.EqualValues(t, 1, source.AcceptedCount)
	assert.Zero(t, source.AcceptedThrough, "a newer receipt cannot fill an older gap")
	assert.Zero(t, source.LastCompleteAt)
	conflict := o
	conflict.ID = model.AlertIdentity("prefix-position-collision")
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, conflict), model.ErrAlertObservationConflict)
	o.ID, o.Sequence, o.SourceSequence = model.AlertIdentity("prefix-first"), 1, 1
	require.NoError(t, model.AcceptAlertObservation(ctx, o))
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, boot))
	require.NoError(t, model.DB.First(&source, "id = ?", boot).Error)
	assert.EqualValues(t, 1, source.AcceptedThrough)
	assert.EqualValues(t, now-1000, source.LastCompleteAt)
	require.NoError(t, model.DB.Model(&source).Update("offered_sequence", 2).Error)
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, boot))
	require.NoError(t, model.DB.First(&source, "id = ?", boot).Error)
	assert.EqualValues(t, 2, source.AcceptedThrough)
	old := model.AlertWorkerCheckpoint{ID: "retired-fast-clock-boot", BootID: "retired-fast-clock-boot", NodeID: "alert-test-node", Kind: "producer", ProtocolVersion: model.AlertSourceProtocolVersion, Healthy: true, UsageHealthy: true, UpdatedAt: now + 3600000, CapturedAt: now + 3600000, CoverageStartedAt: now - 86400000}
	require.NoError(t, model.DB.Create(&old).Error)
	_, status, reason, err := alerting.SourceWatermark(ctx, time.UnixMilli(now))
	require.NoError(t, err)
	assert.Equal(t, "complete", status, reason)
	// The reporter switches boot identity before the new collector is ready.
	// An older healthy heartbeat cannot conceal this missing current producer.
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("info", `{"runtime":{"alerting_boot_id":"not-yet-covered-boot"}}`).Error)
	_, status, reason, err = alerting.SourceWatermark(ctx, time.UnixMilli(now))
	require.NoError(t, err)
	assert.Equal(t, "partial", status)
	assert.Equal(t, "producer_not_covered", reason)
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("info", `{"runtime":{"alerting_boot_id":"alert-test-boot"}}`).Error)
}

func testAlertNotificationRevocation(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	oldShadow := common.AlertingShadowMode
	common.AlertingShadowMode = false
	t.Cleanup(func() { common.AlertingShadowMode = oldShadow })
	row, spec, target := alertRuleFixture(t, "request_count")
	spec.Condition.Operator, spec.Condition.ThresholdCount, spec.Condition.RecoveryThresholdCount, spec.Condition.MinSamples = "gte", 1, 0, 1
	actor := alerting.Actor{ID: 1, Role: common.RoleCommonUser}
	updated, err := alerting.SaveRule(ctx, actor, 1, row.ID, row.Revision, spec)
	require.NoError(t, err)
	row = *updated
	spec, err = model.AlertRuleConfig(row)
	require.NoError(t, err)
	engine := alerting.Engine{BootID: "notification-authorization"}
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: 1, Samples: 1}, now))
	var job model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "notify").First(&job).Error)
	replacement := model.AlertTarget{OwnerUserID: 1, Name: "Replacement operations destination", Type: "email", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&replacement).Error)
	spec.Notify.TargetIDs = []int64{replacement.ID}
	_, err = alerting.SaveRule(ctx, actor, 1, row.ID, row.Revision, spec)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&job, job.ID).Error)
	assert.Equal(t, "canceled", job.Status)
	assert.Equal(t, "authorization_revoked", job.LastError)
	// Replaying stale ready work cannot restore a withdrawn subscription.
	require.NoError(t, model.DB.Model(&job).Updates(map[string]any{"status": "pending", "available_at": time.Now().Add(-time.Second).UnixMilli()}).Error)
	delivered := 0
	engine.Deliver = func(_ context.Context, _ model.AlertTarget, event model.AlertEvent) (int, error) {
		if event.RuleID == row.ID {
			delivered++
		}
		return 200, nil
	}
	require.NoError(t, engine.RunJobs(ctx, "notify", time.Now()))
	assert.Zero(t, delivered)
	require.NoError(t, model.DB.First(&job, job.ID).Error)
	assert.Equal(t, "canceled", job.Status)
	require.NoError(t, model.DB.Model(&job).Update("status", "failed").Error)
	api := gin.New()
	api.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleCommonUser); c.Next() })
	api.POST("/jobs/:id/retry", controller.RetryAlertJob)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/jobs/%d/retry", job.ID), strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	api.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code, "a manual retry needs a destination in the current rule configuration")

	paused, pausedSpec, pausedTarget := alertRuleFixture(t, "request_count")
	pausedSpec.Condition.Operator, pausedSpec.Condition.ThresholdCount, pausedSpec.Condition.MinSamples = "gte", 1, 1
	updated, err = alerting.SaveRule(ctx, actor, 1, paused.ID, paused.Revision, pausedSpec)
	require.NoError(t, err)
	paused = *updated
	pausedSpec, err = model.AlertRuleConfig(paused)
	require.NoError(t, err)
	require.NoError(t, engine.ApplyMeasurement(ctx, paused, pausedSpec, pausedTarget, alerting.Measurement{Status: "complete", Value: 1, Samples: 1}, time.Now()))
	pausedSpec.Enabled = false
	_, err = alerting.SaveRule(ctx, actor, 1, paused.ID, paused.Revision, pausedSpec)
	require.NoError(t, err)
	pausedDeliveries := 0
	engine.Deliver = func(_ context.Context, _ model.AlertTarget, event model.AlertEvent) (int, error) {
		if event.RuleID == paused.ID {
			pausedDeliveries++
		}
		return 200, nil
	}
	require.NoError(t, engine.RunJobs(ctx, "notify", time.Now()))
	assert.Equal(t, 1, pausedDeliveries, "pause retains already generated notices when their destination remains authorized")
}

func testAlertFutureKey(t *testing.T) {
	ctx := context.Background()
	_, spec, _ := alertRuleFixture(t, "spend_net")
	spec.Scope = model.AlertScope{Type: "api_key", Selector: "all_owned", IncludeFuture: true, GroupMode: "per_entity"}
	spec.Condition.ThresholdUSD, spec.Condition.RecoveryThresholdUSD = "100", "0"
	spec.Condition.Window.Type = "cumulative"
	spec.Condition.MinSamples = 0
	spec.Trigger.Mode, spec.Trigger.StepUSD = "milestone", "100"
	rule, err := alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, 0, 0, spec)
	require.NoError(t, err)
	spec, err = model.AlertRuleConfig(*rule)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Token{Id: 6013, UserId: 1, Key: "future-first-charge", Name: "Future business key", CreatedTime: rule.EffectiveAt / 1000, Status: common.TokenStatusEnabled}).Error)
	for i, owner := range []int{1, 2} {
		fact := model.AlertObservation{ID: model.AlertIdentity("future-key-fact", i), Namespace: "usage", SourceID: fmt.Sprintf("future-key-fact-%d", i), Kind: "usage", UserID: owner, TokenID: 6013, Quota: 75000000, RequestCount: 1, OccurredAt: rule.EffectiveAt + 1000}
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	targets, err := alerting.ResolveTargets(ctx, *rule, spec)
	require.NoError(t, err)
	var target alerting.MonitoredTarget
	for _, candidate := range targets {
		if candidate.ID == 6013 {
			target = candidate
			break
		}
	}
	require.Equal(t, 6013, target.ID)
	now := time.UnixMilli(rule.EffectiveAt + 10000)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
	t.Cleanup(func() {
		at := time.Now()
		require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": at.UnixMilli(), "captured_at": at.Add(-time.Second).UnixMilli()}).Error)
	})
	m, err := alerting.Measure(ctx, *rule, spec, target, now)
	require.NoError(t, err)
	require.Equal(t, "complete", m.Status, m.Reason)
	assert.EqualValues(t, 75000000, m.Value, "retain the first $150 charge before discovery and exclude a different owner's historical attribution")
}

func testAlertBudgetPools(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	date := now.AddDate(0, 0, 1).Format("20060102")
	project := model.Project{Id: 71, ProjectName: "Research", ActivePlanId: 81, Status: model.ProjectStatusEnabled}
	require.NoError(t, model.DB.Create(&project).Error)
	for _, id := range []int{81, 82} {
		require.NoError(t, model.DB.Create(&model.ProjectAllocationPlan{Id: id, ProjectId: 71, PlanName: fmt.Sprintf("Plan %d", id), EndDate: date}).Error)
	}
	for _, allocation := range []model.ProjectAllocation{{Id: 91, ProjectId: 71, PlanId: 81, ClientUserId: "business-uid", AllocatedQuota: 100, UsedQuota: 40000000}, {Id: 92, ProjectId: 71, PlanId: 82, ClientUserId: "business-uid", AllocatedQuota: 500, UsedQuota: 1}} {
		require.NoError(t, model.DB.Create(&allocation).Error)
	}
	spec := model.AlertRuleSpec{Scope: model.AlertScope{Type: "project_budget", EntityIDs: []int{91}}, Condition: model.AlertCondition{Metric: "budget_remaining_ratio"}}
	row := model.AlertRule{OwnerUserID: 0, EffectiveAt: now.UnixMilli()}
	targets, err := alerting.ResolveTargets(ctx, row, spec)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, 81, targets[0].PlanID)
	assert.Contains(t, targets[0].Name, "Research")
	m, err := alerting.Measure(ctx, row, spec, targets[0], now)
	require.NoError(t, err)
	assert.Equal(t, "complete", m.Status)
	assert.EqualValues(t, 2000, m.Value, "$80 used out of $100 leaves 20%, independent of the other plan")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", 750000).Error)
	spec.Scope.Type = "account"
	spec.Condition.Metric = "balance_remaining"
	m, err = alerting.Measure(ctx, model.AlertRule{OwnerUserID: 1}, spec, alerting.MonitoredTarget{Type: "account", ID: 1}, now)
	require.NoError(t, err)
	assert.EqualValues(t, 750000, m.Value, "wallet values remain internal quota units")
}

type alertHTTPTransport struct {
	send func(*http.Request) (*http.Response, error)
}

func (t alertHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return t.send(request)
}

func TestAlertDeliveryProtocolsAndBusinessErrors(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	vault, err := alerting.NewTargetVault("test-key", map[string]string{"test-key": key})
	require.NoError(t, err)
	for _, kind := range []string{"feishu", "wecom", "webhook_v1", "webhook_v2", "bark", "gotify"} {
		t.Run(kind, func(t *testing.T) {
			secret := alerting.TargetSecret{Address: "https://1.1.1.1/notify?key=private", Secret: "signature-secret", Token: "application-secret"}
			sealed, err := vault.Seal(1, kind, secret)
			require.NoError(t, err)
			target := model.AlertTarget{ID: 17, OwnerUserID: 1, Type: kind, Revision: 3, KeyID: "test-key", Ciphertext: sealed, Language: "en"}
			var deliveryID string
			businessFailure := false
			vault.Client = &http.Client{Transport: alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.Unmarshal(body, &payload))
				switch kind {
				case "feishu":
					assert.Equal(t, "text", payload["msg_type"])
					stamp, ok := payload["timestamp"].(string)
					require.True(t, ok)
					mac := hmac.New(sha256.New, []byte(stamp+"\n"+secret.Secret))
					assert.Equal(t, base64.StdEncoding.EncodeToString(mac.Sum(nil)), payload["sign"])
				case "wecom":
					assert.Equal(t, "text", payload["msgtype"])
				case "gotify":
					assert.Equal(t, secret.Token, request.Header.Get("X-Gotify-Key"))
				case "webhook_v2":
					current := request.Header.Get("X-NewAPI-Delivery-ID")
					require.NotEmpty(t, current)
					if deliveryID != "" {
						assert.Equal(t, deliveryID, current)
					}
					deliveryID = current
					mac := hmac.New(sha256.New, []byte(secret.Secret))
					_, err = mac.Write([]byte(request.Header.Get("X-NewAPI-Timestamp") + "." + current + "."))
					require.NoError(t, err)
					_, err = mac.Write(body)
					require.NoError(t, err)
					assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), request.Header.Get("X-NewAPI-Signature"))
				}
				response := `{"code":0,"errcode":0}`
				if kind == "bark" {
					response = `{"code":200}`
				}
				if businessFailure {
					response = `{"code":1,"errcode":1}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
			}}}
			event := model.AlertEvent{ID: 91, Kind: "test", Summary: "Test alert"}
			for range 2 {
				status, err := vault.Deliver(context.Background(), target, event)
				require.NoError(t, err)
				assert.Equal(t, 200, status)
			}
			if kind == "feishu" || kind == "wecom" || kind == "bark" {
				businessFailure = true
				_, err := vault.Deliver(context.Background(), target, event)
				require.Error(t, err, "HTTP 200 with a platform error is not successful delivery")
				assert.NotContains(t, err.Error(), "private")
			}
		})
	}
}

func testAlertRedisTransport(t *testing.T) {
	t.Setenv("ALERTING_REDIS_INGEST_ENABLED", "true")
	socket := os.Getenv("ALERTING_TEST_REDIS_SOCKET")
	if socket == "" {
		t.Skip("ALERTING_TEST_REDIS_SOCKET not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Ping(ctx).Err())
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldRDB })
	// The caller supplies a disposable Redis endpoint; touch only our namespace.
	keys, err := client.Keys(ctx, model.AlertRedisKey("*")).Result()
	require.NoError(t, err)
	if len(keys) > 0 {
		require.NoError(t, client.Del(ctx, keys...).Err())
	}
	now := time.Now().UnixMilli()
	o := model.AlertObservation{ID: model.AlertIdentity("redis-start"), Namespace: "health", SourceID: model.AlertIdentity("redis-life"), Kind: "started", Layer: "channel_attempt", Sequence: 1, SourceSequence: 1, BootID: "redis-boot", NodeID: "redis-node", StartedAt: now, OccurredAt: now, ChannelID: 51, ModelKey: "model-c", SampleClass: "production"}
	require.NoError(t, model.EnqueueAlertObservation(ctx, o))
	var count int64
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("id = ?", o.ID).Count(&count).Error)
	assert.Zero(t, count, "Redis transports observations before the SQL consumer applies them")
	require.NoError(t, model.ConsumeAlertHealth(ctx, client, "worker-a"))
	var source model.AlertWorkerCheckpoint
	require.NoError(t, model.DB.First(&source, "id = ?", o.BootID).Error)
	assert.EqualValues(t, 1, source.AcceptedCount)
	payload, err := common.Marshal(o)
	require.NoError(t, err)
	id, err := client.XAdd(ctx, &redis.XAddArgs{Stream: model.AlertHealthStream, Values: map[string]any{"event": string(payload)}}).Result()
	require.NoError(t, err)
	// Redis 7+ adds XINFO fields that go-redis v8's fixed reply decoder
	// does not accept. Read the forward-compatible protocol map here.
	info, err := client.Do(ctx, "XINFO", "GROUPS", model.AlertHealthStream).Slice()
	require.NoError(t, err)
	require.Len(t, info, 1)
	groupFields, ok := info[0].([]any)
	require.True(t, ok)
	group := ""
	for i := 0; i+1 < len(groupFields); i += 2 {
		if groupFields[i] == "name" {
			group = fmt.Sprint(groupFields[i+1])
		}
	}
	require.NotEmpty(t, group)
	_, err = client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: group, Consumer: "crashed-worker", Streams: []string{model.AlertHealthStream, ">"}, Count: 1}).Result()
	require.NoError(t, err)
	// Set pending idle age on the real server rather than sleeping in a test.
	require.NoError(t, client.Do(ctx, "XCLAIM", model.AlertHealthStream, group, "crashed-worker", 0, id, "IDLE", 120000).Err())
	require.NoError(t, model.ConsumeAlertHealth(ctx, client, "takeover-worker"))
	require.NoError(t, model.DB.First(&source, "id = ?", o.BootID).Error)
	assert.EqualValues(t, 1, source.AcceptedCount, "a reclaimed duplicate cannot advance the accepted watermark twice")
	require.NoError(t, model.PublishAlertOutbox(ctx, client))
	assert.NoError(t, client.ZScore(ctx, model.AlertRedisKey("active"), o.SourceID).Err())
	dead := redis.NewClient(&redis.Options{Network: "unix", Addr: filepath.Join(t.TempDir(), "unavailable.sock"), MaxRetries: -1, DialTimeout: 10 * time.Millisecond})
	defer dead.Close()
	common.RDB = dead
	fallback := o
	fallback.ID = model.AlertIdentity("redis-fallback")
	fallback.SourceID = model.AlertIdentity("redis-fallback-life")
	fallback.SourceSequence = 2
	require.NoError(t, model.EnqueueAlertObservation(ctx, fallback))
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("id = ?", fallback.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	// SQL and Redis outage accepts to a synced local spool; replay uses the
	// original immutable identity and cannot manufacture a terminal outcome.
	t.Setenv("ALERTING_SPOOL_DIR", filepath.Join(t.TempDir(), "spool"))
	workingDB := model.DB
	brokenDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "closed.db")), &gorm.Config{})
	require.NoError(t, err)
	connection, err := brokenDB.DB()
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	model.DB = brokenDB
	spooled := o
	spooled.ID = model.AlertIdentity("spooled-start")
	spooled.SourceID = model.AlertIdentity("spooled-life")
	spooled.SourceSequence = 3
	err = model.EnqueueAlertObservation(ctx, spooled)
	model.DB = workingDB
	require.NoError(t, err)
	require.NoError(t, model.ReplayAlertSpool(ctx))
	var lifecycle model.AlertLifecycle
	require.NoError(t, model.DB.First(&lifecycle, "id = ?", spooled.SourceID).Error)
	assert.Zero(t, lifecycle.FinishedAt)
	require.NoError(t, model.SpoolAlertObservation([]byte("{invalid")))
	spooled.ID, spooled.SourceID, spooled.SourceSequence = model.AlertIdentity("spool-after-poison"), model.AlertIdentity("spool-next-life"), 4
	payload, err = common.Marshal(spooled)
	require.NoError(t, err)
	require.NoError(t, model.SpoolAlertObservation(payload))
	assert.ErrorIs(t, model.ReplayAlertSpool(ctx), model.ErrInvalidAlertObservation)
	require.NoError(t, model.ReplayAlertSpool(ctx))
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("id = ?", spooled.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "poison input must not block later valid files")
	assert.False(t, model.AlertLocalUsageReady(), "quarantined evidence keeps coverage incomplete until repaired")
}

func testAlertLifecycle(t *testing.T) {
	ctx := context.Background()
	start := time.Now().Add(-time.Minute).UnixMilli()
	duration := int64(60000)
	o := model.AlertObservation{ID: model.AlertIdentity("terminal"), Namespace: "health", SourceID: model.AlertIdentity("lifecycle"), Kind: "finished", Layer: "request", Sequence: 3, OccurredAt: start + duration, StartedAt: start, Outcome: "success", SampleClass: "production", DurationMS: &duration}
	for range 2 {
		require.NoError(t, model.AcceptAlertObservation(ctx, o))
	}
	var row model.AlertLifecycle
	require.NoError(t, model.DB.First(&row, "id = ?", o.SourceID).Error)
	assert.Equal(t, o.OccurredAt, row.FinishedAt)
	late := o
	late.ID = model.AlertIdentity("late-start")
	late.Kind = "started"
	late.Sequence = 1
	late.OccurredAt = start
	late.DurationMS = nil
	require.NoError(t, model.AcceptAlertObservation(ctx, late))
	require.NoError(t, model.DB.First(&row, "id = ?", o.SourceID).Error)
	assert.NotZero(t, row.FinishedAt)
	var count int64
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("source_id = ? AND kind = ?", o.SourceID, "finished").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	conflict := o
	conflict.Outcome = "failure"
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, conflict), model.ErrAlertObservationConflict)
	otherTerminal := o
	otherTerminal.ID = model.AlertIdentity("another-finished")
	otherTerminal.Sequence++
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, otherTerminal), model.ErrAlertObservationConflict)
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("source_id = ? AND kind = ?", o.SourceID, "finished").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func testAlertChannelIsolation(t *testing.T) {
	ctx := context.Background()
	previous := common.AlertingControlsEnabled
	common.AlertingControlsEnabled = true
	t.Cleanup(func() { common.AlertingControlsEnabled = previous })
	autoBan := 1
	channels := []model.Channel{{Id: 41, Name: "Primary channel", Type: 1, Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", AutoBan: &autoBan}, {Id: 42, Name: "Backup channel", Type: 1, Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", AutoBan: &autoBan}}
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(&channel).Error)
		for _, name := range []string{"model-a", "model-b"} {
			require.NoError(t, model.DB.Create(&model.Ability{ChannelId: channel.Id, Group: "default", Model: name, Enabled: true}).Error)
		}
		require.NoError(t, model.SetAlertChannelPolicy(ctx, channel.Id, 3, "scoped", 0))
	}
	row := model.AlertRule{OwnerUserID: 0, CreatedBy: 3, UpdatedBy: 3, Name: "Local protection", Enabled: true, Revision: 1, Generation: 1, EffectiveAt: time.Now().Add(-time.Hour).UnixMilli()}
	spec := model.AlertRuleSpec{ManagementMode: "platform_enforced", Actions: []model.AlertAction{{Type: "disable_channel_model", Target: "matched_entity"}}, ActionGuard: &model.AlertActionGuard{MinRemainingChannels: 1, OnNoAlternative: "notify_only"}, Recovery: model.AlertRecovery{Mode: "manual"}}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	require.NoError(t, model.DB.Create(&row).Error)
	_, _, version := model.ResolveChannelModelIdentity(&channels[0], "model-a")
	target := alerting.MonitoredTarget{Type: "channel_model", ID: 41, Model: "model-a", Name: "Primary channel / model-a", ConfigVersion: version}
	engine := alerting.Engine{BootID: "action-test"}
	job := model.AlertJob{Action: "disable_channel_model", DedupKey: "action-a"}
	event := model.AlertEvent{ID: 1001}
	for range 2 {
		require.NoError(t, engine.ExecuteAction(ctx, job, row, spec, target, event))
	}
	assert.Error(t, model.CheckChannelModelAvailability(ctx, 41, "model-a"))
	available, filterErr := model.FilterAlertAvailableChannels(ctx, []int{41, 42}, "model-a")
	require.NoError(t, filterErr)
	assert.Equal(t, []int{42}, available)
	require.NoError(t, model.CheckChannelModelAvailability(ctx, 41, "model-b"))
	require.NoError(t, model.CheckChannelModelAvailability(ctx, 42, "model-a"))
	var primary model.Channel
	require.NoError(t, model.DB.First(&primary, 41).Error)
	assert.Equal(t, common.ChannelStatusEnabled, primary.Status)
	assert.False(t, model.DisableLegacyAlertChannel(ctx, 41, "", "legacy error"), "a callback cannot widen scoped protection")
	require.NoError(t, model.CheckChannelModelAvailability(ctx, 41, "model-b"))
	backup := target
	backup.ID = 42
	backup.Name = "Backup channel / model-a"
	_, _, backup.ConfigVersion = model.ResolveChannelModelIdentity(&channels[1], "model-a")
	err = engine.ExecuteAction(ctx, model.AlertJob{Action: "disable_channel_model", DedupKey: "action-b"}, row, spec, backup, model.AlertEvent{ID: 1002})
	var blocked *alerting.BlockedAction
	require.ErrorAs(t, err, &blocked)
	assert.Equal(t, "no_alternative", blocked.Code)
	var holds []model.AlertDisableHold
	require.NoError(t, model.DB.Where("channel_id = ? AND active = ?", 41, true).Find(&holds).Error)
	require.Len(t, holds, 1)
	// Rebuilding declared abilities cannot delete the independent hold.
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 41).Update("enabled", true).Error)
	assert.Error(t, model.CheckChannelModelAvailability(ctx, 41, "model-a"))
	_, err = model.ReleaseAlertHold(ctx, holds[0].ID, 1, false)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = model.ReleaseAlertHold(ctx, holds[0].ID, 3, true)
	require.NoError(t, err)
	require.NoError(t, model.CheckChannelModelAvailability(ctx, 41, "model-a"))
	// Keep this fixture out of subsequent tests' target namespaces.
	require.NoError(t, model.DB.Delete(&model.Ability{}, "channel_id IN ?", []int{41, 42}).Error)
	require.NoError(t, model.DB.Delete(&model.Channel{}, "id IN ?", []int{41, 42}).Error)
}

func testAlertTokenHolds(t *testing.T) {
	ctx := context.Background()
	previous := common.AlertingControlsEnabled
	common.AlertingControlsEnabled = true
	t.Cleanup(func() { common.AlertingControlsEnabled = previous })
	rule := model.AlertRule{Name: "Token protection", OwnerUserID: 1, CreatedBy: 1, UpdatedBy: 1, Enabled: true, Revision: 1, Generation: 1}
	require.NoError(t, model.DB.Create(&rule).Error)
	holds := []model.AlertDisableHold{{DedupKey: model.AlertIdentity("token-hold-a"), TargetKey: model.AlertIdentity("api_key", 11), TargetType: "api_key", TokenID: 11, OwnerUserID: 1, RuleID: rule.ID}, {DedupKey: model.AlertIdentity("token-hold-b"), TargetKey: model.AlertIdentity("api_key", 11), TargetType: "api_key", TokenID: 11, OwnerUserID: 1, RuleID: rule.ID}}
	for i := range holds {
		require.NoError(t, model.CommitAlertHold(ctx, &holds[i], rule, nil))
	}
	_, err := model.ValidateUserToken("alert-test-token")
	assert.ErrorIs(t, err, model.ErrTokenInvalid)
	_, err = model.ReleaseAlertHold(ctx, holds[0].ID, 1, false)
	require.NoError(t, err)
	held, err := model.AlertTokenHeld(ctx, 11)
	require.NoError(t, err)
	assert.True(t, held)
	// A user edits the disabled token before the final source is released.
	token, err := model.GetTokenById(11)
	require.NoError(t, err)
	version := token.AuthVersion
	require.NoError(t, token.SelectUpdate())
	_, err = model.ReleaseAlertHold(ctx, holds[1].ID, 1, false)
	require.NoError(t, err)
	token, err = model.GetTokenById(11)
	require.NoError(t, err)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	assert.Greater(t, token.AuthVersion, version)
}

func testAlertMilestones(t *testing.T) {
	ctx := context.Background()
	row, spec, target := alertRuleFixture(t, "spend_net")
	spec.Condition.ThresholdUSD = "100"
	spec.Condition.RecoveryThresholdUSD = "0"
	spec.Condition.Window.Type = "cumulative"
	spec.Trigger.Mode = "milestone"
	spec.Trigger.StepUSD = "100"
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	require.NoError(t, model.DB.Model(&row).Update("config", row.Config).Error)
	engine := alerting.Engine{BootID: "milestone-worker"}
	now := time.Now()
	for i, usd := range []string{"350", "400", "250", "400", "500"} {
		quota, err := alerting.USDQuota(usd)
		require.NoError(t, err)
		m := alerting.Measurement{Status: "complete", Samples: 1, Value: quota}
		require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now.Add(time.Duration(i)*time.Second)))
	}
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "milestone").Find(&events).Error)
	assert.Len(t, events, 3, "refunds must not replay already published tiers")
	for i, quota := range []int64{175000000, -50000000, 25000000} {
		o := model.AlertObservation{ID: model.AlertIdentity("usage-fact", i), Namespace: "usage", SourceID: model.AlertIdentity("usage-source", i), Kind: "settlement", OccurredAt: now.Add(-time.Minute).UnixMilli(), UserID: 1, TokenID: 11, Quota: quota}
		for range 2 {
			require.NoError(t, model.AcceptAlertObservation(ctx, o))
		}
	}
	m, err := alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 150000000, m.Value)
}

func TestAlertLifecycleWithoutDebugTiming(t *testing.T) {
	oldEnabled, oldTiming := common.AlertingEnabled, common.RequestTimingEnabled
	common.AlertingEnabled = true
	common.RequestTimingEnabled = false
	t.Cleanup(func() {
		common.AlertingEnabled, common.RequestTimingEnabled = oldEnabled, oldTiming
		common.SetAlertHealthSink(nil)
	})
	var events []common.AlertHealthEvent
	common.SetAlertHealthSink(func(_ context.Context, event common.AlertHealthEvent) error {
		events = append(events, event)
		return nil
	})
	ctx, span := common.BeginAlertSpan(context.Background(), common.AlertHealthEvent{Layer: "channel_attempt", Model: "model-a", OutputCovered: true})
	common.ObserveTimingProtocol(ctx, "first_text_at_ms", true, false)
	common.ObserveTimingProtocol(ctx, "first_text_at_ms", true, false)
	cutoff, covered := common.AlertPublicationWatermark(time.Now().Add(10 * time.Second))
	assert.True(t, covered)
	assert.Less(t, cutoff, span.Snapshot().LastOutputAt, "buffered progress limits coverage even while its timer has not run")
	span.End(ctx, "success")
	span.End(ctx, "failure")
	require.Len(t, events, 3)
	assert.Equal(t, "started", events[0].Kind)
	assert.Equal(t, "first_output", events[1].Kind)
	assert.Equal(t, "finished", events[2].Kind)
	assert.NotNil(t, events[2].TTFTMS)
	future := time.Now().Add(10 * time.Second)
	cutoff, covered = common.AlertPublicationWatermark(future)
	assert.True(t, covered)
	assert.Equal(t, future.Add(-time.Second).UnixMilli(), cutoff)
}

func alertRuleFixture(t *testing.T, metric string) (model.AlertRule, model.AlertRuleSpec, alerting.MonitoredTarget) {
	t.Helper()
	now := time.Now()
	target := model.AlertTarget{OwnerUserID: 1, Name: "Operations", Type: "email", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&target).Error)
	spec := model.AlertRuleSpec{Name: "Business latency", Category: "health", Severity: "P1", Enabled: true, ManagementMode: "user_managed", Scope: model.AlertScope{Type: "api_key", Selector: "ids", EntityIDs: []int{11}, GroupMode: "per_entity"}, Condition: model.AlertCondition{Metric: metric, Aggregation: "p95", LatencyBasis: "gateway_request", Operator: "gt", ThresholdMS: 30000, RecoveryThresholdMS: 10000, SlowThresholdMS: 30000, ThresholdBPS: 500, RecoveryThresholdBPS: 200, MinSamples: 2, Window: model.AlertWindow{Type: "rolling", DurationSeconds: 300, Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "incident", RepeatSeconds: 1800}, Notify: model.AlertNotify{TargetIDs: []int64{target.ID}, OnRecovery: true}, Recovery: model.AlertRecovery{Mode: "manual"}}
	if metric == "spend_net" {
		spec.Condition.ThresholdUSD = "100"
		spec.Condition.RecoveryThresholdUSD = "0"
	}
	row, err := alerting.SaveRule(context.Background(), alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, 0, 0, spec)
	require.NoError(t, err)
	row.EffectiveAt = now.Add(-10 * time.Minute).UnixMilli()
	require.NoError(t, model.DB.Model(row).Update("effective_at", row.EffectiveAt).Error)
	spec, err = model.AlertRuleConfig(*row)
	require.NoError(t, err)
	return *row, spec, alerting.MonitoredTarget{Type: "api_key", ID: 11, Name: "business"}
}

func testAlertIncident(t *testing.T) {
	ctx := context.Background()
	row, spec, target := alertRuleFixture(t, "duration_ms")
	now := time.Now()
	for i, duration := range []int64{3000, 4000, 90000, 100000} {
		o := model.AlertObservation{ID: model.AlertIdentity("latency", i), Namespace: "health", SourceID: model.AlertIdentity("latency-life", i), Kind: "finished", Layer: "request", Sequence: 1, OccurredAt: now.Add(-time.Minute).UnixMilli() + int64(i), StartedAt: now.Add(-3 * time.Minute).UnixMilli(), UserID: 1, TokenID: 11, RequestID: fmt.Sprintf("request-%d", i), SampleClass: "production", Outcome: "success", DurationMS: &duration, AttributionCovered: true}
		require.NoError(t, model.AcceptAlertObservation(ctx, o))
	}
	m, err := alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.Equal(t, "complete", m.Status)
	assert.EqualValues(t, 100000, m.Value)
	engine := alerting.Engine{BootID: "test-worker"}
	for range 2 {
		require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now))
	}
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, "business", events[0].TargetName)
	var jobs []model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	engine.Deliver = func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) {
		return 0, errors.New("https://secret.example/hook?token=usable-secret")
	}
	require.NoError(t, engine.RunJobs(ctx, "notify", now))
	require.NoError(t, model.DB.First(&jobs[0], jobs[0].ID).Error)
	assert.Equal(t, "retry_wait", jobs[0].Status)
	assert.NotContains(t, jobs[0].LastError, "usable-secret")
	m.Value = 3000
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now.Add(time.Second)))
	var incident model.AlertIncident
	require.NoError(t, model.DB.First(&incident, events[0].IncidentID).Error)
	assert.Equal(t, "resolved", incident.Status)
	changed := spec
	changed.Scope.EntityIDs = []int{9999}
	_, err = alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: 1}, 1, 0, 0, changed)
	assert.Error(t, err)
	_, err = alerting.SaveRule(ctx, alerting.Actor{ID: 2, Role: 1}, 1, row.ID, row.Revision, spec)
	assert.ErrorIs(t, err, alerting.ErrForbidden)
	_, err = alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: 1}, 1, row.ID, row.Revision+1, spec)
	assert.ErrorIs(t, err, model.ErrAlertConflict)
}

func testAlertInflight(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	rule := model.AlertRule{EffectiveAt: now.Add(-10 * time.Minute).UnixMilli()}
	spec := model.AlertRuleSpec{Scope: model.AlertScope{Type: "channel_model"}, Condition: model.AlertCondition{Metric: "inflight_overdue_count", LatencyBasis: "channel_attempt", DeadlineKind: "first_output", DeadlineMS: 60000, ThresholdCount: 1}}
	target := alerting.MonitoredTarget{Type: "channel_model", ID: 41, Model: "model-a", Name: "Sample channel / model-a"}
	checkpoint := model.AlertWorkerCheckpoint{ID: "inflight-boot", BootID: "inflight-boot", Kind: "producer", UsageHealthy: true, Healthy: true, UpdatedAt: now.UnixMilli(), ProtocolVersion: model.AlertSourceProtocolVersion, CapturedAt: now.Add(-time.Second).UnixMilli(), CoverageStartedAt: now.Add(-72 * time.Hour).UnixMilli()}
	require.NoError(t, model.DB.Create(&checkpoint).Error)
	row := model.AlertObservation{ID: model.AlertIdentity("inflight-start"), Namespace: "health", SourceID: model.AlertIdentity("inflight-source"), Kind: "started", Layer: "channel_attempt", Sequence: 1, ChannelID: 41, ModelKey: "model-a", RequestID: "still-running", BootID: checkpoint.ID, OccurredAt: now.Add(-90 * time.Second).UnixMilli(), StartedAt: now.Add(-90 * time.Second).UnixMilli(), OutputCovered: true}
	require.NoError(t, model.AcceptAlertObservation(ctx, row))
	m, err := alerting.Measure(ctx, rule, spec, target, now)
	require.NoError(t, err)
	assert.Equal(t, "complete", m.Status)
	assert.EqualValues(t, 1, m.Value)
	require.NoError(t, model.DB.Model(&checkpoint).Update("healthy", false).Error)
	m, err = alerting.Measure(ctx, rule, spec, target, now)
	require.NoError(t, err)
	assert.Equal(t, "stale", m.Status)
	require.NoError(t, model.DB.Model(&checkpoint).Update("healthy", true).Error)
	progress := row
	progress.ID = model.AlertIdentity("inflight-output")
	progress.Kind = "first_output"
	progress.Sequence = 2
	progress.FirstOutputAt = now.Add(-time.Second).UnixMilli()
	progress.LastOutputAt = progress.FirstOutputAt
	progress.OccurredAt = progress.FirstOutputAt
	require.NoError(t, model.AcceptAlertObservation(ctx, progress))
	m, err = alerting.Measure(ctx, rule, spec, target, now)
	require.NoError(t, err)
	assert.Zero(t, m.Value)
	spec.Condition = model.AlertCondition{Metric: "duration_ms", Aggregation: "p95", LatencyBasis: "channel_attempt", ThresholdMS: 30000, SlowThresholdMS: 30000, MinSamples: 2, Window: model.AlertWindow{Type: "rolling", DurationSeconds: 300, Timezone: "Asia/Shanghai"}}
	for i, duration := range []int64{60000, 100} {
		finished := row
		finished.ID, finished.SourceID = model.AlertIdentity("evidence-finished", i), model.AlertIdentity("evidence-life", i)
		finished.Kind, finished.Outcome, finished.RequestID = "finished", "success", fmt.Sprintf("finished-evidence-%d", i)
		finished.OccurredAt = now.Add(-5 * time.Second).UnixMilli()
		finished.StartedAt, finished.DurationMS = finished.OccurredAt-duration, &duration
		require.NoError(t, model.AcceptAlertObservation(ctx, finished))
	}
	m, err = alerting.Measure(ctx, rule, spec, target, now)
	require.NoError(t, err)
	require.Equal(t, "complete", m.Status, m.Reason)
	assert.EqualValues(t, 60000, m.Value)
	kinds := map[string]bool{}
	for _, request := range m.Requests {
		kinds[request.Kind] = true
		if request.Kind == "inflight" {
			assert.Zero(t, request.FinishedAt, "diagnostics cannot invent a terminal outcome")
			assert.Equal(t, "still-running", request.RequestID)
		}
	}
	assert.True(t, kinds["slow"])
	assert.True(t, kinds["normal"])
	assert.True(t, kinds["inflight"])
}

func testAlertHTTPIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	row, _, _ := alertRuleFixture(t, "duration_ms")
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 2); c.Set("role", common.RoleCommonUser); c.Next() })
	router.GET("/rules/:id", controller.GetAlertRule)
	router.GET("/rules", controller.ListAlertRules)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/rules/%d", row.ID), nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?token_id=11", nil))
	assert.Equal(t, http.StatusNotFound, response.Code, "an object link cannot reveal another user's Key or rules")
	require.NoError(t, model.DB.Create(&model.Token{Id: 24, UserId: 2, Name: "Own linked Key", Key: "own-linked-key"}).Error)
	spec, err := model.AlertRuleConfig(row)
	require.NoError(t, err)
	spec.Scope.EntityIDs = []int{24}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	own := model.AlertRule{OwnerUserID: 2, Name: "Own related rule", Revision: 1, Generation: 1, Config: string(encoded)}
	require.NoError(t, model.DB.Create(&own).Error)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?token_id=24", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var related struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &related))
	require.Len(t, related.Data, 1)
	assert.Equal(t, own.ID, related.Data[0].ID)
}

func TestAlertTargetCredentialBinding(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	vault, err := alerting.NewTargetVault("key-1", map[string]string{"key-1": key})
	require.NoError(t, err)
	secret := alerting.TargetSecret{Address: "https://example.com/private-hook?key=secret", Secret: "signing-secret"}
	sealed, err := vault.Seal(1, "feishu", secret)
	require.NoError(t, err)
	assert.NotContains(t, sealed, "secret")
	target := model.AlertTarget{OwnerUserID: 1, Type: "feishu", KeyID: "key-1", Ciphertext: sealed}
	actual, err := vault.Open(target)
	require.NoError(t, err)
	assert.Equal(t, secret, actual)
	target.OwnerUserID = 2
	_, err = vault.Open(target)
	assert.ErrorIs(t, err, alerting.ErrTargetKeyUnavailable)
	for _, address := range []string{"http://example.com", "https://127.0.0.1/hook", "https://169.254.169.254/", "https://[::1]/", "https://user:secret@example.com", "https://example.com:8443/"} {
		_, err = alerting.ValidateTargetAddress(context.Background(), "feishu", alerting.TargetSecret{Address: address})
		assert.ErrorIs(t, err, alerting.ErrTargetRejected, address)
	}
}

func TestAlertTargetValidationResponse(t *testing.T) {
	vault, err := alerting.NewTargetVault("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("v", 32)))})
	require.NoError(t, err)
	previous := service.AlertTargetVault
	service.AlertTargetVault = vault
	t.Cleanup(func() { service.AlertTargetVault = previous })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleCommonUser); c.Next() })
	router.POST("/targets", controller.SaveAlertTarget)
	for _, tc := range []struct{ name, kind, address, message, field string }{
		{"http URL", "feishu", "http://example.com/hook?secret=do-not-echo", "Notification URL must use HTTPS on port 443 without a username, password or fragment.", "credentials.address"},
		{"custom port", "feishu", "https://example.com:8443/hook?secret=do-not-echo", "Notification URL must use HTTPS on port 443 without a username, password or fragment.", "credentials.address"},
		{"invalid email", "email", "invalid-email-do-not-echo", "Enter a valid notification email address.", "credentials.address"},
		{"blocked destination", "feishu", "https://127.0.0.1/hook?secret=do-not-echo", "Notification destination is blocked by network access rules.", "credentials.address"},
		{"missing Gotify token", "gotify", "https://1.1.1.1/notify", "Gotify requires an application token.", "credentials.token"},
		{"unsupported type", "unknown", "https://1.1.1.1/notify", "Unsupported notification target type.", "type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := common.Marshal(alerting.TargetInput{Name: "Operations", Type: tc.kind, Enabled: true, Language: "en", Credentials: &alerting.TargetSecret{Address: tc.address, Secret: "signing-secret-do-not-echo"}})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code)
			var result struct {
				Success    bool   `json:"success"`
				Message    string `json:"message"`
				MessageKey string `json:"message_key"`
				Field      string `json:"field"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
			assert.Equal(t, tc.message, result.Message)
			assert.Equal(t, tc.message, result.MessageKey)
			assert.Equal(t, tc.field, result.Field)
			assert.NotContains(t, response.Body.String(), "do-not-echo")
		})
	}
}

func testAlertRecovery(t *testing.T) {
	ctx := context.Background()
	oldActions, oldShadow, oldControls := common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled
	common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = true, false, true
	t.Cleanup(func() {
		common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = oldActions, oldShadow, oldControls
	})
	now := time.Now()
	ban := 1
	channel := model.Channel{Id: 61, Name: "Recovery channel", Type: 1, Models: "model-a,model-b", Group: "default", Status: common.ChannelStatusEnabled, AutoBan: &ban}
	require.NoError(t, model.DB.Create(&channel).Error)
	_, _, version := model.ResolveChannelModelIdentity(&channel, "model-a")
	spec := model.AlertRuleSpec{Name: "Recovery latency", ManagementMode: "platform_enforced", Scope: model.AlertScope{Type: "channel_model", ChannelIDs: []int{61}}, Condition: model.AlertCondition{Metric: "duration_ms", LatencyBasis: "channel_attempt"}, Recovery: model.AlertRecovery{Mode: "probe", Protocol: "openai", DurationLimitMS: 15000, TTFTLimitMS: 5000, Successes: 3, ObserveSeconds: 120}}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row := model.AlertRule{OwnerUserID: 0, CreatedBy: 3, UpdatedBy: 3, Name: spec.Name, Enabled: true, Revision: 1, Generation: 1, EffectiveAt: now.Add(-time.Hour).UnixMilli(), Config: string(encoded)}
	require.NoError(t, model.DB.Create(&row).Error)
	recovery, err := common.Marshal(spec.Recovery)
	require.NoError(t, err)
	incident := model.AlertIncident{RuleID: row.ID, Status: "firing", OpenedAt: now.UnixMilli()}
	require.NoError(t, model.DB.Create(&incident).Error)
	hold := model.AlertDisableHold{DedupKey: model.AlertIdentity("recovery-test"), TargetKey: model.AlertIdentity("channel_model", 61, "routing", "model-a"), TargetType: "channel_model", ChannelID: 61, ModelKey: "model-a", RuleID: row.ID, IncidentID: incident.ID, Platform: true, ConfigVersion: version, Recovery: string(recovery), NextProbeAt: now.UnixMilli()}
	require.NoError(t, model.CommitAlertHold(ctx, &hold, row, nil))
	parent := model.AlertDisableHold{DedupKey: model.AlertIdentity("recovery-parent"), TargetKey: model.AlertIdentity("channel", 61), TargetType: "channel", ChannelID: 61, RuleID: row.ID, Platform: true, ConfigVersion: version}
	require.NoError(t, model.CommitAlertHold(ctx, &parent, row, nil))
	duration, ttft := int64(1000), int64(100)
	probeVersion := version
	calls := 0
	rotateDuringProbe := false
	engine := alerting.Engine{BootID: "recovery-test", Probe: func(_ context.Context, h model.AlertDisableHold) alerting.ProbeResult {
		calls++
		assert.Equal(t, "model-a", h.ModelKey)
		if rotateDuringProbe {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", h.ChannelID).Update("key", "second-rotated-test-key").Error)
		}
		return alerting.ProbeResult{Success: true, DurationMS: &duration, TTFTMS: &ttft, ConfigVersion: probeVersion}
	}}
	reload := func() { require.NoError(t, model.DB.First(&hold, hold.ID).Error) }
	tick := func(offset int) {
		at := now.Add(time.Duration(offset) * time.Second)
		require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("last_seen_at", at.Unix()).Error)
		require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"healthy": true, "updated_at": at.UnixMilli(), "captured_at": at.Add(-time.Second).UnixMilli(), "offered_sequence": 0, "accepted_count": 0}).Error)
		require.NoError(t, engine.RecoverHolds(ctx, at))
		reload()
	}
	tick(0)
	assert.Equal(t, 1, hold.ProbeCount)
	assert.True(t, hold.Active)
	duration = 16000
	tick(60)
	assert.Zero(t, hold.ProbeCount, "HTTP success above the latency limit resets recovery")
	duration = 1000
	probeVersion = "obsolete"
	tick(120)
	assert.Zero(t, hold.ProbeCount, "old configuration cannot qualify")
	probeVersion = version
	tick(180)
	tick(240)
	assert.True(t, hold.Active, "two successes cannot release")
	// Three good probes still cannot release while production input is incomplete.
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Update("healthy", false).Error)
	at := now.Add(300 * time.Second)
	require.NoError(t, engine.RecoverHolds(ctx, at))
	reload()
	assert.True(t, hold.Active)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", hold.ChannelID).Update("key", "first-rotated-test-key").Error)
	tick(360)
	assert.True(t, hold.Active, "credentials changed after earlier successful probes")
	assert.Equal(t, 1, hold.ProbeCount)
	rotateDuringProbe = true
	tick(420)
	assert.True(t, hold.Active)
	assert.Zero(t, hold.ProbeCount, "a key change during a probe invalidates its result")
	rotateDuringProbe = false
	tick(480)
	tick(540)
	assert.True(t, hold.Active)
	tick(600)
	assert.False(t, hold.Active)
	assert.GreaterOrEqual(t, calls, 6)
	var liveParent model.AlertDisableHold
	require.NoError(t, model.DB.First(&liveParent, parent.ID).Error)
	assert.True(t, liveParent.Active, "recovery only releases its own source")
	assert.Error(t, model.CheckChannelModelAvailability(ctx, 61, "model-a"), "parent hold remains effective")
	require.NoError(t, model.DB.First(&incident, incident.ID).Error)
	assert.Equal(t, "resolved", incident.Status)
	// Restore the source clock for other matrix subtests.
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("last_seen_at", now.Unix()).Error)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
}

func testAlertReports(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	local := now.In(loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	now = end.Add(40 * time.Minute)
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("started_at", end.Add(-48*time.Hour).Unix()).Error)
	start := end.AddDate(0, 0, -1)
	spec := model.AlertRuleSpec{Name: "Account daily", Scope: model.AlertScope{Type: "account", GroupMode: "aggregate"}, Condition: model.AlertCondition{Metric: "spend_net", Operator: "gt", ThresholdUSD: "0", Window: model.AlertWindow{Type: "day", Timezone: "Asia/Shanghai", SendAt: "00:10"}}, Trigger: model.AlertTrigger{Mode: "schedule_report", Report: model.AlertReportOptions{BackfillPeriods: 7, PartialAfterSeconds: 1800, SendRevision: true}}}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	rule := model.AlertRule{OwnerUserID: 2, Name: spec.Name, Enabled: true, Revision: 1, Generation: 1, EffectiveAt: start.Add(-time.Hour).UnixMilli(), Config: string(encoded)}
	require.NoError(t, model.DB.Create(&rule).Error)
	for i, fact := range []model.AlertObservation{{Quota: 500, RequestCount: 1, InputTokens: 10, OutputTokens: 20, TokenID: 0}, {Quota: -100, RequestCount: 0, InputTokens: 0, OutputTokens: 0, TokenID: 0}} {
		fact.ID = model.AlertIdentity("report-test", i)
		fact.SourceID = fact.ID
		fact.Namespace = "usage"
		fact.Kind = "usage"
		fact.UserID = 2
		fact.OccurredAt = start.Add(time.Hour).UnixMilli()
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("last_seen_at", now.Unix()).Error)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli(), "healthy": false}).Error)
	engine := alerting.Engine{BootID: "reports-test"}
	target := alerting.MonitoredTarget{Type: "account", ID: 2, Name: "Other account"}
	for range 2 {
		require.NoError(t, engine.EvaluateReports(ctx, rule, spec, target, now))
	}
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ?", rule.ID).Order("id asc").Find(&events).Error)
	require.Len(t, events, 2, "one immutable partial report and one data-gap event")
	var evidence struct {
		Measurement alerting.Measurement `json:"measurement"`
	}
	require.NoError(t, common.UnmarshalJsonStr(events[0].Evidence, &evidence))
	require.NotNil(t, evidence.Measurement.Report)
	assert.EqualValues(t, 400, evidence.Measurement.Report.Totals.Quota)
	assert.EqualValues(t, 1, evidence.Measurement.Report.Totals.Requests)
	assert.EqualValues(t, 20, evidence.Measurement.Report.Totals.OutputTokens)
	assert.Equal(t, "partial", evidence.Measurement.Status)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli(), "healthy": true, "accepted_count": 0, "offered_sequence": 0}).Error)
	for range 2 {
		require.NoError(t, engine.EvaluateReports(ctx, rule, spec, target, now))
	}
	require.NoError(t, model.DB.Where("rule_id = ?", rule.ID).Find(&events).Error)
	require.Len(t, events, 3, "exactly one complete revision")
	assert.Equal(t, "report_revision", events[2].Kind)
	for i, name := range []string{"=SUM(1,1)", "Second business key"} {
		tokenID := 5011 + i
		require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: 2, Key: fmt.Sprintf("report-page-%d", tokenID), Name: name}).Error)
		fact := model.AlertObservation{ID: model.AlertIdentity("report-page", tokenID), Namespace: "usage", SourceID: model.AlertIdentity("report-page-source", tokenID), Kind: "usage", UserID: 2, TokenID: tokenID, Quota: int64(10 * (i + 1)), RequestCount: 1, OccurredAt: start.Add(time.Hour).UnixMilli()}
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	first, err := alerting.MeasureReport(ctx, rule, spec, target, start.UnixMilli(), end.UnixMilli(), now, alerting.ReportKeyPage{AfterTokenID: -1, Limit: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 430, first.Report.Totals.Quota)
	assert.True(t, first.Report.HasMoreKeys)
	assert.Zero(t, first.Report.NextTokenID)
	second, err := alerting.MeasureReport(ctx, rule, spec, target, start.UnixMilli(), end.UnixMilli(), now, alerting.ReportKeyPage{AfterTokenID: first.Report.NextTokenID, Limit: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 430, second.Report.Totals.Quota, "cursor only narrows key details, never totals")
	assert.Equal(t, "=SUM(1,1)", second.Report.Keys[0].TokenName)
	api := gin.New()
	actorID := 2
	api.Use(func(c *gin.Context) { c.Set("id", actorID); c.Set("role", common.RoleCommonUser); c.Next() })
	api.GET("/rules/:id/report", controller.QueryAlertReport)
	path := fmt.Sprintf("/rules/%d/report?start=%d&end=%d&format=csv", rule.ID, start.UnixMilli(), end.UnixMilli())
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "'=SUM(1,1)", "CSV names cannot execute spreadsheet formulas")
	actorID = 1
	response = httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ?", rule.ID).Count(&eventCount).Error)
	assert.EqualValues(t, 3, eventCount, "historical query/export never creates delivery events")
	current := time.Now()
	require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("last_seen_at", current.Unix()).Error)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": current.UnixMilli(), "captured_at": current.Add(-time.Second).UnixMilli()}).Error)
}

func testAlertBatchMigration(t *testing.T) {
	ctx := context.Background()
	vault, err := alerting.NewTargetVault("batch-test", map[string]string{"batch-test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))})
	require.NoError(t, err)
	previous := service.AlertTargetVault
	service.AlertTargetVault = vault
	t.Cleanup(func() { service.AlertTargetVault = previous })
	for _, user := range []model.User{
		{Id: 91, Username: "batch-owner", AffCode: "batch-owner", Setting: `{"notify_type":"email","webhook_url":"https://1.1.1.1/batch-public-secret"}`, Email: "ops@example.com", Quota: 9999, UsedQuota: 123},
		{Id: 92, Username: "batch-private", AffCode: "batch-private", Setting: `{"notify_type":"webhook","webhook_url":"https://127.0.0.1/batch-private-secret"}`},
		{Id: 93, Username: "batch-empty", AffCode: "batch-empty"},
	} {
		require.NoError(t, model.DB.Create(&user).Error)
	}
	for _, token := range []model.Token{
		{Id: 91, UserId: 91, Name: "Batch business key", Key: "batch-public-token", Status: common.TokenStatusEnabled, AlertThreshold: 100, AlertNotifiedQuota: 2500, AlertLastNotifiedTime: 12345, UsedQuota: 7000, RemainQuota: 10000},
		{Id: 92, UserId: 92, Name: "Batch blocked key", Key: "batch-private-token", Status: common.TokenStatusEnabled, AlertThreshold: 200},
		{Id: 93, UserId: 93, Name: "Batch unconfigured key", Key: "batch-empty-token", Status: common.TokenStatusEnabled, AlertThreshold: 300},
	} {
		require.NoError(t, model.DB.Create(&token).Error)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 3)
		c.Set("role", common.RoleRootUser)
		if c.GetHeader("Test-Common-User") != "" {
			c.Set("role", common.RoleCommonUser)
		}
		c.Next()
	})
	router.GET("/migrations", controller.ListAlertMigrations)
	router.POST("/migrations/batch", controller.ImportAlertMigrationsBatch)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/migrations", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var candidates struct {
		Data struct {
			Tokens []struct {
				ID         int  `json:"id"`
				Configured bool `json:"configured"`
			} `json:"legacy_tokens"`
			Items []struct {
				Kind       string `json:"kind"`
				ObjectID   string `json:"object_id"`
				Configured bool   `json:"configured"`
			} `json:"legacy_quota_items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &candidates))
	configured := make(map[int]bool)
	for _, item := range candidates.Data.Tokens {
		configured[item.ID] = item.Configured
	}
	assert.True(t, configured[91])
	assert.True(t, configured[92], "configured is destination presence, not an SSRF bypass")
	assert.False(t, configured[93])
	for _, item := range candidates.Data.Items {
		if item.Kind == "user_notifications" && item.ObjectID == "93" {
			assert.False(t, item.Configured)
		}
		if item.Kind == "user_notifications" && item.ObjectID == "91" {
			assert.True(t, item.Configured)
		}
	}
	assert.NotContains(t, response.Body.String(), "batch-public-secret")
	assert.NotContains(t, response.Body.String(), "ops@example.com")
	payload := `{"items":[{"kind":"token_spend","object_id":"91"},{"kind":"token_spend","object_id":"92"},{"kind":"user_notifications","object_id":"91"}]}`
	// Denial runs before any item can be imported.
	request := httptest.NewRequest(http.MethodPost, "/migrations/batch", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Test-Common-User", "true")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
	var migrations int64
	require.NoError(t, model.DB.Model(&model.AlertMigration{}).Where("owner_user_id IN ?", []int{91, 92, 93}).Count(&migrations).Error)
	assert.Zero(t, migrations)
	for _, invalid := range []string{`{"items":[]}`, `{"items":[` + strings.TrimSuffix(strings.Repeat(`{"kind":"unknown","object_id":"site"},`, 11), ",") + `]}`} {
		request = httptest.NewRequest(http.MethodPost, "/migrations/batch", strings.NewReader(invalid))
		request.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code, "reject empty and oversized batches before imports")
	}
	service.AlertTargetVault = nil
	request = httptest.NewRequest(http.MethodPost, "/migrations/batch", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	service.AlertTargetVault = vault
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	var firstID int64
	for range 2 {
		request = httptest.NewRequest(http.MethodPost, "/migrations/batch", strings.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Items []struct {
					Success   bool                 `json:"success"`
					Migration model.AlertMigration `json:"migration"`
					Error     struct {
						MessageKey string `json:"message_key"`
						Field      string `json:"field"`
					} `json:"error"`
				} `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success)
		require.Len(t, result.Data.Items, 3)
		assert.True(t, result.Data.Items[0].Success)
		assert.False(t, result.Data.Items[1].Success)
		assert.Equal(t, "Notification destination is blocked by network access rules.", result.Data.Items[1].Error.MessageKey)
		assert.Equal(t, "credentials.address", result.Data.Items[1].Error.Field)
		assert.True(t, result.Data.Items[2].Success, "one failure does not prevent later imports")
		migration := result.Data.Items[0].Migration
		assert.Equal(t, "legacy", migration.Executor)
		assert.Equal(t, "ambiguous", migration.Status)
		if firstID == 0 {
			firstID = migration.RuleID
		}
		assert.Equal(t, firstID, migration.RuleID, "retry reuses the imported rule")
		var checkpoint alerting.LegacyTokenPreview
		require.NoError(t, common.UnmarshalJsonStr(migration.Checkpoint, &checkpoint))
		assert.EqualValues(t, 2500, checkpoint.NotifiedQuota)
		assert.EqualValues(t, 12345, checkpoint.LastNotifiedTime)
		assert.NotContains(t, response.Body.String(), "batch-private-secret")
		var targets, rules, jobs int64
		require.NoError(t, model.DB.Model(&model.AlertTarget{}).Where("owner_user_id = ?", 91).Count(&targets).Error)
		require.NoError(t, model.DB.Model(&model.AlertRule{}).Where("owner_user_id = ?", 91).Count(&rules).Error)
		require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id IN (?)", model.DB.Model(&model.AlertRule{}).Select("id").Where("owner_user_id = ?", 91)).Count(&jobs).Error)
		assert.EqualValues(t, 2, targets)
		assert.EqualValues(t, 2, rules)
		assert.Zero(t, jobs, "import does not send notifications or execute actions")
	}
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, 91).Error)
	require.NoError(t, model.DB.First(&token, 91).Error)
	assert.Equal(t, 9999, user.Quota)
	assert.Equal(t, 123, user.UsedQuota)
	assert.Equal(t, 7000, token.UsedQuota)
	assert.Equal(t, 10000, token.RemainQuota)
	assert.Equal(t, 2500, token.AlertNotifiedQuota)
	assert.EqualValues(t, 12345, token.AlertLastNotifiedTime)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	usesUnified, err := model.LegacyAlertUsesUnified(ctx, "token_spend", "91")
	require.NoError(t, err)
	assert.False(t, usesUnified)
}

func testAlertMigration(t *testing.T) {
	ctx := context.Background()
	oldEnabled, oldControls, oldShadow, oldExport := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = true, true, false, true
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = oldEnabled, oldControls, oldShadow, oldExport
	})
	now := time.Now()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("setting", `{"webhook_url":"https://1.1.1.1/test-secret","language":"en"}`).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: 21, UserId: 1, Name: "Migrated business key", Key: "migration-test-key", Status: common.TokenStatusEnabled, AlertThreshold: 100, AlertNotifiedQuota: 100000000, AlertLastNotifiedTime: now.Add(-time.Hour).Unix()}).Error)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 1, TokenId: 21, Quota: 125000000, CreatedAt: now.Add(-time.Hour).Unix()}).Error)
	fact := model.AlertObservation{ID: model.AlertIdentity("migration-existing"), SourceID: "migration-existing", Namespace: "usage", Kind: "settlement", UserID: 1, TokenID: 21, Quota: 15000000, OccurredAt: now.Add(-time.Hour).UnixMilli()}
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	vault, err := alerting.NewTargetVault("key", map[string]string{"key": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("m", 32)))})
	require.NoError(t, err)
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	migration, err := alerting.ImportLegacyToken(ctx, root, vault, 21)
	require.NoError(t, err)
	assert.Equal(t, "legacy", migration.Executor)
	assert.Equal(t, "ambiguous", migration.Status)
	var row model.AlertRule
	require.NoError(t, model.DB.First(&row, migration.RuleID).Error)
	spec, err := model.AlertRuleConfig(row)
	require.NoError(t, err)
	target := alerting.MonitoredTarget{Type: "api_key", ID: 21, Name: "Migrated business key"}
	engine := alerting.Engine{BootID: "migration-test"}
	// Shadow progress is separate from the authoritative legacy checkpoint.
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: 75000000}, now))
	var shadowJobs []model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&shadowJobs).Error)
	require.Len(t, shadowJobs, 1)
	assert.Equal(t, "shadow", shadowJobs[0].Status)
	snapshot, err := alerting.LegacyTokenSnapshot(ctx, 21)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"healthy": true, "updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli(), "accepted_count": 0, "offered_sequence": 0}).Error)
	assert.Error(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, false), "ambiguous old delivery needs explicit baseline acknowledgement")
	require.NoError(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, true))
	unified, err := model.LegacyAlertUsesUnified(ctx, "token_spend", "21")
	require.NoError(t, err)
	assert.True(t, unified)
	api := gin.New()
	api.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleCommonUser); c.Next() })
	api.GET("/tokens/:id", controller.GetToken)
	api.PUT("/tokens", controller.UpdateToken)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tokens/21", nil))
	var tokenView struct {
		Success bool `json:"success"`
		Data    struct {
			Managed bool `json:"alert_managed"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &tokenView))
	assert.True(t, tokenView.Data.Managed)
	for _, attempt := range []struct {
		body    string
		allowed bool
	}{{`{"id":21,"name":"Renamed migrated key","unlimited_quota":true,"alert_threshold":999}`, false}, {`{"id":21,"name":"Renamed migrated key","unlimited_quota":true}`, true}} {
		response = httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/tokens", strings.NewReader(attempt.body))
		request.Header.Set("Content-Type", "application/json")
		api.ServeHTTP(response, request)
		var result struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.Equal(t, attempt.allowed, result.Success, response.Body.String())
	}
	var unchanged model.Token
	require.NoError(t, model.DB.First(&unchanged, 21).Error)
	assert.Equal(t, float64(100), unchanged.AlertThreshold)
	require.NoError(t, model.DB.First(&row, row.ID).Error)
	now = time.Now().Add(2 * time.Second)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
	measured, err := alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 25000000, measured.Value, "carry the $50 unnotified remainder, not shadow progress")
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.QuotaData{UserID: 1, TokenId: 21, Quota: 25000000, CreatedAt: now.Unix()}).Error; err != nil {
			return err
		}
		next := fact
		next.ID = model.AlertIdentity("migration-new")
		next.SourceID = "migration-new"
		next.Quota = 25000000
		next.OccurredAt = now.UnixMilli()
		return model.AcceptAlertObservationTx(tx, next)
	}))
	measured, err = alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 50000000, measured.Value)
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, measured, now))
	snapshot, err = alerting.LegacyTokenSnapshot(ctx, 21)
	require.NoError(t, err)
	assert.Error(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true), "pending deliveries must drain before changing the executor")
	engine.Deliver = func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) { return 200, nil }
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id = ?", row.ID).Update("available_at", time.Now().Add(-time.Second).UnixMilli()).Error)
	require.NoError(t, engine.RunJobs(ctx, "notify", now))
	require.NoError(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true))
	unified, err = model.LegacyAlertUsesUnified(ctx, "token_spend", "21")
	require.NoError(t, err)
	assert.False(t, unified)
	var token model.Token
	require.NoError(t, model.DB.First(&token, 21).Error)
	assert.Equal(t, 150000000, token.AlertNotifiedQuota, "transferred checkpoint prevents duplicate historical notifications")
	assert.Equal(t, common.TokenStatusEnabled, token.Status, "migration never issues a key status or financial adjustment")
	// A settled refund below the transferred high-water mark must remain a
	// negative remainder, including through another cutover and rollback.
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Where("token_id = ? AND created_at = ?", 21, now.Add(-time.Hour).Unix()).Update("quota", 75000000).Error)
	refund := fact
	refund.ID = model.AlertIdentity("migration-refunded")
	refund.SourceID = "migration-refunded"
	refund.Quota = -50000000
	require.NoError(t, model.AcceptAlertObservation(ctx, refund))
	snapshot, err = alerting.LegacyTokenSnapshot(ctx, 21)
	require.NoError(t, err)
	require.NoError(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 3, "cutover", snapshot.SnapshotHash, true))
	require.NoError(t, alerting.SwitchLegacyToken(ctx, root, migration.ID, 4, "rollback", snapshot.SnapshotHash, true))
	require.NoError(t, model.DB.First(&token, 21).Error)
	assert.Equal(t, 150000000, token.AlertNotifiedQuota, "rollback must preserve the original high-water mark after a refund")

}

func testAlertUserSpendMigration(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	oldEnabled, oldControls, oldShadow, oldStats := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = true, true, false, true
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = oldEnabled, oldControls, oldShadow, oldStats
	})
	require.NoError(t, model.DB.Create(&model.User{Id: 8, Username: "account-spend-migration", AffCode: "account-spend", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 90000}).Error)
	state := model.LegacyUserQuotaMilestone{Initialized: true, BaselineAt: now.Add(-30 * 24 * time.Hour).Unix(), WindowStart: now.Unix() - now.Unix()%3600 - 3600, WindowQuota: 500000000, AccumulatedQuota: 1000000000, MilestoneUSD: 2000}
	states := map[string]model.LegacyUserQuotaMilestone{"8": state, "9": state}
	encoded, err := common.Marshal(states)
	require.NoError(t, err)
	for _, option := range []model.Option{{Key: "quota_alert_user_cumulative_milestone", Value: string(encoded)}, {Key: "uid_quota_warning_robot", Value: "https://8.8.8.8/account-bot"}} {
		require.NoError(t, model.DB.Create(&option).Error)
	}
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 8, CreatedAt: now.Unix() - now.Unix()%3600, Quota: 1250000000}).Error)
	fact := model.AlertObservation{ID: model.AlertIdentity("account-spend-initial"), Namespace: "usage", SourceID: "account-spend-initial", Kind: "usage", UserID: 8, Quota: 1250000000, RequestCount: 1, OccurredAt: now.Add(-time.Minute).UnixMilli()}
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": key})
	require.NoError(t, err)
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	_, err = alerting.ImportLegacyUserSpend(ctx, alerting.Actor{ID: 8, Role: common.RoleCommonUser}, vault, 8)
	assert.ErrorIs(t, err, alerting.ErrForbidden)
	migration, err := alerting.ImportLegacyUserSpend(ctx, root, vault, 8)
	require.NoError(t, err)
	again, err := alerting.ImportLegacyUserSpend(ctx, root, vault, 8)
	require.NoError(t, err)
	assert.Equal(t, migration.ID, again.ID)
	assert.Equal(t, "legacy", migration.Executor)
	assert.NotContains(t, migration.Checkpoint, "8.8.8.8")
	snapshot, err := alerting.LegacyUserSpendSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.EqualValues(t, 750000000, snapshot.UnnotifiedQuota, "$3500 accumulated minus the old $2000 tier retains $1500")
	assert.Error(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, false, 1))
	require.NoError(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, true, 1))
	markers, err := model.UnifiedLegacyQuotaAlerts(ctx, []string{"user_spend"})
	require.NoError(t, err)
	assert.True(t, markers[migration.ID], "the compatible old loop skips the switched account")
	var rule model.AlertRule
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	spec, err := model.AlertRuleConfig(rule)
	require.NoError(t, err)
	targets, err := alerting.ResolveTargets(ctx, rule, spec)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	// Advance the controlled producer snapshot past the cutover without waiting
	// for the real five-second heartbeat loop.
	now = time.Now().Add(2 * time.Second)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
	m, err := alerting.Measure(ctx, rule, spec, targets[0], now)
	require.NoError(t, err)
	require.Equal(t, "complete", m.Status, m.Reason)
	assert.EqualValues(t, 750000000, m.Value)
	engine := alerting.Engine{BootID: "account-cutover-worker", Deliver: func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) { return http.StatusOK, nil }}
	require.NoError(t, engine.ApplyMeasurement(ctx, rule, spec, targets[0], m, now))
	assert.Error(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true, rule.Revision), "accepted work must drain before authority returns to the old sender")
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id = ?", rule.ID).Update("available_at", time.Now().Add(-time.Second).UnixMilli()).Error)
	for range 8 {
		require.NoError(t, engine.RunJobs(ctx, "notify", now.Add(time.Second)))
	}
	require.NoError(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true, rule.Revision))
	updated, err := alerting.LegacyUserSpendSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.EqualValues(t, 3000, updated.State.MilestoneUSD)
	assert.EqualValues(t, 250000000, updated.UnnotifiedQuota, "the unsent $500 remainder survives rollback")
	var option model.Option
	require.NoError(t, model.DB.Where(map[string]any{"key": "quota_alert_user_cumulative_milestone"}).First(&option).Error)
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &states))
	assert.Equal(t, state, states["9"], "transferring one account must preserve every other option-map entry")
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Where("user_id = ?", 8).Update("quota", 250000000).Error)
	fact.ID, fact.SourceID, fact.Quota, fact.RequestCount = model.AlertIdentity("account-spend-refund"), "account-spend-refund", -1000000000, 0
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	updated, err = alerting.LegacyUserSpendSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.EqualValues(t, -750000000, updated.UnnotifiedQuota)
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	require.NoError(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 3, "cutover", updated.SnapshotHash, true, rule.Revision))
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	require.NoError(t, alerting.SwitchLegacyUserSpend(ctx, root, migration.ID, 4, "rollback", updated.SnapshotHash, true, rule.Revision))
	updated, err = alerting.LegacyUserSpendSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.EqualValues(t, 3000, updated.State.MilestoneUSD, "refunds cannot replay an already notified tier")
	assert.EqualValues(t, -750000000, updated.UnnotifiedQuota)
	var account model.User
	require.NoError(t, model.DB.First(&account, 8).Error)
	assert.Equal(t, 90000, account.Quota, "migration transfers alert checkpoints without changing money")
}

func testAlertUserReportMigration(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	local := now.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	previous := day.AddDate(0, 0, -1).Unix()
	oldEnabled, oldControls, oldShadow, oldStats := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = true, true, false, true
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = oldEnabled, oldControls, oldShadow, oldStats
	})
	var original model.SystemInstance
	require.NoError(t, model.DB.First(&original, "node_name = ?", "alert-test-node").Error)
	clock := func(at time.Time) {
		require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Updates(map[string]any{"started_at": day.Add(-72 * time.Hour).Unix(), "last_seen_at": at.Unix()}).Error)
		require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"healthy": true, "updated_at": at.UnixMilli(), "captured_at": at.Add(-time.Second).UnixMilli(), "coverage_started_at": day.Add(-72 * time.Hour).UnixMilli()}).Error)
	}
	t.Cleanup(func() {
		current := time.Now()
		require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Updates(map[string]any{"started_at": original.StartedAt, "last_seen_at": current.Unix()}).Error)
		require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"healthy": true, "updated_at": current.UnixMilli(), "captured_at": current.Add(-time.Second).UnixMilli()}).Error)
	})
	clock(now)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 8, CreatedAt: previous + 3600, Quota: 100000000}).Error)
	days := map[string]int64{"9": previous}
	encoded, err := common.Marshal(days)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Option{Key: "quota_alert_user_last_day_summary_map", Value: string(encoded)}).Error)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": key})
	require.NoError(t, err)
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	migration, err := alerting.ImportLegacyUserReport(ctx, root, vault, 8)
	require.NoError(t, err)
	snapshot, err := alerting.LegacyUserReportSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.Error(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, true, 1, now), "the previous legacy report must not disappear at cutover")
	days["8"] = previous
	encoded, err = common.Marshal(days)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Option{}).Where(map[string]any{"key": "quota_alert_user_last_day_summary_map"}).Update("value", string(encoded)).Error)
	snapshot, err = alerting.LegacyUserReportSnapshot(ctx, 8)
	require.NoError(t, err)
	require.NoError(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, true, 1, now))
	var rule model.AlertRule
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	assert.Equal(t, day.UnixMilli(), rule.EffectiveAt)
	spec, err := model.AlertRuleConfig(rule)
	require.NoError(t, err)
	targets, err := alerting.ResolveTargets(ctx, rule, spec)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	engine := alerting.Engine{BootID: "daily-cutover", Deliver: func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) { return 200, nil }}
	require.NoError(t, engine.EvaluateReports(ctx, rule, spec, targets[0], now))
	var count int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ?", rule.ID).Count(&count).Error)
	assert.Zero(t, count, "the prior date was already handled by the legacy executor")
	future := day.AddDate(0, 0, 1).Add(40 * time.Minute)
	clock(future)
	for range 2 {
		require.NoError(t, engine.EvaluateReports(ctx, rule, spec, targets[0], future))
	}
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND kind = ?", rule.ID, "report").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Error(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true, rule.Revision, future))
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id = ?", rule.ID).Update("status", "failed").Error)
	require.NoError(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true, rule.Revision, future))
	snapshot, err = alerting.LegacyUserReportSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.Equal(t, previous, snapshot.LastReportDay, "failed delivery cannot advance the old report date")
	clock(now)
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	require.NoError(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 3, "cutover", snapshot.SnapshotHash, true, rule.Revision, now))
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	clock(future)
	require.NoError(t, engine.EvaluateReports(ctx, rule, spec, targets[0], future))
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id = ? AND status = ?", rule.ID, "pending").Update("available_at", time.Now().Add(-time.Second).UnixMilli()).Error)
	require.NoError(t, engine.RunJobs(ctx, "notify", time.Now()))
	require.NoError(t, alerting.SwitchLegacyUserReport(ctx, root, migration.ID, 4, "rollback", snapshot.SnapshotHash, true, rule.Revision, future))
	snapshot, err = alerting.LegacyUserReportSnapshot(ctx, 8)
	require.NoError(t, err)
	assert.Equal(t, day.Unix(), snapshot.LastReportDay, "accepted report dates survive rollback without another send")
	var option model.Option
	require.NoError(t, model.DB.Where(map[string]any{"key": "quota_alert_user_last_day_summary_map"}).First(&option).Error)
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &days))
	assert.Equal(t, previous, days["9"])
}

func testAlertKeyPeriodMigration(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	oldEnabled, oldControls, oldShadow, oldStats := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = true, true, false, true
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.DataExportEnabled = oldEnabled, oldControls, oldShadow, oldStats
	})
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
	configs := map[string]*model.LegacyKeyQuotaUserConfig{"1": {FeishuRobotURL: "https://8.8.8.8/key-bot", OneHour: []model.LegacyKeyQuotaRule{{QuotaUSD: 100, Action: "warn"}, {QuotaUSD: 200, Action: "warn"}}, OneDay: []model.LegacyKeyQuotaRule{{QuotaUSD: 500, Action: "disable_key"}}}, "2": {FeishuRobotURL: "https://8.8.8.8/other-key-bot"}}
	encoded, err := common.Marshal(configs)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Option{Key: "key_quota_warning_config", Value: string(encoded)}).Error)
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))})
	require.NoError(t, err)
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	object := model.LegacyKeyQuotaObject(1, "hour", configs["1"].OneHour[0])
	migration, err := alerting.ImportLegacyKey(ctx, root, vault, "key_hour", object)
	require.NoError(t, err)
	again, err := alerting.ImportLegacyKey(ctx, root, vault, "key_hour", object)
	require.NoError(t, err)
	assert.Equal(t, migration.ID, again.ID)
	second, err := alerting.ImportLegacyKey(ctx, root, vault, "key_hour", model.LegacyKeyQuotaObject(1, "hour", configs["1"].OneHour[1]))
	require.NoError(t, err)
	assert.NotEqual(t, migration.ID, second.ID, "different thresholds never share a unified identity")
	day, err := alerting.ImportLegacyKey(ctx, root, vault, "key_day", model.LegacyKeyQuotaObject(1, "day", configs["1"].OneDay[0]))
	require.NoError(t, err)
	var dayRule model.AlertRule
	require.NoError(t, model.DB.First(&dayRule, day.RuleID).Error)
	daySpec, err := model.AlertRuleConfig(dayRule)
	require.NoError(t, err)
	require.Len(t, daySpec.Actions, 1)
	assert.Equal(t, "disable_api_key", daySpec.Actions[0].Type)
	var rule model.AlertRule
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	snapshot, err := alerting.LegacyKeySnapshot(ctx, "key_hour", object, now)
	require.NoError(t, err)
	assert.True(t, snapshot.Ambiguous)
	assert.Error(t, alerting.SwitchLegacyKey(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, false, rule.Revision, now))
	require.NoError(t, alerting.SwitchLegacyKey(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, true, rule.Revision, now))
	require.NoError(t, model.DB.First(&rule, rule.ID).Error)
	assert.Equal(t, snapshot.CutoverAt, rule.EffectiveAt)
	unified, err := model.LegacyAlertUsesUnified(ctx, "key_hour", object)
	require.NoError(t, err)
	assert.False(t, unified, "old executor owns the current ambiguous period")
	configs["2"].FeishuRobotURL = "https://8.8.8.8/unrelated-owner"
	encoded, err = common.Marshal(configs)
	require.NoError(t, err)
	require.NoError(t, model.ValidateLegacyKeyQuotaEdit(ctx, string(encoded)))
	configs["1"].OneHour[0].QuotaUSD = 999
	encoded, err = common.Marshal(configs)
	require.NoError(t, err)
	assert.ErrorIs(t, model.ValidateLegacyKeyQuotaEdit(ctx, string(encoded)), model.ErrAlertConflict)
	spec, err := model.AlertRuleConfig(rule)
	require.NoError(t, err)
	spec.Enabled = false
	paused, err := alerting.SaveRule(ctx, root, 1, rule.ID, rule.Revision, spec)
	require.NoError(t, err)
	spec.Enabled = true
	resumed, err := alerting.SaveRule(ctx, root, 1, rule.ID, paused.Revision, spec)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resumed.EffectiveAt, snapshot.CutoverAt)
	assert.ErrorIs(t, func() error {
		_, err := alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, resumed.ID, resumed.Revision, spec)
		return err
	}(), alerting.ErrForbidden)
	// A rollback before the boundary cancels the scheduled takeover, without
	// inventing old delivery markers or changing Key/quota state.
	require.NoError(t, alerting.SwitchLegacyKey(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, true, resumed.Revision, now))
	var token model.Token
	require.NoError(t, model.DB.Create(&model.Token{Id: 29, UserId: 1, Name: "Legacy quota status", Key: "legacy-quota-status", Status: common.TokenStatusEnabled, RemainQuota: 12345}).Error)
	require.NoError(t, model.DB.First(&token, 29).Error)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	beforeVersion := token.AuthVersion
	require.NoError(t, model.DisableLegacyQuotaToken(ctx, 29))
	require.NoError(t, model.DisableLegacyQuotaToken(ctx, 29))
	require.NoError(t, model.DB.First(&token, 29).Error)
	assert.Equal(t, beforeVersion+1, token.AuthVersion)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	assert.Equal(t, 12345, token.RemainQuota)
}

func testAlertLegacyNoticeMigration(t *testing.T) {
	ctx := context.Background()
	t.Setenv("ENABLE_UID_FEISHU_ALERTS", "true")
	oldStats := common.DataExportEnabled
	common.DataExportEnabled = true
	t.Cleanup(func() { common.DataExportEnabled = oldStats })
	oldEnabled, oldControls, oldShadow := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode = true, true, false
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode = oldEnabled, oldControls, oldShadow
	})
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))})
	require.NoError(t, err)
	option := model.Option{Key: "uid_quota_warning_robot"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: option.Key}).Error)
	require.NoError(t, model.DB.Model(&option).Update("value", "https://8.8.8.8/uid-events").Error)
	migration, err := alerting.ImportLegacyNotice(ctx, root, vault, "uid_day", "site")
	require.NoError(t, err)
	var rule model.AlertRule
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	oldSends := 0
	notice := alerting.LegacyNotice{Title: "UID daily threshold", Content: "UID=persisted-uid, plan=7, day=2026-11-05, quota=$10000"}
	legacy := func(context.Context) error { oldSends++; return nil }
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "uid-7-day", "persisted-uid", notice, legacy))
	assert.Equal(t, 1, oldSends)
	snapshot, err := alerting.LegacyNoticeSnapshot(ctx, "uid_day", "site")
	require.NoError(t, err)
	common.DataExportEnabled = false
	assert.Error(t, alerting.SwitchLegacyNotice(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, rule.Revision), "a compatibility checkpoint cannot take over an inactive statistics source")
	common.DataExportEnabled = true
	require.NoError(t, alerting.SwitchLegacyNotice(ctx, root, migration.ID, 1, "cutover", snapshot.SnapshotHash, rule.Revision))
	require.ErrorIs(t, model.ValidateLegacyNoticeEdit(ctx, map[string]string{"uid_quota_warning_robot": ""}), model.ErrAlertConflict)
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "uid-7-day", "persisted-uid", notice, legacy))
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "uid-7-day", "persisted-uid", notice, legacy))
	assert.Equal(t, 1, oldSends, "unified acceptance never falls through to a legacy send")
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ?", rule.ID).Find(&events).Error)
	require.Len(t, events, 2, "observation and active delivery retain separate records")
	assert.Contains(t, alerting.RenderNotification("zh-CN", events[0]), "plan=7")
	require.NoError(t, model.DB.First(&rule, rule.ID).Error)
	assert.Error(t, alerting.SwitchLegacyNotice(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, rule.Revision))
	engine := alerting.Engine{BootID: "notice-delivery", Deliver: func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) { return 200, nil }}
	require.NoError(t, engine.RunJobs(ctx, "notify", time.Now()))
	require.NoError(t, alerting.SwitchLegacyNotice(ctx, root, migration.ID, 2, "rollback", snapshot.SnapshotHash, rule.Revision))
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "uid-new-day", "persisted-uid", notice, legacy))
	assert.Equal(t, 2, oldSends)
	cooldown := alerting.LegacyNoticeCooldown{Key: "channel-7", Duration: time.Minute}
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "new-channel-event", "channel", notice, legacy, cooldown))
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "uid_day", "site", "another-node-event", "channel", notice, legacy, cooldown))
	assert.Equal(t, 3, oldSends, "SQL cooldown survives a different event identity or process")

	require.NoError(t, model.DB.Create(&model.Option{Key: "uid_quota_warning_email", Value: "first@example.com;second@example.com"}).Error)
	budgetMigration, err := alerting.ImportLegacyNotice(ctx, root, vault, "uid_budget_notice", "site")
	require.NoError(t, err)
	var budgetRule model.AlertRule
	require.NoError(t, model.DB.First(&budgetRule, budgetMigration.RuleID).Error)
	budgetSpec, err := model.AlertRuleConfig(budgetRule)
	require.NoError(t, err)
	assert.Len(t, budgetSpec.Notify.TargetIDs, 3, "preserve the legacy semicolon email recipient list")
	// Balance/subscription and channel status notices keep the user's chosen
	// destination and payload. No wallet or subscription mutation occurs here.
	setting := `{"notify_type":"webhook","webhook_url":"https://8.8.8.8/legacy-user","webhook_secret":"not-a-live-secret"}`
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 2).Update("setting", setting).Error)
	userMigration, err := alerting.ImportLegacyNotice(ctx, root, vault, "user_notifications", "2")
	require.NoError(t, err)
	userSnapshot, err := alerting.LegacyNoticeSnapshot(ctx, "user_notifications", "2")
	require.NoError(t, err)
	var userRule model.AlertRule
	require.NoError(t, model.DB.First(&userRule, userMigration.RuleID).Error)
	require.NoError(t, alerting.SwitchLegacyNotice(ctx, root, userMigration.ID, 1, "cutover", userSnapshot.SnapshotHash, userRule.Revision))
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "user_notifications", "2", "balance-crossing", "account", alerting.LegacyNotice{Type: "quota_exceed", Title: "Balance", Content: "Remaining 10", Values: []any{10}}, legacy))
	var user model.User
	require.NoError(t, model.DB.First(&user, 2).Error)
	assert.Zero(t, user.Quota)
	assert.Zero(t, user.UsedQuota)
	require.NoError(t, engine.RunJobs(ctx, "notify", time.Now()))
}

func testAlertEvaluationIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	row, spec, target := alertRuleFixture(t, "request_count")
	spec.Condition.Operator = "gte"
	spec.Condition.ThresholdCount = 1
	spec.Condition.MinSamples = 1
	spec.Condition.Window.DurationSeconds = 60
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	row.EffectiveAt = now.Add(-time.Hour).UnixMilli()
	row.NextEvaluationAt = 0
	require.NoError(t, model.DB.Model(&row).Select("config", "effective_at", "next_evaluation_at").Updates(&row).Error)
	fact := model.AlertObservation{ID: model.AlertIdentity("isolated-eval-fact"), Namespace: "health", SourceID: "isolated-eval", Kind: "finished", Layer: "request", Sequence: 1, StartedAt: now.Add(-time.Minute).UnixMilli(), OccurredAt: now.Add(-30 * time.Second).UnixMilli(), UserID: 1, TokenID: 11, Outcome: "success", AttributionCovered: true}
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	broken := model.AlertRule{Name: "Malformed imported configuration", OwnerUserID: 1, Enabled: true, Revision: 1, Generation: 1, Config: "{"}
	require.NoError(t, model.DB.Create(&broken).Error)
	engine := alerting.Engine{BootID: "isolated-evaluator"}
	require.NoError(t, engine.Evaluate(ctx, now))
	require.NoError(t, model.DB.First(&broken, broken.ID).Error)
	assert.Equal(t, "invalid_configuration", broken.LastEvaluationError)
	var count int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND kind = ?", row.ID, "trigger").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	for _, tokenID := range []int{6011, 6012} {
		require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: 1, Name: fmt.Sprintf("Sweep key %d", tokenID), Key: fmt.Sprintf("sweep-test-key-%d", tokenID)}).Error)
		queued := fact
		queued.ID, queued.SourceID, queued.TokenID = model.AlertIdentity("resume-fact", tokenID), fmt.Sprintf("resume-life-%d", tokenID), tokenID
		require.NoError(t, model.AcceptAlertObservation(ctx, queued))
	}
	// A persisted interrupted sweep must resume on another evaluator boot.
	spec.Scope.EntityIDs = []int{6011, 6012}
	resumed, err := alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, 0, 0, spec)
	require.NoError(t, err)
	groups, err := alerting.ResolveTargets(ctx, *resumed, spec)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	if groups[0].Key() > groups[1].Key() {
		groups[0], groups[1] = groups[1], groups[0]
	}
	resumed.EffectiveAt, resumed.EvaluationCursor, resumed.NextEvaluationAt = now.Add(-time.Hour).UnixMilli(), groups[0].Key(), 0
	require.NoError(t, model.DB.Model(resumed).Select("effective_at", "evaluation_cursor", "next_evaluation_at").Updates(resumed).Error)
	require.NoError(t, engine.ApplyMeasurement(ctx, *resumed, spec, groups[0], alerting.Measurement{Status: "complete", Samples: 999, Value: 999}, now))
	takeover := alerting.Engine{BootID: "takeover-evaluator"}
	require.NoError(t, takeover.Evaluate(ctx, now.Add(time.Second)))
	var resumedEvents []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", resumed.ID, "trigger").Find(&resumedEvents).Error)
	assert.Len(t, resumedEvents, 2)
	var committed model.AlertRuleState
	require.NoError(t, model.DB.Where("id = ?", model.AlertIdentity("state", resumed.ID, resumed.Generation, groups[0].Type, groups[0].ID, groups[0].Model)).First(&committed).Error)
	assert.Equal(t, "999", committed.LastValue, "resume does not repeatedly evaluate the earlier group")
	require.NoError(t, model.DB.First(resumed, resumed.ID).Error)
	assert.Empty(t, resumed.EvaluationCursor, "completed sweeps start a fresh pass next time")
	// A fresh heartbeat does not prove a full historical window after restart.
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Update("coverage_started_at", now.Add(-time.Second).UnixMilli()).Error)
	measurement, err := alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.NotEqual(t, "complete", measurement.Status)
	priorEnd := now.Add(-30 * time.Second).UnixMilli()
	priorBoot := model.AlertWorkerCheckpoint{ID: "completed-prior-boot", NodeID: "alert-test-node", BootID: "completed-prior-boot", Kind: "producer", ProtocolVersion: model.AlertSourceProtocolVersion, Healthy: true, CoverageStartedAt: now.Add(-time.Hour).UnixMilli(), CapturedAt: priorEnd, LastCompleteAt: priorEnd, UpdatedAt: priorEnd}
	require.NoError(t, model.DB.Create(&priorBoot).Error)
	status, _, err := alerting.WindowCoverage(ctx, now.Add(-time.Minute).UnixMilli(), priorEnd, now)
	require.NoError(t, err)
	assert.Equal(t, "complete", status, "a completed old boot can prove an entirely historical period")
	status, _, err = alerting.WindowCoverage(ctx, now.Add(-time.Minute).UnixMilli(), now.Add(-time.Second).UnixMilli(), now)
	require.NoError(t, err)
	assert.NotEqual(t, "complete", status, "old evidence cannot bridge an unobserved restart gap")
	require.NoError(t, model.DB.Delete(&priorBoot).Error)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Update("coverage_started_at", now.Add(-72*time.Hour).UnixMilli()).Error)
}

func testAlertPriority(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	target := model.AlertTarget{OwnerUserID: 0, Name: "Platform on-call", Type: "email", Enabled: true, Revision: 1, Language: "en"}
	require.NoError(t, model.DB.Create(&target).Error)
	event := model.AlertEvent{DedupKey: model.AlertIdentity("priority-event"), OwnerUserID: 0, Kind: "test", Severity: "P1", CreatedAt: now.UnixMilli()}
	require.NoError(t, model.DB.Create(&event).Error)
	jobs := []model.AlertJob{
		{DedupKey: model.AlertIdentity("personal-P0"), Platform: false, Priority: 0},
		{DedupKey: model.AlertIdentity("platform-P1"), Platform: true, Priority: 1},
	}
	for i := range jobs {
		jobs[i].EventID = event.ID
		jobs[i].Kind = "notify"
		jobs[i].TargetID = target.ID
		jobs[i].TargetRevision = target.Revision
		jobs[i].Status = "pending"
		jobs[i].AvailableAt = now.UnixMilli()
		jobs[i].ExpiresAt = now.Add(time.Hour).UnixMilli()
		jobs[i].CreatedAt = now.UnixMilli()
		require.NoError(t, model.DB.Create(&jobs[i]).Error)
	}
	deliveries := 0
	engine := alerting.Engine{BootID: "reserved-worker", Deliver: func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) { deliveries++; return 200, nil }}
	require.NoError(t, engine.RunUrgentJobs(ctx, "notify", now))
	require.NoError(t, model.DB.First(&jobs[0], jobs[0].ID).Error)
	require.NoError(t, model.DB.First(&jobs[1], jobs[1].ID).Error)
	assert.Equal(t, 1, deliveries)
	assert.Equal(t, "pending", jobs[0].Status, "personal P0 cannot consume the reserved worker")
	assert.Equal(t, "succeeded", jobs[1].Status, "the urgent worker retains its own completion fence")
	destination := model.AlertTarget{Type: "feishu", RateKey: model.AlertIdentity("atomic-rate-test")}
	for _, tc := range []struct {
		priority int
		allowed  bool
	}{{3, true}, {3, true}, {3, false}, {0, true}, {0, true}, {0, true}, {0, false}} {
		allowed, err := model.ReserveAlertNotification(ctx, destination, tc.priority, now)
		require.NoError(t, err)
		assert.Equal(t, tc.allowed, allowed)
	}
}

func testAlertMetricCache(t *testing.T) {
	socket := os.Getenv("ALERTING_TEST_REDIS_SOCKET")
	if socket == "" {
		t.Skip("ALERTING_TEST_REDIS_SOCKET not configured")
	}
	ctx := context.Background()
	now := time.Now()
	first := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	second := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, first.Close()); require.NoError(t, second.Close()) })
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, first
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldRDB })
	require.NoError(t, model.DB.Create(&model.Token{Id: 31, UserId: 1, Name: "Projected business key", Key: "projected-business-key", Status: common.TokenStatusEnabled}).Error)
	row, spec, target := alertRuleFixture(t, "request_count")
	target.ID, target.Name = 31, "Projected business key"
	spec.Scope.EntityIDs = []int{31}
	spec.Condition.MinSamples = 1
	row.EffectiveAt = now.Add(-time.Hour).UnixMilli()
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	require.NoError(t, model.DB.Model(&row).Select("config", "effective_at").Updates(&row).Error)
	fact := model.AlertObservation{ID: model.AlertIdentity("shared-metric-1"), SourceID: "shared-metric-1", Namespace: "health", Kind: "finished", Layer: "request", Sequence: 1, StartedAt: now.Add(-2 * time.Minute).UnixMilli(), OccurredAt: now.Add(-time.Minute).UnixMilli(), UserID: 1, TokenID: 31, Outcome: "success", AttributionCovered: true}
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	m, err := alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	require.Equal(t, "complete", m.Status, m.Reason)
	assert.EqualValues(t, 1, m.Value)
	// A second client represents a separate worker, sharing the committed snapshot.
	hits, _ := alerting.MetricProjectionStats()
	common.RDB = second
	m, err = alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, m.Value)
	after, _ := alerting.MetricProjectionStats()
	assert.Equal(t, hits+1, after)
	late := fact
	late.ID = model.AlertIdentity("shared-metric-late")
	late.SourceID = "shared-metric-late"
	require.NoError(t, model.AcceptAlertObservation(ctx, late))
	m, err = alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, m.Value, "a late fact invalidates the exact same window")
	require.NoError(t, model.AcceptAlertObservation(ctx, late))
	m, err = alerting.Measure(ctx, row, spec, target, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, m.Value, "replay does not increment the minute revision or the result")
	// Cached quantities retain int64 precision beyond JavaScript's safe integer.
	usage := model.AlertObservation{ID: model.AlertIdentity("shared-metric-money"), SourceID: "shared-metric-money", Namespace: "usage", Kind: "settlement", OccurredAt: now.Add(-time.Minute).UnixMilli(), UserID: 1, TokenID: 31, Quota: 9007199254740993}
	require.NoError(t, model.AcceptAlertObservation(ctx, usage))
	spec.Condition.Metric = "spend_net"
	spec.Condition.ThresholdUSD = "100"
	spec.Condition.RecoveryThresholdUSD = "0"
	for range 2 {
		m, err = alerting.Measure(ctx, row, spec, target, now)
		require.NoError(t, err)
		assert.EqualValues(t, 9007199254740993, m.Value)
	}
}

func testAlertMilestoneActions(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	oldActions, oldShadow, oldControls := common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled
	common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = true, false, true
	t.Cleanup(func() {
		common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = oldActions, oldShadow, oldControls
	})
	require.NoError(t, model.DB.Create(&model.Token{Id: 32, UserId: 1, Name: "Milestone business key", Key: "milestone-action-key", Status: common.TokenStatusEnabled}).Error)
	row, spec, target := alertRuleFixture(t, "spend_net")
	target.ID, target.Name = 32, "Milestone business key"
	spec.Scope.EntityIDs = []int{32}
	spec.Condition.MinSamples = 1
	spec.Condition.Window.Type = "cumulative"
	spec.Trigger.Mode = "milestone"
	spec.Trigger.StepUSD = "100"
	spec.Actions = []model.AlertAction{{Type: "disable_api_key", Target: "matched_entity"}}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	row.EffectiveAt = now.Add(-time.Hour).UnixMilli()
	require.NoError(t, model.DB.Model(&row).Select("config", "effective_at").Updates(&row).Error)
	quota, err := alerting.USDQuota("150")
	require.NoError(t, err)
	fact := model.AlertObservation{ID: model.AlertIdentity("milestone-action-fact"), SourceID: "milestone-action-fact", Namespace: "usage", Kind: "settlement", OccurredAt: now.Add(-time.Minute).UnixMilli(), UserID: 1, TokenID: 32, Quota: quota}
	require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	engine := alerting.Engine{BootID: "milestone-actions"}
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: quota, Samples: 1}, now))
	require.NoError(t, engine.RunJobs(ctx, "action", now))
	var jobs []model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "action").Find(&jobs).Error)
	require.Len(t, jobs, 1)
	assert.Equal(t, "succeeded", jobs[0].Status)
	var token model.Token
	require.NoError(t, model.DB.First(&token, 32).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	// The same event must be rechecked against accepted refunds at execution.
	refund := fact
	refund.ID = model.AlertIdentity("milestone-action-refund")
	refund.SourceID = "milestone-action-refund"
	refund.Quota = -quota
	require.NoError(t, model.AcceptAlertObservation(ctx, refund))
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: quota * 2, Samples: 1}, now))
	require.NoError(t, engine.RunJobs(ctx, "action", now))
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "action").Order("id asc").Find(&jobs).Error)
	require.Len(t, jobs, 2)
	assert.Equal(t, "canceled", jobs[1].Status)
	// Historical preview shares exact window arithmetic and has no side effects.
	spec.Condition.Window.Type = "rolling"
	spec.Condition.Window.DurationSeconds = 300
	points, err := alerting.ReplayRule(ctx, row, spec, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 12, now)
	require.NoError(t, err)
	require.Len(t, points, 12)
	for _, point := range points {
		assert.Equal(t, "Milestone business key", point.Target.Name)
		assert.False(t, point.Matched)
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("rule_id = ? AND kind = ?", row.ID, "action").Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func testAlertFallback(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	oldShadow := common.AlertingShadowMode
	common.AlertingShadowMode = false
	t.Cleanup(func() { common.AlertingShadowMode = oldShadow })
	row, spec, target := alertRuleFixture(t, "request_count")
	fallback := model.AlertTarget{OwnerUserID: 1, Name: "Independent backup destination", Type: "email", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&fallback).Error)
	spec.Acknowledgement.EscalationTargetIDs = []int64{spec.Notify.TargetIDs[0], fallback.ID}
	encoded, err := common.Marshal(spec)
	require.NoError(t, err)
	row.Config = string(encoded)
	require.NoError(t, model.DB.Model(&row).Update("config", row.Config).Error)
	engine := alerting.Engine{BootID: "fallback-receipts", Deliver: func(context.Context, model.AlertTarget, model.AlertEvent) (int, error) {
		return 503, errors.New("transport unavailable")
	}}
	require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: 100000, Samples: 1}, now))
	require.NoError(t, engine.RunJobs(ctx, "notify", now))
	var jobs []model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&jobs).Error)
	require.Len(t, jobs, 2)
	ids := map[int64]bool{}
	for _, job := range jobs {
		ids[job.TargetID] = true
		assert.Equal(t, "retry_wait", job.Status)
	}
	assert.True(t, ids[fallback.ID])
	assert.True(t, ids[spec.Notify.TargetIDs[0]])
}

func testAlertTargetManagement(t *testing.T) {
	ctx := context.Background()
	keys := map[string]string{"old": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("o", 32))), "new": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("n", 32)))}
	old, err := alerting.NewTargetVault("old", keys)
	require.NoError(t, err)
	input := alerting.TargetInput{Name: "Private operations address", Type: "email", Enabled: true, Language: "en", Credentials: &alerting.TargetSecret{Address: "ops@example.com"}}
	user := alerting.Actor{ID: 1, Role: common.RoleCommonUser}
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	target, err := old.Save(ctx, user, 1, 0, input)
	require.NoError(t, err)
	encoded, err := common.Marshal(target)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "ops@example.com")
	assert.NotContains(t, string(encoded), "ciphertext")
	rotated, err := alerting.NewTargetVault("new", keys)
	require.NoError(t, err)
	input.ExpectedRevision, input.Credentials = target.Revision, nil
	updated, err := rotated.Save(ctx, user, 1, target.ID, input)
	require.NoError(t, err)
	assert.Equal(t, "new", updated.KeyID)
	secret, err := rotated.Open(*updated)
	require.NoError(t, err)
	assert.Equal(t, "ops@example.com", secret.Address)
	input.Credentials = &alerting.TargetSecret{Address: "other@example.com"}
	input.ExpectedRevision = 0
	_, err = rotated.Save(ctx, user, 2, 0, input)
	assert.ErrorIs(t, err, alerting.ErrForbidden)
	_, err = rotated.Save(ctx, root, 2, 0, input)
	require.NoError(t, err)
	t.Setenv("ALERTING_PRIVATE_TARGET_CIDRS", "10.0.0.0/24")
	input.Type = "webhook_v2"
	input.Credentials = &alerting.TargetSecret{Address: "https://10.0.0.7/notify", PrivateNetwork: true}
	_, err = rotated.Save(ctx, user, 1, 0, input)
	assert.ErrorIs(t, err, alerting.ErrForbidden)
	_, err = rotated.Save(ctx, root, 0, 0, input)
	require.NoError(t, err)
	input.Credentials.Address = "https://10.1.0.7/notify"
	_, err = rotated.Save(ctx, root, 0, 0, input)
	assert.ErrorIs(t, err, alerting.ErrTargetRejected)
	input.Credentials.Address = "https://169.254.169.254/metadata"
	_, err = rotated.Save(ctx, root, 0, 0, input)
	assert.ErrorIs(t, err, alerting.ErrTargetRejected)
	for _, item := range []struct {
		setting map[string]any
		kind    string
		feishu  bool
	}{
		{map[string]any{"notify_type": "email", "notification_email": "legacy@example.com"}, "email", false},
		{map[string]any{"notify_type": "webhook", "webhook_url": "https://8.8.8.8/generic", "webhook_secret": "old-secret"}, "webhook_v1", false},
		{map[string]any{"notify_type": "webhook", "webhook_url": "https://8.8.8.8/bot", "webhook_secret": "old-secret"}, "feishu", true},
		{map[string]any{"notify_type": "bark", "bark_url": "https://8.8.8.8/key/{{title}}/{{content}}"}, "bark", false},
		{map[string]any{"notify_type": "gotify", "gotify_url": "https://8.8.8.8", "gotify_token": "old-token", "gotify_priority": 7}, "gotify", false},
	} {
		setting, err := common.Marshal(item.setting)
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 2).Update("setting", string(setting)).Error)
		_, err = alerting.ImportLegacyNotification(ctx, user, rotated, 2, item.feishu)
		assert.ErrorIs(t, err, alerting.ErrForbidden)
		first, err := alerting.ImportLegacyNotification(ctx, root, rotated, 2, item.feishu)
		require.NoError(t, err)
		assert.Equal(t, item.kind, first.Type)
		second, err := alerting.ImportLegacyNotification(ctx, root, rotated, 2, item.feishu)
		require.NoError(t, err)
		assert.Equal(t, first.ID, second.ID, "repeat import reuses the same encrypted destination")
		secret, err := rotated.Open(*first)
		require.NoError(t, err)
		if item.kind == "gotify" {
			assert.Equal(t, "old-token", secret.Token)
			assert.Equal(t, 7, secret.Priority)
		}
		var unchanged model.User
		require.NoError(t, model.DB.Select("id", "setting").First(&unchanged, 2).Error)
		assert.Equal(t, string(setting), unchanged.Setting)
	}
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 2).Update("setting", "").Error)
}

func testAlertWatchdog(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	destination := model.AlertTarget{OwnerUserID: 0, Name: "Independent platform operations", Type: "email", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&destination).Error)
	engine := alerting.Engine{BootID: "independent-watchdog"}
	for range 2 {
		require.NoError(t, engine.Watchdog(ctx, []int64{destination.ID}, now))
	}
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("kind = ?", "engine_health").Find(&events).Error)
	require.Len(t, events, 1)
	var jobs []model.AlertJob
	require.NoError(t, model.DB.Where("event_id = ?", events[0].ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	assert.True(t, jobs[0].Platform)
}

func testAlertSlowChannelCalibration(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	// Business-equivalent histograms reproduce the documented rounded means,
	// exact percentile ranks and 82/60 slow-request counts without user log IDs.
	cases := []struct {
		id                             int
		name                           string
		distribution                   [][2]int
		count                          int64
		mean, p95, p99, slow, verySlow int64
	}{
		{101, "Control provider A", [][2]int{{316, 2000}, {1274, 3000}, {100, 7000}, {26, 9000}}, 1716, 3139, 7000, 9000, 0, 0},
		{102, "Control provider B", [][2]int{{232, 2000}, {1347, 3000}, {100, 7000}, {26, 9000}}, 1705, 3190, 7000, 9000, 0, 0},
		{103, "Slow provider", [][2]int{{644, 11000}, {586, 12000}, {22, 48000}, {46, 90000}, {14, 224000}}, 1312, 17109, 48000, 224000, 82, 60},
	}
	rule := model.AlertRule{EffectiveAt: now.Add(-time.Hour).UnixMilli()}
	spec := model.AlertRuleSpec{Scope: model.AlertScope{Type: "channel", GroupMode: "per_entity"}, Condition: model.AlertCondition{Metric: "duration_ms", Aggregation: "p95", LatencyBasis: "channel_attempt", Operator: "gt", ThresholdMS: 30000, RecoveryThresholdMS: 10000, MinSamples: 20, Window: model.AlertWindow{Type: "rolling", DurationSeconds: 300, Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "incident"}}
	for _, tc := range cases {
		channel := model.Channel{Id: tc.id, Name: tc.name, Type: 1, Models: "sample-model", Group: "default", Status: common.ChannelStatusEnabled}
		require.NoError(t, model.DB.Create(&channel).Error)
		spec.Scope.ChannelIDs = []int{tc.id}
		targets, err := alerting.ResolveTargets(ctx, rule, spec)
		require.NoError(t, err)
		require.Len(t, targets, 1)
		assert.Equal(t, tc.name, targets[0].Name)
		facts := make([]model.AlertObservation, 0, tc.count)
		for _, bucket := range tc.distribution {
			for range bucket[0] {
				duration := int64(bucket[1])
				id := model.AlertIdentity("latency-calibration", tc.id, len(facts))
				facts = append(facts, model.AlertObservation{ID: id, SourceID: id, Namespace: "health", Kind: "finished", Layer: "channel_attempt", ChannelID: tc.id, ChannelName: tc.name, ModelKey: "sample-model", ConfigVersion: targets[0].ConfigVersion, SampleClass: "production", Outcome: "success", OccurredAt: now.Add(-time.Minute).UnixMilli(), DurationMS: &duration, AttributionCovered: true})
			}
		}
		require.NoError(t, model.DB.CreateInBatches(&facts, 100).Error)
		for _, aggregation := range []struct {
			name string
			want int64
		}{{"mean", tc.mean}, {"p95", tc.p95}, {"p99", tc.p99}} {
			spec.Condition.Aggregation = aggregation.name
			m, err := alerting.Measure(ctx, rule, spec, targets[0], now)
			require.NoError(t, err)
			require.Equal(t, "complete", m.Status, m.Reason)
			assert.Equal(t, tc.count, m.Samples)
			assert.Equal(t, aggregation.want, m.Value)
		}
		spec.Condition.Metric = "slow_request_count"
		for _, threshold := range []struct{ ms, want int64 }{{30000, tc.slow}, {60000, tc.verySlow}} {
			spec.Condition.SlowThresholdMS = threshold.ms
			m, err := alerting.Measure(ctx, rule, spec, targets[0], now)
			require.NoError(t, err)
			assert.Equal(t, threshold.want, m.Value)
		}
		spec.Condition.Metric = "duration_ms"
		spec.Condition.Aggregation = "p95"
		points, err := alerting.ReplayRule(ctx, rule, spec, now.Add(-time.Minute).UnixMilli(), now.UnixMilli(), 1, now)
		require.NoError(t, err)
		require.Len(t, points, 1)
		assert.Equal(t, tc.id == 103, points[0].Matched, "only the slow provider exceeds a 30 second p95 threshold")
	}
}

func testAlertUsageBridge(t *testing.T) {
	oldAlert, oldStats, oldLogs, oldRedis := common.AlertingEnabled, common.DataExportEnabled, common.LogConsumeEnabled, common.RedisEnabled
	common.AlertingEnabled, common.DataExportEnabled, common.LogConsumeEnabled, common.RedisEnabled = true, false, false, false
	t.Cleanup(func() {
		common.AlertingEnabled, common.DataExportEnabled, common.LogConsumeEnabled, common.RedisEnabled = oldAlert, oldStats, oldLogs, oldRedis
	})
	params := model.LogQuotaDataCache{EventID: model.QuotaDataActionID("alert-fixture", "statistics-toggle"), UserId: 1, Username: "alert-owner", ModelName: "alert-usage-bridge", ClientUserId: strings.Repeat("u", 200), ClientScenairo: strings.Repeat("s", 200), ProjectName: strings.Repeat("p", 200), PlanId: 1, Quota: 5000, CreatedAt: time.Now().Unix(), TokenId: 11, ChannelId: 77, TokenUsed: 15, PromptTokens: 10, CompletionTokens: 5, CachedTokens: 4, ClaudeCacheCreation5mTokens: 2, ClaudeCacheCreation1hTokens: 3, FirstTokenMs: 100, UseTimeSeconds: 2}
	require.NoError(t, model.LogQuotaData(&params))
	var count int64
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Where("model_name = ?", params.ModelName).Count(&count).Error)
	assert.Zero(t, count, "alert-only acceptance must not enable statistics")
	common.DataExportEnabled = true
	require.NoError(t, model.LogQuotaData(&params))
	require.NoError(t, model.SaveQuotaDataCache())
	refund := params
	refund.EventID = model.QuotaDataActionID("alert-fixture", "statistics-refund")
	refund.Quota = -2000
	refund.IsSettlementAdjustment = true
	require.NoError(t, model.LogQuotaData(&refund))
	require.NoError(t, model.SaveQuotaDataCache())
	var aggregate model.QuotaData
	require.NoError(t, model.DB.Where("model_name = ?", params.ModelName).First(&aggregate).Error)
	assert.Equal(t, 3000, aggregate.Quota)
	assert.Equal(t, 1, aggregate.Count)
	assert.Equal(t, 15, aggregate.TokenUsed)
	assert.Equal(t, 10, aggregate.PromptTokens)
	assert.Equal(t, 5, aggregate.CompletionTokens)
	assert.Equal(t, 4, aggregate.CachedTokens)
	assert.Equal(t, 2, aggregate.ClaudeCacheCreation5mTokens)
	assert.Equal(t, 3, aggregate.ClaudeCacheCreation1hTokens)
	assert.Equal(t, 1, aggregate.CacheWrite5mRequestCount)
	assert.Equal(t, 1, aggregate.CacheWrite1hRequestCount)
	assert.Equal(t, 1, aggregate.CacheReadRequestCount)
	assert.Equal(t, 1, aggregate.StreamRequestCount)
	assert.Equal(t, 100, aggregate.FrtSum)
	assert.Equal(t, 2000, aggregate.RequestTimeSum)
	common.DataExportEnabled = false
	model.RefundQuotaData(&model.Task{ID: 501, TaskID: "legacy-video-refund-fixture", UserId: 1, Quota: 1000, ChannelId: 77, Properties: model.Properties{TokenId: 11, OriginModelName: params.ModelName, ClientUserId: params.ClientUserId, ClientScenairo: params.ClientScenairo, ProjectName: params.ProjectName, PlanId: params.PlanId}})
	var totals struct{ Count, Quota, Requests, InputTokens int64 }
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("namespace = ? AND model_key = ?", "usage", params.ModelName).Select("COUNT(*) AS count,SUM(quota) AS quota,SUM(request_count) AS requests,SUM(input_tokens) AS input_tokens").Scan(&totals).Error)
	assert.EqualValues(t, 3, totals.Count)
	assert.EqualValues(t, 2000, totals.Quota)
	assert.EqualValues(t, 1, totals.Requests)
	assert.EqualValues(t, 10, totals.InputTokens)
	var accepted model.AlertObservation
	require.NoError(t, model.DB.Where("source_id = ? AND namespace = ?", params.EventID, "usage").First(&accepted).Error)
	assert.Equal(t, params.ClientUserId, accepted.ClientUserID)
	assert.Equal(t, params.ClientScenairo, accepted.ClientScenario)
	assert.Equal(t, params.ProjectName, accepted.ProjectName)
	assert.Equal(t, params.PlanId, accepted.PlanID)

	require.NoError(t, model.DB.Where("id = ?", aggregate.Id).First(&aggregate).Error)
	assert.Equal(t, 3000, aggregate.Quota, "independent alert refund never updates quota_data")
	// Exercise the actual consume-log entry: all traditional logging switches
	// are off, and no log database is configured by this fixture.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("username", "alert-owner")
	c.Set(common.RequestIdKey, "minimal-alert-only-log")
	model.RecordConsumeLog(c, 1, model.RecordConsumeLogParams{ModelName: "alert-minimal-only", Quota: 250, TokenId: 11, ChannelId: 77, Other: model.NewLogOther()})
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("namespace = ? AND model_key = ?", "usage", "alert-minimal-only").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Where("model_name = ?", "alert-minimal-only").Count(&count).Error)
	assert.Zero(t, count)
}
