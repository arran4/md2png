package md2png

import (
	"encoding/xml"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestRenderSVG_ValidXMLAndStructure(t *testing.T) {
	md := `# Heading 1

This is a paragraph with **bold** and *italic* and [a link](https://example.com).

` + "```go" + `
package main
func main() {}
` + "```" + `

---

| A | B |
|---|---|
| 1 | 2 |

* List item A
* List item B

> Blockquote text
`

	svgBytes, err := RenderSVG([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	svgStr := string(svgBytes)

	// Must be valid XML
	var parsed struct {
		XMLName xml.Name `xml:"svg"`
		ViewBox string   `xml:"viewBox,attr"`
		Width   string   `xml:"width,attr"`
		Height  string   `xml:"height,attr"`
		Texts   []string `xml:"text"`
		Rects   []struct {
			Width  string `xml:"width,attr"`
			Height string `xml:"height,attr"`
		} `xml:"rect"`
	}
	if err := xml.Unmarshal(svgBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated SVG as valid XML: %v\nOutput:\n%s", err, svgStr)
	}

	if parsed.ViewBox == "" {
		t.Fatalf("expected non-empty viewBox attribute")
	}
	if len(parsed.Texts) == 0 {
		t.Fatalf("expected vector <text> elements in SVG output")
	}
	if len(parsed.Rects) == 0 {
		t.Fatalf("expected vector <rect> elements in SVG output")
	}

	// Must NOT contain an embedded full-canvas raster bitmap
	if strings.Contains(svgStr, `<image width="800"`) {
		t.Fatalf("SVG should not embed whole document as a raster image")
	}

	// Must contain text from the Markdown
	if !strings.Contains(svgStr, "Heading") {
		t.Fatalf("SVG output missing 'Heading'")
	}
	if !strings.Contains(svgStr, "Blockquote") {
		t.Fatalf("SVG output missing 'Blockquote'")
	}
}

func TestRenderSVG_DimensionsAndViewBox(t *testing.T) {
	md := `# Small doc`
	svgBytes, err := RenderSVG([]byte(md), RenderOptions{
		Width: 640,
	})
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	svgStr := string(svgBytes)
	if !strings.Contains(svgStr, `width="640"`) {
		t.Fatalf("expected width 640 in SVG: %s", svgStr)
	}
	if !strings.Contains(svgStr, `viewBox="0 0 640 `) {
		t.Fatalf("expected viewBox starting with '0 0 640 ' in SVG: %s", svgStr)
	}
}

func TestRenderSVG_XMLEscaping(t *testing.T) {
	md := `Text with & < > " ' special characters.`
	svgBytes, err := RenderSVG([]byte(md), RenderOptions{Width: 800})
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	var parsed struct {
		XMLName xml.Name `xml:"svg"`
	}
	if err := xml.Unmarshal(svgBytes, &parsed); err != nil {
		t.Fatalf("failed to parse SVG containing escaped entities: %v", err)
	}
}

func TestColorToHexOrRGBA_UnpremultipliedAlpha(t *testing.T) {
	// Opaque color should serialize to hex
	opaque := color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF}
	if hex := colorToHexOrRGBA(opaque); hex != "#123456" {
		t.Errorf("expected #123456, got %s", hex)
	}

	// Partially transparent color: NRGBA has R=200, G=100, B=50, A=128 (~0.502)
	// Must NOT be premultiplied to R=100, G=50, B=25
	semi := color.NRGBA{R: 200, G: 100, B: 50, A: 128}
	rgbaStr := colorToHexOrRGBA(semi)
	if !strings.HasPrefix(rgbaStr, "rgba(200, 100, 50,") {
		t.Errorf("expected unpremultiplied rgba(200, 100, 50, ...), got %s", rgbaStr)
	}

	// Fully transparent should be "none"
	trans := color.RGBA{R: 0, G: 0, B: 0, A: 0}
	if none := colorToHexOrRGBA(trans); none != "none" {
		t.Errorf("expected none for transparent color, got %s", none)
	}

	// Nil color should be "none"
	if none := colorToHexOrRGBA(nil); none != "none" {
		t.Errorf("expected none for nil color, got %s", none)
	}
}

func TestRenderSVG_FontStylesAndClasses(t *testing.T) {
	md := `
**Bold text**
*Italic text*
***Bold italic text***
` + "```go\nvar x = 42\n```" + `
- [ ] Unchecked item
- [x] Checked item
`
	svgBytes, err := RenderSVG([]byte(md), RenderOptions{Width: 600})
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}
	svgStr := string(svgBytes)

	if !strings.Contains(svgStr, `font-weight="bold"`) {
		t.Errorf("expected font-weight=\"bold\" in SVG output")
	}
	if !strings.Contains(svgStr, `font-style="italic"`) {
		t.Errorf("expected font-style=\"italic\" in SVG output")
	}
	if !strings.Contains(svgStr, `class="mono"`) {
		t.Errorf("expected class=\"mono\" for monospace code blocks in SVG output")
	}
	if !strings.Contains(svgStr, `<line `) {
		t.Errorf("expected <line ...> elements for checked task list checkbox in SVG output")
	}
}

// SVG text uses CSS pixels, while the shared display list uses typographic
// points measured with a 96 DPI Go font face. The same scale is required to
// avoid excessive gaps between independently positioned styled text runs.
func TestSVGPainter_PointToCSSPixelFontSize(t *testing.T) {
	dl := &DisplayList{
		Width: 200, Height: 100,
		Items: []DisplayItem{{
			Kind: ItemText, Text: "abc", Color: color.Black,
			FontSize: 16, Point: image.Pt(10, 30),
		}},
	}
	out, err := (&SVGPainter{}).PaintSVG(dl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<text x="10" y="30" font-size="21.33"`) {
		t.Fatalf("expected 16pt to become 21.33 CSS px at 96 DPI; got: %s", out)
	}
}
