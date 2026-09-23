package md2png

import (
	"testing"
)

func TestTableAlignmentsAndNarrowWidth(t *testing.T) {
	md := []byte(`
| Left | Center | Right |
| :--- | :----: | ----: |
| L    | C      | R     |
| Long UnbrokenTextStringTestForNarrowWidthSupport | Many many words that will be heavily wrapped because they are very long and the table width might be narrow | R     |
`)
	policy := DefaultCLIImagePolicy()

	opts := RenderOptions{
		ImagePolicy: &policy,
		Width: 800,
	}
	img, err := Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed for normal width table: %v", err)
	}
	if img.Bounds().Dx() != 800 {
		t.Fatalf("expected width 800")
	}

	opts = RenderOptions{
		ImagePolicy: &policy,
		Width: 250,
		Margin: 10,
	}
	img2, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed for narrow width table: %v", err)
	}
	for _, diag := range img2.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			t.Fatalf("Table should fit in width 250 with margin 10, but got table_layout_impossible")
		}
	}
	imgNarrow := img2.Image
	if imgNarrow.Bounds().Dx() != 250 {
		t.Fatalf("expected width 250")
	}
	// Also prove table is rendered by checking that the center is not purely white
	centerNonWhite := false
	for y := imgNarrow.Bounds().Min.Y; y < imgNarrow.Bounds().Max.Y; y++ {
		for x := 100; x < 150; x++ {
			r, g, b, _ := imgNarrow.At(x, y).RGBA()
			if r < 0xff00 || g < 0xff00 || b < 0xff00 {
				centerNonWhite = true
				break
			}
		}
	}
	if !centerNonWhite {
		t.Fatalf("Table does not appear to be rendered, center is purely background")
	}

	// Test impossible layout (too narrow to fit borders + 1 char)
	opts = RenderOptions{
		ImagePolicy: &policy,
		Width: 100, Margin: 10,
	}
	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed for impossible width table: %v", err)
	}

	foundWarning := false
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("Expected table_layout_impossible diagnostic for very narrow table")
	}

	// Test strict mode error on impossible layout
	strictOpts := RenderOptions{
		ImagePolicy: &policy,
		Width: 100, Margin: 10,
		DiagnosticPolicy: &DiagnosticPolicy{
			FailOnUnsupported: true,
		},
	}
	_, err = Render(md, strictOpts)
	if err != ErrImpossibleTableLayout {
		t.Fatalf("Expected ErrImpossibleTableLayout in strict mode for impossible table, got: %v", err)
	}

	// Verify pixels to ensure table is rendered within bounds
	// For image with width 250, table should be bounded by margin 10.
	// So x < 10 and x >= 240 should be pure background (white or transparent).
	bounds := imgNarrow.Bounds()
	margin := 10
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		// Check left margin
		for x := bounds.Min.X; x < margin; x++ {
			r, g, b, _ := imgNarrow.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				// We expect white background
				// Actually, default background might be transparent or white depending on implementation,
				// but md2png draws white background for light theme.
				// Let's just check if it's white to prove it didn't draw black text/borders.
				if r < 0xff00 || g < 0xff00 || b < 0xff00 {
					t.Fatalf("Found non-white pixel in left margin at (%d, %d)", x, y)
				}
			}
		}
		// Check right margin
		for x := bounds.Max.X - margin; x < bounds.Max.X; x++ {
			r, g, b, _ := imgNarrow.At(x, y).RGBA()
			if r < 0xff00 || g < 0xff00 || b < 0xff00 {
				t.Fatalf("Found non-white pixel in right margin at (%d, %d)", x, y)
			}
		}
	}
}

func TestTableLayoutWithImages(t *testing.T) {
	md := []byte(`
| Image | Text |
| :--- | :--- |
| ![img](testdata/test_image.png) | Should fit because image scales |
`)

	policy := DefaultCLIImagePolicy()
	opts := RenderOptions{
		ImagePolicy: &policy,
		Width: 300,
	}
	img, err := Render(md, opts)
	if err != nil {
		t.Fatalf("Render failed for image table: %v", err)
	}
	if img == nil {
		t.Fatalf("Expected image")
	}

	// Check that we didn't fallback
	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			t.Fatalf("Image table should not trigger impossible layout because images can scale down")
		}
	}
}


func TestTableAlignmentPixels(t *testing.T) {
	md := []byte(`
| L | C | R |
| :--- | :----: | ----: |
| L | C | R |
`)
	policy := DefaultCLIImagePolicy()
	opts := RenderOptions{
		ImagePolicy: &policy,
		Width: 300,
		Margin: 10,
	}
	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	img := res.Image

	findTextX := func(minX, maxX, startY, endY int) (int, int) {
		firstX := maxX
		lastX := minX
		found := false
		for y := startY; y < endY; y++ {
			for x := minX; x < maxX; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 0x8000 && g < 0x8000 && b < 0x8000 {
					if x < firstX {
						firstX = x
					}
					if x > lastX {
						lastX = x
					}
					found = true
				}
			}
		}
		if !found {
			return -1, -1
		}
		return firstX, lastX
	}

	lxMin, _ := findTextX(15, 95, 80, 130)
	cxMin, cxMax := findTextX(105, 185, 80, 130)
	_, rxMax := findTextX(195, 285, 80, 130)

	if lxMin == -1 || cxMin == -1 || rxMax == -1 {
		t.Fatalf("Failed to find text in cells")
	}

	if lxMin > 30 {
		t.Errorf("Left aligned text is too far right: %d", lxMin)
	}

	cCenter := (105 + 185) / 2
	textCenter := (cxMin + cxMax) / 2
	if textCenter < cCenter - 15 || textCenter > cCenter + 15 {
		t.Errorf("Center aligned text is not centered: textCenter=%d vs cellCenter=%d", textCenter, cCenter)
	}

	if rxMax < 260 {
		t.Errorf("Right aligned text is too far left: %d", rxMax)
	}
}
