package ali

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
	IsSyncImageModel bool
}

const aliAnthropicMessagesModelsEnv = "ALI_ANTHROPIC_MESSAGES_MODELS"
const defaultAliAnthropicMessagesModels = "qwen,deepseek-v4,glm,minimax-m"

/*
	var syncModels = []string{
		"z-image",
		"qwen-image",
		"wan2.6",
	}
*/
func supportsAliAnthropicMessages(modelName string) bool {
	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	if normalizedModelName == "" {
		return false
	}

	return lo.SomeBy(aliAnthropicMessagesModelPatterns(), func(pattern string) bool {
		return strings.Contains(normalizedModelName, pattern)
	})
}

func aliAnthropicMessagesModelPatterns() []string {
	configuredModels := common.GetEnvOrDefaultString(aliAnthropicMessagesModelsEnv, defaultAliAnthropicMessagesModels)
	return lo.FilterMap(strings.Split(configuredModels, ","), func(item string, _ int) (string, bool) {
		pattern := strings.ToLower(strings.TrimSpace(item))
		return pattern, pattern != ""
	})
}

var syncModels = []string{
	"z-image",
	"qwen-image",
	"wan2.6",
}

func isSyncImageModel(modelName string) bool {
	return model_setting.IsSyncImageModel(modelName)
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if supportsAliAnthropicMessages(info.UpstreamModelName) {
		return req, nil
	}

	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, req)
	if err != nil {
		return nil, err
	}
	oaiReq, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	if info.SupportStreamOptions && info.IsStream {
		oaiReq.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var fullRequestURL string
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if supportsAliAnthropicMessages(info.UpstreamModelName) {
			fullRequestURL = fmt.Sprintf("%s/apps/anthropic/v1/messages", info.ChannelBaseUrl)
		} else {
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/chat/completions", info.ChannelBaseUrl)
		}
	default:
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/embeddings", info.ChannelBaseUrl)
		case constant.RelayModeRerank:
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/rerank/text-rerank/text-rerank", info.ChannelBaseUrl)
		case constant.RelayModeResponses:
			fullRequestURL = fmt.Sprintf("%s/api/v2/apps/protocols/compatible-mode/v1/responses", info.ChannelBaseUrl)
		case constant.RelayModeImagesGenerations:
			if isSyncImageModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
			} else {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text2image/image-synthesis", info.ChannelBaseUrl)
			}
		case constant.RelayModeAudioSpeech:
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
		case constant.RelayModeImagesEdits:
			if isOldWanModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/image2image/image-synthesis", info.ChannelBaseUrl)
			} else if isWanModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/image-generation/generation", info.ChannelBaseUrl)
			} else {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
			}
		case constant.RelayModeCompletions:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/completions", info.ChannelBaseUrl)
		default:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/chat/completions", info.ChannelBaseUrl)
		}
	}

	return fullRequestURL, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	//req.Set("X-DashScope-DataInspection", "{\"input\":\"disable\",\"output\":\"disable\"}")
	if info.IsStream {
		req.Set("X-DashScope-SSE", "enable")
	}
	if c.GetString("plugin") != "" {
		req.Set("X-DashScope-Plugin", c.GetString("plugin"))
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		if isSyncImageModel(info.OriginModelName) {

		} else {
			req.Set("X-DashScope-Async", "enable")
		}
	}
	if info.RelayMode == constant.RelayModeImagesEdits {
		if isWanModel(info.OriginModelName) {
			req.Set("X-DashScope-Async", "enable")
		}
		req.Set("Content-Type", "application/json")
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// docs: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2712216
	// fix: InternalError.Algo.InvalidParameter: The value of the enable_thinking parameter is restricted to True.
	enableThinking, _ := request.GetEnableThinking()
	if strings.Contains(request.Model, "thinking") {
		request.SetEnableThinking(true)
		request.Stream = common.GetPointer(true)
		info.IsStream = true
	}
	if request.THINKING != nil {
		thinking := &dto.AnthropicThinking{}
		err := json.Unmarshal(request.THINKING, thinking)
		if err != nil {
			// 对于阿里云的kimi-模型来说，关闭thinking
			if thinking.Type == "disabled" {
				request.SetEnableThinking(false)
			} else if thinking.Type == "enabled" {
				request.SetEnableThinking(true)
			}
		}
	}
	// fix: ali parameter.enable_thinking must be set to false for non-streaming calls
	// aliyun现在还有其他的类型的模型，比如kimi-k2.5,minimax等,这些模型，就不受这个逻辑限制
	if !info.IsStream && strings.Contains(request.Model, "qwen") {
		request.SetEnableThinking(false)
	}

	qwen3SupportThinkingModels := common.OptionMap["qwen3_support_thinking_models"]
	modelList := strings.Split(qwen3SupportThinkingModels, ",")
	if len(qwen3SupportThinkingModels) == 0 {
		modelList = []string{"qwen3-max-preview"}
	}
	if slices.Contains(modelList, request.Model) {
		request.SetEnableThinking(enableThinking)
	}
	// 这个模型，必须要开启思考模式
	if request.Model == "qwen3-235b-a22b-thinking-2507" {
		request.SetEnableThinking(true)
	}
	//common.PrintJson("req", request)
	switch info.RelayMode {
	default:
		aliReq := requestOpenAI2Ali(*request, info.UpstreamModelName)
		return aliReq, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode == constant.RelayModeImagesGenerations {
		if isSyncImageModel(info.OriginModelName) {
			a.IsSyncImageModel = true
		}
		aliRequest, err := oaiImage2AliImageRequest(info, request, a.IsSyncImageModel)
		if err != nil {
			return nil, fmt.Errorf("convert image request to async ali image request failed: %w", err)
		}
		return aliRequest, nil
	} else if info.RelayMode == constant.RelayModeImagesEdits {
		if isOldWanModel(info.OriginModelName) {
			return oaiFormEdit2WanxImageEdit(c, info, request)
		}
		if isSyncImageModel(info.OriginModelName) {
			if isWanModel(info.OriginModelName) {
				a.IsSyncImageModel = false
			} else {
				a.IsSyncImageModel = true
			}
		}
		// ali image edit https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2976416
		// 如果用户使用表单，则需要解析表单数据
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			aliRequest, err := oaiFormEdit2AliImageEdit(c, info, request)
			if err != nil {
				return nil, fmt.Errorf("convert image edit form request failed: %w", err)
			}
			return aliRequest, nil
		} else {
			aliRequest, err := oaiImage2AliImageRequest(info, request, a.IsSyncImageModel)
			if err != nil {
				return nil, fmt.Errorf("convert image request to async ali image request failed: %w", err)
			}
			return aliRequest, nil
		}
	}
	return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return ConvertRerankRequest(request), nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func getLanguageType(request dto.AudioRequest) string {
	if request.Instructions != "" {
		var instructions TTSInstructions
		err := json.Unmarshal([]byte(request.Instructions), &instructions)
		if err != nil {
			return "Auto"
		}
		return instructions.LanguageType
	}
	return "Auto"
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode == constant.RelayModeAudioSpeech {
		aliRequest := QwenTTSReqeust{
			Model: request.Model,
			Input: QwenTTSInput{
				Text:         request.Input,
				Voice:        request.Voice,
				LanguageType: getLanguageType(request),
			},
		}
		jsonData, err := json.Marshal(aliRequest)
		if err != nil {
			return nil, fmt.Errorf("marshal qwen tts request failed: %w", err)
		}
		return bytes.NewReader(jsonData), nil
	}
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if supportsAliAnthropicMessages(info.UpstreamModelName) {
			adaptor := claude.Adaptor{}
			return adaptor.DoResponse(c, resp, info)
		}

		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		switch info.RelayMode {
		case constant.RelayModeImagesGenerations:
			err, usage = aliImageHandler(a, c, resp, info)
		case constant.RelayModeImagesEdits:
			err, usage = aliImageHandler(a, c, resp, info)
		case constant.RelayModeRerank:
			err, usage = RerankHandler(c, resp, info)
		case constant.RelayModeAudioSpeech:
			err, usage = aliAudioHandler(c, resp, info)
		default:
			adaptor := openai.Adaptor{}
			usage, err = adaptor.DoResponse(c, resp, info)
		}
		return usage, err
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
