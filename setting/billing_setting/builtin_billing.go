package billing_setting

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// deepSeekPeak is DeepSeek's peak window: Monday to Friday, 09:00-12:00 and
// 14:00-18:00 Beijing time. The Chinese public holidays DeepSeek bills
// off-peak are not known in advance and bill at peak here.
const deepSeekPeak = `weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") <= 5 && ((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) || (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18))`

const deepSeekFlashExpr = deepSeekPeak + ` ? tier("peak", p * 0.3 + cr * 0.006 + c * 1.2) : tier("off_peak", p * 0.15 + cr * 0.003 + c * 0.6)`

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
	// Gemini API Standard USD/M tokens, checked 2026-11-12.
	// https://aistudio.google.com/docs/pricing#gemini-omni-1.2-flash
	// Task usage expressions return USD; thinking is separate from output.
	"gemini-omni-1.2-flash": `tier("standard", u("input_tokens") * 1.5 / 1000000 + u("text_output_tokens") * 9 / 1000000 + u("thought_tokens") * 9 / 1000000 + u("video_output_tokens") * 12 / 1000000)`,
	// H3 image regular CNY/M-token prices, checked 2026-11-11. Fixed
	// conversion USD 1 = CNY 6.8, as with MiniMax speech below. Temporary
	// provider promotions are not part of these regular default prices.
	// https://platform.minimax.cn/docs/pricing/overview
	"MiniMax-H3-Image":           `tier("china", (p * 15 + img * 10 + c * 25) / 6.8)`,
	"MiniMax-H3-Image-Highspeed": `tier("china", (p * 15 + img * 20 + c * 50) / 6.8)`,
	// Native speech prices checked 2026-11-10; chars is USD/M billable
	// characters. Qwen uses Beijing international USD prices; Eleven uses
	// public API pay-as-you-go. Doubao and MiniMax use domestic CNY prices
	// with the explicit fixed conversion USD 1 = CNY 6.8.
	// https://docs.modelstudio.console.alibabacloud.com/en/model-studio/qwen-audio-3-1-tts-plus
	// https://elevenlabs.io/pricing/api
	// https://docs.volcengine.com/docs/zh/doubao-voice/billing-instructions.md
	// https://platform.minimax.cn/docs/pricing/overview
	"qwen-audio-3.1-tts-plus":  `tier("beijing", chars * 20)`,
	"qwen-audio-3.1-tts-flash": `tier("beijing", p * 0.212 + c * 1.696)`,
	"qwen-audio-3.1-tts-next":  `tier("beijing", p * 0.848 + c * 1.696)`,
	"eleven_v4":                `tier("standard", chars * 80)`,
	"eleven_v4_turbo":          `tier("standard", chars * 40)`,
	"seed-tts-2.0":             `tier("payg", chars * 300 / 6.8)`,
	"seed-icl-2.0":             `tier("payg", chars * 300 / 6.8)`,
	"speech-2.8-hd":            `tier("china", chars * 350 / 6.8)`,
	"speech-2.8-turbo":         `tier("china", chars * 200 / 6.8)`,
	// Beijing USD per million tokens, checked 2026-11-05:
	// https://www.alibabacloud.com/help/en/model-studio/qwen3-8-omni-flash-realtime
	// https://www.alibabacloud.com/help/en/model-studio/qwen-audio-3-1-realtime-plus
	// https://www.alibabacloud.com/help/en/model-studio/qwen-audio-3-1-realtime-flash
	// Audio 3.1 excludes accompanying speech text in its per-response billing copy.
	"qwen3.8-omni-flash-realtime":   `tier("beijing", p * 0.212 + ai * 0.848 + c * 0.636 + ao * 1.696)`,
	"qwen-audio-3.1-realtime-plus":  `tier("beijing", p * 0.688 + ai * 5.501 + c * 5.501 + ao * 20.628)`,
	"qwen-audio-3.1-realtime-flash": `tier("beijing", p * 0.212 + ai * 0.848 + c * 0.636 + ao * 1.696)`,
	// https://docs.volcengine.com/docs/doubao-voice/billing-instructions?lang=zh
	// CNY per million tokens, checked 2026-11-05; fixed conversion USD 1 =
	// CNY 6.8. Cached text and audio both cost CNY 5; ai excludes cached audio.
	"seeduplex-1.2.6.1": `tier("standard", (p * 10 + ai * 80 + cr * 5 + c * 80 + ao * 300) / 6.8)`,
	// https://kling.ai/document-api/pricing/base/video.md (2026-11-05).
	// Official international USD per billed second. The plugin reports output
	// seconds without video input, otherwise max(input, output) + output.
	"kling-4.0": `u("video_input") == "video" && u("resolution") == "1080p" ? tier("1080p_video", u("billable_seconds") * 0.141) : u("video_input") == "video" ? tier("720p_video", u("billable_seconds") * 0.094) : u("resolution") == "1080p" ? tier("1080p", u("billable_seconds") * 0.235) : tier("720p", u("billable_seconds") * 0.157)`,
	// https://www.alibabacloud.com/help/en/model-studio/qwen-audio-3-1-asr-flash
	// Beijing USD per million actual input/output tokens, checked 2026-10-26.
	"qwen-audio-3.1-asr-flash": `tier("beijing", p * 0.113 + c * 0.382)`,
	// https://developers.openai.com/api/docs/guides/decisions and
	// https://developers.openai.com/api/docs/models/gpt-6-luna (2026-10-15).
	// Decisions prices differ from Chat/Responses. Global processing defaults;
	// cache reads, cache writes and output are free, including long context.
	"gpt-6-luna-decisions":        `len <= 272000 ? tier("standard", p * 0.1 + cr * 0 + cc * 0 + c * 0) : tier("long_context", p * 0.2 + cr * 0 + cc * 0 + c * 0)`,
	"openai/gpt-6-luna-decisions": `len <= 272000 ? tier("standard", p * 0.1 + cr * 0 + cc * 0 + c * 0) : tier("long_context", p * 0.2 + cr * 0 + cc * 0 + c * 0)`,
	// https://www.alibabacloud.com/help/en/model-studio/happyoyster-1-0-directing
	// Singapore USD per call / second, checked 2026-10-13. Directing defaults
	// intentionally use Singapore prices for every channel region. Protocol usage
	// expressions use quantities directly (not token-per-million scaling).
	"happyoyster-1.0-directing": `tier("international", u("world_creations_without_reference") * 0.3113 + u("world_creations_with_reference") * 0.0077 + u("instruction_injections") * 0.0154 + u("experience_480p_seconds") * 0.0539 + u("experience_720p_seconds") * 0.0858)`,
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-09-09).
	// The Images API reports image output in output_tokens, normalized to c.
	"gpt-image-2":            `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-sunburst": `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-flare":    `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	// https://developers.openai.com/api/docs/models/gpt-6-astra
	// Standard pricing; the long-context rates apply to the whole request.
	// Do not infer service-tier discounts from incoming request parameters:
	// channels filter service_tier by default, so it may not reach the upstream.
	"gpt-6-astra": `len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)`,
	// https://docs.typesafe.ai/models (Jev 1.13, 2026-09-20).
	// TypeSafe charges input tokens only; output tokens are free.
	"jev-1.13.0": `tier("standard", p * 0.042 + c * 0)`,
	// https://openrouter.ai/~typesafe/jev-latest (2026-09-22).
	// OpenRouter publishes the same $0.042/M input and free output price.
	"~typesafe/jev-latest": `tier("standard", p * 0.042 + c * 0)`,
	"typesafe/jev-1.13":    `tier("standard", p * 0.042 + c * 0)`,
	// https://cloud.tencent.com/document/product/1823/130055 (2026-09-10):
	// CNY 10 / 1M tokens. Converted at the 2026-09-21 PBOC midpoint of
	// USD 1 = CNY 6.7487. TokenHub reports each model's billable total as input.
	"hy-asr-3.0-preview":    `tier("standard", p * 1.481766859)`,
	"hy-image-v3.5-preview": `tier("standard", p * 1.481766859)`,
	// https://platform.qianwenai.com/docs/api-reference/world-model/happyoyster-adventure-openapi-reference.md
	// HappyOyster Adventure OpenAPI pricing, checked 2026-09-22. `price_scope`
	// is selected explicitly on the channel (international or global).
	"happyoyster-1.0-adventure": `u("price_scope") == "international" ? tier("international", u("world_creations") * 0.0077 + u("experience_seconds") * 0.0308) : tier("global", u("world_creations") * 0.007067 + u("experience_seconds") * 0.028267)`,
	// https://api-docs.deepseek.com/quick_start/pricing (2026-10-17). Off-peak
	// is half the peak price. deepseek-v4-flash and deepseek-v4-flash-vision-exp
	// run on V4.1 Flash and bill at the Flash price. Images bill as input
	// tokens, and there is no cache-write price.
	"deepseek-flash":               deepSeekFlashExpr,
	"deepseek-v4-flash":            deepSeekFlashExpr,
	"deepseek-v4-flash-vision-exp": deepSeekFlashExpr,
	"deepseek-v4-pro":              deepSeekPeak + ` ? tier("peak", p * 1.32 + cr * 0.044 + c * 3.96) : tier("off_peak", p * 0.66 + cr * 0.022 + c * 1.98)`,
	// The Search and Fetch API routing names: $0.75 per 1,000 successful
	// requests each, the same pricing page. The calls carry no tokens.
	constant.DeepSeekSearchModel: `tier("request", fixed(0.00075))`,
	constant.DeepSeekFetchModel:  `tier("request", fixed(0.00075))`,
}

// GetBuiltinTaskUsageSchema describes native task meters for the existing
// pricing editor and expression validation. It does not select a task protocol.
func GetBuiltinTaskUsageSchema(modelName string) map[string]jsplugin.UsageFieldSchema {
	if !relaycommon.IsGeminiOmniVideoModel(modelName) {
		return nil
	}
	return map[string]jsplugin.UsageFieldSchema{
		"input_tokens": {Type: "number", Unit: "token", Description: jsplugin.LocalizedText{
			"en": "Input unit price", "zh": "输入单价",
		}},
		"text_output_tokens": {Type: "number", Unit: "token", Description: jsplugin.LocalizedText{
			"en": "Text output unit price", "zh": "文本输出单价",
		}},
		"thought_tokens": {Type: "number", Unit: "token", Description: jsplugin.LocalizedText{
			"en": "Thinking output unit price", "zh": "思考输出单价",
		}},
		"video_output_tokens": {Type: "number", Unit: "token", Description: jsplugin.LocalizedText{
			"en": "Video output unit price", "zh": "视频输出单价",
		}},
	}
}
