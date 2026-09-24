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
	// A table with unequal width content to prove the algorithm works and alignments hold
	md := []byte(`
| Left | CenterCellText | R |
| :--- | :----: | ----: |
| L | C | RightAlignedCellText |
`)
	policy := DefaultCLIImagePolicy()
	opts := RenderOptions{
		ImagePolicy: &policy,
		Width: 500,
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

	// We need to find the table bounding box.
	// Since we know the background is pure white (0xffff, 0xffff, 0xffff) in default light theme,
	// and borders are drawn with HRule color (grey), we can find vertical borders.
	// We scan the horizontal lines to find the top of the table.
	topBorderY := -1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		// table border usually starts at margin
		r, g, b, _ := img.At(10, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			topBorderY = y
			break
		}
	}
	if topBorderY == -1 {
		t.Fatalf("Could not find table top border")
	}

	// Find the 4 vertical borders
	var vBorders []int
	for x := 10; x < bounds.Max.X; x++ {
		r, g, b, _ := img.At(x, topBorderY+5).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			vBorders = append(vBorders, x)
		}
	}

	if len(vBorders) != 4 {
		t.Fatalf("Expected exactly 4 vertical borders, found %d: %v", len(vBorders), vBorders)
	}

	// We have header row and body row. Let's find the separator border.
	sepBorderY := -1
	for y := topBorderY + 1; y < bounds.Max.Y; y++ {
		r, g, b, _ := img.At(vBorders[0]+5, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			sepBorderY = y
			break
		}
	}
	if sepBorderY == -1 {
		t.Fatalf("Could not find header separator border")
	}

	// And the bottom border
	bottomBorderY := -1
	for y := sepBorderY + 1; y < bounds.Max.Y; y++ {
		r, g, b, _ := img.At(vBorders[0]+5, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			bottomBorderY = y
			break
		}
	}
	if bottomBorderY == -1 {
		t.Fatalf("Could not find bottom border")
	}

	// Helper to find text bounds within a cell
	findTextX := func(minX, maxX, startY, endY int) (int, int) {
		firstX := maxX
		lastX := minX
		found := false
		for y := startY; y < endY; y++ {
			for x := minX; x < maxX; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				// Text is dark, background is white.
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

	checkCell := func(rowName string, startY, endY int) {
		col0Min, col0Max := vBorders[0]+1, vBorders[1]-1
		col1Min, col1Max := vBorders[1]+1, vBorders[2]-1
		col2Min, col2Max := vBorders[2]+1, vBorders[3]-1

		lxMin, _ := findTextX(col0Min, col0Max, startY, endY)
		cxMin, cxMax := findTextX(col1Min, col1Max, startY, endY)
		_, rxMax := findTextX(col2Min, col2Max, startY, endY)

		if lxMin == -1 || cxMin == -1 || rxMax == -1 {
			t.Fatalf("[%s] Failed to find text in cells. lxMin=%d cxMin=%d rxMax=%d", rowName, lxMin, cxMin, rxMax)
		}

		expectedLeft := col0Min + 9
		if lxMin > expectedLeft + 5 {
			t.Errorf("[%s] Left aligned text is too far right: %d (expected ~%d)", rowName, lxMin, expectedLeft)
		}

		cellCenter := (col1Min + col1Max) / 2
		textCenter := (cxMin + cxMax) / 2
		if textCenter < cellCenter - 5 || textCenter > cellCenter + 5 {
			t.Errorf("[%s] Center aligned text is not centered: textCenter=%d vs cellCenter=%d", rowName, textCenter, cellCenter)
		}

		expectedRight := col2Max - 9
		if rxMax < expectedRight - 5 {
			t.Errorf("[%s] Right aligned text is too far left: %d (expected ~%d)", rowName, rxMax, expectedRight)
		}
	}

	checkCell("Header", topBorderY+1, sepBorderY-1)
	checkCell("Body", sepBorderY+1, bottomBorderY-1)
}

func TestTableMixedContent(t *testing.T) {
	// Mixed text, newlines and images in a cell
	md := []byte(`
| Mixed Cell |
| :--- |
| Line 1<br>![img](testdata/test_image.png)<br>Line 3 |
`)

	policy := DefaultCLIImagePolicy()
	opts := RenderOptions{
		ImagePolicy: &policy,
		Width: 300,
	}
	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			t.Fatalf("Table should fit but got table_layout_impossible")
		}
		if diag.Code == DiagnosticCode("image_load_failed") {
			t.Fatalf("Image failed to load: %s", diag.Message)
		}
	}
	if res.Image == nil {
		t.Fatalf("Expected image")
	}
}
