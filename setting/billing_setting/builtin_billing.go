package billing_setting

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
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
	// USD 1 = CNY 6.7487. TokenHub reports the billable ASR tokens as input.
	"hy-asr-3.0-preview": `tier("standard", p * 1.481766859)`,
	// https://platform.qianwenai.com/docs/api-reference/world-model/happyoyster-adventure-openapi-reference.md
	// HappyOyster Adventure OpenAPI pricing, checked 2026-09-22. `price_scope`
	// is selected explicitly on the channel (international or global).
	"happyoyster-1.0-adventure": `u("price_scope") == "international" ? tier("international", u("world_creations") * 0.0077 + u("experience_seconds") * 0.0308) : tier("global", u("world_creations") * 0.007067 + u("experience_seconds") * 0.028267)`,
}
