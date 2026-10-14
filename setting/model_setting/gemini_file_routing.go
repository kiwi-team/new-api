package model_setting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tidwall/gjson"
)

const GeminiFileRoutingKey = "GeminiFileRouting"
const LargeFileChannelsKey = "LargeFileChannels"

// GeminiFileRouting is snapshotted for the lifetime of a request, including retries.
type GeminiFileRouting struct {
	Enabled               bool     `json:"enabled"`
	Mode                  string   `json:"mode"`
	ChatModelPatterns     []string `json:"chat_model_patterns"`
	InlineThresholdBytes  int64    `json:"inline_threshold_bytes"`
	MaxInlineRequestBytes int64    `json:"max_inline_request_bytes"`
}

func ParseGeminiFileRouting(raw string) (GeminiFileRouting, error) {
	config := GeminiFileRouting{Mode: "forward", ChatModelPatterns: []string{}, InlineThresholdBytes: 10 << 20, MaxInlineRequestBytes: 32 << 20}
	if raw == "" {
		return config, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return config, fmt.Errorf("GeminiFileRouting must be a JSON object")
	}
	if err := common.UnmarshalJsonStr(raw, &config); err != nil {
		return config, fmt.Errorf("invalid GeminiFileRouting JSON")
	}
	for _, field := range []string{"enabled", "mode", "chat_model_patterns", "inline_threshold_bytes", "max_inline_request_bytes"} {
		if value := gjson.Get(raw, field); value.Exists() && value.Type == gjson.Null {
			return config, fmt.Errorf("file routing fields must not be null")
		}
	}
	if config.Mode != "forward" && config.Mode != "process" {
		return config, fmt.Errorf("file routing mode must be forward or process")
	}
	if config.InlineThresholdBytes < 1 || config.InlineThresholdBytes > 16<<20 {
		return config, fmt.Errorf("inline_threshold_bytes must be between 1 and 16777216")
	}
	if config.MaxInlineRequestBytes < 1 {
		return config, fmt.Errorf("max_inline_request_bytes must be a positive integer")
	}
	for _, pattern := range config.ChatModelPatterns {
		if strings.TrimSpace(pattern) == "" {
			return config, fmt.Errorf("chat model patterns must not be empty")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return config, fmt.Errorf("invalid chat model pattern")
		}
	}
	return config, nil
}

func GetGeminiFileRouting() (GeminiFileRouting, error) {
	common.OptionMapRWMutex.RLock()
	raw, exists := common.OptionMap[GeminiFileRoutingKey]
	common.OptionMapRWMutex.RUnlock()
	if exists && strings.TrimSpace(raw) == "" {
		return GeminiFileRouting{}, fmt.Errorf("GeminiFileRouting must be a JSON object")
	}
	return ParseGeminiFileRouting(raw)
}

func ParseLargeFileChannels(raw string) ([]dto.SpecailChannels, error) {
	if raw == "" {
		return nil, nil
	}
	var rules []dto.SpecailChannels
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil || rules == nil {
		return nil, fmt.Errorf("LargeFileChannels must be a JSON array")
	}
	for i := range rules {
		rule := &rules[i]
		if rule.RandomType == "" {
			rule.RandomType = "order"
		}
		if strings.TrimSpace(rule.ModelName) == "" || (rule.RandomType != "order" && rule.RandomType != "random") || len(rule.ChannelIds) == 0 {
			return nil, fmt.Errorf("invalid large file channel rule")
		}
		if _, err := regexp.Compile(rule.ModelName); err != nil {
			return nil, fmt.Errorf("invalid large file model pattern")
		}
		for _, tier := range rule.ChannelIds {
			if len(tier) == 0 {
				return nil, fmt.Errorf("large file channel tiers must not be empty")
			}
			for _, id := range tier {
				if id <= 0 {
					return nil, fmt.Errorf("large file channel IDs must be positive")
				}
			}
		}
	}
	return rules, nil
}

func ValidateGeminiFileRoutingOption(key, value string) error {
	switch key {
	case GeminiFileRoutingKey:
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("GeminiFileRouting must be a JSON object")
		}
		_, err := ParseGeminiFileRouting(value)
		return err
	case LargeFileChannelsKey:
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("use [] to clear large file rules")
		}
		_, err := ParseLargeFileChannels(value)
		return err
	case "OnlyTextChannels", "VideoChannels", "OnlyImageChannels", "NoVideoChannels":
		return fmt.Errorf("modality routing rules are retired; use LargeFileChannels or ordinary routing")
	}
	return nil
}
