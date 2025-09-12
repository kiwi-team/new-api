package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	"one-api/relay/channel/openrouter"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting"
	"one-api/setting/model_setting"
	"one-api/setting/operation_setting"
	"one-api/types"
	"os"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

func getAndValidateTextRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	textRequest := &dto.GeneralOpenAIRequest{}
	err := common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}
	if relayInfo.RelayMode == relayconstant.RelayModeModerations && textRequest.Model == "" {
		textRequest.Model = "text-moderation-latest"
	}
	if relayInfo.RelayMode == relayconstant.RelayModeEmbeddings && textRequest.Model == "" {
		textRequest.Model = c.Param("model")
	}

	if textRequest.MaxTokens > math.MaxInt32/2 {
		return nil, errors.New("max_tokens is invalid")
	}
	if textRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if textRequest.WebSearchOptions != nil {
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			validSizes := map[string]bool{
				"high":   true,
				"medium": true,
				"low":    true,
			}
			if !validSizes[textRequest.WebSearchOptions.SearchContextSize] {
				return nil, errors.New("invalid search_context_size, must be one of: high, medium, low")
			}
		} else {
			textRequest.WebSearchOptions.SearchContextSize = "medium"
		}
	}
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeCompletions:
		if textRequest.Prompt == "" {
			return nil, errors.New("field prompt is required")
		}
	case relayconstant.RelayModeChatCompletions:
		if len(textRequest.Messages) == 0 {
			return nil, errors.New("field messages is required")
		}
	case relayconstant.RelayModeEmbeddings:
	case relayconstant.RelayModeModerations:
		if textRequest.Input == nil || textRequest.Input == "" {
			return nil, errors.New("field input is required")
		}
	case relayconstant.RelayModeEdits:
		if textRequest.Instruction == "" {
			return nil, errors.New("field instruction is required")
		}
	}
	filterParmas(textRequest)
	transParmas(textRequest, relayInfo)
	// 判断字符串是否以"o3"开头
	if strings.HasPrefix(textRequest.Model, "o3") ||
		strings.HasPrefix(textRequest.Model, "o1") {
		// 处理以o3开头的模型
		textRequest.TopK = 0
	}
	// 处理不同渠道的参数，很多渠道用openai的格式来提供claude的服务，但是开启thinking的方法不一样

	relayInfo.IsStream = textRequest.Stream
	return textRequest, nil
}

func transParmas(textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) error {
	isOpenRouter := info.ChannelType == common.ChannelTypeOpenRouter || strings.Contains(info.BaseUrl, "openrouter")
	isChat := strings.Contains(info.BaseUrl, "chataiapi")
	isNuwa := strings.Contains(info.BaseUrl, "nuwaapi")
	isYunwu := strings.Contains(info.BaseUrl, "yunwu")
	isSiliconflow := strings.Contains(info.BaseUrl, "siliconflow")
	isVolcengine := strings.Contains(info.BaseUrl, "volces")
	// openrouter 用的是openai的格式，但是claude的模型需要开启thinking
	var thinking dto.AnthropicThinking
	//var extraBody dto.ExtraBody
	if textRequest.THINKING != nil {
		err := json.Unmarshal(textRequest.THINKING, &thinking)
		if err != nil {
			return err
		}
	}
	if isOpenRouter && textRequest.THINKING != nil {
		if textRequest.THINKING != nil && thinking.Type == "enabled" {
			if thinking.BudgetTokens > 0 {
				reasoning := openrouter.RequestReasoning{
					MaxTokens: thinking.BudgetTokens,
				}
				reasoningJSON, err := json.Marshal(reasoning)
				if err != nil {
					return fmt.Errorf("failed to marshal reasoning: %w", err)
				}
				textRequest.Reasoning = reasoningJSON
			} else {
				reasoning := openrouter.RequestReasoning{
					Effort: "medium",
				}
				if textRequest.ReasoningEffort != "" {
					reasoning.Effort = textRequest.ReasoningEffort
				}
				reasoningJSON, err := json.Marshal(reasoning)
				if err != nil {
					return fmt.Errorf("failed to marshal reasoning: %w", err)
				}
				textRequest.Reasoning = reasoningJSON
			}
		}
	} else if isChat && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" || strings.Contains(textRequest.Model, "claude") {
				textRequest.Model = textRequest.Model + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
		}
	} else if isNuwa && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" || strings.Contains(textRequest.Model, "claude") {
				textRequest.Model = textRequest.Model + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
			if thinking.BudgetTokens > 0 {
				// extraBody.Google.ThinkingConfig.IncludeThoughts = true
				// extraBody.Google.ThinkingConfig.ThinkingBudget = thinking.BudgetTokens
				// extraBodyJSON, err := json.Marshal(struct {
				// 	ExtraBody dto.ExtraBody `json:"extra_body"`
				// }{ExtraBody: extraBody})
				// if err != nil {
				// 	return fmt.Errorf("failed to marshal reasoning: %w", err)
				// }
				// textRequest.ExtraBody = extraBodyJSON
				//OpenAI API 提供三种思考控制级别："low"、"medium" 和 "high"，分别对应于 1,024、8,192 和 24,576 个令牌
				if thinking.BudgetTokens >= 24576 {
					textRequest.ReasoningEffort = "high"
				} else if thinking.BudgetTokens >= 8000 {
					textRequest.ReasoningEffort = "medium"
				} else {
					textRequest.ReasoningEffort = "low"
				}

			} else {
				if textRequest.ReasoningEffort == "" {
					textRequest.ReasoningEffort = "medium"
				}
			}
		} else {
			//if strings.Contains(strings.ToLower(textRequest.Model), "gemini") {
			if textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = textRequest.Model + "-nothinking"
				info.UpstreamModelName = textRequest.Model
			}
		}
	} else if isYunwu && textRequest.THINKING != nil {
		if thinking.Type == "enabled" {
			//if !strings.Contains(strings.ToLower(textRequest.Model), "doubao") {
			if textRequest.Model == "gemini-2.5-pro" || textRequest.Model == "gemini-2.5-flash" || strings.Contains(textRequest.Model, "claude") {
				textRequest.Model = textRequest.Model + "-thinking"
				info.UpstreamModelName = textRequest.Model
			}
		} else {
			if textRequest.Model == "gemini-2.5-flash" {
				textRequest.Model = textRequest.Model + "-nothinking"
				info.UpstreamModelName = textRequest.Model
			}
		}
	}

	if textRequest.Model == "deepseek-reasoner" {
		if isSiliconflow {
			textRequest.EnableThinking = true
		} else if isVolcengine {
			textRequest.THINKING = json.RawMessage(`{"type": "enabled"}`)
		}
	}

	return nil
}

func filterParmas(textRequest *dto.GeneralOpenAIRequest) {
	filterConfig := common.OptionMap["ModelParamsFilter"]
	if filterConfig == "" {
		return
	}
	filterConfigMap := &model.ModelParamsFilterMap{}
	err := json.Unmarshal([]byte(filterConfig), filterConfigMap)
	if err != nil {
		common.SysError("filterParmas Unmarshal failed: " + err.Error())
		return
	}
	if filterConfigMap.SetTopKZero != nil {
		for _, model := range *filterConfigMap.SetTopKZero {
			if textRequest.Model == model || common.RegMatch(model, textRequest.Model) {
				textRequest.TopK = 0
			}
		}
	}
	if filterConfigMap.SetTopPZero != nil {
		for _, model := range *filterConfigMap.SetTopPZero {
			if textRequest.Model == model || common.RegMatch(model, textRequest.Model) {
				textRequest.TopP = 0
			}
		}
	}
	if filterConfigMap.SetMaxTokensZero != nil {
		for _, model := range *filterConfigMap.SetMaxTokensZero {
			if textRequest.Model == model || common.RegMatch(model, textRequest.Model) {
				textRequest.MaxTokens = 0
			}
		}
	}
	if filterConfigMap.SetTemperatureZero != nil {
		for _, model := range *filterConfigMap.SetTemperatureZero {
			if textRequest.Model == model || common.RegMatch(model, textRequest.Model) {
				textRequest.Temperature = nil
			}
		}
	}
	if filterConfigMap.SetTemperatureOne != nil {
		for _, model := range *filterConfigMap.SetTemperatureOne {
			if textRequest.Model == model || common.RegMatch(model, textRequest.Model) {
				temp := 1.0
				textRequest.Temperature = &temp
			}
		}
	}
	if filterConfigMap.SetMaxTokens != nil {
		for _, item := range *filterConfigMap.SetMaxTokens {
			// get key from item
			for key, val := range item {
				if key == textRequest.Model || common.RegMatch(key, textRequest.Model) {
					if int(textRequest.MaxTokens) > val && val > 0 {
						textRequest.MaxTokens = uint(val)
					}
				}
			}
		}
	}
	if filterConfigMap.SetStopNil != nil {
		for _, item := range *filterConfigMap.SetStopNil {
			// get key from item
			if item == textRequest.Model || common.RegMatch(item, textRequest.Model) {
				textRequest.Stop = nil
			}
		}
	}
}

// func TextHelper(c *gin.Context) (openaiErr *dto.OpenAIErrorWithStatusCode) {
func TextHelper(c *gin.Context) (newAPIError *types.NewAPIError) {

	relayInfo := relaycommon.GenRelayInfo(c)

	// get & validate textRequest 获取并验证文本请求
	textRequest, err := getAndValidateTextRequest(c, relayInfo)

	if err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest)
	}
	saveRequestResponse := os.Getenv("SAVE_REQUEST_RESPONSE") == "true"

	requestStr := ""
	responseStr := ""
	// 序列化 textRequest 为 JSON 字符串
	if saveRequestResponse {
		requestBytes, marshalErr := json.Marshal(textRequest)
		if marshalErr != nil {
			common.LogError(c, fmt.Sprintf("marshal textRequest failed: %s", marshalErr.Error()))
		} else {
			requestStr = string(requestBytes)
		}
	}

	if textRequest.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", textRequest.WebSearchOptions.SearchContextSize)
	}

	if setting.ShouldCheckPromptSensitive() {
		words, err := checkRequestSensitive(textRequest, relayInfo)
		if err != nil {
			common.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			return types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
		}
	}

	err = helper.ModelMappedHelper(c, relayInfo, textRequest)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError)
	}

	// 获取 promptTokens，如果上下文中已经存在，则直接使用
	var promptTokens int
	if value, exists := c.Get("prompt_tokens"); exists {
		promptTokens = value.(int)
		relayInfo.PromptTokens = promptTokens
	} else {
		promptTokens, err = getPromptTokens(textRequest, relayInfo)
		// count messages token error 计算promptTokens错误
		if err != nil {
			return types.NewError(err, types.ErrorCodeCountTokenFailed)
		}
		c.Set("prompt_tokens", promptTokens)
	}

	priceData, err := helper.ModelPriceHelper(c, relayInfo, promptTokens, int(math.Max(float64(textRequest.MaxTokens), float64(textRequest.MaxCompletionTokens))))
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError)
	}

	// pre-consume quota 预消耗配额
	preConsumedQuota, userQuota, newApiErr := preConsumeQuota(c, priceData.ShouldPreConsumedQuota, relayInfo)
	if newApiErr != nil {
		return newApiErr
	}
	defer func() {
		if newApiErr != nil {
			returnPreConsumedQuota(c, relayInfo, userQuota, preConsumedQuota)
		}
	}()
	includeUsage := false
	// 判断用户是否需要返回使用情况
	if textRequest.StreamOptions != nil && textRequest.StreamOptions.IncludeUsage {
		includeUsage = true
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !relayInfo.SupportStreamOptions || !textRequest.Stream {
		textRequest.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			textRequest.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	if includeUsage {
		relayInfo.ShouldIncludeUsage = true
	}

	adaptor := GetAdaptor(relayInfo.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", relayInfo.ApiType), types.ErrorCodeInvalidApiType)
	}
	adaptor.Init(relayInfo)
	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest)
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, relayInfo, textRequest)
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
			_ = common.Unmarshal(jsonData, &reqMap)
			for key, value := range relayInfo.ParamOverride {
				reqMap[key] = value
			}
			jsonData, err = common.Marshal(reqMap)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
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
		relayInfo.IsStream = relayInfo.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr = service.RelayErrorHandler(httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}

	// 创建流式响应记录器（如果需要记录响应）
	var streamRecorder *helper.StreamResponseRecorder
	if saveRequestResponse {
		// 使用流式记录器包装原始响应体，不影响实时传输
		streamRecorder = helper.NewStreamResponseRecorder(httpResp.Body)
		httpResp.Body = streamRecorder
	}

	//usage, openaiErr := adaptor.DoResponse(c, httpResp, relayInfo)
	//if openaiErr != nil {
	usage, newApiErr := adaptor.DoResponse(c, httpResp, relayInfo)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
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

func getPromptTokens(textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) (int, error) {
	var promptTokens int
	var err error
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions:
		promptTokens, err = service.CountTokenChatRequest(info, *textRequest)
	case relayconstant.RelayModeCompletions:
		promptTokens = service.CountTokenInput(textRequest.Prompt, textRequest.Model)
	case relayconstant.RelayModeModerations:
		promptTokens = service.CountTokenInput(textRequest.Input, textRequest.Model)
	case relayconstant.RelayModeEmbeddings:
		promptTokens = service.CountTokenInput(textRequest.Input, textRequest.Model)
	default:
		err = errors.New("unknown relay mode")
		promptTokens = 0
	}
	info.PromptTokens = promptTokens
	return promptTokens, err
}

func checkRequestSensitive(textRequest *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) ([]string, error) {
	var err error
	var words []string
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions:
		words, err = service.CheckSensitiveMessages(textRequest.Messages)
	case relayconstant.RelayModeCompletions:
		words, err = service.CheckSensitiveInput(textRequest.Prompt)
	case relayconstant.RelayModeModerations:
		words, err = service.CheckSensitiveInput(textRequest.Input)
	case relayconstant.RelayModeEmbeddings:
		words, err = service.CheckSensitiveInput(textRequest.Input)
	}
	return words, err
}

// 预扣费并返回用户剩余配额
func preConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) (int, int, *types.NewAPIError) {
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return 0, 0, types.NewError(err, types.ErrorCodeQueryDataError)
	}
	if userQuota <= 0 {
		return 0, 0, types.NewErrorWithStatusCode(errors.New("user quota is not enough"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	}
	if userQuota-preConsumedQuota < 0 {
		return 0, 0, types.NewErrorWithStatusCode(fmt.Errorf("pre-consume quota failed, user quota: %s, need quota: %s", common.FormatQuota(userQuota), common.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	}
	relayInfo.UserQuota = userQuota
	if userQuota > 100*preConsumedQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > 100*preConsumedQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				common.LogInfo(c, fmt.Sprintf("user %d quota %s and token %d quota %d are enough, trusted and no need to pre-consume", relayInfo.UserId, common.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			common.LogInfo(c, fmt.Sprintf("user %d with unlimited token has enough quota %s, trusted and no need to pre-consume", relayInfo.UserId, common.FormatQuota(userQuota)))
		}
	}

	if preConsumedQuota > 0 {
		err := service.PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return 0, 0, types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return 0, 0, types.NewError(err, types.ErrorCodeUpdateDataError)
		}
	}
	return preConsumedQuota, userQuota, nil
}

func returnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo, userQuota int, preConsumedQuota int) {
	if preConsumedQuota != 0 {
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := service.PostConsumeQuota(&relayInfoCopy, -preConsumedQuota, 0, false)
			if err != nil {
				common.SysError("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo,
	usage *dto.Usage, preConsumedQuota int, userQuota int, priceData helper.PriceData, extraContent string, requestStr string, responseStr string) {
	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.PromptTokens,
			CompletionTokens: 0,
			TotalTokens:      relayInfo.PromptTokens,
		}
		extraContent += "（可能是请求出错）"
	}
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	cacheTokens := usage.PromptTokensDetails.CachedTokens
	imageTokens := usage.PromptTokensDetails.ImageTokens
	audioTokens := usage.PromptTokensDetails.AudioTokens
	completionTokens := usage.CompletionTokens
	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := priceData.CompletionRatio
	cacheRatio := priceData.CacheRatio
	imageRatio := priceData.ImageRatio
	modelRatio := priceData.ModelRatio
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	modelPrice := priceData.ModelPrice

	// Convert values to decimal for precise calculation
	dPromptTokens := decimal.NewFromInt(int64(promptTokens))
	dCacheTokens := decimal.NewFromInt(int64(cacheTokens))
	dImageTokens := decimal.NewFromInt(int64(imageTokens))
	dAudioTokens := decimal.NewFromInt(int64(audioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(completionTokens))
	dCompletionRatio := decimal.NewFromFloat(completionRatio)
	dCacheRatio := decimal.NewFromFloat(cacheRatio)
	dImageRatio := decimal.NewFromFloat(imageRatio)
	dModelRatio := decimal.NewFromFloat(modelRatio)
	dGroupRatio := decimal.NewFromFloat(groupRatio)
	dModelPrice := decimal.NewFromFloat(modelPrice)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)

	// openai web search 工具计费
	var dWebSearchQuota decimal.Decimal
	var webSearchPrice float64
	// response api 格式工具计费
	if relayInfo.ResponsesUsageInfo != nil {
		if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool.CallCount > 0 {
			// 计算 web search 调用的配额 (配额 = 价格 * 调用次数 / 1000 * 分组倍率)
			webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, webSearchTool.SearchContextSize)
			dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
				webSearchTool.CallCount, webSearchTool.SearchContextSize, dWebSearchQuota.String())
		}
	} else if strings.HasSuffix(modelName, "search-preview") {
		// search-preview 模型不支持 response api
		searchContextSize := ctx.GetString("chat_completion_web_search_context_size")
		if searchContextSize == "" {
			searchContextSize = "medium"
		}
		webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
		dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Web Search 调用 1 次，上下文大小 %s，调用花费 %s",
			searchContextSize, dWebSearchQuota.String())
	}
	// claude web search tool 计费
	var dClaudeWebSearchQuota decimal.Decimal
	var claudeWebSearchPrice float64
	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
		dClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		extraContent += fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			claudeWebSearchCallCount, dClaudeWebSearchQuota.String())
	}
	// file search tool 计费
	var dFileSearchQuota decimal.Decimal
	var fileSearchPrice float64
	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			dFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
				fileSearchTool.CallCount, dFileSearchQuota.String())
		}
	}

	var quotaCalculateDecimal decimal.Decimal

	var audioInputQuota decimal.Decimal
	var audioInputPrice float64
	if !priceData.UsePrice {
		baseTokens := dPromptTokens
		// 减去 cached tokens
		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			baseTokens = baseTokens.Sub(dCacheTokens)
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}

		// 减去 image tokens
		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			baseTokens = baseTokens.Sub(dImageTokens)
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		// 减去 Gemini audio tokens
		if !dAudioTokens.IsZero() {
			audioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(modelName)
			if audioInputPrice > 0 {
				// 重新计算 base tokens
				baseTokens = baseTokens.Sub(dAudioTokens)
				audioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
				extraContent += fmt.Sprintf("Audio Input 花费 %s", audioInputQuota.String())
			}
		}
		promptQuota := baseTokens.Add(cachedTokensWithRatio).Add(imageTokensWithRatio)

		completionQuota := dCompletionTokens.Mul(dCompletionRatio)

		quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(ratio)

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
	} else {
		quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
	}
	// 添加 responses tools call 调用的配额
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dWebSearchQuota)
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dFileSearchQuota)
	// 添加 audio input 独立计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)

	quota := int(quotaCalculateDecimal.Round(0).IntPart())
	totalTokens := promptTokens + completionTokens

	var logContent string
	if !priceData.UsePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，分组倍率 %.2f", modelRatio, completionRatio, groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		common.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, preConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	quotaDelta := quota - preConsumedQuota
	if quotaDelta != 0 {
		err := service.PostConsumeQuota(relayInfo, quotaDelta, preConsumedQuota, true)
		if err != nil {
			common.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	logModel := modelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := service.GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if imageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = imageRatio
		other["image_output"] = imageTokens
	}
	if !dWebSearchQuota.IsZero() {
		if relayInfo.ResponsesUsageInfo != nil {
			if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists {
				other["web_search"] = true
				other["web_search_call_count"] = webSearchTool.CallCount
				other["web_search_price"] = webSearchPrice
			}
		} else if strings.HasSuffix(modelName, "search-preview") {
			other["web_search"] = true
			other["web_search_call_count"] = 1
			other["web_search_price"] = webSearchPrice
		}
	} else if !dClaudeWebSearchQuota.IsZero() {
		other["web_search"] = true
		other["web_search_call_count"] = claudeWebSearchCallCount
		other["web_search_price"] = claudeWebSearchPrice
	}
	if !dFileSearchQuota.IsZero() && relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists {
			other["file_search"] = true
			other["file_search_call_count"] = fileSearchTool.CallCount
			other["file_search_price"] = fileSearchPrice
		}
	}
	if !audioInputQuota.IsZero() {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = audioTokens
		other["audio_input_price"] = audioInputPrice
	}
	/*
		model.RecordConsumeLog(ctx, relayInfo.UserId, relayInfo.ChannelId, promptTokens, completionTokens, logModel,
			tokenName, quota, logContent, relayInfo.TokenId, userQuota, int(useTimeSeconds), relayInfo.IsStream, relayInfo.Group, other, requestStr, responseStr)
	*/
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UserQuota:        userQuota,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Request:          requestStr,
		Response:         responseStr,
	})
}
