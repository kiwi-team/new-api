package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/happyoyster"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const happyOysterModel = "happyoyster-1.0-adventure"

type happyOysterEnvelope struct {
	Code    *int            `json:"code"`
	Message any             `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e happyOysterEnvelope) successful() bool {
	return e.Code != nil && *e.Code == 0
}

func decodeHappyOysterResponse(payload []byte) ([]byte, happyOysterEnvelope, string, error) {
	var metadata struct {
		Output    json.RawMessage `json:"output"`
		RequestId string          `json:"request_id"`
	}
	if err := common.Unmarshal(payload, &metadata); err != nil {
		return payload, happyOysterEnvelope{}, "", err
	}

	var envelope happyOysterEnvelope
	if err := common.Unmarshal(payload, &envelope); err == nil && envelope.Code != nil {
		return payload, envelope, metadata.RequestId, nil
	}
	if common.GetJsonType(metadata.Output) == "object" {
		var nested happyOysterEnvelope
		if err := common.Unmarshal(metadata.Output, &nested); err != nil {
			return payload, happyOysterEnvelope{}, metadata.RequestId, fmt.Errorf("invalid HappyOyster output: %w", err)
		}
		if nested.Code != nil {
			return metadata.Output, nested, metadata.RequestId, nil
		}

		zero := 0
		envelope = happyOysterEnvelope{Code: &zero, Data: metadata.Output}
		normalized, err := common.Marshal(envelope)
		if err != nil {
			return payload, happyOysterEnvelope{}, metadata.RequestId, err
		}
		return normalized, envelope, metadata.RequestId, nil
	}

	var fields map[string]json.RawMessage
	if common.Unmarshal(payload, &fields) == nil {
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return payload, happyOysterEnvelope{}, metadata.RequestId, fmt.Errorf("upstream returned a malformed HappyOyster response: missing code (fields: %s)", strings.Join(keys, ","))
	}
	return payload, happyOysterEnvelope{}, metadata.RequestId, fmt.Errorf("upstream returned a malformed HappyOyster response: missing code")
}

type happyOysterWorldData struct {
	EncryptedWorldId string  `json:"encryptedWorldId"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	FirstFrame       *string `json:"firstFrame"`
}

type happyOysterTicketData struct {
	Ticket           string `json:"ticket"`
	ExpiresIn        int    `json:"expiresIn"`
	EncryptedWorldId string `json:"encryptedWorldId"`
}

type happyOysterTravelData struct {
	EncryptedTravelId      string `json:"encryptedTravelId"`
	EncryptedWorldId       string `json:"encryptedWorldId"`
	Status                 string `json:"status"`
	DurationSec            *int   `json:"durationSec"`
	EndedAt                string `json:"endedAt"`
	ErrorCode              string `json:"errorCode"`
	MaxExperienceTimeSec   *int   `json:"maxExperienceTimeSec"`
	NoStreamAutoEndTimeout *int   `json:"noStreamAutoEndTimeoutSec"`
}

type happyOysterArtifactsData struct {
	EncryptedTravelId string `json:"encryptedTravelId"`
	Video             struct {
		Original struct {
			DurationSec *int `json:"durationSec"`
		} `json:"original"`
	} `json:"video"`
}

func happyOysterError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"code": status, "message": message, "data": nil})
}

func happyOysterBusinessError(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "message": message, "data": nil})
}

func happyOysterClientToken(c *gin.Context) (*model.HappyOysterClientToken, bool) {
	value, exists := c.Get(middleware.ContextKeyHappyOysterClientToken)
	if !exists {
		return nil, false
	}
	token, ok := value.(*model.HappyOysterClientToken)
	return token, ok
}

func happyOysterClientTokenAllowsWorld(c *gin.Context, world *model.HappyOysterWorld) bool {
	token, scoped := happyOysterClientToken(c)
	if !scoped {
		return true
	}
	if token.WorldId != 0 {
		return token.WorldId == world.Id
	}
	return token.ChannelId == world.ChannelId &&
		token.ChannelMultiKeyIndex == world.ChannelMultiKeyIndex &&
		token.ChannelAccountHash == world.ChannelAccountHash &&
		token.BaseURL == world.BaseURL
}

func happyOysterClientTokenAllowsTravel(c *gin.Context, travel *model.HappyOysterTravel) bool {
	token, scoped := happyOysterClientToken(c)
	if !scoped {
		return true
	}
	if token.WorldId != 0 {
		return token.WorldId == travel.WorldRecordId
	}
	return token.ChannelId == travel.ChannelId &&
		token.ChannelMultiKeyIndex == travel.ChannelMultiKeyIndex &&
		token.ChannelAccountHash == travel.ChannelAccountHash &&
		token.BaseURL == travel.BaseURL
}

func happyOysterTemporaryAPIKeyAllows(path string) bool {
	return path == "/worlds/build-status" || path == "/travels/enter-travel" || path == "/travels/status" || path == "/travels/end"
}

func happyOysterBody(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func validateHappyOysterFirstFrame(rawURL, encoded string) error {
	if rawURL != "" {
		if len(rawURL) > 4096 {
			return fmt.Errorf("firstFrameImage.url is too long")
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("firstFrameImage.url must be an HTTP or HTTPS URL")
		}
		return nil
	}
	if prefix, value, ok := strings.Cut(encoded, ","); ok && strings.HasPrefix(prefix, "data:") {
		encoded = value
	}
	var data []byte
	var err error
	for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		data, err = decoder.DecodeString(encoded)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("firstFrameImage.base64 is invalid")
	}
	if len(data) == 0 || len(data) >= 6<<20 {
		return fmt.Errorf("firstFrameImage must be smaller than 6 MB")
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		return fmt.Errorf("firstFrameImage must be JPEG, PNG, or WebP")
	}
	width, height := 0, 0
	if mime == "image/webp" {
		width, height = happyOysterWebPDimensions(data)
	} else {
		config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr == nil {
			width, height = config.Width, config.Height
		}
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("firstFrameImage has invalid image data")
	}
	ratio := float64(width) / float64(height)
	if ratio < 1.5 || ratio > 2.0 {
		return fmt.Errorf("firstFrameImage aspect ratio must be between 1.5 and 2.0")
	}
	return nil
}

func happyOysterWebPDimensions(data []byte) (int, int) {
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0
	}
	switch string(data[12:16]) {
	case "VP8X":
		return 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16, 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
	case "VP8L":
		if data[20] != 0x2f {
			return 0, 0
		}
		bits := uint32(data[21]) | uint32(data[22])<<8 | uint32(data[23])<<16 | uint32(data[24])<<24
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1
	case "VP8 ":
		if data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return 0, 0
		}
		return int(uint16(data[26])|uint16(data[27])<<8) & 0x3fff, int(uint16(data[28])|uint16(data[29])<<8) & 0x3fff
	default:
		return 0, 0
	}
}

func happyOysterAccountHash(baseURL, key string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(baseURL, "/") + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

func happyOysterRelayInfo(c *gin.Context) (*relaycommon.RelayInfo, error) {
	common.SetContextKey(c, constant.ContextKeyOriginalModel, happyOysterModel)
	c.Set("original_model", happyOysterModel)
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return nil, err
	}
	info.InitChannelMeta(c)
	info.OriginModelName = happyOysterModel
	info.BillingModelName = happyOysterModel
	info.UpstreamModelName = happyOysterModel
	info.ForcePreConsume = true
	return info, nil
}

func happyOysterPriceScope(configured, baseURL string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(configured))
	if scope == "" {
		host := strings.ToLower(baseURL)
		switch {
		case strings.Contains(host, ".ap-southeast-1.maas.aliyuncs.com"):
			return "international", nil
		case strings.Contains(host, ".cn-beijing.maas.aliyuncs.com"), strings.Contains(host, ".us-east-1.maas.aliyuncs.com"):
			return "global", nil
		default:
			return "", fmt.Errorf("channel Other must specify international or global when the API host region cannot be inferred")
		}
	}
	if scope != "international" && scope != "global" {
		return "", fmt.Errorf("channel Other must be either international or global")
	}
	return scope, nil
}

func happyOysterPrice(c *gin.Context, info *relaycommon.RelayInfo, configuredScope, baseURL string, worlds, seconds int) (int, error) {
	scope, err := happyOysterPriceScope(configuredScope, baseURL)
	if err != nil {
		return 0, err
	}
	expr, ok := billing_setting.GetBillingExpr(happyOysterModel)
	if !ok {
		return 0, fmt.Errorf("HappyOyster billing expression is not configured")
	}
	facts := map[string]any{"price_scope": scope, "world_creations": worlds, "experience_seconds": seconds}
	cost, trace, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
	if err != nil || cost < 0 {
		if err == nil {
			err = fmt.Errorf("billing expression returned negative cost")
		}
		return 0, err
	}
	ratio := helper.HandleGroupRatio(c, info)
	quota, clamp := common.QuotaRoundChecked(cost * common.QuotaPerUnit * ratio.GroupRatio)
	info.QuotaClamp = clamp
	info.PriceData = hosttypes.PriceData{Quota: quota, QuotaToPreConsume: quota, GroupRatioInfo: ratio}
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
		BillingMode: billing_setting.BillingModeTieredExpr, ModelName: happyOysterModel,
		ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: ratio.GroupRatio,
		EstimatedQuotaBeforeGroup: cost * common.QuotaPerUnit, EstimatedQuotaAfterGroup: quota,
		EstimatedTier: trace.MatchedTier, QuotaPerUnit: common.QuotaPerUnit,
		ExprVersion: billingexpr.ExprVersion(expr), TaskUsageBilling: true, UsageFacts: facts,
	}
	return quota, nil
}

func happyOysterPinned(c *gin.Context, channelId, keyIndex int, accountHash, baseURL string) (*model.Channel, string, error) {
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		return nil, "", err
	}
	if channel.Type != constant.ChannelTypeHappyOyster || channel.Status != common.ChannelStatusEnabled {
		return nil, "", fmt.Errorf("bound HappyOyster channel is unavailable")
	}
	if specificChannel := common.GetContextKeyInt(c, constant.ContextKeyTokenSpecificChannelId); specificChannel > 0 && specificChannel != channel.Id {
		return nil, "", fmt.Errorf("API token is not allowed to use the bound channel")
	}
	keys := channel.GetKeys()
	if keyIndex < 0 || keyIndex >= len(keys) {
		return nil, "", fmt.Errorf("bound channel key no longer exists")
	}
	if channel.ChannelInfo.IsMultiKey {
		if keyStatus, exists := channel.ChannelInfo.MultiKeyStatusList[keyIndex]; exists && keyStatus != common.ChannelStatusEnabled {
			return nil, "", fmt.Errorf("bound channel key is disabled")
		}
	}
	key := keys[keyIndex]
	resolvedBase := channel.GetBaseURL()
	if baseURL != "" {
		resolvedBase = baseURL
	}
	if happyOysterAccountHash(resolvedBase, key) != accountHash {
		return nil, "", fmt.Errorf("bound channel account changed")
	}
	if apiErr := middlewareSetupHappyOysterContext(c, channel, key, keyIndex, resolvedBase); apiErr != nil {
		return nil, "", apiErr
	}
	return channel, key, nil
}

// Kept local so pinned resources never rotate to another key in a multi-key channel.
func middlewareSetupHappyOysterContext(c *gin.Context, channel *model.Channel, key string, keyIndex int, baseURL string) error {
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, baseURL)
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, keyIndex)
	return nil
}

func happyOysterProxy(c *gin.Context, channel *model.Channel, key, method, path string, query url.Values, body []byte) (int, []byte, happyOysterEnvelope, error) {
	if value, exists := c.Get(middleware.ContextKeyHappyOysterUpstreamToken); exists && happyOysterTemporaryAPIKeyAllows(path) {
		if temporaryKey, ok := value.(string); ok && temporaryKey != "" {
			key = temporaryKey
		}
	}
	status, payload, err := happyoyster.Do(c.Request, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl), key, channel.GetSetting().Proxy, channel.GetSetting(), method, path, query, body)
	var envelope happyOysterEnvelope
	if err == nil && status >= 200 && status < 300 {
		var requestId string
		payload, envelope, requestId, err = decodeHappyOysterResponse(payload)
		requestId = strings.TrimSpace(requestId)
		if requestId != "" {
			if len(requestId) <= 128 {
				c.Set(common.UpstreamRequestIdKey, requestId)
			} else {
				logger.LogWarn(c, "ignored oversized HappyOyster request_id: length=%d", len(requestId))
			}
		}
	}
	return status, payload, envelope, err
}

func HappyOysterCreateWorld(c *gin.Context) {
	if _, scoped := happyOysterClientToken(c); scoped {
		happyOysterBusinessError(c, 403003, "temporary API keys cannot call this endpoint")
		return
	}
	body, err := happyOysterBody(c)
	if err != nil {
		happyOysterBusinessError(c, 400000, err.Error())
		return
	}
	var req struct {
		Async           *bool  `json:"async,omitempty"`
		Perspective     string `json:"perspective"`
		Prompt          string `json:"prompt"`
		CreationModel   string `json:"creationModel,omitempty"`
		UploadMode      string `json:"uploadMode,omitempty"`
		RefWorldId      string `json:"refWorldId,omitempty"`
		FirstFrameImage struct {
			URL    string `json:"url,omitempty"`
			Base64 string `json:"base64,omitempty"`
		} `json:"firstFrameImage"`
	}
	if err := common.Unmarshal(body, &req); err != nil {
		happyOysterBusinessError(c, 400000, "invalid JSON body")
		return
	}
	if req.Perspective != "first_person" && req.Perspective != "third_person" {
		happyOysterBusinessError(c, 400000, "perspective must be first_person or third_person")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" || len([]rune(req.Prompt)) > 2000 {
		happyOysterBusinessError(c, 400000, "prompt must contain 1 to 2000 characters")
		return
	}
	if (req.FirstFrameImage.URL == "") == (req.FirstFrameImage.Base64 == "") {
		happyOysterBusinessError(c, 400000, "firstFrameImage must contain exactly one of url or base64")
		return
	}
	if req.CreationModel != "" && req.CreationModel != "simple" {
		happyOysterBusinessError(c, 400000, "creationModel must be simple")
		return
	}
	if len(req.RefWorldId) > 512 {
		happyOysterBusinessError(c, 400000, "refWorldId is too long")
		return
	}
	if req.UploadMode != "" && req.UploadMode != "first_frame" {
		happyOysterBusinessError(c, 400000, "uploadMode must be first_frame")
		return
	}
	if err := validateHappyOysterFirstFrame(req.FirstFrameImage.URL, req.FirstFrameImage.Base64); err != nil {
		happyOysterBusinessError(c, 400000, err.Error())
		return
	}
	var channel *model.Channel
	var key string
	if req.RefWorldId != "" {
		refWorld, lookupErr := model.GetHappyOysterWorld(common.GetContextKeyInt(c, constant.ContextKeyUserId), req.RefWorldId)
		if lookupErr != nil {
			happyOysterBusinessError(c, 403001, "refWorldId is not available to this account")
			return
		}
		channel, key, err = happyOysterPinned(c, refWorld.ChannelId, refWorld.ChannelMultiKeyIndex, refWorld.ChannelAccountHash, refWorld.BaseURL)
		if err != nil {
			happyOysterError(c, http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeHappyOyster {
		happyOysterError(c, 400, "selected channel is not HappyOyster Adventure")
		return
	}
	info, err := happyOysterRelayInfo(c)
	if err != nil {
		happyOysterError(c, 500, err.Error())
		return
	}
	if channel == nil {
		channel, err = model.GetChannelById(info.ChannelId, true)
		if err != nil {
			happyOysterError(c, 502, err.Error())
			return
		}
		key = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	}
	baseURL := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
	quota, err := happyOysterPrice(c, info, channel.Other, baseURL, 1, 0)
	if err != nil {
		happyOysterError(c, 400, err.Error())
		return
	}
	if apiErr := service.PreConsumeBilling(c, quota, info); apiErr != nil {
		happyOysterError(c, apiErr.StatusCode, apiErr.Error())
		return
	}
	status, payload, envelope, err := happyOysterProxy(c, channel, key, http.MethodPost, "/worlds", nil, body)
	if err != nil || status < 200 || status >= 300 || !envelope.successful() {
		if info.Billing != nil {
			info.Billing.Refund(c)
		}
		if err != nil {
			happyOysterError(c, 502, err.Error())
			return
		}
		c.Data(status, "application/json", payload)
		return
	}
	var data happyOysterWorldData
	if err := common.Unmarshal(envelope.Data, &data); err != nil || data.EncryptedWorldId == "" {
		_ = service.SettleBilling(c, info, quota)
		dataType := common.GetJsonType(envelope.Data)
		logger.LogWarn(c, "HappyOyster create-world success response is missing data.encryptedWorldId: data_type=%s data_bytes=%d", dataType, len(envelope.Data))
		_, _ = happyOysterLog(c, info, quota, "HappyOyster world creation returned invalid routing data", "", "")
		happyOysterError(c, 502, "upstream success response is missing data.encryptedWorldId")
		return
	}
	firstFrame := ""
	if data.FirstFrame != nil {
		firstFrame = *data.FirstFrame
	}
	world := model.HappyOysterWorld{UserId: info.UserId, TokenId: info.TokenId, EncryptedWorldId: data.EncryptedWorldId, ChannelId: channel.Id,
		ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex), ChannelAccountHash: happyOysterAccountHash(baseURL, key),
		BaseURL: baseURL, Status: data.Status, Name: data.Name, Perspective: req.Perspective, CreationModel: "simple", UploadMode: "first_frame", FirstFrame: firstFrame}
	if err := model.DB.Create(&world).Error; err != nil {
		_ = service.SettleBilling(c, info, quota)
		_, _ = happyOysterLog(c, info, quota, "HappyOyster world creation routing persistence failure", data.EncryptedWorldId, "")
		happyOysterError(c, 500, "failed to persist world routing")
		return
	}
	_ = service.SettleBilling(c, info, quota)
	_, _ = happyOysterLog(c, info, quota, "HappyOyster world creation", data.EncryptedWorldId, "")
	c.Data(status, "application/json", payload)
}

func HappyOysterWorldOperation(c *gin.Context) {
	path := c.FullPath()
	suffix := strings.TrimPrefix(path, happyoyster.APIPath)
	if _, scoped := happyOysterClientToken(c); scoped && suffix != "/worlds/build-status" {
		happyOysterBusinessError(c, 403003, "temporary API keys cannot call this endpoint")
		return
	}
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	worldId := c.Query("encryptedWorldId")
	body := []byte(nil)
	if c.Request.Method == http.MethodPost {
		var err error
		body, err = happyOysterBody(c)
		if err != nil {
			happyOysterError(c, 400, err.Error())
			return
		}
		var v struct {
			EncryptedWorldId string `json:"encryptedWorldId"`
		}
		if common.Unmarshal(body, &v) == nil && v.EncryptedWorldId != "" {
			worldId = v.EncryptedWorldId
		}
	}
	if worldId == "" || len(worldId) > 512 {
		happyOysterBusinessError(c, 400000, "encryptedWorldId is required and must not exceed 512 characters")
		return
	}
	world, err := model.GetHappyOysterWorld(userId, worldId)
	if err != nil {
		if suffix == "/worlds/delete" {
			if _, deletedErr := model.GetDeletedHappyOysterWorld(userId, worldId); deletedErr == nil {
				c.JSON(http.StatusOK, gin.H{"code": 0, "message": nil, "data": gin.H{"encryptedWorldId": worldId, "deleted": false}})
				return
			}
		}
		happyOysterError(c, 404, "world not found")
		return
	}
	if !happyOysterClientTokenAllowsWorld(c, world) {
		happyOysterError(c, http.StatusForbidden, "temporary API key and World belong to different channel accounts")
		return
	}
	channel, key, err := happyOysterPinned(c, world.ChannelId, world.ChannelMultiKeyIndex, world.ChannelAccountHash, world.BaseURL)
	if err != nil {
		happyOysterError(c, 503, err.Error())
		return
	}
	status, payload, envelope, err := happyOysterProxy(c, channel, key, c.Request.Method, suffix, c.Request.URL.Query(), body)
	if err != nil {
		happyOysterError(c, 502, err.Error())
		return
	}
	if status >= 200 && status < 300 && envelope.successful() {
		if suffix == "/worlds/build-status" || suffix == "/worlds/detail" {
			var data happyOysterWorldData
			if common.Unmarshal(envelope.Data, &data) == nil && data.Status != "" {
				updates := map[string]any{"status": data.Status}
				if data.FirstFrame != nil {
					updates["first_frame"] = *data.FirstFrame
				}
				if data.Name != "" {
					updates["name"] = data.Name
				}
				model.DB.Model(world).Updates(updates)
			}
		}
		if suffix == "/worlds/delete" {
			model.DB.Model(world).Update("deleted", true)
			model.DB.Model(&model.HappyOysterClientToken{}).Where("world_id = ? AND revoked_at = 0", world.Id).Update("revoked_at", time.Now().Unix())
		}
		if suffix == "/worlds/get-travel-credential" {
			var data happyOysterTicketData
			decodeErr := common.Unmarshal(envelope.Data, &data)
			credentialMismatch := data.EncryptedWorldId != "" && data.EncryptedWorldId != world.EncryptedWorldId
			if decodeErr != nil || !strings.HasPrefix(data.Ticket, "tk_") || data.ExpiresIn <= 0 || data.ExpiresIn > 1800 || credentialMismatch {
				happyOysterError(c, 502, "upstream returned an invalid travel credential")
				return
			}
			hash := sha256.Sum256([]byte(data.Ticket))
			if err := model.DB.Create(&model.HappyOysterTicket{TicketHash: hex.EncodeToString(hash[:]), WorldId: world.Id, UserId: userId, TokenId: common.GetContextKeyInt(c, constant.ContextKeyTokenId), State: model.HappyOysterTicketIssued, ExpiresAt: time.Now().Unix() + int64(data.ExpiresIn)}).Error; err != nil {
				happyOysterError(c, 500, "failed to persist travel credential")
				return
			}
		}
	}
	c.Data(status, "application/json", payload)
}

func HappyOysterEnterTravel(c *gin.Context) {
	body, err := happyOysterBody(c)
	if err != nil {
		happyOysterError(c, 400, err.Error())
		return
	}
	var req struct {
		Ticket               string `json:"ticket"`
		MaxExperienceTimeSec *int   `json:"maxExperienceTimeSec,omitempty"`
	}
	if common.Unmarshal(body, &req) != nil {
		happyOysterBusinessError(c, 400000, "invalid JSON body")
		return
	}
	if req.Ticket == "" {
		happyOysterBusinessError(c, 400000, "ticket is required")
		return
	}
	if len(req.Ticket) > 8192 {
		happyOysterBusinessError(c, 400000, "ticket is too long")
		return
	}
	duration := 60
	if req.MaxExperienceTimeSec != nil {
		duration = *req.MaxExperienceTimeSec
	}
	if duration != 60 && duration != 90 && duration != 120 {
		happyOysterBusinessError(c, 400000, "maxExperienceTimeSec must be 60, 90, or 120")
		return
	}
	hash := sha256.Sum256([]byte(req.Ticket))
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	ticketHash := hex.EncodeToString(hash[:])
	ticket, err := model.GetHappyOysterTicket(userId, ticketHash)
	if err != nil {
		happyOysterError(c, 400, "ticket is invalid, expired, or already used")
		return
	}
	var world model.HappyOysterWorld
	if model.DB.First(&world, ticket.WorldId).Error != nil {
		happyOysterError(c, 404, "world not found")
		return
	}
	if !happyOysterClientTokenAllowsWorld(c, &world) {
		happyOysterError(c, 403, "temporary API key and ticket belong to different channel accounts")
		return
	}
	channel, key, err := happyOysterPinned(c, world.ChannelId, world.ChannelMultiKeyIndex, world.ChannelAccountHash, world.BaseURL)
	if err != nil {
		happyOysterError(c, 503, err.Error())
		return
	}
	info, err := happyOysterRelayInfo(c)
	if err != nil {
		happyOysterError(c, 500, err.Error())
		return
	}
	quota, err := happyOysterPrice(c, info, channel.Other, world.BaseURL, 0, duration)
	if err != nil {
		happyOysterError(c, 400, err.Error())
		return
	}
	if apiErr := service.PreConsumeBilling(c, quota, info); apiErr != nil {
		happyOysterError(c, apiErr.StatusCode, apiErr.Error())
		return
	}
	ticket, err = model.ClaimHappyOysterTicket(userId, ticketHash)
	if err != nil {
		if info.Billing != nil {
			info.Billing.Refund(c)
		}
		happyOysterError(c, 400, "ticket is invalid, expired, or already used")
		return
	}
	status, payload, envelope, err := happyOysterProxy(c, channel, key, http.MethodPost, "/travels/enter-travel", nil, body)
	if err != nil || status < 200 || status >= 300 || !envelope.successful() {
		if info.Billing != nil {
			info.Billing.Refund(c)
		}
		if err != nil {
			happyOysterError(c, 502, err.Error())
			return
		}
		c.Data(status, "application/json", payload)
		return
	}
	var data happyOysterTravelData
	if common.Unmarshal(envelope.Data, &data) != nil || !strings.HasPrefix(data.EncryptedTravelId, "trvl_") || (data.EncryptedWorldId != "" && data.EncryptedWorldId != world.EncryptedWorldId) {
		_ = service.SettleBilling(c, info, quota)
		_, _ = happyOysterLog(c, info, quota, "HappyOyster travel returned invalid routing data", world.EncryptedWorldId, "")
		happyOysterError(c, 502, "upstream returned an invalid travel")
		return
	}
	adoptedDuration := duration
	if data.MaxExperienceTimeSec != nil {
		adoptedDuration = *data.MaxExperienceTimeSec
		if adoptedDuration != 60 && adoptedDuration != 90 && adoptedDuration != 120 {
			_ = service.SettleBilling(c, info, quota)
			_, _ = happyOysterLog(c, info, quota, "HappyOyster travel returned invalid duration", world.EncryptedWorldId, data.EncryptedTravelId)
			happyOysterError(c, 502, "upstream returned an invalid maxExperienceTimeSec")
			return
		}
	}
	snapshot, _ := common.Marshal(info.TieredBillingSnapshot)
	travel := model.HappyOysterTravel{UserId: userId, TokenId: info.TokenId, WorldRecordId: world.Id, EncryptedWorldId: world.EncryptedWorldId, EncryptedTravelId: data.EncryptedTravelId,
		ChannelId: world.ChannelId, ChannelMultiKeyIndex: world.ChannelMultiKeyIndex, ChannelAccountHash: world.ChannelAccountHash, BaseURL: world.BaseURL, Status: "pending",
		MaxExperienceSeconds: adoptedDuration, PreConsumedQuota: quota, BillingSource: info.BillingSource, SubscriptionId: info.SubscriptionId, BillingSnapshot: string(snapshot), UsingGroup: info.UsingGroup, SettlementState: model.HappyOysterSettlementPending}
	travel.ClientUserId = common.GetContextKeyString(c, constant.ContextKeyClientUserId)
	travel.ClientScenairo = common.GetContextKeyString(c, constant.ContextKeyClientScenairo)
	travel.ProjectId = common.GetContextKeyInt(c, constant.ContextKeyProjectId)
	if err := model.DB.Create(&travel).Error; err != nil {
		_ = service.SettleBilling(c, info, quota)
		_, _ = happyOysterLog(c, info, quota, "HappyOyster travel routing persistence failure", world.EncryptedWorldId, data.EncryptedTravelId)
		happyOysterError(c, 500, "failed to persist travel routing")
		return
	}
	model.DB.Model(ticket).Updates(map[string]any{"state": model.HappyOysterTicketConsumed, "travel_id": travel.Id, "updated_at": time.Now().Unix()})
	travel.ProjectName, travel.PlanId = happyOysterLog(c, info, quota, "HappyOyster travel pre-consume", world.EncryptedWorldId, travel.EncryptedTravelId)
	model.DB.Model(&travel).Updates(map[string]any{"project_name": travel.ProjectName, "plan_id": travel.PlanId})
	c.Data(status, "application/json", payload)
}

func HappyOysterTravelOperation(c *gin.Context) {
	if _, scoped := happyOysterClientToken(c); scoped && !happyOysterTemporaryAPIKeyAllows(strings.TrimPrefix(c.FullPath(), happyoyster.APIPath)) {
		happyOysterBusinessError(c, 403003, "temporary API keys cannot call this endpoint")
		return
	}
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	travelId := c.Query("encryptedTravelId")
	body := []byte(nil)
	var err error
	if c.Request.Method == http.MethodPost {
		body, err = happyOysterBody(c)
		if err != nil {
			happyOysterError(c, 400, err.Error())
			return
		}
		var v struct {
			EncryptedTravelId string `json:"encryptedTravelId"`
		}
		if common.Unmarshal(body, &v) == nil && v.EncryptedTravelId != "" {
			travelId = v.EncryptedTravelId
		}
	}
	if travelId == "" || len(travelId) > 512 {
		happyOysterBusinessError(c, 400000, "encryptedTravelId is required and must not exceed 512 characters")
		return
	}
	travel, err := model.GetHappyOysterTravel(userId, travelId)
	if err != nil {
		happyOysterError(c, 404, "travel not found")
		return
	}
	if !happyOysterClientTokenAllowsTravel(c, travel) {
		happyOysterError(c, 403, "temporary API key and Travel belong to different channel accounts")
		return
	}
	channel, key, err := happyOysterPinned(c, travel.ChannelId, travel.ChannelMultiKeyIndex, travel.ChannelAccountHash, travel.BaseURL)
	if err != nil {
		happyOysterError(c, 503, err.Error())
		return
	}
	suffix := strings.TrimPrefix(c.FullPath(), happyoyster.APIPath)
	status, payload, envelope, err := happyOysterProxy(c, channel, key, c.Request.Method, suffix, c.Request.URL.Query(), body)
	if err != nil {
		happyOysterError(c, 502, err.Error())
		return
	}
	if status >= 200 && status < 300 && envelope.successful() && (suffix == "/travels/status" || suffix == "/travels/end") {
		var data happyOysterTravelData
		if common.Unmarshal(envelope.Data, &data) == nil {
			updates := map[string]any{}
			if data.Status != "" {
				updates["status"] = data.Status
			}
			if data.DurationSec != nil {
				updates["duration_seconds"] = *data.DurationSec
			}
			if data.EndedAt != "" {
				updates["ended_at"] = data.EndedAt
			}
			if len(updates) != 0 {
				model.DB.Model(travel).Updates(updates)
			}
			if data.Status == "failed" {
				duration := 0
				if data.DurationSec != nil {
					duration = *data.DurationSec
				}
				happyOysterSettleTravel(c, travel, duration, data.Status)
			} else if data.Status == "completed" && data.DurationSec != nil {
				happyOysterSettleTravel(c, travel, *data.DurationSec, data.Status)
			}
		}
	}
	if status >= 200 && status < 300 && envelope.successful() && suffix == "/travels/artifacts" {
		var data happyOysterArtifactsData
		if common.Unmarshal(envelope.Data, &data) == nil && data.Video.Original.DurationSec != nil {
			happyOysterSettleTravel(c, travel, *data.Video.Original.DurationSec, "completed")
		}
	}
	c.Data(status, "application/json", payload)
}

func happyOysterSettleTravel(ctx context.Context, travel *model.HappyOysterTravel, seconds int, status string) {
	seconds = min(max(seconds, 0), travel.MaxExperienceSeconds)
	var snap billingexpr.BillingSnapshot
	if common.Unmarshal([]byte(travel.BillingSnapshot), &snap) != nil {
		return
	}
	result, _, err := service.EvaluateTaskCompletionUsage(&snap, map[string]any{"experience_seconds": seconds})
	if err != nil || result.Clamp != nil {
		return
	}
	actual := result.ActualQuotaAfterGroup
	delta := actual - travel.PreConsumedQuota
	claimed, err := model.ClaimHappyOysterSettlement(travel.Id)
	if err != nil || !claimed {
		return
	}
	if delta != 0 {
		if travel.BillingSource == service.BillingSourceSubscription && travel.SubscriptionId > 0 {
			err = model.PostConsumeUserSubscriptionDelta(travel.SubscriptionId, int64(delta))
		} else if delta > 0 {
			err = model.DecreaseUserQuota(travel.UserId, delta, false)
		} else {
			err = model.IncreaseUserQuota(travel.UserId, -delta, false)
		}
		if err != nil {
			model.RetryHappyOysterSettlement(travel.Id)
			return
		}
		if token, tokenErr := model.GetTokenById(travel.TokenId); tokenErr == nil {
			if delta > 0 {
				tokenErr = model.DecreaseTokenQuota(travel.TokenId, token.Key, delta)
			} else {
				tokenErr = model.IncreaseTokenQuota(travel.TokenId, token.Key, -delta)
			}
			if tokenErr != nil {
				logger.LogWarn(ctx, fmt.Sprintf("HappyOyster settlement token adjustment failed: travel_id=%d delta=%d: %v", travel.Id, delta, tokenErr))
			}
		} else {
			logger.LogWarn(ctx, fmt.Sprintf("HappyOyster settlement token lookup failed: travel_id=%d token_id=%d: %v", travel.Id, travel.TokenId, tokenErr))
		}
		model.UpdateUserUsedQuota(travel.UserId, delta)
		model.UpdateChannelUsedQuota(travel.ChannelId, delta)
		if travel.ProjectId > 0 && travel.ClientUserId != "" {
			if delta > 0 {
				err = model.IncreaseUsedQuotaByProjectAndUser(travel.ProjectId, travel.ClientUserId, delta)
			} else {
				err = model.DecreaseUsedQuotaByProjectAndUser(travel.ProjectId, travel.ClientUserId, -delta)
			}
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("HappyOyster settlement project adjustment failed: travel_id=%d project_id=%d delta=%d: %v", travel.Id, travel.ProjectId, delta, err))
			}
		}
	}
	if err := model.FinishHappyOysterSettlement(travel.Id, actual, seconds, status); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("HappyOyster settlement persistence failed: travel_id=%d: %v", travel.Id, err))
		return
	}
	other := model.NewLogOther()
	other.SetPublic("world_id", travel.EncryptedWorldId)
	other.SetPublic("travel_id", travel.EncryptedTravelId)
	other.SetPublic("duration_sec", seconds)
	other.SetPublic("pre_consumed_quota", travel.PreConsumedQuota)
	other.SetPublic("actual_quota", actual)
	logType, amount := model.LogTypeConsume, delta
	if delta < 0 {
		logType, amount = model.LogTypeRefund, -delta
	}
	if delta != 0 {
		model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{UserId: travel.UserId, LogType: logType, Content: "HappyOyster travel settlement", ChannelId: travel.ChannelId, ModelName: happyOysterModel, Quota: amount, TokenId: travel.TokenId, Group: travel.UsingGroup, Other: other, ClientUserId: travel.ClientUserId, ClientScenairo: travel.ClientScenairo, ProjectName: travel.ProjectName, PlanId: travel.PlanId})
	}
}

// RunHappyOysterPollingOnce settles travels even when the client disconnects
// before calling the status or end endpoint.
func RunHappyOysterPollingOnce(ctx context.Context) int {
	processed := 0
	for _, travel := range model.GetUnsettledHappyOysterTravels(100) {
		if ctx.Err() != nil {
			break
		}
		channel, err := model.GetChannelById(travel.ChannelId, true)
		if err != nil || channel.Type != constant.ChannelTypeHappyOyster || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		keys := channel.GetKeys()
		if travel.ChannelMultiKeyIndex < 0 || travel.ChannelMultiKeyIndex >= len(keys) {
			continue
		}
		if channel.ChannelInfo.IsMultiKey {
			if keyStatus, exists := channel.ChannelInfo.MultiKeyStatusList[travel.ChannelMultiKeyIndex]; exists && keyStatus != common.ChannelStatusEnabled {
				continue
			}
		}
		key := keys[travel.ChannelMultiKeyIndex]
		if happyOysterAccountHash(travel.BaseURL, key) != travel.ChannelAccountHash {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/", nil)
		if err != nil {
			continue
		}
		query := url.Values{"encryptedTravelId": []string{travel.EncryptedTravelId}}
		status, payload, err := happyoyster.Do(req, travel.BaseURL, key, channel.GetSetting().Proxy, channel.GetSetting(), http.MethodGet, "/travels/status", query, nil)
		if err != nil || status < 200 || status >= 300 {
			continue
		}
		_, envelope, _, err := decodeHappyOysterResponse(payload)
		if err != nil || !envelope.successful() {
			continue
		}
		var data happyOysterTravelData
		if common.Unmarshal(envelope.Data, &data) != nil {
			continue
		}
		updates := map[string]any{}
		if data.Status != "" {
			updates["status"] = data.Status
		}
		if data.DurationSec != nil {
			updates["duration_seconds"] = *data.DurationSec
		}
		if data.EndedAt != "" {
			updates["ended_at"] = data.EndedAt
		}
		if len(updates) != 0 {
			model.DB.Model(&travel).Updates(updates)
		}
		if data.Status == "failed" {
			duration := 0
			if data.DurationSec != nil {
				duration = *data.DurationSec
			}
			happyOysterSettleTravel(ctx, &travel, duration, data.Status)
		} else if data.Status == "completed" && data.DurationSec != nil {
			happyOysterSettleTravel(ctx, &travel, *data.DurationSec, data.Status)
		} else if data.Status == "completed" {
			artifactStatus, artifactPayload, artifactErr := happyoyster.Do(req, travel.BaseURL, key, channel.GetSetting().Proxy, channel.GetSetting(), http.MethodGet, "/travels/artifacts", query, nil)
			if artifactErr == nil && artifactStatus >= 200 && artifactStatus < 300 {
				_, artifactEnvelope, _, decodeErr := decodeHappyOysterResponse(artifactPayload)
				var artifacts happyOysterArtifactsData
				if decodeErr == nil && artifactEnvelope.successful() && common.Unmarshal(artifactEnvelope.Data, &artifacts) == nil && artifacts.Video.Original.DurationSec != nil {
					happyOysterSettleTravel(ctx, &travel, *artifacts.Video.Original.DurationSec, "completed")
				}
			}
		}
		processed++
	}
	return processed
}

func HappyOysterListWorlds(c *gin.Context)  { happyOysterList(c, true) }
func HappyOysterListTravels(c *gin.Context) { happyOysterList(c, false) }
func happyOysterList(c *gin.Context, worlds bool) {
	if _, scoped := happyOysterClientToken(c); scoped {
		happyOysterBusinessError(c, 403003, "temporary API keys cannot call this endpoint")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		happyOysterBusinessError(c, 400000, "page must be an integer")
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if err != nil {
		happyOysterBusinessError(c, 400000, "pageSize must be an integer")
		return
	}
	page = max(page, 1)
	if size <= 0 {
		size = 20
	} else {
		size = min(size, 100)
	}
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if worlds {
		if mode := c.Query("mode"); mode != "" && mode != "1" {
			happyOysterBusinessError(c, 400000, "mode must be 1 for HappyOyster Adventure")
			return
		}
		rows, total, err := model.ListHappyOysterWorlds(userId, page, size, c.Query("status"))
		if err != nil {
			happyOysterError(c, 500, err.Error())
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, gin.H{"encryptedWorldId": row.EncryptedWorldId, "name": row.Name, "status": row.Status, "mode": 1, "previewUrl": nil, "createdAt": time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339), "perspective": row.Perspective, "creationModel": row.CreationModel, "uploadMode": row.UploadMode})
		}
		c.JSON(200, gin.H{"code": 0, "message": nil, "data": gin.H{"items": items, "pagination": gin.H{"page": page, "pageSize": size, "total": total, "hasMore": int64(page*size) < total}}})
		return
	}
	if c.Query("mode") != "" {
		happyOysterBusinessError(c, 400000, "mode is not supported by the travel list endpoint")
		return
	}
	if worldId := c.Query("encryptedWorldId"); worldId != "" {
		if len(worldId) > 512 {
			happyOysterBusinessError(c, 400000, "encryptedWorldId is too long")
			return
		}
		if _, err := model.GetHappyOysterWorld(userId, worldId); err != nil {
			happyOysterError(c, 403, "world is not available to this account")
			return
		}
	}
	rows, total, err := model.ListHappyOysterTravels(userId, page, size, c.Query("status"), c.Query("encryptedWorldId"))
	if err != nil {
		happyOysterError(c, 500, err.Error())
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		var endedAt any
		var duration any
		if row.EndedAt != "" {
			endedAt = row.EndedAt
		}
		if row.DurationSeconds > 0 {
			duration = row.DurationSeconds
		}
		items = append(items, gin.H{"encryptedTravelId": row.EncryptedTravelId, "status": row.Status, "mode": 1, "encryptedWorldId": row.EncryptedWorldId, "durationSec": duration, "createdAt": time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339), "endedAt": endedAt})
	}
	c.JSON(200, gin.H{"code": 0, "message": nil, "data": gin.H{"items": items, "pagination": gin.H{"page": page, "pageSize": size, "total": total, "hasMore": int64(page*size) < total}}})
}

func HappyOysterIssueTemporaryAPIKey(c *gin.Context) {
	expireIn := 60
	if raw := c.Query("expire_in_seconds"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			happyOysterError(c, http.StatusBadRequest, "expire_in_seconds must be an integer between 1 and 1800")
			return
		}
		expireIn = parsed
	}
	if expireIn < 1 || expireIn > 1800 {
		happyOysterError(c, http.StatusBadRequest, "expire_in_seconds must be between 1 and 1800")
		return
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeHappyOyster {
		happyOysterError(c, http.StatusBadRequest, "selected channel is not HappyOyster Adventure")
		return
	}
	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		happyOysterError(c, http.StatusBadGateway, err.Error())
		return
	}
	key := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	baseURL := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
	status, payload, err := happyoyster.GenerateTemporaryAPIKey(c.Request, baseURL, key, channel.GetSetting().Proxy, channel.GetSetting(), expireIn)
	if err != nil {
		happyOysterError(c, http.StatusBadGateway, err.Error())
		return
	}
	if status < 200 || status >= 300 {
		c.Data(status, "application/json", payload)
		return
	}
	var response struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if common.Unmarshal(payload, &response) != nil || !strings.HasPrefix(response.Token, "st-") || response.ExpiresAt <= time.Now().Unix() {
		happyOysterError(c, http.StatusBadGateway, "upstream returned an invalid temporary API key")
		return
	}
	sum := sha256.Sum256([]byte(response.Token))
	record := model.HappyOysterClientToken{
		TokenHash: hex.EncodeToString(sum[:]), UserId: common.GetContextKeyInt(c, constant.ContextKeyUserId), TokenId: common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		ChannelId: channelId, ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex), ChannelAccountHash: happyOysterAccountHash(baseURL, key),
		BaseURL: baseURL, Scopes: "world:build-status,travel:enter,travel:status,travel:end", ExpiresAt: response.ExpiresAt,
	}
	if model.DB.Create(&record).Error != nil {
		happyOysterError(c, http.StatusInternalServerError, "failed to persist temporary API key")
		return
	}
	logger.LogInfo(c, fmt.Sprintf("HappyOyster temporary API key issued: user_id=%d token_id=%d channel_id=%d expires_at=%d", record.UserId, record.TokenId, record.ChannelId, record.ExpiresAt))
	c.Header("Cache-Control", "no-store")
	c.Data(status, "application/json", payload)
}

func happyOysterLog(c *gin.Context, info *relaycommon.RelayInfo, quota int, content, worldId, travelId string) (string, int) {
	other := model.NewLogOther()
	other.SetPublic("protocol", "happyoyster-adventure")
	other.SetPublic("world_id", worldId)
	if travelId != "" {
		other.SetPublic("travel_id", travelId)
	}
	if info.TieredBillingSnapshot != nil {
		other.SetPublic("billing_mode", "tiered_expr")
		other.SetPublic("matched_tier", info.TieredBillingSnapshot.EstimatedTier)
		other.SetPublic("usage_facts", info.TieredBillingSnapshot.UsageFacts)
	}
	service.AttachQuotaSaturation(c, info, other)
	projectName, planId, _ := service.TrackProjectConsumption(c, quota)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{ChannelId: info.ChannelId, ModelName: happyOysterModel, TokenName: c.GetString("token_name"), Quota: quota, Content: content, TokenId: info.TokenId, Group: info.UsingGroup, Other: other, RequestId: c.GetString(common.RequestIdKey), ClientUserId: common.GetContextKeyString(c, constant.ContextKeyClientUserId), ClientScenairo: common.GetContextKeyString(c, constant.ContextKeyClientScenairo), ProjectName: projectName, PlanId: planId})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, quota)
	model.UpdateChannelUsedQuota(info.ChannelId, quota)
	return projectName, planId
}
