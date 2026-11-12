package speech

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Run owns the upstream lifetime and translates native audio into the shared
// HTTP contract. No producer goroutine can outlive a disconnected consumer.
func Run(c *gin.Context, info *relaycommon.RelayInfo, r *Request) (result Result, err error) {
	timeout := 5 * time.Minute
	if common.RelayTimeout > 0 {
		timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	original := c.Request
	c.Request = c.Request.WithContext(ctx)
	defer func() { c.Request = original }()
	o := &output{c: c, info: info, r: r}
	switch r.Model {
	case QwenPlus, QwenFlash, QwenNext:
		err = o.qwen()
	case ElevenV4, ElevenTurbo:
		err = o.eleven()
	case SeedTTS, SeedICL:
		err = o.doubao()
	case MiniMaxHD, MiniMaxTurbo:
		err = o.minimax()
	default:
		err = errors.New("unsupported speech model")
	}
	if err == nil && !o.complete {
		err = errors.New("speech stream ended before its final event")
	}
	if err == nil {
		err = o.finish()
	}
	result = o.finalize(err)
	if r.StreamFormat == "sse" && result.Written {
		if err != nil {
			_ = o.event("error", gin.H{"error": gin.H{"message": "Speech generation or delivery failed", "type": "upstream_error"}})
		} else {
			err = o.event("speech.audio.done", gin.H{"status": "completed", "usage": result.Usage, "usage_estimated": info.UsageBillingFallback != "", "billing_fallback": info.UsageBillingFallback})
		}
	}
	return result, err
}

func (o *output) post(path string, body any, headers http.Header) (*http.Response, error) {
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	// The native request contains no credentials; record the exact converted
	// body rather than the client's metadata envelope.
	if o.r.Model == QwenNext && len(o.r.Extensions.References) > 0 {
		logBody := maps.Clone(body.(map[string]any))
		logInput := maps.Clone(logBody["input"].(map[string]any))
		references := append([]any(nil), logInput["references"].([]any)...)
		for i, reference := range references {
			item := maps.Clone(reference.(map[string]any))
			if item["audio_data"] != nil {
				item["audio_data"] = "[audio omitted]"
			}
			references[i] = item
		}
		logInput["references"] = references
		logBody["input"] = logInput
		o.c.Set("speech_request", common.JsonStringify(logBody))
	} else {
		o.c.Set("speech_request", string(data))
	}
	req, err := http.NewRequestWithContext(o.c.Request.Context(), http.MethodPost, o.baseURL()+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.info.ApiKey)
	overrides, err := channel.ResolveHeaderOverride(o.info, o.c)
	if err != nil {
		return nil, err
	}
	for name, value := range overrides {
		req.Header.Set(name, value)
	}
	// Native streaming and downstream SSE are independent. The shared
	// transport's chat pinger must not prepend SSE to raw audio responses.
	transportInfo := *o.info
	transportInfo.IsStream = false
	resp, err := channel.DoRequest(o.c, req, &transportInfo)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("speech upstream returned HTTP %d", resp.StatusCode), types.ErrorCodeBadResponse, resp.StatusCode)
	}
	return resp, nil
}

func (o *output) baseURL() string {
	base := strings.TrimRight(o.info.ChannelBaseUrl, "/")
	if base != "" && base != constant.GetChannelBaseURL(o.info.GetChannelType()) {
		return base
	}
	switch o.r.Model {
	case ElevenV4, ElevenTurbo:
		return "https://api.elevenlabs.io"
	case SeedTTS, SeedICL:
		return "https://openspeech.bytedance.com"
	case MiniMaxHD, MiniMaxTurbo:
		return "https://api.minimax.cn"
	}
	return base
}

// readSSE handles arbitrary HTTP read boundaries, multiline data and comments.
// Audio frames have an explicit bound independent of the request body limit.
func readSSE(reader io.Reader, consume func([]byte) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				if err := consume([]byte(strings.TrimSuffix(data.String(), "\n"))); err != nil {
					return err
				}
				data.Reset()
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len()+len(value) > 16<<20 {
				return errors.New("speech event exceeds 16 MiB")
			}
			data.WriteString(strings.TrimPrefix(value, " "))
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if data.Len() > 0 {
		return consume([]byte(strings.TrimSuffix(data.String(), "\n")))
	}
	return nil
}

func (o *output) qwen() error {
	r, e := o.r, o.r.Extensions
	if o.baseURL() == "" || o.baseURL() == constant.GetChannelBaseURL(o.info.GetChannelType()) {
		return errors.New("Qwen Audio TTS requires a workspace-specific channel Base URL")
	}
	encoded, err := common.Marshal(e)
	if err != nil {
		return err
	}
	input := map[string]any{}
	if err := common.Unmarshal(encoded, &input); err != nil {
		return err
	}
	input["format"] = r.ResponseFormat
	if r.ResponseFormat == "opus" && e.SampleRate == nil {
		input["sample_rate"] = 48000
	}
	if r.Model == QwenNext {
		input["text_prompt"] = r.Input
	} else {
		input["text"], input["voice"] = r.Input, r.Voice
	}
	if r.Speed != nil {
		input["rate"] = *r.Speed
	}
	if r.Instructions != "" {
		input["instruction"] = r.Instructions
	}
	body := map[string]any{"model": r.Model, "input": input}
	mergeNative(body, r.Native)
	headers := http.Header{}
	if r.Stream {
		headers.Set("X-DashScope-SSE", "enable")
	}
	resp, err := o.post("/api/v1/services/audio/tts/SpeechSynthesizer", body, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if r.Stream {
		return readSSE(resp.Body, o.qwenFrame)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 16<<20 {
		return errors.New("speech response exceeds 16 MiB")
	}
	return o.qwenFrame(data)
}

func (o *output) qwenFrame(data []byte) error {
	if string(data) == "[DONE]" {
		return nil
	}
	var frame struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Output    struct {
			FinishReason string          `json:"finish_reason"`
			Type         string          `json:"type"`
			Sentence     json.RawMessage `json:"sentence"`
			Audio        struct {
				Data string `json:"data"`
				URL  string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
		Usage map[string]any `json:"usage"`
	}
	if err := common.Unmarshal(data, &frame); err != nil {
		return err
	}
	if frame.Code != "" {
		return fmt.Errorf("Qwen speech error: %s", frame.Code)
	}
	if frame.RequestID != "" {
		o.c.Set(common.UpstreamRequestIdKey, frame.RequestID)
	}
	if len(frame.Output.Sentence) > 0 && string(frame.Output.Sentence) != "null" {
		if err := o.annotation(frame.Output.Type, frame.Output.Sentence); err != nil {
			return err
		}
	}
	if err := o.usage(frame.Usage, "characters"); err != nil {
		o.info.UsageBillingFallback = "invalid_usage"
		return err
	}
	if frame.Output.Audio.Data != "" {
		audio, err := base64.StdEncoding.Strict().DecodeString(frame.Output.Audio.Data)
		if err != nil {
			return errors.New("invalid Qwen speech base64")
		}
		if err := o.audio(audio); err != nil {
			return err
		}
	}
	if frame.Output.Audio.URL != "" {
		o.url = frame.Output.Audio.URL
		o.result.Generated = true
	}
	if frame.Output.FinishReason == "stop" {
		o.complete = true
	}
	return nil
}

func (o *output) minimax() error {
	r, e := o.r, o.r.Extensions
	voice := map[string]any{"voice_id": r.Voice}
	if r.Speed != nil {
		voice["speed"] = *r.Speed
	}
	if e.Volume != nil {
		voice["vol"] = *e.Volume
	}
	if e.Pitch != nil {
		voice["pitch"] = *e.Pitch
	}
	if e.Emotion != "" {
		voice["emotion"] = e.Emotion
	}
	audio := map[string]any{"format": r.ResponseFormat}
	if e.SampleRate != nil {
		audio["sample_rate"] = *e.SampleRate
	}
	if e.Channels != nil {
		audio["channel"] = *e.Channels
	}
	if e.BitRate != nil {
		audio["bitrate"] = *e.BitRate * 1000
	}
	body := map[string]any{"model": r.Model, "text": r.Input, "stream": r.Stream, "voice_setting": voice, "audio_setting": audio, "output_format": "hex"}
	if r.URL {
		body["output_format"] = "url"
	}
	if r.Stream {
		body["stream_options"] = map[string]any{"exclude_aggregated_audio": true}
	}
	if e.SubtitleEnable != nil {
		body["subtitle_enable"] = *e.SubtitleEnable
	}
	mergeNative(body, r.Native)
	resp, err := o.post("/v1/t2a_v2", body, http.Header{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if r.Stream {
		return readSSE(resp.Body, o.minimaxFrame)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes*2+1))
	if err != nil {
		return err
	}
	if len(data) > maxAudioBytes*2 {
		return errors.New("speech response exceeds its size limit")
	}
	return o.minimaxFrame(data)
}

func (o *output) minimaxFrame(data []byte) error {
	if string(data) == "[DONE]" {
		return nil
	}
	var frame struct {
		Data struct {
			Audio        string          `json:"audio"`
			Status       *int            `json:"status"`
			Subtitle     json.RawMessage `json:"subtitle"`
			Subtitles    json.RawMessage `json:"subtitles"`
			SubtitleFile string          `json:"subtitle_file"`
		} `json:"data"`
		BaseResp *struct {
			StatusCode *int `json:"status_code"`
		} `json:"base_resp"`
		ExtraInfo map[string]any `json:"extra_info"`
		TraceID   string         `json:"trace_id"`
	}
	if err := common.Unmarshal(data, &frame); err != nil {
		return err
	}
	if frame.BaseResp == nil || frame.BaseResp.StatusCode == nil || *frame.BaseResp.StatusCode != 0 {
		return errors.New("MiniMax speech returned a business error or missing status")
	}
	if frame.TraceID != "" {
		o.c.Set(common.UpstreamRequestIdKey, frame.TraceID)
	}
	for kind, data := range map[string]json.RawMessage{"subtitle": frame.Data.Subtitle, "subtitles": frame.Data.Subtitles} {
		if len(data) > 0 && string(data) != "null" {
			if err := o.annotation(kind, data); err != nil {
				return err
			}
		}
	}
	if frame.Data.SubtitleFile != "" {
		if err := o.annotation("subtitle_file", gin.H{"url": frame.Data.SubtitleFile}); err != nil {
			return err
		}
	}
	if err := o.usage(frame.ExtraInfo, "usage_characters"); err != nil {
		o.info.UsageBillingFallback = "invalid_usage"
		return err
	}
	if frame.Data.Audio != "" {
		if strings.HasPrefix(frame.Data.Audio, "https://") || strings.HasPrefix(frame.Data.Audio, "http://") {
			o.url = frame.Data.Audio
			o.result.Generated = true
		} else {
			audio, err := hex.DecodeString(frame.Data.Audio)
			if err != nil {
				return errors.New("invalid MiniMax speech hex audio")
			}
			if err := o.audio(audio); err != nil {
				return err
			}
		}
	}
	if frame.Data.Status != nil && *frame.Data.Status == 2 {
		o.complete = true
	}
	return nil
}
