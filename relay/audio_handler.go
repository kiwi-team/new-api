package relay

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/ali_dashscope"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func AudioHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	audioReq, ok := info.Request.(*dto.AudioRequest)
	if !ok {
		return types.NewError(errors.New("invalid request type"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	requestStr := ""
	responseStr := ""

	request, err := common.DeepCopy(audioReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to AudioRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if saveRequestResponse {
		requestStr = common.JsonStringify(request)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	qwenASR := ali_dashscope.IsQwenAudioASRModel(request.Model)
	if qwenASR {
		if info.RelayMode != relayconstant.RelayModeAudioTranscription {
			return types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgRelayAudioTranscriptionOnly)), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if _, err := ali_dashscope.PrepareQwenAudioASRRequest(c); err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		// A retry may select a channel whose mapping first reveals this model.
		// Re-estimate from the frozen expression before sending that attempt.
		if constant.CountToken && info.GetEstimatePromptTokens() != ali_dashscope.QwenAudioASRInputEstimate {
			if err := reserveQwenASRInput(c, info, request); err != nil {
				return err
			}
		}
		if saveRequestResponse {
			requestStr = c.GetString(ali_dashscope.QwenASRRequestLogKey)
		}
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	ioReader, err := adaptor.ConvertAudioRequest(c, info, *request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	resp, err := adaptor.DoRequest(c, info, ioReader)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}
	if saveRequestResponse {
		responseStr = fmt.Sprintf(`{"audio_url": "%s"}`, common.GetContextKeyString(c, constant.ContextKeyAudioUrl))
		if qwenASR {
			responseStr = common.GetContextKeyString(c, constant.ContextKeySdkResponseStr)
		}
	}

	// https://platform.minimaxi.com/docs/guides/pricing#%E8%AF%AD%E9%9F%B3%E8%B5%84%E6%BA%90%E5%8C%85
	// https://help.aliyun.com/zh/model-studio/qwen-tts?spm=a2c4g.11186623.0.0.26e12b19qDqr2k
	// https://console.volcengine.com/speech/service/10035/buy-word?AppID=1921851494&ActiveID=volc.seedtts.default
	// tts 按照字符数计费
	/*
		common.SetContextKey(c, constant.ContextKeyTTSCount, utf8.RuneCountInString(audioReq.Input))
		postConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	*/
	// todo fix tts
	var extraContent []*common.Message
	if usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0 {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), nil, requestStr, responseStr)
	} else {
		common.SetContextKey(c, constant.ContextKeyTTSCount, utf8.RuneCountInString(audioReq.Input))
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), extraContent, requestStr, responseStr)
	}

	return nil
}

func reserveQwenASRInput(c *gin.Context, info *relaycommon.RelayInfo, request *dto.AudioRequest) *types.NewAPIError {
	const estimate = ali_dashscope.QwenAudioASRInputEstimate
	if snap := info.TieredBillingSnapshot; snap != nil {
		input := billingexpr.RequestInput{}
		if info.BillingRequestInput != nil {
			input = *info.BillingRequestInput
		}
		raw, trace, err := billingexpr.RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, billingexpr.TokenParams{P: estimate, Len: estimate}, input)
		if err != nil {
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
		multiplier := snap.PreConsumeMultiplier
		if multiplier <= 0 || trace.BillingUnit == billingexpr.BillingUnitRequest {
			multiplier = 1
		}
		beforeGroup := raw / 1_000_000 * snap.QuotaPerUnit * multiplier
		quota, err := billingexpr.QuotaRoundStrict(beforeGroup * snap.GroupRatio)
		if err != nil {
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
		}
		snap.EstimatedPromptTokens, snap.EstimatedCompletionTokens = estimate, 0
		snap.EstimatedQuotaBeforeGroup, snap.EstimatedQuotaAfterGroup = beforeGroup, quota
		snap.EstimatedTier, snap.EstimatedBillingUnit, snap.EstimatedFixedPrice = trace.MatchedTier, trace.BillingUnit, trace.FixedPrice
		info.PriceData.QuotaToPreConsume = quota
		info.SetEstimatePromptTokens(estimate)
		return service.PrepareTieredBillingForSelectedGroup(c, info)
	}
	// Preserve an administrator's explicit legacy pricing override as well.
	price, err := helper.ModelPriceHelper(c, info, estimate, request.GetTokenCountMeta())
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}
	info.SetEstimatePromptTokens(estimate)
	if price.FreeModel {
		return nil
	}
	if info.Billing == nil {
		return service.PreConsumeBilling(c, price.QuotaToPreConsume, info)
	}
	if err := info.Billing.Reserve(price.QuotaToPreConsume); err != nil {
		return types.NewError(err, types.ErrorCodeInsufficientUserQuota, types.ErrOptionWithSkipRetry())
	}
	info.FinalPreConsumedQuota = info.Billing.GetPreConsumedQuota()
	return nil
}
