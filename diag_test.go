package md2png

import (
	"strings"
	"testing"
)

func TestDiagnostics(t *testing.T) {
	md := []byte("Testing <br> unsupported and missing image ![alt](missing.png)")

	t.Run("Warning only render", func(t *testing.T) {
		opts := RenderOptions{}
		res, err := RenderWithDiagnostics(md, opts)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if res.Image == nil {
			t.Fatal("Expected image, got nil")
		}

		if len(res.Diagnostics) == 0 {
			t.Fatal("Expected diagnostics, got none")
		}
	})

	t.Run("Strict HTML failure", func(t *testing.T) {
		opts := RenderOptions{
			DiagnosticPolicy: &DiagnosticPolicy{
				FailOnRawHTML: true,
			},
		}
		_, err := RenderWithDiagnostics(md, opts)
		if err == nil {
			t.Fatal("Expected error due to strict raw HTML policy, got nil")
		}
		if !strings.Contains(err.Error(), "Raw HTML is intentionally stripped") {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("Strict image failure", func(t *testing.T) {
		mdImg := []byte("![alt](missing.png)")
		opts := RenderOptions{
			DiagnosticPolicy: &DiagnosticPolicy{
				FailOnImageError: true,
			},
		}
		_, err := RenderWithDiagnostics(mdImg, opts)
		if err == nil {
			t.Fatal("Expected error due to strict image error policy, got nil")
		}
		if !strings.Contains(err.Error(), "no such file or directory") && !strings.Contains(err.Error(), "Failed to load") {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("Diagnostic order and codes", func(t *testing.T) {
		mdHtmlImg := []byte("A <br> B ![alt](missing.png)")
		opts := RenderOptions{}
		res, err := RenderWithDiagnostics(mdHtmlImg, opts)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundHtml := false
		foundImg := false
		for _, d := range res.Diagnostics {
			if d.Code == DiagRawHTML {
				foundHtml = true
			}
			if d.Code == DiagImageLoadFailed {
				foundImg = true
			}
		}
		if !foundHtml || !foundImg {
			t.Fatalf("Expected raw HTML and Image load failed diagnostics, got %v", res.Diagnostics)
		}
	})
}

func TestRenderSetupRegression(t *testing.T) {
	md := []byte("Hello")

	t.Run("Empty RenderOptions does not panic", func(t *testing.T) {
		opts := RenderOptions{}
		_, err := RenderWithDiagnostics(md, opts)
		if err != nil {
			t.Fatalf("RenderWithDiagnostics with empty opts failed: %v", err)
		}

		_, err = Render(md, opts)
		if err != nil {
			t.Fatalf("Render with empty opts failed: %v", err)
		}
	})

	t.Run("Partial Fonts Fallback", func(t *testing.T) {
		baseFont, _ := LoadFonts(FontConfig{})
		opts := RenderOptions{
			Fonts: Fonts{
				Regular: baseFont.Regular,
				// Missing Bold and Mono should be filled in by setup path
			},
		}

		_, err := RenderWithDiagnostics(md, opts)
		if err != nil {
			t.Fatalf("RenderWithDiagnostics failed with partial fonts: %v", err)
		}
	})
}
