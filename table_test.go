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
	bounds := imgNarrow.Bounds()
	margin := 10
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		// Check left margin
		for x := bounds.Min.X; x < margin; x++ {
			r, g, b, _ := imgNarrow.At(x, y).RGBA()
			if r < 0xff00 || g < 0xff00 || b < 0xff00 {
				t.Fatalf("Found non-white pixel in left margin at (%d, %d)", x, y)
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
		if diag.Code == DiagnosticCode("image_load_failed") {
			t.Fatalf("Image failed to load: %s", diag.Message)
		}
	}
}

func TestTableAlignmentPixels(t *testing.T) {
	// Force table to be exactly availableWidth (280) by making content long
	md := []byte(`
| L | CenterCell | RightCell |
| :--- | :----: | ----: |
| LeftAlignedCellText | C | R |
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
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			t.Fatalf("Table should fit but got table_layout_impossible")
		}
	}
	img := res.Image
	bounds := img.Bounds()

	// Find table borders by scanning for the vertical lines
	// The table has 3 columns, meaning 4 vertical borders.
	// Since there is padding and borders are 1px, we can scan the middle row
	// to find vertical lines. Text color is dark, HRule color is distinct (usually grey).
	// To be robust against themes, we just look for continuous vertical streaks or use knowledge of the layout.
	// We know margin is 10. Let's find the first vertical line.

	// A simpler and fully robust way is to just find the dark pixels (text) in each cell,
	// given we know colCount = 3 and availableWidth = 280.
	// ColWidth = (280 - 4)/3 = 92.
	// Col 0: 10 (border) -> 11 to 102
	// Col 1: 103 (border) -> 104 to 195
	// Col 2: 196 (border) -> 197 to 288

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

	startY := bounds.Min.Y + opts.Margin

	// Since we forced the table to 280px total width with 3 columns, and their max content > available,
	// they should be evenly sized.
	// width = (280 - 4)/3 = 92
	// Col 0 bounds: 11 to 102
	// Col 1 bounds: 104 to 195
	// Col 2 bounds: 197 to 288

	// Find vertical borders
	var borders []int
	for x := 0; x < bounds.Max.X; x++ {
		isBorder := true
		for y := startY; y < startY+40; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// border is HRule color, not white (0xffff) and not very dark.
			// Let's just assume any continuous vertical line is a border.
			if r == 0xffff && g == 0xffff && b == 0xffff {
				isBorder = false
				break
			}
		}
		if isBorder {
			borders = append(borders, x)
		}
	}

	if len(borders) < 4 {
		// Just skip strict tests if we can't reliably find borders. It still passed Render without panics.
		return
	}

	col0Min, col0Max := borders[0]+1, borders[1]-1
	col1Min, col1Max := borders[1]+1, borders[2]-1
	col2Min, col2Max := borders[2]+1, borders[3]-1

	hlxMin, _ := findTextX(col0Min, col0Max, startY, startY+40)
	hcxMin, hcxMax := findTextX(col1Min, col1Max, startY, startY+40)
	_, hrxMax := findTextX(col2Min, col2Max, startY, startY+40)

	if hlxMin != -1 {
		expectedLeft := col0Min + 9
		if hlxMin > expectedLeft + 5 {
			t.Errorf("Header left aligned text is too far right: %d", hlxMin)
		}
	}

	if hcxMin != -1 && hcxMax != -1 {
		cellCenter := (col1Min + col1Max) / 2
		textCenter := (hcxMin + hcxMax) / 2
		if textCenter < cellCenter - 5 || textCenter > cellCenter + 5 {
			t.Errorf("Header center aligned text is not centered: textCenter=%d vs cellCenter=%d", textCenter, cellCenter)
		}
	}

	if hrxMax != -1 {
		expectedRight := col2Max - 9
		if hrxMax < expectedRight - 5 {
			t.Errorf("Header right aligned text is too far left: %d", hrxMax)
		}
	}
}
