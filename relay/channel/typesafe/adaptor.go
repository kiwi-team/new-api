package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct{}

type systemOneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type systemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   *systemOneUsage            `json:"usage"`
}

func parseSystemOneResponse(responseBody []byte) (*systemOneResponse, *dto.Usage, error) {
	var upstream systemOneResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return nil, nil, err
	}
	if upstream.Usage == nil {
		return nil, nil, errors.New("TypeSafe response is missing usage")
	}
	if upstream.Usage.InputTokens < 0 || upstream.Usage.OutputTokens < 0 {
		return nil, nil, errors.New("TypeSafe usage token counts must not be negative")
	}
	if upstream.Usage.InputTokens > int(^uint(0)>>1)-upstream.Usage.OutputTokens {
		return nil, nil, errors.New("TypeSafe usage token count overflow")
	}
	usage := &dto.Usage{
		PromptTokens:     upstream.Usage.InputTokens,
		CompletionTokens: upstream.Usage.OutputTokens,
		TotalTokens:      upstream.Usage.InputTokens + upstream.Usage.OutputTokens,
		InputTokens:      upstream.Usage.InputTokens,
		OutputTokens:     upstream.Usage.OutputTokens,
		UsageSource:      dto.BillingUsageSourceTypeSafe,
	}
	return &upstream, usage, nil
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/systemone", info.ChannelBaseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, kitreasoning.AsClientError(errors.New("TypeSafe channels do not support /v1/chat/completions; use the native /v1/systemone endpoint"))
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support rerank requests")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support embedding requests")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("TypeSafe channels do not support audio requests")
}

func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support image requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support Responses API requests")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support Anthropic Messages requests")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("TypeSafe channels do not support Gemini requests")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, types.NewErrorWithStatusCode(errors.New("TypeSafe channels only support native System One responses"), types.ErrorCodeNotSupported, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// DoNativeResponse validates billing usage before forwarding the untouched
// System One response body to the caller.
func (a *Adaptor) DoNativeResponse(c *gin.Context, resp *http.Response) (*dto.Usage, []byte, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	_, usage, err := parseSystemOneResponse(responseBody)
	if err != nil {
		return nil, responseBody, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, responseBody)
	return usage, responseBody, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
