package qwen_realtime

import (
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
