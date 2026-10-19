package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// tieredCacheCreation1hMultiplier 阶梯档位 1h 缓存创建价相对 5m 基准价的倍数。
// 与 relay/helper.claudeCacheCreation1hMultiplier 保持一致（6/3.75 ≈ 1.6）。
// 参见 https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const tieredCacheCreation1hMultiplier = 6.0 / 3.75

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
	ImageTokens int
	VideoTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
	// 阶梯价格相关字段
	UseTieredPrice    bool
	TieredInputPrice  float64
	TieredOutputPrice float64
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp) {
	if info.UseTieredPrice {
		// 阶梯价格计费：文本 tokens 使用阶梯价格，音频/视频 tokens 使用原始倍率
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)
		dMillion := decimal.NewFromInt(1_000_000)
		inputPrice := decimal.NewFromFloat(info.TieredInputPrice).Div(dMillion)
		outputPrice := decimal.NewFromFloat(info.TieredOutputPrice).Div(dMillion)

		// 文本 tokens 使用阶梯价格
		inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
		outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
		textInputQuota := inputTextTokens.Mul(inputPrice).Mul(quotaPerUnit).Mul(groupRatio)
		textOutputQuota := outputTextTokens.Mul(outputPrice).Mul(quotaPerUnit).Mul(groupRatio)
		quota := textInputQuota.Add(textOutputQuota)

		// 音频/视频/图片 tokens 使用原始倍率 * 阶梯输入价格
		audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
		audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))
		imageRatio, _ := ratio_setting.GetImageRatio(info.ModelName)
		dImageRatio := decimal.NewFromFloat(imageRatio)
		videoRatio := decimal.NewFromFloat(ratio_setting.GetVideoRatio(info.ModelName))

		inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
		outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))
		inputImageTokens := decimal.NewFromInt(int64(info.InputDetails.ImageTokens))
		inputVideoTokens := decimal.NewFromInt(int64(info.InputDetails.VideoTokens))

		if !inputAudioTokens.IsZero() {
			quota = quota.Add(inputAudioTokens.Mul(audioRatio).Mul(inputPrice).Mul(quotaPerUnit).Mul(groupRatio))
		}
		if !outputAudioTokens.IsZero() {
			quota = quota.Add(outputAudioTokens.Mul(audioCompletionRatio).Mul(outputPrice).Mul(quotaPerUnit).Mul(groupRatio))
		}
		if !inputImageTokens.IsZero() {
			quota = quota.Add(inputImageTokens.Mul(dImageRatio).Mul(inputPrice).Mul(quotaPerUnit).Mul(groupRatio))
		}
		if !inputVideoTokens.IsZero() {
			quota = quota.Add(inputVideoTokens.Mul(videoRatio).Mul(inputPrice).Mul(quotaPerUnit).Mul(groupRatio))
		}

		return common.QuotaFromDecimalChecked(quota)
	}
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return common.QuotaFromDecimalChecked(quota)
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))
	imgRatio, _ := ratio_setting.GetImageRatio(info.ModelName)
	dImageRatio := decimal.NewFromFloat(imgRatio)
	videoRatio := decimal.NewFromFloat(ratio_setting.GetVideoRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))
	inputImageTokens := decimal.NewFromInt(int64(info.InputDetails.ImageTokens))
	inputVideoTokens := decimal.NewFromInt(int64(info.InputDetails.VideoTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioCompletionRatio))
	quota = quota.Add(inputImageTokens.Mul(dImageRatio))
	quota = quota.Add(inputVideoTokens.Mul(videoRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return common.QuotaFromDecimalChecked(quota)
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return err
	}

	token, err := model.GetTokenByKey(strings.TrimPrefix(relayInfo.TokenKey, "sk-"), false)
	if err != nil {
		return err
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	videoInputTokens := usage.InputTokenDetails.VideoTokens
	groupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)

	autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup)
	if exists {
		groupRatio = ratio_setting.GetGroupRatio(autoGroup.(string))
		logger.LogDebug(ctx, "final group ratio: %f", groupRatio)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	actualGroupRatio := groupRatio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		actualGroupRatio = userGroupRatio
	}

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
			VideoTokens: videoInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:      modelName,
		UsePrice:       relayInfo.UsePrice,
		ModelRatio:     modelRatio,
		GroupRatio:     actualGroupRatio,
		UseTieredPrice: relayInfo.PriceData.UseTieredPrice,
	}
	if relayInfo.PriceData.UseTieredPrice {
		// 阶梯价格：根据实际 inputTokens 重新匹配档位
		tieredPriceTiers, useTiered := ratio_setting.GetTieredPrice(modelName)
		if useTiered && len(tieredPriceTiers) > 0 {
			tier := ratio_setting.MatchPriceTier(tieredPriceTiers, textInputTokens+audioInputTokens+videoInputTokens)
			quotaInfo.TieredInputPrice = tier.InputPrice
			quotaInfo.TieredOutputPrice = tier.OutputPrice
		}
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)

	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
	}

	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}

	err = PostConsumeQuota(relayInfo, quota, 0, false)
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, common.LogText("realtime streaming consume quota success, quota: %d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent []*common.Message) {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVarsByHash(snap.ExprString, snap.ExprHash)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredRealtimeTokenParams(usage, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	videoInputTokens := usage.InputTokenDetails.VideoTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))
	videoRatio := decimal.NewFromFloat(ratio_setting.GetVideoRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
			VideoTokens: videoInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:      modelName,
		UsePrice:       usePrice,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		UseTieredPrice: relayInfo.PriceData.UseTieredPrice,
	}
	if relayInfo.PriceData.UseTieredPrice {
		// 阶梯价格：根据实际 inputTokens 重新匹配档位
		tieredPriceTiers, useTiered := ratio_setting.GetTieredPrice(modelName)
		if useTiered && len(tieredPriceTiers) > 0 {
			tier := ratio_setting.MatchPriceTier(tieredPriceTiers, textInputTokens+audioInputTokens+videoInputTokens)
			quotaInfo.TieredInputPrice = tier.InputPrice
			quotaInfo.TieredOutputPrice = tier.OutputPrice
			// 更新 PriceData 中的档位信息，确保日志展示正确的匹配档位
			relayInfo.PriceData.TieredInputPrice = tier.InputPrice
			relayInfo.PriceData.TieredOutputPrice = tier.OutputPrice
			relayInfo.PriceData.TieredMaxTokens = tier.MaxTokens
		}
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent []*common.Message
	if tieredOk {
		logContent = []*common.Message{tieredRealtimeLogContent(relayInfo, usage, tieredResult)}
		if tieredResult == nil {
			logContent = append(logContent, common.NewMessage("Settlement used the reserved quota"))
		}
	} else if relayInfo.PriceData.UseTieredPrice {
		logContent = []*common.Message{common.NewMessage("Tiered price (up to {{max_tokens}} tokens): input {{input_price}} and output {{output_price}} per 1M tokens, audio ratio {{audio_ratio}}, audio completion ratio {{audio_completion_ratio}}, group ratio {{group_ratio}}", map[string]any{
			"max_tokens":             relayInfo.PriceData.TieredMaxTokens,
			"input_price":            fmt.Sprintf("%.6f", quotaInfo.TieredInputPrice),
			"output_price":           fmt.Sprintf("%.6f", quotaInfo.TieredOutputPrice),
			"audio_ratio":            fmt.Sprintf("%.2f", audioRatio.InexactFloat64()),
			"audio_completion_ratio": fmt.Sprintf("%.2f", audioCompletionRatio.InexactFloat64()),
			"group_ratio":            fmt.Sprintf("%.2f", groupRatio),
		})}
		logContent = append(logContent, common.NewMessage("Video ratio {{ratio}}", map[string]any{"ratio": fmt.Sprintf("%.2f", videoRatio.InexactFloat64())}))
	} else if !usePrice {
		logContent = []*common.Message{common.NewMessage("Model ratio {{model_ratio}}, completion ratio {{completion_ratio}}, audio ratio {{audio_ratio}}, audio completion ratio {{audio_completion_ratio}}, group ratio {{group_ratio}}", map[string]any{
			"model_ratio":            fmt.Sprintf("%.2f", modelRatio),
			"completion_ratio":       fmt.Sprintf("%.2f", completionRatio.InexactFloat64()),
			"audio_ratio":            fmt.Sprintf("%.2f", audioRatio.InexactFloat64()),
			"audio_completion_ratio": fmt.Sprintf("%.2f", audioCompletionRatio.InexactFloat64()),
			"group_ratio":            fmt.Sprintf("%.2f", groupRatio),
		})}
		logContent = append(logContent, common.NewMessage("Video ratio {{ratio}}", map[string]any{"ratio": fmt.Sprintf("%.2f", videoRatio.InexactFloat64())}))
	} else {
		logContent = append(logContent, common.NewMessage("Model price {{model_price}}, group ratio {{group_ratio}}", map[string]any{
			"model_price": fmt.Sprintf("%.2f", modelPrice),
			"group_ratio": fmt.Sprintf("%.2f", groupRatio),
		}))
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent = append(logContent, common.NewMessage("Possibly an upstream timeout"))
		logger.LogError(ctx, common.LogText("total tokens is 0, cannot consume quota, userId %d, channelId %d, tokenId %d, model %s， pre-consumed quota %d",
			relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// Track project consumption and get project name for logging
	projectName, planId, _ := TrackProjectConsumption(ctx, quota)
	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, common.LogText("error settling billing: %s", err.Error()))
	}

	logModel := modelName
	logContent = append(logContent, extraContent...)
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), videoRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if relayInfo.PriceData.UseTieredPrice {
		other.SetPublic("use_tiered_price", true)
		other.SetPublic("tiered_input_price", relayInfo.PriceData.TieredInputPrice)
		other.SetPublic("tiered_output_price", relayInfo.PriceData.TieredOutputPrice)
		other.SetPublic("tiered_max_tokens", relayInfo.PriceData.TieredMaxTokens)
		if relayInfo.PriceData.TieredCachedInputPrice > 0 {
			other.SetPublic("tiered_cached_input_price", relayInfo.PriceData.TieredCachedInputPrice)
		}
		if relayInfo.PriceData.TieredCacheWritePrice > 0 {
			other.SetPublic("tiered_cache_write_price", relayInfo.PriceData.TieredCacheWritePrice)
		}
	}
	clientUserId := common.GetContextKeyString(ctx, constant.ContextKeyClientUserId)
	clientScenairo := common.GetContextKeyString(ctx, constant.ContextKeyClientScenairo)
	sessionId := common.GetContextKeyString(ctx, constant.ContextKeyClaudeSessionId)
	requestId := ctx.GetString(common.RequestIdKey)

	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)

	var usageStr string
	if usage != nil {
		if usageBytes, err := common.Marshal(usage); err == nil {
			usageStr = string(usageBytes)
		}
	}

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Request:          strings.Join(relayInfo.WsRequestMessages, "\n"),
		Response:         strings.Join(relayInfo.WsResponseMessages, "\n"),
		ClientUserId:     clientUserId,
		ClientScenairo:   clientScenairo,
		SessionId:        sessionId,
		RequestId:        requestId,
		ProjectName:      projectName,
		PlanId:           planId,
		Usage:            usageStr,
	})
}

func tieredRealtimeLogContent(relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, result *billingexpr.TieredResult) *common.Message {
	tier := "-"
	unit := billingexpr.BillingUnitToken
	if result != nil {
		if result.MatchedTier != "" {
			tier = result.MatchedTier
		}
		if result.BillingUnit != "" {
			unit = result.BillingUnit
		}
	} else if snapshot := relayInfo.TieredBillingSnapshot; snapshot != nil {
		if snapshot.EstimatedTier != "" {
			tier = snapshot.EstimatedTier
		}
		if snapshot.EstimatedBillingUnit != "" {
			unit = snapshot.EstimatedBillingUnit
		}
	}

	return common.NewMessage("Billing expression: tier {{tier}}, unit {{unit}}, input {{input}} tokens (text {{input_text}}, audio {{input_audio}}, image {{input_image}}, video {{input_video}}), output {{output}} tokens (text {{output_text}}, audio {{output_audio}}, image {{output_image}}), group ratio {{group_ratio}}", map[string]any{
		"tier": tier, "unit": unit, "input": usage.InputTokens,
		"input_text": usage.InputTokenDetails.TextTokens, "input_audio": usage.InputTokenDetails.AudioTokens, "input_image": usage.InputTokenDetails.ImageTokens, "input_video": usage.InputTokenDetails.VideoTokens,
		"output": usage.OutputTokens, "output_text": usage.OutputTokenDetails.TextTokens, "output_audio": usage.OutputTokenDetails.AudioTokens, "output_image": usage.OutputTokenDetails.ImageTokens,
		"group_ratio": fmt.Sprintf("%.2f", relayInfo.PriceData.GroupRatioInfo.GroupRatio),
	})
}

// BuildTieredRealtimeTokenParams normalizes Realtime usage for expression
// billing. Realtime input/output totals include their modality details, so a
// separately referenced modality must be removed from the p/c fallback.
func BuildTieredRealtimeTokenParams(usage *dto.RealtimeUsage, usedVars map[string]bool) billingexpr.TokenParams {
	if usage == nil {
		return billingexpr.TokenParams{}
	}
	p := float64(usage.InputTokens)
	c := float64(usage.OutputTokens)
	cr := float64(usage.InputTokenDetails.CachedTokens)
	ai := float64(usage.InputTokenDetails.AudioTokens)
	img := float64(usage.InputTokenDetails.ImageTokens)
	ao := float64(usage.OutputTokenDetails.AudioTokens)
	imgO := float64(usage.OutputTokenDetails.ImageTokens)
	if usedVars["cr"] {
		p -= cr
	}
	if usedVars["ai"] {
		p -= ai
	}
	if usedVars["img"] {
		p -= img
	}
	if usedVars["ao"] {
		c -= ao
	}
	if usedVars["img_o"] {
		c -= imgO
	}
	return billingexpr.TokenParams{
		P:    max(p, 0),
		C:    max(c, 0),
		Len:  float64(usage.InputTokens),
		CR:   cr,
		AI:   ai,
		Img:  img,
		AO:   ao,
		ImgO: imgO,
	}
}

// ReserveWssUsage raises the current request reservation to the cumulative
// realtime usage cost. It avoids directly charging each partial update, which
// would otherwise be charged again during final settlement.
func ReserveWssUsage(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo == nil || usage == nil {
		return errors.New("invalid realtime billing state")
	}

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  usage.InputTokenDetails.TextTokens,
			AudioTokens: usage.InputTokenDetails.AudioTokens,
			ImageTokens: usage.InputTokenDetails.ImageTokens,
			VideoTokens: usage.InputTokenDetails.VideoTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  usage.OutputTokenDetails.TextTokens,
			AudioTokens: usage.OutputTokenDetails.AudioTokens,
			ImageTokens: usage.OutputTokenDetails.ImageTokens,
		},
		ModelName:      relayInfo.GetBillingModelName(),
		UsePrice:       relayInfo.PriceData.UsePrice,
		ModelPrice:     relayInfo.PriceData.ModelPrice,
		ModelRatio:     relayInfo.PriceData.ModelRatio,
		GroupRatio:     relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		UseTieredPrice: relayInfo.PriceData.UseTieredPrice,
	}
	if relayInfo.PriceData.UseTieredPrice {
		tiers, ok := ratio_setting.GetTieredPrice(quotaInfo.ModelName)
		if ok && len(tiers) > 0 {
			tier := ratio_setting.MatchPriceTier(tiers, usage.InputTokens)
			quotaInfo.TieredInputPrice = tier.InputPrice
			quotaInfo.TieredOutputPrice = tier.OutputPrice
		}
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		usedVars := billingexpr.UsedVarsByHash(snap.ExprString, snap.ExprHash)
		if ok, tieredQuota, _ := TryTieredSettle(relayInfo, BuildTieredRealtimeTokenParams(usage, usedVars)); ok {
			quota = tieredQuota
		}
	}
	if relayInfo.Billing == nil {
		if quota == 0 {
			return nil
		}
		return errors.New("realtime billing session is missing")
	}
	if err := relayInfo.Billing.Reserve(quota); err != nil {
		return err
	}
	relayInfo.FinalPreConsumedQuota = relayInfo.Billing.GetPreConsumedQuota()
	logger.LogInfo(ctx, "realtime streaming reserve quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	value := (cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)
	quota, clamp := common.QuotaRoundChecked(value)
	if clamp != nil {
		return -1
	}
	return quota
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent []*common.Message, bodies ...string) {
	var requestStr, responseStr string
	if len(bodies) >= 2 {
		requestStr, responseStr = bodies[0], bodies[1]
	}
	if usage == nil {
		usage = &dto.Usage{PromptTokens: relayInfo.GetEstimatePromptTokens(), TotalTokens: relayInfo.GetEstimatePromptTokens()}
	}

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVarsByHash(snap.ExprString, snap.ExprHash)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}
	fixedPriceBilling := tieredOk && isFixedPriceSettlement(relayInfo, tieredRes)

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens
	imageInputTokens := usage.PromptTokensDetails.ImageTokens
	videoInputTokens := usage.PromptTokensDetails.VideoTokens

	tokenName := ctx.GetString("token_name")
	billingModelName := relayInfo.GetBillingModelName()
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(billingModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(billingModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(billingModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
			ImageTokens: imageInputTokens,
			VideoTokens: videoInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  billingModelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent []*common.Message
	if relayInfo.PriceData.UseTieredPrice {
		logContent = []*common.Message{common.NewMessage("Tiered price (up to {{max_tokens}} tokens): input {{input_price}} and output {{output_price}} per 1M tokens, audio ratio {{audio_ratio}}, audio completion ratio {{audio_completion_ratio}}, group ratio {{group_ratio}}", map[string]any{
			"max_tokens":             relayInfo.PriceData.TieredMaxTokens,
			"input_price":            fmt.Sprintf("%.6f", quotaInfo.TieredInputPrice),
			"output_price":           fmt.Sprintf("%.6f", quotaInfo.TieredOutputPrice),
			"audio_ratio":            fmt.Sprintf("%.2f", audioRatio.InexactFloat64()),
			"audio_completion_ratio": fmt.Sprintf("%.2f", audioCompletionRatio.InexactFloat64()),
			"group_ratio":            fmt.Sprintf("%.2f", groupRatio),
		})}
	} else if !usePrice {
		logContent = []*common.Message{common.NewMessage("Model ratio {{model_ratio}}, completion ratio {{completion_ratio}}, audio ratio {{audio_ratio}}, audio completion ratio {{audio_completion_ratio}}, group ratio {{group_ratio}}", map[string]any{
			"model_ratio":            fmt.Sprintf("%.2f", modelRatio),
			"completion_ratio":       fmt.Sprintf("%.2f", completionRatio.InexactFloat64()),
			"audio_ratio":            fmt.Sprintf("%.2f", audioRatio.InexactFloat64()),
			"audio_completion_ratio": fmt.Sprintf("%.2f", audioCompletionRatio.InexactFloat64()),
			"group_ratio":            fmt.Sprintf("%.2f", groupRatio),
		})}
	} else {
		logContent = append(logContent, common.NewMessage("Model price {{model_price}}, group ratio {{group_ratio}}", map[string]any{
			"model_price": fmt.Sprintf("%.2f", modelPrice),
			"group_ratio": fmt.Sprintf("%.2f", groupRatio),
		}))
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 && !fixedPriceBilling {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent = append(logContent, common.NewMessage("Possibly an upstream timeout"))
		logger.LogError(ctx, common.LogText("total tokens is 0, cannot consume quota, userId %d, channelId %d, tokenId %d, model %s， pre-consumed quota %d",
			relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, billingModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}
	projectName, planId, _ := TrackProjectConsumption(ctx, quota)

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, common.LogText("error settling billing: %s", err.Error()))
	}

	logModel := billingModelName
	logContent = append(logContent, extraContent...)
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if relayInfo.PriceData.UseTieredPrice {
		other.SetPublic("use_tiered_price", true)
		other.SetPublic("tiered_input_price", relayInfo.PriceData.TieredInputPrice)
		other.SetPublic("tiered_output_price", relayInfo.PriceData.TieredOutputPrice)
		other.SetPublic("tiered_max_tokens", relayInfo.PriceData.TieredMaxTokens)
		if relayInfo.PriceData.TieredCachedInputPrice > 0 {
			other.SetPublic("tiered_cached_input_price", relayInfo.PriceData.TieredCachedInputPrice)
		}
		if relayInfo.PriceData.TieredCacheWritePrice > 0 {
			other.SetPublic("tiered_cache_write_price", relayInfo.PriceData.TieredCacheWritePrice)
		}
	}
	clientUserId := common.GetContextKeyString(ctx, constant.ContextKeyClientUserId)
	clientScenairo := common.GetContextKeyString(ctx, constant.ContextKeyClientScenairo)
	sessionId := common.GetContextKeyString(ctx, constant.ContextKeyClaudeSessionId)
	requestId := ctx.GetString(common.RequestIdKey)

	var usageStr string
	if usage != nil {
		if usageBytes, err := common.Marshal(usage); err == nil {
			usageStr = string(usageBytes)
		}
	}

	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Request:          requestStr,
		Response:         responseStr,
		ClientUserId:     clientUserId,
		ClientScenairo:   clientScenairo,
		SessionId:        sessionId,
		RequestId:        requestId,
		ProjectName:      projectName,
		PlanId:           planId,
		Usage:            usageStr,
	})
	relayInfo.PerformanceOutputTokens = int64(usage.CompletionTokens)
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	// 原子预扣：检查与扣减在同一操作中完成，并发请求不可能同时通过检查后超扣。
	reserved, err := model.TryReserveTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		remainQuota := 0
		if token, tokenErr := model.GetTokenByKey(relayInfo.TokenKey, false); tokenErr == nil && token != nil {
			remainQuota = token.RemainQuota
		}
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(quota))
	}
	return nil
}

type postConsumeQuotaResult struct {
	FundingApplied bool
	TokenApplied   bool
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	_, err := postConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail)
	return err
}

func postConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (result postConsumeQuotaResult, err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return result, errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return result, err
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota, false)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return result, err
		}
		result.TokenApplied = true
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return result, nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		// UserQuota is the balance before this request; quota plus
		// preConsumedQuota is what the request finally cost.
		balanceAfter := relayInfo.UserQuota - (quota + preConsumedQuota)
		if !quotaReminderDue(userSetting, int64(relayInfo.UserQuota), int64(balanceAfter)) {
			return
		}
		lang := userSetting.Language
		prompt := i18n.Translate(lang, i18n.MsgNotifyQuotaLowTitle)
		content, values := quotaNotifyContent(lang, userSetting.NotifyType, prompt, logger.FormatQuota(balanceAfter))
		err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
		if err != nil {
			common.SysError(common.LogText("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		userSetting := relayInfo.UserSetting
		usedBefore := relayInfo.SubscriptionAmountUsedAfterPreConsume - relayInfo.SubscriptionPreConsumed
		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		if !quotaReminderDue(userSetting, relayInfo.SubscriptionAmountTotal-usedBefore, remaining) {
			return
		}

		lang := userSetting.Language
		prompt := i18n.Translate(lang, i18n.MsgNotifySubscriptionQuotaLowTitle)
		content, values := quotaNotifyContent(lang, userSetting.NotifyType, prompt, logger.FormatQuota(int(remaining)))
		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(common.LogText("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}

// quotaReminderDue reports whether a charge that moved a balance (wallet or
// subscription) from before to after earns a low-balance reminder. The site
// and the user must both have reminders on, and the charge must cross the
// user's threshold, or the site default when the user set none. A balance
// already below the threshold earns nothing more until a top-up lifts it back
// above, so a user gets one reminder per drop.
func quotaReminderDue(userSetting dto.UserSetting, before int64, after int64) bool {
	if !common.QuotaRemindEnabled {
		return false
	}
	if userSetting.QuotaWarningEnabled != nil && !*userSetting.QuotaWarningEnabled {
		return false
	}
	threshold := float64(common.QuotaRemindThreshold)
	if userSetting.QuotaWarningThreshold != 0 {
		threshold = userSetting.QuotaWarningThreshold
	}
	return float64(before) >= threshold && float64(after) < threshold
}

// quotaNotifyContent renders a low-quota notice in the user's saved language.
// Bark and Gotify get plain text; email and webhook get HTML with the top-up
// link. values stay in the notify payload for webhook receivers.
func quotaNotifyContent(lang string, notifyType string, prompt string, remaining string) (string, []any) {
	switch notifyType {
	case dto.NotifyTypeBark:
		// Bark推送使用简短文本，不支持HTML
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowBark, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	case dto.NotifyTypeGotify:
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowGotify, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	default:
		topUpLink := PaymentReturnURL("/wallet")
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowEmail, map[string]any{"Prompt": prompt, "Quota": remaining, "Link": topUpLink}), []any{prompt, remaining, topUpLink, topUpLink}
	}
}
