package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestRequestTimingDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
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
			default:
				driver = sqlite.Open(":memory:")
			}
			mainDriver := driver
			separate := false
			if dialect == "mysql" && os.Getenv("TEST_LOG_MYSQL_DSN") != "" {
				driver = mysql.Open(os.Getenv("TEST_LOG_MYSQL_DSN"))
				separate = true
			}
			if dialect == "postgres" && os.Getenv("TEST_LOG_POSTGRES_DSN") != "" {
				driver = postgres.New(postgres.Config{DSN: os.Getenv("TEST_LOG_POSTGRES_DSN"), PreferSimpleProtocol: true})
				separate = true
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "timing_" + strings.ReplaceAll(common.GetUUID(), "-", "") + "_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			mainDB := db
			if separate {
				mainDB, err = gorm.Open(mainDriver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "timing_main_" + strings.ReplaceAll(common.GetUUID(), "-", "") + "_"}})
				require.NoError(t, err)
				require.NoError(t, mainDB.AutoMigrate(&model.ErrorLog{}, &model.Token{}))
				t.Cleanup(func() {
					require.NoError(t, mainDB.Migrator().DropTable(&model.ErrorLog{}, &model.Token{}))
					conn, err := mainDB.DB()
					require.NoError(t, err)
					require.NoError(t, conn.Close())
				})
			}

			// Consume/settlement logging reads only these account fields. Keep the
			// fixture explicit without migrating unrelated authentication tables.
			userTable := mainDB.NamingStrategy.TableName("User")
			require.NoError(t, mainDB.Table(userTable).AutoMigrate(&struct {
				Id        int
				Username  string
				Setting   string
				DeletedAt gorm.DeletedAt
			}{}))
			require.NoError(t, mainDB.Table(userTable).Create(map[string]any{"id": 1, "username": "timing-log-user", "setting": "{}"}).Error)
			var version string
			versionSQL := "SELECT sqlite_version()"
			if dialect == "mysql" {
				versionSQL = "SELECT VERSION()"
			}
			if dialect == "postgres" {
				versionSQL = "SELECT version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("database: %s %s; separate log store: %t", dialect, version, separate)
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousTiming, previousErrors, previousDebug := common.RequestTimingEnabled, common.SaveErrorLog, common.DebugEnabled
			previousConsume, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
			previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = mainDB, db
			common.RequestTimingEnabled, common.SaveErrorLog = true, true
			common.DebugEnabled = false
			common.LogConsumeEnabled, common.DataExportEnabled = true, false
			common.RedisEnabled, common.MemoryCacheEnabled = false, false
			common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
			t.Cleanup(func() {
				require.NoError(t, mainDB.Migrator().DropTable(userTable))
				require.NoError(t, db.Migrator().DropTable(&model.ErrorLog{}, &model.Log{}, &model.Token{}))
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.RequestTimingEnabled, common.SaveErrorLog = previousTiming, previousErrors
				common.DebugEnabled = previousDebug
				common.LogConsumeEnabled, common.DataExportEnabled = previousConsume, previousExport
				common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
				common.SetDatabaseTypes(previousMain, previousLog)
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.ErrorLog{}, &model.Log{}, &model.Token{}))
			// Model a released error table without timing, preserving its other columns and indexes.
			require.NoError(t, db.Migrator().DropColumn(&model.ErrorLog{}, "timing"))
			require.NoError(t, db.Model(&model.ErrorLog{}).Create(map[string]any{"request_id": "legacy", "message": "legacy error", "status_code": 502, "created_at": time.Now().Unix()}).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.ErrorLog{}))
			}

			columns, err := db.Migrator().ColumnTypes(&model.ErrorLog{})
			require.NoError(t, err)
			for _, column := range columns {
				if column.Name() != "timing" {
					continue
				}
				expectedType := "text"
				if dialect == "mysql" {
					expectedType = "longtext"
				}
				if dialect == "postgres" {
					expectedType = "jsonb"
				}
				assert.Equal(t, expectedType, strings.ToLower(column.DatabaseTypeName()))
				nullable, known := column.Nullable()
				assert.True(t, known)
				assert.True(t, nullable)
			}
			var legacy model.ErrorLog
			require.NoError(t, db.Where("request_id = ?", "legacy").First(&legacy).Error)
			assert.Nil(t, legacy.Timing)
			assert.Equal(t, "legacy error", legacy.Message)
			assert.True(t, db.Migrator().HasIndex(&model.ErrorLog{}, "idx_error_request_id"))
			var timing *common.RequestTiming
			engine := gin.New()
			engine.Use(middleware.RequestTiming(), middleware.RequestId(), middleware.RequestErrorLog())
			engine.POST("/v1/responses", func(c *gin.Context) {
				timing = common.TimingFromContext(c.Request.Context())
				c.Set("id", 7)
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "timing-model")
				_, err := common.GetBodyStorage(c)
				require.NoError(t, err)
				defer common.CleanupBodyStorage(c)
				service.RecordRequestError(c, types.NewErrorWithStatusCode(io.ErrUnexpectedEOF, types.ErrorCodeBadResponse, 499))
				c.Status(499)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"timing-model"}`))
			engine.ServeHTTP(response, request)
			require.Equal(t, 499, response.Code)
			var stored model.ErrorLog
			require.NoError(t, db.Where("request_id = ?", response.Header().Get(common.RequestIdKey)).First(&stored).Error)
			require.NotNil(t, stored.Timing)
			var snapshot map[string]any
			require.NoError(t, common.UnmarshalJsonStr(string(*stored.Timing), &snapshot))
			assert.Equal(t, "error_log", snapshot["phase"])
			requestSnapshot := snapshot["request"].(map[string]any)
			assert.Nil(t, requestSnapshot["gateway_total_ms"])
			assert.Equal(t, float64(len(`{"model":"timing-model"}`)), requestSnapshot["body_bytes"])
			assert.NotNil(t, timing.Snapshot("request_end")["request"].(map[string]any)["gateway_total_ms"])
			text, err := model.GetErrorLogTiming(stored.Id)
			require.NoError(t, err)
			assert.JSONEq(t, string(*stored.Timing), text)
			entries, _, err := model.GetAllErrorLog(&hostdto.ErrorLogsRequest{StartTime: time.Now().Unix() - 60, Page: 1, PageSize: 10})
			require.NoError(t, err)
			require.Len(t, entries, 2)
			for _, entry := range entries {
				assert.Nil(t, entry.Timing)
			}
			var consumeCount int64
			require.NoError(t, db.Model(&model.Log{}).Count(&consumeCount).Error)
			assert.Zero(t, consumeCount)
			common.RequestTimingEnabled = false
			engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`)))
			var withoutTiming model.ErrorLog
			require.NoError(t, db.Order("id desc").First(&withoutTiming).Error)
			assert.Nil(t, withoutTiming.Timing)
			common.RequestTimingEnabled = true
			engine.POST("/consume", func(c *gin.Context) {
				model.RecordConsumeLog(c, 1, model.RecordConsumeLogParams{ModelName: "timing-model", Quota: 321, PromptTokens: 10, CompletionTokens: 20, UseTimeSeconds: 3})
				c.Status(http.StatusOK)
			})
			engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/consume", nil))
			var consume model.Log
			require.NoError(t, db.First(&consume).Error)
			assert.Equal(t, 321, consume.Quota)
			assert.Equal(t, 10, consume.PromptTokens)
			assert.Equal(t, 20, consume.CompletionTokens)
			assert.Equal(t, 3, consume.UseTime)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(consume.Other, &other))
			metadata := other["admin_info"].(map[string]any)
			assert.Equal(t, "consume_log", metadata["timing"].(map[string]any)["phase"])
			require.NoError(t, db.Model(&model.Log{}).Count(&consumeCount).Error)
			assert.EqualValues(t, 1, consumeCount, "timing only enriches the existing consumption entry")
			taskCtx, _ := common.NewRequestTiming(t.Context(), "settlement-operation", "", nil)
			model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{TimingContext: taskCtx, UserId: 1, LogType: model.LogTypeRefund, Quota: 2})
			var settlement model.Log
			require.NoError(t, db.Order("id desc").First(&settlement).Error)
			assert.Equal(t, model.LogTypeRefund, settlement.Type)
			assert.Equal(t, -2, settlement.Quota)
			assert.Contains(t, settlement.Other, "settlement-operation")

		})
	}
}

func TestRequestTimingServiceLogsRequireDebug(t *testing.T) {
	for _, tc := range []struct {
		timing bool
		debug  bool
	}{
		{false, false}, {false, true}, {true, false}, {true, true},
	} {
		t.Run(fmt.Sprintf("timing=%t/debug=%t", tc.timing, tc.debug), func(t *testing.T) {
			previousTiming, previousDebug := common.RequestTimingEnabled, common.DebugEnabled
			common.RequestTimingEnabled, common.DebugEnabled = tc.timing, tc.debug
			t.Cleanup(func() { common.RequestTimingEnabled, common.DebugEnabled = previousTiming, previousDebug })
			output, err := os.CreateTemp(t.TempDir(), "timing-*.log")
			require.NoError(t, err)
			common.LogWriterMu.Lock()
			previousWriter, previousErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
			gin.DefaultWriter, gin.DefaultErrorWriter = output, output
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultWriter, gin.DefaultErrorWriter = previousWriter, previousErrorWriter
				common.LogWriterMu.Unlock()
				require.NoError(t, output.Close())
			})
			var timing *common.RequestTiming
			var attempt *common.TimingAttempt
			var call *common.TimingCall
			var consumeSnapshot map[string]any
			engine := gin.New()
			engine.Use(middleware.RequestTiming())
			engine.POST("/test", func(c *gin.Context) {
				_, err := common.GetBodyStorage(c)
				require.NoError(t, err)
				defer common.CleanupBodyStorage(c)
				ctx := c.Request.Context()
				timing = common.TimingFromContext(ctx)
				ctx, attempt = common.BeginTimingAttempt(ctx, 7)
				call = common.BeginTimingOperation(ctx, "inference", true)
				consumeSnapshot = common.TimingSnapshot(ctx, "consume_log")
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("body")))
			require.Equal(t, http.StatusNoContent, response.Code)
			// Call and attempt completion after the HTTP boundary use independent events.
			call.End()
			attempt.End()
			background := common.BackgroundTimingContext("task_poll", "task-1")
			common.BeginTimingOperation(background, "poll", false).End()
			scopeCtx, scope := common.BeginTimingOperationScope(t.Context(), "file_prepare", "task-2")
			common.BeginTimingOperation(scopeCtx, "file_upload", false).End()
			scope.EndOperation()
			if tc.timing {
				require.NotNil(t, consumeSnapshot, "DEBUG must not disable snapshots used by existing database logs")
				assert.Equal(t, int64(4), consumeSnapshot["request"].(map[string]any)["body_bytes"])
				assert.NotNil(t, timing.Snapshot("request_end")["request"].(map[string]any)["gateway_total_ms"])
				require.NotNil(t, common.TimingSnapshot(background, "consume_log"))
				require.NotNil(t, common.TimingSnapshot(scopeCtx, "consume_log"))
			} else {
				assert.Nil(t, consumeSnapshot)
				assert.Nil(t, common.TimingSnapshot(background, "consume_log"))
				assert.Nil(t, common.TimingSnapshot(scopeCtx, "consume_log"))
			}
			data, err := os.ReadFile(output.Name())
			require.NoError(t, err)
			if !tc.timing || !tc.debug {
				assert.NotContains(t, string(data), `"event":"request_timing"`)
				return
			}
			phases := make(map[string]int)
			for line := range strings.SplitSeq(string(data), "\n") {
				if !strings.Contains(line, `"event":"request_timing"`) {
					continue
				}
				var event map[string]any
				require.NoError(t, common.UnmarshalJsonStr(line[strings.IndexByte(line, '{'):], &event))
				phases[event["phase"].(string)]++
			}
			assert.Equal(t, map[string]int{"request_end": 1, "call_end": 1, "attempt_end": 1, "operation_end": 2}, phases)
		})
	}
}

func TestRequestTimingHTTPStagesAndCancellation(t *testing.T) {
	ctx, timing := common.NewRequestTiming(t.Context(), "timing-request", "", nil)
	ctx, attempt := common.BeginTimingAttempt(ctx, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "payload", string(data))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: hello\n\n")
	}))
	t.Cleanup(server.Close)
	req := httptest.NewRequest(http.MethodPost, server.URL, strings.NewReader("payload"))
	req.RequestURI = ""
	resp, err := common.DoTimedHTTP(ctx, server.Client(), req, "inference", true)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	attempt.End()
	before := timing.Snapshot("error_log")
	call := before["outbound_calls"].([]map[string]any)[0]
	assert.Equal(t, int64(7), call["request_body_bytes"])
	assert.Equal(t, int64(13), call["response_body_bytes"])
	assert.Equal(t, "eof", call["body_end_kind"])
	assert.Equal(t, true, call["request_write_succeeded"])
	assert.NotNil(t, call["response_headers_ready_at_ms"])
	assert.Nil(t, before["request"].(map[string]any)["gateway_total_ms"])
	timing.Finish()
	assert.Nil(t, before["request"].(map[string]any)["gateway_total_ms"], "snapshots must remain frozen")
	// A canceled downstream context must not replace a detached upstream context.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	detached := common.WithTimingContext(t.Context(), canceled)
	assert.NoError(t, detached.Err())
}

func TestRequestTimingSeparatesAuthenticationDecodeAndBusinessValidation(t *testing.T) {
	previousTiming, previousErrors := common.RequestTimingEnabled, common.SaveErrorLog
	common.RequestTimingEnabled, common.SaveErrorLog = true, false
	t.Cleanup(func() { common.RequestTimingEnabled, common.SaveErrorLog = previousTiming, previousErrors })
	for _, tc := range []struct {
		name string
		body string
		tags []string
	}{
		{"missing model", `{"input":"hello","tools":[{"type":"function","function":{"name":"noop"}}]}`, []string{"tools"}},
		{"malformed JSON", `{"input":`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var timing *common.RequestTiming
			engine := gin.New()
			engine.Use(middleware.RequestTiming())
			engine.POST("/v1/responses", func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				timing = common.TimingFromContext(c.Request.Context())
				require.NoError(t, middleware.SetupContextForToken(c, &model.Token{UserId: 1}))
				assert.Equal(t, tc.tags, c.GetStringSlice("multi_model_tags"))
				beforeValidation := timing.Snapshot("auth_done")["request"].(map[string]any)
				assert.Nil(t, beforeValidation["request_parse_ms"], "authentication decoding must not populate business validation timing")
				RelayWithoutRace(c, types.RelayFormatOpenAIResponses)
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code)
			snapshot := timing.Snapshot("request_end")
			assert.Equal(t, 2, snapshot["schema_version"])
			spans := snapshot["business_spans"].([]map[string]any)
			require.Len(t, spans, 3)
			assert.Equal(t, "auth_tag_decode", spans[0]["phase"])
			assert.Equal(t, "request_parse", spans[1]["phase"])
			assert.Equal(t, "request_decode", spans[2]["phase"])
			requestSnapshot := snapshot["request"].(map[string]any)
			offsets := requestSnapshot["offsets_ms"].(map[string]any)
			assert.Equal(t, spans[1]["started_at_ms"], offsets["request_parse_started_at_ms"])
			assert.Equal(t, spans[1]["ended_at_ms"], offsets["request_parse_ended_at_ms"])
			assert.Equal(t, spans[1]["duration_ms"], requestSnapshot["request_parse_ms"])
			assert.Equal(t, int64(len(tc.body)), requestSnapshot["body_bytes"], "decoding cached bodies must not read the incoming body again")
		})
	}
}

// The body is blocked before its first Read returns, so a snapshot must show
// that ingress started while neither routing nor an upstream call has begun.
func TestRequestTimingPendingIngressAndReplay(t *testing.T) {
	previous := common.RequestTimingEnabled
	common.RequestTimingEnabled = true
	t.Cleanup(func() { common.RequestTimingEnabled = previous })
	reader, writer := io.Pipe()
	started := make(chan *common.RequestTiming, 1)
	finished := make(chan struct{})
	engine := gin.New()
	engine.Use(middleware.RequestTiming())
	engine.POST("/upload", func(c *gin.Context) {
		timing := common.TimingFromContext(c.Request.Context())
		body := c.Request.Body
		c.Request.Body = &timingReadSignal{ReadCloser: body, signal: func() { started <- timing }}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			panic(err)
		}
		defer common.CleanupBodyStorage(c)
		for range 2 {
			_, err = storage.Seek(0, io.SeekStart)
			if err != nil {
				panic(err)
			}
			if _, err = io.Copy(io.Discard, storage); err != nil {
				panic(err)
			}
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/upload", reader)
	request.ContentLength = -1
	go func() { defer close(finished); engine.ServeHTTP(httptest.NewRecorder(), request) }()
	timing := <-started
	// The signal wrapper executes before the timing reader; allow a deterministic
	// Read boundary handshake by making the pipe supply one byte first.
	_, err := writer.Write([]byte("x"))
	require.NoError(t, err)
	pending := timing.Snapshot("error_log")
	requestSnapshot := pending["request"].(map[string]any)
	offsets := requestSnapshot["offsets_ms"].(map[string]any)
	assert.NotNil(t, offsets["body_read_started_at_ms"])
	assert.Nil(t, offsets["body_read_ended_at_ms"])
	assert.Nil(t, requestSnapshot["pre_upstream_ms"])
	require.NoError(t, writer.Close())
	<-finished
	final := timing.Snapshot("request_end")["request"].(map[string]any)
	assert.Equal(t, int64(1), final["body_bytes"], "cached replays cannot count ingress twice")
	assert.Equal(t, "complete", final["body_state"])
	assert.Nil(t, requestSnapshot["gateway_total_ms"], "an earlier snapshot must stay frozen")
}

type timingReadSignal struct {
	io.ReadCloser
	signal func()
}

func (r *timingReadSignal) Read(p []byte) (int, error) {
	if r.signal != nil {
		r.signal()
		r.signal = nil
	}
	return r.ReadCloser.Read(p)
}

type timingFailureWriter struct{ gin.ResponseWriter }

func (w timingFailureWriter) Write(data []byte) (int, error) {
	return min(2, len(data)), io.ErrClosedPipe
}
func (w timingFailureWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w timingFailureWriter) FlushError() error                    { return io.ErrClosedPipe }

func TestRequestTimingDownstreamAndRecovery(t *testing.T) {
	previous := common.RequestTimingEnabled
	common.RequestTimingEnabled = true
	t.Cleanup(func() { common.RequestTimingEnabled = previous })
	for _, name := range []string{"partial_write", "head", "panic"} {
		t.Run(name, func(t *testing.T) {
			var timing *common.RequestTiming
			engine := gin.New()
			if name == "partial_write" {
				engine.Use(func(c *gin.Context) { c.Writer = timingFailureWriter{c.Writer}; c.Next() })
			}
			engine.Use(middleware.RequestTiming(), gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) { c.AbortWithStatus(500) }))
			engine.Any("/test", func(c *gin.Context) {
				timing = common.TimingFromContext(c.Request.Context())
				if name == "panic" {
					panic("test panic")
				}
				if name == "head" {
					c.Status(http.StatusNoContent)
					return
				}
				n, err := c.Writer.WriteString("hello")
				assert.Equal(t, 2, n)
				assert.ErrorIs(t, err, io.ErrClosedPipe)
				err = http.NewResponseController(c.Writer).Flush()
				assert.ErrorIs(t, err, io.ErrClosedPipe)
			})
			method := http.MethodGet
			if name == "head" {
				method = http.MethodHead
			}
			engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/test", nil))
			snapshot := timing.Snapshot("request_end")
			request := snapshot["request"].(map[string]any)
			assert.NotNil(t, request["gateway_total_ms"])
			assert.Equal(t, "no_body", request["body_state"])
			downstream := snapshot["downstream"].(map[string]any)
			if name == "partial_write" {
				assert.Equal(t, int64(2), downstream["body_bytes"])
				assert.Equal(t, int64(1), downstream["write_error_count"])
				assert.Equal(t, int64(1), downstream["flush_error_count"])
			} else {
				assert.Nil(t, downstream["first_write_at_ms"])
				assert.Zero(t, downstream["body_bytes"])
			}
		})
	}
}

func TestRequestTimingLateCallbacksAndOpaqueSDK(t *testing.T) {
	var emitted []map[string]any
	ctx, timing := common.NewRequestTiming(t.Context(), "late-request", "", func(value map[string]any) { emitted = append(emitted, value) })
	ctx, attempt := common.BeginTimingAttempt(ctx, 9)
	phase := common.StartTimingPhase(ctx, "task_wait")
	call := common.BeginTimingOperation(ctx, "file_upload", false)
	timing.Finish()
	before := timing.Snapshot("request_end")
	opaque := before["outbound_calls"].([]map[string]any)[0]
	assert.Nil(t, opaque["upstream_body_write_ms"])
	assert.Nil(t, opaque["upstream_read_call_sum_ms"])
	assert.Equal(t, "unsupported_transport", opaque["missing_reason"])
	assert.Equal(t, "pending_at_finalize", opaque["status"])
	phase.End()
	call.End()
	attempt.End()
	assert.Equal(t, "pending_at_finalize", opaque["status"])
	assert.Nil(t, before["business_spans"].([]map[string]any)[0]["ended_at_ms"])
	require.Len(t, emitted, 2)
	assert.Equal(t, "outbound_call", emitted[0]["scope"])
	assert.Equal(t, "late-request", emitted[1]["request_id"])
}

// A redirect retains one logical call and counts the payload sent on both
// HTTP exchanges; no observer may replay the body a third time.
func TestRequestTimingRedirectKeepsExistingClientAndTrace(t *testing.T) {
	ctx, timing := common.NewRequestTiming(t.Context(), "redirect-request", "", nil)
	var writes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "payload", string(data))
		if r.URL.Path == "/first" {
			http.Redirect(w, r, "/last", http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/first", strings.NewReader("payload"))
	require.NoError(t, err)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { writes.Add(1) }}))
	client := &http.Client{Transport: timingTestTransport{http.DefaultTransport}}
	resp, err := common.DoTimedHTTP(ctx, client, req, "inference", true)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	call := timing.Snapshot("request_end")["outbound_calls"].([]map[string]any)[0]
	assert.EqualValues(t, 2, writes.Load(), "existing httptrace callbacks are composed")
	assert.Equal(t, int64(14), call["request_body_bytes"])
	assert.Equal(t, "ambiguous_transport_retry", call["missing_reason"])
	assert.Nil(t, call["upstream_wait_headers_ms"])
	assert.Len(t, call["transport_exchanges"], 2)
}

type timingTestTransport struct{ http.RoundTripper }

func (t timingTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return common.TimedRoundTrip(t.RoundTripper, req)
}

func TestErrorTimingDetailRequiresRoot(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.ErrorLog{}))
	stored := model.ErrorLog{Message: "upstream failed", Timing: common.GetPointer(model.LogJSON(`{"schema_version":1}`))}
	require.NoError(t, model.LOG_DB.Create(&stored).Error)
	engine := gin.New()
	engine.GET("/timing/:id", middleware.RootAuth(), GetErrorLogTiming)
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		user.Role = role
		require.NoError(t, model.DB.Model(user).Update("role", role).Error)
		bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "timing-detail-test")
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/timing/%d", stored.Id), nil)
		req.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if role != common.RoleRootUser {
			assert.NotContains(t, response.Body.String(), "schema_version")
			continue
		}
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "schema_version")
	}
}

// Measures collector bookkeeping plus final JSON encoding, excluding network
// latency and the configured log sink. It is not an end-to-end load test.
func BenchmarkRequestTimingCollector(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("enabled=%t", enabled), func(b *testing.B) {
			b.ReportAllocs()
			var snapshotBytes int
			for b.Loop() {
				ctx := context.Background()
				var timing *common.RequestTiming
				if enabled {
					ctx, timing = common.NewRequestTiming(ctx, "benchmark", "", nil)
				}
				phase := common.StartTimingPhase(ctx, "auth")
				phase.End()
				if timing != nil {
					for range 64 {
						timing.ObserveDownstream("write", time.Microsecond, 128, true, true)
					}
					timing.Finish()
					data, err := common.Marshal(timing.Snapshot("request_end"))
					if err != nil {
						b.Fatal(err)
					}
					snapshotBytes = len(data)
					if len(data) == 0 {
						b.Fatal(errors.New("empty snapshot"))
					}
				}
			}
			b.ReportMetric(float64(snapshotBytes), "snapshot-bytes/op")
		})
	}
}

func TestRequestTimingResponsesSeparatesStartupOutputAndTerminal(t *testing.T) {
	ctx, timing := common.NewRequestTiming(t.Context(), "responses-events", "", nil)
	ctx, attempt := common.BeginTimingAttempt(ctx, 3)
	defer attempt.End()
	accumulator := service.NewResponsesUsageAccumulator(&relaycommon.RelayInfo{TimingContext: ctx})
	for _, wire := range []string{`{"type":"response.created"}`, `{"type":"vendor.unknown.delta","delta":"unrecognized"}`} {
		var event kitdto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(wire, &event))
		accumulator.Observe(&event, []byte(wire))
	}
	started := timing.Snapshot("error_log")["attempts"].([]map[string]any)[0]
	assert.NotNil(t, started["first_protocol_event_at_ms"])
	assert.Nil(t, started["first_output_at_ms"])
	assert.Nil(t, started["protocol_terminal_at_ms"])
	accumulator.Observe(&kitdto.ResponsesStreamResponse{Type: "response.reasoning_text.delta", Delta: "thinking"}, nil)
	accumulator.Observe(&kitdto.ResponsesStreamResponse{Type: "response.output_text.done"}, nil)
	output := timing.Snapshot("error_log")["attempts"].([]map[string]any)[0]
	assert.NotNil(t, output["first_reasoning_at_ms"])
	assert.Equal(t, "reasoning", output["first_output_kind"])
	assert.Nil(t, output["protocol_terminal_at_ms"], "a local done is not the response terminal")
	accumulator.Observe(&kitdto.ResponsesStreamResponse{Type: "response.incomplete"}, nil)
	terminal := timing.Snapshot("error_log")["attempts"].([]map[string]any)[0]
	assert.NotNil(t, terminal["protocol_terminal_at_ms"])
	assert.Equal(t, "response.incomplete", terminal["protocol_terminal_kind"])
}

func TestRequestTimingExcludesSuccessfulWebSocketLifetime(t *testing.T) {
	previous := common.RequestTimingEnabled
	common.RequestTimingEnabled = true
	t.Cleanup(func() { common.RequestTimingEnabled = previous })
	observed := make(chan *common.RequestTiming, 1)
	finished := make(chan struct{})
	engine := gin.New()
	engine.Use(middleware.RequestTiming())
	engine.GET("/socket", func(c *gin.Context) {
		timing := common.TimingFromContext(c.Request.Context())
		observed <- timing
		conn, err := (&websocket.Upgrader{}).Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	})
	// Handler completion is signaled by the outer HTTP server, after Gin's
	// middleware finalizer has had a chance to skip the hijacked connection.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { engine.ServeHTTP(w, r); close(finished) }))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/socket", nil)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	timing := <-observed
	<-finished
	assert.Nil(t, timing.Snapshot("request_end")["request"].(map[string]any)["gateway_total_ms"])
}
