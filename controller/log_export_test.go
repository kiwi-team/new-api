package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// callLogHandler runs a log handler as the given user and returns the JSON
// lines of the response.
func callLogHandler(t *testing.T, handler gin.HandlerFunc, userId int, role int, query string) []map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log?"+query, nil)
	ctx.Set("id", userId)
	ctx.Set("role", role)
	handler(ctx)
	var lines []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(recorder.Body.String()), "\n") {
		var value map[string]any
		require.NoError(t, common.UnmarshalJsonStr(line, &value))
		lines = append(lines, value)
	}
	return lines
}

func exportedRequestIds(lines []map[string]any) []any {
	ids := make([]any, 0, len(lines))
	for _, line := range lines[:len(lines)-1] {
		ids = append(ids, line["request_id"])
	}
	return ids
}

// seedExportLogs stores logs of two users: three in the last hour and one
// from 400 days ago.
func seedExportLogs(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB, now int64) {
	t.Helper()
	require.NoError(t, mainDB.Create(&[]model.User{
		{Id: 1, Username: "alice", AffCode: "alice", Role: common.RoleCommonUser},
		{Id: 2, Username: "bob", AffCode: "bob", Role: common.RoleCommonUser},
		{Id: 3, Username: "admin", AffCode: "admin", Role: common.RoleAdminUser},
	}).Error)
	require.NoError(t, mainDB.Create(&model.Channel{Id: 9, Name: "primary"}).Error)
	extra := model.LogJSON(`{"mt_session_id":"mt-1","trace_id":"trace-1","traj_id":"traj-1"}`)
	header := model.LogJSON(`{"X-Request-ID":"private-header"}`)
	logs := []model.Log{
		{UserId: 1, Username: "alice", CreatedAt: now - 3600, Type: model.LogTypeConsume, ModelName: "gpt-a", ChannelId: 9, Quota: 10, RequestId: "a-1", Other: `{"admin_info":{"use_channel":["9"]}}`, Request: "private-request", Response: "private-response", Usage: "private-usage", Extra: &extra, Header: &header, SessionId: "session-1"},
		{UserId: 2, Username: "bob", CreatedAt: now - 1800, Type: model.LogTypeConsume, ModelName: "gpt-b", ChannelId: 9, Quota: 20, RequestId: "b-1"},
		{UserId: 1, Username: "alice", CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "gpt-a", RequestId: "a-2"},
		{UserId: 1, Username: "alice", CreatedAt: now - 400*24*3600, Type: model.LogTypeConsume, ModelName: "gpt-a", RequestId: "a-old"},
	}
	for i := range logs {
		require.NoError(t, logDB.Create(&logs[i]).Error)
	}
}

func useLogExportDatabases(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB, mainType common.DatabaseType, logType common.DatabaseType) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	model.DB, model.LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(mainType, logType)
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
	})
}

func TestLogExportDatabaseMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	for _, tc := range []struct {
		kind, env string
		typ       common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"mysql", "AUDIT_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "AUDIT_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
		{"clickhouse", "AUDIT_CLICKHOUSE_DSN", common.DatabaseTypeClickHouse},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade=%v", upgrade), func(t *testing.T) {
					dsn := os.Getenv(tc.env)
					if tc.env != "" && dsn == "" {
						t.Skip(tc.env + " is not configured")
					}
					db, isolatedDSN := newAuditTestDatabase(t, tc.kind, dsn)
					mainDB, mainType := db, tc.typ
					if tc.kind == "clickhouse" {
						mainDB, _ = newAuditTestDatabase(t, "sqlite", "")
						mainType = common.DatabaseTypeSQLite
					}
					useLogExportDatabases(t, mainDB, db, mainType, tc.typ)

					if upgrade && tc.kind != "clickhouse" {
						require.NoError(t, mainDB.Table("tokens").AutoMigrate(&releasedTokenRC45{}))
						require.NoError(t, mainDB.Table("tokens").Create(&releasedTokenRC45{Key: "release-key", UserId: 1, Name: "release-token"}).Error)
						require.NoError(t, db.Table("logs").AutoMigrate(&releasedLogRC45{}))
						require.NoError(t, db.Table("logs").Create(&releasedLogRC45{RequestId: "release-log", Content: "preserved", Quota: 123, CreatedAt: 1}).Error)
					}
					require.NoError(t, mainDB.AutoMigrate(&model.Channel{}, &model.User{}, &model.Token{}))
					if upgrade && tc.kind != "clickhouse" {
						for range 2 {
							require.NoError(t, mainDB.AutoMigrate(&model.Token{}))
						}
						var token model.Token
						require.NoError(t, mainDB.Where(model.Token{Key: "release-key"}).First(&token).Error)
						assert.Equal(t, "release-token", token.Name)
						assert.False(t, token.AllowUserQuotaQuery)
						require.Error(t, mainDB.Create(&model.Token{Key: "release-key"}).Error, "token uniqueness survives the upgrade")
					}

					if tc.kind == "clickhouse" {
						if upgrade {
							require.NoError(t, db.Exec(releasedClickHouseLogsRC45).Error)
							require.NoError(t, db.Table("logs").Create(&releasedLogRC45{RequestId: "release-log", Content: "preserved", Quota: 123, CreatedAt: 1}).Error)
						}
						t.Setenv("LOG_SQL_DSN", isolatedDSN)
						require.NoError(t, model.InitLogDB())
						require.NoError(t, model.InitLogDB())
						require.NoError(t, model.InitLogDB())
					} else {
						require.NoError(t, db.AutoMigrate(&model.Log{}, &model.ErrorLog{}))
					}
					versionSQL := "SELECT version()"
					if tc.kind == "sqlite" {
						versionSQL = "SELECT sqlite_version()"
					}
					var version string
					require.NoError(t, model.LOG_DB.Raw(versionSQL).Scan(&version).Error)
					t.Logf("log database: %s %s", tc.kind, version)
					now := time.Now().Unix()
					seedExportLogs(t, mainDB, model.LOG_DB, now)
					if tc.kind != "clickhouse" {
						for range 2 {
							require.NoError(t, db.AutoMigrate(&model.Log{}, &model.ErrorLog{}))
						}
					}

					if upgrade {
						var preserved model.Log
						require.NoError(t, db.Where("request_id = ?", "release-log").First(&preserved).Error)
						assert.Equal(t, "preserved", preserved.Content)
						assert.Equal(t, 123, preserved.Quota)
						if tc.kind != "clickhouse" {
							assert.True(t, db.Migrator().HasIndex(&model.Log{}, "idx_created_at_id"))
							assert.True(t, db.Migrator().HasIndex(&model.Log{}, "idx_user_id_id"))
						}
					}
					query := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-2*24*3600, now+3600)

					listed := callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, query)[0]
					require.Equal(t, true, listed["success"])
					assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])
					listed = callLogHandler(t, GetAllLogs, 3, common.RoleAdminUser, query+"&username=alice")[0]
					require.Equal(t, true, listed["success"])
					assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])

					own := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, query)
					require.Len(t, own, 3)
					assert.Equal(t, []any{"a-2", "a-1"}, exportedRequestIds(own))
					assert.Equal(t, map[string]any{"success": true, "message": "", "data": map[string]any{"count": float64(2)}}, own[2])
					assert.Equal(t, float64(1), own[0]["id"], "user exports carry list positions, not row ids")
					assert.Empty(t, own[1]["channel_name"])
					assert.NotContains(t, own[1]["other"], "admin_info")

					all := callLogHandler(t, ExportAllLogs, 3, common.RoleAdminUser, query+"&model_name=gpt-%25")
					require.Len(t, all, 4)
					assert.Equal(t, []any{"a-2", "b-1", "a-1"}, exportedRequestIds(all))
					assert.Empty(t, all[2]["channel_name"])
					assert.NotContains(t, all[2]["other"], "admin_info")
					for _, logs := range [][]map[string]any{own, all} {
						for _, log := range logs[:len(logs)-1] {
							assert.Empty(t, log["request"])
							assert.Empty(t, log["response"])
							assert.Empty(t, log["usage"])
							assert.Nil(t, log["header"])
						}
					}
					assert.Equal(t, true, all[3]["success"])

					root := callLogHandler(t, ExportAllLogs, 3, common.RoleRootUser, query+"&mt_session_id=mt-1&trace_id=trace-1&traj_id=traj-1&session_id=session-1")
					require.Len(t, root, 2)
					assert.Equal(t, "a-1", root[0]["request_id"])
					assert.Equal(t, "primary", root[0]["channel_name"])
					assert.Contains(t, root[0]["other"], "admin_info")
					assert.Equal(t, "private-request", root[0]["request"])
					assert.Equal(t, "private-response", root[0]["response"])
					assert.Equal(t, "private-usage", root[0]["usage"])
					assert.JSONEq(t, `{"X-Request-ID":"private-header"}`, root[0]["header"].(string))

					// Both log types share JSON filtering and preserve SQL NULL values.
					if tc.kind != "clickhouse" {
						extra := model.LogJSON(`{"mt_session_id":"mt-error","trace_id":"trace-error","traj_id":"traj-error"}`)
						require.NoError(t, db.Create(&model.ErrorLog{UserId: 1, CreatedAt: now, Extra: &extra, SessionId: "error-session", Body: "error-body"}).Error)
						require.NoError(t, db.Create(&model.ErrorLog{UserId: 1, CreatedAt: now}).Error)
						filter := &dto.ErrorLogsRequest{StartTime: now - 60, EndTime: now + 1, MtSessionId: "mt-error", TraceId: "trace-error", TrajId: "traj-error", SessionId: "error-session"}
						failures, total, err := model.GetAllErrorLog(filter)
						require.NoError(t, err)
						require.Len(t, failures, 1)
						assert.EqualValues(t, 1, total)
						assert.Empty(t, failures[0].Body)
						assert.Nil(t, failures[0].Header)
						exported, err := model.GetErrorLogsForExport(filter)
						require.NoError(t, err)
						require.Len(t, exported, 1)
						assert.Equal(t, "error-body", exported[0].Body)
						assert.JSONEq(t, string(extra), string(*exported[0].Extra))
					}

					model.RecordLog(1, model.LogTypeTopup, common.NewMessage("merge quota regression"), 456)
					var topup model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeTopup).First(&topup).Error)
					assert.Equal(t, 456, topup.Quota)
					assert.Contains(t, topup.Content, "merge quota regression")
					assert.Contains(t, topup.Other, "merge quota regression")
				})
			}
		})
	}
}

func TestLogExportRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := newAuditTestDatabase(t, "sqlite", "")
	useLogExportDatabases(t, db, db, common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.User{}, &model.Log{}, &model.QuotaData{}))
	now := time.Now().Unix()
	seedExportLogs(t, db, db, now)
	logSetting := operation_setting.GetLogSetting()
	previousSetting := *logSetting
	t.Cleanup(func() { *logSetting = previousSetting })
	recent := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-2*24*3600, now)
	tooLong := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-366*24*3600, now)

	t.Run("time range", func(t *testing.T) {
		rejected := callLogHandler(t, ExportAllLogs, 3, common.RoleAdminUser, tooLong)
		require.Len(t, rejected, 1)
		assert.Equal(t, false, rejected[0]["success"])
		assert.Equal(t, "The time range cannot exceed {{days}} days", rejected[0]["message_key"])
		assert.Equal(t, map[string]any{"days": float64(365)}, rejected[0]["message_params"])

		assert.Equal(t, false, callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, tooLong)[0]["success"])
		assert.Equal(t, false, callLogHandler(t, GetLogsSelfStat, 1, common.RoleCommonUser, tooLong)[0]["success"])
		assert.Equal(t, true, callLogHandler(t, GetUserLogs, 3, common.RoleAdminUser, tooLong)[0]["success"], "administrators search their own logs without a limit")

		// The dashboard follows the same range instead of its old one-month limit.
		fortyDays := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-40*24*3600, now)
		for _, handler := range []gin.HandlerFunc{GetUserQuotaDates, GetUserFlowQuotaDates} {
			assert.Equal(t, "The time range cannot exceed {{days}} days", callLogHandler(t, handler, 1, common.RoleCommonUser, tooLong)[0]["message_key"])
			assert.Equal(t, true, callLogHandler(t, handler, 1, common.RoleCommonUser, fortyDays)[0]["success"])
		}

		// Without a start, a user's search reaches back the whole range.
		listed := callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, "")[0]
		assert.Equal(t, true, listed["success"])
		assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])

		logSetting.MaxRangeDays = 400
		assert.Equal(t, true, callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, tooLong)[0]["success"])
		logSetting.MaxRangeDays = previousSetting.MaxRangeDays
	})

	t.Run("user export switch", func(t *testing.T) {
		logSetting.UserExportEnabled = false
		t.Cleanup(func() { logSetting.UserExportEnabled = previousSetting.UserExportEnabled })
		disabled := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent)
		require.Len(t, disabled, 1)
		assert.Equal(t, "Log export is disabled", disabled[0]["message_key"])
		assert.Equal(t, true, callLogHandler(t, ExportUserLogs, 3, common.RoleAdminUser, recent)[0]["success"], "administrators export their own logs even when users cannot")
	})

	t.Run("one export per user", func(t *testing.T) {
		release, ok, err := service.AcquireLogExportLock(1)
		require.NoError(t, err)
		require.True(t, ok)
		busy := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent)
		require.Len(t, busy, 1)
		assert.Equal(t, "Another log export is running. Try again after it finishes.", busy[0]["message_key"])
		other := callLogHandler(t, ExportUserLogs, 2, common.RoleCommonUser, recent)
		assert.Equal(t, true, other[len(other)-1]["success"], "other users are not blocked")
		release()
		assert.Len(t, callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent), 3)
	})
}

func TestAcquireLogExportLock(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis=%v", useRedis), func(t *testing.T) {
			previousRedis, previousClient := common.RedisEnabled, common.RDB
			common.RedisEnabled = useRedis
			if useRedis {
				server := miniredis.RunT(t)
				common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
				require.NoError(t, common.RDB.Ping(context.Background()).Err())
			}
			t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedis, previousClient })

			release, ok, err := service.AcquireLogExportLock(1)
			require.NoError(t, err)
			require.True(t, ok)
			_, ok, err = service.AcquireLogExportLock(1)
			require.NoError(t, err)
			assert.False(t, ok, "a second export of the same user must wait")
			releaseOther, ok, err := service.AcquireLogExportLock(2)
			require.NoError(t, err)
			assert.True(t, ok)
			releaseOther()
			release()
			releaseAgain, ok, err := service.AcquireLogExportLock(1)
			require.NoError(t, err)
			assert.True(t, ok, "a finished export frees the user")
			releaseAgain()
		})
	}
}

// Schema fixture from v1.0.0-rc.45, retained to verify real upgrades.
type releasedLogRC45 struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

// Schema fixture from v1.0.0-rc.45, retained to verify real upgrades.
type releasedTokenRC45 struct {
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

// Released ClickHouse log schema before branch diagnostic columns.
const releasedClickHouseLogsRC45 = `
CREATE TABLE IF NOT EXISTS logs (
	id Int64 DEFAULT 0,
	user_id Int32 DEFAULT 0,
	created_at Int64 DEFAULT 0,
	type Int32 DEFAULT 0,
	content String DEFAULT '',
	username String DEFAULT '',
	token_name String DEFAULT '',
	model_name String DEFAULT '',
	quota Int32 DEFAULT 0,
	prompt_tokens Int32 DEFAULT 0,
	completion_tokens Int32 DEFAULT 0,
	use_time Int32 DEFAULT 0,
	is_stream UInt8 DEFAULT 0,
	channel_id Int32 DEFAULT 0,
	token_id Int32 DEFAULT 0,
	` + "`group`" + ` String DEFAULT '',
	ip String DEFAULT '',
	request_id String DEFAULT '',
	upstream_request_id String DEFAULT '',
	other String DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)`
