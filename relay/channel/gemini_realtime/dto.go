package gemini_realtime

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GeminiLiveEvent is a minimal struct for parsing server-to-client WebSocket messages
// from the Gemini Live API. Only the fields needed for usage extraction are defined;
// all other messages are passed through transparently.
type GeminiLiveEvent struct {
	SetupComplete *struct{}            `json:"setupComplete,omitempty"`
	ServerContent *GeminiServerContent `json:"serverContent,omitempty"`
	ToolCall      any                  `json:"toolCall,omitempty"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	GoAway        *GeminiGoAway        `json:"goAway,omitempty"`
}

type GeminiServerContent struct {
	TurnComplete       bool `json:"turnComplete,omitempty"`
	GenerationComplete bool `json:"generationComplete,omitempty"`
	Interrupted        bool `json:"interrupted,omitempty"`
}

type GeminiGoAway struct {
	TimeLeft string `json:"timeLeft,omitempty"`
}

// GeminiUsageMetadata holds token usage reported by the Gemini Live API.
// It can appear alongside any server message.
type GeminiUsageMetadata struct {
	PromptTokenCount      int                  `json:"promptTokenCount"`
	ResponseTokenCount    int                  `json:"responseTokenCount"`
	ThoughtsTokenCount    int                  `json:"thoughtsTokenCount"`
	TotalTokenCount       int                  `json:"totalTokenCount"`
	PromptTokensDetails   []ModalityTokenCount `json:"promptTokensDetails,omitempty"`
	ResponseTokensDetails []ModalityTokenCount `json:"responseTokensDetails,omitempty"`
}

// ModalityTokenCount is a per-modality token breakdown.
type ModalityTokenCount struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

// toRealtimeUsage converts a GeminiUsageMetadata to the standard dto.RealtimeUsage
// used by the billing system.
func (u *GeminiUsageMetadata) ToRealtimeUsage() *dto.RealtimeUsage {
	usage := &dto.RealtimeUsage{
		TotalTokens:  u.TotalTokenCount,
		InputTokens:  u.PromptTokenCount,
		OutputTokens: u.ResponseTokenCount,
	}
	for _, detail := range u.PromptTokensDetails {
		switch detail.Modality {
		case "TEXT":
			usage.InputTokenDetails.TextTokens += detail.TokenCount
		case "AUDIO":
			usage.InputTokenDetails.AudioTokens += detail.TokenCount
		}
	}
	for _, detail := range u.ResponseTokensDetails {
		switch detail.Modality {
		case "TEXT":
			usage.OutputTokenDetails.TextTokens += detail.TokenCount
		case "AUDIO":
			usage.OutputTokenDetails.AudioTokens += detail.TokenCount
		}
	}
	return usage
}
