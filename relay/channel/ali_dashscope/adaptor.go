package ali_dashscope

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// https://help.aliyun.com/zh/dashscope/developer-reference/api-details

type Adaptor struct {
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// func (a *Adaptor) Init(meta *meta.Meta) {
// 	a.meta = meta
// }

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return req, nil
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	fullRequestURL := ""
	switch info.RelayMode {
	case constant.RelayModeEmbeddings:
		fullRequestURL = fmt.Sprintf("%s/api/v1/services/embeddings/text-embedding/text-embedding", info.ChannelBaseUrl)
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		if isWan27Model(info.UpstreamModelName) {
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
		} else {
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text2image/image-synthesis", info.ChannelBaseUrl)
		}
	default:
		fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text-generation/generation", info.ChannelBaseUrl)
	}

	return fullRequestURL, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if info.IsStream || IsDeepResearchModel(info.UpstreamModelName) {
		req.Set("Accept", "text/event-stream")
		req.Set("X-DashScope-SSE", "enable")
	}
	req.Set("Authorization", "Bearer "+info.ApiKey)

	if c.GetString("plugin") != "" {
		req.Set("X-DashScope-Plugin", c.GetString("plugin"))
	}
	if info.RelayMode == constant.RelayModeImagesGenerations || info.RelayMode == constant.RelayModeImagesEdits {
		if isWan27Model(info.UpstreamModelName) {
			// wan2.7 使用同步接口
			req.Set("Content-Type", "application/json")
		} else {
			req.Set("X-DashScope-Async", "enable")
			req.Set("Content-Type", "application/json")
		}
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	case constant.RelayModeEmbeddings:
		aliEmbeddingRequest := ConvertEmbeddingRequest(*request)
		return aliEmbeddingRequest, nil
	default:
		if IsDeepResearchModel(info.UpstreamModelName) {
			aliRequest := ConvertDeepResearchRequest(*request)
			return aliRequest, nil
		}
		aliRequest := ConvertRequest(*request)
		return aliRequest, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if isWan27Model(info.UpstreamModelName) {
		return convertWan27ImageRequest(c, info, request)
	}
	imageRequest := ConvertImageRequest(request)
	return imageRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

/*
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}
*/

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if IsDeepResearchModel(info.UpstreamModelName) {
		err, usage = DeepResearchStreamHandler(c, info, resp)
		return
	}
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		err, usage = wan27ImageHandler(c, info, resp)
		return
	default:
		if info.IsStream {
			err, usage = StreamHandler(c, info, resp)
		} else {
			err, usage = Handler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "ali_dashscope"
}
