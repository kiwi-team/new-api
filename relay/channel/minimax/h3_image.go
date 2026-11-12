package minimax

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// H3 uses /v1/images, not the legacy image-01 /v1/image_generation API.
// https://platform.minimax.cn/docs/api-reference/image-generation-h3-generations.md
// https://platform.minimax.cn/docs/api-reference/image-generation-h3-edits.md
func isH3ImageModel(model string) bool {
	return model == "MiniMax-H3-Image" || model == "MiniMax-H3-Image-Highspeed"
}

type h3ImageRequest struct {
	Model                 string          `json:"model"`
	Prompt                string          `json:"prompt"`
	N                     *uint           `json:"n,omitempty"`
	Size                  string          `json:"size,omitempty"`
	ResponseFormat        string          `json:"response_format,omitempty"`
	Stream                *bool           `json:"stream,omitempty"`
	Images                json.RawMessage `json:"images,omitempty"`
	OutputFormat          json.RawMessage `json:"output_format,omitempty"`
	OutputCompression     *int            `json:"output_compression,omitempty"`
	AspectRatio           json.RawMessage `json:"aspect_ratio,omitempty"`
	Area                  json.RawMessage `json:"area,omitempty"`
	DisablePromptEnhanced *bool           `json:"disable_prompt_enhanced,omitempty"`
	AigcWatermark         *bool           `json:"aigc_watermark,omitempty"`
}

func convertH3ImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode != constant.RelayModeImagesGenerations && info.RelayMode != constant.RelayModeImagesEdits {
		return nil, fmt.Errorf("unsupported H3 image relay mode: %d", info.RelayMode)
	}
	if request.N != nil && (*request.N < 1 || *request.N > 10) {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("H3 image n must be an integer between 1 and 10"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info.RelayMode == constant.RelayModeImagesEdits && strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		form := c.Request.MultipartForm
		if form == nil {
			var err error
			form, err = common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, err
			}
			c.Request.MultipartForm = form
		}
		// Validate the actual form too: ingress normalizes n=0 to one, but
		// the shared multipart converter forwards the original form fields.
		if values, ok := form.Value["n"]; ok {
			if len(values) != 1 {
				return nil, types.NewErrorWithStatusCode(fmt.Errorf("H3 image n must occur once"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			n, err := strconv.ParseUint(values[0], 10, 64)
			if err != nil || n < 1 || n > 10 {
				return nil, types.NewErrorWithStatusCode(fmt.Errorf("H3 image n must be an integer between 1 and 10"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}
		return (&openai.Adaptor{}).ConvertImageRequest(c, info, request)
	}

	converted := h3ImageRequest{
		Model: request.Model, Prompt: request.Prompt, N: request.N,
		Size: request.Size, ResponseFormat: request.ResponseFormat, Stream: request.Stream,
		OutputFormat: request.OutputFormat, AigcWatermark: request.Watermark,
		AspectRatio: request.Extra["aspect_ratio"], Area: request.Extra["area"],
	}
	for _, field := range []struct {
		name   string
		raw    json.RawMessage
		target any
	}{
		{"output_compression", request.OutputCompression, &converted.OutputCompression},
		{"disable_prompt_enhanced", request.Extra["disable_prompt_enhanced"], &converted.DisablePromptEnhanced},
		{"aigc_watermark", request.Extra["aigc_watermark"], &converted.AigcWatermark},
	} {
		if len(field.raw) == 0 {
			continue
		}
		if err := common.Unmarshal(field.raw, field.target); err != nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid H3 image %s: %w", field.name, err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	if info.RelayMode == constant.RelayModeImagesEdits {
		// H3 edits always enhance the prompt; this option is generation-only.
		converted.DisablePromptEnhanced = nil
		converted.Images = request.Images
		// Accept the gateway's string/string-array aliases as well as the
		// native [{"image_url": "..."}] form, including data: and mm_file: URLs.
		var urls []string
		if len(request.Images) == 0 || common.Unmarshal(request.Images, &urls) == nil {
			urls, err := request.GetImageURLs()
			if err != nil {
				return nil, err
			}
			images := make([]map[string]string, 0, len(urls))
			for _, imageURL := range urls {
				images = append(images, map[string]string{"image_url": imageURL})
			}
			converted.Images, _ = common.Marshal(images)
		}
	}
	return converted, nil
}

func h3ImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		if info.IsStream {
			return openai.OpenaiImageStreamHandler(c, info, resp)
		}
		return openai.OpenaiImageHandler(c, info, resp)
	}

	usage := &dto.Usage{}
	succeeded := make(map[int]struct{})
	receivedUsage := false
	var streamErr *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var event struct {
			Type       string             `json:"type"`
			ImageIndex *int               `json:"image_index"`
			URL        string             `json:"url"`
			Usage      *dto.Usage         `json:"usage"`
			Error      *types.OpenAIError `json:"error"`
		}
		if err := common.Unmarshal([]byte(data), &event); err != nil {
			sr.Stop(err)
			return
		}
		switch event.Type {
		case "image_generation.partial_succeeded", "image_edit.partial_succeeded":
			if event.URL != "" && event.ImageIndex != nil && *event.ImageIndex >= 0 && *event.ImageIndex < dto.MaxImageN {
				succeeded[*event.ImageIndex] = struct{}{}
			}
		case "image_generation.completed", "image_edit.completed":
			// H3 completes the whole batch once. Its partial_succeeded events
			// carry the image payloads; the completion event carries usage only.
			service.ApplyResponsesUsage(usage, event.Usage)
			receivedUsage = event.Usage != nil
		case "error":
			if event.Error == nil {
				event.Error = &types.OpenAIError{Type: "minimax_image_error", Message: "H3 image stream failed"}
			}
			streamErr = types.WithOpenAIError(*event.Error, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
			sr.Error(streamErr)
		}
		if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, data); err != nil {
			sr.Stop(err)
		}
	})
	if streamErr != nil && len(succeeded) == 0 {
		// The host refunds failed submissions. Do not settle the reserved
		// quantity for the provider's all-images-failed SSE response.
		return nil, streamErr
	}
	if !receivedUsage {
		info.UsageBillingFallback = "missing_usage"
	}
	count := int64(len(succeeded))
	if info.StreamStatus != nil {
		finished := info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone || info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF
		if finished || count > int64(info.RequestedImageCount()) {
			info.UpdateImageCount(count)
		}
		if info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone {
			helper.Done(c)
		}
	}
	return usage, nil
}
