package qwen_realtime

import (
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Qwen Realtime client→server event types
const (
	QwenEventSessionUpdate          = "session.update"
	QwenEventInputAudioBufferAppend = "input_audio_buffer.append"
	QwenEventInputImageBufferAppend = "input_image_buffer.append"
	QwenEventInputAudioBufferCommit = "input_audio_buffer.commit"
	QwenEventInputAudioBufferClear  = "input_audio_buffer.clear"
	QwenEventResponseCreate         = "response.create"
	QwenEventResponseCancel         = "response.cancel"
)

// Qwen Realtime server→client event types
const (
	QwenEventSessionCreated     = "session.created"
	QwenEventSessionUpdated     = "session.updated"
	QwenEventResponseDone       = "response.done"
	QwenEventError              = "error"
	QwenEventResponseAudioDelta = "response.audio.delta"
)

// QwenRealtimeEvent is used to parse downstream WebSocket messages.
type QwenRealtimeEvent struct {
	EventId  string                `json:"event_id"`
	Type     string                `json:"type"`
	Response *QwenRealtimeResponse `json:"response,omitempty"`
	Error    *QwenRealtimeError    `json:"error,omitempty"`
}

// QwenRealtimeResponse represents the response field in a response.done event.
type QwenRealtimeResponse struct {
	Usage *QwenRealtimeUsage `json:"usage,omitempty"`
}

// raw usage: {"total_tokens": 622, "input_tokens": 532, "output_tokens": 90, "input_tokens_details": {"text_tokens": 314, "audio_tokens": 75, "video_tokens": 143}, "output_tokens_details": {"text_tokens": 25, "audio_tokens": 65}}
// QwenRealtimeUsage holds token usage from the upstream response.
type QwenRealtimeUsage struct {
	TotalTokens         int               `json:"total_tokens"`
	InputTokens         int               `json:"input_tokens"`
	OutputTokens        int               `json:"output_tokens"`
	InputTokensDetails  *QwenTokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *QwenTokenDetails `json:"output_tokens_details,omitempty"`
}

// QwenTokenDetails holds modality-level token breakdown.
type QwenTokenDetails struct {
	TextTokens  int `json:"text_tokens"`
	AudioTokens int `json:"audio_tokens"`
	VideoTokens int `json:"video_tokens,omitempty"`
}

// QwenRealtimeError represents an error event from the upstream.
type QwenRealtimeError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type asrRealtimeEvent struct {
	Header struct {
		TaskID string `json:"task_id"`
		Event  string `json:"event"`
	} `json:"header"`
	Payload struct {
		Usage *struct {
			Duration float64 `json:"duration"`
		} `json:"usage"`
	} `json:"payload"`
}

func isFunASRRealtimeModel(model string) bool {
	return strings.HasPrefix(model, "fun-asr") || strings.HasPrefix(model, "qwen-audio-3.0-asr-flash-streaming")
}

func isQwen3ASRRealtimeModel(model string) bool {
	return strings.HasPrefix(model, "qwen3-asr-flash-realtime")
}

func isASRRealtimeModel(model string) bool {
	return isFunASRRealtimeModel(model) || isQwen3ASRRealtimeModel(model)
}

type qwen3ASRClientEvent struct {
	Type    string `json:"type"`
	Audio   string `json:"audio,omitempty"`
	Session *struct {
		InputAudioFormat string `json:"input_audio_format,omitempty"`
		SampleRate       int    `json:"sample_rate,omitempty"`
	} `json:"session,omitempty"`
}

type qwen3ASRUsageTracker struct {
	sampleRate       int
	inputAudioFormat string
	totalSeconds     float64
	accountedTokens  int
}

func newQwen3ASRUsageTracker() *qwen3ASRUsageTracker {
	return &qwen3ASRUsageTracker{sampleRate: 16000, inputAudioFormat: "pcm"}
}

func (t *qwen3ASRUsageTracker) consume(message []byte) (*dto.RealtimeUsage, *common.QuotaClamp, error) {
	var event qwen3ASRClientEvent
	if err := common.Unmarshal(message, &event); err != nil {
		return nil, nil, err
	}

	if event.Type == QwenEventSessionUpdate && event.Session != nil {
		if event.Session.SampleRate != 0 {
			switch event.Session.SampleRate {
			case 8000, 16000:
				t.sampleRate = event.Session.SampleRate
			default:
				return nil, nil, fmt.Errorf("unsupported Qwen3 ASR sample_rate %d", event.Session.SampleRate)
			}
		}
		if event.Session.InputAudioFormat != "" {
			format := strings.ToLower(event.Session.InputAudioFormat)
			switch format {
			case "pcm", "pcm16", "pcm_s16le":
				t.inputAudioFormat = format
			default:
				return nil, nil, fmt.Errorf("unsupported Qwen3 ASR input_audio_format %q", event.Session.InputAudioFormat)
			}
		}
		return nil, nil, nil
	}
	if event.Type != QwenEventInputAudioBufferAppend || event.Audio == "" {
		return nil, nil, nil
	}
	if t.inputAudioFormat != "pcm" && t.inputAudioFormat != "pcm16" && t.inputAudioFormat != "pcm_s16le" {
		return nil, nil, fmt.Errorf("cannot bill Qwen3 ASR audio format %q", t.inputAudioFormat)
	}

	decodedBytes, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(event.Audio)))
	if err != nil {
		return nil, nil, fmt.Errorf("decode Qwen3 ASR PCM audio: %w", err)
	}
	if decodedBytes%2 != 0 {
		return nil, nil, fmt.Errorf("Qwen3 ASR PCM audio must contain complete 16-bit samples")
	}
	t.totalSeconds += float64(decodedBytes) / float64(t.sampleRate*2)
	tokens, clamp := common.QuotaRoundChecked(math.Ceil(t.totalSeconds) / 60 * 1000)
	if tokens <= t.accountedTokens {
		return nil, clamp, nil
	}
	delta := tokens - t.accountedTokens
	t.accountedTokens = tokens
	return &dto.RealtimeUsage{
		TotalTokens: delta,
		InputTokens: delta,
		InputTokenDetails: dto.InputTokenDetails{
			AudioTokens: delta,
		},
	}, clamp, nil
}

func rewriteASRRunTaskModel(message []byte, model string) ([]byte, error) {
	var event map[string]any
	if err := common.Unmarshal(message, &event); err != nil {
		return message, err
	}
	header, _ := event["header"].(map[string]any)
	if header["action"] != "run-task" {
		return message, nil
	}
	payload, ok := event["payload"].(map[string]any)
	if !ok {
		return message, nil
	}
	payload["model"] = model
	return common.Marshal(event)
}

func asrUsageDelta(message []byte, accounted map[string]int) (*dto.RealtimeUsage, *common.QuotaClamp, error) {
	var event asrRealtimeEvent
	if err := common.Unmarshal(message, &event); err != nil {
		return nil, nil, err
	}
	if event.Header.Event != "result-generated" || event.Payload.Usage == nil {
		return nil, nil, nil
	}
	tokens, clamp := common.QuotaRoundChecked(math.Ceil(max(event.Payload.Usage.Duration, 0)) / 60 * 1000)
	previous := accounted[event.Header.TaskID]
	if tokens <= previous {
		return nil, clamp, nil
	}
	accounted[event.Header.TaskID] = tokens
	delta := tokens - previous
	return &dto.RealtimeUsage{
		TotalTokens: delta,
		InputTokens: delta,
		InputTokenDetails: dto.InputTokenDetails{
			AudioTokens: delta,
		},
	}, clamp, nil
}

// accumulateUsage adds source QwenRealtimeUsage into the target dto.RealtimeUsage.
// It safely handles nil InputTokensDetails and OutputTokensDetails.
func accumulateUsage(target *dto.RealtimeUsage, source *QwenRealtimeUsage) {
	target.TotalTokens += source.TotalTokens
	target.InputTokens += source.InputTokens
	target.OutputTokens += source.OutputTokens
	if source.InputTokensDetails != nil {
		target.InputTokenDetails.TextTokens += source.InputTokensDetails.TextTokens
		target.InputTokenDetails.AudioTokens += source.InputTokensDetails.AudioTokens
		target.InputTokenDetails.VideoTokens += source.InputTokensDetails.VideoTokens
	}
	if source.OutputTokensDetails != nil {
		target.OutputTokenDetails.TextTokens += source.OutputTokensDetails.TextTokens
		target.OutputTokenDetails.AudioTokens += source.OutputTokensDetails.AudioTokens
	}
}
