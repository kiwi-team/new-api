package relay

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func configureGeminiFileRelayTest(t *testing.T) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{model_setting.GeminiFileRoutingKey: `{"enabled":true,"mode":"forward","chat_model_patterns":["gemini-test"]}`}
	common.OptionMapRWMutex.Unlock()
	protection := system_setting.GetFetchSetting()
	previousProtection := *protection
	protection.EnableSSRFProtection = true
	protection.ApplyIPFilterForDomain = true
	previousTLS := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = false
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		*protection = previousProtection
		common.TLSInsecureSkipVerify = previousTLS
	})
	service.InitHttpClient()
}

func TestGeminiInlineFilesPreserveUpstreamErrors(t *testing.T) {
	configureGeminiFileRelayTest(t)
	const message = "Image generation is not supported for this model"
	const upstreamError = `{"error":{"code":400,"message":"` + message + `","status":"INVALID_ARGUMENT"}}`
	for _, endpoint := range []struct {
		name           string
		native, stream bool
	}{
		{name: "native JSON", native: true},
		{name: "native SSE", native: true, stream: true},
		{name: "chat JSON"},
		{name: "chat SSE", stream: true},
	} {
		for _, tc := range []struct {
			name              string
			remote, mixed     bool
			transport, mapped bool
		}{
			{name: "inline upstream error"},
			{name: "inline status mapping", mapped: true},
			{name: "inline transport error", transport: true},
			{name: "remote reference upstream error", remote: true},
			{name: "remote reference transport error", remote: true, transport: true},
			{name: "mixed attachment upstream error", remote: true, mixed: true},
		} {
			t.Run(endpoint.name+"/"+tc.name, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, upstreamError)
				}))
				t.Cleanup(upstream.Close)
				path := "/v1/chat/completions"
				inline := `{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}`
				remote := `{"type":"image_url","image_url":{"url":"https://generativelanguage.googleapis.com/v1beta/files/test","mime_type":"image/png"}}`
				if endpoint.native {
					path = "/v1beta/models/gemini-test:generateContent"
					if endpoint.stream {
						path = "/v1beta/models/gemini-test:streamGenerateContent"
					}
					inline = `{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}`
					remote = `{"fileData":{"mimeType":"image/png","fileUri":"https://generativelanguage.googleapis.com/v1beta/files/test"}}`
				}
				attachments := inline
				if tc.remote {
					attachments = remote
					if tc.mixed {
						attachments = inline + "," + remote
					}
				}
				body := `{"model":"gemini-test","messages":[{"role":"user","content":[` + attachments + `]}]}`
				if endpoint.native {
					body = `{"contents":[{"role":"user","parts":[{"text":"Generate an image"},` + attachments + `]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"],"imageConfig":{"aspectRatio":"16:9","imageSize":"2K"}}}`
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
				common.SetContextKey(c, constant.ContextKeyChannelKey, "selected-service-key")
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-test")
				if tc.mapped {
					c.Set("status_code_mapping", `{"400":422}`)
				}
				plan, apiErr := service.PlanGeminiFiles(c, "gemini-test")
				require.Nil(t, apiErr)
				require.NotNil(t, plan)
				t.Cleanup(func() { plan.Close(c) })
				assert.False(t, plan.RequiresStorage, "even small remote references must retain their error redaction")
				if tc.transport {
					ctx, cancel := context.WithCancel(c.Request.Context())
					cancel()
					c.Request = c.Request.WithContext(ctx)
				}
				info := &relaycommon.RelayInfo{
					OriginModelName: "gemini-test", RequestURLPath: path, IsStream: endpoint.stream,
					RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
				}
				if endpoint.native {
					var request dto.GeminiChatRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					info.Request = &request
					info.RelayFormat = types.RelayFormatGemini
					info.RelayMode = relayconstant.RelayModeGemini
					apiErr = GeminiHelper(c, info)
				} else {
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(body, &request))
					request.Stream = &endpoint.stream
					info.Request = &request
					apiErr = TextHelper(c, info)
				}
				require.NotNil(t, apiErr)
				if tc.remote {
					assert.Equal(t, types.ErrorCode("file_inference_failed"), apiErr.GetErrorCode())
					assert.NotContains(t, apiErr.Error(), message)
					if tc.transport {
						assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					} else {
						assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
					}
					return
				}
				if tc.transport {
					assert.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
					assert.Contains(t, apiErr.ToOpenAIError().Message, "do request failed")
					return
				}
				assert.Equal(t, types.ErrorCode("400"), apiErr.GetErrorCode())
				assert.EqualError(t, apiErr, message)
				assert.Equal(t, message, apiErr.ToOpenAIError().Message)
				if tc.mapped {
					assert.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
				} else {
					assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				}
			})
		}
	}
}

func TestGeminiFileRequestLoggingDatabaseMatrix(t *testing.T) {
	configureGeminiFileRelayTest(t)
	// The complete image exceeds MySQL TEXT's 64 KiB limit after base64 encoding.
	imageData := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 64<<10)))
	settings := model_setting.GetGlobalSettings()
	previousSettings := *settings
	settings.PassThroughRequestEnabled = false
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		*settings = previousSettings
		constant.StreamingTimeout = previousTimeout
	})
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, migration := range []string{"fresh", "upgrade release", "upgrade previous"} {
				t.Run(migration, func(t *testing.T) {
					db, logDB := setupGeminiFileLogDatabases(t, dialect, migration)
					user := model.User{Username: "gemini-log-owner", AffCode: "gemini-log", Quota: 1_000_000}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: "gemini-log-token", UnlimitedQuota: true}
					require.NoError(t, db.Create(&token).Error)
					channel := model.Channel{Name: "gemini-log-channel", Type: constant.ChannelTypeGemini}
					require.NoError(t, db.Create(&channel).Error)
					for _, tc := range []struct {
						name                                                  string
						native, stream, remote, passthrough, failed, disabled bool
					}{
						{name: "native JSON", native: true},
						{name: "native SSE", native: true, stream: true},
						{name: "native passthrough with URL", native: true, remote: true, passthrough: true},
						{name: "native error with URL", native: true, remote: true, failed: true},
						{name: "native logging disabled", native: true, disabled: true},
						{name: "chat JSON with URL", remote: true},
						{name: "chat SSE", stream: true},
						{name: "chat passthrough with URL", remote: true, passthrough: true},
						{name: "chat error with URL", remote: true, failed: true},
						{name: "chat logging disabled", disabled: true},
					} {
						t.Run(tc.name, func(t *testing.T) {
							t.Setenv("SAVE_REQUEST_RESPONSE", fmt.Sprint(!tc.disabled))
							const fileURL = "https://generativelanguage.googleapis.com/v1beta/files/private?signature=original-value"
							path := "/v1/chat/completions"
							attachments := `{"type":"image_url","image_url":{"url":"data:image/png;base64,` + imageData + `"}}`
							if tc.remote {
								attachments += `,{"type":"image_url","image_url":{"url":"` + fileURL + `","mime_type":"image/png"}}`
							}
							body := fmt.Sprintf(`{
  "model":"gemini-test","stream":%t,
  "messages":[{"role":"user","content":[{"type":"text","text":"保留完整请求"},%s]}],
  "future_option":{"keep":true}
}`, tc.stream, attachments)
							if tc.native {
								path = "/v1beta/models/gemini-test:generateContent"
								if tc.stream {
									path = "/v1beta/models/gemini-test:streamGenerateContent"
								}
								attachments = `{"inlineData":{"mimeType":"image/png","data":"` + imageData + `"}}`
								if tc.remote {
									attachments += `,{"fileData":{"mimeType":"image/png","fileUri":"` + fileURL + `"}}`
								}
								body = fmt.Sprintf(`{
  "contents":[{"role":"user","parts":[{"text":"保留完整请求"},{"functionResponse":{"name":"lookup","response":{"ok":true},"parts":[%s]}}]}],
  "generationConfig":{"temperature":0},"future_option":{"keep":true}
}`, attachments)
							}
							captured := make(chan string, 1)
							upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								data, err := io.ReadAll(r.Body)
								assert.NoError(t, err)
								captured <- string(data)
								if tc.failed {
									w.Header().Set("Content-Type", "application/json")
									w.WriteHeader(http.StatusBadRequest)
									_, _ = io.WriteString(w, `{"error":{"message":"file rejected"}}`)
									return
								}
								const response = `{"candidates":[{"content":{"role":"model","parts":[{"text":"file received"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":4,"totalTokenCount":14}}`
								if tc.stream {
									w.Header().Set("Content-Type", "text/event-stream")
									_, _ = io.WriteString(w, "data: "+response+"\n\n")
								} else {
									w.Header().Set("Content-Type", "application/json")
									_, _ = io.WriteString(w, response)
								}
							}))
							t.Cleanup(upstream.Close)
							c, _ := gin.CreateTestContext(httptest.NewRecorder())
							c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
							c.Request.Header.Set("Content-Type", "application/json")
							t.Cleanup(func() { common.CleanupBodyStorage(c) })
							requestID := dialect + "/" + tc.name
							c.Set(common.RequestIdKey, requestID)
							c.Set("id", user.Id)
							common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
							common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
							common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
							common.SetContextKey(c, constant.ContextKeyChannelKey, "selected-service-key")
							common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-test")
							common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passthrough})
							if !tc.passthrough {
								common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"log_probe": "upstream-only"})
							}
							plan, apiErr := service.PlanGeminiFiles(c, "gemini-test")
							require.Nil(t, apiErr)
							require.NotNil(t, plan)
							t.Cleanup(func() { plan.Close(c) })
							info := &relaycommon.RelayInfo{
								UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TokenUnlimited: true,
								OriginModelName: "gemini-test", RequestURLPath: path, IsStream: tc.stream,
								UsingGroup: "default", StartTime: time.Now(),
								RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
							}
							if tc.native {
								var request dto.GeminiChatRequest
								require.NoError(t, common.UnmarshalJsonStr(body, &request))
								info.Request, info.RelayFormat, info.RelayMode = &request, types.RelayFormatGemini, relayconstant.RelayModeGemini
								apiErr = GeminiHelper(c, info)
							} else {
								var request dto.GeneralOpenAIRequest
								require.NoError(t, common.UnmarshalJsonStr(body, &request))
								info.Request = &request
								apiErr = TextHelper(c, info)
							}
							if tc.failed {
								require.NotNil(t, apiErr)
								service.RecordRequestError(c, apiErr)
								var stored model.ErrorLog
								require.NoError(t, logDB.Where("request_id = ?", requestID).Take(&stored).Error)
								assert.Equal(t, body, stored.Body)
							} else {
								require.Nil(t, apiErr)
								var stored model.Log
								require.NoError(t, logDB.Where("request_id = ?", requestID).Take(&stored).Error)
								if tc.disabled {
									assert.Empty(t, stored.Request)
									assert.Empty(t, stored.Response)
								} else {
									assert.Equal(t, body, stored.Request)
									assert.Contains(t, stored.Response, "file received")
								}
							}
							wire := <-captured
							assert.Contains(t, wire, imageData)
							if tc.remote {
								assert.Contains(t, wire, fileURL)
							}
							if !tc.passthrough {
								assert.Equal(t, "upstream-only", gjson.Get(wire, "log_probe").String())
							}
						})
					}
				})
			}
		})
	}
}

// The released v1.0.0-rc.47 Log schema has no request column or error_logs table.
type geminiReleasedLog struct {
	Id                int   `gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int   `gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64 `gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int   `gorm:"index:idx_created_at_type"`
	Content           string
	Username          string `gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `gorm:"index;default:''"`
	ModelName         string `gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `gorm:"default:0"`
	PromptTokens      int    `gorm:"default:0"`
	CompletionTokens  int    `gorm:"default:0"`
	UseTime           int    `gorm:"default:0"`
	IsStream          bool
	ChannelId         int    `gorm:"index"`
	ChannelName       string `gorm:"->"`
	TokenId           int    `gorm:"default:0;index"`
	Group             string `gorm:"index"`
	Ip                string `gorm:"index;default:''"`
	RequestId         string `gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string
}

func setupGeminiFileLogDatabases(t *testing.T, dialect, migration string) (*gorm.DB, *gorm.DB) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory, previousBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	previousConsume, previousError, previousExport := common.LogConsumeEnabled, common.SaveErrorLog, common.DataExportEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = previousRedis, previousMemory, previousBatch
		common.LogConsumeEnabled, common.SaveErrorLog, common.DataExportEnabled = previousConsume, previousError, previousExport
	})
	var databases [2]*gorm.DB
	for index, suffix := range []string{"_DSN", "_LOG_DSN"} {
		var driver gorm.Dialector = sqlite.Open(":memory:")
		if dialect != "sqlite" {
			key := "TEST_" + strings.ToUpper(dialect)
			dsn := os.Getenv(key + suffix)
			if dsn == "" {
				dsn = os.Getenv(key + "_DSN")
			}
			if dsn == "" {
				t.Skip(key + "_DSN is not configured")
			}
			if dialect == "mysql" {
				driver = mysql.Open(dsn)
			} else {
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
		}
		db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("gemini_log_%d_%d_", time.Now().UnixNano(), index)}})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		models := []any{&model.User{}, &model.Token{}, &model.Channel{}}
		if index == 1 {
			models = []any{&model.Log{}, &model.ErrorLog{}}
		}
		t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
		logTable := db.NamingStrategy.TableName("Log")
		errorTable := db.NamingStrategy.TableName("ErrorLog")
		var previousIndexes []gorm.Index
		if index == 1 && migration != "fresh" {
			if migration == "upgrade release" {
				require.NoError(t, db.Table(logTable).AutoMigrate(&geminiReleasedLog{}))
			} else {
				legacyLog := &struct {
					model.Log
					Request string `gorm:"type:text"`
				}{}
				legacyError := &struct {
					model.ErrorLog
					Body string `gorm:"default:''"`
				}{}
				require.NoError(t, db.Table(logTable).AutoMigrate(legacyLog))
				require.NoError(t, db.Table(errorTable).AutoMigrate(legacyError))
				require.NoError(t, db.Table(errorTable).Create(map[string]any{"id": 100, "body": "existing error body"}).Error)
			}
			preserved := map[string]any{
				"id": 100, "request_id": "migration-preserved", "content": "existing log content", "quota": 123,
			}
			if migration == "upgrade previous" {
				preserved["request"] = `{"existing":"request body"}`
			}
			require.NoError(t, db.Table(logTable).Create(preserved).Error)
			previousIndexes, err = db.Migrator().GetIndexes(&model.Log{})
			require.NoError(t, err)
		}
		var alterations []string
		require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("gemini_log_migration", func(tx *gorm.DB) {
			if sql := tx.Statement.SQL.String(); strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "ALTER TABLE") {
				alterations = append(alterations, sql)
			}
		}))
		for attempt := range 2 {
			alterations = nil
			require.NoError(t, db.AutoMigrate(models...))
			if attempt == 1 && index == 1 {
				assert.Empty(t, alterations, "repeated startup must not re-alter migrated columns")
			}
		}
		if index == 1 && migration != "fresh" {
			var preserved model.Log
			require.NoError(t, db.Where("request_id = ?", "migration-preserved").Take(&preserved).Error)
			assert.Equal(t, "existing log content", preserved.Content)
			assert.Equal(t, 123, preserved.Quota)
			indexes, err := db.Migrator().GetIndexes(&model.Log{})
			require.NoError(t, err)
			byName := make(map[string]gorm.Index, len(indexes))
			for _, current := range indexes {
				byName[current.Name()] = current
			}
			for _, previous := range previousIndexes {
				current, ok := byName[previous.Name()]
				require.True(t, ok, "existing index %s must survive migration", previous.Name())
				assert.Equal(t, previous.Columns(), current.Columns())
				wasUnique, _ := previous.Unique()
				isUnique, _ := current.Unique()
				assert.Equal(t, wasUnique, isUnique)
			}
			if migration == "upgrade previous" {
				assert.Equal(t, `{"existing":"request body"}`, preserved.Request)
				var preservedError model.ErrorLog
				require.NoError(t, db.First(&preservedError, 100).Error)
				assert.Equal(t, "existing error body", preservedError.Body)
			}
		}
		versionSQL := "SELECT version()"
		if dialect == "sqlite" {
			versionSQL = "SELECT sqlite_version()"
		}
		var version string
		require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
		t.Logf("%s database %d: %s", dialect, index, version)
		databases[index] = db
	}
	model.DB, model.LOG_DB = databases[0], databases[1]
	common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled, common.SaveErrorLog, common.DataExportEnabled = true, true, false
	return databases[0], databases[1]
}
