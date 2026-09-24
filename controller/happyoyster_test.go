package controller

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/happyoyster"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
