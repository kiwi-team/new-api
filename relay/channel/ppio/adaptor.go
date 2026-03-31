package ppio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
)

// =====================
// Request / Response
// =====================

// POST https://api.ppio.com/v3/gemini-3-pro-image-text-to-image
type TextToImageRequest struct {
	Prompt       string        `json:"prompt"`
	Size         string        `json:"size,omitempty"`
	AspectRatio  string        `json:"aspect_ratio,omitempty"`
	OutputFormat string        `json:"output_format,omitempty"`
	Google       *GoogleOption `json:"google,omitempty"`
}

// POST https://api.ppio.com/v3/gemini-3-pro-image-edit
type ImageEditRequest struct {
	Prompt       string        `json:"prompt"`
	Size         string        `json:"size,omitempty"`
	AspectRatio  string        `json:"aspect_ratio,omitempty"`
	OutputFormat string        `json:"output_format,omitempty"`
	ImageURLs    []string      `json:"image_urls,omitempty"`
	ImageBase64s []string      `json:"image_base64s,omitempty"`
	Google       *GoogleOption `json:"google,omitempty"`
}

type GoogleOption struct {
	WebSearch bool `json:"web_search,omitempty"`
}

type ImageResponse struct {
	ImageURLs         []string    `json:"image_urls"`
	GroundingMetadata interface{} `json:"grounding_metadata,omitempty"`
}

// =====================
// Adaptor
// =====================

type Adaptor struct {
	hasImageInput bool
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetChannelName() string  { return "PPIO" }
func (a *Adaptor) GetModelList() []string  { return []string{"gemini-3-pro-image-preview"} }

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimSuffix(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = "https://api.ppio.com"
	}
	if a.hasImageInput {
		return baseURL + "/v3/gemini-3-pro-image-edit", nil
	}
	return baseURL + "/v3/gemini-3-pro-image-text-to-image", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, errors.New("ppio adaptor: prompt is required")
	}

	imageURLs, err := request.GetImageURLs()
	if err != nil {
		return nil, fmt.Errorf("ppio adaptor: failed to get image URLs: %w", err)
	}

	// Parse extra fields for ppio-specific parameters
	var extraFields map[string]any
	if len(request.ExtraFields) > 0 {
		if err := common.Unmarshal(request.ExtraFields, &extraFields); err != nil {
			return nil, fmt.Errorf("ppio adaptor: failed to decode extra_fields: %w", err)
		}
	}
	var extraMap map[string]any
	if len(request.Extra) > 0 {
		extraMap = make(map[string]any)
		for key, raw := range request.Extra {
			if raw == nil {
				continue
			}
			var val any
			if err := common.Unmarshal(raw, &val); err != nil {
				continue
			}
			extraMap[key] = val
		}
	}

	merged := mergeExtras(extraFields, extraMap)

	a.hasImageInput = len(imageURLs) > 0

	if a.hasImageInput {
		body := &ImageEditRequest{
			Prompt:    prompt,
			Size:      request.Size,
			ImageURLs: imageURLs,
		}
		applyExtras(nil, body, merged)
		return body, nil
	}

	body := &TextToImageRequest{
		Prompt: prompt,
		Size:   request.Size,
	}
	applyExtras(body, nil, merged)
	return body, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewError(errors.New("ppio adaptor: empty response"), types.ErrorCodeBadResponse)
	}

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewError(readErr, types.ErrorCodeReadResponseBodyFailed)
	}
	_ = resp.Body.Close()

	var ppioResp ImageResponse
	if unmarshalErr := common.Unmarshal(responseBody, &ppioResp); unmarshalErr != nil {
		return nil, types.NewError(fmt.Errorf("ppio adaptor: failed to decode response: %w, body: %s", unmarshalErr, string(responseBody)), types.ErrorCodeBadResponseBody)
	}

	if len(ppioResp.ImageURLs) == 0 {
		return nil, types.NewError(errors.New("ppio adaptor: no images in response"), types.ErrorCodeBadResponseBody)
	}

	// Gemini native format: return GeminiChatResponse with inlineData
	if info.RelayMode == relayconstant.RelayModeGemini {
		return a.doGeminiResponse(c, ppioResp)
	}

	return a.doOpenAIImageResponse(c, info, ppioResp)
}

// doGeminiResponse builds a Gemini-native response with image data as inlineData parts.
func (a *Adaptor) doGeminiResponse(c *gin.Context, ppioResp ImageResponse) (any, *types.NewAPIError) {
	var parts []dto.GeminiPart

	for _, rawURL := range ppioResp.ImageURLs {
		url := rawURL
		if uploaded, uploadErr := service.SimpleUploadToS3(context.Background(), url); uploadErr == nil {
			url = uploaded
		}

		mimeType, b64Data, downloadErr := service.GetImageFromUrl(url)
		if downloadErr != nil {
			return nil, types.NewError(fmt.Errorf("ppio adaptor: failed to download image: %w", downloadErr), types.ErrorCodeBadResponse)
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		if b64Data != "" {
			parts = append(parts, dto.GeminiPart{
				InlineData: &dto.GeminiInlineData{
					MimeType: mimeType,
					Data:     b64Data,
				},
			})
		}
	}

	if len(parts) == 0 {
		return nil, types.NewError(errors.New("ppio adaptor: no usable image data"), types.ErrorCodeBadResponse)
	}

	finishReason := "STOP"
	geminiResp := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Parts: parts,
					Role:  "model",
				},
				FinishReason: &finishReason,
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     0,
			CandidatesTokenCount: len(parts) * 258,
			TotalTokenCount:      len(parts) * 258,
		},
	}

	respBytes, marshalErr := common.Marshal(geminiResp)
	if marshalErr != nil {
		return nil, types.NewError(fmt.Errorf("ppio adaptor: encode gemini response failed: %w", marshalErr), types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBytes)

	return &dto.Usage{
		PromptTokens:     0,
		CompletionTokens: len(parts) * 258,
		TotalTokens:      len(parts) * 258,
	}, nil
}

// doOpenAIImageResponse builds an OpenAI-compatible image response.
func (a *Adaptor) doOpenAIImageResponse(c *gin.Context, info *relaycommon.RelayInfo, ppioResp ImageResponse) (any, *types.NewAPIError) {
	var wantsBase64 bool
	if info != nil {
		if req, ok := info.Request.(*dto.ImageRequest); ok {
			wantsBase64 = strings.EqualFold(req.ResponseFormat, "b64_json")
		}
	}

	imageResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0, len(ppioResp.ImageURLs)),
	}

	for _, rawURL := range ppioResp.ImageURLs {
		url := rawURL
		if uploaded, uploadErr := service.SimpleUploadToS3(context.Background(), url); uploadErr == nil {
			url = uploaded
		}

		if wantsBase64 {
			_, b64Data, downloadErr := service.GetImageFromUrl(url)
			if downloadErr != nil {
				return nil, types.NewError(fmt.Errorf("ppio adaptor: failed to download image: %w", downloadErr), types.ErrorCodeBadResponse)
			}
			if b64Data != "" {
				imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: b64Data})
			}
		} else {
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: url})
		}
	}

	if len(imageResponse.Data) == 0 {
		return nil, types.NewError(errors.New("ppio adaptor: no usable image data"), types.ErrorCodeBadResponse)
	}

	responseBytes, marshalErr := common.Marshal(imageResponse)
	if marshalErr != nil {
		return nil, types.NewError(fmt.Errorf("ppio adaptor: encode response failed: %w", marshalErr), types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(responseBytes)

	return &dto.Usage{}, nil
}

// =====================
// Stub methods (not used for image generation)
// =====================

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("ppio adaptor: ConvertOpenAIRequest is not implemented")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("ppio adaptor: ConvertRerankRequest is not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("ppio adaptor: ConvertEmbeddingRequest is not implemented")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("ppio adaptor: ConvertAudioRequest is not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("ppio adaptor: ConvertOpenAIResponsesRequest is not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("ppio adaptor: ConvertClaudeRequest is not implemented")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// Extract prompt and images from Gemini native request
	var promptParts []string
	var imageURLs []string
	var imageBase64s []string

	for _, content := range request.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				promptParts = append(promptParts, part.Text)
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				imageBase64s = append(imageBase64s, part.InlineData.Data)
			}
			if part.FileData != nil && part.FileData.FileUri != "" {
				imageURLs = append(imageURLs, part.FileData.FileUri)
			}
		}
	}

	prompt := strings.Join(promptParts, "\n")
	if prompt == "" {
		return nil, errors.New("ppio adaptor: prompt is required")
	}

	a.hasImageInput = len(imageURLs) > 0 || len(imageBase64s) > 0

	// Extract optional params from GenerationConfig.ImageConfig (json.RawMessage)
	var size, aspectRatio, outputFormat string
	if len(request.GenerationConfig.ImageConfig) > 0 {
		var imageConfig map[string]any
		if err := common.Unmarshal(request.GenerationConfig.ImageConfig, &imageConfig); err == nil {
			if s, ok := imageConfig["outputImageSize"].(string); ok {
				size = s
			}
			if s, ok := imageConfig["aspectRatio"].(string); ok {
				aspectRatio = s
			}
			if s, ok := imageConfig["outputFormat"].(string); ok {
				outputFormat = s
			}
		}
	}

	if a.hasImageInput {
		body := &ImageEditRequest{
			Prompt:       prompt,
			Size:         size,
			AspectRatio:  aspectRatio,
			OutputFormat: outputFormat,
			ImageURLs:    imageURLs,
			ImageBase64s: imageBase64s,
		}
		return body, nil
	}

	body := &TextToImageRequest{
		Prompt:       prompt,
		Size:         size,
		AspectRatio:  aspectRatio,
		OutputFormat: outputFormat,
	}
	return body, nil
}

// =====================
// Helpers
// =====================

func mergeExtras(a, b map[string]any) map[string]any {
	if a == nil && b == nil {
		return nil
	}
	merged := make(map[string]any)
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

func applyExtras(t2i *TextToImageRequest, edit *ImageEditRequest, extras map[string]any) {
	if extras == nil {
		return
	}

	aspectRatio, _ := extras["aspect_ratio"].(string)
	outputFormat, _ := extras["output_format"].(string)
	size, _ := extras["size"].(string)

	var google *GoogleOption
	if g, ok := extras["google"].(map[string]any); ok {
		if ws, ok := g["web_search"].(bool); ok {
			google = &GoogleOption{WebSearch: ws}
		}
	}

	if t2i != nil {
		if aspectRatio != "" {
			t2i.AspectRatio = aspectRatio
		}
		if outputFormat != "" {
			t2i.OutputFormat = outputFormat
		}
		if size != "" {
			t2i.Size = size
		}
		if google != nil {
			t2i.Google = google
		}
	}
	if edit != nil {
		if aspectRatio != "" {
			edit.AspectRatio = aspectRatio
		}
		if outputFormat != "" {
			edit.OutputFormat = outputFormat
		}
		if size != "" {
			edit.Size = size
		}
		if google != nil {
			edit.Google = google
		}
		if v, ok := extras["image_base64s"].([]any); ok {
			for _, item := range v {
				if s, ok := item.(string); ok {
					edit.ImageBase64s = append(edit.ImageBase64s, s)
				}
			}
		}
	}
}
