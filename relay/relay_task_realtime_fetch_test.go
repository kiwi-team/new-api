package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false

	if err := db.AutoMigrate(&model.Task{}, &model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

func TestTryRealtimeKling4PreservesPendingBilling(t *testing.T) {
	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })
	var missingTask atomic.Bool
	missingTask.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tasks", r.URL.Path)
		assert.Equal(t, "vendor-pending", r.URL.Query().Get("task_ids"))
		assert.Equal(t, "Bearer official-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if missingTask.Load() {
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":[{"id":"vendor-pending","status":"succeeded","outputs":[{"type":"video","url":"https://cdn.example/video.mp4"}]}]}`))
	}))
	defer upstream.Close()
	baseURL := upstream.URL
	ch := model.Channel{Id: 9010, Type: constant.ChannelTypeKling, Name: "kling4-pending", Key: "official-key", BaseURL: &baseURL, Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&ch).Error)
	user := model.User{Id: 9010, Username: "kling4-pending-user", AffCode: "kling4-pending-aff", Quota: 1000}
	require.NoError(t, model.DB.Create(&user).Error)
	task := &model.Task{TaskID: "task-kling4-pending", UserId: user.Id, ChannelId: ch.Id, Platform: "kling", Status: model.TaskStatusInProgress, Progress: "50%", Quota: 500,
		Properties: model.Properties{OriginModelName: "kling-4.0", UpstreamModelName: "kling-4.0"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "vendor-pending", Key: "official-key", BillingSource: "wallet", PluginState: []byte(`{"kling4":true,"resolution":"1080p","videoInput":"none"}`),
			BillingContext: &model.TaskBillingContext{TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", u("seconds"))`, ExprVersion: 1, TaskUsageBilling: true, QuotaPerUnit: 1, GroupRatio: 1, UsageFacts: map[string]any{"seconds": float64(500)}}}},
	}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() {
		model.DB.Delete(task)
		model.DB.Unscoped().Delete(&user)
		model.DB.Delete(&ch)
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
	_, done := tryRealtimeFetch(c, task, true)
	require.False(t, done)
	assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
	missingTask.Store(false)
	_, done = tryRealtimeFetch(c, task, true)
	require.True(t, done)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, stored.Status)
	assert.Equal(t, "100%", stored.Progress)
	assert.True(t, stored.PrivateData.BillingPending)
	assert.Equal(t, 500, stored.Quota)
	assert.Equal(t, "https://cdn.example/video.mp4", stored.GetResultURL())
	assert.True(t, model.HasUnfinishedSyncTasks())
	_, done = tryRealtimeFetch(c, &stored, true)
	require.True(t, done)
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 1000, user.Quota)
}

// 客户端 GET 通常比 15 秒一轮的后台轮询更早看到上游的终态。tryRealtimeFetch 一旦
// 把任务写成 FAILURE，GetAllUnFinishSyncTasks 和 GetTimedOutUnfinishedTasks 都会
// 过滤掉它，后台轮询再也没有机会退款——所以退款必须由这条路径自己完成。
func TestTryRealtimeFetchRefundsOnTerminalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"upstream-1","status":"FAILED"}`)
	}))
	defer upstream.Close()

	const (
		userID    = 9001
		channelID = 9001
		quota     = 500
	)
	baseURL := upstream.URL
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeRunwayML,
		Name:    "runway-test",
		Key:     "sk-test",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: userID, Username: "realtime_fetch_user", Quota: 0, Status: common.UserStatusEnabled,
	}).Error)

	task := &model.Task{
		TaskID:    "task_realtime_refund",
		UserId:    userID,
		ChannelId: channelID,
		Platform:  constant.TaskPlatform("0"), // 未知平台，走渠道类型回退
		Quota:     quota,
		Status:    model.TaskStatusInProgress,
		Progress:  "50%",
		Group:     "default",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-1",
			BillingSource:  "wallet",
		},
	}
	require.NoError(t, task.Insert())
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM logs")
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_realtime_refund", nil)

	_, done := tryRealtimeFetch(c, task, true)
	require.True(t, done)

	persisted, exists, err := model.GetByTaskId(userID, "task_realtime_refund")
	require.NoError(t, err)
	require.True(t, exists)

	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), persisted.Status)
	assert.Equal(t, "100%", persisted.Progress)
	assert.NotZero(t, persisted.FinishTime)
	// Quota 归零是「已退款」的持久化标记，非零意味着这笔预扣费被永久吞掉。
	assert.Zero(t, persisted.Quota)

	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, quota, user.Quota)
}

// 任务进入终态后重复查询不得再次退款：CAS 拿不到状态流转时必须跳过结算。
func TestTryRealtimeFetchDoesNotDoubleRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"upstream-2","status":"FAILED"}`)
	}))
	defer upstream.Close()

	const (
		userID    = 9002
		channelID = 9002
		quota     = 500
	)
	baseURL := upstream.URL
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeRunwayML,
		Name:    "runway-test-2",
		Key:     "sk-test",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: userID, Username: "realtime_fetch_user2", Quota: 0, Status: common.UserStatusEnabled,
	}).Error)

	task := &model.Task{
		TaskID:    "task_realtime_no_double",
		UserId:    userID,
		ChannelId: channelID,
		Platform:  constant.TaskPlatform("0"),
		Quota:     quota,
		Status:    model.TaskStatusInProgress,
		Progress:  "50%",
		Group:     "default",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-2",
			BillingSource:  "wallet",
		},
	}
	require.NoError(t, task.Insert())
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM logs")
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_realtime_no_double", nil)

	for i := 0; i < 3; i++ {
		reloaded, exists, err := model.GetByTaskId(userID, "task_realtime_no_double")
		require.NoError(t, err)
		require.True(t, exists)
		_, done := tryRealtimeFetch(c, reloaded, true)
		require.True(t, done)
	}

	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, quota, user.Quota)
}

func TestVideoTaskQueryPreservesUpstreamURLDatabaseMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldAddress, oldMemory := system_setting.ServerAddress, common.MemoryCacheEnabled
	system_setting.ServerAddress, common.MemoryCacheEnabled = "https://domestic.example", false
	t.Cleanup(func() {
		system_setting.ServerAddress, common.MemoryCacheEnabled = oldAddress, oldMemory
	})
	const directURL = "https://bucket.s3.example/videos/generated.mp4?signature=test&expires=9999999999"
	for _, dialect := range []struct{ kind, env string }{
		{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"},
	} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("set " + dialect.env + " to run this database")
			}
			var driver gorm.Dialector
			switch dialect.kind {
			case "mysql":
				driver = mysql.Open(os.Getenv(dialect.env))
			case "postgres":
				driver = postgres.New(postgres.Config{DSN: os.Getenv(dialect.env), PreferSimpleProtocol: true})
			default:
				driver = sqlite.Open(":memory:")
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{
				TablePrefix: fmt.Sprintf("video_url_%d_", time.Now().UnixNano()),
			}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			models := []any{&model.Task{}, &model.Channel{}, &model.User{}}
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
			require.NoError(t, db.AutoMigrate(models...))
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = db, db
			common.SetDatabaseTypes(common.DatabaseType(dialect.kind), common.DatabaseType(dialect.kind))
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(oldMain, oldLog)
			})
			versionQuery := "SELECT version()"
			if dialect.kind == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect.kind, version)

			for i, tc := range []struct {
				name         string
				status       model.TaskStatus
				upstreamBody string
				upstreamHTTP int
				cachedDirect bool
				wantDirect   bool
				wantFetches  int32
			}{
				{"new success", model.TaskStatusInProgress, `{"status":"completed","video_url":"` + directURL + `"}`, 200, false, true, 1},
				{"repair completed proxy", model.TaskStatusSuccess, `{"status":"completed","video_url":"` + directURL + `","result_url":"https://other.example/result.mp4","url":"https://other.example/video.mp4"}`, 200, false, true, 1},
				{"result_url fallback", model.TaskStatusSuccess, `{"status":"completed","video_url":"  ","result_url":"` + directURL + `"}`, 200, false, true, 1},
				{"url fallback", model.TaskStatusSuccess, `{"status":"completed","video_url":null,"result_url":"","url":" ` + directURL + ` "}`, 200, false, true, 1},
				{"no direct URL", model.TaskStatusSuccess, `{"status":"completed"}`, 200, false, false, 2},
				{"failed refresh", model.TaskStatusSuccess, `{"status":"failed","error":{"message":"expired"}}`, 200, false, false, 2},
				{"stale queued refresh", model.TaskStatusSuccess, `{"status":"queued","url":"https://other.example/unfinished.mp4"}`, 200, false, false, 2},
				{"expired upstream record", model.TaskStatusSuccess, `{"error":{"message":"not found"}}`, 404, false, false, 2},
				{"reuse direct URL", model.TaskStatusSuccess, `{}`, 500, true, true, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var fetches atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						fetches.Add(1)
						assert.Equal(t, http.MethodGet, r.Method)
						assert.Equal(t, "/v1/videos/upstream-task", r.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.upstreamHTTP)
						fmt.Fprint(w, tc.upstreamBody)
					}))
					t.Cleanup(upstream.Close)
					baseURL := upstream.URL
					ch := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: tc.name, Key: "test-key", BaseURL: &baseURL, Status: common.ChannelStatusEnabled}
					require.NoError(t, db.Create(ch).Error)
					user := &model.User{Username: "video_user_" + strconv.Itoa(i), AffCode: fmt.Sprintf("vid%d", i), Quota: 1000, Status: common.UserStatusEnabled}
					require.NoError(t, db.Create(user).Error)
					task := &model.Task{
						TaskID: "task_video_" + strconv.Itoa(i), UserId: user.Id, ChannelId: ch.Id,
						Platform: constant.TaskPlatform(strconv.Itoa(ch.Type)), Status: tc.status,
						Quota: 500, Progress: "100%", FinishTime: 100, Group: "default",
						Properties:  model.Properties{OriginModelName: "gemini-omni-1.1-flash-preview"},
						PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-task", BillingSource: "wallet"},
						Data:        []byte(`{"id":"upstream-task","status":"completed"}`),
					}
					task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
					if tc.status != model.TaskStatusSuccess {
						task.Progress, task.FinishTime, task.PrivateData.ResultURL = "50%", 0, ""
					}
					wantURL := taskcommon.BuildProxyURL(task.TaskID)
					if tc.wantDirect {
						wantURL = directURL
					}
					if tc.cachedDirect {
						task.PrivateData.ResultURL = directURL
					}
					require.NoError(t, task.Insert())

					for range 2 {
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+task.TaskID, nil)
						c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
						c.Set("id", user.Id)
						body, taskErr := videoFetchByIDRespBodyBuilder(c)
						require.Nil(t, taskErr)
						var response struct {
							Code string
							Data struct {
								Status string `json:"status"`
								TaskID string `json:"task_id"`
								URL    string `json:"url"`
							}
						}
						require.NoError(t, common.Unmarshal(body, &response))
						assert.Equal(t, "success", response.Code)
						assert.Equal(t, "succeeded", response.Data.Status)
						assert.Equal(t, task.TaskID, response.Data.TaskID)
						assert.Equal(t, wantURL, response.Data.URL)
					}
					assert.Equal(t, tc.wantFetches, fetches.Load())
					persisted, exists, err := model.GetByTaskId(user.Id, task.TaskID)
					require.NoError(t, err)
					require.True(t, exists)
					assert.Equal(t, wantURL, persisted.GetResultURL())
					assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), persisted.Status)
					assert.Equal(t, "100%", persisted.Progress)
					assert.NotZero(t, persisted.FinishTime)
					if tc.status == model.TaskStatusSuccess {
						assert.Equal(t, task.FinishTime, persisted.FinishTime)
					}
					assert.Equal(t, 500, persisted.Quota)
					require.NoError(t, db.First(user, user.Id).Error)
					assert.Equal(t, 1000, user.Quota, "URL refresh must not charge or refund again")
				})
			}
		})
	}
}
