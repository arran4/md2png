package md2png

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"image/color"
	"image/png"
	"strings"
)

// SVGPainter renders a DisplayList into valid SVG XML vector output.
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
		_, _, _, a := dl.BGColor.RGBA()
		if a > 0 {
			bgHex := colorToHexOrRGBA(dl.BGColor)
			fmt.Fprintf(&buf, `  <rect width="100%%" height="100%%" fill="%s"/>`+"\n", bgHex)
		}
	}

	for _, item := range dl.Items {
		switch item.Kind {
		case ItemRect:
			if item.Color != nil && !item.Rect.Empty() {
				fill := colorToHexOrRGBA(item.Color)
				fmt.Fprintf(&buf, `  <rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n",
					item.Rect.Min.X, item.Rect.Min.Y, item.Rect.Dx(), item.Rect.Dy(), fill)
			}
		case ItemText:
			if item.Text != "" && item.Color != nil {
				fill := colorToHexOrRGBA(item.Color)
				var textBuf bytes.Buffer
				_ = xml.EscapeText(&textBuf, []byte(item.Text))

				class := ""
				if item.Font != nil && (strings.Contains(strings.ToLower(fmt.Sprintf("%v", item.Font)), "mono")) {
					class = ` class="mono"`
				}

				fmt.Fprintf(&buf, `  <text x="%d" y="%d" font-size="%.2f" fill="%s"%s xml:space="preserve">%s</text>`+"\n",
					item.Point.X, item.Point.Y, item.FontSize, fill, class, textBuf.String())
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
		}
	}

	buf.WriteString("</svg>\n")
	return buf.Bytes(), nil
}

func colorToHexOrRGBA(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return "none"
	}
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	a8 := uint8(a >> 8)
	if a8 == 255 {
		return fmt.Sprintf("#%02x%02x%02x", r8, g8, b8)
	}
	alpha := float64(a8) / 255.0
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", r8, g8, b8, alpha)
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
