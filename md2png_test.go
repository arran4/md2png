package md2png

import (
	"image"
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
