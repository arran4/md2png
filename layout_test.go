package md2png

import (
	"image/color"
	"testing"
)

func TestLayoutDocument_IndependentFromPainting(t *testing.T) {
	md := `# Heading 1

This is a paragraph with [a link](https://example.com) and some text.

` + "```go" + `
func main() {}
` + "```" + `

* List item 1
* List item 2

---

> A blockquote
`
	res, err := LayoutDocument([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("layout failed: %v", err)
	}

	dl := res.DisplayList
	if dl == nil {
		t.Fatalf("expected non-nil display list")
	}
	if dl.Width != 800 {
		t.Fatalf("expected width 800, got %d", dl.Width)
	}
	if dl.Height <= 0 {
		t.Fatalf("expected height > 0, got %d", dl.Height)
	}
	if len(dl.Items) == 0 {
		t.Fatalf("expected display items to be recorded")
	}

	// Verify different kinds of items are present
	var hasText, hasRect bool
	for _, item := range dl.Items {
		switch item.Kind {
		case ItemText:
			hasText = true
			if item.Text == "" {
				t.Fatalf("expected non-empty text in ItemText")
			}
			if item.Point.Y <= 0 {
				t.Fatalf("expected positive baseline Y coordinate")
			}
		case ItemRect:
			hasRect = true
			if item.Rect.Empty() {
				t.Fatalf("expected non-empty rect in ItemRect")
			}
		}
	}
	if !hasText {
		t.Fatalf("expected ItemText in display list")
	}
	if !hasRect {
		t.Fatalf("expected ItemRect (e.g. code background or HRule) in display list")
	}

	// Verify RasterPainter paints the DisplayList accurately
	painter := &RasterPainter{}
	img, err := painter.Paint(dl)
	if err != nil {
		t.Fatalf("painter failed: %v", err)
	}
	if img.Bounds().Dx() != dl.Width || img.Bounds().Dy() != dl.Height {
		t.Fatalf("painted image bounds %v did not match display list %dx%d",
			img.Bounds(), dl.Width, dl.Height)
	}
}

func TestLayoutDocument_TableMetrics(t *testing.T) {
	md := `| Col A | Col B |
|---|---|
| Value 1 | Value 2 |
`
	res, err := LayoutDocument([]byte(md), RenderOptions{
		Width: 600,
	})
	if err != nil {
		t.Fatalf("table layout failed: %v", err)
	}
	if len(res.DisplayList.Items) == 0 {
		t.Fatalf("expected table display items")
	}

	// Verify table borders exist as ItemRect
	var rectCount int
	for _, item := range res.DisplayList.Items {
		if item.Kind == ItemRect {
			rectCount++
		}
	}
	if rectCount < 3 {
		t.Fatalf("expected at least 3 border rectangles for table, got %d", rectCount)
	}
}

func TestLayoutDocument_ResourceLimitHandling(t *testing.T) {
	md := `# Huge Title`
	_, err := LayoutDocument([]byte(md), RenderOptions{
		Width:     800,
		MaxHeight: 10, // too small
	})
	if err == nil {
		t.Fatalf("expected resource limit error for MaxHeight = 10")
	}
}

func TestDisplayList_AddHelpers(t *testing.T) {
	dl := &DisplayList{Width: 400, Height: 400}
	dl.AddRect(dl.Bounds(), color.White)
	dl.AddText("Hello", nil, FontStyleRegular, 16, color.Black, dl.Bounds().Min) // nil font ignored safely
	dl.AddImage(nil, dl.Bounds())                                                // nil image ignored safely
	dl.AddLine(dl.Bounds().Min, dl.Bounds().Max, nil, 1)                         // nil color ignored safely

	if len(dl.Items) != 1 {
		t.Fatalf("expected only 1 valid item added, got %d", len(dl.Items))
	}
}

func TestLayoutDocument_TaskCheckBoxAndFormatting(t *testing.T) {
	md := `- [ ] Unchecked
- [x] Checked with ~~strikethrough~~ and **bold**
`
	res, err := LayoutDocument([]byte(md), RenderOptions{
		Width: 600,
	})
	if err != nil {
		t.Fatalf("layout failed: %v", err)
	}
	dl := res.DisplayList
	var hasLine bool
	var hasBoldStyle bool
	for _, item := range dl.Items {
		if item.Kind == ItemLine {
			hasLine = true
		}
		if item.Kind == ItemText && item.FontStyle == FontStyleBold {
			hasBoldStyle = true
		}
	}
	if !hasLine {
		t.Errorf("expected ItemLine for checked task box")
	}
	if !hasBoldStyle {
		t.Errorf("expected FontStyleBold for bold text run")
	}

	painter := &RasterPainter{}
	img, err := painter.Paint(dl)
	if err != nil {
		t.Fatalf("Paint failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected rendered image, got nil")
	}
}
