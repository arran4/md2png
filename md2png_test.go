package md2png

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type mockTransport func(*http.Request) (*http.Response, error)

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestSyntaxHighlightingTokensExtended(t *testing.T) {
	r := &renderer{}
	th := lightTheme
	thDark := darkTheme

	// 1. Multiline no-language fenced fallback
	// 2. Multiline unknown-language fallback
	// 3. Multiline recognized fenced code with DisableHighlighting: true
	// 4. Multiline indented code remaining plain
	// 5. Empty interior line preservation: "line one\n\nline three" remains three logical lines
	testCases := []struct {
		name     string
		text     string
		lang     string
		disabled bool
		lines    int
	}{
		{"no language", "line one\n\nline three", "", false, 3},
		{"unknown lang", "line one\n\nline three", "unknownlanguage123", false, 3},
		{"disabled highlighting", "line one\n\nline three", "go", true, 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spans := r.tokenizeCodeBlock(tc.text, tc.lang, th, tc.disabled)
			if len(spans) != tc.lines {
				t.Fatalf("expected %d lines, got %d", tc.lines, len(spans))
			}
			if len(spans[1]) != 0 {
				t.Fatalf("expected empty interior line to be length 0, got %d", len(spans[1]))
			}
			if len(spans[0]) == 0 || spans[0][0].Color != th.FG {
				t.Fatalf("expected base FG color for fallback")
			}
		})
	}

	// 6. Light-theme highlighting (Go)
	spansLight := r.tokenizeCodeBlock("func main() {\n\n}", "go", th, false)
	hasKeywordLight := false
	for _, s := range spansLight[0] {
		if s.Text == "func" && s.Color != th.FG {
			hasKeywordLight = true
		}
	}
	if !hasKeywordLight {
		t.Fatalf("did not find highlighted 'func' in light theme")
	}

	// 7. Dark-theme highlighting (Go)
	spansDark := r.tokenizeCodeBlock("func main() {\n\n}", "go", thDark, false)
	hasKeywordDark := false
	for _, s := range spansDark[0] {
		if s.Text == "func" && s.Color != thDark.FG {
			hasKeywordDark = true
		}
	}
	if !hasKeywordDark {
		t.Fatalf("did not find highlighted 'func' in dark theme")
	}

	// 8. Second recognized language (e.g. JSON)
	spansJSON := r.tokenizeCodeBlock("{\n  \"key\": \"value\"\n}", "json", th, false)
	if len(spansJSON) != 3 {
		t.Fatalf("expected 3 lines for JSON, got %d", len(spansJSON))
	}
	hasKeyJSON := false
	for _, s := range spansJSON[1] {
		if s.Text == "\"key\"" && s.Color != th.FG {
			hasKeyJSON = true
		}
	}
	if !hasKeyJSON {
		t.Fatalf("did not find highlighted 'key' in JSON block")
	}

	// 9. Zero/empty SyntaxPalette falling back safely to Theme.FG
	thEmpty := Theme{FG: color.White}
	spansEmpty := r.tokenizeCodeBlock("func main()", "go", thEmpty, false)
	for _, s := range spansEmpty[0] {
		if s.Text == "func" && s.Color != thEmpty.FG {
			t.Fatalf("expected fallback to default FG color natively if palette missing securely")
		}
	}

	// 11. Nested list highlighting asserting actual HTML/markdown token colours vs standard text blocks
	nestedListMD := "- list item\n  ```go\n  func main() {}\n  ```"
	nestedResWith, err := RenderWithDiagnostics([]byte(nestedListMD), RenderOptions{DisableHighlighting: false})
	if err != nil {
		t.Fatalf("Render nested failed: %v", err)
	}
	nestedResWithout, err := RenderWithDiagnostics([]byte(nestedListMD), RenderOptions{DisableHighlighting: true})
	if err != nil {
		t.Fatalf("Render nested failed: %v", err)
	}
	if imagesEqual(nestedResWith.Image, nestedResWithout.Image) {
		t.Fatalf("Nested list highlighting failed to change output image")
	}

	// 12. RenderOptions zero-value default allows highlighting
	mdZero := "```go\nfunc main(){}\n```"
	zeroResWith, err := RenderWithDiagnostics([]byte(mdZero), RenderOptions{})
	if err != nil {
		t.Fatalf("Render zero-value failed: %v", err)
	}
	zeroResWithout, err := RenderWithDiagnostics([]byte(mdZero), RenderOptions{DisableHighlighting: true})
	if err != nil {
		t.Fatalf("Render zero-value without failed: %v", err)
	}
	if imagesEqual(zeroResWith.Image, zeroResWithout.Image) {
		t.Fatalf("Zero-value options failed to apply highlighting")
	}

	// 13. Indented code must be proven plain (disabled highlighting shouldn't change the outcome)
	indentedMD := "    func main() {\n        fmt.Println(\"plain\")\n    }"
	indentedResWith, err := RenderWithDiagnostics([]byte(indentedMD), RenderOptions{})
	if err != nil {
		t.Fatalf("Render indented with failed: %v", err)
	}
	indentedResWithout, err := RenderWithDiagnostics([]byte(indentedMD), RenderOptions{DisableHighlighting: true})
	if err != nil {
		t.Fatalf("Render indented without failed: %v", err)
	}
	if !imagesEqual(indentedResWith.Image, indentedResWithout.Image) {
		t.Fatalf("Indented code images should be identical regardless of highlighting config")
	}

	// 14. Prove interior blank line survives wrapCodeSpans
	spansBlank := r.tokenizeCodeBlock("line one\n\nline three", "", th, false)
	fontsForBlank, err := LoadFonts(FontConfig{SizeBase: 14})
	if err != nil {
		t.Fatalf("load fonts for blank line test: %v", err)
	}
	wrappedBlank := wrapCodeSpans(fontsForBlank.Mono, 14, spansBlank, 800)
	if len(wrappedBlank) != 3 {
		t.Fatalf("expected 3 logical lines for blank line test, got %d", len(wrappedBlank))
	}
	if len(wrappedBlank[1]) != 0 {
		t.Fatalf("expected the second wrapped line to remain empty")
	}
	// 10. Real tokenized highlighted long line wrapping while retaining multiple syntax colours
	fonts, err := LoadFonts(FontConfig{SizeBase: 14})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	longCode := "func VeryLongFunctionNameThatForcesWrappingOverMultipleLinesBecauseItsVeryLongAndHasTokens() {}"
	spansLong := r.tokenizeCodeBlock(longCode, "go", th, false)
	wrappedLong := wrapCodeSpans(fonts.Mono, 14, spansLong, 80)

	if len(wrappedLong) < 2 {
		t.Fatalf("expected highlighted long line to wrap, got %d lines", len(wrappedLong))
	}
	colorsSeen := make(map[color.Color]bool)
	for _, l := range wrappedLong {
		for _, s := range l {
			colorsSeen[s.Color] = true
		}
	}
	if len(colorsSeen) < 2 {
		t.Fatalf("expected multiple syntax colors to survive real tokenized wrapping")
	}
}

func TestSyntaxHighlighting(t *testing.T) {
	md := "~~~go\nfunc main() {\n  fmt.Println(\"Hello\")\n}\n~~~\n\n~~~\nplain block\n~~~\n\n    indented block"

	opts := RenderOptions{Width: 800, DisableHighlighting: false, Margin: 10}

	// Preflight check: just ensuring it renders and lexes
	res, err := RenderWithDiagnostics([]byte(md), opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if res.Image == nil {
		t.Fatalf("expected image")
	}

	optsDisabled := RenderOptions{Width: 800, DisableHighlighting: true, Margin: 10}
	resDisabled, err := RenderWithDiagnostics([]byte(md), optsDisabled)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if resDisabled.Image == nil {
		t.Fatalf("expected image")
	}
}

func TestSyntaxHighlightingTokens(t *testing.T) {
	r := &renderer{}
	th := lightTheme

	spans := r.tokenizeCodeBlock("func main()", "go", th, false)
	if len(spans) == 0 || len(spans[0]) < 2 {
		t.Fatalf("expected highlighted tokens, got %v", spans)
	}
	hasKeyword := false
	for _, s := range spans[0] {
		if s.Text == "func" {
			hasKeyword = true
			if s.Color == th.FG {
				t.Fatalf("expected keyword to be highlighted, got base FG color")
			}
		}
	}
	if !hasKeyword {
		t.Fatalf("did not find expected 'func' token")
	}

	// test disabled
	spansDisabled := r.tokenizeCodeBlock("func main()", "go", th, true)
	if len(spansDisabled) != 1 || len(spansDisabled[0]) != 1 {
		t.Fatalf("expected 1 span when disabled")
	}
	if spansDisabled[0][0].Color != th.FG {
		t.Fatalf("expected base FG color when disabled")
	}

	// test fallback
	spansFallback := r.tokenizeCodeBlock("func main()", "unknownlanguage123", th, false)
	if len(spansFallback) != 1 || len(spansFallback[0]) != 1 {
		t.Fatalf("expected 1 span for unknown language")
	}
	if spansFallback[0][0].Color != th.FG {
		t.Fatalf("expected base FG color for unknown language")
	}
}

func TestNormalizeCodeSpans(t *testing.T) {
	tests := []struct {
		name     string
		input    [][]codeSpan
		tabWidth int
		expected [][]codeSpan
	}{
		{
			name: "no tabs",
			input: [][]codeSpan{
				{{Text: "hello world", Color: color.Black}},
			},
			tabWidth: 4,
			expected: [][]codeSpan{
				{{Text: "hello world", Color: color.Black}},
			},
		},
		{
			name: "single leading tab",
			input: [][]codeSpan{
				{{Text: "\thello", Color: color.Black}},
			},
			tabWidth: 4,
			expected: [][]codeSpan{
				{{Text: "    hello", Color: color.Black}},
			},
		},
		{
			name: "multiple tabs",
			input: [][]codeSpan{
				{{Text: "a\tb\tc", Color: color.Black}},
			},
			tabWidth: 4,
			expected: [][]codeSpan{
				{{Text: "a   b   c", Color: color.Black}},
			},
		},
		{
			name: "custom tab width 8",
			input: [][]codeSpan{
				{{Text: "\thello", Color: color.Black}},
			},
			tabWidth: 8,
			expected: [][]codeSpan{
				{{Text: "        hello", Color: color.Black}},
			},
		},
		{
			name: "mixed colors",
			input: [][]codeSpan{
				{{Text: "func", Color: color.Black}, {Text: " main() {\t", Color: color.White}},
			},
			tabWidth: 4,
			expected: [][]codeSpan{
				{{Text: "func", Color: color.Black}, {Text: " main() {   ", Color: color.White}},
			},
		},
		{
			name: "control characters removed/normalized",
			input: [][]codeSpan{
				{{Text: "hello\r\vworld\u2028!", Color: color.Black}},
			},
			tabWidth: 4,
			expected: [][]codeSpan{
				{{Text: "helloworld!", Color: color.Black}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := normalizeCodeSpans(tt.input, tt.tabWidth)
			if len(res) != len(tt.expected) {
				t.Fatalf("expected %d lines, got %d", len(tt.expected), len(res))
			}
			for i, line := range res {
				if len(line) != len(tt.expected[i]) {
					t.Fatalf("line %d: expected %d spans, got %d", i, len(tt.expected[i]), len(line))
				}
				for j, span := range line {
					if span.Text != tt.expected[i][j].Text || span.Color != tt.expected[i][j].Color {
						t.Errorf("line %d span %d: expected {%q, %v}, got {%q, %v}", i, j, tt.expected[i][j].Text, tt.expected[i][j].Color, span.Text, span.Color)
					}
				}
			}
		})
	}
}

func TestWrapCodeSpans(t *testing.T) {
	fonts, err := LoadFonts(FontConfig{SizeBase: 14})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}

	text := "    spaced  out"
	lines := wrapCodeSpans(fonts.Mono, 14, [][]codeSpan{{{Text: text, Color: color.White}}}, 140)
	if len(lines) == 0 || len(lines[0]) == 0 {
		t.Fatalf("expected at least one line")
	}
	if !strings.HasPrefix(lines[0][0].Text, "    ") {
		t.Fatalf("expected leading spaces to be preserved, got %q", lines[0][0].Text)
	}

	joined := ""
	for _, l := range lines {
		for _, tok := range l {
			joined += tok.Text
		}
	}
	if !strings.Contains(joined, "  out") {
		t.Fatalf("expected double spaces inside wrapped lines to be preserved, got %q", joined)
	}

	spans := []codeSpan{
		{Text: "avery", Color: color.White},
		{Text: "verylongtokenwithout", Color: color.Black},
		{Text: "spaces", Color: color.White},
	}

	longLines := wrapCodeSpans(fonts.Mono, 14, [][]codeSpan{spans}, 80)
	if len(longLines) < 2 {
		t.Fatalf("expected long token to wrap across multiple lines, got %d lines", len(longLines))
	}

	colorsSeen := make(map[color.Color]bool)
	for _, l := range longLines {
		for _, s := range l {
			colorsSeen[s.Color] = true
		}
	}

	if len(colorsSeen) < 2 {
		t.Fatalf("expected multiple colors to survive wrapping")
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
	c := newCanvas(640, 48, lightTheme, fonts, 16, 32768)
	mockClient := &http.Client{
		Transport: mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	policy := DefaultCLIImagePolicy()
	policy.HTTPClient = mockClient
	r := &renderer{
		c:              c,
		baseSize:       16,
		linkFootnotes:  true,
		imageFootnotes: true,
		imagePolicy:    policy,
	}
	r.ensureImageResolvers()
	markdown := []byte("First [link](https://example.com) and second [same](https://example.com) ![img](https://example.com/image.png)")
	mdParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	doc := mdParser.Parser().Parse(text.NewReader(markdown))
	if err := r.renderDocument(markdown, doc); err != nil {
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
	c := newCanvas(640, 48, lightTheme, fonts, 16, 32768)
	mockClient := &http.Client{
		Transport: mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	policy := DefaultCLIImagePolicy()
	policy.HTTPClient = mockClient
	r := &renderer{
		c:              c,
		baseSize:       16,
		linkFootnotes:  false,
		imageFootnotes: true,
		imagePolicy:    policy,
	}
	r.ensureImageResolvers()
	markdown := []byte("[link](https://example.com) ![img](https://example.com/image.png)")
	mdParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	doc := mdParser.Parser().Parse(text.NewReader(markdown))
	if err := r.renderDocument(markdown, doc); err != nil {
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
		_ = file.Close()
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestMaxHeightLimit(t *testing.T) {
	markdown := "Test height limit\n\n"
	for i := 0; i < 1000; i++ {
		markdown += "Line " + fmt.Sprint(i) + "\n\n"
	}

	_, err := Render([]byte(markdown), RenderOptions{MaxHeight: 2000})
	if err == nil {
		t.Fatalf("expected error due to height limit exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "maximum output height limit exceeded") {
		t.Fatalf("expected height limit error message, got: %v", err)
	}
}

func TestNegativeMaxHeight(t *testing.T) {
	_, err := Render([]byte("# Hello"), RenderOptions{MaxHeight: -1})
	if err == nil {
		t.Fatalf("expected error for negative MaxHeight, got nil")
	}
	if !strings.Contains(err.Error(), "invalid max height") {
		t.Fatalf("expected specific negative MaxHeight error message, got: %v", err)
	}
}

func TestZeroMaxHeightDefaults(t *testing.T) {
	// 0 defaults to 32768, so parsing 2000 lines (which takes ~70000 pixels) should fail
	markdown := "Test height limit\n\n"
	for i := 0; i < 2000; i++ {
		markdown += "Line " + fmt.Sprint(i) + "\n\n"
	}

	_, err := Render([]byte(markdown), RenderOptions{MaxHeight: 0})
	if err == nil {
		t.Fatalf("expected error due to height limit exceeded on default 0 (32768), got nil")
	}
	if !strings.Contains(err.Error(), "maximum output height limit exceeded") {
		t.Fatalf("expected height limit error message, got: %v", err)
	}
}

func TestRenderBeyond8192pxElements(t *testing.T) {
	// We want to force the cursor specifically near the 8192px boundary, then
	// have the target block start just *above* 8192 and finish *below* 8192.
	// Since font rendering logic can be complex to guess exact pixel heights,
	// we use exact canvas measurement inside the test loops.

	getCursorHeight := func(md []byte) int {
		// Do a quick dry-run render to see how tall it gets.
		img, _ := Render(md, RenderOptions{MaxHeight: 60000, Width: 400})
		if img == nil {
			return 0
		}
		return img.Bounds().Dy()
	}

	buildSpacersTo := func(target int) string {
		markdown := "# Boundary\n\n"
		for {
			nextMarkdown := markdown + "Spacer to push content down\n\n"
			height := getCursorHeight([]byte(nextMarkdown))
			if height > target {
				break
			}
			markdown = nextMarkdown
		}
		return markdown
	}

	// 1. Image
	t.Run("Image", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Make a very tall image so it easily spans across the boundary.
		block := image.NewRGBA(image.Rect(0, 0, 40, 600))
		want := color.RGBA{R: 0xCC, G: 0x22, B: 0x22, A: 0xFF}
		draw.Draw(block, block.Bounds(), image.NewUniform(want), image.Point{}, draw.Src)
		imgPath := filepath.Join(tmpDir, "block.png")
		file, err := os.Create(imgPath)
		if err != nil {
			t.Fatalf("create temp image: %v", err)
		}
		if err := png.Encode(file, block); err != nil {
			_ = file.Close()
			t.Fatalf("encode temp image: %v", err)
		}
		_ = file.Close()

		// Get exactly under 8192
		markdown := buildSpacersTo(8150)
		markdown += fmt.Sprintf("![local image](%s)\n\n", filepath.Base(imgPath))

		img, err := Render([]byte(markdown), RenderOptions{BaseDir: tmpDir, MaxHeight: 60000, Width: 400})
		if err != nil {
			t.Fatalf("render failed: %v", err)
		}
		if img.Bounds().Dy() <= 8192 {
			t.Fatalf("expected image height > 8192px, got %d", img.Bounds().Dy())
		}

		// Ensure we see image pixels *above* 8192 (i.e. the image started before the growth bound)
		foundAbove := false
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < 8192 && !foundAbove; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(b>>8) == want.B && uint8(a>>8) == want.A {
					foundAbove = true
					break
				}
			}
		}
		if !foundAbove {
			t.Fatalf("expected embedded image pixels ABOVE 8192px (image should span across boundary)")
		}

		// Ensure we see image pixels *below* 8192 (i.e. the image successfully forced a growth)
		foundBelow := false
		for y := 8192; y < bounds.Max.Y && !foundBelow; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(b>>8) == want.B && uint8(a>>8) == want.A {
					foundBelow = true
					break
				}
			}
		}
		if !foundBelow {
			t.Fatalf("expected embedded image pixels BELOW 8192px")
		}
	})

	// 2. Code Block
	t.Run("CodeBlock", func(t *testing.T) {
		markdown := buildSpacersTo(8150)
		markdown += "```\n"
		for i := 0; i < 50; i++ {
			markdown += "func bottomCodeBlockLine() {}\n"
		}
		markdown += "```\n"

		img, err := Render([]byte(markdown), RenderOptions{MaxHeight: 60000, Width: 400})
		if err != nil {
			t.Fatalf("render failed: %v", err)
		}
		if img.Bounds().Dy() <= 8192 {
			t.Fatalf("expected image height > 8192px, got %d", img.Bounds().Dy())
		}

		bounds := img.Bounds()
		cR, cG, cB, cA := lightTheme.CodeBG.RGBA()

		// Above boundary
		foundAbove := false
		for y := bounds.Min.Y; y < 8192 && !foundAbove; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if r == cR && g == cG && b == cB && a == cA {
					foundAbove = true
					break
				}
			}
		}
		if !foundAbove {
			t.Fatalf("expected code block background pixels ABOVE 8192px (codeblock should span across boundary)")
		}

		// Below boundary
		foundBelow := false
		for y := 8192; y < bounds.Max.Y && !foundBelow; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if r == cR && g == cG && b == cB && a == cA {
					foundBelow = true
					break
				}
			}
		}
		if !foundBelow {
			t.Fatalf("expected code block background pixels BELOW 8192px")
		}
	})

	// 3. Table
	t.Run("Table", func(t *testing.T) {
		markdown := buildSpacersTo(8150)
		markdown += "| Col 1 | Col 2 |\n|---|---|\n"
		for i := 0; i < 50; i++ {
			markdown += fmt.Sprintf("| Val %d | [link](https://example.com) |\n", i)
		}

		img, err := Render([]byte(markdown), RenderOptions{MaxHeight: 60000, Width: 400})
		if err != nil {
			t.Fatalf("render failed: %v", err)
		}
		if img.Bounds().Dy() <= 8192 {
			t.Fatalf("expected image height > 8192px, got %d", img.Bounds().Dy())
		}

		bounds := img.Bounds()

		// Above boundary
		foundAbove := false
		for y := bounds.Min.Y; y < 8192 && !foundAbove; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if uint8(r>>8) == linkColor.R && uint8(g>>8) == linkColor.G && uint8(b>>8) == linkColor.B && uint8(a>>8) == linkColor.A {
					foundAbove = true
					break
				}
			}
		}
		if !foundAbove {
			t.Fatalf("expected table cell content (link pixels) ABOVE 8192px (table should span across boundary)")
		}

		// Below boundary
		foundBelow := false
		for y := 8192; y < bounds.Max.Y && !foundBelow; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if uint8(r>>8) == linkColor.R && uint8(g>>8) == linkColor.G && uint8(b>>8) == linkColor.B && uint8(a>>8) == linkColor.A {
					foundBelow = true
					break
				}
			}
		}
		if !foundBelow {
			t.Fatalf("expected table cell content (link pixels) BELOW 8192px")
		}
	})
}

func TestRenderBeyond8192px(t *testing.T) {
	markdown := "# Tall Document\n\n"
	for i := 0; i < 1000; i++ {
		markdown += "Paragraph line " + fmt.Sprint(i) + "\n\n"
	}

	// Add an explicit colored marker at the bottom that we can test for.
	markdown += "## The End\n\nThis is the bottom text with a [link](https://example.com/bottom) at the end."

	img, err := Render([]byte(markdown), RenderOptions{MaxHeight: 60000})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected image result")
	}
	if img.Bounds().Dy() < 10000 {
		t.Fatalf("expected image height to be well over 8192px, got %d", img.Bounds().Dy())
	}

	// Ensure that meaningful pixels exist near the bottom of the image
	// by searching for the link color which we added at the end of the markdown.
	bounds := img.Bounds()
	foundLinkPixel := false
	// Start searching from the bottom upwards (last 200 pixels should cover it)
	startY := bounds.Max.Y - 200
	if startY < bounds.Min.Y {
		startY = bounds.Min.Y
	}
	for y := startY; y < bounds.Max.Y && !foundLinkPixel; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if uint8(r>>8) == linkColor.R && uint8(g>>8) == linkColor.G && uint8(b>>8) == linkColor.B && uint8(a>>8) == linkColor.A {
				foundLinkPixel = true
				break
			}
		}
	}
	if !foundLinkPixel {
		t.Fatalf("expected meaningful rendered pixels (link) near the bottom of a >8192px image")
	}
}

func TestRendererFallbackBehaviour(t *testing.T) {
	markdown := "Paragraph with a ![missing image](file:///non-existent-file.png)."

	// This uses the DefaultCLIImagePolicy which allows local images
	opts := RenderOptions{ImagePolicy: &ImagePolicy{AllowLocal: true, SandboxLocal: false}}

	// The image file does not exist. Since it's just a regular file-not-found error,
	// it should NOT cause a fatal error. It should use the fallback mechanism.
	img, err := Render([]byte(markdown), opts)
	if err != nil {
		t.Fatalf("expected success with fallback, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected an image returned")
	}
}

func imagesEqual(a, b image.Image) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	b1 := a.Bounds()
	b2 := b.Bounds()
	if b1 != b2 {
		return false
	}
	for y := b1.Min.Y; y < b1.Max.Y; y++ {
		for x := b1.Min.X; x < b1.Max.X; x++ {
			c1 := a.At(x, y)
			c2 := b.At(x, y)
			r1, g1, b1c, a1 := c1.RGBA()
			r2, g2, b2c, a2 := c2.RGBA()
			if r1 != r2 || g1 != g2 || b1c != b2c || a1 != a2 {
				return false
			}
		}
	}
	return true
}

func TestRendererTabExpansionPaths(t *testing.T) {
	// Instead of testing raw pixel output, we instantiate a renderer and use its tokenization directly
	// to ensure all logical paths go through the normalization logic with the proper configurations.
	th, _ := ThemeByName("light")
	r := &renderer{
		opts: RenderOptions{CodeTabWidth: 4, DisableHighlighting: false},
		c:    &canvas{th: th},
	}

	tests := []struct {
		name       string
		text       string
		lang       string
		disable    bool
		wantColors bool // Should have multiple colors if highlighted
		wantTexts  [][]string
	}{
		{
			name: "highlighted go code",
			text: "func main() {\n\tprintln()\n}",
			lang: "go",
			wantColors: true,
			wantTexts: [][]string{
				{"func", " ", "main", "()", " ", "{"},
				{"    ", "println", "()"},
				{"}"},
			},
		},
		{
			name: "no language plain block",
			text: "foo\n\tbar",
			lang: "",
			wantColors: false,
			wantTexts: [][]string{
				{"foo"},
				{"    bar"},
			},
		},
		{
			name: "unknown language",
			text: "foo\n\tbar",
			lang: "foobar",
			wantColors: false,
			wantTexts: [][]string{
				{"foo"},
				{"    bar"},
			},
		},
		{
			name: "DisableHighlighting=true",
			text: "func main() {\n\tprintln()\n}",
			lang: "go",
			disable: true,
			wantColors: false,
			wantTexts: [][]string{
				{"func main() {"},
				{"    println()"},
				{"}"},
			},
		},
		{
			name: "CRLF and mixed",
			text: "foo \tbar\r\n\t\tbaz",
			lang: "",
			wantColors: false,
			wantTexts: [][]string{
				{"foo     bar"},
				{"        baz"},
			},
		},
		{
			name: "blank lines and repeated spaces",
			text: "\n  foo  \n\n",
			lang: "",
			wantColors: false,
			wantTexts: [][]string{
				{},
				{"  foo  "},
				{},
				{},
			},
		},
		{
			name: "punctuation and unicode",
			text: "{} [] () <> \" ' \\ / | & # % @ ~ ^ _ - + = : ; , . ? ! $ *\nnaïve café µ Ω",
			lang: "",
			wantColors: false,
			wantTexts: [][]string{
				{"{} [] () <> \" ' \\ / | & # % @ ~ ^ _ - + = : ; , . ? ! $ *"},
				{"naïve café µ Ω"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.opts.DisableHighlighting = tt.disable
			spans := r.tokenizeCodeBlock(tt.text, tt.lang, th, tt.disable)

			if len(spans) != len(tt.wantTexts) {
				t.Fatalf("expected %d lines, got %d", len(tt.wantTexts), len(spans))
			}

			foundMultipleColors := false
			var firstColor color.Color
			for i, line := range spans {
				if len(line) != len(tt.wantTexts[i]) {
					t.Fatalf("line %d: expected %d spans, got %d", i, len(tt.wantTexts[i]), len(line))
				}
				for j, span := range line {
					if span.Text != tt.wantTexts[i][j] {
						t.Errorf("line %d span %d: expected text %q, got %q", i, j, tt.wantTexts[i][j], span.Text)
					}
					if i == 0 && j == 0 {
						firstColor = span.Color
					} else if firstColor != nil && span.Color != firstColor && span.Text != " " && span.Text != "" {
						foundMultipleColors = true
					}
				}
			}
			if tt.wantColors && !foundMultipleColors {
				t.Errorf("expected multiple colors for syntax highlighting, but all were the same")
			}
			if !tt.wantColors && foundMultipleColors {
				t.Errorf("expected no syntax highlighting (single color), but found multiple colors")
			}
		})
	}
}

func TestRendererTabExpansionIndentedAndNestedPaths(t *testing.T) {
	tests := []struct {
		name     string
		md       string
		wantText string
	}{
		{
			name: "indented code block",
			md:   "    func main() {\n    \tprintln()\n    }",
		},
		{
			name: "nested list fenced code",
			md:   "* Item\n  * Item 2\n    ```go\n    func test() {\n    \treturn\n    }\n    ```",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := RenderOptions{
				CodeTabWidth: 4,
				Width:        1024,
				Margin:       10,
				BaseFontSize: 16,
			}
			res, err := RenderWithDiagnostics([]byte(tt.md), opts)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.Image == nil {
				t.Fatalf("expected an image")
			}
			if len(res.Diagnostics) > 0 {
				t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
			}
		})
	}
}

func TestWrapCodeSpansWithTabExpansion(t *testing.T) {
	th, _ := ThemeByName("light")
	r := &renderer{
		opts: RenderOptions{CodeTabWidth: 4, DisableHighlighting: false},
		c:    &canvas{th: th},
	}

	text := "x\tBoundary"
	spans := r.tokenizeCodeBlock(text, "", th, true)

	fonts, err := LoadFonts(FontConfig{SizeBase: 14})
	if err != nil {
		t.Fatalf("could not load fonts: %v", err)
	}

	maxWidth := 50.0
	wrapped := wrapCodeSpans(fonts.Mono, 14, spans, maxWidth)

	if len(wrapped) <= 1 {
		t.Fatalf("expected text %q to wrap into multiple lines, got %d lines", text, len(wrapped))
	}

	// Expected wrapped lines based on the 50.0 width check.
	// Since "x   Boundary" will split at "x   Boun" then "dary"
	expectedSubstrings := [][]string{
		{"x", "   "},
		{"Boun"},
		{"dary"},
	}
	var foundSubstrings [][]string
	for _, line := range wrapped {
		var lineStrs []string
		for _, span := range line {
			lineStrs = append(lineStrs, span.Text)
		}
		foundSubstrings = append(foundSubstrings, lineStrs)
	}

	if len(foundSubstrings) != len(expectedSubstrings) {
		t.Fatalf("expected wrapped substrings %v, got %v", expectedSubstrings, foundSubstrings)
	}
	for i, line := range foundSubstrings {
		if len(line) != len(expectedSubstrings[i]) {
			t.Fatalf("line %d expected length %d, got %d", i, len(expectedSubstrings[i]), len(line))
		}
		for j, span := range line {
			if span != expectedSubstrings[i][j] {
				t.Errorf("line %d, span %d: expected %q, got %q", i, j, expectedSubstrings[i][j], span)
			}
		}
	}
}

func TestTabColumnResetOnNewline(t *testing.T) {
	lines := [][]codeSpan{
		{{Text: "a\tword", Color: color.Black}},
		{{Text: "ab\tword", Color: color.Black}},
	}
	res := normalizeCodeSpans(lines, 4)
	if res[0][0].Text != "a   word" {
		t.Errorf("line 1: expected 'a   word', got %q", res[0][0].Text)
	}
	if res[1][0].Text != "ab  word" {
		t.Errorf("line 2: expected 'ab  word', got %q", res[1][0].Text)
	}
}
