package service

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// TaskTokenUsage reads explicit OpenAI-style token meters from a task submit
// response for logs and statistics only. It does not infer tokens from image
// counts, credits or task usage facts, and never changes the billing meter.
func TaskTokenUsage(response any) *dto.Usage {
	body, ok := response.(map[string]any)
	if !ok || body["usage"] == nil {
		return nil
	}
	encoded, err := common.Marshal(body["usage"])
	if err != nil {
		return nil
	}
	var reported struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
		InputTokens      *int `json:"input_tokens"`
		OutputTokens     *int `json:"output_tokens"`
		TotalTokens      *int `json:"total_tokens"`
	}
	if common.Unmarshal(encoded, &reported) != nil {
		return nil
	}
	if reported.PromptTokens == nil && reported.CompletionTokens == nil && reported.InputTokens == nil && reported.OutputTokens == nil {
		return nil
	}
	for _, count := range []*int{reported.PromptTokens, reported.CompletionTokens, reported.InputTokens, reported.OutputTokens, reported.TotalTokens} {
		if count != nil && (*count < 0 || *count > math.MaxInt32) {
			return nil
		}
	}
	var usage dto.Usage
	if common.Unmarshal(encoded, &usage) != nil {
		return nil
	}
	if reported.PromptTokens == nil && reported.InputTokens != nil {
		usage.PromptTokens = *reported.InputTokens
	}
	if reported.CompletionTokens == nil && reported.OutputTokens != nil {
		usage.CompletionTokens = *reported.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = usage.InputTokensDetails.Clone()
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *usage.OutputTokensDetails
	}
	for _, count := range []int{usage.PromptTokensDetails.CachedTokens, usage.PromptTokensDetails.CachedCreationTokens, usage.PromptTokensDetails.CacheWriteTokens, usage.ClaudeCacheCreation5mTokens, usage.ClaudeCacheCreation1hTokens} {
		if count < 0 || count > math.MaxInt32 {
			return nil
		}
	}
	if usage.PromptTokens > math.MaxInt32-usage.CompletionTokens {
		return nil
	}
	total := usage.PromptTokens + usage.CompletionTokens
	if reported.TotalTokens == nil {
		usage.TotalTokens = total
	} else if usage.TotalTokens < total {
		return nil
	}
	return &usage
}

const (
	usageBillingPathLocal              = "local"
	usageBillingPathUpstream           = "upstream"
	usageBillingPathOpenAI             = "billing-usage-openai"
	usageBillingPathOpenAIEstimated    = "billing-usage-openai-estimated"
	usageBillingPathAnthropic          = "billing-usage-anthropic"
	usageBillingPathAnthropicEstimated = "billing-usage-anthropic-estimated"
	usageBillingPathGemini             = "billing-usage-gemini"
	usageBillingPathGeminiEstimated    = "billing-usage-gemini-estimated"
)

func effectiveBillingUsage(usage *dto.Usage) *dto.Usage {
	if billingUsage, ok := usageFromBillingUsage(usage); ok {
		return billingUsage
	}
	return usage
}

func usageBillingPathForLog(isLocalCountTokens bool, usage *dto.Usage) string {
	effectiveUsage, ok := usageFromBillingUsage(usage)
	if !ok {
		if isLocalCountTokens {
			return usageBillingPathLocal
		}
		return usageBillingPathUpstream
	}

	switch effectiveUsage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI:
		if usage.BillingUsage.Estimated {
			return usageBillingPathOpenAIEstimated
		}
		return usageBillingPathOpenAI
	case dto.BillingUsageSemanticAnthropic:
		if usage.BillingUsage.Estimated {
			return usageBillingPathAnthropicEstimated
		}
		return usageBillingPathAnthropic
	case dto.BillingUsageSemanticGemini:
		if usage.BillingUsage.Estimated {
			return usageBillingPathGeminiEstimated
		}
		return usageBillingPathGemini
	}

	return usageBillingPathUpstream
}

func appendUsageBillingPathForLog(other *model.LogOther, isLocalCountTokens bool, usage *dto.Usage) {
	if other == nil {
		return
	}
	other.SetAdmin("usage_billing_path", usageBillingPathForLog(isLocalCountTokens, usage))
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
