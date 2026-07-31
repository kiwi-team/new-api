package ali

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func aliAudioHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	var aliResp Qwen3TTSResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewErrorWithStatusCode(
			errors.New("failed to read volcengine response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		), nil
	}
	err = json.Unmarshal(body, &aliResp)
	if err != nil {
		return types.NewErrorWithStatusCode(
			errors.New("failed to unmarshal qwen tts response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		), nil
	}
	if aliResp.StatusCode != 200 && aliResp.StatusCode != 0 {
		return types.NewErrorWithStatusCode(
			errors.New("failed : status code != 200 or code != 0"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		), nil
	}
	audioURL := aliResp.Output.Audio.URL
	if len(audioURL) == 0 {
		return types.NewErrorWithStatusCode(
			errors.New("ali tts failed: empty audio data"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		), nil
	}
	s3Url, err := service.SimpleUploadToS3(c, audioURL)
	if err == nil {
		audioURL = s3Url
	}
	common.SetContextKey(c, constant.ContextKeyAudioUrl, audioURL)
	common.ApiSuccess(c, gin.H{
		"audio_url": audioURL,
	})
	return nil, &dto.Usage{
		PromptTokens:     aliResp.Usage.InputTokens,
		CompletionTokens: aliResp.Usage.OutputTokens,
		TotalTokens:      aliResp.Usage.TotalTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			TextTokens: aliResp.Usage.InputTokensDetails.TextTokens,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{
			AudioTokens: aliResp.Usage.OutputTokensDetails.AudioTokens,
			TextTokens:  aliResp.Usage.OutputTokensDetails.TextTokens,
		},
	}
}
