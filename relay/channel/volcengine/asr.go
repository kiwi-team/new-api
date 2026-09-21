package volcengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	seedASRFileResourceID = "volc.seedasr.auc"
	seedASRMaxFileBytes   = int64(512 << 20)
	seedASRMaxBodyBytes   = int64(64 << 20)
	contextKeyASRRequest  = "volcengine_seed_asr_request_id"
	contextKeyASRAudioURL = "volcengine_seed_asr_audio_url"
)

type seedASRCorpus struct {
	BoostingTableName string `json:"boosting_table_name,omitempty"`
	BoostingTableID   string `json:"boosting_table_id,omitempty"`
	CorrectTableName  string `json:"correct_table_name,omitempty"`
	CorrectTableID    string `json:"correct_table_id,omitempty"`
	Context           string `json:"context,omitempty"`
}

type seedASRMetadata struct {
	EnableITN              *bool          `json:"enable_itn,omitempty"`
	EnablePunc             *bool          `json:"enable_punc,omitempty"`
	EnableDDC              *bool          `json:"enable_ddc,omitempty"`
	EnableSpeakerInfo      *bool          `json:"enable_speaker_info,omitempty"`
	EnableChannelSplit     *bool          `json:"enable_channel_split,omitempty"`
	ShowUtterances         *bool          `json:"show_utterances,omitempty"`
	ShowSpeechRate         *bool          `json:"show_speech_rate,omitempty"`
	ShowVolume             *bool          `json:"show_volume,omitempty"`
	EnableLID              *bool          `json:"enable_lid,omitempty"`
	EnableEmotionDetection *bool          `json:"enable_emotion_detection,omitempty"`
	EnableGenderDetection  *bool          `json:"enable_gender_detection,omitempty"`
	VadSegment             *bool          `json:"vad_segment,omitempty"`
	EndWindowSize          *int           `json:"end_window_size,omitempty"`
	OutputZHVariant        string         `json:"output_zh_variant,omitempty"`
	AudioFormat            string         `json:"audio_format,omitempty"`
	Codec                  string         `json:"codec,omitempty"`
	Rate                   *int           `json:"rate,omitempty"`
	Bits                   *int           `json:"bits,omitempty"`
	Channel                *int           `json:"channel,omitempty"`
	Corpus                 *seedASRCorpus `json:"corpus,omitempty"`
}

type seedASRFileRequest struct {
	User struct {
		UID string `json:"uid"`
	} `json:"user"`
	Audio struct {
		URL      string `json:"url"`
		Format   string `json:"format"`
		Codec    string `json:"codec,omitempty"`
		Rate     int    `json:"rate,omitempty"`
		Bits     int    `json:"bits,omitempty"`
		Channel  int    `json:"channel,omitempty"`
		Language string `json:"language,omitempty"`
	} `json:"audio"`
	Request struct {
		ModelName              string         `json:"model_name"`
		EnableITN              bool           `json:"enable_itn"`
		EnablePunc             bool           `json:"enable_punc"`
		EnableDDC              *bool          `json:"enable_ddc,omitempty"`
		EnableSpeakerInfo      *bool          `json:"enable_speaker_info,omitempty"`
		EnableChannelSplit     *bool          `json:"enable_channel_split,omitempty"`
		ShowUtterances         bool           `json:"show_utterances"`
		ShowSpeechRate         *bool          `json:"show_speech_rate,omitempty"`
		ShowVolume             *bool          `json:"show_volume,omitempty"`
		EnableLID              *bool          `json:"enable_lid,omitempty"`
		EnableEmotionDetection *bool          `json:"enable_emotion_detection,omitempty"`
		EnableGenderDetection  *bool          `json:"enable_gender_detection,omitempty"`
		VadSegment             *bool          `json:"vad_segment,omitempty"`
		EndWindowSize          *int           `json:"end_window_size,omitempty"`
		OutputZHVariant        string         `json:"output_zh_variant,omitempty"`
		Corpus                 *seedASRCorpus `json:"corpus,omitempty"`
	} `json:"request"`
}

type seedASRFileResponse struct {
	AudioInfo struct {
		Duration int64 `json:"duration"`
	} `json:"audio_info"`
	Result struct {
		Text       string             `json:"text"`
		Utterances []seedASRUtterance `json:"utterances"`
	} `json:"result"`
}

type seedASRUtterance struct {
	Text      string `json:"text"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

func isSeedASRFile(info *relaycommon.RelayInfo) bool {
	return info != nil && info.RelayMode == relayconstant.RelayModeAudioTranscription
}

func parseSeedASRAuth(apiKey string) (appKey, accessKey, modernKey string, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", "", "", errors.New("empty Volcengine API key")
	}
	if appKey, accessKey, ok := strings.Cut(apiKey, "|"); ok {
		appKey = strings.TrimSpace(appKey)
		accessKey = strings.TrimSpace(accessKey)
		if appKey == "" || accessKey == "" || strings.Contains(accessKey, "|") {
			return "", "", "", errors.New("legacy Volcengine key must be app_key|access_key")
		}
		return appKey, accessKey, "", nil
	}
	return "", "", apiKey, nil
}

func setupSeedASRHeaders(c *gin.Context, header http.Header, info *relaycommon.RelayInfo, resourceID string) error {
	appKey, accessKey, modernKey, err := parseSeedASRAuth(info.ApiKey)
	if err != nil {
		return err
	}
	if modernKey != "" {
		header.Set("X-Api-Key", modernKey)
	} else {
		header.Set("X-Api-App-Key", appKey)
		header.Set("X-Api-Access-Key", accessKey)
	}
	header.Set("X-Api-Resource-Id", resourceID)
	requestID := c.GetString(contextKeyASRRequest)
	if requestID == "" {
		requestID = uuid.NewString()
		c.Set(contextKeyASRRequest, requestID)
	}
	header.Set("X-Api-Request-Id", requestID)
	return nil
}

func convertSeedASRRequest(c *gin.Context, request dto.AudioRequest) (io.Reader, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse audio form: %w", err)
	}
	defer form.RemoveAll()

	metadata, err := parseSeedASRMetadata(form)
	if err != nil {
		return nil, err
	}
	responseFormat := request.ResponseFormat
	if responseFormat == "" {
		responseFormat = "json"
	}
	if responseFormat != "json" && responseFormat != "text" && responseFormat != "verbose_json" && responseFormat != "srt" && responseFormat != "vtt" {
		return nil, fmt.Errorf("unsupported response_format %q", responseFormat)
	}

	audioURL := firstFormValue(form, "audio_url")
	files := form.File["file"]
	if (audioURL == "") == (len(files) == 0) {
		return nil, errors.New("exactly one of file or audio_url is required")
	}
	audioFormat := strings.ToLower(strings.TrimSpace(metadata.AudioFormat))
	if audioURL != "" {
		if err := service.ValidateSSRFProtectedFetchURL(audioURL); err != nil {
			return nil, fmt.Errorf("invalid audio_url: %w", err)
		}
		if audioFormat == "" {
			audioFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.Split(audioURL, "?")[0])), ".")
		}
	} else {
		fileHeader := files[0]
		if fileHeader.Size <= 0 {
			return nil, errors.New("audio file must not be empty")
		}
		if fileHeader.Size > seedASRMaxFileBytes {
			return nil, fmt.Errorf("audio file exceeds the 512 MB Seed-ASR limit")
		}
		audioURL = c.GetString(contextKeyASRAudioURL)
		if audioURL == "" {
			audioFile, openErr := fileHeader.Open()
			if openErr != nil {
				return nil, fmt.Errorf("open audio file: %w", openErr)
			}
			defer audioFile.Close()
			probe := make([]byte, 512)
			readBytes, readErr := audioFile.Read(probe)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return nil, fmt.Errorf("inspect audio file: %w", readErr)
			}
			if _, seekErr := audioFile.Seek(0, io.SeekStart); seekErr != nil {
				return nil, fmt.Errorf("rewind audio file: %w", seekErr)
			}
			contentType := http.DetectContentType(probe[:readBytes])
			if detectedFormat := seedASRFormatFromContentType(contentType); detectedFormat != "" {
				audioFormat = detectedFormat
			}
			if audioFormat == "pcm" {
				audioFormat = "raw"
			}
			if audioFormat != "raw" && audioFormat != "wav" && audioFormat != "mp3" && audioFormat != "ogg" {
				return nil, fmt.Errorf("unsupported or unknown audio format %q", audioFormat)
			}
			audioURL, err = service.UploadReaderToS3(c.Request.Context(), audioFile, fileHeader.Size, contentType, filepath.Ext(fileHeader.Filename))
			if err != nil {
				return nil, fmt.Errorf("upload audio file (configure S3 or pass audio_url): %w", err)
			}
			c.Set(contextKeyASRAudioURL, audioURL)
		}
	}
	if audioFormat == "pcm" {
		audioFormat = "raw"
	}
	if audioFormat != "raw" && audioFormat != "wav" && audioFormat != "mp3" && audioFormat != "ogg" {
		return nil, fmt.Errorf("unsupported or unknown audio format %q", audioFormat)
	}

	requestID := c.GetString(contextKeyASRRequest)
	if requestID == "" {
		requestID = uuid.NewString()
		c.Set(contextKeyASRRequest, requestID)
	}
	upstream := seedASRFileRequest{}
	upstream.User.UID = "new-api-" + requestID
	upstream.Audio.URL = audioURL
	upstream.Audio.Format = audioFormat
	upstream.Audio.Codec = metadata.Codec
	upstream.Audio.Rate = valueOr(metadata.Rate, 0)
	upstream.Audio.Bits = valueOr(metadata.Bits, 0)
	upstream.Audio.Channel = valueOr(metadata.Channel, 0)
	upstream.Audio.Language = firstFormValue(form, "language")
	upstream.Request.ModelName = "bigmodel"
	upstream.Request.EnableITN = valueOr(metadata.EnableITN, true)
	upstream.Request.EnablePunc = valueOr(metadata.EnablePunc, true)
	upstream.Request.ShowUtterances = valueOr(metadata.ShowUtterances, responseFormat == "verbose_json" || responseFormat == "srt" || responseFormat == "vtt")
	upstream.Request.EnableDDC = metadata.EnableDDC
	upstream.Request.EnableSpeakerInfo = metadata.EnableSpeakerInfo
	upstream.Request.EnableChannelSplit = metadata.EnableChannelSplit
	upstream.Request.ShowSpeechRate = metadata.ShowSpeechRate
	upstream.Request.ShowVolume = metadata.ShowVolume
	upstream.Request.EnableLID = metadata.EnableLID
	upstream.Request.EnableEmotionDetection = metadata.EnableEmotionDetection
	upstream.Request.EnableGenderDetection = metadata.EnableGenderDetection
	upstream.Request.VadSegment = metadata.VadSegment
	upstream.Request.EndWindowSize = metadata.EndWindowSize
	upstream.Request.OutputZHVariant = metadata.OutputZHVariant
	upstream.Request.Corpus = metadata.Corpus

	body, err := common.Marshal(upstream)
	if err != nil {
		return nil, fmt.Errorf("marshal Seed-ASR request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func parseSeedASRMetadata(form *multipart.Form) (seedASRMetadata, error) {
	var metadata seedASRMetadata
	raw := firstFormValue(form, "metadata")
	if raw == "" {
		return metadata, nil
	}
	var fields map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(raw, &fields); err != nil {
		return metadata, fmt.Errorf("metadata must be a JSON object: %w", err)
	}
	allowed := map[string]bool{
		"enable_itn": true, "enable_punc": true, "enable_ddc": true, "enable_speaker_info": true,
		"enable_channel_split": true, "show_utterances": true, "show_speech_rate": true, "show_volume": true,
		"enable_lid": true, "enable_emotion_detection": true, "enable_gender_detection": true, "vad_segment": true,
		"end_window_size": true, "output_zh_variant": true, "audio_format": true, "codec": true, "rate": true,
		"bits": true, "channel": true, "corpus": true,
	}
	for name := range fields {
		if !allowed[name] {
			return metadata, fmt.Errorf("unsupported Seed-ASR metadata field %q", name)
		}
	}
	if err := common.UnmarshalJsonStr(raw, &metadata); err != nil {
		return metadata, fmt.Errorf("invalid Seed-ASR metadata: %w", err)
	}
	if metadata.EndWindowSize != nil && (*metadata.EndWindowSize < 300 || *metadata.EndWindowSize > 5000) {
		return metadata, errors.New("metadata.end_window_size must be between 300 and 5000")
	}
	if metadata.Rate != nil && *metadata.Rate != 8000 && *metadata.Rate != 16000 {
		return metadata, errors.New("metadata.rate must be 8000 or 16000")
	}
	if metadata.Codec != "" && metadata.Codec != "raw" && metadata.Codec != "opus" {
		return metadata, errors.New("metadata.codec must be raw or opus")
	}
	if metadata.Bits != nil && *metadata.Bits != 16 {
		return metadata, errors.New("metadata.bits must be 16")
	}
	if metadata.Channel != nil && *metadata.Channel != 1 && *metadata.Channel != 2 {
		return metadata, errors.New("metadata.channel must be 1 or 2")
	}
	if corpusJSON, ok := fields["corpus"]; ok {
		var corpusFields map[string]json.RawMessage
		if err := common.Unmarshal(corpusJSON, &corpusFields); err != nil {
			return metadata, fmt.Errorf("metadata.corpus must be a JSON object: %w", err)
		}
		allowedCorpus := map[string]bool{
			"boosting_table_name": true, "boosting_table_id": true, "correct_table_name": true,
			"correct_table_id": true, "context": true,
		}
		for name := range corpusFields {
			if !allowedCorpus[name] {
				return metadata, fmt.Errorf("unsupported Seed-ASR metadata corpus field %q", name)
			}
		}
	}
	return metadata, nil
}

func seedASRFormatFromContentType(contentType string) string {
	contentType, _, _ = strings.Cut(contentType, ";")
	switch strings.TrimSpace(strings.ToLower(contentType)) {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav"
	case "audio/ogg", "application/ogg":
		return "ogg"
	default:
		return ""
	}
}

func firstFormValue(form *multipart.Form, key string) string {
	if values := form.Value[key]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

func valueOr[T any](value *T, fallback T) T {
	if value != nil {
		return *value
	}
	return fallback
}

func seedASRFileEndpoint(action string) string {
	return "https://openspeech.bytedance.com/api/v3/auc/bigmodel/" + action
}

func handleSeedASRResponse(c *gin.Context, submit *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(submit)
	captureSeedASRLogID(c, submit)
	if err := validateSeedASRStatus(submit); err != nil {
		return nil, seedASRAPIError(err, false)
	}

	delays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}
	queryFailures := 0
	for attempt := 0; ; attempt++ {
		delay := delays[min(attempt, len(delays)-1)]
		select {
		case <-c.Request.Context().Done():
			return nil, seedASRAPIError(c.Request.Context().Err(), true)
		case <-time.After(delay):
		}

		response, err := querySeedASR(c, info)
		if err != nil {
			queryFailures++
			if queryFailures < 3 {
				continue
			}
			return nil, seedASRAPIError(err, true)
		}
		queryFailures = 0
		status := response.Header.Get("X-Api-Status-Code")
		captureSeedASRLogID(c, response)
		body, readErr := readSeedASRBody(response.Body)
		service.CloseResponseBodyGracefully(response)
		if readErr != nil {
			return nil, seedASRAPIError(readErr, true)
		}
		if response.StatusCode != http.StatusOK {
			return nil, seedASRAPIError(fmt.Errorf("Seed-ASR query returned HTTP %d: %s", response.StatusCode, common.LocalLogPreview(string(body))), true)
		}
		switch status {
		case "20000001", "20000002":
			continue
		case "20000000", "20000003":
			var result seedASRFileResponse
			if len(body) > 0 {
				if err := common.Unmarshal(body, &result); err != nil {
					return nil, seedASRAPIError(fmt.Errorf("decode Seed-ASR result: %w", err), true)
				}
			}
			if err := writeSeedASRResponse(c, info, &result); err != nil {
				return nil, seedASRAPIError(err, true)
			}
			seconds := int(math.Ceil(float64(result.AudioInfo.Duration) / 1000))
			audioTokens, clamp := common.QuotaRoundChecked(float64(seconds) / 60 * 1000)
			if clamp != nil && info.QuotaClamp == nil {
				info.QuotaClamp = clamp
			}
			return &dto.Usage{
				PromptTokens: audioTokens, TotalTokens: audioTokens, Seconds: seconds,
				PromptTokensDetails: dto.InputTokenDetails{AudioTokens: audioTokens},
			}, nil
		default:
			message := response.Header.Get("X-Api-Message")
			return nil, seedASRAPIError(fmt.Errorf("Seed-ASR query failed (%s): %s", status, message), true)
		}
	}
}

func querySeedASR(c *gin.Context, info *relaycommon.RelayInfo) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, seedASRFileEndpoint("query"), strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := setupSeedASRHeaders(c, req.Header, info, seedASRFileResourceID); err != nil {
		return nil, err
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func validateSeedASRStatus(response *http.Response) error {
	status := response.Header.Get("X-Api-Status-Code")
	if response.StatusCode == http.StatusOK && status == "20000000" {
		return nil
	}
	body, _ := readSeedASRBody(response.Body)
	message := response.Header.Get("X-Api-Message")
	if message == "" {
		message = common.LocalLogPreview(string(body))
	}
	return fmt.Errorf("Seed-ASR submit failed (HTTP %d, status %s): %s", response.StatusCode, status, message)
}

func captureSeedASRLogID(c *gin.Context, response *http.Response) {
	if response == nil {
		return
	}
	if logID := response.Header.Get("X-Tt-Logid"); logID != "" {
		c.Set(common.UpstreamRequestIdKey, logID)
	}
}

func readSeedASRBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, seedASRMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Seed-ASR response: %w", err)
	}
	if int64(len(body)) > seedASRMaxBodyBytes {
		return nil, errors.New("Seed-ASR response is too large")
	}
	return body, nil
}

func writeSeedASRResponse(c *gin.Context, info *relaycommon.RelayInfo, result *seedASRFileResponse) error {
	request, _ := info.Request.(*dto.AudioRequest)
	responseFormat := "json"
	if request != nil && request.ResponseFormat != "" {
		responseFormat = request.ResponseFormat
	}
	switch responseFormat {
	case "json":
		c.JSON(http.StatusOK, dto.AudioResponse{Text: result.Result.Text})
	case "text":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(result.Result.Text))
	case "verbose_json":
		response := dto.WhisperVerboseJSONResponse{Task: "transcribe", Duration: float64(result.AudioInfo.Duration) / 1000, Text: result.Result.Text}
		for i, utterance := range result.Result.Utterances {
			response.Segments = append(response.Segments, dto.Segment{Id: i, Start: float64(utterance.StartTime) / 1000, End: float64(utterance.EndTime) / 1000, Text: utterance.Text})
		}
		c.JSON(http.StatusOK, response)
	case "srt", "vtt":
		var output strings.Builder
		if responseFormat == "vtt" {
			output.WriteString("WEBVTT\n\n")
		}
		for i, utterance := range result.Result.Utterances {
			if responseFormat == "srt" {
				fmt.Fprintf(&output, "%d\n", i+1)
			}
			fmt.Fprintf(&output, "%s --> %s\n%s\n\n", formatSeedASRTimestamp(utterance.StartTime, responseFormat), formatSeedASRTimestamp(utterance.EndTime, responseFormat), utterance.Text)
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(output.String()))
	default:
		return fmt.Errorf("unsupported response_format %q", responseFormat)
	}
	return nil
}

func formatSeedASRTimestamp(milliseconds int64, responseFormat string) string {
	milliseconds = max(milliseconds, 0)
	separator := ','
	if responseFormat == "vtt" {
		separator = '.'
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", milliseconds/3_600_000, milliseconds/60_000%60, milliseconds/1000%60, separator, milliseconds%1000)
}

func seedASRAPIError(err error, submitted bool) *types.NewAPIError {
	options := []types.NewAPIErrorOptions{}
	if submitted {
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, options...)
}
