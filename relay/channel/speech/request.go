// Package speech implements the HTTP speech contract shared by native TTS providers.
package speech

import (
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	QwenPlus     = "qwen-audio-3.1-tts-plus"
	QwenFlash    = "qwen-audio-3.1-tts-flash"
	QwenNext     = "qwen-audio-3.1-tts-next"
	ElevenV4     = "eleven_v4"
	ElevenTurbo  = "eleven_v4_turbo"
	SeedTTS      = "seed-tts-2.0"
	SeedICL      = "seed-icl-2.0"
	MiniMaxHD    = "speech-2.8-hd"
	MiniMaxTurbo = "speech-2.8-turbo"
)

var ErrICLSystemVoice = errors.New("seed-icl-2.0 requires an account-owned cloned voice; system voices use seed-tts-2.0")

func IsModel(model string) bool {
	return slices.Contains([]string{QwenPlus, QwenFlash, QwenNext, ElevenV4, ElevenTurbo, SeedTTS, SeedICL, MiniMaxHD, MiniMaxTurbo}, model)
}

func Supports(channelType int, model string) bool {
	switch model {
	case QwenPlus, QwenFlash, QwenNext:
		return channelType == constant.ChannelTypeAli || channelType == constant.ChannelTypeAliDashScope
	case ElevenV4, ElevenTurbo:
		return channelType == constant.ChannelTypeElevenLabs
	case SeedTTS, SeedICL:
		return channelType == constant.ChannelTypeVolcEngine
	case MiniMaxHD, MiniMaxTurbo:
		return channelType == constant.ChannelTypeMiniMax
	}
	return false
}

// Extensions is the validated view of known controls. Native retains the full
// provider request patch, including extensions not declared in this struct.
type Extensions struct {
	SampleRate     *int        `json:"sample_rate,omitempty"`
	Volume         *float64    `json:"volume,omitempty"`
	Pitch          *float64    `json:"pitch,omitempty"`
	Seed           *int        `json:"seed,omitempty"`
	BitRate        *int        `json:"bit_rate,omitempty"`
	Channels       *int        `json:"channels,omitempty"`
	Quality        *int        `json:"quality,omitempty"`
	EnableCBR      *bool       `json:"enable_cbr,omitempty"`
	AIGC           *bool       `json:"enable_aigc_tag,omitempty"`
	EnableSSML     *bool       `json:"enable_ssml,omitempty"`
	WordTimestamp  *bool       `json:"word_timestamp_enabled,omitempty"`
	LanguageHints  []string    `json:"language_hints,omitempty"`
	References     []Reference `json:"references,omitempty"`
	Emotion        string      `json:"emotion,omitempty"`
	SubtitleEnable *bool       `json:"subtitle_enable,omitempty"`
}

type Reference struct {
	AudioURL  string `json:"audio_url,omitempty"`
	AudioData string `json:"audio_data,omitempty"`
}

type Request struct {
	dto.AudioRequest
	Extensions Extensions
	Native     map[string]any
	Characters int
	Stream     bool
	URL        bool
}

// Prepare validates the final mapped request before any upstream call.
func Prepare(input *dto.AudioRequest) (*Request, error) {
	r := &Request{AudioRequest: *input}
	if !IsModel(r.Model) {
		return nil, errors.New("unsupported speech model")
	}
	if !utf8.ValidString(r.Input) || strings.TrimSpace(r.Input) == "" {
		return nil, errors.New("input must contain valid UTF-8 text")
	}
	if err := r.prepareNativeMetadata(); err != nil {
		return nil, err
	}
	if r.ResponseFormat == "" {
		r.ResponseFormat = "mp3"
		if r.Model == QwenNext {
			r.ResponseFormat = "wav"
		}
	}
	if !slices.Contains([]string{"", "audio", "url"}, r.ResponseMode) {
		return nil, errors.New("response_mode must be audio or url")
	}
	if !slices.Contains([]string{"", "audio", "sse"}, r.StreamFormat) {
		return nil, errors.New("stream_format must be audio or sse")
	}
	r.URL = r.ResponseMode == "url" || (r.Model == SeedTTS && r.ResponseMode == "" && r.StreamFormat == "")
	if r.URL && r.StreamFormat != "" {
		return nil, errors.New("response_mode=url cannot be combined with stream_format")
	}
	r.Stream = !r.URL && r.Model != QwenNext
	if r.Speed != nil && (math.IsNaN(*r.Speed) || math.IsInf(*r.Speed, 0) || *r.Speed < .5 || *r.Speed > 2) {
		return nil, errors.New("speed must be between 0.5 and 2")
	}
	if r.Model == QwenNext {
		if r.Voice != "" || r.Instructions != "" || r.StreamFormat == "sse" {
			return nil, errors.New("Qwen Next does not support voice, instructions or SSE")
		}
	} else if strings.TrimSpace(r.Voice) == "" {
		if _, blended := r.Native["timbre_weights"]; !blended || (r.Model != MiniMaxHD && r.Model != MiniMaxTurbo) {
			return nil, errors.New("voice is required")
		}
	}
	if r.Model == SeedICL && strings.HasSuffix(r.Voice, "_bigtts") && (strings.HasPrefix(r.Voice, "zh_") || strings.HasPrefix(r.Voice, "en_") || strings.HasPrefix(r.Voice, "multi_")) {
		return nil, ErrICLSystemVoice
	}
	if (r.Model == ElevenV4 || r.Model == ElevenTurbo) && (r.Speed != nil || r.Instructions != "") {
		return nil, errors.New("Eleven v4 models do not support top-level speed or instructions")
	}
	if (r.Model == MiniMaxHD || r.Model == MiniMaxTurbo || r.Model == SeedICL) && r.Instructions != "" {
		return nil, errors.New("instructions are not supported by this speech model")
	}
	e := &r.Extensions
	formats := []string{"mp3", "wav", "pcm", "opus"}
	rates := []int{8000, 12000, 16000, 22050, 24000, 44100, 48000}
	volumeMax, pitchMin, pitchMax := 100.0, .5, 2.0
	limit := 10000
	switch r.Model {
	case QwenNext:
		formats, rates, limit = []string{"mp3", "wav", "pcm"}, []int{8000, 16000, 24000, 44100, 48000}, 3000
		if len(e.References) > 3 {
			return nil, errors.New("at most three reference recordings are allowed")
		}
		// References are forwarded for provider-side decoding, with a strict
		// encoded size bound. The gateway never fetches caller reference URLs.
		for _, ref := range e.References {
			if (ref.AudioURL == "") == (ref.AudioData == "") {
				return nil, errors.New("reference requires exactly one of audio_url and audio_data")
			}
			if ref.AudioURL != "" {
				if err := common.ValidateRedirectURL(ref.AudioURL); err != nil {
					return nil, errors.New("invalid reference audio URL")
				}
			}
			if len(ref.AudioData) > 14*1024*1024 || (ref.AudioData != "" && !strings.HasPrefix(ref.AudioData, "data:audio/")) {
				return nil, errors.New("invalid reference audio data")
			}
			if ref.AudioData != "" {
				prefix, data, ok := strings.Cut(ref.AudioData, ";base64,")
				if !ok || !slices.Contains([]string{"data:audio/wav", "data:audio/mpeg", "data:audio/mp3", "data:audio/ogg"}, prefix) {
					return nil, errors.New("reference audio must be WAV, MP3 or OGG Opus")
				}
				decoded, err := base64.StdEncoding.Strict().DecodeString(data)
				if err != nil || len(decoded) == 0 || len(decoded) > 10*1024*1024 {
					return nil, errors.New("invalid reference audio or reference exceeds 10 MiB")
				}
			}
		}
	case ElevenV4, ElevenTurbo:
		formats, limit = []string{"mp3"}, 5000
	case SeedTTS, SeedICL:
		formats, rates = []string{"mp3", "pcm", "opus"}, []int{8000, 16000, 22050, 24000, 32000, 44100, 48000}
	case MiniMaxHD, MiniMaxTurbo:
		formats, rates = []string{"mp3", "wav", "flac"}, []int{8000, 16000, 22050, 24000, 32000, 44100}
		volumeMax, pitchMin, pitchMax = 10, -12, 12
		r.Stream = !r.URL && r.ResponseFormat == "mp3"
		if r.StreamFormat == "sse" && r.ResponseFormat != "mp3" {
			return nil, errors.New("MiniMax streaming requires mp3")
		}
	}
	if !slices.Contains(formats, r.ResponseFormat) {
		return nil, errors.New("unsupported speech audio format")
	}
	if e.SampleRate != nil && !slices.Contains(rates, *e.SampleRate) {
		return nil, errors.New("unsupported speech sample_rate")
	}
	if r.ResponseFormat == "opus" && (r.Model == SeedTTS || r.Model == SeedICL) && e.SampleRate != nil && *e.SampleRate != 48000 {
		return nil, errors.New("opus requires sample_rate=48000")
	}
	if r.ResponseFormat == "opus" && (r.Model == QwenPlus || r.Model == QwenFlash) && e.SampleRate != nil && slices.Contains([]int{22050, 44100}, *e.SampleRate) {
		return nil, errors.New("Qwen opus does not support this sample_rate")
	}
	if e.Volume != nil && (*e.Volume < 0 || *e.Volume > volumeMax) {
		return nil, errors.New("volume is outside the supported range")
	}
	if e.Volume != nil && (r.Model == QwenPlus || r.Model == QwenFlash || r.Model == QwenNext) && math.Trunc(*e.Volume) != *e.Volume {
		return nil, errors.New("Qwen volume must be an integer")
	}
	if e.Pitch != nil && (r.Model == MiniMaxHD || r.Model == MiniMaxTurbo) && math.Trunc(*e.Pitch) != *e.Pitch {
		return nil, errors.New("MiniMax pitch must be an integer")
	}
	if e.Pitch != nil && (*e.Pitch < pitchMin || *e.Pitch > pitchMax) {
		return nil, errors.New("pitch is outside the supported range")
	}
	if e.Seed != nil && (*e.Seed < 0 || *e.Seed > 65535) {
		return nil, errors.New("seed must be between 0 and 65535")
	}
	if e.Channels != nil && *e.Channels != 1 && *e.Channels != 2 {
		return nil, errors.New("channels must be 1 or 2")
	}
	if e.Quality != nil && (*e.Quality < 0 || *e.Quality > 9) {
		return nil, errors.New("quality must be between 0 and 9")
	}
	if e.BitRate != nil {
		if r.Model == QwenPlus || r.Model == QwenFlash {
			if *e.BitRate < 6 || *e.BitRate > 510 || r.ResponseFormat != "opus" {
				return nil, errors.New("Qwen bit_rate requires opus and a value between 6 and 510 kbps")
			}
		} else if *e.BitRate < 8 || *e.BitRate > 320 || r.ResponseFormat != "mp3" {
			return nil, errors.New("bit_rate requires mp3 and a value between 8 and 320 kbps")
		}
	}
	if e.WordTimestamp != nil && *e.WordTimestamp && !r.Stream {
		return nil, errors.New("word timestamps require streaming")
	}
	if err := r.validateNativeSynthesis(); err != nil {
		return nil, err
	}
	text := r.Input
	if e.EnableSSML != nil && *e.EnableSSML {
		decoder := xml.NewDecoder(strings.NewReader(text))
		var spoken strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, errors.New("invalid SSML")
			}
			if data, ok := token.(xml.CharData); ok {
				spoken.Write(data)
			}
		}
		text = spoken.String()
	}
	expanded, err := r.estimatedSynthesisText(text, limit)
	if err != nil {
		return nil, err
	}
	for _, estimate := range []string{text, expanded} {
		characters := 0
		for _, char := range estimate {
			characters++
			if (r.Model == QwenPlus || r.Model == MiniMaxHD || r.Model == MiniMaxTurbo) && unicode.Is(unicode.Han, char) {
				characters++
			}
		}
		r.Characters = max(r.Characters, characters)
	}
	if r.Characters >= limit && (r.Model == MiniMaxHD || r.Model == MiniMaxTurbo) || utf8.RuneCountInString(r.Input) > limit {
		return nil, fmt.Errorf("speech input exceeds the %d character limit", limit)
	}
	return r, nil
}

func (r *Request) CharacterBilling() bool {
	return r.Model != QwenFlash && r.Model != QwenNext
}
