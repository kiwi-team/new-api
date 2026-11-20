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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Representative persisted user/token schemas shared by v1.0.0-rc.47 and v1.0.0-rc.51. The release uses
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
	t.Setenv("ENV", "production")
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
			require.True(t, db.Migrator().HasIndex(&model.AlertOutbox{}, "idx_alert_outbox_pending_created"))
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
			t.Run("concurrent producer batches and superseded heartbeats", testAlertCheckpointWrites)
			t.Run("ownership and incident delivery", testAlertIncident)
			t.Run("withdrawn notification authorization", testAlertNotificationRevocation)
			t.Run("low traffic and source loss", testAlertInflight)
			t.Run("HTTP object isolation", testAlertHTTPIsolation)
			t.Run("rule list pagination and filters", testAlertRuleList)
			t.Run("rule preview requires a notification target or action", testAlertRuleDeliveryRequirement)
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
			t.Run("natural Key migration schedules an unambiguous boundary", testAlertKeyPeriodMigration)
			t.Run("legacy event adapters retain checkpoints and a single executor", testAlertLegacyNoticeMigration)
			t.Run("channel balance migration requires its legacy destination", testAlertChannelArrearsMigration)
			t.Run("bad rule cannot starve other rules", testAlertEvaluationIsolation)
			t.Run("reserved platform delivery and atomic limits", testAlertPriority)
			t.Run("shared metric projection and late facts", testAlertMetricCache)
			t.Run("milestone action freshness and historical preview", testAlertMilestoneActions)
			t.Run("high priority fallback preserves primary receipts", testAlertFallback)
			t.Run("target ownership rotation and private allowlist", testAlertTargetManagement)
			t.Run("independent watchdog deduplicates receipts", testAlertWatchdog)
			t.Run("slow channel distribution and threshold calibration", testAlertSlowChannelCalibration)
			t.Run("usage bridge with statistics and logs disabled", testAlertUsageBridge)
			t.Run("natural periods follow accepted input", testAlertPeriodWatermark)
			t.Run("recurrent incidents require fresh recovery", testAlertRecurrentRecovery)
			t.Run("one-shot incidents retain acknowledgement escalation", testAlertOnceEscalation)
			t.Run("realtime custom endpoints retain metric coverage", testAlertRealtimeCoverage)
			t.Run("target tests distinguish cooldown from configuration changes", testAlertTargetTestCooldown)
			t.Run("legacy channel notifications respect selected channels", testAlertLegacyChannelNotifications)
			t.Run("traffic baselines and zero volume", testAlertTrafficBaselines)
			t.Run("presentation edits retain approved actions", testAlertActionPresentationEdits)
			t.Run("root selects targets across accounts", testAlertRootTargets)
			t.Run("log retention preserves historical metrics and replay identities", testAlertLogRetention)
			t.Run("daily report cutover retains accepted dates", testAlertUserReportMigration)
		})
	}
}

func testAlertRuleDeliveryRequirement(t *testing.T) {
	ctx := context.Background()
	channel := model.Channel{Id: 49, Name: "usapi-gemini", Type: 24, Status: common.ChannelStatusEnabled, Models: "gemini-3.8-flash", Group: "default"}
	require.NoError(t, model.DB.Create(&channel).Error)
	target := model.AlertTarget{OwnerUserID: 0, Name: "Platform Feishu", Type: "feishu", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&target).Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Delete(&target).Error)
		require.NoError(t, model.DB.Delete(&channel).Error)
	})
	var spec model.AlertRuleSpec
	require.NoError(t, common.UnmarshalJsonStr(`{"name":"usapi-gemini channel","category":"health","severity":"P1","enabled":true,"management_mode":"platform_enforced","scope":{"type":"channel","selector":"ids","entity_ids":[],"channel_ids":[49],"group_mode":"per_entity","model_selector":{"mode":"explicit","models":[]}},"condition":{"metric":"success_rate","aggregation":"p95","latency_basis":"channel_attempt","operator":"lt","threshold_bps":9500,"recovery_threshold_bps":9900,"slow_threshold_ms":30000,"min_samples":2,"hold_seconds":30,"recovery_seconds":30,"deadline_kind":"first_output","deadline_ms":30000,"window":{"type":"rolling","duration_seconds":60,"timezone":"Asia/Shanghai","send_at":"00:10"}},"trigger":{"mode":"incident","repeat_seconds":100},"notify":{"target_ids":[],"on_recovery":true},"actions":[],"action_guard":{"min_remaining_channels":1,"on_no_alternative":"notify_only"},"recovery":{"mode":"manual"},"acknowledgement":{"required":true,"remind_after_seconds":300,"escalate_after_seconds":600,"escalation_target_ids":[]}}`, &spec))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 3); c.Set("role", common.RoleRootUser); c.Next() })
	router.POST("/rules/preview", controller.PreviewAlertRule)
	router.POST("/rules/backtest", controller.PreviewAlertRule)
	router.POST("/rules", controller.SaveAlertRule)
	router.PATCH("/rules/:id", controller.SaveAlertRule)
	for _, configured := range []bool{false, true} {
		if configured {
			spec.Notify.TargetIDs = []int64{target.ID}
		}
		body, err := common.Marshal(map[string]any{"config": spec, "owner_user_id": 0, "rule_id": 0, "expected_revision": 0})
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "/rules/preview", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var result struct {
			Success    bool   `json:"success"`
			MessageKey string `json:"message_key"`
			Field      string `json:"field"`
			Data       struct {
				Config model.AlertRuleSpec `json:"config"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		if !configured {
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.False(t, result.Success)
			assert.Equal(t, "Select at least one notification target or automatic action.", result.MessageKey)
			assert.Equal(t, "notify.target_ids", result.Field)
			continue
		}
		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, result.Success)
		assert.Equal(t, []int64{target.ID}, result.Data.Config.Notify.TargetIDs)
		assert.EqualValues(t, 9500, result.Data.Config.Condition.ThresholdBPS)
		assert.EqualValues(t, 9900, result.Data.Config.Condition.RecoveryThresholdBPS)
	}
	t.Run("maximum rolling preview reads a full accepted window without writes", func(t *testing.T) {
		candidate := spec
		candidate.Condition.Metric = "request_started_count"
		candidate.Condition.Window.DurationSeconds = 86400
		candidate.Condition.MinSamples = 0
		candidate.Condition.Operator = "lt"
		candidate.Condition.ThresholdCount, candidate.Condition.RecoveryThresholdCount = 1, 1
		body, err := common.Marshal(map[string]any{"config": candidate, "owner_user_id": 0})
		require.NoError(t, err)
		var before, after int64
		require.NoError(t, model.DB.Model(&model.AlertRule{}).Count(&before).Error)
		request := httptest.NewRequest(http.MethodPost, "/rules/preview", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Targets []struct {
					Measurement alerting.Measurement `json:"measurement"`
				} `json:"targets"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success)
		require.Len(t, result.Data.Targets, 1)
		measured := result.Data.Targets[0].Measurement
		assert.Equal(t, "complete", measured.Status, measured.Reason)
		assert.EqualValues(t, 86400000, measured.WindowEnd-measured.WindowStart)
		require.NoError(t, model.DB.Model(&model.AlertRule{}).Count(&after).Error)
		assert.Equal(t, before, after)
	})
	t.Run("target ownership across preview replay create and edit", func(t *testing.T) {
		personal := model.AlertTarget{OwnerUserID: 3, Name: "Root personal destination", Type: "feishu", Enabled: true, Revision: 1}
		foreign := model.AlertTarget{OwnerUserID: 1, Name: "Other user destination", Type: "feishu", Enabled: true, Revision: 1}
		disabled := model.AlertTarget{OwnerUserID: 0, Name: "Disabled platform destination", Type: "feishu", Enabled: false, Revision: 1}
		for _, destination := range []*model.AlertTarget{&personal, &foreign, &disabled} {
			require.NoError(t, model.DB.Create(destination).Error)
		}
		spec.Condition.ThresholdBPS = 9000
		seed, err := alerting.SaveRule(ctx, alerting.Actor{ID: 3, Role: common.RoleRootUser}, 0, 0, 0, spec)
		require.NoError(t, err)
		created := []int64{seed.ID}
		t.Cleanup(func() {
			require.NoError(t, model.DB.Where("id IN ?", created).Delete(&model.AlertRule{}).Error)
			for _, destination := range []*model.AlertTarget{&personal, &foreign, &disabled} {
				require.NoError(t, model.DB.Delete(destination).Error)
			}
		})
		for _, endpoint := range []struct{ method, path string }{{http.MethodPost, "/rules/preview"}, {http.MethodPost, "/rules/backtest"}, {http.MethodPost, "/rules"}, {http.MethodPatch, fmt.Sprintf("/rules/%d", seed.ID)}} {
			for _, tc := range []struct {
				name, field            string
				targetID, escalationID int64
				status                 int
				message                string
			}{
				{"root personal", "", personal.ID, 0, http.StatusOK, ""},
				{"other user", "", foreign.ID, 0, http.StatusOK, ""},
				{"missing", "notify.target_ids", 1000000000, 0, http.StatusForbidden, "Select notification targets available to this rule."},
				{"disabled", "notify.target_ids", disabled.ID, 0, http.StatusBadRequest, "Enable the selected notification target before enabling this rule."},
				{"escalation other user", "", target.ID, foreign.ID, http.StatusOK, ""},
				{"escalation missing", "acknowledgement.escalation_target_ids", target.ID, 1000000000, http.StatusForbidden, "Select notification targets available to this rule."},
				{"escalation disabled", "acknowledgement.escalation_target_ids", target.ID, disabled.ID, http.StatusBadRequest, "Enable the selected notification target before enabling this rule."},
				{"platform", "", target.ID, target.ID, http.StatusOK, ""},
			} {
				t.Run(endpoint.method+endpoint.path+tc.name, func(t *testing.T) {
					candidate := spec
					candidate.Notify.TargetIDs = []int64{tc.targetID}
					candidate.Acknowledgement.EscalationTargetIDs = nil
					if tc.escalationID > 0 {
						candidate.Acknowledgement.EscalationTargetIDs = []int64{tc.escalationID}
					}
					revision := int64(0)
					if endpoint.method == http.MethodPatch {
						revision = seed.Revision
					}
					body, err := common.Marshal(map[string]any{"config": candidate, "owner_user_id": 0, "rule_id": 0, "expected_revision": revision})
					require.NoError(t, err)
					response := httptest.NewRecorder()
					request := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(response, request)
					var result struct {
						Success    bool   `json:"success"`
						MessageKey string `json:"message_key"`
						Field      string `json:"field"`
						Data       any    `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
					assert.Equal(t, tc.status, response.Code, response.Body.String())
					assert.Equal(t, tc.status == http.StatusOK, result.Success)
					if tc.status != http.StatusOK {
						assert.Equal(t, tc.message, result.MessageKey)
						assert.Equal(t, tc.field, result.Field)
						assert.Nil(t, result.Data)
					} else if endpoint.path == "/rules" {
						data, ok := result.Data.(map[string]any)
						require.True(t, ok)
						created = append(created, int64(data["id"].(float64)))
					} else if endpoint.method == http.MethodPatch {
						data, ok := result.Data.(map[string]any)
						require.True(t, ok)
						seed.Revision = int64(data["revision"].(float64))
					}
				})
			}
		}
	})
	spec.Notify.TargetIDs = nil
	assert.ErrorIs(t, alerting.Validate(ctx, alerting.Actor{ID: 3, Role: common.RoleRootUser}, 0, &spec), alerting.ErrInvalid)
	spec.Actions = []model.AlertAction{{Type: "disable_channel", Target: "matched_entity"}}
	assert.NoError(t, alerting.Validate(ctx, alerting.Actor{ID: 3, Role: common.RoleRootUser}, 0, &spec), "action-only rules remain valid")
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

func testAlertCheckpointWrites(t *testing.T) {
	ctx := context.Background()
	db := model.DB.WithContext(ctx)
	now := time.Now().UnixMilli()
	makeFact := func(boot string, seq int64) model.AlertObservation {
		return model.AlertObservation{ID: model.AlertIdentity("checkpoint-fact", boot, seq), BootID: boot, SourceSequence: seq,
			Namespace: "health", SourceID: model.AlertIdentity("checkpoint-lifecycle", boot), Kind: "progress",
			Sequence: seq, Layer: "request", StartedAt: now, OccurredAt: now}
	}
	a, b := "batch-producer-a", "batch-producer-b"
	first, second := makeFact(a, 1), makeFact(a, 2)
	other := makeFact(b, 1)
	second.Kind = "finished"
	start, results := make(chan struct{}), make(chan error, 2)
	for _, batch := range [][]model.AlertObservation{{first, other, second}, {second, other, first, first}} {
		go func() {
			<-start
			results <- db.Transaction(func(tx *gorm.DB) error { return model.AcceptAlertObservationsTx(tx, batch) })
		}()
	}
	close(start)
	require.NoError(t, errors.Join(<-results, <-results))
	var sources []model.AlertWorkerCheckpoint
	require.NoError(t, db.Where("id IN ?", []string{a, b}).Order("id").Find(&sources).Error)
	require.Len(t, sources, 2)
	assert.EqualValues(t, 2, sources[0].AcceptedCount)
	assert.EqualValues(t, 1, sources[1].AcceptedCount)
	var lifecycle model.AlertLifecycle
	require.NoError(t, db.Where("id = ?", first.SourceID).Take(&lifecycle).Error)
	assert.Equal(t, now, lifecycle.FinishedAt)

	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		// A duplicate can finish on a second connection even while a heartbeat
		// holds the producer row. SQLite serializes every writer.
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", a).Update("captured_at", now).Error; err != nil {
				return err
			}
			replay, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			return model.AcceptAlertObservation(replay, first)
		}))
	}

	rollback := errors.New("roll back accepted batch")
	third := makeFact(a, 3)
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		if err := model.AcceptAlertObservationsTx(tx, []model.AlertObservation{third}); err != nil {
			return err
		}
		return rollback
	}), rollback)
	var count int64
	require.NoError(t, db.Model(&model.AlertObservation{}).Where("id = ?", third.ID).Count(&count).Error)
	assert.Zero(t, count)

	// Advance the heartbeat on a separate connection immediately after verification
	// reads its snapshot. A proof for the first two positions cannot certify three.
	require.NoError(t, db.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", a).
		Updates(map[string]any{"offered_sequence": 2, "captured_at": now, "healthy": true, "accepted_through": nil, "last_complete_at": nil}).Error)
	var superseded bool
	const callback = "test:supersede_alert_heartbeat"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		source, ok := tx.Statement.Dest.(*model.AlertWorkerCheckpoint)
		if !ok || source.ID != a || superseded {
			return
		}
		superseded = true
		tx.AddError(db.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", a).
			Updates(map[string]any{"offered_sequence": 3, "captured_at": now + 1}).Error)
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, a))
	require.True(t, superseded)
	var source model.AlertWorkerCheckpoint
	require.NoError(t, db.Where("id = ?", a).Take(&source).Error)
	assert.EqualValues(t, 2, source.AcceptedCount)
	assert.Zero(t, source.AcceptedThrough)
	assert.Zero(t, source.LastCompleteAt)
	require.NoError(t, model.AcceptAlertObservation(ctx, third))
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, a))
	require.NoError(t, db.First(&source).Error)
	assert.EqualValues(t, 3, source.AcceptedCount)
	assert.EqualValues(t, 3, source.AcceptedThrough)
	assert.Equal(t, now+1, source.LastCompleteAt)
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

func alertNotificationFixture(t *testing.T, condition model.AlertCondition, value int64, kind string) model.AlertEvent {
	t.Helper()
	start := time.Date(2026, 11, 15, 13, 6, 28, 0, time.UTC)
	config, err := common.Marshal(model.AlertRuleSpec{Name: "usapi-gemini", Severity: "P1", Condition: condition})
	require.NoError(t, err)
	evidence, err := common.Marshal(map[string]any{"measurement": alerting.Measurement{Value: value, Samples: 16, Status: "complete", WindowStart: start.UnixMilli(), WindowEnd: start.Add(time.Minute).UnixMilli()}})
	require.NoError(t, err)
	return model.AlertEvent{ID: 29, Kind: kind, TargetName: "usapi-gemini", RuleConfig: string(config), Evidence: string(evidence)}
}

func TestAlertNotificationPercentageAndTimezone(t *testing.T) {
	condition := model.AlertCondition{Metric: "success_rate", ThresholdBPS: 9500, RecoveryThresholdBPS: 9900, Window: model.AlertWindow{Timezone: "Asia/Shanghai"}}
	event := alertNotificationFixture(t, condition, 0, "trigger")
	for _, tc := range []struct{ language, line, basisPoints string }{
		{"en", "Success rate: 0%; threshold: 95%", "basis points"},
		{"zh-CN", "成功率：0%；阈值：95%", "基点"},
		{"zh-TW", "成功率：0%；閾值：95%", "基點"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			message := alerting.RenderNotification(tc.language, event)
			assert.Contains(t, message, tc.line)
			assert.NotContains(t, message, tc.basisPoints)
			assert.Contains(t, message, "2026-11-15T21:06:28+08:00")
			assert.Contains(t, message, "2026-11-15T21:07:28+08:00")
			assert.Contains(t, message, "Asia/Shanghai")
			assert.Contains(t, message, "/alerts?event_id=29")
		})
	}
	for _, tc := range []struct {
		metric, kind               string
		value, threshold, recovery int64
		want                       string
	}{
		{"success_rate", "trigger", 10000, 10000, 10000, "100%; threshold: 100%"},
		{"failure_rate", "trigger", 1234, 500, 200, "12.34%; threshold: 5%"},
		{"timeout_rate", "trigger", 1, 10, 0, "0.01%; threshold: 0.1%"},
		{"slow_request_rate", "trigger", 8001, 8000, 7000, "80.01%; threshold: 80%"},
		{"budget_remaining_ratio", "trigger", 1999, 2000, 3000, "19.99%; threshold: 20%"},
		{"success_rate", "recovery", 9900, 9500, 9900, "99%; threshold: 99%"},
	} {
		t.Run(tc.metric+"/"+tc.kind, func(t *testing.T) {
			event := alertNotificationFixture(t, model.AlertCondition{Metric: tc.metric, ThresholdBPS: tc.threshold, RecoveryThresholdBPS: tc.recovery}, tc.value, tc.kind)
			assert.Contains(t, alerting.RenderNotification("en", event), tc.want)
		})
	}
	for _, tc := range []struct{ metric, want string }{
		{"request_count", "Requests: 16; threshold: 20"},
		{"duration_ms", "Request duration: 16 ms; threshold: 200 ms"},
	} {
		t.Run(tc.metric, func(t *testing.T) {
			event := alertNotificationFixture(t, model.AlertCondition{Metric: tc.metric, ThresholdCount: 20, ThresholdMS: 200}, 16, "trigger")
			message := alerting.RenderNotification("en", event)
			assert.Contains(t, message, tc.want)
			assert.NotContains(t, message, "%")
		})
	}
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
					assert.Equal(t, "interactive", payload["msg_type"])
					assert.NotNil(t, payload["card"])
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

func feishuCardContents(value any) string {
	var text strings.Builder
	switch item := value.(type) {
	case map[string]any:
		if content, ok := item["content"].(string); ok {
			text.WriteString(content + "\n")
		}
		for key, child := range item {
			if key != "content" {
				text.WriteString(feishuCardContents(child))
			}
		}
	case []any:
		for _, child := range item {
			text.WriteString(feishuCardContents(child))
		}
	}
	return text.String()
}

func TestAlertFeishuCards(t *testing.T) {
	vault, err := alerting.NewTargetVault("cards", map[string]string{"cards": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))})
	require.NoError(t, err)
	secret := alerting.TargetSecret{Address: "https://1.1.1.1/fixture"}
	sealed, err := vault.Seal(1, "feishu", secret)
	require.NoError(t, err)
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://console.example"
	t.Cleanup(func() { system_setting.ServerAddress = previous })
	for _, tc := range []struct{ kind, severity, color, emoji, title string }{
		{"trigger", "P0", "red", "🚨", "异常触发"},
		{"trigger", "P1", "red", "🚨", "异常触发"},
		{"trigger", "P2", "orange", "🚨", "异常触发"},
		{"trigger", "P3", "blue", "🚨", "异常触发"},
		{"repeat", "P1", "red", "⏳", "异常持续"},
		{"recovery", "P1", "green", "✅", "条件恢复"},
		{"recovery_probe", "P1", "green", "🧪", "恢复探测合格"},
		{"escalation", "P1", "purple", "📣", "未确认升级"},
		{"unacknowledged", "P2", "orange", "⏰", "待人工确认"},
		{"milestone", "P2", "orange", "💰", "消费档位"},
		{"report", "P3", "blue", "📊", "使用汇总"},
		{"report_revision", "P3", "blue", "📝", "使用汇总修订"},
		{"report_data_gap", "P3", "yellow", "⚠️", "汇总数据覆盖不完整"},
		{"test", "P3", "blue", "🔔", "通知测试"},
		{"engine_health", "P1", "red", "🛠️", "告警服务异常"},
	} {
		t.Run(tc.kind+tc.severity, func(t *testing.T) {
			event := alertNotificationFixture(t, model.AlertCondition{Metric: "success_rate", Operator: "lt", ThresholdBPS: 9500, RecoveryThresholdBPS: 9900, Window: model.AlertWindow{Timezone: "Asia/Shanghai"}}, 0, tc.kind)
			event.Severity = tc.severity
			event.CreatedAt = time.Date(2026, 11, 15, 13, 7, 28, 0, time.UTC).UnixMilli()
			if tc.kind == "engine_health" {
				event.Summary = "delivery_backlog"
			}
			vault.Client = &http.Client{Transport: alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
				var payload map[string]any
				require.NoError(t, common.DecodeJson(request.Body, &payload))
				assert.Equal(t, "interactive", payload["msg_type"])
				card, ok := payload["card"].(map[string]any)
				require.True(t, ok)
				header, ok := card["header"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.color, header["template"])
				title, ok := header["title"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "plain_text", title["tag"])
				assert.Contains(t, title["content"], tc.emoji)
				assert.Contains(t, title["content"], tc.severity)
				assert.Contains(t, title["content"], tc.title)
				body, err := common.Marshal(card)
				require.NoError(t, err)
				assert.Contains(t, string(body), "https://console.example/alerts?event_id=29")
				assert.NotContains(t, feishuCardContents(card), "alert.")
				if tc.kind == "engine_health" {
					assert.Contains(t, feishuCardContents(card), "通知投递积压")
				}
				assert.NotContains(t, feishuCardContents(card), "1970-")
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
			}}}
			_, err := vault.Deliver(context.Background(), model.AlertTarget{OwnerUserID: 1, Type: "feishu", KeyID: "cards", Ciphertext: sealed, Language: "zh-CN"}, event)
			require.NoError(t, err)
		})
	}
}

func TestAlertFeishuCardUnitsAndObjects(t *testing.T) {
	vault, err := alerting.NewTargetVault("cards", map[string]string{"cards": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))})
	require.NoError(t, err)
	sealed, err := vault.Seal(1, "feishu", alerting.TargetSecret{Address: "https://1.1.1.1/fixture"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, language, kind, metric, object, want, threshold, icon string
		value                                                       int64
	}{
		{"model failure", "zh-CN", "trigger", "failure_rate", "channel_model", "12.34%", "5%", "🧩", 1234},
		{"channel recovery", "en", "recovery", "success_rate", "channel", "100%", "99%", "🌐", 10000},
		{"model recovery", "zh-TW", "recovery", "duration_ms", "model", "100 ms", "10000 ms", "🧩", 100},
		{"key spending", "zh-CN", "trigger", "spend_net", "api_key", "157.34 USD", "150 USD", "🔑", 157340000},
		{"account spending", "en", "recovery", "spend_net", "account", "10 USD", "20 USD", "👤", 10000000},
		{"system count", "en", "trigger", "request_count", "system", "16", "20", "⚙️", 16},
		{"traffic change", "zh-CN", "trigger", "request_volume_change", "channel_model", "500%", "50%", "🧩", 50000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			condition := model.AlertCondition{Metric: tc.metric, Operator: "gt", ThresholdBPS: 500, RecoveryThresholdBPS: 9900, ThresholdMS: 30000, RecoveryThresholdMS: 10000, ThresholdUSD: "150", RecoveryThresholdUSD: "20", ThresholdCount: 20, Window: model.AlertWindow{Timezone: "Asia/Shanghai"}}
			if tc.metric == "success_rate" {
				condition.Operator = "lt"
			}
			if tc.metric == "request_volume_change" {
				condition.ChangeDirection = "increase"
				condition.ThresholdBPS = 5000
			}
			event := alertNotificationFixture(t, condition, tc.value, tc.kind)
			spec := model.AlertRuleSpec{Name: "usapi-gemini model test", Severity: "P1", QuotaUnit: 1000000, Condition: condition}
			config, err := common.Marshal(spec)
			require.NoError(t, err)
			event.RuleConfig = string(config)
			target := alerting.MonitoredTarget{Type: tc.object, ID: 49, Name: "usapi-gemini", Model: "gemini-3.8-flash", OwnerUserID: 1}
			if tc.object == "channel_model" {
				target.Name += " / " + target.Model
			}
			measurement := alerting.Measurement{Value: tc.value, Samples: 16, Status: "complete", WindowStart: 1794741600000, WindowEnd: 1794741660000}
			encoded, err := common.Marshal(map[string]any{"target": target, "measurement": measurement})
			require.NoError(t, err)
			event.Evidence = string(encoded)
			vault.Client = &http.Client{Transport: alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
				var payload map[string]any
				require.NoError(t, common.DecodeJson(request.Body, &payload))
				text := feishuCardContents(payload["card"])
				assert.Contains(t, text, tc.want)
				assert.Contains(t, text, tc.threshold)
				assert.Contains(t, text, tc.icon)
				assert.NotContains(t, text, "alert.")
				if tc.object == "channel_model" || tc.object == "model" {
					assert.Contains(t, text, "gemini-3.8-flash")
				}
				if tc.metric == "success_rate" && tc.kind == "recovery" {
					assert.Contains(t, text, "≥ 99%")
				}
				if tc.kind == "recovery" && tc.metric == "duration_ms" {
					assert.Contains(t, text, "≤ 10000 ms")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
			}}}
			_, err = vault.Deliver(context.Background(), model.AlertTarget{OwnerUserID: 1, Type: "feishu", KeyID: "cards", Ciphertext: sealed, Language: tc.language}, event)
			require.NoError(t, err)
		})
	}
}

func TestAlertFeishuCardDetailsAndBounds(t *testing.T) {
	vault, err := alerting.NewTargetVault("cards", map[string]string{"cards": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))})
	require.NoError(t, err)
	sealed, err := vault.Seal(1, "feishu", alerting.TargetSecret{Address: "https://1.1.1.1/fixture"})
	require.NoError(t, err)
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://console.example"
	t.Cleanup(func() { system_setting.ServerAddress = previous })
	for _, kind := range []string{"report", "report_revision", "legacy_notice", "trigger", "recovery_probe", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			name := `<at id=all></at> **literal** [name](https://attacker.example)`
			spec := model.AlertRuleSpec{Name: name, Severity: "P2", QuotaUnit: 1000000, Condition: model.AlertCondition{Metric: "spend_net", ThresholdUSD: "150", Window: model.AlertWindow{Timezone: "Asia/Shanghai"}}}
			measurement := alerting.Measurement{Value: 744720000, Samples: 120, Status: "complete"}
			switch kind {
			case "report", "report_revision":
				measurement.Report = &alerting.ReportSummary{Totals: alerting.ReportUsage{Requests: 120, InputTokens: 12345}, Keys: []alerting.ReportUsage{{TokenName: "key2", TokenID: 2, Quota: 744720000, Requests: 120}}, Backfill: true}
			case "trigger":
				spec.Condition.Metric, spec.Condition.ChangeDirection = "request_volume_change", "increase"
				measurement.Value = 50000
				measurement.Traffic = &alerting.TrafficMeasurement{Count: 6, BaselineScaled: 10000, ChangeBPS: 50000}
			}
			config, err := common.Marshal(spec)
			require.NoError(t, err)
			evidence, err := common.Marshal(map[string]any{"target": alerting.MonitoredTarget{Type: "api_key", ID: 2, OwnerUserID: 1, Name: name}, "measurement": measurement})
			require.NoError(t, err)
			event := model.AlertEvent{ID: 29, Kind: kind, Severity: "P2", RuleConfig: string(config), Evidence: string(evidence), Summary: "Fallback notification"}
			if kind == "legacy_notice" {
				data, err := common.Marshal(alerting.LegacyNotice{ChannelID: 49, Title: "Slow channel error", Content: strings.Repeat("<&>模型💡", 10000)})
				require.NoError(t, err)
				event.Evidence = string(data)
			}
			if kind == "malformed" {
				event.RuleConfig, event.Evidence = "invalid", "invalid"
			}
			vault.Client = &http.Client{Transport: alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				assert.LessOrEqual(t, len(body), 20*1024)
				var payload map[string]any
				require.NoError(t, common.Unmarshal(body, &payload))
				card, ok := payload["card"].(map[string]any)
				require.True(t, ok)
				text := feishuCardContents(card)
				assert.NotContains(t, string(body), "lark_md", "user names and legacy content must not become card markup or @all mentions")
				assert.NotContains(t, text, "\ufffd", "truncation must preserve Unicode")
				assert.NotContains(t, text, "alert.card.")
				switch kind {
				case "report", "report_revision":
					assert.Contains(t, text, "744.72 USD")
					assert.Contains(t, text, "key2")
					assert.Contains(t, text, "12345")
					assert.Contains(t, text, "补发报告")
					assert.NotContains(t, text, "告警阈值")
					assert.Contains(t, text, name)
				case "trigger":
					assert.Contains(t, text, "500%")
					assert.Contains(t, text, "当前调用：6 次")
					assert.Contains(t, text, "基线：1 次")
				case "legacy_notice":
					assert.Contains(t, text, "Slow channel error")
					assert.Contains(t, text, "…")
				case "recovery_probe":
					assert.Contains(t, text, "恢复探测合格")
					assert.NotContains(t, text, "告警阈值")
				case "malformed":
					assert.Contains(t, text, "Fallback notification")
				}
				assert.Contains(t, string(body), "https://console.example/alerts?event_id=29")
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
			}}}
			_, err = vault.Deliver(context.Background(), model.AlertTarget{OwnerUserID: 1, Type: "feishu", KeyID: "cards", Ciphertext: sealed, Language: "zh-CN"}, event)
			require.NoError(t, err)
		})
	}
}

func TestLegacyFeishuDeliveryProtocolsAndBusinessErrors(t *testing.T) {
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	client := service.GetHttpClient()
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	for _, tc := range []struct {
		name, secret, body string
		status             int
		wantError          bool
	}{
		{name: "unsigned success", body: `{"code":0}`, status: 200},
		{name: "signed success", secret: "test-signing-secret", body: `{"code":0}`, status: 200},
		{name: "legacy response", body: `{"StatusCode":0}`, status: 200},
		{name: "business rejection", body: `{"code":19021,"msg":"private-response"}`, status: 200, wantError: true},
		{name: "legacy rejection", body: `{"StatusCode":19021}`, status: 200, wantError: true},
		{name: "missing code", body: `{}`, status: 200, wantError: true},
		{name: "empty body", status: 204, wantError: true},
		{name: "invalid JSON", body: `invalid-private-response`, status: 200, wantError: true},
		{name: "oversized body", body: `{"code":0}` + strings.Repeat(" ", 65536), status: 200, wantError: true},
		{name: "HTTP rejection", body: `{"code":0}`, status: 503, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client.Transport = alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPost, request.Method)
				assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
				assert.Empty(t, request.Header.Get("X-Webhook-Signature"))
				var payload map[string]any
				require.NoError(t, common.DecodeJson(request.Body, &payload))
				assert.Equal(t, "text", payload["msg_type"])
				assert.Equal(t, map[string]any{"text": "Test alert"}, payload["content"])
				if tc.secret == "" {
					assert.NotContains(t, payload, "timestamp")
					assert.NotContains(t, payload, "sign")
				} else {
					stamp, ok := payload["timestamp"].(string)
					require.True(t, ok)
					seconds, err := strconv.ParseInt(stamp, 10, 64)
					require.NoError(t, err)
					assert.Positive(t, seconds)
					mac := hmac.New(sha256.New, []byte(stamp+"\n"+tc.secret))
					assert.Equal(t, base64.StdEncoding.EncodeToString(mac.Sum(nil)), payload["sign"])
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}}
			err := service.SendFeishuNotifyContext(context.Background(), "https://notify.example/hook/private-token", tc.secret, dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: "Test alert"}})
			if tc.wantError {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "private-response")
				assert.NotContains(t, err.Error(), "private-token")
			} else {
				require.NoError(t, err)
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

func testAlertPeriodWatermark(t *testing.T) {
	ctx := context.Background()
	var source model.AlertWorkerCheckpoint
	var instance model.SystemInstance
	require.NoError(t, model.DB.First(&source, "id = ?", "alert-test-boot").Error)
	require.NoError(t, model.DB.First(&instance, "node_name = ?", "alert-test-node").Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Save(&source).Error)
		require.NoError(t, model.DB.Save(&instance).Error)
	})
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	for _, period := range []string{"hour", "day", "month"} {
		t.Run(period, func(t *testing.T) {
			boundary := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
			now := boundary.Add(2 * time.Second)
			row, spec, target := alertRuleFixture(t, "spend_net")
			token := model.Token{UserId: 1, Name: "Period " + period, Key: "period-" + period, Status: common.TokenStatusEnabled}
			require.NoError(t, model.DB.Create(&token).Error)
			target.ID, spec.Scope.EntityIDs = token.Id, []int{token.Id}
			spec.Condition.Window = model.AlertWindow{Type: period, Timezone: "Asia/Shanghai"}
			spec.Condition.MinSamples = 0
			spec.Trigger.Mode = "once_per_period"
			row.EffectiveAt = boundary.AddDate(0, -2, 0).UnixMilli()
			encoded, err := common.Marshal(spec)
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(&row).Updates(map[string]any{"effective_at": row.EffectiveAt, "config": string(encoded)}).Error)
			require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", instance.NodeName).Updates(map[string]any{"started_at": boundary.AddDate(0, -3, 0).Unix(), "last_seen_at": now.Unix()}).Error)
			require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", source.ID).Updates(map[string]any{"healthy": true, "usage_healthy": true, "updated_at": now.UnixMilli(), "captured_at": boundary.Add(-4 * time.Second).UnixMilli(), "coverage_started_at": boundary.AddDate(0, -3, 0).UnixMilli()}).Error)
			quota, err := alerting.USDQuota("200")
			require.NoError(t, err)
			fact := model.AlertObservation{ID: model.AlertIdentity("period-usage", period), Namespace: "usage", SourceID: "period-usage-" + period, Kind: "settlement", OccurredAt: boundary.Add(-time.Minute).UnixMilli(), UserID: 1, TokenID: token.Id, Quota: quota}
			require.NoError(t, model.AcceptAlertObservation(ctx, fact))
			engine := alerting.Engine{BootID: "period-worker"}
			previous := now.Add(-15 * time.Second)
			start, end, _, err := alerting.WindowBounds(row, spec.Condition, previous)
			require.NoError(t, err)
			require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: quota, Samples: 1, WindowStart: start, WindowEnd: end}, previous))
			m, err := alerting.Measure(ctx, row, spec, target, now)
			require.NoError(t, err)
			require.Equal(t, "complete", m.Status)
			require.Less(t, m.WindowEnd, boundary.UnixMilli())
			require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now))
			var events []model.AlertEvent
			require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "trigger").Find(&events).Error)
			assert.Len(t, events, 1, "late input must retain the previous period's deduplication")
			fact.ID, fact.SourceID, fact.OccurredAt = model.AlertIdentity("current-period-usage", period), "current-period-usage-"+period, boundary.Add(time.Second).UnixMilli()
			require.NoError(t, model.AcceptAlertObservation(ctx, fact))
			now = boundary.Add(10 * time.Second)
			require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", source.ID).Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
			m, err = alerting.Measure(ctx, row, spec, target, now)
			require.NoError(t, err)
			require.Equal(t, "complete", m.Status)
			require.Equal(t, boundary.UnixMilli(), m.WindowStart)
			require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now))
			require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "trigger").Find(&events).Error)
			assert.Len(t, events, 2, "the new period must still have its own trigger")
		})
	}
}

func testAlertRecurrentRecovery(t *testing.T) {
	ctx := context.Background()
	row, spec, target := alertRuleFixture(t, "duration_ms")
	spec.Condition.RecoverySeconds = 60
	engine := alerting.Engine{BootID: "recurrent-worker"}
	now := time.Now()
	for _, step := range []struct {
		seconds int
		value   int64
		status  string
	}{
		{0, 90000, "firing"},
		{15, 1000, "firing"},
		{75, 1000, "resolved"},
		{90, 90000, "firing"},
		{105, 1000, "firing"},
		{164, 1000, "firing"},
		{165, 1000, "resolved"},
	} {
		require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, alerting.Measurement{Status: "complete", Value: step.value, Samples: 10}, now.Add(time.Duration(step.seconds)*time.Second)))
		var state model.AlertRuleState
		require.NoError(t, model.DB.Where("rule_id = ?", row.ID).First(&state).Error)
		assert.Equal(t, step.status, state.Status, "at %d seconds", step.seconds)
		if step.status == "resolved" {
			assert.Zero(t, state.RecoveryAt)
		}
	}
	var incidents []model.AlertIncident
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&incidents).Error)
	require.Len(t, incidents, 2)
	for _, incident := range incidents {
		assert.Equal(t, "resolved", incident.Status)
		assert.Positive(t, incident.ResolvedAt)
	}
}

func testAlertOnceEscalation(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ mode, stop string }{{"once", ""}, {"once_per_period", ""}, {"once", "acknowledged_at"}, {"once", "closed_at"}} {
		t.Run(tc.mode+"/"+tc.stop, func(t *testing.T) {
			row, spec, target := alertRuleFixture(t, "duration_ms")
			spec.Trigger.Mode, spec.Acknowledgement.Required = tc.mode, true
			spec.Condition.Window = model.AlertWindow{Type: "day", Timezone: "Asia/Shanghai"}
			now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			engine := alerting.Engine{BootID: "once-worker"}
			m := alerting.Measurement{Status: "complete", Value: 90000, Samples: 10}
			for _, seconds := range []int{0, 301, 302} {
				require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now.Add(time.Duration(seconds)*time.Second)))
			}
			if tc.stop != "" {
				require.NoError(t, model.DB.Model(&model.AlertIncident{}).Where("rule_id = ?", row.ID).Update(tc.stop, now.Add(303*time.Second).UnixMilli()).Error)
			}
			for _, seconds := range []int{601, 602} {
				require.NoError(t, engine.ApplyMeasurement(ctx, row, spec, target, m, now.Add(time.Duration(seconds)*time.Second)))
			}
			for kind, want := range map[string]int64{"trigger": 1, "unacknowledged": 1, "escalation": 1} {
				if kind == "escalation" && tc.stop != "" {
					want = 0
				}
				var count int64
				require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND kind = ?", row.ID, kind).Count(&count).Error)
				assert.Equal(t, want, count, kind)
			}
		})
	}
}

func testAlertRealtimeCoverage(t *testing.T) {
	ctx := context.Background()
	oldEnabled, oldControls := common.AlertingEnabled, common.AlertingControlsEnabled
	common.AlertingEnabled, common.AlertingControlsEnabled = true, true
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled = oldEnabled, oldControls
		common.SetAlertHealthSink(nil)
	})
	baseURL := "https://custom-upstream.example.com"
	channel := model.Channel{Name: "Realtime custom endpoint", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-realtime", BaseURL: &baseURL, Group: "default", OtherSettings: "{}"}
	require.NoError(t, model.DB.Create(&channel).Error)
	token := model.Token{UserId: 1, Name: "Realtime coverage", Key: "realtime-coverage-key", Status: common.TokenStatusEnabled}
	require.NoError(t, model.DB.Create(&token).Error)
	var terminal common.AlertHealthEvent
	common.SetAlertHealthSink(func(_ context.Context, event common.AlertHealthEvent) error {
		if event.Kind == "finished" && event.Layer == "channel_attempt" {
			terminal = event
		}
		return nil
	})
	tracker := service.NewRealtimeAlertTracker(common.AlertHealthEvent{UserID: 1, TokenID: token.Id, ChannelID: channel.Id, Model: "gpt-realtime"})
	require.NoError(t, tracker.Admit(ctx, "response.create"))
	tracker.Observe(ctx, "response.done", "completed", false)
	require.NotEmpty(t, terminal.SourceID)
	require.NoError(t, model.AcceptAlertObservation(ctx, model.AlertObservation{ID: model.AlertIdentity("realtime-coverage", terminal.SourceID), Namespace: "health", SourceID: terminal.SourceID, Kind: terminal.Kind, Layer: terminal.Layer, Sequence: terminal.Sequence, StartedAt: terminal.StartedAt, OccurredAt: terminal.OccurredAt, UserID: terminal.UserID, ChannelID: terminal.ChannelID, ModelKey: terminal.Model, Protocol: terminal.Protocol, Stream: terminal.Stream, ConfigVersion: terminal.ConfigVersion, SampleClass: "production", Outcome: terminal.Outcome, DurationMS: terminal.DurationMS}))
	now := time.Now().Add(2 * time.Second)
	var source model.AlertWorkerCheckpoint
	require.NoError(t, model.DB.First(&source, "id = ?", "alert-test-boot").Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Save(&source).Error) })
	require.NoError(t, model.DB.Model(&source).Updates(map[string]any{"updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
	row := model.AlertRule{EffectiveAt: now.Add(-time.Hour).UnixMilli()}
	spec := model.AlertRuleSpec{Scope: model.AlertScope{Type: "channel_model", ChannelIDs: []int{channel.Id}, ModelSelector: model.AlertModelSelector{Models: []string{"gpt-realtime"}}}, Condition: model.AlertCondition{Metric: "duration_ms", Aggregation: "mean", LatencyBasis: "channel_attempt", MinSamples: 1, Window: model.AlertWindow{Type: "rolling", DurationSeconds: 60, Timezone: "Asia/Shanghai"}}}
	targets, err := alerting.ResolveTargets(ctx, row, spec)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	m, err := alerting.Measure(ctx, row, spec, targets[0], now)
	require.NoError(t, err)
	assert.Equal(t, "complete", m.Status)
	assert.EqualValues(t, 1, m.Samples, "custom endpoint responses must contribute to channel metrics")
	require.NoError(t, model.DB.Model(&channel).Update("base_url", "https://changed-upstream.example.com").Error)
	assert.ErrorContains(t, tracker.Admit(ctx, "response.create"), "configuration changed")
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

func testAlertRootTargets(t *testing.T) {
	ctx := context.Background()
	row, spec, _ := alertRuleFixture(t, "duration_ms")
	ownTargetID := spec.Notify.TargetIDs[0]
	var targets []model.AlertTarget
	for i := range 205 {
		targets = append(targets, model.AlertTarget{OwnerUserID: 2, Name: fmt.Sprintf("root-picker-%03d", i), Type: "email", Enabled: true, Revision: 1, Ciphertext: "private-fixture-ciphertext"})
	}
	require.NoError(t, model.DB.Create(&targets).Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("name LIKE ?", "root-picker-%").Delete(&model.AlertTarget{}).Error)
	})
	actorID, actorRole := 3, common.RoleRootUser
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", actorID); c.Set("role", actorRole); c.Next() })
	router.GET("/targets", controller.ListAlertTargets)
	list := func(path string, expected int) []model.AlertTarget {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, expected, response.Code, response.Body.String())
		assert.NotContains(t, response.Body.String(), "private-fixture-ciphertext")
		if expected != http.StatusOK {
			return nil
		}
		var result struct {
			Data []model.AlertTarget `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result.Data
	}
	first := list("/targets?scope=all&q=root-picker&limit=200", http.StatusOK)
	require.Len(t, first, 200)
	second := list(fmt.Sprintf("/targets?scope=all&q=root-picker&limit=200&before_id=%d", first[len(first)-1].ID), http.StatusOK)
	require.Len(t, second, 5)
	assert.Equal(t, targets[0].ID, second[len(second)-1].ID)
	matched := list(fmt.Sprintf("/targets?scope=all&q=%d", targets[0].ID), http.StatusOK)
	matchedIDs := make([]int64, len(matched))
	for i := range matched {
		matchedIDs[i] = matched[i].ID
	}
	assert.Contains(t, matchedIDs, targets[0].ID)
	assert.Len(t, list(fmt.Sprintf("/targets?scope=all&ids=%d,%d", targets[0].ID, targets[204].ID), http.StatusOK), 2)
	list("/targets?scope=all&ids=invalid", http.StatusBadRequest)
	list("/targets?scope=all&ids="+strings.TrimSuffix(strings.Repeat("1,", 21), ","), http.StatusBadRequest)
	spec.Notify.TargetIDs = []int64{targets[0].ID}
	spec.Acknowledgement.EscalationTargetIDs = []int64{targets[204].ID}
	spec.ManagementMode = "user_managed"
	saved, err := alerting.SaveRule(ctx, alerting.Actor{ID: 3, Role: common.RoleRootUser}, 1, row.ID, row.Revision, spec)
	require.NoError(t, err)
	stored, err := model.AlertRuleConfig(*saved)
	require.NoError(t, err)
	assert.Equal(t, "platform_enforced", stored.ManagementMode)
	assert.False(t, alerting.CanManage(alerting.Actor{ID: 1, Role: common.RoleCommonUser}, *saved))
	actorID, actorRole = 1, common.RoleCommonUser
	list("/targets?scope=all", http.StatusForbidden)
	assert.Empty(t, list(fmt.Sprintf("/targets?ids=%d", targets[0].ID), http.StatusOK))
	assert.Empty(t, list(fmt.Sprintf("/targets?q=%d", targets[0].ID), http.StatusOK))
	for _, escalation := range []bool{false, true} {
		candidate := spec
		if escalation {
			candidate.Notify.TargetIDs = []int64{ownTargetID}
		}
		err := alerting.Validate(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, &candidate)
		require.ErrorIs(t, err, alerting.ErrForbidden)
		var field *alerting.RuleTargetValidationError
		require.ErrorAs(t, err, &field)
		if escalation {
			assert.Equal(t, "acknowledgement.escalation_target_ids", field.Field)
		} else {
			assert.Equal(t, "notify.target_ids", field.Field)
		}
	}
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

func testAlertRuleList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var rows []model.AlertRule
	for i := range 205 {
		spec := model.AlertRuleSpec{Name: fmt.Sprintf("list-fixture-%03d", i), Severity: "P1", Enabled: true, ManagementMode: "platform_enforced", Scope: model.AlertScope{Type: "channel"}, Condition: model.AlertCondition{Metric: "success_rate"}}
		if i > 0 {
			spec.Severity, spec.Scope.Type, spec.Condition.Metric = "P2", "account", "request_count"
		}
		encoded, err := common.Marshal(spec)
		require.NoError(t, err)
		rows = append(rows, model.AlertRule{OwnerUserID: 1, Name: spec.Name, Enabled: true, Revision: 1, Generation: 1, Config: string(encoded)})
	}
	require.NoError(t, model.DB.Create(&rows).Error)
	paused := model.AlertRule{OwnerUserID: 1, Name: "list-fixture-paused", Config: rows[0].Config}
	foreign := model.AlertRule{OwnerUserID: 2, Name: "list-fixture-foreign", Enabled: true, Config: rows[0].Config}
	corrupt := model.AlertRule{OwnerUserID: 1, Name: "list-fixture-corrupt", Config: "invalid-json"}
	require.NoError(t, model.DB.Create(&paused).Error)
	require.NoError(t, model.DB.Create(&foreign).Error)
	require.NoError(t, model.DB.Create(&corrupt).Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Unscoped().Where("name LIKE ?", "list-fixture-%").Delete(&model.AlertRule{}).Error)
	})
	actorID, actorRole := 1, common.RoleCommonUser
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", actorID); c.Set("role", actorRole); c.Next() })
	router.GET("/rules", controller.ListAlertRules)
	type page struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	seen := map[int64]bool{}
	for number := 1; number <= 5; number++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/rules?page=%d&limit=50&q=LIST-FIXTURE&status=enabled", number), nil))
		require.Equal(t, http.StatusOK, response.Code)
		var result page
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success)
		assert.Equal(t, 205, result.Data.Total)
		assert.Equal(t, number, result.Data.Page)
		assert.Equal(t, 50, result.Data.PageSize)
		previous := int64(1 << 62)
		for _, row := range result.Data.Items {
			assert.False(t, seen[row.ID], "pages must not repeat rules")
			assert.Less(t, row.ID, previous)
			seen[row.ID], previous = true, row.ID
		}
	}
	assert.Len(t, seen, 205, "all rows beyond the old 200 limit must be reachable")
	for _, test := range []struct {
		query string
		total int
		id    int64
	}{
		{"q=list-fixture&status=enabled&scope_type=channel&metric=success_rate&severity=P1&management_mode=platform_enforced", 1, rows[0].ID},
		{"q=list-fixture&status=paused&scope_type=channel", 1, paused.ID},
		{fmt.Sprintf("q=%d", corrupt.ID), 1, corrupt.ID},
		{"q=missing", 0, 0},
		{"q=list-fixture-%25", 0, 0},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?page=9&limit=50&"+test.query, nil))
		require.Equal(t, http.StatusOK, response.Code)
		var result page
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.Equal(t, test.total, result.Data.Total)
		assert.Equal(t, 1, result.Data.Page)
		if test.total > 0 {
			require.Len(t, result.Data.Items, test.total)
			assert.Equal(t, test.id, result.Data.Items[0].ID)
		}
	}
	for _, query := range []string{"page=0", "page=bad", "page=1000001", "page=1&before_id=2", "page=1&limit=201", "page=1&status=bad", "page=1&scope_type=bad", "page=1&metric=bad", "page=1&severity=bad", "page=1&management_mode=bad"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?"+query, nil))
		assert.Equal(t, http.StatusBadRequest, response.Code, query)
	}
	for _, query := range []string{"scope=all", "owner_user_id=2"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?page=1&"+query, nil))
		assert.Equal(t, http.StatusForbidden, response.Code)
	}
	actorID, actorRole = 3, common.RoleRootUser
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rules?page=1&scope=all&q=list-fixture&status=enabled", nil))
	var all page
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &all))
	assert.Equal(t, 206, all.Data.Total)
	actorID, actorRole = 1, common.RoleCommonUser
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/rules?limit=2&before_id=%d&q=list-fixture&status=enabled", rows[204].ID), nil))
	var legacy struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &legacy))
	require.Len(t, legacy.Data, 2)
	assert.Equal(t, rows[203].ID, legacy.Data[0].ID)
}

func TestAlertTargetDevelopmentNetworkValidation(t *testing.T) {
	for _, environment := range []string{"dev", "", "production", "DEV", "development"} {
		t.Run(fmt.Sprintf("ENV=%q", environment), func(t *testing.T) {
			t.Setenv("ENV", environment)
			client := alerting.NewTargetHTTPClient()
			t.Cleanup(client.CloseIdleConnections)
			transport := client.Transport.(*http.Transport)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for _, host := range []string{"127.0.0.1", "10.0.0.7", "169.254.169.254", "198.18.0.38", "::1", "2001:2::26"} {
				address := "https://" + net.JoinHostPort(host, "443") + "/hook"
				_, err := alerting.ValidateTargetAddress(context.Background(), "feishu", alerting.TargetSecret{Address: address})
				if environment == "dev" {
					assert.NoError(t, err, address)
				} else {
					assert.ErrorIs(t, err, alerting.ErrTargetRejected, address)
				}
				// A cancelled context lets us verify the actual connection policy
				// without connecting to a private service or requiring port 443.
				connection, err := transport.DialContext(ctx, "tcp", net.JoinHostPort(host, "443"))
				if connection != nil {
					_ = connection.Close()
				}
				if environment == "dev" {
					assert.ErrorIs(t, err, context.Canceled, address)
				} else {
					assert.ErrorIs(t, err, alerting.ErrTargetRejected, address)
				}
			}
		})
	}

	t.Run("dev validates and delivers without a DNS preflight", func(t *testing.T) {
		t.Setenv("ENV", "dev")
		var lookups atomic.Int64
		previousResolver := net.DefaultResolver
		net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
			lookups.Add(1)
			return nil, errors.New("DNS is unavailable")
		}}
		t.Cleanup(func() { net.DefaultResolver = previousResolver })
		secret := alerting.TargetSecret{Address: "https://unresolvable-target.invalid/hook?key=do-not-echo"}
		hint, err := alerting.ValidateTargetAddress(context.Background(), "feishu", secret)
		require.NoError(t, err)
		assert.Equal(t, "https://unresolvable-target.invalid/…", hint)
		vault, err := alerting.NewTargetVault("test-key", map[string]string{"test-key": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
		require.NoError(t, err)
		sealed, err := vault.Seal(1, "feishu", secret)
		require.NoError(t, err)
		target := model.AlertTarget{ID: 17, OwnerUserID: 1, Type: "feishu", Revision: 1, KeyID: "test-key", Ciphertext: sealed, Language: "en"}
		var requests int
		vault.Client = &http.Client{Transport: alertHTTPTransport{send: func(request *http.Request) (*http.Response, error) {
			requests++
			assert.Equal(t, secret.Address, request.URL.String())
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
		}}}
		status, err := vault.Deliver(context.Background(), target, model.AlertEvent{ID: 1, Kind: "test", Summary: "Local delivery test"})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, 1, requests)
		assert.Zero(t, lookups.Load())
	})

	t.Run("dev retains URL format and required credential validation", func(t *testing.T) {
		t.Setenv("ENV", "dev")
		for _, test := range []struct{ kind, address string }{
			{"feishu", "http://127.0.0.1/hook"},
			{"feishu", "https://127.0.0.1:8443/hook"},
			{"feishu", "https://user:password@127.0.0.1/hook"},
			{"feishu", "https://127.0.0.1/hook#fragment"},
			{"unknown", "https://127.0.0.1/hook"},
			{"gotify", "https://127.0.0.1/hook"},
			{"email", "invalid-email"},
		} {
			_, err := alerting.ValidateTargetAddress(context.Background(), test.kind, alerting.TargetSecret{Address: test.address})
			assert.ErrorIs(t, err, alerting.ErrTargetRejected, test.address)
		}
	})
}

func TestAlertTargetCredentialBinding(t *testing.T) {
	t.Setenv("ENV", "production")
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
	t.Setenv("ENV", "production")
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

func testAlertTargetTestCooldown(t *testing.T) {
	previousEnabled, previousVault := common.AlertingEnabled, service.AlertTargetVault
	common.AlertingEnabled = true
	t.Cleanup(func() {
		common.AlertingEnabled, service.AlertTargetVault = previousEnabled, previousVault
	})
	vault, err := alerting.NewTargetVault("test-key", map[string]string{"test-key": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))})
	require.NoError(t, err)
	service.AlertTargetVault = vault
	sealed, err := vault.Seal(1, "email", alerting.TargetSecret{Address: "test@example.com"})
	require.NoError(t, err)
	target := model.AlertTarget{OwnerUserID: 1, Name: "Test cooldown", Type: "email", Enabled: true, Revision: 1, Ciphertext: sealed, KeyID: "test-key", Language: "en", LastTestAt: time.Now().Add(-61 * time.Second).UnixMilli()}
	require.NoError(t, model.DB.Create(&target).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleCommonUser); c.Next() })
	router.POST("/targets/:id/test", controller.TestAlertTarget)
	testTarget := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/targets/%d/test", target.ID), nil))
		return response
	}

	first := testTarget()
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var accepted model.AlertTarget
	require.NoError(t, model.DB.First(&accepted, target.ID).Error)
	second := testTarget()
	require.Equal(t, http.StatusTooManyRequests, second.Code, second.Body.String())
	var result struct {
		Success    bool   `json:"success"`
		MessageKey string `json:"message_key"`
	}
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &result))
	assert.False(t, result.Success)
	assert.Equal(t, "Please wait 60 seconds between notification tests.", result.MessageKey)
	retryAfter, err := strconv.Atoi(second.Header().Get("Retry-After"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, retryAfter, 1)
	assert.LessOrEqual(t, retryAfter, 60)
	var current model.AlertTarget
	require.NoError(t, model.DB.First(&current, target.ID).Error)
	assert.Equal(t, accepted.LastTestAt, current.LastTestAt, "rejected requests must not extend the cooldown")
	assert.Equal(t, accepted.Revision, current.Revision)
	var jobs, audits int64
	require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("target_id = ?", target.ID).Count(&jobs).Error)
	require.NoError(t, model.DB.Model(&model.AlertAudit{}).Where("object_id = ? AND operation = ?", target.ID, "target.test").Count(&audits).Error)
	assert.EqualValues(t, 1, jobs)
	assert.EqualValues(t, 1, audits)

	for _, test := range []struct {
		name, column, message string
		value                 any
		status                int
	}{
		{"concurrent test", "last_test_at", "Please wait 60 seconds between notification tests.", time.Now().UnixMilli(), http.StatusTooManyRequests},
		{"concurrent edit", "revision", "Alert configuration changed. Please reload.", gorm.Expr("revision + 1"), http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, model.DB.Model(&target).Update("last_test_at", time.Now().Add(-61*time.Second).UnixMilli()).Error)
			var changed atomic.Bool
			require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register("alert_target_concurrent_change", func(tx *gorm.DB) {
				row, ok := tx.Statement.Dest.(*model.AlertTarget)
				if !ok || row.ID != target.ID || !changed.CompareAndSwap(false, true) {
					return
				}
				require.NoError(t, model.DB.Model(&target).UpdateColumn(test.column, test.value).Error)
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove("alert_target_concurrent_change")) })
			response := testTarget()
			assert.True(t, changed.Load())
			require.Equal(t, test.status, response.Code, response.Body.String())
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, test.message, result.MessageKey)
			if test.status == http.StatusTooManyRequests {
				assert.NotEmpty(t, response.Header().Get("Retry-After"))
			} else {
				assert.Empty(t, response.Header().Get("Retry-After"))
			}
			require.NoError(t, model.DB.Model(&model.AlertJob{}).Where("target_id = ?", target.ID).Count(&jobs).Error)
			assert.EqualValues(t, 1, jobs, "conflicting requests must not enqueue another test")
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
	require.NoError(t, model.DB.Where("rule_id = ?", rule.ID).Order("id").Find(&events).Error)
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
	assert.True(t, configured[92])
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
	payload := `{"items":[{"kind":"token_spend","object_id":"91"},{"kind":"token_spend","object_id":"92"},{"kind":"unknown","object_id":"site"},{"kind":"user_notifications","object_id":"91"}]}`
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
					Status    int                  `json:"status"`
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
		require.Len(t, result.Data.Items, 4)
		assert.True(t, result.Data.Items[0].Success)
		require.True(t, result.Data.Items[1].Success, "existing private destinations must be importable")
		assert.False(t, result.Data.Items[2].Success)
		assert.Equal(t, http.StatusBadRequest, result.Data.Items[2].Status)
		assert.True(t, result.Data.Items[3].Success, "one failure does not prevent later imports")
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
		var privateTarget model.AlertTarget
		require.NoError(t, model.DB.Where("owner_user_id = ? AND type = ?", 92, "feishu").First(&privateTarget).Error)
		secret, err := vault.Open(privateTarget)
		require.NoError(t, err)
		assert.Equal(t, "https://127.0.0.1/batch-private-secret", secret.Address)
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
	for _, option := range []model.Option{{Key: "quota_alert_user_cumulative_milestone", Value: string(encoded)}, {Key: "uid_quota_warning_robot", Value: "https://198.18.0.7/account-bot"}} {
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
	assert.NotContains(t, migration.Checkpoint, "198.18.0.7")
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
	var delivery model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ? AND status = ?", rule.ID, "pending").Take(&delivery).Error)
	require.NoError(t, model.DB.Model(&delivery).Update("available_at", time.Now().Add(-time.Second).UnixMilli()).Error)
	// Other fixtures share this queue. One worker batch can finish before the report is claimed.
	deliveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for delivery.Status == "pending" {
		require.NoError(t, engine.RunJobs(deliveryCtx, "notify", time.Now()))
		require.NoError(t, model.DB.WithContext(deliveryCtx).Take(&delivery).Error)
	}
	require.Equal(t, "succeeded", delivery.Status)
	// Expire the delivered report before switching back to the old executor.
	for {
		more, err := model.CleanAlertLogBatch(ctx, future.AddDate(0, 0, 4))
		require.NoError(t, err)
		if !more {
			break
		}
	}
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND kind = ?", rule.ID, "report").Count(&count).Error)
	require.Zero(t, count)
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
	configs := map[string]*model.LegacyKeyQuotaUserConfig{"1": {FeishuRobotURL: "http://10.0.0.7:8080/key-bot", OneHour: []model.LegacyKeyQuotaRule{{QuotaUSD: 100, Action: "warn"}, {QuotaUSD: 200, Action: "warn"}}, OneDay: []model.LegacyKeyQuotaRule{{QuotaUSD: 500, Action: "disable_key"}}}, "2": {FeishuRobotURL: "https://8.8.8.8/other-key-bot"}}
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
	require.NoError(t, model.DB.Model(&option).Update("value", "https://127.0.0.1/uid-events").Error)
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
	// Terminal work cannot be drained or retried. Retain both records while
	// switching back to the legacy executor; only live work blocks rollback.
	var finishedJobs []model.AlertJob
	require.NoError(t, model.DB.Where("rule_id = ?", rule.ID).Find(&finishedJobs).Error)
	require.GreaterOrEqual(t, len(finishedJobs), 2)
	for i, job := range finishedJobs {
		status := "expired"
		if i%2 == 1 {
			status = "canceled"
		}
		require.NoError(t, model.DB.Model(&job).Update("status", status).Error)
	}
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

func testAlertChannelArrearsMigration(t *testing.T) {
	ctx := context.Background()
	previousVault, previousControls := service.AlertTargetVault, common.AlertingControlsEnabled
	common.AlertingControlsEnabled = true
	t.Cleanup(func() {
		service.AlertTargetVault, common.AlertingControlsEnabled = previousVault, previousControls
	})
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))})
	require.NoError(t, err)
	service.AlertTargetVault = vault
	option := model.Option{Key: "feishu_qianfei_webhook_url"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: option.Key}).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 3); c.Set("role", common.RoleRootUser); c.Next() })
	router.POST("/migrations", controller.ImportAlertMigration)
	importTarget := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/migrations", strings.NewReader(`{"kind":"channel_arrears","object_id":"site"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	var result struct {
		Success    bool                 `json:"success"`
		MessageKey string               `json:"message_key"`
		Reason     string               `json:"reason"`
		Data       model.AlertMigration `json:"data"`
	}
	for _, address := range []string{"", " \t "} {
		require.NoError(t, model.DB.Model(&option).Update("value", address).Error)
		response := importTarget()
		require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "Configure a legacy notification destination before importing.", result.MessageKey)
		assert.Equal(t, "migration_destination_missing", result.Reason)
		var migrations, targets int64
		require.NoError(t, model.DB.Model(&model.AlertMigration{}).Where("kind = ? AND object_id = ?", "channel_arrears", "site").Count(&migrations).Error)
		require.NoError(t, model.DB.Model(&model.AlertTarget{}).Where("name = ?", "Imported channel_arrears").Count(&targets).Error)
		assert.Zero(t, migrations)
		assert.Zero(t, targets)
	}
	address := "https://198.18.0.38/channel-balance?key=legacy-secret"
	require.NoError(t, model.DB.Model(&option).Update("value", address).Error)
	require.NoError(t, model.DB.Save(&model.Option{Key: "feishu_qianfei_secret", Value: "channel-signature"}).Error)
	snapshot, err := alerting.LegacyNoticeSnapshot(ctx, "channel_arrears", "site")
	require.NoError(t, err)
	assert.True(t, snapshot.DestinationPresent)
	response := importTarget()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success)
	migration := result.Data
	assert.Equal(t, "channel_arrears", migration.Kind)
	assert.Equal(t, "site", migration.ObjectID)
	assert.Equal(t, "legacy", migration.Executor)
	assert.Equal(t, "observing", migration.Status)
	var rule model.AlertRule
	require.NoError(t, model.DB.First(&rule, migration.RuleID).Error)
	spec, err := model.AlertRuleConfig(rule)
	require.NoError(t, err)
	assert.True(t, spec.Enabled)
	assert.Equal(t, "channel_arrears", spec.Condition.LegacySignal)
	require.Len(t, spec.Notify.TargetIDs, 1)
	var target model.AlertTarget
	require.NoError(t, model.DB.First(&target, spec.Notify.TargetIDs[0]).Error)
	secret, err := vault.Open(target)
	require.NoError(t, err)
	assert.Equal(t, address, secret.Address)
	assert.Equal(t, "channel-signature", secret.Secret)
	response = importTarget()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, migration.ID, result.Data.ID)
	assert.Equal(t, migration.RuleID, result.Data.RuleID)
	var targets int64
	require.NoError(t, model.DB.Model(&model.AlertTarget{}).Where("name = ?", "Imported channel_arrears").Count(&targets).Error)
	assert.EqualValues(t, 1, targets)
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

func testAlertActionPresentationEdits(t *testing.T) {
	ctx := context.Background()
	oldActions, oldShadow, oldControls := common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled
	common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = true, false, true
	t.Cleanup(func() {
		common.AlertingActionsEnabled, common.AlertingShadowMode, common.AlertingControlsEnabled = oldActions, oldShadow, oldControls
	})
	for i, tc := range []struct {
		name, status, edit, expected string
		reauthorize                  bool
	}{
		{"pending rename", "pending", "presentation", "succeeded", true},
		{"retry delivery edit", "retry_wait", "presentation", "succeeded", true},
		{"claimed action gets a fresh lease", "running", "presentation", "succeeded", true},
		{"expired work is not revived", "pending", "expired", "expired", false},
		{"canceled work stays terminal", "canceled", "presentation", "canceled", false},
		{"changed condition requires a new event", "pending", "condition", "canceled", false},
		{"changed action is not reauthorized", "pending", "action", "canceled", false},
		{"paused rule withdraws authorization", "pending", "pause", "canceled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			// Keep source coverage explicit even when the full database matrix
			// takes longer than a producer heartbeat lease on a slow machine.
			require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Updates(map[string]any{"healthy": true, "updated_at": now.UnixMilli(), "captured_at": now.Add(-time.Second).UnixMilli()}).Error)
			require.NoError(t, model.DB.Model(&model.SystemInstance{}).Where("node_name = ?", "alert-test-node").Update("last_seen_at", now.Unix()).Error)
			token := model.Token{Id: 55000 + i, UserId: 1, Name: tc.name, Key: fmt.Sprintf("presentation-action-%d", i), Status: common.TokenStatusEnabled}
			require.NoError(t, model.DB.Create(&token).Error)
			row, spec, target := alertRuleFixture(t, "request_count")
			target.ID, target.Name = token.Id, token.Name
			spec.Scope.EntityIDs = []int{token.Id}
			spec.Condition.Operator, spec.Condition.ThresholdCount, spec.Condition.RecoveryThresholdCount = "gte", 1, 0
			spec.Condition.MinSamples = 1
			spec.Actions = []model.AlertAction{{Type: "disable_api_key", Target: "matched_entity"}}
			saved, err := alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, row.ID, row.Revision, spec)
			require.NoError(t, err)
			saved.EffectiveAt = now.Add(-10 * time.Minute).UnixMilli()
			require.NoError(t, model.DB.Model(saved).Update("effective_at", saved.EffectiveAt).Error)
			spec, err = model.AlertRuleConfig(*saved)
			require.NoError(t, err)
			fact := model.AlertObservation{ID: model.AlertIdentity("presentation-action", i), SourceID: fmt.Sprintf("presentation-action-life-%d", i), Namespace: "health", Kind: "finished", Layer: "request", Sequence: 1, OccurredAt: now.Add(-time.Minute).UnixMilli(), StartedAt: now.Add(-2 * time.Minute).UnixMilli(), UserID: 1, TokenID: token.Id, SampleClass: "production", Outcome: "success", AttributionCovered: true}
			require.NoError(t, model.AcceptAlertObservation(ctx, fact))
			measured, err := alerting.Measure(ctx, *saved, spec, target, now)
			require.NoError(t, err)
			require.Equal(t, "complete", measured.Status, measured.Reason)
			require.EqualValues(t, 1, measured.Value)
			engine := alerting.Engine{BootID: "presentation-actions"}
			require.NoError(t, engine.ApplyMeasurement(ctx, *saved, spec, target, measured, now))
			var job model.AlertJob
			require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", saved.ID, "action").First(&job).Error)
			if tc.status == "running" {
				claimed, err := model.ClaimAlertJob(ctx, "action", "old-action-worker", now.UnixMilli())
				require.NoError(t, err)
				require.NotNil(t, claimed)
				require.Equal(t, job.ID, claimed.ID)
				job = *claimed
			} else {
				job.Status, job.Attempts = tc.status, 2
				require.NoError(t, model.DB.Model(&job).Updates(map[string]any{"status": job.Status, "attempts": job.Attempts}).Error)
			}
			if tc.edit == "expired" {
				job.ExpiresAt = now.Add(-time.Second).UnixMilli()
				require.NoError(t, model.DB.Model(&job).Update("expires_at", job.ExpiresAt).Error)
			}
			spec.Name = "Edited " + tc.name
			spec.Severity, spec.Notify.OnRecovery, spec.Acknowledgement.RemindAfterSeconds = "P2", false, 60
			switch tc.edit {
			case "condition":
				spec.Condition.ThresholdCount = 10
			case "action":
				spec.Actions = nil
			case "pause":
				spec.Enabled = false
			}
			if i == 0 {
				// Presentation edits still authorize enabled actions through the
				// existing HTTP step-up flow; the internal save is not a bypass.
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleCommonUser); c.Next() })
				router.PATCH("/rules/:id", controller.SaveAlertRule)
				body, err := common.Marshal(map[string]any{"config": spec, "owner_user_id": 1, "expected_revision": saved.Revision})
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/rules/%d", saved.ID), bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
				var untouched model.AlertRule
				require.NoError(t, model.DB.First(&untouched, saved.ID).Error)
				assert.Equal(t, saved.Revision, untouched.Revision)
			}
			_, err = alerting.SaveRule(ctx, alerting.Actor{ID: 2, Role: common.RoleCommonUser}, 1, saved.ID, saved.Revision, spec)
			require.ErrorIs(t, err, alerting.ErrForbidden)
			edited, err := alerting.SaveRule(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 1, saved.ID, saved.Revision, spec)
			require.NoError(t, err)
			var current model.AlertJob
			require.NoError(t, model.DB.First(&current, job.ID).Error)
			assert.Equal(t, job.EventID, current.EventID)
			assert.Equal(t, job.DedupKey, current.DedupKey)
			assert.Equal(t, job.ExpiresAt, current.ExpiresAt)
			assert.Equal(t, job.Attempts, current.Attempts)
			assert.Equal(t, job.AvailableAt, current.AvailableAt)
			if tc.reauthorize {
				assert.Equal(t, saved.Generation, edited.Generation)
				assert.Equal(t, edited.Revision, current.RuleRevision)
				assert.Empty(t, current.LockedBy)
				assert.Zero(t, current.LockedUntil)
				var audits, outbox int64
				require.NoError(t, model.DB.Model(&model.AlertAudit{}).Where("operation = ? AND object_id = ? AND revision = ?", "action.reauthorize", job.ID, edited.Revision).Count(&audits).Error)
				require.NoError(t, model.DB.Model(&model.AlertOutbox{}).Where("id = ?", model.AlertIdentity("action-reauthorize", job.ID, edited.Revision)).Count(&outbox).Error)
				assert.EqualValues(t, 1, audits)
				assert.EqualValues(t, 1, outbox)
			} else {
				assert.Equal(t, saved.Revision, current.RuleRevision)
			}
			if tc.status == "running" {
				var event model.AlertEvent
				require.NoError(t, model.DB.First(&event, job.EventID).Error)
				require.ErrorIs(t, engine.ExecuteAction(ctx, job, *saved, spec, target, event), model.ErrAlertConflict, "the old revision cannot commit after reauthorization")
				stale := model.DB.Model(&model.AlertJob{}).Where("id = ? AND status = ? AND locked_by = ? AND locked_until = ?", job.ID, "running", job.LockedBy, job.LockedUntil).Update("status", "canceled")
				require.NoError(t, stale.Error)
				assert.Zero(t, stale.RowsAffected, "old completion cannot overwrite requeued work")
			}
			require.NoError(t, engine.RunJobs(ctx, "action", time.Now()))
			require.NoError(t, model.DB.First(&current, job.ID).Error)
			assert.Equal(t, tc.expected, current.Status, current.LastError)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			if tc.expected == "succeeded" {
				assert.Equal(t, common.TokenStatusDisabled, token.Status)
			} else {
				assert.Equal(t, common.TokenStatusEnabled, token.Status)
			}
			var holds int64
			require.NoError(t, model.DB.Model(&model.AlertDisableHold{}).Where("rule_id = ?", row.ID).Count(&holds).Error)
			if tc.expected == "succeeded" {
				assert.EqualValues(t, 1, holds)
			} else {
				assert.Zero(t, holds)
			}
		})
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
	var lookups atomic.Int64
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		lookups.Add(1)
		return nil, errors.New("DNS is unavailable during migration")
	}}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	for _, item := range []struct {
		setting map[string]any
		kind    string
		feishu  bool
	}{
		{map[string]any{"notify_type": "email", "notification_email": "legacy@example.com"}, "email", false},
		{map[string]any{"notify_type": "webhook", "webhook_url": "https://legacy-alert.invalid/generic", "webhook_secret": "old-secret"}, "webhook_v1", false},
		{map[string]any{"notify_type": "webhook", "webhook_url": "https://127.0.0.1/bot", "webhook_secret": "old-secret"}, "feishu", true},
		{map[string]any{"notify_type": "bark", "bark_url": "https://198.18.0.7/key/{{title}}/{{content}}"}, "bark", false},
		{map[string]any{"notify_type": "gotify", "gotify_url": "http://10.0.0.7:8080", "gotify_token": "old-token", "gotify_priority": 7}, "gotify", false},
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
		switch item.kind {
		case "email":
			assert.Equal(t, item.setting["notification_email"], secret.Address)
		case "webhook_v1", "feishu":
			assert.Equal(t, item.setting["webhook_url"], secret.Address)
			assert.Equal(t, "old-secret", secret.Secret)
		case "bark":
			assert.Equal(t, item.setting["bark_url"], secret.Address)
		case "gotify":
			assert.Equal(t, item.setting["gotify_url"], secret.Address)
		}
		if item.kind == "gotify" {
			assert.Equal(t, "old-token", secret.Token)
			assert.Equal(t, 7, secret.Priority)
		}
		var unchanged model.User
		require.NoError(t, model.DB.Select("id", "setting").First(&unchanged, 2).Error)
		assert.Equal(t, string(setting), unchanged.Setting)
		if item.kind == "feishu" {
			_, err = rotated.Save(ctx, root, 2, 0, alerting.TargetInput{Name: "New target", Type: "feishu", Enabled: true, Credentials: &secret})
			assert.ErrorIs(t, err, alerting.ErrTargetRejected, "new target validation remains independent of migration")
			_, err = rotated.Deliver(ctx, *first, model.AlertEvent{Kind: "test"})
			assert.ErrorIs(t, err, alerting.ErrTargetRejected, "import must not bypass delivery-time network checks")
		}
	}
	assert.Zero(t, lookups.Load(), "legacy imports must not depend on DNS availability")
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

func testAlertLegacyChannelNotifications(t *testing.T) {
	require.NoError(t, i18n.Init())
	ctx := context.Background()
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	oldEnabled, oldControls, oldShadow, oldDisable, oldErrorLog := common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled
	oldOptions := common.OptionMap
	common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode = true, true, false
	common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = false, false
	common.OptionMap = map[string]string{"feishu_qianfei_webhook_url": "https://127.0.0.1/local-notice"}
	t.Cleanup(func() {
		common.AlertingEnabled, common.AlertingControlsEnabled, common.AlertingShadowMode, common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = oldEnabled, oldControls, oldShadow, oldDisable, oldErrorLog
		common.OptionMap = oldOptions
	})
	// Production cooldowns are process-wide; each database uses distinct channel
	// identities so one engine fixture cannot suppress another engine's notice.
	base := map[string]int{"sqlite": 900, "mysql": 1900, "postgres": 2900}[model.DB.Dialector.Name()]
	for _, id := range []int{base + 1, base + 2, base + 3, base + 4} {
		require.NoError(t, model.DB.Create(&model.Channel{Id: id, Name: fmt.Sprintf("local-notice-%d", id), Type: 24, Status: common.ChannelStatusEnabled, Models: "model-a"}).Error)
	}
	vault, err := alerting.NewTargetVault("active", map[string]string{"active": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))})
	require.NoError(t, err)
	option := model.Option{Key: "feishu_qianfei_webhook_url"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: option.Key}).Error)
	require.NoError(t, model.DB.Model(&option).Update("value", common.OptionMap[option.Key]).Error)
	migration, err := alerting.ImportLegacyNotice(ctx, root, vault, "channel_arrears", "site")
	require.NoError(t, err)
	var row model.AlertRule
	require.NoError(t, model.DB.First(&row, migration.RuleID).Error)
	spec, err := model.AlertRuleConfig(row)
	require.NoError(t, err)
	spec.Scope.ChannelIDs = []int{base + 3, base + 1, base + 4, base + 1}
	require.NoError(t, alerting.Validate(ctx, root, 0, &spec))
	assert.Equal(t, []int{base + 1, base + 3, base + 4}, spec.Scope.ChannelIDs)
	for _, invalid := range []struct {
		signal   string
		channels []int
	}{{"channel_arrears", []int{999999}}, {"uid_day", []int{base + 1}}} {
		bad := spec
		bad.Condition.LegacySignal, bad.Scope.ChannelIDs = invalid.signal, invalid.channels
		assert.ErrorIs(t, alerting.Validate(ctx, root, 0, &bad), alerting.ErrInvalid)
	}
	saved, err := alerting.SaveRule(ctx, root, 0, row.ID, row.Revision, spec)
	require.NoError(t, err)
	snapshot, err := alerting.LegacyNoticeSnapshot(ctx, migration.Kind, migration.ObjectID)
	require.NoError(t, err)
	if migration.Executor != "unified" {
		require.NoError(t, alerting.SwitchLegacyNotice(ctx, root, migration.ID, migration.Revision, "cutover", snapshot.SnapshotHash, saved.Revision))
	}
	for _, test := range []struct {
		id, status int
		message    string
		events     int64
	}{
		{base + 1, 402, "Insufficient Balance api_key:must-be-masked", 1},
		{base + 2, 403, "Access denied. https://help.aliyun.com/zh/model-studio/error-code#access-denied", 0},
		{base + 3, 403, "Access denied. https://help.aliyun.com/zh/model-studio/error-code#access-denied", 1},
		{base + 4, 403, "Access denied: model permission is missing", 0},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		apiErr := types.NewOpenAIError(errors.New(test.message), types.ErrorCodeBadResponseStatusCode, test.status)
		channel := types.ChannelError{ChannelId: test.id, ChannelName: fmt.Sprintf("local-notice-%d", test.id), AutoBan: true}
		service.ProcessChannelError(c, channel, apiErr, nil, "", 0, false)
		var count int64
		require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND target_name = ?", row.ID, channel.ChannelName).Count(&count).Error)
		assert.Equal(t, test.events, count, test.message)
		var loaded model.Channel
		require.NoError(t, model.DB.First(&loaded, test.id).Error)
		assert.Equal(t, common.ChannelStatusEnabled, loaded.Status, "sending a notification must not enable automatic channel disablement")
	}
	var event model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ? AND target_name = ?", row.ID, fmt.Sprintf("local-notice-%d", base+1)).First(&event).Error)
	assert.NotContains(t, event.Evidence, "must-be-masked")
	assert.Contains(t, event.Evidence, fmt.Sprintf(`"channel_id":%d`, base+1))
	// A source without channel metadata must not escape a configured restriction.
	oldSends := 0
	legacy := func(context.Context) error { oldSends++; return nil }
	require.NoError(t, alerting.DispatchLegacyNotice(ctx, "channel_arrears", "site", "missing-channel-metadata", "unknown-local-channel", alerting.LegacyNotice{Title: "Unknown channel", Content: "must be skipped"}, legacy))
	assert.Zero(t, oldSends)
	var unknown int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ? AND target_name = ?", row.ID, "unknown-local-channel").Count(&unknown).Error)
	assert.Zero(t, unknown)
	option = model.Option{Key: "slow_error_feishu_webhook_url"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: option.Key}).Error)
	require.NoError(t, model.DB.Model(&option).Update("value", "https://127.0.0.1/slow-events").Error)
	slow, err := alerting.ImportLegacyNotice(ctx, root, vault, "slow_error", "site")
	require.NoError(t, err)
	row = model.AlertRule{}
	require.NoError(t, model.DB.First(&row, slow.RuleID).Error)
	spec, err = model.AlertRuleConfig(row)
	require.NoError(t, err)
	spec.Scope.ChannelIDs = []int{base + 1}
	saved, err = alerting.SaveRule(ctx, root, 0, row.ID, row.Revision, spec)
	require.NoError(t, err)
	snapshot, err = alerting.LegacyNoticeSnapshot(ctx, slow.Kind, slow.ObjectID)
	require.NoError(t, err)
	require.NoError(t, alerting.SwitchLegacyNotice(ctx, root, slow.ID, slow.Revision, "cutover", snapshot.SnapshotHash, saved.Revision))
	for _, id := range []int{base + 1, base + 2} {
		notice := alerting.LegacyNotice{ChannelID: id, Title: "Slow error", Content: "500 after 211 seconds; threshold 180 seconds; request local-slow-request"}
		require.NoError(t, alerting.DispatchLegacyNotice(ctx, "slow_error", "site", fmt.Sprintf("local-slow-%d", id), fmt.Sprintf("local-notice-%d", id), notice, legacy))
	}
	var slowEvents []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ?", slow.RuleID).Find(&slowEvents).Error)
	require.Len(t, slowEvents, 1)
	assert.Equal(t, fmt.Sprintf("local-notice-%d", base+1), slowEvents[0].TargetName)
	assert.Contains(t, alerting.RenderNotification("zh-CN", slowEvents[0]), "local-slow-request")
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", root.ID); c.Set("role", root.Role); c.Next() })
	router.GET("/rules", controller.ListAlertRules)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/rules?page=1&scope=all&channel_id=%d", base+1), nil))
	require.Equal(t, http.StatusOK, response.Code)
	var related struct {
		Data struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &related))
	assert.Equal(t, 2, related.Data.Total, "channel links must include both scoped legacy rules")
	require.Len(t, related.Data.Items, 2)
	assert.ElementsMatch(t, []int64{migration.RuleID, slow.RuleID}, []int64{related.Data.Items[0].ID, related.Data.Items[1].ID})
}

func TestAlertNotificationUsageAndLegacyDetails(t *testing.T) {
	for _, language := range []string{"en", "zh-CN", "zh-TW"} {
		t.Run(language, func(t *testing.T) {
			condition := model.AlertCondition{Metric: "spend_net", ThresholdUSD: "1000", Window: model.AlertWindow{Timezone: "Asia/Shanghai"}}
			event := alertNotificationFixture(t, condition, 157340000, "milestone")
			spec := model.AlertRuleSpec{Name: "User spending", Severity: "P2", QuotaUnit: 1000000, Scope: model.AlertScope{Type: "account"}, Condition: condition, Trigger: model.AlertTrigger{Mode: "milestone", StepUSD: "1000"}}
			config, err := common.Marshal(spec)
			require.NoError(t, err)
			event.RuleConfig = string(config)
			event.MilestoneFirst, event.MilestoneLast = 443, 444
			for _, target := range []alerting.MonitoredTarget{{Type: "account", ID: 62, OwnerUserID: 62}, {Type: "api_key", ID: 19, OwnerUserID: 62}} {
				evidence, err := common.Marshal(map[string]any{"target": target, "measurement": alerting.Measurement{Value: 444500000000, Samples: 16, Status: "complete"}})
				require.NoError(t, err)
				event.Evidence = string(evidence)
				message := alerting.RenderNotification(language, event)
				assert.Contains(t, message, "UserID=62")
				if target.Type == "api_key" {
					assert.Contains(t, message, "KeyID=19")
				}
				assert.Contains(t, message, "444000 USD")
				assert.Contains(t, message, "1000 USD")
				assert.NotContains(t, message, "alert.target.")
				assert.NotContains(t, message, "alert.milestone.amount")
			}
			body, err := common.Marshal(alerting.LegacyNotice{ChannelID: 49, Title: "Slow channel error", Content: "local environment\nChannel usapi-gemini (49)\nStatusCode: 500\nRequestId: local-request-123\n211s > 180s"})
			require.NoError(t, err)
			event.Kind, event.Evidence, event.CreatedAt = "legacy_notice", string(body), time.Date(2026, 11, 15, 13, 6, 28, 0, time.UTC).UnixMilli()
			message := alerting.RenderNotification(language, event)
			assert.Contains(t, message, "local-request-123")
			assert.Contains(t, message, "211s > 180s")
			assert.Contains(t, message, "2026-11-15T21:06:28+08:00")
			assert.Contains(t, message, "/alerts?event_id=29")
			assert.NotContains(t, message, "alert.notice.context")
		})
	}
}

func testAlertTrafficBaselines(t *testing.T) {
	ctx := context.Background()
	root := alerting.Actor{ID: 3, Role: common.RoleRootUser}
	target := model.AlertTarget{OwnerUserID: 0, Name: "Traffic operations", Type: "email", Enabled: true, Revision: 1}
	require.NoError(t, model.DB.Create(&target).Error)
	for _, id := range []int{3101, 3102} {
		require.NoError(t, model.DB.Create(&model.Channel{Id: id, Name: fmt.Sprintf("traffic-%d", id), Type: 1, Status: common.ChannelStatusEnabled, Models: "traffic-model", Group: "default"}).Error)
	}
	var spec model.AlertRuleSpec
	require.NoError(t, common.UnmarshalJsonStr(fmt.Sprintf(`{"name":"Traffic drop","category":"health","severity":"P2","enabled":true,"management_mode":"platform_enforced","scope":{"type":"channel_model","channel_ids":[3101],"group_mode":"per_entity","model_selector":{"mode":"explicit","models":["traffic-model"]}},"condition":{"metric":"request_volume_change","change_direction":"decrease","baseline":{"mode":"manual","count":10},"operator":"gte","threshold_bps":5000,"recovery_threshold_bps":2000,"min_samples":0,"hold_seconds":0,"recovery_seconds":0,"window":{"type":"rolling","duration_seconds":300,"timezone":"Asia/Shanghai"}},"trigger":{"mode":"incident","repeat_seconds":60},"notify":{"target_ids":[%d],"on_recovery":true},"actions":[],"recovery":{"mode":"manual"}}`, target.ID), &spec))
	require.NoError(t, alerting.Validate(ctx, root, 0, &spec))
	row, err := alerting.SaveRule(ctx, root, 0, 0, 0, spec)
	require.NoError(t, err)
	now := time.Now().Add(-2 * time.Second)
	row.EffectiveAt = now.Add(-time.Hour).UnixMilli()
	require.NoError(t, model.DB.Model(row).Update("effective_at", row.EffectiveAt).Error)
	targets, err := alerting.ResolveTargets(ctx, *row, spec)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	monitored := targets[0]
	for i := range 4 {
		at := now.Add(-time.Duration(30+i) * time.Second).UnixMilli()
		fact := model.AlertObservation{ID: model.AlertIdentity("traffic-start", i), SourceID: model.AlertIdentity("traffic-source", i), Namespace: "health", Kind: "started", Layer: "channel_attempt", ChannelID: 3101, ChannelName: "traffic-3101", UserID: 1, TokenID: 11, ModelKey: monitored.Model, ConfigVersion: monitored.ConfigVersion, SampleClass: "production", AttributionCovered: true, Sequence: 1, StartedAt: at, OccurredAt: at, RequestID: "same-user-request"}
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
		fact.ID = model.AlertIdentity("traffic-metadata", i)
		fact.Kind = "metadata"
		fact.Sequence = 2
		fact.OccurredAt++
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	parent := model.AlertObservation{ID: model.AlertIdentity("traffic-parent-start"), SourceID: model.AlertIdentity("traffic-parent"), Namespace: "health", Kind: "started", Layer: "request", SampleClass: "security", Sequence: 1, StartedAt: now.Add(-time.Minute).UnixMilli(), OccurredAt: now.Add(-time.Minute).UnixMilli(), RequestID: "same-user-request"}
	require.NoError(t, model.AcceptAlertObservation(ctx, parent))
	parent.ID = model.AlertIdentity("traffic-parent-metadata")
	parent.Kind = "metadata"
	parent.Sequence = 2
	parent.SampleClass = "production"
	parent.ModelKey = "traffic-model"
	parent.UserID = 1
	parent.AttributionCovered = true
	parent.OccurredAt++
	require.NoError(t, model.AcceptAlertObservation(ctx, parent))
	measured, err := alerting.Measure(ctx, *row, spec, monitored, now)
	require.NoError(t, err)
	require.Equal(t, "complete", measured.Status, measured.Reason)
	assert.EqualValues(t, 6000, measured.Value, "four started attempts against a baseline of ten is a 60 percent decrease, even while all are in flight")
	engine := alerting.Engine{}
	require.NoError(t, engine.ApplyMeasurement(ctx, *row, spec, monitored, measured, now))
	var events []model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).Find(&events).Error)
	require.Len(t, events, 1)
	for _, language := range []string{"en", "zh-CN", "zh-TW"} {
		notification := alerting.RenderNotification(language, events[0])
		assert.Contains(t, notification, "60%")
		assert.Contains(t, notification, "50%")
		assert.NotContains(t, notification, "6000")
	}
	// A drifted history must not replace the incident's original baseline.
	drifted := measured
	var payload map[string]any
	encoded, err := common.Marshal(drifted)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &payload))
	traffic := payload["traffic"].(map[string]any)
	traffic["baseline_scaled"] = "40000"
	encoded, err = common.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &drifted))
	require.NoError(t, engine.ApplyMeasurement(ctx, *row, spec, monitored, drifted, now.Add(time.Second)))
	var incident model.AlertIncident
	require.NoError(t, model.DB.Where("rule_id = ?", row.ID).First(&incident).Error)
	assert.Equal(t, "firing", incident.Status, "a falling baseline must not silently recover an ongoing drop")
	// A fresh engine must recover against the persisted baseline, with its own
	// recovery hold, even when the current historical baseline is unavailable.
	recovered := measured
	recovered.Traffic = &alerting.TrafficMeasurement{Count: 8, CurrentComplete: true}
	recovered.Status, recovered.Reason = "warming_up", "baseline_insufficient"
	recoverySpec := spec
	recoverySpec.Condition.RecoverySeconds = 10
	restarted := alerting.Engine{}
	require.NoError(t, restarted.ApplyMeasurement(ctx, *row, recoverySpec, monitored, recovered, now.Add(2*time.Second)))
	require.NoError(t, model.DB.First(&incident, incident.ID).Error)
	assert.Equal(t, "firing", incident.Status)
	require.NoError(t, restarted.ApplyMeasurement(ctx, *row, recoverySpec, monitored, recovered, now.Add(12*time.Second)))
	require.NoError(t, model.DB.First(&incident, incident.ID).Error)
	assert.Equal(t, "resolved", incident.Status)
	var recoveryEvent model.AlertEvent
	require.NoError(t, model.DB.Where("rule_id = ? AND kind = ?", row.ID, "recovery").First(&recoveryEvent).Error)
	assert.Contains(t, alerting.RenderNotification("en", recoveryEvent), "20%")
	// Absolute request counts treat a fully covered empty window as zero.
	absolute := spec
	absolute.Condition.Metric = "request_started_count"
	absolute.Condition.Operator = "lt"
	absolute.Condition.ThresholdCount = 1
	absolute.Condition.RecoveryThresholdCount = 1
	absolute.Condition.Window.DurationSeconds = 60
	absolute.Scope.Type = "channel"
	absolute.Scope.ChannelIDs = []int{3102}
	absolute.Condition.Baseline = nil
	absolute.Condition.ChangeDirection = ""
	require.NoError(t, alerting.Validate(ctx, root, 0, &absolute))
	emptyRow, err := alerting.SaveRule(ctx, root, 0, 0, 0, absolute)
	require.NoError(t, err)
	emptyRow.EffectiveAt = row.EffectiveAt
	require.NoError(t, model.DB.Model(emptyRow).Update("effective_at", emptyRow.EffectiveAt).Error)
	emptyTargets, err := alerting.ResolveTargets(ctx, *emptyRow, absolute)
	require.NoError(t, err)
	zero, err := alerting.Measure(ctx, *emptyRow, absolute, emptyTargets[0], now)
	require.NoError(t, err)
	assert.Equal(t, "complete", zero.Status, zero.Reason)
	assert.Zero(t, zero.Value)
	require.NoError(t, engine.ApplyMeasurement(ctx, *emptyRow, absolute, emptyTargets[0], zero, now))
	var zeroEvents int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Where("rule_id = ?", emptyRow.ID).Count(&zeroEvents).Error)
	assert.EqualValues(t, 1, zeroEvents)
	// Global models use parent operations, not the four retry attempts.
	global := spec
	global.Scope.Type = "model"
	global.Scope.ChannelIDs = nil
	global.Condition.LatencyBasis = "gateway_request"
	globalRow, err := alerting.SaveRule(ctx, root, 0, 0, 0, global)
	require.NoError(t, err)
	globalRow.EffectiveAt = row.EffectiveAt
	require.NoError(t, model.DB.Model(globalRow).Update("effective_at", globalRow.EffectiveAt).Error)
	globalTargets, err := alerting.ResolveTargets(ctx, *globalRow, global)
	require.NoError(t, err)
	require.Len(t, globalTargets, 1)
	overall, err := alerting.Measure(ctx, *globalRow, global, globalTargets[0], now)
	require.NoError(t, err)
	assert.Equal(t, "complete", overall.Status, overall.Reason)
	assert.EqualValues(t, 9000, overall.Value)
	assert.Error(t, alerting.Validate(ctx, alerting.Actor{ID: 1, Role: common.RoleCommonUser}, 0, &global))
	for _, invalid := range []struct {
		name   string
		change func(*model.AlertRuleSpec)
	}{
		{"zero manual baseline", func(s *model.AlertRuleSpec) { s.Condition.Baseline.Count = 0 }},
		{"decrease above 100 percent", func(s *model.AlertRuleSpec) { s.Condition.ThresholdBPS = 10001 }},
		{"recovery overlaps trigger", func(s *model.AlertRuleSpec) { s.Condition.RecoveryThresholdBPS = s.Condition.ThresholdBPS }},
		{"overlapping history", func(s *model.AlertRuleSpec) {
			s.Condition.Baseline = &model.AlertTrafficBaseline{Mode: "historical", HistorySeconds: 60}
		}},
		{"non-incident trigger", func(s *model.AlertRuleSpec) { s.Trigger.Mode = "once" }},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			encoded, err := common.Marshal(spec)
			require.NoError(t, err)
			var candidate model.AlertRuleSpec
			require.NoError(t, common.Unmarshal(encoded, &candidate))
			invalid.change(&candidate)
			assert.ErrorIs(t, alerting.Validate(ctx, root, 0, &candidate), alerting.ErrInvalid)
		})
	}
	historical := spec
	historical.Condition.ChangeDirection = "increase"
	historical.Condition.ThresholdBPS = 15000
	historical.Condition.Baseline = &model.AlertTrafficBaseline{Mode: "historical", HistorySeconds: 1800, MinimumCount: 1}
	historyRow, err := alerting.SaveRule(ctx, root, 0, 0, 0, historical)
	require.NoError(t, err)
	historyRow.EffectiveAt = row.EffectiveAt
	require.NoError(t, model.DB.Model(historyRow).Update("effective_at", historyRow.EffectiveAt).Error)
	for i := range 9 {
		at := now.Add(-10 * time.Minute).UnixMilli()
		fact := model.AlertObservation{ID: model.AlertIdentity("traffic-history", i), SourceID: model.AlertIdentity("traffic-history-source", i), Namespace: "health", Kind: "started", Layer: "channel_attempt", ChannelID: 3101, UserID: 1, ModelKey: monitored.Model, ConfigVersion: monitored.ConfigVersion, SampleClass: "production", AttributionCovered: true, Sequence: 1, StartedAt: at, OccurredAt: at}
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	for _, excluded := range []struct {
		class   string
		channel int
	}{{"probe", 3101}, {"production", 3102}} {
		at := now.Add(-10 * time.Minute).UnixMilli()
		fact := model.AlertObservation{ID: model.AlertIdentity("traffic-excluded", excluded.class), SourceID: model.AlertIdentity("traffic-excluded-source", excluded.class), Namespace: "health", Kind: "started", Layer: "channel_attempt", ChannelID: excluded.channel, UserID: 1, ModelKey: monitored.Model, ConfigVersion: monitored.ConfigVersion, SampleClass: excluded.class, Sequence: 1, StartedAt: at, OccurredAt: at}
		require.NoError(t, model.AcceptAlertObservation(ctx, fact))
	}
	growth, err := alerting.Measure(ctx, *historyRow, historical, monitored, now)
	require.NoError(t, err)
	require.Equal(t, "complete", growth.Status, growth.Reason)
	assert.EqualValues(t, 15000, growth.Traffic.BaselineScaled, "nine historical starts over 30 minutes normalize to 1.5 calls per five-minute window")
	assert.EqualValues(t, 16666, growth.Value, "growth beyond 100 percent is valid")
	var beforeEvents, afterEvents int64
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Count(&beforeEvents).Error)
	points, err := alerting.ReplayRule(ctx, *historyRow, historical, now.Add(-time.Second).UnixMilli(), now.UnixMilli(), 1, now)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.True(t, points[0].Matched)
	assert.EqualValues(t, 16666, points[0].Measurement.Value)
	require.NoError(t, model.DB.Model(&model.AlertEvent{}).Count(&afterEvents).Error)
	assert.Equal(t, beforeEvents, afterEvents, "replay does not emit notifications")
	historical.Condition.Baseline.MinimumCount = 2
	low, err := alerting.Measure(ctx, *historyRow, historical, monitored, now)
	require.NoError(t, err)
	assert.Equal(t, "warming_up", low.Status)
	assert.Equal(t, "baseline_insufficient", low.Reason)
	// Previously configured final-request counts also accept a healthy zero.
	absolute.Condition.Metric = "request_count"
	finalCount, err := alerting.Measure(ctx, *emptyRow, absolute, emptyTargets[0], now)
	require.NoError(t, err)
	assert.Equal(t, "complete", finalCount.Status, finalCount.Reason)
	assert.Zero(t, finalCount.Value)
	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Update("healthy", false).Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", "alert-test-boot").Update("healthy", true).Error)
	})
	absolute.Condition.Metric = "request_started_count"
	unknown, err := alerting.Measure(ctx, *emptyRow, absolute, emptyTargets[0], now)
	require.NoError(t, err)
	assert.NotEqual(t, "complete", unknown.Status, "a telemetry outage cannot be treated as zero traffic")
}

func testAlertLogRetention(t *testing.T) {
	t.Setenv("ALERTING_LOG_RETENTION_DAYS", "")
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	cutoff := now.Add(-72 * time.Hour).UnixMilli()
	old := cutoff - 1
	boot := model.AlertIdentity("retention-boot")
	life := model.AlertIdentity("retention-life")
	owner := 76001
	require.NoError(t, model.DB.Create(&model.User{Id: owner, Username: "retention-test", AffCode: "retention-test", Status: common.UserStatusEnabled}).Error)
	start := model.AlertObservation{ID: model.AlertIdentity("retention-start"), Namespace: "health", Layer: "request", Kind: "started", RequestID: "retention-diagnostic", UserID: owner, SourceID: life, BootID: boot, StartedAt: old - 1, OccurredAt: old - 1, SourceSequence: 1, Sequence: 1, SampleClass: "production"}
	duration := int64(317)
	finish := start
	finish.ID, finish.Kind, finish.Outcome, finish.DurationMS = model.AlertIdentity("retention-finish"), "finished", "success", &duration
	finish.Sequence, finish.SourceSequence, finish.OccurredAt = 2, 2, old
	inflight := start
	inflight.ID, inflight.SourceID, inflight.SourceSequence = model.AlertIdentity("retention-inflight"), model.AlertIdentity("inflight-life"), 4
	usage := model.AlertObservation{ID: model.AlertIdentity("retention-usage"), Namespace: "usage", Kind: "settlement", SourceID: "retention-charge", UserID: owner, TokenID: 76, OccurredAt: old, Quota: 100, RequestCount: 1, InputTokens: 23, OutputTokens: 9, CachedTokens: 7, CacheWrite5mTokens: 3, CacheWrite1hTokens: 2}
	refund := usage
	refund.ID, refund.SourceID, refund.Quota, refund.RequestCount = model.AlertIdentity("retention-refund"), "retention-refund", -25, 0
	refund.InputTokens, refund.OutputTokens, refund.CachedTokens, refund.CacheWrite5mTokens, refund.CacheWrite1hTokens = 0, 0, 0, 0, 0
	boundary := usage
	boundary.ID, boundary.SourceID, boundary.OccurredAt = model.AlertIdentity("retention-boundary"), "retention-boundary", cutoff
	for _, f := range []model.AlertObservation{start, finish, inflight, usage, refund, boundary} {
		require.NoError(t, model.AcceptAlertObservation(ctx, f))
	}

	event := model.AlertEvent{DedupKey: model.AlertIdentity("retention-event"), Kind: "test", OwnerUserID: owner, Summary: "old diagnostic", CreatedAt: old}
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		created, err := model.InsertAlertEventOnce(tx, &event)
		assert.True(t, created)
		return err
	}))
	done := model.AlertJob{DedupKey: model.AlertIdentity("retention-job"), EventID: event.ID, Status: "succeeded", Kind: "notify", CreatedAt: old, UpdatedAt: old}
	require.NoError(t, model.DB.Create(&done).Error)
	attempt := model.AlertJobAttempt{JobID: done.ID, CreatedAt: old}
	require.NoError(t, model.DB.Create(&attempt).Error)
	pendingEvent := model.AlertEvent{DedupKey: model.AlertIdentity("retention-active-event"), CreatedAt: old}
	require.NoError(t, model.DB.Create(&pendingEvent).Error)
	pending := model.AlertJob{EventID: pendingEvent.ID, Kind: "notify", DedupKey: model.AlertIdentity("retention-pending"), Status: "pending", CreatedAt: old, UpdatedAt: old}
	require.NoError(t, model.DB.Create(&pending).Error)
	audit := model.AlertAudit{Operation: "retention-test", CreatedAt: old}
	require.NoError(t, model.DB.Create(&audit).Error)
	dead := model.AlertDeadLetter{ID: model.AlertIdentity("retention-dead"), CreatedAt: old}
	require.NoError(t, model.DB.Create(&dead).Error)

	// Existing installations have raw observations without a compact receipt.
	require.NoError(t, model.DB.Where("id = ?", usage.ID).Delete(&model.AlertObservationReceipt{}).Error)
	t.Setenv("ALERTING_LOG_RETENTION_DAYS", "4")
	_, err := model.CleanAlertLogBatch(ctx, now)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&model.AlertObservation{}, "id = ?", usage.ID).Error)
	t.Setenv("ALERTING_LOG_RETENTION_DAYS", "") // exercise the default cutoff

	for range 2 {
		for {
			more, err := model.CleanAlertLogBatch(ctx, now)
			require.NoError(t, err)
			if !more {
				break
			}
		}
	}
	for _, row := range []any{&attempt, &done, &event, &audit, &dead} {
		assert.ErrorIs(t, model.DB.First(row).Error, gorm.ErrRecordNotFound)
	}
	require.NoError(t, model.DB.First(&pending).Error)
	require.NoError(t, model.DB.First(&pendingEvent).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		duplicate := model.AlertEvent{DedupKey: event.DedupKey}
		created, err := model.InsertAlertEventOnce(tx, &duplicate)
		assert.False(t, created)
		assert.Equal(t, event.ID, duplicate.ID)
		return err
	}))
	var rows int64
	require.NoError(t, model.DB.Model(&model.AlertObservation{}).Where("user_id = ?", owner).Count(&rows).Error)
	assert.EqualValues(t, 1, rows)
	require.NoError(t, model.DB.First(&model.AlertObservation{}, "id = ?", boundary.ID).Error)
	require.NoError(t, model.DB.First(&model.AlertLifecycle{}, "id = ?", inflight.SourceID).Error)
	assert.ErrorIs(t, model.DB.First(&model.AlertOutbox{}, "id = ?", model.AlertIdentity("projection", start.ID)).Error, gorm.ErrRecordNotFound)
	for _, f := range []model.AlertObservation{usage, start, finish} {
		require.NoError(t, model.AcceptAlertObservation(ctx, f))
	}
	conflict := usage
	conflict.Quota++
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, conflict), model.ErrAlertObservationConflict)
	conflict = finish
	conflict.ID = model.AlertIdentity("retention-second-terminal")
	conflict.SourceSequence = 5
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, conflict), model.ErrAlertObservationConflict)
	conflict = start
	conflict.SourceID, conflict.ID = model.AlertIdentity("unrelated-life"), model.AlertIdentity("duplicate-producer-position")
	assert.ErrorIs(t, model.AcceptAlertObservation(ctx, conflict), model.ErrAlertObservationConflict)

	require.NoError(t, model.DB.Model(&model.AlertWorkerCheckpoint{}).Where("id = ?", boot).Update("offered_sequence", 4).Error)
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, boot))
	var checkpoint model.AlertWorkerCheckpoint
	require.NoError(t, model.DB.First(&checkpoint, "id = ?", boot).Error)
	assert.Zero(t, checkpoint.AcceptedThrough)
	late := start
	late.ID, late.Kind, late.SourceSequence, late.Sequence = model.AlertIdentity("retention-late"), "metadata", 3, 3
	require.NoError(t, model.AcceptAlertObservation(ctx, late))
	require.NoError(t, model.VerifyAlertSourcePrefix(ctx, boot))
	require.NoError(t, model.DB.First(&checkpoint).Error)
	assert.EqualValues(t, 4, checkpoint.AcceptedThrough)
	assert.ErrorIs(t, model.DB.First(&model.AlertLifecycle{}, "id = ?", life).Error, gorm.ErrRecordNotFound)

	target := alerting.MonitoredTarget{Type: "account", ID: owner}
	rule := model.AlertRule{OwnerUserID: owner, EffectiveAt: old - 10000}
	spec := model.AlertRuleSpec{Scope: model.AlertScope{Type: "account"}, Condition: model.AlertCondition{Metric: "spend_net", Window: model.AlertWindow{Type: "cumulative", Timezone: "UTC"}}}
	report, err := alerting.MeasureReport(ctx, rule, spec, target, old, cutoff+1, now)
	require.NoError(t, err)
	require.EqualValues(t, 175, report.Report.Totals.Quota)
	assert.EqualValues(t, 2, report.Report.Totals.Requests)
	assert.EqualValues(t, 46, report.Report.Totals.InputTokens)
	assert.EqualValues(t, 18, report.Report.Totals.OutputTokens)
	assert.EqualValues(t, 14, report.Report.Totals.CachedTokens)
	assert.EqualValues(t, 6, report.Report.Totals.CacheWrite5mTokens)
	assert.EqualValues(t, 4, report.Report.Totals.CacheWrite1hTokens)
	report, err = alerting.MeasureReport(ctx, rule, spec, target, cutoff, cutoff+1, now)
	require.NoError(t, err)
	assert.EqualValues(t, 100, report.Report.Totals.Quota)

	// Inspect past measurements through the production replay path, including
	// exact terminal latency and distinct starts across archived/late metadata.
	spec.Condition.Window = model.AlertWindow{Type: "rolling", DurationSeconds: 60, Timezone: "UTC"}
	spec.Condition.Operator = "gte"
	spec.Condition.Metric, spec.Condition.Aggregation, spec.Condition.ThresholdMS = "duration_ms", "p95", 1
	points, err := alerting.ReplayRule(ctx, rule, spec, old-1000, cutoff+1, 1, now)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.EqualValues(t, duration, points[0].Measurement.Value)
	assert.Empty(t, points[0].Measurement.RequestIDs)
	spec.Condition.Metric, spec.Condition.ThresholdUSD = "spend_net", "1"
	points, err = alerting.ReplayRule(ctx, rule, spec, old-1000, cutoff+1, 1, now)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.EqualValues(t, 175, points[0].Measurement.Value)
	assert.EqualValues(t, 3, points[0].Measurement.Samples)
	spec.Condition.Metric, spec.Condition.ThresholdCount = "request_started_count", 1
	points, err = alerting.ReplayRule(ctx, rule, spec, old-1000, cutoff+1, 1, now)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.EqualValues(t, 2, points[0].Measurement.Value)

	checkAt := now.Add(48 * time.Hour)
	instance := model.SystemInstance{NodeName: "retention-rollout", Info: "{}", StartedAt: checkAt.Unix(), LastSeenAt: checkAt.Unix()}
	require.NoError(t, model.DB.Create(&instance).Error)
	ready, err := model.AlertLogReadersReady(ctx, checkAt)
	require.NoError(t, err)
	assert.False(t, ready)
	require.NoError(t, model.DB.Model(&instance).Update("info", `{"runtime":{"alert_metric_history":true}}`).Error)
	ready, err = model.AlertLogReadersReady(ctx, checkAt)
	require.NoError(t, err)
	assert.True(t, ready)
	require.NoError(t, model.DB.Delete(&instance).Error)
}
