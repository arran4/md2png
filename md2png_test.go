package md2png

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrapLinesPreservesIndentation(t *testing.T) {
	fonts, err := LoadFonts(FontConfig{SizeBase: 14})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	text := "    spaced  out"
	lines := wrapLines(fonts.Mono, 14, text, 140)
	if len(lines) == 0 {
		t.Fatalf("expected at least one line")
	}
	if !strings.HasPrefix(lines[0], "    ") {
		t.Fatalf("expected leading spaces to be preserved, got %q", lines[0])
	}
	joined := strings.Join(lines, "")
	if !strings.Contains(joined, "  out") {
		t.Fatalf("expected double spaces inside wrapped lines to be preserved, got %q", joined)
	}

	long := "averyverylongtokenwithoutspaces"
	longLines := wrapLines(fonts.Mono, 14, long, 80)
	if len(longLines) < 2 {
		t.Fatalf("expected long token to wrap across multiple lines, got %v", longLines)
	}
}

func TestRenderHandlesTablesAndUnsupported(t *testing.T) {
	markdown := `# Title

Paragraph text before list.

- Item one
  - Nested bullet

1. First ordered item
2. Second ordered item
   1. Nested ordered item

| A | B |
| --- | --- |
| 1 | 2 |
| 3 | 4 |

::: custom
Unsupported block
:::
`

	img, err := Render([]byte(markdown), RenderOptions{})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected image result")
	}
	if img.Bounds() == (image.Rectangle{}) {
		t.Fatalf("expected non-empty bounds")
	}
}

func TestRenderListInlineFormatting(t *testing.T) {
	markdown := "- **bold text** with `iiWW` and a [link](https://example.com)\n"
	img, err := Render([]byte(markdown), RenderOptions{Width: 640, Margin: 48, BaseFontSize: 18})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	bounds := img.Bounds()
	foundLinkPixel := false
	for y := bounds.Min.Y; y < bounds.Max.Y && !foundLinkPixel; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if uint8(r>>8) == linkColor.R && uint8(g>>8) == linkColor.G && uint8(b>>8) == linkColor.B && uint8(a>>8) == linkColor.A {
				foundLinkPixel = true
				break
			}
		}
	}
	if !foundLinkPixel {
		t.Fatalf("expected rendered list item to include link styling pixel")
	}
}

func TestRendererFootnoteCollection(t *testing.T) {
	fonts, err := LoadFonts(FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	c := newCanvas(640, 48, lightTheme, fonts, 16)
	r := &renderer{
		c:              c,
		baseSize:       16,
		linkFootnotes:  true,
		imageFootnotes: true,
	}
	r.ensureImageResolvers()
	markdown := []byte("First [link](https://example.com) and second [same](https://example.com) ![img](https://example.com/image.png)")
	if err := r.render(markdown); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(r.footnotes) != 2 {
		t.Fatalf("expected two unique footnotes, got %d", len(r.footnotes))
	}
	if idx := r.footnoteIndex["https://example.com"]; idx != 1 {
		t.Fatalf("expected shared link footnote to keep first index, got %d", idx)
	}
}

func TestRendererFootnoteToggles(t *testing.T) {
	fonts, err := LoadFonts(FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	c := newCanvas(640, 48, lightTheme, fonts, 16)
	r := &renderer{
		c:              c,
		baseSize:       16,
		linkFootnotes:  false,
		imageFootnotes: true,
	}
	r.ensureImageResolvers()
	markdown := []byte("[link](https://example.com) ![img](https://example.com/image.png)")
	if err := r.render(markdown); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(r.footnotes) != 1 {
		t.Fatalf("expected only image footnote when link footnotes disabled, got %d", len(r.footnotes))
	}
	if _, ok := r.footnoteIndex["https://example.com"]; ok {
		t.Fatalf("did not expect plain link footnote when disabled")
	}
}

func TestRenderFootnoteDefaults(t *testing.T) {
	markdown := "Paragraph with a [link](https://example.com)."
	imgWith, err := Render([]byte(markdown), RenderOptions{})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	disable := false
	imgWithout, err := Render([]byte(markdown), RenderOptions{LinkFootnotes: &disable})
	if err != nil {
		t.Fatalf("render without footnotes failed: %v", err)
	}
	if imgWith.Bounds().Dy() <= imgWithout.Bounds().Dy() {
		t.Fatalf("expected default link footnotes to increase image height")
	}

	markdownImage := "![alt](https://example.com/image.png)"
	imgDefault, err := Render([]byte(markdownImage), RenderOptions{})
	if err != nil {
		t.Fatalf("render default image failed: %v", err)
	}
	enable := true
	imgWithImages, err := Render([]byte(markdownImage), RenderOptions{ImageFootnotes: &enable})
	if err != nil {
		t.Fatalf("render with image footnotes failed: %v", err)
	}
	if imgWithImages.Bounds().Dy() <= imgDefault.Bounds().Dy() {
		t.Fatalf("expected enabling image footnotes to increase image height")
	}
}

func TestRenderEmbedsLocalImage(t *testing.T) {
	tmpDir := t.TempDir()
	block := image.NewRGBA(image.Rect(0, 0, 40, 20))
	draw.Draw(block, block.Bounds(), image.NewUniform(color.RGBA{R: 0xCC, G: 0x22, B: 0x22, A: 0xFF}), image.Point{}, draw.Src)
	imgPath := filepath.Join(tmpDir, "block.png")
	file, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("create temp image: %v", err)
	}
	if err := png.Encode(file, block); err != nil {
		file.Close()
		t.Fatalf("encode temp image: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp image: %v", err)
	}

	markdown := fmt.Sprintf("![local image](%s)", filepath.Base(imgPath))
	rendered, err := Render([]byte(markdown), RenderOptions{BaseDir: tmpDir, Width: 200, Margin: 24})
	if err != nil {
		t.Fatalf("render with local image failed: %v", err)
	}

	want := color.RGBA{R: 0xCC, G: 0x22, B: 0x22, A: 0xFF}
	bounds := rendered.Bounds()
	found := false
	for y := bounds.Min.Y + 24; y < bounds.Max.Y-24 && !found; y++ {
		for x := bounds.Min.X + 24; x < bounds.Max.X-24; x++ {
			r, g, b, a := rendered.At(x, y).RGBA()
			if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(b>>8) == want.B && uint8(a>>8) == want.A {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected rendered output to include embedded image pixels")
	}
}

func TestRenderEmbedsRemoteImage(t *testing.T) {
	block := image.NewRGBA(image.Rect(0, 0, 20, 12))
	want := color.RGBA{R: 0x20, G: 0x80, B: 0xCC, A: 0xFF}
	draw.Draw(block, block.Bounds(), image.NewUniform(want), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, block); err != nil {
		t.Fatalf("encode sample image: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	markdown := fmt.Sprintf("![remote](%s/sample.png)", srv.URL)
	rendered, err := Render([]byte(markdown), RenderOptions{Width: 220, Margin: 24})
	if err != nil {
		t.Fatalf("render with remote image failed: %v", err)
	}

	bounds := rendered.Bounds()
	found := false
	for y := bounds.Min.Y + 24; y < bounds.Max.Y-24 && !found; y++ {
		for x := bounds.Min.X + 24; x < bounds.Max.X-24; x++ {
			r, g, b, a := rendered.At(x, y).RGBA()
			if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(b>>8) == want.B && uint8(a>>8) == want.A {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected rendered output to include remote image pixels")
	}
}
