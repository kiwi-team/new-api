package relay

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

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
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
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
	extraContent := []string{}
	if usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0 {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "", requestStr, responseStr)
	} else {
		common.SetContextKey(c, constant.ContextKeyTTSCount, utf8.RuneCountInString(audioReq.Input))
		postConsumeQuota(c, info, usage.(*dto.Usage), extraContent, requestStr, responseStr)
	}

	return nil
}
