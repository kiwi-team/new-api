package tencent

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	hyASRModel             = "hy-asr-3.0-preview"
	tokenHubSyncASRPath    = "/v1/wand/asrproxy/sync_transcribe"
	maxTokenHubASRResponse = int64(64 << 20)
)

type TokenHubAdaptor struct {
	openai.Adaptor
}

type tokenHubASRRequest struct {
	Model             string `json:"model"`
	InputURL          string `json:"input_url,omitempty"`
	Data              string `json:"data,omitempty"`
	Source            string `json:"source,omitempty"`
	VoiceEncodeFormat string `json:"voice_encode_format,omitempty"`
}

type tokenHubASRResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	RequestID string `json:"request_id"`
	Output    struct {
		Source     string `json:"source"`
		DurationMS int64  `json:"duration_ms"`
		Text       string `json:"text"`
		Sentences  []struct {
			BeginMS int64  `json:"begin_ms"`
			EndMS   int64  `json:"end_ms"`
			Text    string `json:"text"`
		} `json:"sentences"`
		SubtitleURL string `json:"subtitle_url"`
	} `json:"output"`
	Usage struct {
		TotalToken int64 `json:"total_token"`
	} `json:"usage"`
}

func isTokenHubASR(info *relaycommon.RelayInfo) bool {
	return info != nil && info.RelayMode == relayconstant.RelayModeAudioTranscription && info.UpstreamModelName == hyASRModel
}

func (a *TokenHubAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isTokenHubASR(info) {
		return strings.TrimRight(info.ChannelBaseUrl, "/") + tokenHubSyncASRPath, nil
	}
	return a.Adaptor.GetRequestURL(info)
}

func (a *TokenHubAdaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if !isTokenHubASR(info) {
		return a.Adaptor.SetupRequestHeader(c, header, info)
	}
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *TokenHubAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info != nil && info.UpstreamModelName == hyASRModel && info.RelayMode != relayconstant.RelayModeAudioTranscription {
		return nil, errors.New("hy-asr-3.0-preview only supports audio transcription")
	}
	if !isTokenHubASR(info) {
		return a.Adaptor.ConvertAudioRequest(c, info, request)
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "json" && request.ResponseFormat != "text" && request.ResponseFormat != "verbose_json" && request.ResponseFormat != "srt" && request.ResponseFormat != "vtt" {
		return nil, fmt.Errorf("unsupported response_format %q for TokenHub ASR", request.ResponseFormat)
	}

	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse audio form: %w", err)
	}
	defer form.RemoveAll()

	files := form.File["file"]
	audioURL := firstTencentASRFormValue(form.Value, "audio_url")
	inputURL := firstTencentASRFormValue(form.Value, "input_url")
	if audioURL != "" && inputURL != "" {
		return nil, errors.New("audio_url and input_url cannot both be provided")
	}
	if inputURL == "" {
		inputURL = audioURL
	}
	if (len(files) == 0) == (inputURL == "") {
		return nil, errors.New("exactly one of file or audio_url/input_url is required")
	}

	source := firstTencentASRFormValue(form.Value, "source")
	if source == "" {
		source = firstTencentASRFormValue(form.Value, "language")
	}
	upstream := tokenHubASRRequest{
		Model:             request.Model,
		Source:            source,
		VoiceEncodeFormat: strings.ToLower(firstTencentASRFormValue(form.Value, "voice_encode_format")),
	}
	if inputURL != "" {
		if err := service.ValidateSSRFProtectedFetchURL(inputURL); err != nil {
			return nil, fmt.Errorf("invalid audio_url: %w", err)
		}
		upstream.InputURL = inputURL
		if upstream.VoiceEncodeFormat == "" {
			parsed, parseErr := url.Parse(inputURL)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid audio_url: %w", parseErr)
			}
			upstream.VoiceEncodeFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(parsed.Path)), ".")
		}
	} else {
		fileHeader := files[0]
		file, openErr := fileHeader.Open()
		if openErr != nil {
			return nil, fmt.Errorf("open audio file: %w", openErr)
		}
		defer file.Close()
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			return nil, fmt.Errorf("read audio file: %w", readErr)
		}
		upstream.Data = base64.StdEncoding.EncodeToString(data)
		if upstream.VoiceEncodeFormat == "" {
			upstream.VoiceEncodeFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(fileHeader.Filename)), ".")
		}
	}

	body, err := common.Marshal(upstream)
	if err != nil {
		return nil, fmt.Errorf("encode TokenHub ASR request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *TokenHubAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if isTokenHubASR(info) {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
	return a.Adaptor.DoRequest(c, info, requestBody)
}

func (a *TokenHubAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if !isTokenHubASR(info) {
		return a.Adaptor.DoResponse(c, resp, info)
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenHubASRResponse+1))
	if err != nil {
		return nil, tokenHubASRError(fmt.Errorf("read TokenHub ASR response: %w", err))
	}
	if int64(len(body)) > maxTokenHubASRResponse {
		return nil, tokenHubASRError(errors.New("TokenHub ASR response is too large"))
	}

	var result tokenHubASRResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, tokenHubASRError(fmt.Errorf("decode TokenHub ASR response: %w", err))
	}
	if result.Status != "completed" {
		return nil, tokenHubASRError(fmt.Errorf("TokenHub ASR returned status %q (request_id: %s)", result.Status, result.RequestID))
	}
	if result.Output.DurationMS < 0 || result.Usage.TotalToken < 0 {
		return nil, tokenHubASRError(errors.New("TokenHub ASR returned negative usage"))
	}
	if err := writeTokenHubASRResponse(c, info, &result); err != nil {
		return nil, tokenHubASRError(err)
	}

	audioTokens, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(result.Usage.TotalToken))
	if audioTokens == 0 && result.Output.DurationMS > 0 {
		audioTokens, clamp = common.QuotaRoundChecked(math.Ceil(float64(result.Output.DurationMS) / 1000 * 22))
	}
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	seconds, secondsClamp := common.QuotaRoundChecked(math.Ceil(float64(result.Output.DurationMS) / 1000))
	if secondsClamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = secondsClamp
	}
	return &dto.Usage{
		PromptTokens: audioTokens,
		TotalTokens:  audioTokens,
		Seconds:      seconds,
		PromptTokensDetails: dto.InputTokenDetails{
			AudioTokens: audioTokens,
		},
	}, nil
}

func firstTencentASRFormValue(values map[string][]string, key string) string {
	if items := values[key]; len(items) > 0 {
		return strings.TrimSpace(items[0])
	}
	return ""
}

func writeTokenHubASRResponse(c *gin.Context, info *relaycommon.RelayInfo, result *tokenHubASRResponse) error {
	request, _ := info.Request.(*dto.AudioRequest)
	responseFormat := "json"
	if request != nil && request.ResponseFormat != "" {
		responseFormat = request.ResponseFormat
	}
	switch responseFormat {
	case "json":
		c.JSON(http.StatusOK, dto.AudioResponse{Text: result.Output.Text})
	case "text":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(result.Output.Text))
	case "verbose_json":
		response := dto.WhisperVerboseJSONResponse{
			Task:     "transcribe",
			Language: result.Output.Source,
			Duration: float64(result.Output.DurationMS) / 1000,
			Text:     result.Output.Text,
		}
		for i, sentence := range result.Output.Sentences {
			response.Segments = append(response.Segments, dto.Segment{
				Id: i, Start: float64(sentence.BeginMS) / 1000, End: float64(sentence.EndMS) / 1000, Text: sentence.Text,
			})
		}
		c.JSON(http.StatusOK, response)
	case "srt", "vtt":
		var output strings.Builder
		if responseFormat == "vtt" {
			output.WriteString("WEBVTT\n\n")
		}
		for i, sentence := range result.Output.Sentences {
			if responseFormat == "srt" {
				fmt.Fprintf(&output, "%d\n", i+1)
			}
			fmt.Fprintf(&output, "%s --> %s\n%s\n\n", tokenHubASRTimestamp(sentence.BeginMS, responseFormat), tokenHubASRTimestamp(sentence.EndMS, responseFormat), sentence.Text)
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(output.String()))
	default:
		return fmt.Errorf("unsupported response_format %q for TokenHub ASR", responseFormat)
	}
	return nil
}

func tokenHubASRTimestamp(milliseconds int64, responseFormat string) string {
	milliseconds = max(milliseconds, 0)
	separator := ','
	if responseFormat == "vtt" {
		separator = '.'
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", milliseconds/3_600_000, milliseconds/60_000%60, milliseconds/1000%60, separator, milliseconds%1000)
}

func tokenHubASRError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}
