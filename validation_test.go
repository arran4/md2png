package md2png

import (
	"flag"
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
	// Simulate the flag parsing block from main.go
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	width := fs.Int("width", 0, "Output image width in pixels (0 for default)")
	margin := fs.Int("margin", 0, "Margin in pixels (0 for default)")

	// Parse with explicit margin 0
	err := fs.Parse([]string{"-margin", "0", "-width", "800"})
	if err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}

	marginSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "margin" {
			marginSet = true
		}
	})

	if !marginSet {
		t.Errorf("expected margin to be recorded as set")
	}

	opts := RenderOptions{
		Width:      *width,
		Margin:     *margin,
		ZeroMargin: *margin == 0 && marginSet,
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
}
