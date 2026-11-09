package md2png_test

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/arran4/md2png"
	"golang.org/x/image/font/gofont/goregular"
)

// systemFallbackFont returns the path to a system font suitable for CJK / Unicode fallback testing,
// or skips the test if not present.
func systemFallbackFont(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/usr/share/fonts/opentype/ipafont-gothic/ipag.ttf",
		"/usr/share/fonts/truetype/fonts-japanese-gothic.ttf",
		"/usr/share/fonts/opentype/unifont/unifont.otf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no system CJK / Unicode fallback font available")
	return ""
}

func TestFontLoading_Defaults(t *testing.T) {
	fonts, err := md2png.LoadFonts(md2png.FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatalf("LoadFonts default failed: %v", err)
	}

	if fonts.Regular == nil || fonts.Bold == nil || fonts.Mono == nil {
		t.Fatal("expected Regular, Bold, Mono fonts to be initialized")
	}
	if fonts.Italic == nil || fonts.BoldItalic == nil {
		t.Fatal("expected Italic, BoldItalic fonts to be initialized")
	}

	// Verify legacy .Font field is populated for backwards compatibility
	if fonts.Regular.Font == nil {
		t.Error("expected legacy .Font to be populated on Regular")
	}
	// Verify modern .SFNT and .Face are populated
	if fonts.Regular.SFNT == nil {
		t.Error("expected modern .SFNT to be populated on Regular")
	}
	if fonts.Regular.Face == nil {
		t.Error("expected .Face to be populated on Regular")
	}
}

func TestFontLoading_CustomAndErrors(t *testing.T) {
	tempDir := t.TempDir()

	// Missing font file error
	_, err := md2png.LoadFonts(md2png.FontConfig{
		RegularPath: filepath.Join(tempDir, "nonexistent.ttf"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent font file, got nil")
	}

	// Malformed font file error
	corruptPath := filepath.Join(tempDir, "corrupt.ttf")
	if err := os.WriteFile(corruptPath, []byte("not a real font file"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = md2png.LoadFonts(md2png.FontConfig{
		RegularPath: corruptPath,
	})
	if err == nil {
		t.Fatal("expected error for corrupt font file, got nil")
	}

	// Custom valid TTF font
	validPath := filepath.Join(tempDir, "custom.ttf")
	if err := os.WriteFile(validPath, goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	customFonts, err := md2png.LoadFonts(md2png.FontConfig{
		RegularPath: validPath,
	})
	if err != nil {
		t.Fatalf("failed to load valid custom font: %v", err)
	}
	if customFonts.Regular == nil {
		t.Fatal("expected custom Regular font to be loaded")
	}
}

func TestFontFallback_GlyphSelection(t *testing.T) {
	fbPath := systemFallbackFont(t)

	cfg := md2png.FontConfig{
		SizeBase:      16,
		FallbackPaths: []string{fbPath},
	}
	fonts, err := md2png.LoadFonts(cfg)
	if err != nil {
		t.Fatalf("LoadFonts with fallback failed: %v", err)
	}

	if len(fonts.Regular.Fallbacks) != 1 {
		t.Fatalf("expected 1 fallback font on Regular, got %d", len(fonts.Regular.Fallbacks))
	}

	// Latin letter 'A' should be found in primary font
	selectedA := fonts.Regular.FindFaceForRune('A')
	if selectedA != fonts.Regular {
		t.Errorf("expected 'A' to use primary Regular font, got fallback")
	}

	// Find a rune present in fallback but absent from primary Regular
	var fallbackRune rune
	testRunes := []rune{'你', '世', '界', 'あ', 'Ω', 'ж', 'א', 'م', '✓', '★', '→'}
	for _, r := range testRunes {
		if !fonts.Regular.HasGlyph(r) && fonts.Regular.Fallbacks[0].HasGlyph(r) {
			fallbackRune = r
			break
		}
	}
	if fallbackRune == 0 {
		for r := rune(0x0100); r < 0xFFFF; r++ {
			if !fonts.Regular.HasGlyph(r) && fonts.Regular.Fallbacks[0].HasGlyph(r) {
				fallbackRune = r
				break
			}
		}
	}
	if fallbackRune == 0 {
		t.Skip("fallback font contains no glyphs absent from primary font")
	}

	selectedFallback := fonts.Regular.FindFaceForRune(fallbackRune)
	if selectedFallback == fonts.Regular {
		t.Errorf("expected %q to use fallback font, got primary Regular", fallbackRune)
	}
	if selectedFallback != fonts.Regular.Fallbacks[0] {
		t.Errorf("expected %q to match fallback font instance", fallbackRune)
	}
}

func TestFontFallback_MixedLatinCJKRender(t *testing.T) {
	fbPath := systemFallbackFont(t)

	md := []byte(`# Markdown with CJK / 日本語

This is standard English text mixed with Chinese (你好世界) and Japanese (こんにちは世界).

| English | Chinese | Japanese |
|---------|---------|----------|
| Hello   | 你好    | こんにちは |
| World   | 世界    | 世界     |

` + "```go\n// Code comment with Unicode: 漢字\nconst Greeting = \"こんにちは\"\n```\n")

	opts := md2png.RenderOptions{
		Width:         800,
		BaseFontSize:  16,
		FontFallbacks: []string{fbPath},
	}

	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render with mixed Latin+CJK failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected rendered image, got nil")
	}
	if img.Bounds().Dx() != 800 {
		t.Errorf("expected width 800, got %d", img.Bounds().Dx())
	}
	if img.Bounds().Dy() <= 0 {
		t.Errorf("expected positive height, got %d", img.Bounds().Dy())
	}
}

func TestFontFallback_CombiningMarks(t *testing.T) {
	fbPath := systemFallbackFont(t)

	// Test precomposed vs combining mark
	md := []byte("Precomposed: café, naïve, résumé.\nCombining: cafe\u0301, nai\u0308ve, re\u0301sume\u0301.")

	opts := md2png.RenderOptions{
		Width:         600,
		FontFallbacks: []string{fbPath},
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render with combining marks failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected rendered image, got nil")
	}
}

func TestFontFallback_EmojiBehavior(t *testing.T) {
	// Emoji behavior requirement:
	// "Define initial emoji behaviour explicitly (for example monochrome fallback
	// or unsupported colour emoji) instead of silently promising colour-emoji support."
	// Color emoji tables are unsupported by vector TTF rasterizer; ensure it degrades safely without error or crash.
	md := []byte("# Emoji Test\n\nTesting rocket 🚀, sparkle ✨, and celebration 🎉 in body text.")

	img, err := md2png.Render(md, md2png.RenderOptions{Width: 600})
	if err != nil {
		t.Fatalf("expected rendering text with emoji to succeed gracefully without error, got: %v", err)
	}
	if img == nil {
		t.Fatal("expected rendered image, got nil")
	}
}

func TestFont_ScaledHeadingsAndCode(t *testing.T) {
	md := []byte(`
# Heading 1 (Large)
## Heading 2 (Medium)
### Heading 3 (Smaller)

Paragraph at standard 16pt size.

` + "```\ncode block at mono size\n```")

	opts := md2png.RenderOptions{
		Width:        640,
		BaseFontSize: 16,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("render headings and code failed: %v", err)
	}
	if img.Bounds().Dy() < 200 {
		t.Errorf("expected height > 200, got %d", img.Bounds().Dy())
	}
}

func TestFont_StylePreservation(t *testing.T) {
	fonts, err := md2png.LoadFonts(md2png.FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatalf("LoadFonts failed: %v", err)
	}

	// Verify distinct styles use distinct font faces
	if fonts.Regular.SFNT == fonts.Bold.SFNT {
		t.Error("expected Regular and Bold to have distinct SFNT fonts")
	}
	if fonts.Regular.SFNT == fonts.Mono.SFNT {
		t.Error("expected Regular and Mono to have distinct SFNT fonts")
	}
	if fonts.Regular.SFNT == fonts.Italic.SFNT {
		t.Error("expected Regular and Italic to have distinct SFNT fonts")
	}
	if fonts.Bold.SFNT == fonts.BoldItalic.SFNT {
		t.Error("expected Bold and BoldItalic to have distinct SFNT fonts")
	}
}

func TestFontFallback_WrappingAcrossFallback(t *testing.T) {
	fbPath := systemFallbackFont(t)

	// Long line with mixed script that forces line wrapping
	md := []byte("EnglishWord EnglishWord 漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字 EnglishWord EnglishWord 日本語日本語日本語日本語日本語")

	opts := md2png.RenderOptions{
		Width:         300,
		Margin:        20,
		BaseFontSize:  16,
		FontFallbacks: []string{fbPath},
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("render wrapped fallback text failed: %v", err)
	}
	if img.Bounds().Dy() < 80 {
		t.Errorf("expected multi-line wrapped text to have height >= 80, got %d", img.Bounds().Dy())
	}
}

func TestFont_BackwardsCompatibilitySeam(t *testing.T) {
	fonts, err := md2png.LoadFonts(md2png.FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatal(err)
	}

	// Ensure callers relying on .Font and .Face still get valid objects
	if fonts.Regular.Font == nil {
		t.Error("expected fonts.Regular.Font (*truetype.Font) to be non-nil")
	}
	if fonts.Regular.Face == nil {
		t.Error("expected fonts.Regular.Face (font.Face) to be non-nil")
	}
	if _, ok := any(fonts.Regular.Face).(image.Image); ok {
		t.Error("unexpected type assertion")
	}
}
