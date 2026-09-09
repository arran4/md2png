package md2png

import (
	"math"
	"strings"
	"testing"
)

func TestValidationAndDefaults(t *testing.T) {
	img, err := Render([]byte("test"), RenderOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.Bounds().Dx() != 1024 {
		t.Errorf("expected default width 1024, got %d", img.Bounds().Dx())
	}

	imgZero, err := Render([]byte("test"), RenderOptions{Width: 200, Margin: 0, ZeroMargin: true})
	if err != nil {
		t.Fatalf("unexpected error for zero margin: %v", err)
	}
	if imgZero.Bounds().Dx() != 200 {
		t.Errorf("expected width 200, got %d", imgZero.Bounds().Dx())
	}

	if _, err = Render([]byte("test"), RenderOptions{Width: -10}); err != ErrInvalidWidth {
		t.Errorf("expected ErrInvalidWidth, got %v", err)
	}
	if _, err = Render([]byte("test"), RenderOptions{Margin: -5}); err != ErrInvalidMargin {
		t.Errorf("expected ErrInvalidMargin, got %v", err)
	}
	if _, err = Render([]byte("test"), RenderOptions{BaseFontSize: -1}); err != ErrInvalidFontSize {
		t.Errorf("expected ErrInvalidFontSize, got %v", err)
	}
	if _, err = Render([]byte("test"), RenderOptions{Width: 100, Margin: 50}); err != ErrNoDrawableWidth {
		t.Errorf("expected ErrNoDrawableWidth, got %v", err)
	}

	imgSmall, err := Render([]byte("test"), RenderOptions{Width: 100, Margin: 49})
	if err != nil {
		t.Errorf("unexpected error for small drawable width: %v", err)
	}
	if imgSmall != nil && imgSmall.Bounds().Dx() != 100 {
		t.Errorf("expected width 100, got %d", imgSmall.Bounds().Dx())
	}

	if _, err = Render([]byte("test"), RenderOptions{Width: MaxAllowedWidth + 1}); err != ErrResourceLimit {
		t.Errorf("expected ErrResourceLimit for excessive width, got %v", err)
	}
	if _, err = Render([]byte("test"), RenderOptions{BaseFontSize: MaxAllowedFontSize + 1}); err != ErrResourceLimit {
		t.Errorf("expected ErrResourceLimit for excessive font size, got %v", err)
	}
}

func TestRegressionIssues(t *testing.T) {
	fonts, err := LoadFonts(FontConfig{SizeBase: 0})
	if err != nil {
		t.Fatalf("unexpected error for font size 0: %v", err)
	}
	if fonts.Regular == nil {
		t.Fatalf("expected fonts to be loaded for size 0 (fallback to 16)")
	}

	if _, err = Render([]byte("test"), RenderOptions{Width: 100, Margin: 1 << 30}); err != ErrNoDrawableWidth {
		t.Errorf("expected ErrNoDrawableWidth for max-int margin, got %v", err)
	}

	img, err := Render([]byte("test"), RenderOptions{Width: 4096})
	if err != nil {
		t.Errorf("unexpected error for 4096 width: %v", err)
	}
	if img != nil && img.Bounds().Dx() != 4096 {
		t.Errorf("expected 4096 width, got %d", img.Bounds().Dx())
	}

	origPixelBudget := maxTotalPixels
	maxTotalPixels = 100 * 100
	defer func() { maxTotalPixels = origPixelBudget }()

	largeMD := "test\n\ntest\n\ntest\n\ntest\n\ntest\n\ntest"
	_, err = Render([]byte(largeMD), RenderOptions{Width: 100, MaxHeight: 32768})
	if err == nil {
		t.Errorf("expected error for genuine huge allocation, got nil")
	} else if !strings.Contains(err.Error(), "pixel budget") {
		t.Errorf("expected pixel budget error, got %v", err)
	}

	if _, err = Render([]byte("test"), RenderOptions{BaseFontSize: math.NaN()}); err != ErrInvalidFontSize {
		t.Errorf("expected ErrInvalidFontSize for NaN, got %v", err)
	}

	if _, err = Render([]byte("test"), RenderOptions{MaxHeight: -10}); err != ErrInvalidMaxHeight {
		t.Errorf("expected ErrInvalidMaxHeight, got %v", err)
	}
}
