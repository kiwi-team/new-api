package visualvolcengine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case constant.RelayModeImagesEdits:
		var requestBody bytes.Buffer
		var body service.SubmitTaskRequest
		writer := multipart.NewWriter(&requestBody)

		//writer.WriteField("model", request.Model)
		if request.Model == "SeedEditV3.0" {
			body.ReqKey = "seededit_v3.0"
		}
		// 获取所有表单字段
		formData := c.Request.PostForm
		// 遍历表单字段并打印输出
		for key, values := range formData {
			if key == "model" {
				continue
			}
			if key == "prompt" {
				body.Prompt = values[0]
			} else if key == "req_key" {
				body.ReqKey = values[0]
			} else if key == "scale" {
				if s, err := strconv.ParseFloat(values[0], 32); err == nil {
					body.Scale = float32(s)
				}
			} else if key == "seed" {
				if s, err := strconv.Atoi(values[0]); err == nil {
					body.Seed = s
				}
			}
		}

		// Parse the multipart form to handle both single image and multiple images
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory
			return nil, errors.New("failed to parse multipart form")
		}

		if c.Request.MultipartForm != nil && c.Request.MultipartForm.File != nil {
			// Check if "image" field exists in any form, including array notation
			var imageFiles []*multipart.FileHeader
			var exists bool

			// First check for standard "image" field
			if imageFiles, exists = c.Request.MultipartForm.File["image"]; !exists || len(imageFiles) == 0 {
				// If not found, check for "image[]" field
				if imageFiles, exists = c.Request.MultipartForm.File["image[]"]; !exists || len(imageFiles) == 0 {
					// If still not found, iterate through all fields to find any that start with "image["
					foundArrayImages := false
					for fieldName, files := range c.Request.MultipartForm.File {
						if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
							foundArrayImages = true
							for _, file := range files {
								imageFiles = append(imageFiles, file)
							}
						}
					}

					// If no image fields found at all
					if !foundArrayImages && (len(imageFiles) == 0) {
						return nil, errors.New("image is required")
					}
				}
			}

			// Process all image files
			for i, fileHeader := range imageFiles {
				file, err := fileHeader.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open image file %d: %w", i, err)
				}
				defer file.Close()

				// If multiple images, use image[] as the field name
				// fieldName := "image"
				// if len(imageFiles) > 1 {
				// 	fieldName = "image[]"
				// }

				// Determine MIME type based on file extension

				// Create a form file with the appropriate content type
				// h := make(textproto.MIMEHeader)
				// h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
				// h.Set("Content-Type", mimeType)

				// part, err := writer.CreatePart(h)
				// if err != nil {
				// 	return nil, fmt.Errorf("create form part failed for image %d: %w", i, err)
				// }

				//if _, err := io.Copy(part, file); err != nil {
				// get base64 for file
				buf := new(bytes.Buffer)
				_, err = buf.ReadFrom(file)
				if err != nil {
					return nil, fmt.Errorf("read file failed for image %d: %w", i, err)
				}
				base64 := base64.StdEncoding.EncodeToString(buf.Bytes())
				body.BinaryDataBase64 = append(body.BinaryDataBase64, base64)
				//return nil, fmt.Errorf("copy file failed for image %d: %w", i, err)
				//}
			}

		} else {
			return nil, errors.New("no multipart form data found")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", "application/json")
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body failed: %w", err)
		}
		return bytes.NewReader(bodyBytes), nil

	default:
		return request, nil
	}
}

// detectImageMimeType determines the MIME type based on the file extension
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// Try to detect from extension if possible
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		// Default to png as a fallback
		return "image/png"
	}
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.RelayMode {
	case constant.RelayModeImagesEdits:
		return fmt.Sprintf("%s/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", info.ChannelMeta.ChannelBaseUrl), nil
	default:
	}
	return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	//return channel.DoApiRequest(a, c, info, requestBody)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(requestBody); err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	apiKey := strings.Split(info.ApiKey, "|")
	key := apiKey[0]
	secret := apiKey[1]

	var req service.SubmitTaskRequest
	if err := json.Unmarshal(buf.Bytes(), &req); err != nil { // Unmarshal from the buffer
		return nil, fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	response, err := service.SubmitTask(&req, key, secret)
	if err != nil {
		common.SysLog("submit task failed:" + err.Error())
		return nil, err
	}
	return response, nil

}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		//err, usage = openai.OpenaiHandlerWithUsage(c, resp, info)
		apiKey := strings.Split(info.ApiKey, "|")
		key := apiKey[0]
		secret := apiKey[1]
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
		}
		var respData service.SubmitTaskResponse
		err = json.Unmarshal(body, &respData)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			//return nil, &types.NewAPIError{ StatusCode: http.StatusInternalServerError, ErrorType:  "unmarshal_response_body_failed", }
		}
		taskId := respData.Data.TaskID
		i := 0
		max := 50
		for {
			result, err := service.GetTaskResult(&service.GetTaskResultRequest{
				TaskID:  taskId,
				ReqJson: "{\"return_url\":true}",
			}, key, secret)
			if err != nil {
				//return nil, &types.NewAPIError{ StatusCode: http.StatusInternalServerError, ErrorType:  "get_task_result_failed", }
				return nil, types.NewError(err, types.ErrorCodeGetTaskResultFailed)
			}
			switch result.Status {
			case "in_queue", "generating":
				common.SysLog(fmt.Sprintf("task status: %s", result.Status))
			case "done":
				// convert to openai format response
				openAIResponse := dto.ImageResponse{
					Created: common.GetTimestamp(),
					Data:    make([]dto.ImageData, 0, 1),
				}
				openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{
					Url: result.ImageUrls[0],
				})
				jsonResponse, jsonErr := json.Marshal(openAIResponse)
				if jsonErr != nil {
					return nil, types.NewError(jsonErr, types.ErrorCodeBadResponseBody)
					//return nil, &types.NewAPIError{ Err:        jsonErr, StatusCode: http.StatusInternalServerError, ErrorType:  "marshal_response_failed", }

				}
				c.Writer.Header().Set("Content-Type", "application/json")
				c.Writer.WriteHeader(resp.StatusCode)
				_, _ = c.Writer.Write(jsonResponse)
				return &dto.Usage{
					TotalTokens: 0,
				}, nil
			default:
				//return nil, &types.NewAPIError{ StatusCode: http.StatusInternalServerError, ErrorType:  "get_task_result_failed", }
				return nil, types.NewError(err, types.ErrorCodeGetTaskResultFailed)
			}
			time.Sleep(time.Second * 5)
			if i > max {
				//return nil, &types.NewAPIError{ StatusCode: http.StatusInternalServerError, ErrorType:  "get_task_result_failed:timeout", }
				return nil, types.NewError(err, types.ErrorCodeGetTaskResultTimeout)

			}
			i++
		}
	default:
		//return nil, &types.NewAPIError{ StatusCode: http.StatusInternalServerError, ErrorType:  "not_suported", }
		return nil, types.NewError(errors.New("not supported"), types.ErrorCodeNotSupported)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
