package fal_sync

import (
	"math"
	"testing"
)

// TestComputeGptImage2Usage_RepricesToTable verifies that, for a single input
// image and a single output image, repricing the reverse-engineered usage with
// gpt-image-2's official per-token prices reproduces the fal High Quality
// per-call price for every size in the table.
func TestComputeGptImage2Usage_RepricesToTable(t *testing.T) {
	for size, price := range gptImage2HighQualityPrice {
		u := computeGptImage2Usage(size, 1, 1)
		if u == nil {
			t.Fatalf("size %s: nil usage", size)
		}
		repriced := float64(u.InputTokens)*gptImage2InputPricePerToken +
			float64(u.OutputTokens)*gptImage2OutputPricePerToken
		if math.Abs(repriced-price) > 0.0005 {
			t.Errorf("size %s: repriced $%.4f, want table $%.3f (in=%d out=%d)",
				size, repriced, price, u.InputTokens, u.OutputTokens)
		}
		if u.TotalTokens != u.InputTokens+u.OutputTokens {
			t.Errorf("size %s: total %d != in+out %d", size, u.TotalTokens, u.InputTokens+u.OutputTokens)
		}
		if u.InputTokensDetails == nil || u.InputTokensDetails.ImageTokens != u.InputTokens {
			t.Errorf("size %s: image_tokens detail mismatch", size)
		}
	}
}

// TestComputeGptImage2Usage_FallbackSize ensures empty/auto/unknown sizes fall
// back to 1024x1024 pricing.
func TestComputeGptImage2Usage_FallbackSize(t *testing.T) {
	want := computeGptImage2Usage("1024x1024", 1, 1)
	for _, size := range []string{"", "auto", "AUTO", "999x999", "1024X1024", "1024 x 1024", "1024*1024"} {
		got := computeGptImage2Usage(size, 1, 1)
		if got.InputTokens != want.InputTokens || got.OutputTokens != want.OutputTokens {
			t.Errorf("size %q: got (in=%d out=%d), want (in=%d out=%d)",
				size, got.InputTokens, got.OutputTokens, want.InputTokens, want.OutputTokens)
		}
	}
}

// TestComputeGptImage2Usage_MultipleOutputs ensures output images scale usage
// linearly.
func TestComputeGptImage2Usage_MultipleOutputs(t *testing.T) {
	one := computeGptImage2Usage("1024x1024", 1, 1)
	two := computeGptImage2Usage("1024x1024", 1, 2)
	if two.OutputTokens != 2*one.OutputTokens {
		t.Errorf("output tokens not linear: 1 image=%d, 2 images=%d", one.OutputTokens, two.OutputTokens)
	}
	if two.InputTokens != 2*one.InputTokens {
		t.Errorf("input tokens not linear: 1 image=%d, 2 images=%d", one.InputTokens, two.InputTokens)
	}
}

// TestImageInputTokens checks the 32x32 patch counts for known sizes.
func TestImageInputTokens(t *testing.T) {
	cases := []struct {
		w, h, want int
	}{
		{1024, 1024, 1024}, // 32*32
		{1024, 768, 768},   // 32*24
		{1024, 1536, 1536}, // 32*48 = 1536 (at cap)
	}
	for _, c := range cases {
		if got := imageInputTokens(c.w, c.h); got != c.want {
			t.Errorf("imageInputTokens(%d,%d)=%d, want %d", c.w, c.h, got, c.want)
		}
	}
	// Oversized image must be capped at 1536 patches.
	if got := imageInputTokens(3840, 2160); got > 1536 || got < 1 {
		t.Errorf("imageInputTokens(3840,2160)=%d, want in (0,1536]", got)
	}
}

// TestComputeNanoBananaUsage_RepricesToFalPrice verifies that repricing the
// reverse-engineered output tokens with the official $60/1M rate reproduces
// fal's per-image price for every resolution tier.
func TestComputeNanoBananaUsage_RepricesToFalPrice(t *testing.T) {
	cases := []struct {
		res       string
		wantPrice float64
	}{
		{"0.5K", 0.06},
		{"1K", 0.08},
		{"2K", 0.12},
		{"4K", 0.16},
	}
	for _, c := range cases {
		u := computeNanoBananaUsage(c.res, false, 1)
		if u == nil {
			t.Fatalf("res %s: nil usage", c.res)
		}
		if u.InputTokens != 0 {
			t.Errorf("res %s: input tokens = %d, want 0", c.res, u.InputTokens)
		}
		repriced := float64(u.OutputTokens) * nanoBananaOutputPricePerToken
		if math.Abs(repriced-c.wantPrice) > 0.0006 {
			t.Errorf("res %s: repriced $%.4f, want fal $%.3f (out=%d)", c.res, repriced, c.wantPrice, u.OutputTokens)
		}
		if u.TotalTokens != u.OutputTokens {
			t.Errorf("res %s: total %d != output %d", c.res, u.TotalTokens, u.OutputTokens)
		}
	}
}

// TestComputeNanoBananaUsage_WebSearch verifies the one-time web-search fee adds
// the expected extra output tokens (once, not per image).
func TestComputeNanoBananaUsage_WebSearch(t *testing.T) {
	base := computeNanoBananaUsage("1K", false, 2)
	withWS := computeNanoBananaUsage("1K", true, 2)
	extra := withWS.OutputTokens - base.OutputTokens
	wantExtra := int(math.Round(nanoBananaWebSearchPrice / nanoBananaOutputPricePerToken))
	if extra != wantExtra {
		t.Errorf("web search extra tokens = %d, want %d", extra, wantExtra)
	}
}

// TestComputeNanoBananaUsage_FallbackResolution ensures empty/unknown resolutions
// fall back to the 1K standard rate.
func TestComputeNanoBananaUsage_FallbackResolution(t *testing.T) {
	want := computeNanoBananaUsage("1K", false, 1)
	for _, res := range []string{"", "unknown", "720p", "3K"} {
		got := computeNanoBananaUsage(res, false, 1)
		if got.OutputTokens != want.OutputTokens {
			t.Errorf("res %q: output=%d, want %d (fallback to 1K)", res, got.OutputTokens, want.OutputTokens)
		}
	}
}

// TestComputeNanoBananaUsage_MultipleOutputs ensures per-image tokens scale
// linearly with output count.
func TestComputeNanoBananaUsage_MultipleOutputs(t *testing.T) {
	one := computeNanoBananaUsage("2K", false, 1)
	three := computeNanoBananaUsage("2K", false, 3)
	if three.OutputTokens != 3*one.OutputTokens {
		t.Errorf("output tokens not linear: 1=%d, 3=%d", one.OutputTokens, three.OutputTokens)
	}
}
