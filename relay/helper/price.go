package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}
	if relayInfo == nil {
		return groupRatioInfo
	}
	/*
		// 如果key的维度，针对某个模型设置了分组倍率，就已这个为最优先的分组倍率
		rules, ok := ctx.Get("token_channel_rules")
		if ok {
			tokenChannelRules := rules.(map[string]dto.ChannelRulesItem)
			if tokenGroupRuleItem, exists := (tokenChannelRules)[ctx.GetString("original_model")]; exists {
				// 针对模型设置了分组倍率，就以这个为最优先的分组倍率
				for _, channel := range tokenGroupRuleItem.Channels {
					if channel.Id == relayInfo.ChannelId {
						if ratio, ok1 := channel.GroupRatio[relayInfo.UsingGroup]; ok1 {
							groupRatioInfo.GroupRatio = ratio
							return groupRatioInfo
						}
					}
				}
			}
		}
		//key channel ratio
		ratios, ok1 := ctx.Get("token_channel_ratios")
		if ok1 && ratios != nil {
			ratioMap := ratios.(map[int]float64)
			if ratioMap == nil {
				return groupRatioInfo
			}
			if ratio, exists := ratioMap[relayInfo.ChannelId]; exists {
				groupRatioInfo.GroupRatio = ratio
				return groupRatioInfo
			}
		}

		// channel default ratio
		ratioAny, ok2 := ctx.Get("channel_ratio")
		if ok2 {
			ratio := ratioAny.(*float64)
			if ratio != nil {
				if *ratio > constant.LessIsZero {
					groupRatioInfo.GroupRatio = *ratio
				} else {
					groupRatioInfo.GroupRatio = 0.0
				}
			}
			return groupRatioInfo
		}
	*/

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, fmt.Sprintf("final group: %s", autoGroup))
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	// save original group ratio before user-level discounts
	groupRatioInfo.OriginalGroupRatio = groupRatioInfo.GroupRatio

	// apply user-level group discount
	if relayInfo.UserSetting.GroupDiscount != nil {
		if discount, exists := relayInfo.UserSetting.GroupDiscount[relayInfo.UsingGroup]; exists && discount > 0 {
			groupRatioInfo.UserGroupDiscount = discount
			groupRatioInfo.GroupRatio *= discount
		}
	}

	// apply user-level model extra discount
	if relayInfo.UserSetting.ModelExtraDiscount != nil {
		if modelDiscounts, exists := relayInfo.UserSetting.ModelExtraDiscount[relayInfo.UsingGroup]; exists {
			modelName := ctx.GetString("original_model")
			if modelName == "" && relayInfo != nil {
				modelName = relayInfo.OriginModelName
			}
			if discount, ok := modelDiscounts[modelName]; ok && discount > 0 {
				groupRatioInfo.UserModelExtraDiscount = discount
				groupRatioInfo.GroupRatio *= discount
			}
		}
	}

	// apply settlement discount (结算价格管理里配置的用户模型折扣)
	// 折进 group ratio 后，预扣费/实扣/日志 quota 全链路自动生效并落库。
	{
		modelName := ctx.GetString("original_model")
		if modelName == "" {
			modelName = relayInfo.OriginModelName
		}
		if discount := model.GetUserModelSettlementDiscount(relayInfo.UserId, modelName); discount > 0 && discount != 1 {
			groupRatioInfo.SettlementDiscount = discount
			groupRatioInfo.GroupRatio *= discount
		}
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)

	// 优先检查阶梯价格（tiered price takes priority over ModelPrice and ModelRatio）
	tieredPriceTiers, useTiered := ratio_setting.GetTieredPrice(info.OriginModelName)
	if useTiered && len(tieredPriceTiers) > 0 {
		firstTier := tieredPriceTiers[0]
		preConsumedQuota := int(firstTier.InputPrice / 1_000_000 * float64(promptTokens) * common.QuotaPerUnit * groupRatioInfo.GroupRatio)

		var freeModel bool
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || (firstTier.InputPrice == 0 && firstTier.OutputPrice == 0) {
				preConsumedQuota = 0
				freeModel = true
			}
		}

		// 阶梯价格只覆盖文本 input/output，其他类型（缓存、图片、音频等）仍使用原始倍率
		completionRatio := ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ := ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ := ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m := cacheCreationRatio
		cacheCreationRatio1h := cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ := ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio := ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio := ratio_setting.GetAudioCompletionRatio(info.OriginModelName)

		priceData := types.PriceData{
			FreeModel:            freeModel,
			GroupRatioInfo:       groupRatioInfo,
			UseTieredPrice:       true,
			TieredInputPrice:     firstTier.InputPrice,
			TieredOutputPrice:    firstTier.OutputPrice,
			TieredMaxTokens:      firstTier.MaxTokens,
			QuotaToPreConsume:    preConsumedQuota,
			CompletionRatio:      completionRatio,
			CacheRatio:           cacheRatio,
			CacheCreationRatio:   cacheCreationRatio,
			CacheCreation5mRatio: cacheCreationRatio5m,
			CacheCreation1hRatio: cacheCreationRatio1h,
			ImageRatio:           imageRatio,
			AudioRatio:           audioRatio,
			AudioCompletionRatio: audioCompletionRatio,
		}

		if common.DebugEnabled {
			println(fmt.Sprintf("model_price_helper result (tiered): %s", priceData.ToSetting()))
		}
		info.PriceData = priceData
		return priceData, nil
	}

	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, fmt.Errorf("模型 %s 倍率或价格未配置，请联系管理员设置或开始自用模式；Model %s ratio or price not set, please set or start self-use mode", matchName, matchName)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) types.PerCallPriceData {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	// 如果没有配置价格，则使用默认价格
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if !ok {
			modelPrice = 0.1
		} else {
			modelPrice = defaultPrice
		}
	}
	quota := int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	priceData := types.PerCallPriceData{
		ModelPrice:     modelPrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData
}

func ContainPriceOrRatio(modelName string) bool {
	_, ok := ratio_setting.GetModelPrice(modelName, false)
	if ok {
		return true
	}
	_, ok, _ = ratio_setting.GetModelRatio(modelName)
	if ok {
		return true
	}
	_, ok = ratio_setting.GetTieredPrice(modelName)
	if ok {
		return true
	}
	return false
}
