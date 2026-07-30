package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
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
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, billingexpr.TokenParams{
		P:   float64(usage.InputTokens),
		C:   float64(usage.OutputTokens),
		Len: float64(usage.InputTokens),
	})
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
	var logContent string
	if relayInfo.PriceData.UseTieredPrice {
		logContent = fmt.Sprintf("阶梯价格（≤%d tokens）：输入 %.6f / 输出 %.6f /1M tokens，音频倍率 %.2f，音频补全倍率 %.2f，视频倍率 %.2f，分组倍率 %.2f",
			relayInfo.PriceData.TieredMaxTokens,
			quotaInfo.TieredInputPrice, quotaInfo.TieredOutputPrice,
			audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), videoRatio.InexactFloat64(), groupRatio)
	} else if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，视频倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), videoRatio.InexactFloat64(), groupRatio)
		//if tieredOk {
		//	quota = tieredQuota
		//}
		//totalTokens := usage.TotalTokens
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// Track project consumption and get project name for logging
	projectName, planId, _ := TrackProjectConsumption(ctx, quota)
	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), videoRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if relayInfo.PriceData.UseTieredPrice {
		other["use_tiered_price"] = true
		other["tiered_input_price"] = relayInfo.PriceData.TieredInputPrice
		other["tiered_output_price"] = relayInfo.PriceData.TieredOutputPrice
		other["tiered_max_tokens"] = relayInfo.PriceData.TieredMaxTokens
		if relayInfo.PriceData.TieredCachedInputPrice > 0 {
			other["tiered_cached_input_price"] = relayInfo.PriceData.TieredCachedInputPrice
		}
		if relayInfo.PriceData.TieredCacheWritePrice > 0 {
			other["tiered_cache_write_price"] = relayInfo.PriceData.TieredCacheWritePrice
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

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string, requestStr string, responseStr string) {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens
	imageInputTokens := usage.PromptTokensDetails.ImageTokens
	videoInputTokens := usage.PromptTokensDetails.VideoTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

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
		ModelName:      relayInfo.OriginModelName,
		UsePrice:       usePrice,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		UseTieredPrice: relayInfo.PriceData.UseTieredPrice,
	}
	if relayInfo.PriceData.UseTieredPrice {
		// 阶梯价格：根据实际 inputTokens 重新匹配档位
		tieredPriceTiers, useTiered := ratio_setting.GetTieredPrice(relayInfo.OriginModelName)
		if useTiered && len(tieredPriceTiers) > 0 {
			tier := ratio_setting.MatchPriceTier(tieredPriceTiers, textInputTokens+audioInputTokens+imageInputTokens+videoInputTokens)
			quotaInfo.TieredInputPrice = tier.InputPrice
			quotaInfo.TieredOutputPrice = tier.OutputPrice
			// 更新 PriceData 中的档位信息，确保日志展示正确的匹配档位
			relayInfo.PriceData.TieredInputPrice = tier.InputPrice
			relayInfo.PriceData.TieredOutputPrice = tier.OutputPrice
			relayInfo.PriceData.TieredCachedInputPrice = tier.CachedInputPrice
			relayInfo.PriceData.TieredCacheWritePrice = tier.CacheWritePrice
			relayInfo.PriceData.TieredMaxTokens = tier.MaxTokens
		}
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if relayInfo.PriceData.UseTieredPrice {
		logContent = fmt.Sprintf("阶梯价格（≤%d tokens）：输入 %.6f / 输出 %.6f /1M tokens，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			relayInfo.PriceData.TieredMaxTokens,
			quotaInfo.TieredInputPrice, quotaInfo.TieredOutputPrice,
			audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	// Track project consumption and get project name for logging
	projectName, planId, _ := TrackProjectConsumption(ctx, quota)

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if relayInfo.PriceData.UseTieredPrice {
		other["use_tiered_price"] = true
		other["tiered_input_price"] = relayInfo.PriceData.TieredInputPrice
		other["tiered_output_price"] = relayInfo.PriceData.TieredOutputPrice
		other["tiered_max_tokens"] = relayInfo.PriceData.TieredMaxTokens
		if relayInfo.PriceData.TieredCachedInputPrice > 0 {
			other["tiered_cached_input_price"] = relayInfo.PriceData.TieredCachedInputPrice
		}
		if relayInfo.PriceData.TieredCacheWritePrice > 0 {
			other["tiered_cache_write_price"] = relayInfo.PriceData.TieredCacheWritePrice
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
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(usage.CompletionTokens))
	})
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	//if relayInfo.TokenUnlimited {
	//	return nil
	//}
	token, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err != nil {
		return err
	}
	if !relayInfo.TokenUnlimited && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}
	err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
	if err != nil {
		return err
	}
	return nil
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return err
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
			return err
		}
	}

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return err
		}
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := PaymentReturnURL("/wallet")

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
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
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		if remaining >= int64(threshold) {
			return
		}

		prompt := "您的订阅额度即将用尽"
		topUpLink := PaymentReturnURL("/wallet")

		var content string
		var values []interface{}
		notifyType := userSetting.NotifyType
		if notifyType == "" {
			notifyType = dto.NotifyTypeEmail
		}

		if notifyType == dto.NotifyTypeBark {
			content = "{{value}}，剩余额度：{{value}}，请及时充值"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else if notifyType == dto.NotifyTypeGotify {
			content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else {
			content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining)), topUpLink, topUpLink}
		}

		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(fmt.Sprintf("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}
