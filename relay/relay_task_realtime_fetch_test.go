package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
