package moonshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"

	//"github.com/QuantumNous/new-api/types"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	if specialPlan, ok := channelconstant.ChannelSpecialBases[baseURL]; ok {
		if info.RelayFormat == types.RelayFormatClaude {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		if info.RelayFormat == types.RelayFormatOpenAI {
			return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
		}
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		if info.RelayMode == constant.RelayModeRerank {
			return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeEmbeddings {
			return fmt.Sprintf("%s/v1/embeddings", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeChatCompletions {
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeCompletions {
			return fmt.Sprintf("%s/v1/completions", info.ChannelBaseUrl), nil
		}
		return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	/*
		streamModel := []string{
			"kimi-k2-thinking",
			"kimi-k2-0905",
			"kimi-k2-0905-preview",
		}
		if slices.Contains(streamModel, request.Model) && !info.IsStream {
			return nil, fmt.Errorf("this moonshot model %s only support stream mode now", request.Model)
		}
	*/
	//https://platform.moonshot.cn/docs/api/chat#%E5%AD%97%E6%AE%B5%E8%AF%B4%E6%98%8E
	// https://platform.moonshot.cn/docs/guide/kimi-k2-5-quickstart#%E5%8F%82%E6%95%B0%E5%8F%98%E5%8A%A8%E8%AF%B4%E6%98%8E
	if request.Model == "kimi-k2.5" {
		request.TopP = common.GetPointer(0.95)
		if request.THINKING != nil {
			var thinking dto.Thinking
			err := common.Unmarshal(request.THINKING, &thinking)
			if err == nil {
				if thinking.Type == "enabled" {
					tmp := 1.0
					request.Temperature = &tmp
					request.THINKING = json.RawMessage(`{"type":"enabled"}`)
				} else if thinking.Type == "disabled" {
					tmp := 0.6
					request.Temperature = &tmp
					request.THINKING = json.RawMessage(`{"type":"disabled"}`)
				}
			} else {
				request.THINKING = json.RawMessage(`{"type":"disabled"}`)
			}
		} else {
			tmp := 1.0
			request.Temperature = &tmp
		}
	}

	// Moonshot(Kimi) 系列模型默认只接受 base64 形式的多模态数据
	// （如 data:video/mp4;base64,xxx），不支持直接传入 http(s) URL，
	// 因此在转发前把消息里的远程图片/视频 URL 下载并转换为 base64 data URL。
	if err := convertMediaURLToBase64(c, request); err != nil {
		return nil, err
	}
	if request.Temperature != nil && isTemperatureOneOnlyModel(getUpstreamModelName(info, request.Model)) && *request.Temperature != 1.0 {
		request.Temperature = common.GetPointer[float64](1.0)
	}
	return request, nil
}

// convertMediaURLToBase64 遍历消息内容，把远程的图片/视频 URL 下载并替换为 base64 data URL。
// 已经是 base64 data URL 的内容会被原样保留。
func convertMediaURLToBase64(c *gin.Context, request *dto.GeneralOpenAIRequest) error {
	for i := range request.Messages {
		msg := &request.Messages[i]
		content := msg.ParseContent()
		if len(content) == 0 {
			continue
		}
		changed := false
		for j := range content {
			item := &content[j]
			switch item.Type {
			case dto.ContentTypeImageURL:
				img := item.GetImageMedia()
				if img == nil || !isRemoteURL(img.Url) {
					continue
				}
				dataURL, err := fetchAsDataURL(c, img.Url)
				if err != nil {
					return fmt.Errorf("convert image url to base64 failed: %w", err)
				}
				img.Url = dataURL
				item.ImageUrl = img
				changed = true
			case dto.ContentTypeVideoUrl:
				video := item.GetVideoUrl()
				if video == nil || !isRemoteURL(video.Url) {
					continue
				}
				dataURL, err := fetchAsDataURL(c, video.Url)
				if err != nil {
					return fmt.Errorf("convert video url to base64 failed: %w", err)
				}
				video.Url = dataURL
				item.VideoUrl = video
				changed = true
			}
		}
		if changed {
			msg.SetMediaContent(content)
		}
	}
	return nil
}

func isRemoteURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// fetchAsDataURL 下载文件并返回 data:{mime};base64,{data} 形式的字符串。
func fetchAsDataURL(c *gin.Context, url string) (string, error) {
	fileData, err := service.GetFileBase64FromUrl(c, url, "moonshot media base64 conversion")
	if err != nil {
		return "", err
	}
	mimeType := fileData.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, fileData.Base64Data), nil
}
func getUpstreamModelName(info *relaycommon.RelayInfo, fallback string) string {
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	return fallback
}

func isTemperatureOneOnlyModel(model string) bool {
	return strings.EqualFold(model, "kimi-k2.6")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
