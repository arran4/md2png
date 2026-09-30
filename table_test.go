package md2png

import (
	"fmt"
	"image"
	"image/png"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
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
		Margin:      10,
		ImagePolicy: &policy,
		Width:       800,
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
		Width:       250,
		Margin:      10,
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
		Width:       100, Margin: 10,
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
		Width:       100, Margin: 10,
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
		Width:       300,
		Margin:      10,
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
		Width:       500,
		Margin:      10,
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
		if lxMin > expectedLeft+5 {
			t.Errorf("[%s] Left aligned text is too far right: %d (expected ~%d)", rowName, lxMin, expectedLeft)
		}

		cellCenter := (col1Min + col1Max) / 2
		textCenter := (cxMin + cxMax) / 2
		if textCenter < cellCenter-5 || textCenter > cellCenter+5 {
			t.Errorf("[%s] Center aligned text is not centered: textCenter=%d vs cellCenter=%d", rowName, textCenter, cellCenter)
		}

		expectedRight := col2Max - 9
		if rxMax < expectedRight-5 {
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
		Margin:      10,
		ImagePolicy: &policy,
		Width:       300,
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

	// We will assert the layout geometry explicitly. Line 1 should be above the image, and Line 3 below it.
	img := res.Image
	bounds := img.Bounds()

	// Find top border
	topBorderY := -1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		r, g, b, _ := img.At(10, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			topBorderY = y
			break
		}
	}
	if topBorderY == -1 {
		t.Fatalf("Could not find table top border")
	}

	// Find vertical borders
	var vBorders []int
	for x := 10; x < bounds.Max.X; x++ {
		r, g, b, _ := img.At(x, topBorderY+5).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			vBorders = append(vBorders, x)
		}
	}
	if len(vBorders) < 2 {
		t.Fatalf("Expected at least 2 vertical borders")
	}

	// Find the separator border
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

	// Find bottom border
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

	// Scan cell body from sepBorderY+1 to bottomBorderY-1 for content rows
	var contentY []int
	for y := sepBorderY + 1; y < bottomBorderY; y++ {
		hasContent := false
		for x := vBorders[0] + 1; x < vBorders[1]; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Image colors may be diverse, so anything not white is content
			if r < 0xffff || g < 0xffff || b < 0xffff {
				hasContent = true
				break
			}
		}
		if hasContent {
			contentY = append(contentY, y)
		}
	}

	type blob struct{ min, max int }
	var blobs []blob
	if len(contentY) > 0 {
		current := blob{min: contentY[0], max: contentY[0]}
		for i := 1; i < len(contentY); i++ {
			if contentY[i] <= current.max+3 {
				current.max = contentY[i]
			} else {
				blobs = append(blobs, current)
				current = blob{min: contentY[i], max: contentY[i]}
			}
		}
		blobs = append(blobs, current)
	}

	if len(blobs) < 3 {
		t.Fatalf("Expected at least 3 distinct vertical content blocks (Line1, Image, Line3), found %d", len(blobs))
	}

	// The image is the middle blob and should be significantly taller than a single text line
	imgHeight := blobs[1].max - blobs[1].min
	if imgHeight < 20 {
		t.Fatalf("Expected image block to be taller than 20px, but it was %dpx", imgHeight)
	}
}

func TestTrailingSpaceAlignment(t *testing.T) {
	// A narrow right-aligned column with a long wrapped string that ends exactly near the boundary,
	// followed by a space, which could shift alignment leftwards.
	md := []byte(`
| Right |
| ----: |
| XXXXXXXXXXXXXXXXXX |
`)

	policy := DefaultCLIImagePolicy()
	opts := RenderOptions{
		ImagePolicy: &policy,
		Width:       200,
		Margin:      10,
	}
	_, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	// XXXXXXXXXXXXXXXXXX wrapped will have spaces if we had spaces, but it's unbreakable!
	// Let's use a breakable string with spaces near the wrap boundary.
	md2 := []byte(`
| Right |
| ----: |
| AAAAA BBBBB CCCCC |
`)
	res2, err := RenderWithDiagnostics(md2, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Now we actively verify the alignment!
	// We want to ensure the text in res2 is actually anchored to the right border.
	img := res2.Image
	bounds := img.Bounds()

	// Find top border
	topBorderY := -1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		r, g, b, _ := img.At(10, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			topBorderY = y
			break
		}
	}
	if topBorderY == -1 {
		t.Fatalf("Could not find table top border")
	}

	// Find vertical borders
	var vBorders []int
	for x := 10; x < bounds.Max.X; x++ {
		r, g, b, _ := img.At(x, topBorderY+5).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			vBorders = append(vBorders, x)
		}
	}
	if len(vBorders) < 2 {
		t.Fatalf("Expected at least 2 vertical borders")
	}

	// Find the separator border
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

	// Check text X in body
	// The text is "AAAAA BBBBB CCCCC". It is wrapped.
	// We just find the maximum X of any dark pixel in the cell.
	rightBorder := vBorders[1]
	maxX := -1
	for y := sepBorderY + 1; y < sepBorderY+40; y++ {
		for x := vBorders[0] + 1; x < rightBorder; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0x8000 && g < 0x8000 && b < 0x8000 {
				if x > maxX {
					maxX = x
				}
			}
		}
	}

	if maxX == -1 {
		t.Fatalf("No text found in trailing space alignment test")
	}

	// Ensure that no content breaches the right border (or gets drawn off canvas)
	for y := sepBorderY + 1; y < sepBorderY+40; y++ {
		for x := rightBorder + 1; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				t.Fatalf("Content overflowed past the right border at (%d, %d)", x, y)
			}
		}
	}

	// Padding is ~9. Expect maxX to be very close to rightBorder - 9.
	expectedMaxX := rightBorder - 9
	if maxX < expectedMaxX-5 {
		t.Fatalf("Right-aligned text with trailing spaces shifted too far left! Expected near %d, got %d", expectedMaxX, maxX)
	}
}

func TestTableFitWidth(t *testing.T) {
	policy := DefaultCLIImagePolicy()

	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			t.Fatalf("Image loaded more than once!")
		}

		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, img); err != nil {
			t.Errorf("Failed to encode PNG: %v", err)
		}
	}))
	defer ts.Close()

	md := []byte(fmt.Sprintf(`
Some normal text that wraps to the requested width.

| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |
| -- | -- | -- | -- | -- | -- | -- | -- | -- | --- |
| 1  | 2  | 3  | 4  | 5  | 6  | 7  | 8  | 9  | ![img](%s) |
`, ts.URL))

	opts := RenderOptions{
		Width:         200,
		Margin:        10,
		TableFitWidth: true,
		ImagePolicy:   &policy,
	}

	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("table_layout_impossible") {
			t.Fatalf("Expected table to fit with TableFitWidth, got table_layout_impossible")
		}
	}

	if res.Image == nil {
		t.Fatalf("Expected image")
	}

	bounds := res.Image.Bounds()
	if bounds.Dx() <= 200 || bounds.Dx() > 1500 {
		t.Fatalf("Expected canvas width to be larger than 200 but reasonable, got %d", bounds.Dx())
	}
	if requestCount != 1 {
		t.Fatalf("Expected exactly 1 request to image server, got %d", requestCount)
	}

	// Test resource limits integration
	opts.Width = 200
	opts.TableFitWidth = true
	// Force a huge table that breaks MaxAllowedWidth limit
	hugeCols := strings.Repeat("| Col ", 2000) + "|\n"
	hugeRows := strings.Repeat("| --- ", 2000) + "|\n"
	hugeCells := strings.Repeat("| A ", 2000) + "|\n"
	hugeMd := []byte(hugeCols + hugeRows + hugeCells)
	_, err = RenderWithDiagnostics(hugeMd, opts)
	if err == nil || !strings.Contains(err.Error(), "dimension exceeds resource limit") {
		t.Fatalf("Expected resource limit error, got: %v", err)
	}

	// Test TableFitWidth: false (disabled) produces table_layout_impossible
	opts.TableFitWidth = false
	opts.DiagnosticPolicy = &DiagnosticPolicy{FailOnUnsupported: true}
	narrowMd := []byte(
		"| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n" +
			"| -- | -- | -- | -- | -- | -- | -- | -- | -- | --- |\n" +
			"| 1  | 2  | 3  | 4  | 5  | 6  | 7  | 8  | 9  | 10  |\n",
	)
	_, err = RenderWithDiagnostics(narrowMd, opts)
	if err == nil || !strings.Contains(err.Error(), "table minimum width exceeds available canvas width") {
		t.Fatalf("Expected table layout impossible error when TableFitWidth is false, got: %v", err)
	}
}

func TestTableFitWidthFailedImageMemo(t *testing.T) {
	policy := DefaultCLIImagePolicy()
	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			t.Fatalf("Failed image requested more than once!")
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	md := []byte(fmt.Sprintf(`
| C1 | C2 |
| -- | -- |
| ![failed-img](%s) | Row 1 Col 2 |
`, ts.URL))

	opts := RenderOptions{
		Width:         200,
		TableFitWidth: true,
		ImagePolicy:   &policy,
	}

	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	foundDiag := false
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagnosticCode("image_load_failed") {
			foundDiag = true
		}
	}
	if !foundDiag {
		t.Fatalf("Expected image_load_failed diagnostic for missing image")
	}
	if requestCount != 1 {
		t.Fatalf("Expected exactly 1 request for failed image, got %d", requestCount)
	}

	// Test strict mode
	requestCount = 0
	opts.DiagnosticPolicy = &DiagnosticPolicy{FailOnImageError: true}
	strictRes, err := RenderWithDiagnostics(md, opts)
	if err == nil {
		t.Fatalf("Expected strict error for failed image")
	}
	if requestCount != 1 {
		t.Fatalf("Expected exactly 1 request for failed image in strict mode, got %d", requestCount)
	}
	foundDiag = false
	for _, diag := range strictRes.Diagnostics {
		if diag.Code == DiagnosticCode("image_load_failed") {
			foundDiag = true
		}
	}
	if !foundDiag {
		t.Fatalf("Expected image_load_failed diagnostic for missing image in strict mode")
	}
}

func TestTableFitWidthSemantics(t *testing.T) {
	policy := DefaultCLIImagePolicy()
	baseOpts := RenderOptions{
		Width:         200,
		Margin:        10,
		TableFitWidth: true,
		ImagePolicy:   &policy,
	}

	checkErr := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("Unexpected render error: %v", err)
		}
	}

	// 1. no table => unchanged width
	noTableMd := []byte("Just some regular text without a table.")
	res, err := RenderWithDiagnostics(noTableMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 200 {
		t.Fatalf("Expected unchanged width 200, got %d", res.Image.Bounds().Dx())
	}

	// 2. fitting table => unchanged width
	fittingTableMd := []byte("| A | B |\n|---|---|\n| 1 | 2 |")
	res, err = RenderWithDiagnostics(fittingTableMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 200 {
		t.Fatalf("Expected unchanged width 200, got %d", res.Image.Bounds().Dx())
	}

	// 3. oversized table exact minimum width
	// For default 16pt (size=16), cellPadding = max(8, int(16*0.6)) = 9
	// Columns: 10
	// 10 columns each with minW=20 + 2*9=18 => 38 per col.
	// 10 * 38 = 380.
	// Borders = 1px. 11 borders => 11.
	// Total minimum table width = 380 + 11 = 391.
	// Margin = 10. Required canvas = 391 + 20 = 411.
	oversizedTableMd := []byte("| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n|---|---|---|---|---|---|---|---|---|---|\n| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |")
	res, err = RenderWithDiagnostics(oversizedTableMd, baseOpts)
	checkErr(err)
	width1 := res.Image.Bounds().Dx()
	if width1 != 411 {
		t.Fatalf("Expected exact widened width 411, got %d", width1)
	}

	// 4. widest of multiple tables and order independence
	multipleTablesMd := []byte("| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n|---|---|---|---|---|---|---|---|---|---|\n| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |\n\n| A | B |\n|---|---|\n| 1 | 2 |")
	res, err = RenderWithDiagnostics(multipleTablesMd, baseOpts)
	checkErr(err)
	width2 := res.Image.Bounds().Dx()
	if width1 != width2 {
		t.Fatalf("Expected exact widened width %d for multiple tables, got %d", width1, width2)
	}

	multipleTablesReverseMd := []byte("| A | B |\n|---|---|\n| 1 | 2 |\n\n| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n|---|---|---|---|---|---|---|---|---|---|\n| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |")
	res, err = RenderWithDiagnostics(multipleTablesReverseMd, baseOpts)
	checkErr(err)
	width3 := res.Image.Bounds().Dx()
	if width1 != width3 {
		t.Fatalf("Expected order independence for multiple tables width %d, got %d", width1, width3)
	}

	// 5. breakable long token does not force natural/unwrapped width
	breakableMd := []byte("| " + strings.Repeat("a", 1000) + " |\n| --- |\n| a |")
	res, err = RenderWithDiagnostics(breakableMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 200 {
		t.Fatalf("Expected breakable text to maintain exact width 200, got %d", res.Image.Bounds().Dx())
	}

	// 6. margins/right border remain in bounds
	// Using the oversized table of 411 px width, the right margin should be perfectly preserved.
	// The table is 391 px. Left margin = 10. Table Right = 401. Output Width = 411.
	// We can check pixel at x=400 (last border pixel is x=400), x=401 should be white.
	oversizedTableRes, err := RenderWithDiagnostics(oversizedTableMd, baseOpts)
	checkErr(err)
	foundBorder := false
	for y := oversizedTableRes.Image.Bounds().Min.Y; y < oversizedTableRes.Image.Bounds().Max.Y; y++ {
		rB, gB, bB, _ := oversizedTableRes.Image.At(400, y).RGBA()
		if rB < 0xffff || gB < 0xffff || bB < 0xffff {
			foundBorder = true
		}
		r, g, b, _ := oversizedTableRes.Image.At(401, y).RGBA()
		if r < 0xffff || g < 0xffff || b < 0xffff {
			t.Fatalf("Expected right margin to be clean, but found non-white pixel at %d, %d", 401, y)
		}
	}
	if !foundBorder {
		t.Fatalf("Expected to find a table border at x=400, but found none")
	}

	// 7. styled/multiline cells
	styledMd := []byte("| **Bold** | *Italic* |\n|---|---|\n| Line 1<br>Line 2 | `Code` |\n")
	res, err = RenderWithDiagnostics(styledMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 200 {
		t.Fatalf("Styled table should exact-fit in width 200, got %d", res.Image.Bounds().Dx())
	}

	// 8. wide image does not force natural-width expansion or upscaling
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 800, 100))
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, img); err != nil {
			t.Errorf("Failed to encode png: %v", err)
		}
	}))
	defer ts.Close()
	wideImageMd := []byte(fmt.Sprintf("| Image |\n| --- |\n| ![wide](%s) |", ts.URL))
	res, err = RenderWithDiagnostics(wideImageMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 200 {
		t.Fatalf("Wide image should not force natural width expansion, got %d", res.Image.Bounds().Dx())
	}

	// Ensure that image upscaling does not occur merely due to widened columns
	tsSmall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for x := 0; x < 10; x++ {
			for y := 0; y < 10; y++ {
				// Make it distinctive blue
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
		if err := png.Encode(w, img); err != nil {
			t.Errorf("Failed to encode png: %v", err)
		}
	}))
	defer tsSmall.Close()
	upscaleMd := []byte(fmt.Sprintf("| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n|---|---|---|---|---|---|---|---|---|---|\n| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | ![small](%s) |", tsSmall.URL))
	res, err = RenderWithDiagnostics(upscaleMd, baseOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != 411 {
		t.Fatalf("Upscale scenario should have exactly calculated table width 411, got %d", res.Image.Bounds().Dx())
	}

	// Now prove the image stayed 10x10. Search for the distinctive blue pixels.
	blueCount := 0
	for y := res.Image.Bounds().Min.Y; y < res.Image.Bounds().Max.Y; y++ {
		for x := res.Image.Bounds().Min.X; x < res.Image.Bounds().Max.X; x++ {
			r, g, b, a := res.Image.At(x, y).RGBA()
			// Need exact match for pure blue
			if r == 0 && g == 0 && b == 0xffff && a == 0xffff {
				blueCount++
			}
		}
	}
	if blueCount != 100 {
		t.Fatalf("Expected exactly 100 distinctive blue pixels for the 10x10 small image, got %d (upscaled?)", blueCount)
	}

	// 9. exact MaxAllowedWidth acceptance
	// Current formula: Width = cols*38 + cols + 1 + 2*margin
	// So cols*39 + 1 + 2*margin = 32768
	// 39*839 = 32721. + 1 = 32722. 32768 - 32722 = 46. margin = 23.
	exactBoundsOpts := baseOpts
	exactBoundsOpts.Margin = 23
	colsStr := strings.Repeat("| C ", 839) + "|\n"
	headerStr := strings.Repeat("| --- ", 839) + "|\n"
	rowStr := strings.Repeat("| 1 ", 839) + "|\n"
	exactMd := []byte(colsStr + headerStr + rowStr)
	res, err = RenderWithDiagnostics(exactMd, exactBoundsOpts)
	checkErr(err)
	if res.Image.Bounds().Dx() != MaxAllowedWidth {
		t.Fatalf("Expected exact MaxAllowedWidth limit hit (32768), got %d", res.Image.Bounds().Dx())
	}

	// 10. over-MaxAllowedWidth deterministic failure
	overBoundsOpts := exactBoundsOpts
	overBoundsOpts.Margin = 24
	_, err = RenderWithDiagnostics(exactMd, overBoundsOpts)
	if err == nil || !strings.Contains(err.Error(), "resource limit") {
		t.Fatalf("Expected dimension exceeds resource limit, got %v", err)
	}

	// 11. MaxTotalPixels interaction: fit-width expands width and trips the max total pixels limit
	tallMd := []byte("| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |\n|---|---|---|---|---|---|---|---|---|---|\n" + strings.Repeat("| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |\n", 500))
	tallOpts := baseOpts
	tallOpts.Width = 200
	tallOpts.MaxHeight = 100000
	oldMaxTotalPixels := maxTotalPixels
	maxTotalPixels = 2000000
	defer func() { maxTotalPixels = oldMaxTotalPixels }()
	_, err = RenderWithDiagnostics(tallMd, tallOpts)
	if err == nil || (!strings.Contains(err.Error(), "resource limit") && !strings.Contains(err.Error(), "pixel budget")) {
		t.Fatalf("Expected dimension exceeds resource limit due to total pixel budget, got %v", err)
	}

	// 12. disabled mode preserves impossible-table behaviour
	disabledOpts := baseOpts
	disabledOpts.TableFitWidth = false
	disabledOpts.DiagnosticPolicy = &DiagnosticPolicy{FailOnUnsupported: true}
	_, err = RenderWithDiagnostics(oversizedTableMd, disabledOpts)
	if err == nil {
		t.Fatalf("Expected table layout impossible error when disabled")
	}
}
