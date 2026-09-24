package relay

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/typesafe"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// TypeSafeHelper relays the provider-native System One protocol while sharing
// the ordinary routing, reservation, settlement, retry, and consume-log path.
func TypeSafeHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)

	nativeRequest, ok := info.Request.(*dto.TypeSafeRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.TypeSafeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	request, err := common.DeepCopy(nativeRequest)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy System One request: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	if info.ChannelType != constant.ChannelTypeTypeSafe && info.ChannelType != constant.ChannelTypeOpenRouter {
		return types.NewError(errors.New("the /v1/systemone endpoint requires a TypeSafe or OpenRouter channel"), types.ErrorCodeInvalidApiType)
	}
	adaptor := &typesafe.Adaptor{}
	adaptor.Init(info)

	jsonData, err := common.Marshal(request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}
	logger.LogDebug(c, "System One request body: %s", jsonData)

	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	requestStr := ""
	if saveRequestResponse {
		requestStr = string(jsonData)
	}
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
		return types.NewError(errors.New("System One upstream returned an invalid HTTP response"), types.ErrorCodeBadResponse)
	}
	statusCodeMapping := c.GetString("status_code_mapping")
	if httpResp.StatusCode != http.StatusOK {
		apiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(apiErr, statusCodeMapping)
		return apiErr
	}

	usage, responseBody, apiErr := adaptor.DoNativeResponse(c, httpResp)
	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusCodeMapping)
		return apiErr
	}
	responseStr := ""
	if saveRequestResponse {
		responseStr = string(responseBody)
	}
	info.FinalRequestRelayFormat = types.RelayFormatTypeSafe
	service.PostTextConsumeQuota(c, info, usage, nil, requestStr, responseStr)
	return nil
}
