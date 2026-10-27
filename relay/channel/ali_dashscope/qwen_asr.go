package ali_dashscope

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"github.com/go-audio/wav"
)

const (
	QwenAudioASRModel         = "qwen-audio-3.1-asr-flash"
	QwenAudioASRInputEstimate = 7168
	qwenASRMaxDataURIBytes    = 10_000_000
	qwenASRRequestKey         = "qwen_audio_asr_request"
	QwenASRRequestLogKey      = "qwen_audio_asr_request_log"
)

func IsQwenAudioASRModel(model string) bool { return model == QwenAudioASRModel }

func IsSynchronousASRModel(model string) bool {
	return IsFunASRFlashModel(model) || IsQwenAudioASRModel(model)
}

type qwenASRParameters struct {
	Format             string         `json:"format"`
	SampleRate         *string        `json:"sample_rate,omitempty"`
	SpeakerDiarization *bool          `json:"speaker_diarization_enabled,omitempty"`
	KeepDialect        *bool          `json:"keep_dialect,omitempty"`
	LanguageHints      []string       `json:"language_hints,omitempty"`
	VocabularyID       *string        `json:"vocabulary_id,omitempty"`
	Vocabulary         map[string]int `json:"vocabulary,omitempty"`
}

type qwenASRContent struct {
	Type       string        `json:"type"`
	Text       string        `json:"text,omitempty"`
	InputAudio *qwenASRAudio `json:"input_audio,omitempty"`
}

type qwenASRAudio struct {
	Data string `json:"data"`
}
type qwenASRMessage struct {
	Role    string           `json:"role"`
	Content []qwenASRContent `json:"content"`
}

type QwenAudioASRRequest struct {
	Model string `json:"model"`
	Input struct {
		Messages []qwenASRMessage `json:"messages"`
	} `json:"input"`
	Parameters qwenASRParameters `json:"parameters"`
}

// PrepareQwenAudioASRRequest validates before reservation even when token
// estimation is disabled. Cache the validated payload for conversion/retries.
func PrepareQwenAudioASRRequest(c *gin.Context) (*QwenAudioASRRequest, error) {
	if cached, ok := c.Get(qwenASRRequestKey); ok {
		return cached.(*QwenAudioASRRequest), nil
	}
	request, err := prepareQwenAudioASRRequest(c)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, common.ErrRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		return nil, types.NewErrorWithStatusCode(errors.New(i18n.T(c, "distributor.invalid_request", map[string]any{"Error": err.Error()})), types.ErrorCodeInvalidRequest, status, types.ErrOptionWithSkipRetry())
	}
	c.Set(qwenASRRequestKey, request)
	return request, nil
}

func prepareQwenAudioASRRequest(c *gin.Context) (*QwenAudioASRRequest, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, err
	}
	defer form.RemoveAll()
	if len(form.File) != 1 || len(form.File["file"]) != 1 {
		return nil, errors.New("exactly one audio file is required")
	}
	for key, values := range form.Value {
		if len(values) != 1 {
			return nil, fmt.Errorf("duplicate form field: %s", key)
		}
	}
	for _, key := range []string{"audio_url", "input_url", "input_audio", "input", "messages"} {
		if _, ok := form.Value[key]; ok {
			return nil, fmt.Errorf("%s is unsupported; upload one file", key)
		}
	}
	responseFormat := "json"
	if values := form.Value["response_format"]; len(values) > 0 {
		responseFormat = values[0]
	}
	if !slices.Contains([]string{"", "json", "text", "verbose_json", "dashscope"}, responseFormat) {
		return nil, errors.New("response_format must be json, text, verbose_json or dashscope")
	}
	if values := form.Value["stream_format"]; len(values) > 0 && values[0] != "" {
		return nil, errors.New("streaming is not supported for this ASR model")
	}
	if values := form.Value["stream"]; len(values) > 0 && values[0] != "false" {
		return nil, errors.New("stream must be false for this ASR model")
	}

	request := &QwenAudioASRRequest{Model: QwenAudioASRModel}
	if values := form.Value["parameters"]; len(values) > 0 {
		var fields map[string]common.RawMessage
		if err := common.UnmarshalJsonStr(values[0], &fields); err != nil || fields == nil {
			return nil, errors.New("parameters must be a JSON object")
		}
		for key := range fields {
			if !slices.Contains([]string{"format", "sample_rate", "speaker_diarization_enabled", "keep_dialect", "language_hints", "vocabulary_id", "vocabulary"}, key) {
				return nil, fmt.Errorf("unsupported ASR parameter: %s", key)
			}
			if string(fields[key]) == "null" {
				return nil, fmt.Errorf("ASR parameter %s cannot be null", key)
			}
		}
		if err := common.UnmarshalJsonStr(values[0], &request.Parameters); err != nil {
			return nil, fmt.Errorf("invalid ASR parameters: %w", err)
		}
	}
	p := &request.Parameters
	if values := form.Value["language"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		language := strings.TrimSpace(values[0])
		if len(p.LanguageHints) > 0 && (len(p.LanguageHints) != 1 || p.LanguageHints[0] != language) {
			return nil, errors.New("language conflicts with parameters.language_hints")
		}
		p.LanguageHints = []string{language}
	}
	if len(p.LanguageHints) > 4 {
		return nil, errors.New("language_hints accepts at most four languages")
	}
	for _, language := range p.LanguageHints {
		if !slices.Contains(strings.Fields("zh en ja ko vi th id ms tl hi ar fr de es pt ru it nl sv da fi no el pl cs hu ro bg hr sk"), language) {
			return nil, fmt.Errorf("unsupported ASR language: %s", language)
		}
	}
	if p.SampleRate != nil {
		rate, err := strconv.ParseInt(*p.SampleRate, 10, 32)
		if err != nil || rate <= 0 {
			return nil, errors.New("sample_rate must be a positive integer string")
		}
	}
	if len(p.Vocabulary) > 2000 {
		return nil, errors.New("vocabulary accepts at most 2000 entries")
	}
	superWords := 0
	for word, weight := range p.Vocabulary {
		if strings.TrimSpace(word) == "" || (weight != 50 && (weight < 1 || weight > 5)) {
			return nil, errors.New("vocabulary needs nonempty words with integer weights 1 through 5 or 50")
		}
		if weight == 50 {
			superWords++
		}
	}
	if superWords > 50 {
		return nil, errors.New("vocabulary accepts at most 50 super hotwords")
	}

	if values := form.Value["context_messages"]; len(values) > 0 {
		if _, ok := form.Value["prompt"]; ok {
			return nil, errors.New("prompt and context_messages are mutually exclusive")
		}
		if err := common.UnmarshalJsonStr(values[0], &request.Input.Messages); err != nil {
			return nil, errors.New("context_messages must be a JSON array of text messages")
		}
	}
	if values := form.Value["prompt"]; len(values) > 0 && values[0] != "" {
		request.Input.Messages = []qwenASRMessage{{Role: "user", Content: []qwenASRContent{{Type: "input_text", Text: values[0]}}}}
	}
	if err := validateQwenASRContext(request.Input.Messages); err != nil {
		return nil, err
	}

	fileHeader := form.File["file"][0]
	// Bound before multiplication/base64 allocation; include the URI prefix below.
	if fileHeader.Size > qwenASRMaxDataURIBytes/4*3 {
		return nil, common.ErrRequestBodyTooLarge
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, qwenASRMaxDataURIBytes/4*3+1))
	if err != nil {
		return nil, err
	}
	if len(data) > qwenASRMaxDataURIBytes/4*3 {
		return nil, common.ErrRequestBodyTooLarge
	}
	detected := mimetype.Detect(data)
	format := strings.TrimPrefix(detected.Extension(), ".")
	if format == "oga" {
		format = "ogg"
	}
	if format == "ogg" && bytes.Contains(data[:min(len(data), 512)], []byte("OpusHead")) {
		format = "opus"
	}
	if !slices.Contains([]string{"wav", "mp3", "flac", "m4a", "mp4", "ogg", "opus", "aac"}, format) {
		return nil, errors.New("unsupported audio content; use wav, mp3, flac, m4a, mp4, ogg, opus or aac")
	}
	if p.Format != "" && p.Format != format && !(format == "opus" && p.Format == "ogg") {
		return nil, errors.New("parameters.format does not match audio content")
	}
	if p.Format == "" {
		p.Format = format
	}
	if format == "wav" {
		reader := bytes.NewReader(data)
		decoder := wav.NewDecoder(reader)
		if !decoder.IsValidFile() || decoder.FwdToPCM() != nil {
			return nil, errors.New("invalid WAV audio")
		}
		offset, err := reader.Seek(0, io.SeekCurrent)
		if err != nil || int64(decoder.PCMSize) > int64(len(data))-offset {
			return nil, errors.New("truncated WAV audio")
		}
		if p.SampleRate != nil && *p.SampleRate != strconv.Itoa(int(decoder.SampleRate)) {
			return nil, errors.New("sample_rate does not match WAV audio")
		}
	}
	duration, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(data), "."+format)
	if err != nil {
		return nil, fmt.Errorf("invalid audio: %w", err)
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 300 {
		return nil, errors.New("audio duration must be greater than zero and at most 300 seconds")
	}
	prefix := "data:" + detected.String() + ";base64,"
	if len(prefix)+base64.StdEncoding.EncodedLen(len(data)) > qwenASRMaxDataURIBytes {
		return nil, common.ErrRequestBodyTooLarge
	}
	request.Input.Messages = append(request.Input.Messages, qwenASRMessage{Role: "user", Content: []qwenASRContent{{Type: "input_audio", InputAudio: &qwenASRAudio{Data: "[audio omitted]"}}}})
	if os.Getenv("SAVE_REQUEST_RESPONSE") == "true" {
		c.Set(QwenASRRequestLogKey, common.JsonStringify(request))
	}
	request.Input.Messages[len(request.Input.Messages)-1].Content[0].InputAudio.Data = prefix + base64.StdEncoding.EncodeToString(data)
	return request, nil
}

func validateQwenASRContext(messages []qwenASRMessage) error {
	users, assistants, roundLength := 0, 0, 0
	previousRole := ""
	for _, message := range messages {
		kind := "input_text"
		switch message.Role {
		case "user":
			users++
			roundLength = 0
		case "assistant":
			if previousRole != "user" {
				return errors.New("assistant context must follow a user context message")
			}
			assistants++
			kind = "text"
		default:
			return errors.New("context roles must be user or assistant")
		}
		if len(message.Content) == 0 {
			return errors.New("context message content cannot be empty")
		}
		for _, content := range message.Content {
			if content.Type != kind || content.InputAudio != nil || !utf8.ValidString(content.Text) {
				return errors.New("context must contain only input_text for user or text for assistant")
			}
			roundLength += utf8.RuneCountInString(content.Text)
		}
		if roundLength > 400 || users > 5 || assistants > 5 {
			return errors.New("context allows five rounds and 400 characters per round")
		}
		previousRole = message.Role
	}
	return nil
}

type qwenASRWord struct {
	Text        string `json:"text"`
	Punctuation string `json:"punctuation"`
	BeginTime   *int64 `json:"begin_time"`
	EndTime     *int64 `json:"end_time"`
	SpeakerID   *int   `json:"speaker_id"`
	Fixed       *bool  `json:"fixed"`
}

type qwenASRSentence struct {
	Text        string        `json:"text"`
	SentenceID  int           `json:"sentence_id"`
	ChannelID   int           `json:"channel_id"`
	SentenceEnd bool          `json:"sentence_end"`
	BeginTime   *int64        `json:"begin_time"`
	EndTime     *int64        `json:"end_time"`
	SpeakerID   *int          `json:"speaker_id"`
	Words       []qwenASRWord `json:"words"`
}

func qwenAudioASRHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)
	body, err := readASRBody(resp.Body)
	if err != nil {
		return asrAPIError(errors.New(i18n.T(c, i18n.MsgRelayASRResponseInvalid, map[string]any{"Error": err.Error()}))), nil
	}
	var result struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Output    *struct {
			Text      *string           `json:"text"`
			Sentence  *qwenASRSentence  `json:"sentence"`
			Sentences []qwenASRSentence `json:"sentences"`
		} `json:"output"`
		Usage common.RawMessage `json:"usage"`
	}
	if err := common.Unmarshal(body, &result); err != nil {
		return asrAPIError(errors.New(i18n.T(c, i18n.MsgRelayASRResponseInvalid, map[string]any{"Error": "invalid Qwen ASR JSON response"}))), nil
	}
	if result.RequestID != "" {
		c.Set(common.UpstreamRequestIdKey, result.RequestID)
	}
	if result.Code != "" {
		return asrAPIError(errors.New(i18n.T(c, i18n.MsgRelayASRResponseInvalid, map[string]any{"Error": fmt.Sprintf("%s: %s", result.Code, result.Message)}))), nil
	}
	if result.Output == nil || result.Output.Text == nil {
		return asrAPIError(errors.New(i18n.T(c, i18n.MsgRelayASRResponseInvalid, map[string]any{"Error": "Qwen ASR response is missing output.text"}))), nil
	}
	if os.Getenv("SAVE_REQUEST_RESPONSE") == "true" {
		common.SetContextKey(c, constant.ContextKeySdkResponseStr, string(body))
	}
	var upstreamUsage struct {
		Input    *int64   `json:"input_tokens"`
		Output   *int64   `json:"output_tokens"`
		Total    *int64   `json:"total_tokens"`
		Duration *float64 `json:"duration"`
	}
	usage := &dto.Usage{UsageSource: "dashscope", UsageSemantic: "openai"}
	info.UsageBillingFallback = ""
	if len(result.Usage) == 0 || string(result.Usage) == "null" {
		info.UsageBillingFallback = "missing_usage"
	} else if err := common.Unmarshal(result.Usage, &upstreamUsage); err != nil {
		info.UsageBillingFallback = "invalid_usage"
	} else if upstreamUsage.Input == nil || upstreamUsage.Output == nil {
		info.UsageBillingFallback = "missing_usage"
	} else if *upstreamUsage.Input < 0 || *upstreamUsage.Output < 0 || *upstreamUsage.Input > common.MaxQuota || *upstreamUsage.Output > common.MaxQuota-*upstreamUsage.Input {
		info.UsageBillingFallback = "invalid_usage"
	} else {
		usage.PromptTokens = int(*upstreamUsage.Input)
		usage.CompletionTokens = int(*upstreamUsage.Output)
		usage.InputTokens, usage.OutputTokens = usage.PromptTokens, usage.CompletionTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		usage.CompletionTokenDetails.TextTokens = usage.CompletionTokens
		if upstreamUsage.Total != nil && *upstreamUsage.Total != int64(usage.TotalTokens) {
			logger.LogWarn(c, common.LogText("Qwen ASR total_tokens does not match input_tokens plus output_tokens"))
		}
	}
	duration := 0.0
	if upstreamUsage.Duration != nil && *upstreamUsage.Duration >= 0 && *upstreamUsage.Duration <= 300 {
		duration = *upstreamUsage.Duration
		usage.Seconds = common.QuotaRound(math.Ceil(duration))
	}
	if info.UsageBillingFallback != "" {
		logger.LogWarn(c, common.LogText("Qwen ASR %s; retaining reserved charge without reporting estimated tokens", info.UsageBillingFallback))
	}
	request, _ := info.Request.(*dto.AudioRequest)
	format := "json"
	if request != nil && request.ResponseFormat != "" {
		format = request.ResponseFormat
	}
	switch format {
	case "dashscope":
		c.Data(http.StatusOK, "application/json", body)
	case "text":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(*result.Output.Text))
	case "verbose_json":
		sentences := result.Output.Sentences
		if len(sentences) == 0 && result.Output.Sentence != nil {
			sentences = []qwenASRSentence{*result.Output.Sentence}
		}
		segments := make([]map[string]any, 0, len(sentences))
		indices := make(map[[2]int]int)
		for _, sentence := range sentences {
			key := [2]int{sentence.ChannelID, sentence.SentenceID}
			index, exists := indices[key]
			if !exists {
				index = len(segments)
				indices[key] = index
			}
			segment := map[string]any{"id": index, "sentence_id": sentence.SentenceID, "channel_id": sentence.ChannelID, "sentence_end": sentence.SentenceEnd, "speaker_id": sentence.SpeakerID, "text": sentence.Text}
			if sentence.BeginTime != nil {
				segment["start"] = float64(*sentence.BeginTime) / 1000
			}
			if sentence.EndTime != nil {
				segment["end"] = float64(*sentence.EndTime) / 1000
			}
			words := make([]map[string]any, 0, len(sentence.Words))
			for _, word := range sentence.Words {
				item := map[string]any{"word": word.Text, "punctuation": word.Punctuation, "speaker_id": word.SpeakerID}
				if word.BeginTime != nil {
					item["start"] = float64(*word.BeginTime) / 1000
				}
				if word.EndTime != nil {
					item["end"] = float64(*word.EndTime) / 1000
				}
				if word.Fixed != nil {
					item["fixed"] = *word.Fixed
				}
				words = append(words, item)
			}
			segment["words"] = words
			if exists {
				segments[index] = segment
			} else {
				segments = append(segments, segment)
			}
		}
		response := gin.H{"task": "transcribe", "text": *result.Output.Text, "segments": segments}
		if upstreamUsage.Duration != nil && *upstreamUsage.Duration >= 0 && *upstreamUsage.Duration <= 300 {
			response["duration"] = duration
		}
		c.JSON(http.StatusOK, response)
	case "", "json":
		c.JSON(http.StatusOK, dto.AudioResponse{Text: *result.Output.Text})
	default:
		return asrAPIError(errors.New(i18n.T(c, i18n.MsgRelayASRResponseInvalid, map[string]any{"Error": "unsupported Qwen ASR response format"}))), nil
	}
	return nil, usage
}
