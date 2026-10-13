package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/happyoyster"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestValidateHappyOysterFirstFrame(t *testing.T) {
	encodePNG := func(width, height int) string {
		var output bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		img.Set(0, 0, color.White)
		require.NoError(t, png.Encode(&output, img))
		return base64.StdEncoding.EncodeToString(output.Bytes())
	}

	assert.NoError(t, validateHappyOysterFirstFrame("https://example.com/frame.webp", ""))
	assert.Error(t, validateHappyOysterFirstFrame("file:///tmp/frame.png", ""))
	assert.NoError(t, validateHappyOysterFirstFrame("", encodePNG(160, 100)))
	assert.Error(t, validateHappyOysterFirstFrame("", encodePNG(100, 100)))
	assert.Error(t, validateHappyOysterFirstFrame("", "not-base64"))
}

func TestHappyOysterModelsAutoMigrateIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:happyoyster-migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	models := []any{&model.HappyOysterWorld{}, &model.HappyOysterTicket{}, &model.HappyOysterTravel{}, &model.HappyOysterClientToken{}}
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, db.AutoMigrate(models...))
	for _, table := range []string{"happy_oyster_worlds", "happy_oyster_tickets", "happy_oyster_travels", "happy_oyster_client_tokens"} {
		assert.True(t, db.Migrator().HasTable(table), table)
	}
}

func TestHappyOysterDirectingValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"text world", `{"prompt":"海边", "resolution":"480p", "async":false}`, true},
		{"image world", `{"prompt":"海边", "resolution":"720p", "inputImages":[{"url":"https://example.com/ref.png","referenceType":"default"}]}`, true},
		{"script defaults", `{"creationModel":"scriptlist","resolution":"720p","firstFrameImage":{"url":"https://example.com/frame.png"},"scriptList":{"synopsis":"故事","subjects":[{"name":"主角"}],"acts":[{"content":"出场"}]}}`, true},
		{"missing resolution", `{"prompt":"海边"}`, false},
		{"unsupported resolution", `{"prompt":"海边","resolution":"1080p"}`, false},
		{"wrong mode", `{"prompt":"海边","resolution":"480p","mode":1}`, false},
		{"dual image", `{"prompt":"海边","resolution":"480p","firstFrameImage":{"url":"https://example.com/a","base64":"bad"}}`, false},
		{"script missing frame", `{"creationModel":"scriptlist","resolution":"480p","scriptList":{"synopsis":"故事","acts":[{"content":"出场"}]}}`, false},
		{"duplicate turn", `{"creationModel":"scriptlist","resolution":"480p","firstFrameImage":{"url":"https://example.com/a"},"scriptList":{"synopsis":"故事","acts":[{"turn":1,"content":"出场"},{"turn":1,"content":"离场"}]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req happyOysterCreateRequest
			require.NoError(t, common.Unmarshal([]byte(tc.body), &req))
			if tc.valid {
				assert.NoError(t, req.validateDirecting())
			} else {
				assert.Error(t, req.validateDirecting())
			}
		})
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"rewindToSec":0}`, true}, {`{"rewindToSec":6.5}`, true}, {`{}`, false}, {`{"rewindToSec":-1}`, false},
	} {
		var req happyOysterControlRequest
		require.NoError(t, common.Unmarshal([]byte(tc.body), &req))
		if tc.valid {
			assert.NoError(t, req.validate("/travels/rewind"))
		} else {
			assert.Error(t, req.validate("/travels/rewind"))
		}
	}
	script := happyOysterScript{Synopsis: strings.Repeat("ignored", 1000)}
	for i := range 45 {
		turn := i + 1
		script.Acts = append(script.Acts, happyOysterAct{Turn: &turn, Content: "场景"})
	}
	require.NoError(t, script.validate(true), "update-script ignores creation metadata")
	script.Acts[44].Turn = script.Acts[0].Turn
	assert.Error(t, script.validate(true))
}

func TestHappyOysterModelAndTemporaryScopeIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, tokenModel, scope, path string
		allowed                       bool
	}{
		{"directing pause", constant.HappyOysterDirecting, "travel:pause", "/travels/pause", true},
		{"adventure cannot pause", constant.HappyOysterAdventure, "travel:pause", "/travels/pause", false},
		{"scope missing", constant.HappyOysterDirecting, "travel:status", "/travels/pause", false},
		{"main only", constant.HappyOysterDirecting, "world:build-status", "/worlds/detail", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET(constant.HappyOysterAPIPath(constant.HappyOysterDirecting)+tc.path, func(c *gin.Context) {
				c.Set("resolved_task_model", constant.HappyOysterDirecting)
				c.Set(middleware.ContextKeyHappyOysterClientToken, &model.HappyOysterClientToken{Model: tc.tokenModel, Scopes: tc.scope})
			}, middleware.HappyOysterModelAuth(), func(c *gin.Context) { c.Status(204) })
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, httptest.NewRequest("GET", constant.HappyOysterAPIPath(constant.HappyOysterDirecting)+tc.path, nil))
			if tc.allowed {
				assert.Equal(t, 204, out.Code)
			} else {
				assert.Equal(t, 403, out.Code)
			}
		})
	}
	for _, allowed := range []bool{true, false} {
		out := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(out)
		c.Set("resolved_task_model", constant.HappyOysterDirecting)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
		common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{constant.HappyOysterAdventure: true, constant.HappyOysterDirecting: allowed})
		middleware.HappyOysterModelAuth()(c)
		assert.Equal(t, !allowed, c.IsAborted())
	}
}

// Optional DSNs must point at disposable databases. Only this test's own
// prefixed tables are created/dropped; no application tables are touched.
func TestHappyOysterDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open("file:happyoyster-matrix?mode=memory&cache=shared")
			case "mysql":
				dsn := os.Getenv("HAPPYOYSTER_TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("set HAPPYOYSTER_TEST_MYSQL_DSN for real MySQL verification")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("HAPPYOYSTER_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("set HAPPYOYSTER_TEST_POSTGRES_DSN for real PostgreSQL verification")
				}
				driver = postgres.Open(dsn)
			}
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade_%t", upgrade), func(t *testing.T) {
					db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "ho_matrix_"}})
					require.NoError(t, err)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(1)
					if dialect == "sqlite" {
						var version string
						require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
						t.Logf("SQLite %s", version)
					}
					t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
					tables := []any{&model.HappyOysterWorld{}, &model.HappyOysterTicket{}, &model.HappyOysterTravel{}, &model.HappyOysterClientToken{}, &model.HappyOysterInstruction{}}
					for _, table := range tables {
						require.False(t, db.Migrator().HasTable(table), "test database contains existing test tables; refusing to overwrite")
					}
					t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(tables...)) })
					if upgrade {
						// Reconstruct the pre-Directing schema, including its original
						// unique indexes, without depending on new field defaults.
						added := map[string]bool{"Model": true, "Resolution": true, "HasReferenceImage": true, "CreationModel": true, "DurationKnown": true, "ErrorCode": true, "ErrorMessage": true, "LastPolledAt": true}
						for _, table := range tables[:4] {
							typ := reflect.TypeOf(table).Elem()
							var fields []reflect.StructField
							for i := range typ.NumField() {
								f := typ.Field(i)
								if added[f.Name] && !(typ.Name() == "HappyOysterWorld" && f.Name == "CreationModel") {
									continue
								}
								f.Tag = reflect.StructTag(strings.ReplaceAll(string(f.Tag), "_model_user", "_user"))
								fields = append(fields, f)
							}
							legacy := reflect.New(reflect.StructOf(fields)).Interface()
							require.NoError(t, db.Table(db.NamingStrategy.TableName(typ.Name())).AutoMigrate(legacy))
						}
						worldTable := db.NamingStrategy.TableName("HappyOysterWorld")
						require.NoError(t, db.Table(worldTable).Create(map[string]any{"user_id": 7, "encrypted_world_id": "legacy-world", "channel_id": 9, "channel_account_hash": "account", "base_url": "https://workspace.example", "deleted": false, "creation_model": "simple"}).Error)
						require.NoError(t, db.Table(db.NamingStrategy.TableName("HappyOysterTravel")).Create(map[string]any{"user_id": 7, "encrypted_travel_id": "legacy-travel", "world_record_id": 1, "pre_consumed_quota": 12345, "billing_snapshot": "legacy-snapshot", "settlement_state": "pending"}).Error)
						require.NoError(t, db.Table(db.NamingStrategy.TableName("HappyOysterClientToken")).Create(map[string]any{"token_hash": "hash", "user_id": 7, "scopes": "travel:status"}).Error)
					}
					for range 2 {
						require.NoError(t, db.AutoMigrate(tables...))
						require.NoError(t, model.MigrateHappyOysterModels(db))
					}
					if upgrade {
						var world model.HappyOysterWorld
						require.NoError(t, db.First(&world, 1).Error)
						assert.Equal(t, constant.HappyOysterAdventure, world.Model)
						assert.Equal(t, "account", world.ChannelAccountHash)
						var travel model.HappyOysterTravel
						require.NoError(t, db.First(&travel, 1).Error)
						assert.Equal(t, constant.HappyOysterAdventure, travel.Model)
						assert.Equal(t, 12345, travel.PreConsumedQuota)
						assert.Equal(t, "legacy-snapshot", travel.BillingSnapshot)
						var token model.HappyOysterClientToken
						require.NoError(t, db.First(&token, 1).Error)
						assert.Equal(t, constant.HappyOysterAdventure, token.Model)
						assert.Equal(t, "travel:status", token.Scopes)
					}
					for _, name := range []string{constant.HappyOysterAdventure, constant.HappyOysterDirecting} {
						world := model.HappyOysterWorld{UserId: 8, Model: name, EncryptedWorldId: "same-opaque-id"}
						require.NoError(t, db.Create(&world).Error)
						duplicate := model.HappyOysterWorld{UserId: 8, Model: name, EncryptedWorldId: "same-opaque-id"}
						assert.Error(t, db.Create(&duplicate).Error, "duplicates within a model must still be rejected")
					}
					requestKey := fmt.Sprintf("%x", sha256.Sum256([]byte("retry-key")))
					instruction := model.HappyOysterInstruction{TravelId: 1, ClientRequestId: "retry-key", RequestKey: requestKey, State: "pending"}
					require.NoError(t, db.Create(&instruction).Error)
					assert.Error(t, db.Create(&model.HappyOysterInstruction{TravelId: 1, ClientRequestId: "retry-key", RequestKey: requestKey}).Error)
					caseDistinctKey := fmt.Sprintf("%x", sha256.Sum256([]byte("RETRY-KEY")))
					require.NoError(t, db.Create(&model.HappyOysterInstruction{TravelId: 1, ClientRequestId: "RETRY-KEY", RequestKey: caseDistinctKey}).Error, "protocol IDs are case sensitive on every database")
				})
			}
		})
	}
}

func TestHappyOysterWebPDimensions(t *testing.T) {
	data := make([]byte, 30)
	copy(data[:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	data[24], data[27] = 159, 99
	width, height := happyOysterWebPDimensions(data)
	assert.Equal(t, 160, width)
	assert.Equal(t, 100, height)
}

func TestHappyOysterPriceScope(t *testing.T) {
	for _, tc := range []struct {
		configured string
		baseURL    string
		want       string
		wantError  bool
	}{
		{baseURL: "https://workspace.ap-southeast-1.maas.aliyuncs.com", want: "international"},
		{baseURL: "https://workspace.cn-beijing.maas.aliyuncs.com", want: "global"},
		{baseURL: "https://workspace.us-east-1.maas.aliyuncs.com", want: "global"},
		{configured: " International ", baseURL: "https://proxy.example.com", want: "international"},
		{configured: "GLOBAL", baseURL: "https://proxy.example.com", want: "global"},
		{baseURL: "https://proxy.example.com", wantError: true},
		{configured: "unknown", wantError: true},
	} {
		t.Run(tc.configured+tc.baseURL, func(t *testing.T) {
			got, err := happyOysterPriceScope(tc.configured, tc.baseURL)
			if tc.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHappyOysterTemporaryAPIKeyEndpointAllowlist(t *testing.T) {
	for _, path := range []string{"/worlds/build-status", "/travels/enter-travel", "/travels/status", "/travels/end"} {
		assert.True(t, happyOysterTemporaryAPIKeyAllows(path), path)
	}
	for _, path := range []string{"/worlds", "/worlds/get-travel-credential", "/worlds/detail", "/worlds/delete", "/travels", "/travels/artifacts"} {
		assert.False(t, happyOysterTemporaryAPIKeyAllows(path), path)
	}
}

func TestHappyOysterCreateWorldValidationUsesOfficialEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, happyoyster.APIPath+"/worlds", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	HappyOysterCreateWorld(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, 400000, response.Code)
}

func TestHappyOysterOfficialResponseEnvelope(t *testing.T) {
	payload, response, requestId, err := decodeHappyOysterResponse([]byte(`{"code":0,"message":null,"data":{"encryptedWorldId":"enc_test","status":"generating","firstFrame":null}}`))
	require.NoError(t, err)
	assert.Empty(t, requestId)
	assert.JSONEq(t, `{"code":0,"message":null,"data":{"encryptedWorldId":"enc_test","status":"generating","firstFrame":null}}`, string(payload))
	assert.True(t, response.successful())

	var world happyOysterWorldData
	require.NoError(t, common.Unmarshal(response.Data, &world))
	assert.Equal(t, "enc_test", world.EncryptedWorldId)
	assert.Equal(t, "generating", world.Status)
}

func TestHappyOysterDashScopeResponseEnvelope(t *testing.T) {
	payload, response, requestId, err := decodeHappyOysterResponse([]byte(`{"output":{"encryptedWorldId":"enc_wrapped","status":"generating","firstFrame":null},"request_id":"req_test"}`))
	require.NoError(t, err)
	assert.Equal(t, "req_test", requestId)
	assert.True(t, response.successful())
	assert.JSONEq(t, `{"code":0,"message":null,"data":{"encryptedWorldId":"enc_wrapped","status":"generating","firstFrame":null}}`, string(payload))

	var world happyOysterWorldData
	require.NoError(t, common.Unmarshal(response.Data, &world))
	assert.Equal(t, "enc_wrapped", world.EncryptedWorldId)
}

func TestHappyOysterDashScopeNestedOfficialEnvelope(t *testing.T) {
	payload, response, requestId, err := decodeHappyOysterResponse([]byte(`{"output":{"code":0,"message":null,"data":{"encryptedWorldId":"enc_nested","status":"ready"}},"request_id":"req_nested"}`))
	require.NoError(t, err)
	assert.Equal(t, "req_nested", requestId)
	assert.True(t, response.successful())
	assert.JSONEq(t, `{"code":0,"message":null,"data":{"encryptedWorldId":"enc_nested","status":"ready"}}`, string(payload))
}

func TestHappyOysterTravelResponsePreservesOptionalDuration(t *testing.T) {
	_, response, _, err := decodeHappyOysterResponse([]byte(`{"output":{"encryptedTravelId":"trvl_test","encryptedWorldId":"enc_test","status":"completed","durationSec":null,"maxExperienceTimeSec":90},"request_id":"req_travel"}`))
	require.NoError(t, err)

	var travel happyOysterTravelData
	require.NoError(t, common.Unmarshal(response.Data, &travel))
	assert.Nil(t, travel.DurationSec)
	require.NotNil(t, travel.MaxExperienceTimeSec)
	assert.Equal(t, 90, *travel.MaxExperienceTimeSec)
}

func TestHappyOysterTravelRoutingAcceptsOpaqueEncryptedID(t *testing.T) {
	worldId := "zAFXAwF7DMjt_C7Axt4L66bH-plkNAhYQwYYAN0YyOMaq9tS39yZ8Z0EybgApjCt"
	travel := happyOysterTravelData{
		EncryptedTravelId: "fM8u4vA9_wQZc6pL1xY2nR7sK3tB5dE0",
		EncryptedWorldId:  worldId,
	}

	assert.True(t, travel.validForWorld(worldId))
	travel.EncryptedWorldId = "another-world"
	assert.False(t, travel.validForWorld(worldId))
	travel.EncryptedWorldId = worldId
	travel.EncryptedTravelId = ""
	assert.False(t, travel.validForWorld(worldId))
}

func TestHappyOysterLogRetainsCompleteWorldPayloads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:happyoyster-log-payloads?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Channel{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
	})

	requestBody := `{"async":true,"prompt":"完整提示词","firstFrameImage":{"base64":"iVBORw0KGgoAAAANSUhEUg=="}}`
	responseBody := `{"code":0,"message":null,"data":{"encryptedWorldId":"opaque-world-id","status":"generating","firstFrame":null}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, happyoyster.APIPath+"/worlds", strings.NewReader(requestBody))

	happyOysterLog(c, &relaycommon.RelayInfo{UserId: 7, TokenId: 11, UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 9}}, 123, "HappyOyster world creation", "opaque-world-id", "", happyOysterLogPayload{Request: requestBody, Response: responseBody})

	var log model.Log
	require.NoError(t, db.Where("model_name = ?", happyOysterModel).First(&log).Error)
	assert.Equal(t, requestBody, log.Request)
	assert.Equal(t, responseBody, log.Response)
}

func TestHappyOysterListValidationUsesOfficialBusinessEnvelope(t *testing.T) {
	for _, path := range []string{
		happyoyster.APIPath + "/worlds?mode=2",
		happyoyster.APIPath + "/travels?pageSize=invalid",
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)

		happyOysterList(c, strings.Contains(path, "/worlds"))

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Code int `json:"code"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, 400000, response.Code)
	}
}

func TestHappyOysterEnterTravelValidationUsesOfficialBusinessEnvelope(t *testing.T) {
	for _, body := range []string{
		`{"ticket":"tk_test","maxExperienceTimeSec":30}`,
		`{"ticket":"tk_test","maxExperienceTimeSec":"60"}`,
		`{"maxExperienceTimeSec":60}`,
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, happyoyster.APIPath+"/travels/enter-travel", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		HappyOysterEnterTravel(c)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Code int `json:"code"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, 400000, response.Code)
	}
}

func TestHappyOysterDeleteWorldIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:happyoyster-delete-idempotent?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.HappyOysterWorld{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	world := model.HappyOysterWorld{UserId: 7, EncryptedWorldId: "enc_deleted", Deleted: true}
	require.NoError(t, db.Create(&world).Error)

	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST(happyoyster.APIPath+"/worlds/delete", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 7)
		HappyOysterWorldOperation(c)
	})
	request := httptest.NewRequest(http.MethodPost, happyoyster.APIPath+"/worlds/delete", strings.NewReader(`{"encryptedWorldId":"enc_deleted"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"code":0,"message":null,"data":{"encryptedWorldId":"enc_deleted","deleted":false}}`, recorder.Body.String())
}

func TestHappyOysterEnvelopeDoesNotTreatMissingCodeAsSuccess(t *testing.T) {
	var response happyOysterEnvelope
	require.NoError(t, common.Unmarshal([]byte(`{"data":{"encryptedWorldId":"enc_test"}}`), &response))
	assert.False(t, response.successful())
}

func TestHappyOysterDirectingLifecycleAndInstructionBilling(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ho-directing-lifecycle?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.HappyOysterWorld{}, &model.HappyOysterTravel{}, &model.HappyOysterTicket{}, &model.HappyOysterClientToken{}, &model.HappyOysterInstruction{}, &model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.SettlementConfig{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	user := model.User{Username: "ho-directing", Quota: 10_000_000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "ho-client-test", RemainQuota: 10_000_000, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	var instructions atomic.Int32
	instructionEntered := make(chan struct{})
	allowInstruction := make(chan struct{})
	var releaseInstruction sync.Once
	t.Cleanup(func() { releaseInstruction.Do(func() { close(allowInstruction) }) })
	var statusComplete atomic.Bool
	prefix := constant.HappyOysterAPIPath(constant.HappyOysterDirecting)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer upstream-test", r.Header.Get("Authorization"))
		assert.True(t, strings.HasPrefix(r.URL.Path, prefix), r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch strings.TrimPrefix(r.URL.Path, prefix) {
		case "/worlds":
			var req map[string]any
			assert.NoError(t, common.DecodeJson(r.Body, &req))
			assert.Equal(t, false, req["async"])
			data = gin.H{"encryptedWorldId": "world-directing", "status": "ready", "name": "test"}
		case "/worlds/get-travel-credential":
			data = gin.H{"ticket": "tk_test-only", "expiresIn": 1800, "encryptedWorldId": "world-directing"}
		case "/travels/enter-travel":
			data = gin.H{"encryptedTravelId": "travel-directing", "encryptedWorldId": "world-directing", "maxExperienceTimeSec": nil, "rtcConfig": gin.H{"token": "rtc-secret-never-log"}}
		case "/travels/instruct":
			if instructions.Add(1) == 1 {
				close(instructionEntered)
				<-allowInstruction
			}
			var req happyOysterControlRequest
			assert.NoError(t, common.DecodeJson(r.Body, &req))
			assert.True(t, happyOysterClientRequestID.MatchString(req.ClientRequestId))
			if req.Content == "unknown" {
				w.WriteHeader(502)
				return
			}
			if req.Content == "rejected" {
				payload, _ := common.Marshal(gin.H{"code": 403004, "message": "rejected", "data": nil})
				_, _ = w.Write(payload)
				return
			}
			data = gin.H{"encryptedTravelId": "travel-directing", "accepted": true}
		case "/travels/pause":
			data = gin.H{"encryptedTravelId": "travel-directing", "status": "paused"}
		case "/travels/rewind":
			var req map[string]any
			assert.NoError(t, common.DecodeJson(r.Body, &req))
			assert.Equal(t, float64(0), req["rewindToSec"])
			data = gin.H{"encryptedTravelId": "travel-directing", "status": "running", "actualRewindToSec": 0}
		case "/travels/status":
			status := "paused"
			if statusComplete.Load() {
				status = "completed"
			}
			data = gin.H{"encryptedTravelId": "travel-directing", "status": status, "durationSec": nil}
		case "/travels/artifacts":
			data = gin.H{"encryptedTravelId": "travel-directing", "composeStatus": "partial", "video": gin.H{"original": gin.H{"durationSec": 10, "url": "https://example.com/original.mp4"}, "withWatermark": gin.H{"durationSec": 10}}}
		default:
			t.Errorf("unexpected upstream route %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		payload, _ := common.Marshal(gin.H{"code": 0, "message": nil, "data": data})
		_, _ = w.Write(payload)
	}))
	t.Cleanup(upstream.Close)
	baseURL := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeHappyOyster, Status: common.ChannelStatusEnabled, Name: "HappyOyster", Key: "upstream-test", BaseURL: &baseURL, Other: "international", Models: constant.HappyOysterDirecting}
	require.NoError(t, db.Create(&channel).Error)
	engine := gin.New()
	group := engine.Group(prefix, func(c *gin.Context) {
		c.Set("resolved_task_model", constant.HappyOysterDirecting)
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		require.NoError(t, middlewareSetupHappyOysterContext(c, &channel, channel.Key, 0, baseURL))
	})
	group.POST("/worlds", HappyOysterCreateWorld)
	group.POST("/worlds/get-travel-credential", HappyOysterWorldOperation)
	group.POST("/travels/enter-travel", HappyOysterEnterTravel)
	for _, path := range []string{"instruct", "pause", "rewind"} {
		group.POST("/travels/"+path, HappyOysterTravelOperation)
	}
	group.GET("/travels/artifacts", HappyOysterTravelOperation)
	group.GET("/worlds", HappyOysterListWorlds)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		req := httptest.NewRequest(method, prefix+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(out, req)
		return out
	}
	created := call("POST", "/worlds", `{"prompt":"海边","resolution":"480p","async":false}`)
	require.Equal(t, 200, created.Code, created.Body.String())
	world, err := model.GetHappyOysterWorld(user.Id, "world-directing", constant.HappyOysterDirecting)
	require.NoError(t, err)
	assert.Equal(t, "480p", world.Resolution)
	_, err = model.GetHappyOysterWorld(user.Id, "world-directing", constant.HappyOysterAdventure)
	assert.Error(t, err)
	_, err = model.GetHappyOysterWorld(user.Id+1, "world-directing", constant.HappyOysterDirecting)
	assert.Error(t, err)
	require.Equal(t, 200, call("POST", "/worlds/get-travel-credential", `{"encryptedWorldId":"world-directing"}`).Code)
	entered := call("POST", "/travels/enter-travel", `{"ticket":"tk_test-only"}`)
	require.Equal(t, 200, entered.Code, entered.Body.String())
	travel, err := model.GetHappyOysterTravel(user.Id, "travel-directing", constant.HappyOysterDirecting)
	require.NoError(t, err)
	assert.Equal(t, 180, travel.MaxExperienceSeconds)
	assert.Equal(t, 4_851_000, travel.PreConsumedQuota)
	assert.Equal(t, 400, call("POST", "/travels/enter-travel", `{"ticket":"tk_test-only"}`).Code)
	instruction := `{"encryptedTravelId":"travel-directing","content":"turn left","clientRequestId":"once"}`
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- call("POST", "/travels/instruct", instruction) }()
	select {
	case <-instructionEntered:
	case early := <-firstDone:
		t.Fatalf("instruction did not reach upstream: %d %s", early.Code, early.Body.String())
	}
	concurrent := call("POST", "/travels/instruct", instruction)
	assert.Equal(t, 409, concurrent.Code, "an in-flight key cannot execute twice")
	releaseInstruction.Do(func() { close(allowInstruction) })
	first := <-firstDone
	require.Equal(t, 200, first.Code, first.Body.String())
	second := call("POST", "/travels/instruct", instruction)
	assert.Equal(t, first.Body.String(), second.Body.String())
	assert.Equal(t, int32(1), instructions.Load())
	assert.Equal(t, 409, call("POST", "/travels/instruct", strings.Replace(instruction, "turn left", "different", 1)).Code)
	require.Equal(t, 200, call("POST", "/travels/pause", `{"encryptedTravelId":"travel-directing"}`).Code)
	require.NoError(t, db.First(travel, travel.Id).Error)
	assert.Equal(t, "pending", travel.Status, "pause response is not confirmation")
	require.Equal(t, 200, call("POST", "/travels/rewind", `{"encryptedTravelId":"travel-directing","rewindToSec":0}`).Code)
	statusComplete.Store(true)
	assert.Equal(t, 1, RunHappyOysterPollingOnce(context.Background()))
	require.NoError(t, db.First(travel, travel.Id).Error)
	assert.Equal(t, model.HappyOysterSettlementDone, travel.SettlementState)
	assert.Equal(t, 269_500, travel.ActualQuota)
	require.Equal(t, 200, call("GET", "/travels/artifacts?encryptedTravelId=travel-directing", "").Code)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 9_567_150, user.Quota)
	assert.Equal(t, 432_850, user.UsedQuota)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, user.Quota, token.RemainQuota)
	rejected := `{"encryptedTravelId":"travel-directing","content":"rejected","clientRequestId":"rejected-once"}`
	require.Equal(t, 200, call("POST", "/travels/instruct", rejected).Code)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 9_567_150, user.Quota, "explicit rejection refunds")
	unknown := `{"encryptedTravelId":"travel-directing","content":"unknown","clientRequestId":"unknown-once"}`
	assert.Equal(t, 502, call("POST", "/travels/instruct", unknown).Code)
	assert.Equal(t, 409, call("POST", "/travels/instruct", unknown).Code)
	assert.Equal(t, int32(3), instructions.Load())
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 9_559_450, user.Quota, "unknown outcome retains one reservation")
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	for _, log := range logs {
		assert.Equal(t, constant.HappyOysterDirecting, log.ModelName)
		assert.NotContains(t, log.Request+log.Response, "tk_test-only")
		assert.NotContains(t, log.Request+log.Response, "rtc-secret-never-log")
	}
}
