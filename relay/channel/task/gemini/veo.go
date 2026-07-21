package gemini

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ============================
// Google Veo per-second billing support
//
// The Veo video models (e.g. veo-3.0-generate-001, veo-3.1-generate-001) bill per
// second of generated video, the same way the Gemini Omni and Sora-2 models do. The
// generated duration is written into info.PriceData.OtherRatios["seconds"], which the
// task billing path multiplies into the final ratio
// (quota = modelPrice(per-second) × seconds × groupRatio).
// ============================

// veoDefaultSeconds is the default video length (seconds) billed when the client does
// not specify a duration. Veo generates 8-second videos by default.
const veoDefaultSeconds = 8

// IsVeoModel reports whether the model name refers to a Google Veo video model.
// Exported for reuse by other providers (e.g. Vertex AI) sharing the Veo models.
func IsVeoModel(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "veo")
}

// isVeoModel is the package-local alias used within the gemini adaptor.
func isVeoModel(name string) bool {
	return IsVeoModel(name)
}

// ApplyVeoSecondsRatio sets the per-second billing multiplier for a Veo video task.
// It reads the request's `seconds` (falling back to `duration`, then veoDefaultSeconds)
// and writes it into info.PriceData.OtherRatios["seconds"].
//
// Like ApplyOmniSecondsRatio, it MUST be called from ValidateRequestAndSetAction (which runs
// before the task pricing step) rather than BuildRequestBody (which runs after pricing), so the
// generated duration participates in quota calculation.
func ApplyVeoSecondsRatio(info *relaycommon.RelayInfo, seconds string, duration int) {
	sec := common.String2Int(seconds)
	if sec <= 0 {
		sec = duration
	}
	if sec <= 0 {
		sec = veoDefaultSeconds
	}
	if info.PriceData.OtherRatios == nil {
		info.PriceData.OtherRatios = map[string]float64{}
	}
	info.PriceData.OtherRatios["seconds"] = float64(sec)
}
