package fal_sync

import (
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// gpt-image-2 official per-1M-token prices (Standard tier), used only to keep
// the reverse-engineered token counts consistent with OpenAI's billing model.
//
//	image input : $8.00 / 1M tokens
//	image output: $30.00 / 1M tokens
const (
	gptImage2InputPricePerToken  = 8.0 / 1_000_000.0
	gptImage2OutputPricePerToken = 30.0 / 1_000_000.0
)

// gptImage2HighQualityPrice maps "WxH" -> fal High Quality per-call price (USD),
// each value already including ONE input image. Sizes match the fal pricing
// table for ChatGPT Images 2.0 (High Quality column).
var gptImage2HighQualityPrice = map[string]float64{
	"1024x768":  0.151,
	"1024x1024": 0.219,
	"1024x1536": 0.178,
	"1920x1080": 0.158,
	"2560x1440": 0.234,
	"3840x2160": 0.413,
}

// gptImage2DefaultSize is used when the requested size is empty, "auto", or not
// present in the High Quality price table.
const gptImage2DefaultSize = "1024x1024"

// computeGptImage2Usage reverse-engineers an OpenAI-style images `usage` object
// for a gpt-image-2 edit call so that token-based billing produces (roughly) the
// same cost as fal's per-call High Quality price.
//
// Approach (per product decision):
//   - input image tokens are computed with OpenAI's 32x32 patch formula, summed
//     over every input image (numInputImages);
//   - output tokens are solved from
//     price = inputImageTokens*$8/1M + outputTokens*$30/1M
//     so that repricing the usage with the official token prices reproduces the
//     fal per-call price for a single input image, and additional input images
//     only add their (cheap) input cost.
//
// size is the requested output size ("WxH"); unknown/auto sizes fall back to
// 1024x1024. numInputImages is the number of reference images supplied (>=1 is
// assumed for the edit endpoint). numOutputImages is the number of images fal
// actually generated (fal bills per output image).
func computeGptImage2Usage(size string, numInputImages, numOutputImages int) *dto.ImageUsage {
	normSize, price := resolveGptImage2SizeAndPrice(size)
	w, h := parseSize(normSize)

	if numInputImages < 1 {
		numInputImages = 1
	}
	if numOutputImages < 1 {
		numOutputImages = 1
	}

	perImageInputTokens := imageInputTokens(w, h)

	// Solve output tokens from the (single-image) table price. The table price
	// already accounts for exactly one input image, so we base the output-token
	// solve on a single image's input cost; extra input images only add input
	// cost.
	baseInputCost := float64(perImageInputTokens) * gptImage2InputPricePerToken
	outputTokensPerImage := int(math.Round((price - baseInputCost) / gptImage2OutputPricePerToken))
	if outputTokensPerImage < 0 {
		outputTokensPerImage = 0
	}

	// fal charges per generated output image; extra reference images add input.
	inputImageTokens := perImageInputTokens * numInputImages * numOutputImages
	outputTokens := outputTokensPerImage * numOutputImages

	return &dto.ImageUsage{
		InputTokens:  inputImageTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputImageTokens + outputTokens,
		InputTokensDetails: &dto.ImageInputTokenDetails{
			ImageTokens: inputImageTokens,
			TextTokens:  0,
		},
	}
}

// resolveGptImage2SizeAndPrice normalises the requested size and returns the
// matching High Quality price, falling back to 1024x1024 for empty / "auto" /
// unknown sizes.
func resolveGptImage2SizeAndPrice(size string) (string, float64) {
	norm := normalizeSize(size)
	if price, ok := gptImage2HighQualityPrice[norm]; ok {
		return norm, price
	}
	return gptImage2DefaultSize, gptImage2HighQualityPrice[gptImage2DefaultSize]
}

// normalizeSize lowercases and canonicalises a size string to the "WxH" form.
// Empty strings and "auto" return "" so callers fall back to the default.
func normalizeSize(size string) string {
	s := strings.ToLower(strings.TrimSpace(size))
	if s == "" || s == "auto" {
		return ""
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "×", "x") // unicode multiplication sign
	s = strings.ReplaceAll(s, "*", "x")
	return s
}

// parseSize parses a "WxH" string into width/height, defaulting to 1024x1024 on
// any parse failure.
func parseSize(size string) (int, int) {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return 1024, 1024
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 1024, 1024
	}
	return w, h
}

// imageInputTokens computes the number of image input tokens for a WxH image
// using OpenAI's 32x32 patch tokenization, capped at 1536 patches (images larger
// than the cap are scaled down before counting), matching how other channels
// count gpt-image input tokens.
func imageInputTokens(w, h int) int {
	if w <= 0 || h <= 0 {
		w, h = 1024, 1024
	}
	patchesW := ceilDiv(w, 32)
	patchesH := ceilDiv(h, 32)
	total := patchesW * patchesH
	if total <= 1536 {
		return total
	}
	// Scale the image down so the patch count fits under the 1536 cap.
	scale := math.Sqrt(float64(1536*32*32) / float64(w*h))
	scaledW := int(float64(w) * scale / 32)
	scaledH := int(float64(h) * scale / 32)
	total = scaledW * scaledH
	if total < 1 {
		return 1
	}
	if total > 1536 {
		return 1536
	}
	return total
}

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

// ============================================================================
// nano-banana-2 (gemini-3.1-flash-image) usage reverse-engineering
// ============================================================================

// nano-banana-2 fal pricing (per generated image):
//
//	standard (1K) rate : $0.08 / image
//	0.5K (512px)       : 0.75x  -> $0.06
//	2K                 : 1.5x   -> $0.12
//	4K                 : 2x     -> $0.16
//	web search (once)  : +$0.015
//
// Official Gemini image output price is $60 / 1M tokens. We reverse fal's price
// at the official token rate so that repricing the usage with the official rate
// reproduces fal's per-image cost.
const (
	nanoBananaImagePrice          = 0.08
	nanoBananaWebSearchPrice      = 0.015
	nanoBananaOutputPricePerToken = 60.0 / 1_000_000.0
)

// nanoBananaResolutionMultiplier maps a normalised resolution tier to fal's
// price multiplier.
var nanoBananaResolutionMultiplier = map[string]float64{
	"0.5k": 0.75,
	"1k":   1.0,
	"2k":   1.5,
	"4k":   2.0,
}

// nanoBananaDefaultResolution is used when no resolution is specified (fal's
// standard rate).
const nanoBananaDefaultResolution = "1k"

// computeNanoBananaUsage reverse-engineers an OpenAI-style images `usage` object
// for a nano-banana-2 call. fal bills purely per output image (with a resolution
// multiplier) plus an optional one-time web-search fee, so all tokens are booked
// as output tokens (input_tokens = 0).
//
// resolution is the requested output resolution ("0.5K"/"1K"/"2K"/"4K"; unknown
// values fall back to 1K). webSearch indicates whether fal's web-search add-on
// was used. numOutputImages is the number of images fal actually generated.
func computeNanoBananaUsage(resolution string, webSearch bool, numOutputImages int) *dto.ImageUsage {
	if numOutputImages < 1 {
		numOutputImages = 1
	}

	mult := nanoBananaResolutionMultiplier[normalizeResolution(resolution)]

	perImagePrice := nanoBananaImagePrice * mult
	perImageOutputTokens := int(math.Round(perImagePrice / nanoBananaOutputPricePerToken))

	outputTokens := perImageOutputTokens * numOutputImages

	// Web search is charged once per request, not per image.
	if webSearch {
		outputTokens += int(math.Round(nanoBananaWebSearchPrice / nanoBananaOutputPricePerToken))
	}

	return &dto.ImageUsage{
		InputTokens:  0,
		OutputTokens: outputTokens,
		TotalTokens:  outputTokens,
		InputTokensDetails: &dto.ImageInputTokenDetails{
			ImageTokens: 0,
			TextTokens:  0,
		},
	}
}

// normalizeResolution canonicalises a resolution string to one of the tier keys
// used by nanoBananaResolutionMultiplier ("0.5k"/"1k"/"2k"/"4k"), defaulting to
// 1K for empty / unknown values.
func normalizeResolution(res string) string {
	r := strings.ToLower(strings.TrimSpace(res))
	r = strings.ReplaceAll(r, "px", "")
	r = strings.ReplaceAll(r, " ", "")
	switch r {
	case "0.5k", "512", "512x512", "0.5":
		return "0.5k"
	case "1k", "1024", "1024x1024", "1":
		return "1k"
	case "2k", "2048", "2048x2048", "2":
		return "2k"
	case "4k", "4096", "4096x4096", "4":
		return "4k"
	}
	return nanoBananaDefaultResolution
}
