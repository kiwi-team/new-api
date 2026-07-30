package ali_dashscope

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

func isWan27Model(model string) bool {
	return strings.HasPrefix(model, "wan2.7")
}

// convertWan27ImageRequest 将 OpenAI 格式的图片请求转换为 wan2.7 的 messages 格式
func convertWan27ImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*Wan27ImageRequest, error) {
	wan27Req := &Wan27ImageRequest{
		Model: request.Model,
		Parameters: Wan27ImageParameters{
			N:         int(lo.FromPtr(request.N)),
			Size:      strings.ReplaceAll(request.Size, "x", "*"),
			Watermark: request.Watermark,
		},
	}

	// 从 extra 中提取扩展参数
	if request.Extra != nil {
		if val, ok := request.Extra["seed"]; ok {
			var seed int
			if err := common.Unmarshal(val, &seed); err == nil {
				wan27Req.Parameters.Seed = seed
			}
		}
		if val, ok := request.Extra["thinking_mode"]; ok {
			var thinkingMode bool
			if err := common.Unmarshal(val, &thinkingMode); err == nil {
				wan27Req.Parameters.ThinkingMode = &thinkingMode
			}
		}
		if val, ok := request.Extra["enable_sequential"]; ok {
			var enableSequential bool
			if err := common.Unmarshal(val, &enableSequential); err == nil {
				wan27Req.Parameters.EnableSequential = &enableSequential
			}
		}
		if val, ok := request.Extra["bbox_list"]; ok {
			wan27Req.Parameters.BboxList = val
		}
		if val, ok := request.Extra["color_palette"]; ok {
			wan27Req.Parameters.ColorPalette = val
		}
	}

	// 构建 messages content
	var contentParts []Wan27Content

	// 图片编辑模式：先添加图片
	if info.RelayMode == constant.RelayModeImagesEdits {
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			// 表单模式：从 multipart form 中提取图片转 base64
			imageBase64s, err := getImageBase64sFromForm(c)
			if err != nil {
				return nil, fmt.Errorf("get image base64s from form failed: %w", err)
			}
			for _, img := range imageBase64s {
				contentParts = append(contentParts, Wan27Content{Image: img})
			}
		} else {
			// JSON 模式：从 request 中提取图片 URL
			imageURLs, err := request.GetImageURLs()
			if err != nil {
				return nil, fmt.Errorf("get image urls failed: %w", err)
			}
			for _, url := range imageURLs {
				contentParts = append(contentParts, Wan27Content{Image: url})
			}
		}
	}

	// 添加文本 prompt
	contentParts = append(contentParts, Wan27Content{Text: request.Prompt})

	wan27Req.Input = Wan27ImageInput{
		Messages: []Wan27Message{
			{
				Role:    "user",
				Content: contentParts,
			},
		},
	}

	return wan27Req, nil
}

func getImageBase64sFromForm(c *gin.Context) ([]string, error) {
	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("failed to parse multipart form: %w", err)
		}
		mf = c.Request.MultipartForm
	}

	var imageFiles []*multipart.FileHeader

	// 支持 image, image[], image[0] 等字段名
	if files, ok := mf.File["image"]; ok && len(files) > 0 {
		imageFiles = append(imageFiles, files...)
	}
	if files, ok := mf.File["image[]"]; ok && len(files) > 0 {
		imageFiles = append(imageFiles, files...)
	}
	for fieldName, files := range mf.File {
		if strings.HasPrefix(fieldName, "image[") && !strings.HasPrefix(fieldName, "image[]") && len(files) > 0 {
			imageFiles = append(imageFiles, files...)
		}
	}

	if len(imageFiles) == 0 {
		return nil, fmt.Errorf("image is required")
	}

	var imageBase64s []string
	for _, file := range imageFiles {
		f, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open image file: %w", err)
		}
		imageData, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read image file: %w", err)
		}
		mimeType := http.DetectContentType(imageData)
		base64Data := base64.StdEncoding.EncodeToString(imageData)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
		imageBase64s = append(imageBase64s, dataURL)
	}
	return imageBase64s, nil
}

// wan27ImageHandler 处理 wan2.7 同步图片响应，将 choices 格式转换为 OpenAI ImageResponse
func wan27ImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
	responseFormat := c.GetString("response_format")

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	service.CloseResponseBodyGracefully(resp)

	var wan27Resp Wan27ImageResponse
	err = common.Unmarshal(responseBody, &wan27Resp)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}

	if wan27Resp.Code != "" {
		logger.LogError(c, "wan27 image failed: "+wan27Resp.Message)
		return types.NewError(errors.New(wan27Resp.Message), types.ErrorCodeBadResponse), nil
	}

	logger.LogDebug(c, "wan27 image result: "+string(responseBody))

	// 将 choices 格式转换为 OpenAI ImageResponse
	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(),
	}

	for _, choice := range wan27Resp.Output.Choices {
		var data dto.ImageData
		for _, content := range choice.Message.Content {
			if content.Image != "" {
				if strings.HasPrefix(content.Image, "http") {
					data.Url = content.Image
					if responseFormat == "b64_json" {
						_, b64, err := service.GetImageFromUrl(content.Image)
						if err != nil {
							logger.LogError(c, "wan27 get_image_data_failed: "+err.Error())
							continue
						}
						data.B64Json = b64
					}
				} else {
					data.B64Json = content.Image
				}
			} else if content.Text != "" {
				data.RevisedPrompt = content.Text
			}
		}
		imageResponse.Data = append(imageResponse.Data, data)
	}

	imageResponse.Metadata = responseBody

	// 根据实际生成的图片数量修正计费
	if wan27Resp.Usage.ImageCount > 0 {
		info.PriceData.AddOtherRatio("n", float64(wan27Resp.Usage.ImageCount))
	} else if len(imageResponse.Data) > 0 {
		info.PriceData.AddOtherRatio("n", float64(len(imageResponse.Data)))
	}

	jsonResponse, err := common.Marshal(imageResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return nil, &dto.Usage{}
}
