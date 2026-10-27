package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled
	previousSaveErrorLog := common.SaveErrorLog

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}, &model.ErrorLog{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	common.SaveErrorLog = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		common.SaveErrorLog = previousSaveErrorLog
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	ctx.Set(common.KeyBodyStorage, storage)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	ctx.Set(common.UpstreamRequestIdKey, "later-attempt-id")
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil, 1000, "upstream-attempt-101", true, false)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	assert.Equal(t, "upstream-attempt-101", stored.UpstreamRequestId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogFilter{Type: model.LogTypeError}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}

	var storedError model.ErrorLog
	require.NoError(t, database.First(&storedError).Error)
	assert.Equal(t, channelSnapshot.ChannelId, storedError.ChannelId)
	assert.Equal(t, channelSnapshot.ChannelName, storedError.ChannelName)
	assert.Equal(t, "gpt-test", storedError.ModelName)
	assert.Equal(t, string(body), storedError.Body)
	assert.Equal(t, http.StatusBadGateway, storedError.StatusCode)
}

// Run with TEST_TASK_DB_DIALECT=sqlite/mysql/postgres and the existing DSN
// variables. The same HTTP/log contract is exercised on every real engine.
func TestRequestErrorLoggingDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db, dialect := openTaskDialectDatabase(t, &model.ErrorLog{}, &model.Token{})
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousSave, previousRedis := common.SaveErrorLog, common.RedisEnabled
	previousOptions := common.OptionMap
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.SaveErrorLog, common.RedisEnabled = true, false
	common.OptionMap = map[string]string{"RewriteMessage": `["(?s).+"]`, "LogHeaderRedactEnabled": "false"}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.SaveErrorLog, common.RedisEnabled = previousSave, previousRedis
		common.OptionMap = previousOptions
	})

	for _, tc := range []struct {
		name       string
		middleware gin.HandlerFunc
		handler    gin.HandlerFunc
		body       string
		status     int
		message    string
		count      int
		rewrite    string
		reply      string
	}{
		{name: "distributor rejects malformed request", middleware: middleware.Distribute(), body: `{`, status: 400, count: 1},
		{name: "relay validation fails before an attempt", handler: func(c *gin.Context) { RelayWithoutRace(c, types.RelayFormatOpenAI) }, body: `{}`, status: 400, count: 1},
		{name: "bare status fallback", handler: func(c *gin.Context) { c.AbortWithStatus(503) }, status: 503, message: "Service Unavailable", count: 1},
		{name: "JSON fallback", handler: func(c *gin.Context) {
			c.JSON(502, gin.H{"sequence": int64(9007199254740993), "error": gin.H{"message": "original upstream failure", "code": "upstream_error"}})
		}, status: 502, message: "original upstream failure", count: 1, reply: `"sequence":9007199254740993`},
		{name: "unmatched JSON is untouched", handler: func(c *gin.Context) {
			c.Data(502, "application/json", []byte(`{"error": {"message":"original upstream failure","code":17}}`))
		}, status: 502, message: "original upstream failure", count: 1, rewrite: `["different failure"]`, reply: `{"error": {"message":"original upstream failure","code":17}}`},
		{name: "panic fallback", handler: func(c *gin.Context) { panic("original panic") }, status: 500, message: "request panic: original panic", count: 1},
		{name: "partial stream with HTTP 200", handler: func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			_, err := c.Writer.WriteString("data: hello\n\n")
			require.NoError(t, err)
			c.Writer.Flush()
			stream := relaycommon.NewStreamStatus()
			stream.RecordError("original SSE failure")
			stream.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("connection reset"))
			service.RecordStreamError(c, stream)
		}, status: 200, count: 1},
		{name: "task original error before sanitization", handler: func(c *gin.Context) {
			respondPluginProtocolSubmissionError(c, &hostdto.TaskError{Code: "upstream_failed", Message: "private task failure", StatusCode: 503})
		}, status: 503, message: "private task failure", count: 1, rewrite: `["^private task failure$"]`, reply: "请求失败，请稍后再尝试"},
		{name: "sanitized fallback must not trigger rewrite", handler: func(c *gin.Context) {
			respondPluginProtocolSubmissionError(c, &hostdto.TaskError{Code: "upstream_failed", Message: "private task failure", StatusCode: 503})
		}, status: 503, message: "private task failure", count: 1, rewrite: `["Task protocol request failed"]`, reply: "Task protocol request failed (request id:"},
		{name: "same channel retry retained terminal deduplicated", handler: func(c *gin.Context) {
			c.Set("channel_id", 91)
			upstream := types.NewOpenAIError(errors.New("retry failure"), types.ErrorCodeBadResponseStatusCode, 503)
			channel := types.ChannelError{ChannelId: 91, ChannelName: "test"}
			recordRelayErrorLog(c, channel, upstream, 12, false)
			recordRelayErrorLog(c, channel, upstream, 24, true)
			client := service.ClientError(c, upstream)
			assert.Equal(t, "retry failure", upstream.Error())
			assert.Contains(t, client.Error(), "请求失败，请稍后再尝试")
			c.JSON(503, gin.H{"error": client.ToOpenAIError()})
		}, status: 503, message: "retry failure", count: 2},
		{name: "local failure after failed attempt", handler: func(c *gin.Context) {
			c.Set("channel_id", 91)
			recordRelayErrorLog(c, types.ChannelError{ChannelId: 91}, types.NewOpenAIError(errors.New("attempt failed"), types.ErrorCodeBadResponseStatusCode, 503), 1, false)
			client := service.ClientError(c, types.NewErrorWithStatusCode(errors.New("no more channels"), types.ErrorCodeGetChannelFailed, 503))
			c.JSON(503, gin.H{"error": client.ToOpenAIError()})
		}, status: 503, count: 2},
		{name: "race local failures each retained", handler: func(c *gin.Context) {
			var workers sync.WaitGroup
			for _, channelID := range []int{91, 92} {
				child := c.Copy()
				child.Set("channel_id", channelID)
				workers.Go(func() {
					service.RecordRequestError(child, types.NewErrorWithStatusCode(errors.New("race setup failed"), types.ErrorCodeGetChannelFailed, 503))
				})
			}
			workers.Wait()
			c.Status(503)
		}, status: 503, message: "race setup failed", count: 2},
		{name: "successful stream is untouched", handler: func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.String(200, "data: success\n\n")
		}, status: 200, count: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousRewrite := common.OptionMap["RewriteMessage"]
			if tc.rewrite != "" {
				common.OptionMap["RewriteMessage"] = tc.rewrite
			}
			t.Cleanup(func() { common.OptionMap["RewriteMessage"] = previousRewrite })
			requestID := "errorlog-" + tc.name
			engine := gin.New()
			engine.Use(gin.Recovery(), func(c *gin.Context) {
				c.Set(common.RequestIdKey, requestID)
				c.Set("id", 7)
				c.Set("token_id", 11)
				c.Set("original_model", "test-model")
				c.Set(string(constant.ContextKeyClientUserId), "test-uid")
				c.Set(string(constant.ContextKeyHeader), `{"Authorization":"Bearer secret-value","X-Api-Key":"another-secret","Content-Type":"application/json"}`)
			}, middleware.RouteTag("relay"), middleware.RequestErrorLog(), middleware.BodyStorageCleanup())
			handler := tc.handler
			if tc.middleware != nil {
				engine.Use(tc.middleware)
			}
			if handler == nil {
				handler = func(c *gin.Context) { t.Error("rejected request reached handler"); c.Status(204) }
			}
			engine.POST("/v1/chat/completions", handler)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			if tc.reply != "" {
				assert.Contains(t, response.Body.String(), tc.reply)
			}
			var rows []model.ErrorLog
			require.NoError(t, db.Where("request_id = ?", requestID).Order("id").Find(&rows).Error)
			require.Len(t, rows, tc.count)
			for _, row := range rows {
				assert.Equal(t, 7, row.UserId)
				assert.Equal(t, 11, row.TokenId)
				assert.Equal(t, "test-uid", row.ClientUserId)
				assert.Equal(t, "test-model", row.ModelName)
				assert.NotEmpty(t, row.Message)
				assert.NotContains(t, row.Message, "请求失败，请稍后再尝试")
				if tc.message != "" {
					assert.Equal(t, tc.message, row.Message)
				}
				header, err := common.Marshal(row.Header)
				require.NoError(t, err)
				assert.NotContains(t, string(header), "secret-value")
				assert.NotContains(t, string(header), "another-secret")
			}
			if tc.name == "partial stream with HTTP 200" {
				assert.Equal(t, "data: hello\n\n", response.Body.String())
				assert.Contains(t, rows[0].Message, "original SSE failure")
				assert.Equal(t, 502, rows[0].StatusCode)
			}
			if tc.name == "JSON fallback" {
				assert.Contains(t, response.Body.String(), "请求失败，请稍后再尝试")
			}
		})
	}

	t.Run("credential exclusions and operational failures", func(t *testing.T) {
		for index, tc := range []struct {
			name       string
			status     int
			expires    int64
			quota      int
			ips        *string
			wantStatus int
			wantLogs   int
		}{
			{name: "missing", wantStatus: 401},
			{name: "unknown", wantStatus: 401},
			{name: "disabled", status: common.TokenStatusDisabled, expires: -1, quota: 10, wantStatus: 401},
			{name: "expired", status: common.TokenStatusEnabled, expires: time.Now().Unix() - 60, quota: 10, wantStatus: 401},
			{name: "exhausted status", status: common.TokenStatusExhausted, expires: -1, wantStatus: 401, wantLogs: 1},
			{name: "exhausted quota", status: common.TokenStatusEnabled, expires: -1, wantStatus: 401, wantLogs: 1},
			{name: "IP restriction", status: common.TokenStatusEnabled, expires: -1, quota: 10, ips: common.GetPointer("198.51.100.1"), wantStatus: 403, wantLogs: 1},
			{name: "database failure", wantStatus: 500, wantLogs: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				key := fmt.Sprintf("testkey%d", index)
				if tc.status != 0 {
					require.NoError(t, db.Create(&model.Token{UserId: 7, Key: key, Status: tc.status, ExpiredTime: tc.expires, RemainQuota: tc.quota, AllowIps: tc.ips}).Error)
				}
				if tc.name == "database failure" {
					model.DB = db.Table("errorlog_missing_tokens")
					defer func() { model.DB = db }()
				}
				engine := gin.New()
				engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, tc.name) }, middleware.RouteTag("relay"), middleware.RequestErrorLog(), middleware.TokenAuth())
				engine.POST("/v1/responses", func(c *gin.Context) { t.Error("rejected token reached handler"); c.Status(204) })
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				if tc.name != "missing" {
					request.Header.Set("Authorization", "Bearer sk-"+key)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, tc.wantStatus, response.Code)
				var rows []model.ErrorLog
				require.NoError(t, db.Where("request_id = ?", tc.name).Find(&rows).Error)
				require.Len(t, rows, tc.wantLogs)
				if len(rows) > 0 {
					assert.NotContains(t, rows[0].Message, "请求失败，请稍后再尝试")
				}
			})
		}
	})
}

func TestResponsesWebSocketOriginalErrorLog(t *testing.T) {
	previousSave, previousOptions := common.SaveErrorLog, common.OptionMap
	common.SaveErrorLog = true
	common.OptionMap = map[string]string{"RewriteMessage": `["^original .* failure$"]`}
	t.Cleanup(func() { common.SaveErrorLog, common.OptionMap = previousSave, previousOptions })
	fixture := newResponsesWSBillingTest(t, `tier("request", fixed(0.002))`, func(ws *websocket.Conn, _ *http.Request) {
		for index := range 2 {
			if _, _, err := ws.ReadMessage(); !assert.NoError(t, err) {
				return
			}
			for _, event := range []string{
				fmt.Sprintf(`{"type":"response.created","response":{"id":"failed-%d","status":"in_progress"}}`, index),
				`{"type":"error","stream_id":"other","status":400,"error":{"code":"control_error","message":"original control failure"}}`,
				fmt.Sprintf(`{"type":"response.failed","trace_id":9007199254740993,"response":{"id":"failed-%d","status":"failed","error":{"code":"upstream_failed","message":"original websocket failure"},"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`, index),
			} {
				if !assert.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(event))) {
					return
				}
			}
		}
		_, _, _ = ws.ReadMessage()
	})
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.ErrorLog{}))
	for range 2 {
		require.NoError(t, fixture.client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","stream_id":"planner","model":"ws-billing","input":"hello"}`)))
		assert.Equal(t, "response.created", readResponsesWSTestEvent(t, fixture.client)["type"])
		control := readResponsesWSTestEvent(t, fixture.client)
		assert.Equal(t, "other", control["stream_id"])
		assert.Contains(t, control["error"].(map[string]any)["message"], "请求失败，请稍后再尝试")
		_, terminal, err := fixture.client.ReadMessage()
		require.NoError(t, err)
		assert.Contains(t, string(terminal), `"trace_id":9007199254740993`)
		assert.Contains(t, string(terminal), "请求失败，请稍后再尝试")
		assert.NotContains(t, string(terminal), "original websocket failure")
	}
	fixture.closeAndWait(t)
	for index := range 2 {
		var rows []model.ErrorLog
		require.NoError(t, model.LOG_DB.Where("request_id = ?", fmt.Sprintf("responses-ws-billing-ws-%d", index)).Order("id").Find(&rows).Error)
		require.Len(t, rows, 2)
		assert.Equal(t, "original control failure", rows[0].Message)
		assert.Contains(t, rows[1].Message, "original websocket failure")
		assert.NotContains(t, rows[1].Message, "请求失败，请稍后再尝试")
		assert.Equal(t, fixture.token.Id, rows[1].TokenId)
	}
}
