package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting"
	"one-api/setting/model_setting"
	"one-api/types"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func getAndValidateResponsesRequest(c *gin.Context) (*dto.OpenAIResponsesRequest, error) {
	request := &dto.OpenAIResponsesRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	if len(request.Input) == 0 {
		return nil, errors.New("input is required")
	}
	return request, nil

}

func checkInputSensitive(textRequest *dto.OpenAIResponsesRequest, info *relaycommon.RelayInfo) ([]string, error) {
	sensitiveWords, err := service.CheckSensitiveInput(textRequest.Input)
	return sensitiveWords, err
}

func getInputTokens(req *dto.OpenAIResponsesRequest, info *relaycommon.RelayInfo) int {
	inputTokens := service.CountTokenInput(req.Input, req.Model)
	info.PromptTokens = inputTokens
	return inputTokens
}

// func ResponsesHelper(c *gin.Context) (openaiErr *dto.OpenAIErrorWithStatusCode) {
func ResponsesHelper(c *gin.Context) (newAPIError *types.NewAPIError) {
	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"
	req, err := getAndValidateResponsesRequest(c)
	if err != nil {
		common.LogError(c, fmt.Sprintf("getAndValidateResponsesRequest error: %s", err.Error()))
		return types.NewError(err, types.ErrorCodeInvalidRequest)
	}

	requestStr := ""
	if saveRequestResponse {
		// 序列化 textRequest 为 JSON 字符串
		requestBytes, marshalErr := json.Marshal(req)
		if marshalErr != nil {
			common.LogError(c, fmt.Sprintf("marshal textRequest failed: %s", marshalErr.Error()))
		} else {
			requestStr = string(requestBytes)
		}
	}
	relayInfo := relaycommon.GenRelayInfoResponses(c, req)

	if setting.ShouldCheckPromptSensitive() {
		sensitiveWords, err := checkInputSensitive(req, relayInfo)
		if err != nil {
			common.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(sensitiveWords, ", ")))
			return types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
		}
	}

	err = helper.ModelMappedHelper(c, relayInfo, req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError)
	}

	if value, exists := c.Get("prompt_tokens"); exists {
		promptTokens := value.(int)
		relayInfo.SetPromptTokens(promptTokens)
	} else {
		promptTokens := getInputTokens(req, relayInfo)
		c.Set("prompt_tokens", promptTokens)
	}

	priceData, err := helper.ModelPriceHelper(c, relayInfo, relayInfo.PromptTokens, int(req.MaxOutputTokens))
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError)
	}
	// pre consume quota
	preConsumedQuota, userQuota, newAPIError := preConsumeQuota(c, priceData.ShouldPreConsumedQuota, relayInfo)
	if newAPIError != nil {
		return newAPIError
	}
	defer func() {
		if newAPIError != nil {
			returnPreConsumedQuota(c, relayInfo, userQuota, preConsumedQuota)
		}
	}()
	adaptor := GetAdaptor(relayInfo.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", relayInfo.ApiType), types.ErrorCodeInvalidApiType)
	}
	adaptor.Init(relayInfo)
	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed)
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, relayInfo, *req)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		jsonData, err := json.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		// apply param override
		if len(relayInfo.ParamOverride) > 0 {
			reqMap := make(map[string]interface{})
			err = json.Unmarshal(jsonData, &reqMap)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
			}
			for key, value := range relayInfo.ParamOverride {
				reqMap[key] = value
			}
			jsonData, err = json.Marshal(reqMap)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed)
			}
		}

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, relayInfo, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)

		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	// 创建流式响应记录器（如果需要记录响应）
	responseStr := ""
	var streamRecorder *helper.StreamResponseRecorder
	if saveRequestResponse {
		// 使用流式记录器包装原始响应体，不影响实时传输
		streamRecorder = helper.NewStreamResponseRecorder(httpResp.Body)
		httpResp.Body = streamRecorder
	}

	//usage, openaiErr := adaptor.DoResponse(c, httpResp, relayInfo)
	//if openaiErr != nil {
	usage, newAPIError := adaptor.DoResponse(c, httpResp, relayInfo)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// 在流式传输完成后，从记录器中获取完整的响应数据
	if saveRequestResponse && streamRecorder != nil {
		responseStr = streamRecorder.GetRecordedString()
	}

	if strings.HasPrefix(relayInfo.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, relayInfo, usage.(*dto.Usage), preConsumedQuota, userQuota, priceData, "")
	} else {
		postConsumeQuota(c, relayInfo, usage.(*dto.Usage), preConsumedQuota, userQuota, priceData, "", requestStr, responseStr)
	}
	return nil
}
