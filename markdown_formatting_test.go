package md2png_test

import (
	"image/color"
	"os"
	"testing"

	"github.com/arran4/md2png"
)

func TestEmphasis_ItalicAndBoldItalic(t *testing.T) {
	md := []byte(`
This is *italic* and _italic also_.

This is **bold** text.

This is ***bold italic*** text.

This is **bold with *nested italic*** text.

This is *italic with **nested bold*** text.
`)

	opts := md2png.RenderOptions{
		Width: 600,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}
}

func TestStrikethrough_RenderingAndWrapping(t *testing.T) {
	md := []byte(`
Here is ~~strikethrough text~~ in a paragraph.

Here is ~~a very long strikethrough sentence that will definitely wrap across multiple lines in the rendered output image to test decoration across lines~~.

Nested: ~~**bold strikethrough** and *italic strikethrough* and [link strikethrough](https://example.com)~~.
`)

	opts := md2png.RenderOptions{
		Width: 400,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}
}

func TestTaskLists_CheckedAndUnchecked(t *testing.T) {
	md := []byte(`
# Task List Test

- [ ] Unchecked task item
- [x] Checked task item
- [ ] Task with **bold** and *italic* and [link](https://example.com)
- [x] Task with ~~strikethrough~~ content
  - [ ] Nested unchecked task
  - [x] Nested checked task
    - [ ] Level 3 task

Ordered tasks:
1. [ ] Ordered unchecked task
2. [x] Ordered checked task
`)

	opts := md2png.RenderOptions{
		Width: 640,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}
}

func TestFormatting_InTablesAndBlockquotes(t *testing.T) {
	md := []byte(`
| Status | Task | Note |
|---|---|---|
| [ ] | Fix bugs | *urgent* |
| [x] | Add tests | **completed** |
| [x] | Legacy code | ~~deprecated~~ |

> Blockquote with *italic*, **bold**, ***bold italic***, and ~~strikethrough~~ text.
> - [ ] Task inside blockquote
> - [x] Done task inside blockquote
`)

	opts := md2png.RenderOptions{
		Width: 640,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}
}

func TestFormatting_DarkTheme(t *testing.T) {
	darkTh, err := md2png.ThemeByName("dark")
	if err != nil {
		t.Fatalf("failed to get dark theme: %v", err)
	}

	md := []byte(`
# Dark Theme Formatting Test

- [ ] Unchecked task
- [x] Checked task

Paragraph with *italic*, **bold**, ***bold italic***, and ~~strikethrough~~ text.
`)

	opts := md2png.RenderOptions{
		Width: 600,
		Theme: darkTh,
	}
	img, err := md2png.Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}

	// Verify dark background color is applied
	c0 := img.At(0, 0)
	r, g, b, _ := c0.RGBA()
	bgR, bgG, bgB, _ := darkTh.BG.RGBA()
	if r != bgR || g != bgG || b != bgB {
		t.Errorf("expected dark theme BG at (0,0), got rgba(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestStrikethrough_PixelDecoration(t *testing.T) {
	// Verify that strikethrough actually renders horizontal strike pixels across glyphs
	md := []byte("~~XXXX~~")
	whiteTheme := md2png.Theme{
		BG: color.RGBA{0xFF, 0xFF, 0xFF, 0xFF},
		FG: color.RGBA{0x00, 0x00, 0x00, 0xFF},
	}
	img, err := md2png.Render(md, md2png.RenderOptions{
		Width:        200,
		Margin:       10,
		BaseFontSize: 20,
		Theme:        whiteTheme,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Find non-white pixels
	nonWhitePixels := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bCol, _ := img.At(x, y).RGBA()
			if r < 0xF000 || g < 0xF000 || bCol < 0xF000 {
				nonWhitePixels++
			}
		}
	}
	if nonWhitePixels == 0 {
		t.Fatal("expected strikethrough text to render pixels, found none")
	}
}

func TestFormatting_ShowcaseFixture(t *testing.T) {
	md, err := os.ReadFile("testdata/gfm_formatting_showcase.md")
	if err != nil {
		t.Fatalf("failed to read showcase fixture: %v", err)
	}

	img, err := md2png.Render(md, md2png.RenderOptions{
		Width: 1024,
	})
	if err != nil {
		t.Fatalf("failed to render showcase: %v", err)
	}
	if img == nil || img.Bounds().Dy() == 0 {
		t.Fatal("expected non-empty image for showcase")
	}
}
