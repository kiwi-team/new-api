package tencent

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	hyImageV35Model             = "hy-image-v3.5-preview"
	tokenHubHyImageV35Path      = "/v1/wand/hunyuan-image/v35-generation"
	maxTokenHubImageResponse    = int64(8 << 20)
	maxHyImageV35Dimension      = int64(8192)
	maxHyImageV35Pixels         = int64(4096 * 4096)
	maxHyImageV35GeneratedPixel = int64(2048 * 2048)
)

type tokenHubHyImageV35Request struct {
	Model             string              `json:"model"`
	Messages          common.RawMessage   `json:"messages"`
	Size              *string             `json:"size,omitempty"`
	Seed              *int64              `json:"seed,omitempty"`
	GenerateMaxPixels *int64              `json:"generate_max_pixels,omitempty"`
	ResizeMaxPixels   *int64              `json:"resize_max_pixels,omitempty"`
	Session           *string             `json:"session,omitempty"`
	Footnote          *string             `json:"footnote,omitempty"`
	UseSearchTool     *tokenHubSearchTool `json:"use_search_tool,omitempty"`
}

// Keep provider messages raw. In particular, multi-round image editing needs
// is_generate and the assembled_history tool messages to survive unchanged.
type tokenHubSearchTool struct {
	Value *bool `json:"value"`
}

type tokenHubHyImageV35Response struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Image *struct {
				URL string `json:"url"`
			} `json:"image"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"usage"`
	TokenHubUsage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"tokenhub_usage"`
	Error *struct {
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func isTokenHubHyImageV35(info *relaycommon.RelayInfo) bool {
	return info != nil && info.RelayMode == relayconstant.RelayModeChatCompletions && info.UpstreamModelName == hyImageV35Model
}

func convertTokenHubHyImageV35Request(c *gin.Context, request *dto.GeneralOpenAIRequest) (*tokenHubHyImageV35Request, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if request.Stream != nil && *request.Stream {
		return nil, errors.New("hy-image-v3.5-preview only supports synchronous requests")
	}

	var upstream tokenHubHyImageV35Request
	if err := common.UnmarshalBodyReusable(c, &upstream); err != nil {
		return nil, fmt.Errorf("decode hy-image-v3.5-preview request: %w", err)
	}
	upstream.Model = request.Model
	var messages []common.RawMessage
	if err := common.Unmarshal(upstream.Messages, &messages); err != nil || len(messages) == 0 {
		return nil, errors.New("messages is required")
	}
	if upstream.Size != nil && *upstream.Size != "" {
		width, height, ok := strings.Cut(*upstream.Size, "x")
		if !ok {
			return nil, errors.New("size must use the WIDTHxHEIGHT format")
		}
		w, widthErr := strconv.ParseInt(width, 10, 64)
		h, heightErr := strconv.ParseInt(height, 10, 64)
		if widthErr != nil || heightErr != nil || w < 256 || h < 256 || w > maxHyImageV35Dimension || h > maxHyImageV35Dimension || w > maxHyImageV35Pixels/h {
			return nil, errors.New("size dimensions must be between 256 and 8192 and contain at most 16777216 pixels")
		}
	}
	if upstream.Seed != nil && *upstream.Seed < 0 {
		return nil, errors.New("seed must be non-negative")
	}
	if upstream.GenerateMaxPixels != nil && (*upstream.GenerateMaxPixels <= 0 || *upstream.GenerateMaxPixels > maxHyImageV35GeneratedPixel) {
		return nil, errors.New("generate_max_pixels must be between 1 and 4194304")
	}
	if upstream.ResizeMaxPixels != nil && *upstream.ResizeMaxPixels <= 0 {
		return nil, errors.New("resize_max_pixels must be positive")
	}
	if upstream.Footnote != nil && utf8.RuneCountInString(*upstream.Footnote) > 16 {
		return nil, errors.New("footnote must contain at most 16 characters")
	}
	if upstream.UseSearchTool != nil && upstream.UseSearchTool.Value == nil {
		return nil, errors.New("use_search_tool.value is required")
	}
	return &upstream, nil
}

func tokenHubHyImageV35Handler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenHubImageResponse+1))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if int64(len(body)) > maxTokenHubImageResponse {
		return nil, types.NewOpenAIError(errors.New("TokenHub image response is too large"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	var result tokenHubHyImageV35Response
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if result.Error != nil && result.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: result.Error.Message,
			Type:    result.Error.Type,
			Code:    result.Error.Code,
		}, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	hasImage := false
	for _, choice := range result.Choices {
		if choice.Delta.Image != nil && choice.Delta.Image.URL != "" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return nil, types.NewErrorWithStatusCode(errors.New("TokenHub image response did not contain an image URL"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}

	totalTokens := result.TokenHubUsage.TotalTokens
	if totalTokens == 0 {
		totalTokens = result.Usage.TotalTokens
	}
	if totalTokens < 0 {
		return nil, types.NewErrorWithStatusCode(errors.New("TokenHub image response returned negative usage"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	tokens, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(totalTokens))
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	info.ObserveResponseModel(result.Model)
	service.IOCopyBytesGracefully(c, resp, body)
	return &dto.Usage{PromptTokens: tokens, TotalTokens: tokens}, nil
}
