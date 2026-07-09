package reve

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("reve adaptor: relay info is nil")
	}
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeReve]
	}
	path := createImagePath
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		path = editImagePath
	}
	return relaycommon.GetFullRequestURL(baseURL, path, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("reve adaptor: relay info is nil")
	}
	if info.ApiKey == "" {
		return errors.New("reve adaptor: api key is required")
	}
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}
	return nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil {
		return nil, errors.New("reve adaptor: relay info is nil")
	}

	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		if v := c.PostForm("prompt"); strings.TrimSpace(v) != "" {
			prompt = strings.TrimSpace(v)
		}
	}
	if prompt == "" {
		return nil, errors.New("reve adaptor: prompt is required")
	}

	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if modelName == "" {
		modelName = ModelList[0]
	}
	info.UpstreamModelName = modelName

	// 透传请求体 extra 中的可选参数
	aspectRatio := extraString(request.ExtraFields, "aspect_ratio")
	version := extraString(request.ExtraFields, "version")

	// 文生图
	if info.RelayMode != relayconstant.RelayModeImagesEdits {
		return CreateRequest{
			Prompt:      prompt,
			AspectRatio: aspectRatio,
			Version:     version,
		}, nil
	}

	// 图生图：image 既可以是 url，也可以是 base64 字符串
	image, err := resolveEditImage(c, &request)
	if err != nil {
		return nil, err
	}
	if image == "" {
		return nil, errors.New("reve adaptor: image is required for edits")
	}
	return EditRequest{
		EditInstruction: prompt,
		ReferenceImage:  image,
		AspectRatio:     aspectRatio,
		Version:         version,
	}, nil
}

// extraString 从请求体的额外参数 map 中取出字符串值，不存在或非字符串时返回空串。
func extraString(extra json.RawMessage, key string) string {
	// 1. 先把 RawMessage 解析成 map[string]json.RawMessage
	var extraMap map[string]json.RawMessage
	err := common.Unmarshal(extra, &extraMap)
	if err != nil {
		// 整体extra不是对象json，直接返回空
		return ""
	}
	raw, ok := extraMap[key]
	if !ok || len(raw) == 0 {
		return ""
	}
	var s string
	if err := common.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// resolveEditImage 解析图生图输入图片，并统一转换为 base64 字符串
// （Reve 图生图接口只接受 base64 图片）。来源优先级：
// 1. multipart 表单中上传的图片文件（读取后 base64 编码）
// 2. 请求体 image / images 字段：
//   - http(s) url：下载图片并转成 base64
//   - base64 / data-uri 字符串：去掉 data-uri 前缀后原样使用
func resolveEditImage(c *gin.Context, request *dto.ImageRequest) (string, error) {
	if c.Request != nil {
		mf := c.Request.MultipartForm
		if mf == nil {
			if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
				if _, err := c.MultipartForm(); err == nil {
					mf = c.Request.MultipartForm
				}
			}
		}
		if mf != nil && mf.File != nil {
			for _, key := range []string{"image", "image[]"} {
				if files := mf.File[key]; len(files) > 0 {
					file, err := files[0].Open()
					if err != nil {
						return "", fmt.Errorf("reve adaptor: failed to open image file: %w", err)
					}
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil {
						return "", fmt.Errorf("reve adaptor: failed to read image file: %w", err)
					}
					return base64.StdEncoding.EncodeToString(data), nil
				}
			}
		}
	}

	urls, _ := request.GetImageURLs()
	for _, u := range urls {
		trimmed := strings.TrimSpace(u)
		if trimmed == "" {
			continue
		}
		// http(s) 链接：下载图片并转为 base64
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			_, data, err := service.GetImageFromUrl(trimmed)
			if err != nil {
				return "", fmt.Errorf("reve adaptor: failed to download image from %s: %w", trimmed, err)
			}
			return data, nil
		}
		// data-uri：去掉 data:...;base64, 前缀，只保留 base64 主体
		if strings.HasPrefix(trimmed, "data:") {
			if idx := strings.Index(trimmed, "base64,"); idx != -1 {
				return trimmed[idx+len("base64,"):], nil
			}
		}
		// 其余情况视为已经是 base64 字符串
		return trimmed, nil
	}
	return "", nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	common.PrintJson("reve", requestBody)
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewError(errors.New("reve adaptor: empty response"), types.ErrorCodeBadResponse)
	}

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewError(readErr, types.ErrorCodeReadResponseBodyFailed)
	}
	_ = resp.Body.Close()

	var reveResp Response
	if unmarshalErr := common.Unmarshal(responseBody, &reveResp); unmarshalErr != nil {
		return nil, types.NewError(fmt.Errorf("reve adaptor: failed to decode response: %w", unmarshalErr), types.ErrorCodeBadResponseBody)
	}

	if strings.TrimSpace(reveResp.Image) == "" {
		return nil, types.NewError(errors.New("reve adaptor: empty image in response"), types.ErrorCodeBadResponseBody)
	}

	imageResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data: []dto.ImageData{
			{B64Json: reveResp.Image},
		},
	}

	responseBytes, marshalErr := common.Marshal(imageResponse)
	if marshalErr != nil {
		return nil, types.NewError(fmt.Errorf("reve adaptor: encode response failed: %w", marshalErr), types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(responseBytes)

	return &dto.Usage{}, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertOpenAIRequest is not implemented")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertRerankRequest is not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertEmbeddingRequest is not implemented")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("reve adaptor: ConvertAudioRequest is not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertOpenAIResponsesRequest is not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertClaudeRequest is not implemented")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("reve adaptor: ConvertGeminiRequest is not implemented")
}
