package md2png

import (
	"encoding/xml"
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
