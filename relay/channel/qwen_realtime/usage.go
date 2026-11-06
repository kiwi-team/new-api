package qwen_realtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func isQwenAudio31Model(model string) bool {
	return model == "qwen-audio-3.1-realtime-plus" || model == "qwen-audio-3.1-realtime-flash"
}

func isBeijingQwenRealtimeModel(model string) bool {
	return model == "qwen3.8-omni-flash-realtime" || isQwenAudio31Model(model)
}

// Validate the configuration without rebuilding the event: native tools, VAD,
// voice precedence, extensions and explicit zero/false values stay upstream-owned.
func validateQwenRealtimeClientEvent(message []byte) error {
	var event struct {
		Type     string          `json:"type"`
		Session  json.RawMessage `json:"session"`
		Response json.RawMessage `json:"response"`
	}
	if err := common.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("invalid Qwen realtime client event")
	}
	var configData json.RawMessage
	switch event.Type {
	case QwenEventSessionUpdate:
		configData = event.Session
	case QwenEventResponseCreate:
		configData = event.Response
	default:
		return nil
	}
	if len(configData) == 0 {
		return nil
	}
	var config struct {
		EnableSearch            *bool           `json:"enable_search"`
		InputAudioFormat        *string         `json:"input_audio_format"`
		OutputAudioFormat       *string         `json:"output_audio_format"`
		SampleRate              *int            `json:"sample_rate"`
		MaxResponseOutputTokens json.RawMessage `json:"max_response_output_tokens"`
		MaxOutputTokens         json.RawMessage `json:"max_output_tokens"`
	}
	if err := common.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("invalid Qwen realtime configuration")
	}
	// Audio Realtime search has no verified published surcharge contract yet.
	// Reject enabling it before inference instead of forwarding an unbilled tool.
	if config.EnableSearch != nil && *config.EnableSearch {
		return fmt.Errorf("Qwen realtime web search is unavailable until search surcharge billing is supported")
	}
	for _, format := range []*string{config.InputAudioFormat, config.OutputAudioFormat} {
		if format != nil && *format != "pcm" && *format != "pcm16" {
			return fmt.Errorf("Qwen realtime audio format must be PCM")
		}
	}
	if config.SampleRate != nil && *config.SampleRate != 16000 {
		return fmt.Errorf("Qwen realtime input sample_rate must be 16000")
	}
	for _, limit := range []json.RawMessage{config.MaxResponseOutputTokens, config.MaxOutputTokens} {
		if len(limit) == 0 || string(limit) == "null" || string(limit) == `"inf"` {
			continue
		}
		var value uint
		if err := common.Unmarshal(limit, &value); err != nil || helper.ExceedsMaxTokensLimit(&value) {
			return fmt.Errorf("Qwen realtime output token limit is outside the supported range")
		}
	}
	return nil
}

// Qwen reports inclusive per-response totals, not session deltas. Validate
// before summing so invalid upstream numbers cannot credit or wrap a charge.
func (u *QwenRealtimeUsage) realtimeUsage() (*dto.RealtimeUsage, error) {
	if u == nil || u.InputTokens == nil || u.OutputTokens == nil {
		return nil, fmt.Errorf("missing Qwen input/output usage totals")
	}
	for _, count := range []*int{u.TotalTokens, u.InputTokens, u.OutputTokens} {
		if count != nil && (*count < 0 || *count > common.MaxQuota) {
			return nil, fmt.Errorf("Qwen usage total is outside the supported range")
		}
	}
	if *u.InputTokens > common.MaxQuota-*u.OutputTokens {
		return nil, fmt.Errorf("Qwen usage total exceeds the supported range")
	}
	usage := &dto.RealtimeUsage{InputTokens: *u.InputTokens, OutputTokens: *u.OutputTokens, TotalTokens: *u.InputTokens + *u.OutputTokens}
	if u.TotalTokens != nil && *u.TotalTokens != usage.TotalTokens {
		return nil, fmt.Errorf("Qwen usage total does not match input/output totals")
	}
	for i, details := range []*QwenTokenDetails{u.InputTokensDetails, u.OutputTokensDetails} {
		total := usage.InputTokens
		if i == 1 {
			total = usage.OutputTokens
		}
		if details == nil {
			if total != 0 {
				return nil, fmt.Errorf("missing Qwen modality usage details")
			}
			continue
		}
		var counts [4]int
		remaining := total
		for j, count := range []*int{details.TextTokens, details.AudioTokens, details.ImageTokens, details.VideoTokens} {
			if count == nil {
				continue
			}
			if *count < 0 || *count > remaining {
				return nil, fmt.Errorf("Qwen modality usage exceeds its total or is negative")
			}
			counts[j] = *count
			remaining -= *count
		}
		// Upstream totals can include tokens absent from the modality breakdown.
		// Keep those tokens in the total instead of rejecting a completed response.
		if i == 1 && counts[3] != 0 {
			return nil, fmt.Errorf("Qwen usage has unsupported video output tokens")
		}
		if i == 0 {
			usage.InputTokenDetails = dto.InputTokenDetails{TextTokens: counts[0], AudioTokens: counts[1], ImageTokens: counts[2], VideoTokens: counts[3]}
		} else {
			usage.OutputTokenDetails = dto.OutputTokenDetails{TextTokens: counts[0], AudioTokens: counts[1], ImageTokens: counts[2]}
		}
	}
	return usage, nil
}

type qwenRealtimeUsageTracker struct {
	seen    map[string]bool // false records a terminal event that lacked usage.
	pending map[string]struct{}
}

func (t *qwenRealtimeUsageTracker) consume(info *relaycommon.RelayInfo, event *QwenRealtimeEvent, total *dto.RealtimeUsage) (bool, error) {
	if event.Type != QwenEventResponseDone {
		id := event.ResponseID
		if event.Type == "response.created" && event.Response != nil {
			id = event.Response.ID
		}
		if id != "" && strings.HasPrefix(event.Type, "response.") {
			if _, ended := t.seen["response:"+id]; !ended {
				if len(t.pending) >= 10000 || len(id) > 512 {
					return false, fmt.Errorf("Qwen realtime session has too many pending responses or an oversized identifier; reconnect")
				}
				if t.pending == nil {
					t.pending = make(map[string]struct{})
				}
				t.pending[id] = struct{}{}
			}
		}
		return false, nil
	}
	response := event.Response
	if response == nil {
		info.RealtimeUsageIncomplete = true
		return false, fmt.Errorf("Qwen response.done is missing its response")
	}
	delete(t.pending, response.ID)
	key := "response:" + response.ID
	if response.ID == "" {
		key = "event:" + event.EventId
	}
	if response.ID == "" && event.EventId == "" {
		info.RealtimeUsageIncomplete = true
		return false, fmt.Errorf("Qwen response.done has no usage identifier")
	}
	if accounted, exists := t.seen[key]; exists && (accounted || response.Usage == nil) {
		info.RealtimeDuplicateResponses++
		return false, nil
	}
	if len(t.seen) >= 10000 || len(key) > 512 {
		return false, fmt.Errorf("Qwen realtime session has too many responses or an oversized identifier; reconnect")
	}
	if t.seen == nil {
		t.seen = make(map[string]bool)
	}
	if response.Usage == nil {
		t.seen[key] = false
		info.RealtimeUsageIncomplete = true
		if len(info.RealtimeUsageDiagnostics) < 32 {
			info.RealtimeUsageDiagnostics = append(info.RealtimeUsageDiagnostics, relaycommon.RealtimeUsageDiagnostic{ResponseID: response.ID, Status: response.Status, Reason: "missing upstream usage"})
		}
		return false, nil
	}
	raw, err := response.Usage.realtimeUsage()
	if err != nil {
		return false, err
	}
	billable := *raw
	inputUnclassified := raw.InputTokens - raw.InputTokenDetails.TextTokens - raw.InputTokenDetails.AudioTokens - raw.InputTokenDetails.ImageTokens - raw.InputTokenDetails.VideoTokens
	outputUnclassified := raw.OutputTokens - raw.OutputTokenDetails.TextTokens - raw.OutputTokenDetails.AudioTokens - raw.OutputTokenDetails.ImageTokens
	// The expression engine's p/c variables cover unclassified tokens. Include
	// the same quantities in the legacy base-rate counters on the billing copy.
	// These are not reported text or audio and must not become free transcripts.
	billable.InputTokenDetails.TextTokens += inputUnclassified
	billable.OutputTokenDetails.TextTokens += outputUnclassified
	if inputUnclassified != 0 || outputUnclassified != 0 {
		info.RealtimeUsageIncomplete = true
		if len(info.RealtimeUsageDiagnostics) < 32 {
			info.RealtimeUsageDiagnostics = append(info.RealtimeUsageDiagnostics, relaycommon.RealtimeUsageDiagnostic{
				ResponseID: response.ID, Status: response.Status,
				Reason:                  "modality details omit tokens included in upstream totals; unclassified tokens remain billable at the base rate",
				InputUnclassifiedTokens: inputUnclassified, OutputUnclassifiedTokens: outputUnclassified,
			})
		}
	}
	if isQwenAudio31Model(info.UpstreamModelName) {
		hasAudio := raw.OutputTokenDetails.AudioTokens > 0
		hasIndependentText := false
		for _, item := range response.Output {
			if item.Type != "message" {
				hasIndependentText = true // Tool tokens cannot be identified as free transcripts.
			}
			for _, content := range item.Content {
				hasAudio = hasAudio || content.Type == "audio" || content.Type == "output_audio"
				hasIndependentText = hasIndependentText || content.Type == "text" || content.Type == "output_text"
			}
		}
		// Modalities alone describe requested output; they do not prove speech was generated.
		if hasAudio && !hasIndependentText {
			freeText := raw.OutputTokenDetails.TextTokens
			billable.OutputTokenDetails.TextTokens -= freeText
			billable.OutputTokens -= freeText
			billable.TotalTokens -= freeText
		}
		if hasAudio && hasIndependentText && raw.OutputTokenDetails.TextTokens > 0 {
			info.RealtimeUsageIncomplete = true
			if len(info.RealtimeUsageDiagnostics) < 32 {
				info.RealtimeUsageDiagnostics = append(info.RealtimeUsageDiagnostics, relaycommon.RealtimeUsageDiagnostic{ResponseID: response.ID, Status: response.Status, Reason: "mixed tool/text and audio output has no separate transcript token count; text remains billable"})
			}
		}
	}
	if err := addQwenRealtimeUsage(total, raw); err != nil {
		return false, err
	}
	if info.RealtimeBillingUsage == nil {
		info.RealtimeBillingUsage = &dto.RealtimeUsage{}
	}
	if err := addQwenRealtimeUsage(info.RealtimeBillingUsage, &billable); err != nil {
		return false, err
	}
	t.seen[key] = true
	info.RealtimeResponseCount++
	if search, exists := response.Usage.Plugins["search"]; exists && string(search) != "null" && string(search) != "{}" {
		var metering struct {
			Count *int `json:"count"`
		}
		if err := common.Unmarshal(search, &metering); err != nil || metering.Count == nil || *metering.Count != 0 {
			return true, fmt.Errorf("Qwen returned unexpected search usage without a verified surcharge contract")
		}
	}
	return true, nil
}

func addQwenRealtimeUsage(total, delta *dto.RealtimeUsage) error {
	if delta.TotalTokens < 0 || delta.TotalTokens > common.MaxQuota-total.TotalTokens {
		return fmt.Errorf("Qwen realtime session usage exceeds the supported range")
	}
	total.TotalTokens += delta.TotalTokens
	total.InputTokens += delta.InputTokens
	total.OutputTokens += delta.OutputTokens
	total.InputTokenDetails.TextTokens += delta.InputTokenDetails.TextTokens
	total.InputTokenDetails.AudioTokens += delta.InputTokenDetails.AudioTokens
	total.InputTokenDetails.ImageTokens += delta.InputTokenDetails.ImageTokens
	total.InputTokenDetails.VideoTokens += delta.InputTokenDetails.VideoTokens
	total.OutputTokenDetails.TextTokens += delta.OutputTokenDetails.TextTokens
	total.OutputTokenDetails.AudioTokens += delta.OutputTokenDetails.AudioTokens
	total.OutputTokenDetails.ImageTokens += delta.OutputTokenDetails.ImageTokens
	return nil
}
