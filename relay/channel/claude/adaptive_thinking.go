package claude

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// RequiresAdaptiveThinking reports whether a Claude model rejects native
// thinking budgets and requires adaptive thinking instead.
func RequiresAdaptiveThinking(model string) bool {
	model = strings.ToLower(model)
	if strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8") {
		return true
	}

	parts := strings.Split(model, "-")
	if len(parts) < 2 || parts[0] != "claude" {
		return false
	}

	versionIndex := 1
	if _, err := strconv.Atoi(parts[versionIndex]); err != nil {
		versionIndex++
	}
	if versionIndex >= len(parts) {
		return false
	}

	majorVersion, err := strconv.Atoi(parts[versionIndex])
	return err == nil && majorVersion >= 5
}

// NormalizeAdaptiveThinking rewrites legacy enabled/budget_tokens requests for
// models that only accept adaptive thinking. It also applies the sampling
// restrictions shared by those models.
func NormalizeAdaptiveThinking(request *dto.ClaudeRequest) bool {
	if request == nil || !RequiresAdaptiveThinking(request.Model) {
		return false
	}

	if request.Thinking != nil && request.Thinking.Type == "enabled" {
		request.Thinking = &dto.Thinking{
			Type:    "adaptive",
			Display: request.Thinking.Display,
		}
		if request.OutputConfig == nil {
			request.OutputConfig = []byte(`{"effort":"high"}`)
		}
	}

	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
	request.Model = strings.TrimSuffix(request.Model, "-thinking")
	return true
}
