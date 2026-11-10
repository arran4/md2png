package md2png

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"image/color"
	"image/png"
)

// SVGPainter renders a DisplayList into valid SVG XML vector output.
// Note on SVG limitations: Text runs are emitted as standard scalable <text> elements
// styled with system font families. Because different SVG renderers and client environments
// compute font metrics with slight variations compared to local embedded TTF/OTF fonts,
// layout positioning is approximate and custom-loaded fonts are not embedded as SVG webfonts.
type SVGPainter struct{}

// PaintSVG converts the given DisplayList into SVG vector XML bytes.
func (p *SVGPainter) PaintSVG(dl *DisplayList) ([]byte, error) {
	if dl == nil {
		return nil, fmt.Errorf("md2png: nil display list")
	}

	w := dl.Width
	h := dl.Height
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 %d %d" width="%d" height="%d">`+"\n", w, h, w, h)

	// Styles for clean SVG font rendering
	buf.WriteString("  <style>\n")
	buf.WriteString("    text { font-family: -apple-system, BlinkMacSystemFont, \"Segoe UI\", Roboto, Helvetica, Arial, sans-serif; }\n")
	buf.WriteString("    .mono { font-family: \"SFMono-Regular\", Consolas, \"Liberation Mono\", Menlo, Courier, monospace; }\n")
	buf.WriteString("  </style>\n")

	// Background
	if dl.BGColor != nil {
		bgHex := colorToHexOrRGBA(dl.BGColor)
		if bgHex != "none" {
			fmt.Fprintf(&buf, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", bgHex)
		}
	}

	for _, item := range dl.Items {
		switch item.Kind {
		case ItemRect:
			if item.Color != nil && !item.Rect.Empty() {
				fill := colorToHexOrRGBA(item.Color)
				if fill != "none" {
					fmt.Fprintf(&buf, `  <rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n",
						item.Rect.Min.X, item.Rect.Min.Y, item.Rect.Dx(), item.Rect.Dy(), fill)
				}
			}
		case ItemText:
			if item.Text != "" && item.Color != nil {
				fill := colorToHexOrRGBA(item.Color)
				if fill != "none" {
					var textBuf bytes.Buffer
					_ = xml.EscapeText(&textBuf, []byte(item.Text))

					extraAttrs := ""
					switch item.FontStyle {
					case FontStyleBold:
						extraAttrs = ` font-weight="bold"`
					case FontStyleItalic:
						extraAttrs = ` font-style="italic"`
					case FontStyleBoldItalic:
						extraAttrs = ` font-weight="bold" font-style="italic"`
					case FontStyleMono:
						extraAttrs = ` class="mono"`
					}

					fmt.Fprintf(&buf, `  <text x="%d" y="%d" font-size="%.2f" fill="%s"%s xml:space="preserve">%s</text>`+"\n",
						item.Point.X, item.Point.Y, item.FontSize, fill, extraAttrs, textBuf.String())
				}
			}
		case ItemImage:
			if item.Image != nil && !item.Rect.Empty() {
				var imgBuf bytes.Buffer
				if err := png.Encode(&imgBuf, item.Image); err == nil {
					b64 := base64.StdEncoding.EncodeToString(imgBuf.Bytes())
					fmt.Fprintf(&buf, `  <image x="%d" y="%d" width="%d" height="%d" href="data:image/png;base64,%s"/>`+"\n",
						item.Rect.Min.X, item.Rect.Min.Y, item.Rect.Dx(), item.Rect.Dy(), b64)
				}
			}
		case ItemLine:
			if item.Color != nil && item.StrokeWidth > 0 {
				stroke := colorToHexOrRGBA(item.Color)
				if stroke != "none" {
					fmt.Fprintf(&buf, `  <line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="%d"/>`+"\n",
						item.Point.X, item.Point.Y, item.Point2.X, item.Point2.Y, stroke, item.StrokeWidth)
				}
			}
		}
	}

	buf.WriteString("</svg>\n")
	return buf.Bytes(), nil
}

func colorToHexOrRGBA(c color.Color) string {
	if c == nil {
		return "none"
	}
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	if nrgba.A == 0 {
		return "none"
	}
	if nrgba.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", nrgba.R, nrgba.G, nrgba.B)
	}
	alpha := float64(nrgba.A) / 255.0
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", nrgba.R, nrgba.G, nrgba.B, alpha)
}

// RenderSVG converts the provided Markdown document into an SVG vector document.
func RenderSVG(data []byte, opts RenderOptions) ([]byte, error) {
	layoutRes, err := LayoutDocument(data, opts)
	if err != nil {
		return nil, err
	}
	painter := &SVGPainter{}
	return painter.PaintSVG(layoutRes.DisplayList)
}
