package md2png

import (
	"flag"
	"math"
	"strings"
	"testing"
)

func TestValidationAndDefaults(t *testing.T) {
	// Default dimensions
	img, err := Render([]byte("test"), RenderOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.Bounds().Dx() != 1024 {
		t.Errorf("expected default width 1024, got %d", img.Bounds().Dx())
	}
	// Margin defaults to 48. This means the content is offset by 48.
	// It's a bit harder to test margin exactly by image output unless we inspect pixels,
	// but the error-free compilation is a good start.

	// Explicit zero margin
	imgZero, err := Render([]byte("test"), RenderOptions{Width: 200, Margin: 0, ZeroMargin: true})
	if err != nil {
		t.Fatalf("unexpected error for zero margin: %v", err)
	}
	if imgZero.Bounds().Dx() != 200 {
		t.Errorf("expected width 200, got %d", imgZero.Bounds().Dx())
	}

	// Negative width
	_, err = Render([]byte("test"), RenderOptions{Width: -10})
	if err != ErrInvalidWidth {
		t.Errorf("expected ErrInvalidWidth, got %v", err)
	}

	// Negative margin
	_, err = Render([]byte("test"), RenderOptions{Margin: -5})
	if err != ErrInvalidMargin {
		t.Errorf("expected ErrInvalidMargin, got %v", err)
	}

	// Negative font size
	_, err = Render([]byte("test"), RenderOptions{BaseFontSize: -1})
	if err != ErrInvalidFontSize {
		t.Errorf("expected ErrInvalidFontSize, got %v", err)
	}

	// Margin >= half width
	_, err = Render([]byte("test"), RenderOptions{Width: 100, Margin: 50})
	if err != ErrNoDrawableWidth {
		t.Errorf("expected ErrNoDrawableWidth, got %v", err)
	}

	// Very small but valid content width
	imgSmall, err := Render([]byte("test"), RenderOptions{Width: 100, Margin: 49})
	if err != nil {
		t.Errorf("unexpected error for small drawable width: %v", err)
	}
	if imgSmall != nil && imgSmall.Bounds().Dx() != 100 {
		t.Errorf("expected width 100, got %d", imgSmall.Bounds().Dx())
	}

	// Excessive width
	_, err = Render([]byte("test"), RenderOptions{Width: MaxAllowedWidth + 1})
	if err != ErrResourceLimit {
		t.Errorf("expected ErrResourceLimit for excessive width, got %v", err)
	}

	// Excessive font size
	_, err = Render([]byte("test"), RenderOptions{BaseFontSize: MaxAllowedFontSize + 1})
	if err != ErrResourceLimit {
		t.Errorf("expected ErrResourceLimit for excessive font size, got %v", err)
	}
}

func TestCLIValidationBehavior(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cliOpts := RegisterFlags(fs)

	err := fs.Parse([]string{"-margin", "0", "-width", "800"}) // -pt is omitted
	if err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}

	opts, err := cliOpts.ToRenderOptions(fs, "")
	if err != nil {
		t.Fatalf("unexpected ToRenderOptions error: %v", err)
	}

	if opts.Width != 800 {
		t.Errorf("expected width 800, got %d", opts.Width)
	}
	if opts.Margin != 0 {
		t.Errorf("expected margin 0, got %d", opts.Margin)
	}
	if !opts.ZeroMargin {
		t.Errorf("expected ZeroMargin to be true")
	}
	// Run render to ensure validation completes
	img, err := Render([]byte("test"), opts)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected image, got nil")
	}

	// Test with explicit -pt 0
	fs2 := flag.NewFlagSet("test2", flag.ContinueOnError)
	cliOpts2 := RegisterFlags(fs2)
	err2 := fs2.Parse([]string{"-pt", "0"})
	if err2 != nil {
		t.Fatalf("unexpected flag parse error: %v", err2)
	}
	opts2, err2 := cliOpts2.ToRenderOptions(fs2, "")
	if err2 != nil {
		t.Fatalf("unexpected ToRenderOptions error: %v", err2)
	}
	img2, err2 := Render([]byte("test"), opts2)
	if err2 != nil {
		t.Fatalf("unexpected render error for explicit 0: %v", err2)
	}
	if img2 == nil {
		t.Fatalf("expected image, got nil")
	}
}

func TestRegressionIssues(t *testing.T) {
	// 1. Omitted/explicit -pt 0 for LoadFonts
	fonts, err := LoadFonts(FontConfig{SizeBase: 0})
	if err != nil {
		t.Fatalf("unexpected error for font size 0: %v", err)
	}
	if fonts.Regular == nil {
		t.Fatalf("expected fonts to be loaded for size 0 (fallback to 16)")
	}

	// 2. Max-int margin (overflow check)
	_, err = Render([]byte("test"), RenderOptions{Width: 100, Margin: 1 << 30})
	if err != ErrNoDrawableWidth {
		t.Errorf("expected ErrNoDrawableWidth for max-int margin, got %v", err)
	}

	// 3. Normal wide document succeeds despite huge ceiling
	img, err := Render([]byte("test"), RenderOptions{Width: 4096})
	if err != nil {
		t.Errorf("unexpected error for 4096 width: %v", err)
	}
	if img != nil && img.Bounds().Dx() != 4096 {
		t.Errorf("expected 4096 width, got %d", img.Bounds().Dx())
	}

	// 3b. Genuine huge allocation fails (via pixel budget panic conversion to error)
	// Test the pixel budget seam without massive allocations in memory
	origPixelBudget := maxTotalPixels
	maxTotalPixels = 100 * 100 // 10k pixels max
	defer func() { maxTotalPixels = origPixelBudget }()

	largeMD := "test\n\ntest\n\ntest\n\ntest\n\ntest\n\ntest"
	_, err = Render([]byte(largeMD), RenderOptions{Width: 100, MaxHeight: 32768})
	if err == nil {
		t.Errorf("expected error for genuine huge allocation, got nil")
	} else if !strings.Contains(err.Error(), "pixel budget") {
		t.Errorf("expected pixel budget error, got %v", err)
	}

	// 4. NaN base font size
	_, err = Render([]byte("test"), RenderOptions{BaseFontSize: math.NaN()})
	if err != ErrInvalidFontSize {
		t.Errorf("expected ErrInvalidFontSize for NaN, got %v", err)
	}

	// 5. Negative MaxHeight
	_, err = Render([]byte("test"), RenderOptions{MaxHeight: -10})
	if err != ErrInvalidMaxHeight {
		t.Errorf("expected ErrInvalidMaxHeight, got %v", err)
	}
}
