package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	"github.com/QuantumNous/new-api/relay/channel/typesafe"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func DecisionsHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)
	switch info.ChannelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeOpenRouter, constant.ChannelTypeNewAPI:
	default:
		return types.NewError(errors.New("channel does not support /v1/decisions"), types.ErrorCodeInvalidApiType)
	}
	native, ok := info.Request.(*dto.DecisionsRequest)
	if !ok {
		return types.NewError(errors.New("expected DecisionsRequest"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	request, err := common.DeepCopy(native)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	jsonData, err := common.Marshal(request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
		// Parse into a fresh value: deleted fields must not survive overrides.
		request = &dto.DecisionsRequest{}
		if err := common.Unmarshal(jsonData, request); err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	if err := helper.ValidateDecisionsRequest(request); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	adaptor := GetAdaptor(info.ApiType)
	info.FinalRequestRelayFormat = types.RelayFormatOpenAIDecisions
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		converted, err := openrouter.ConvertDecisionsRequest(c, request)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		jsonData, err = common.Marshal(converted)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}
		adaptor = &typesafe.Adaptor{}
		info.FinalRequestRelayFormat = types.RelayFormatTypeSafe
		info.AppendRequestConversion(types.RelayFormatTypeSafe)
	}
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType)
	}
	adaptor.Init(info)
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	response, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := response.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(errors.New("invalid Decisions HTTP response"), types.ErrorCodeBadResponse)
	}
	defer service.CloseResponseBodyGracefully(httpResp)
	statusMapping := c.GetString("status_code_mapping")
	if httpResp.StatusCode != http.StatusOK {
		apiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(apiErr, statusMapping)
		return apiErr
	}
	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var result *dto.DecisionsResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		result, err = openrouter.ConvertDecisionsResponse(responseBody, request, info.OriginModelName)
	} else {
		err = common.Unmarshal(responseBody, &result)
	}
	if err == nil && (result == nil || len(result.Answers) != len(request.Questions)) {
		err = errors.New("invalid Decisions response answers")
	}
	var usage *dto.Usage
	if err == nil {
		usage, err = decisionsBillingUsage(result.Usage)
	}
	if err != nil {
		apiErr := types.NewError(err, types.ErrorCodeBadResponseBody)
		service.ResetStatusCode(apiErr, statusMapping)
		return apiErr
	}
	clientBody := responseBody
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		clientBody, err = common.Marshal(result)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed)
		}
	}
	info.SetFirstResponseTime()
	c.Data(http.StatusOK, "application/json", clientBody)
	requestLog, responseLog := "", ""
	if os.Getenv("SAVE_REQUEST_RESPONSE") == "true" {
		requestLog, responseLog = string(jsonData), string(responseBody)
	}
	service.PostTextConsumeQuota(c, info, usage, nil, requestLog, responseLog)
	return nil
}

// decisionsBillingUsage rejects missing, negative, overflowing or inconsistent
// upstream counters before billing or writing a successful client response.
func decisionsBillingUsage(usage *dto.DecisionUsage) (*dto.Usage, error) {
	if usage == nil || usage.InputTokens == nil || usage.OutputTokens == nil {
		return nil, errors.New("Decisions response is missing usage token counts")
	}
	input, output := *usage.InputTokens, *usage.OutputTokens
	read, write := usage.InputTokensDetails.CachedTokens, usage.InputTokensDetails.CacheWriteTokens
	reasoning := usage.OutputTokensDetails.ReasoningTokens
	if input < 0 || output < 0 || input > common.MaxQuota || output > common.MaxQuota-input ||
		read < 0 || read > input || write < 0 || write > common.MaxQuota-read || reasoning < 0 || reasoning > output {
		return nil, errors.New("Decisions response contains invalid usage token counts")
	}
	usage.TotalTokens = input + output
	return &dto.Usage{
		PromptTokens: input, CompletionTokens: output, TotalTokens: input + output,
		InputTokens: input, OutputTokens: output, UsageSource: dto.BillingUsageSourceOAIDecisions,
		UsageSemantic:          dto.BillingUsageSemanticOpenAI,
		PromptTokensDetails:    dto.InputTokenDetails{CachedTokens: read, CacheWriteTokens: write},
		CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokens: reasoning},
	}, nil
}
