package md2png

import (
	"errors"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// DisplayItemKind indicates the type of visual primitive in a DisplayList.
type DisplayItemKind int

const (
	// ItemRect represents a filled rectangle.
	ItemRect DisplayItemKind = iota
	// ItemText represents a styled text run.
	ItemText
	// ItemImage represents a positioned raster image.
	ItemImage
	// ItemLine represents a straight line stroke.
	ItemLine
)

// FontStyle indicates the typographic role/style of a text run.
type FontStyle int

const (
	FontStyleRegular FontStyle = iota
	FontStyleBold
	FontStyleItalic
	FontStyleBoldItalic
	FontStyleMono
)

// DisplayItem represents a single positioned primitive in a DisplayList.
type DisplayItem struct {
	Kind        DisplayItemKind
	Rect        image.Rectangle
	Color       color.Color
	Text        string
	Font        *FontAndFace
	FontStyle   FontStyle
	FontSize    float64
	Point       image.Point // baseline start point for text, start point for line
	Point2      image.Point // end point for line
	StrokeWidth int         // stroke width for line
	Image       image.Image
}

// DisplayList contains the positioned visual primitives produced by the layout pass.
type DisplayList struct {
	Width   int
	Height  int
	BGColor color.Color
	Items   []DisplayItem
}

// Bounds returns the rectangle bounds of the DisplayList.
func (dl *DisplayList) Bounds() image.Rectangle {
	if dl == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, dl.Width, dl.Height)
}

// AddRect appends a filled rectangle to the display list.
func (dl *DisplayList) AddRect(r image.Rectangle, col color.Color) {
	if col == nil || r.Empty() {
		return
	}
	dl.Items = append(dl.Items, DisplayItem{
		Kind:  ItemRect,
		Rect:  r,
		Color: col,
	})
}

// AddText appends a text run to the display list.
func (dl *DisplayList) AddText(text string, fnt *FontAndFace, style FontStyle, size float64, col color.Color, pt image.Point) {
	if text == "" || fnt == nil || col == nil {
		return
	}
	dl.Items = append(dl.Items, DisplayItem{
		Kind:      ItemText,
		Text:      text,
		Font:      fnt,
		FontStyle: style,
		FontSize:  size,
		Color:     col,
		Point:     pt,
	})
}

// AddImage appends an image item to the display list.
func (dl *DisplayList) AddImage(img image.Image, r image.Rectangle) {
	if img == nil || r.Empty() {
		return
	}
	dl.Items = append(dl.Items, DisplayItem{
		Kind:  ItemImage,
		Image: img,
		Rect:  r,
	})
}

// AddLine appends a line stroke to the display list.
func (dl *DisplayList) AddLine(p1, p2 image.Point, col color.Color, strokeWidth int) {
	if col == nil || strokeWidth <= 0 {
		return
	}
	dl.Items = append(dl.Items, DisplayItem{
		Kind:        ItemLine,
		Point:       p1,
		Point2:      p2,
		Color:       col,
		StrokeWidth: strokeWidth,
	})
}

// Painter renders a DisplayList to an output target.
type Painter interface {
	Paint(dl *DisplayList) (image.Image, error)
}

// RasterPainter renders a DisplayList onto an image.RGBA canvas using modern font rasterization.
type RasterPainter struct{}

// Paint rasterizes the given DisplayList into an *image.RGBA.
func (p *RasterPainter) Paint(dl *DisplayList) (image.Image, error) {
	if dl == nil {
		return nil, errors.New("md2png: nil display list")
	}
	w := dl.Width
	h := dl.Height
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if dl.BGColor != nil {
		draw.Draw(img, img.Bounds(), image.NewUniform(dl.BGColor), image.Point{}, draw.Src)
	}

	for _, item := range dl.Items {
		switch item.Kind {
		case ItemRect:
			if item.Color != nil && !item.Rect.Empty() {
				draw.Draw(img, item.Rect, image.NewUniform(item.Color), image.Point{}, draw.Src)
			}
		case ItemText:
			if item.Font != nil && item.Text != "" && item.Color != nil {
				runs := item.Font.segmentRuns(item.Text)
				var d font.Drawer
				d.Dst = img
				d.Src = image.NewUniform(item.Color)
				d.Dot = fixed.Point26_6{X: fixed.I(item.Point.X), Y: fixed.I(item.Point.Y)}

				for _, run := range runs {
					face := run.fnt.getFace(item.FontSize)
					if face == nil {
						continue
					}
					d.Face = face
					d.DrawString(run.text)
				}
			}
		case ItemImage:
			if item.Image != nil && !item.Rect.Empty() {
				draw.Draw(img, item.Rect, item.Image, item.Image.Bounds().Min, draw.Over)
			}
		case ItemLine:
			if item.Color != nil && item.StrokeWidth > 0 {
				drawRasterLine(img, item.Point.X, item.Point.Y, item.Point2.X, item.Point2.Y, item.Color, item.StrokeWidth)
			}
		}
	}

	return img, nil
}

func drawRasterLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, strokeWidth int) {
	b := img.Bounds()
	setPix := func(px, py int) {
		if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
			img.Set(px, py, col)
		}
	}

	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}
	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}
	err := dx - dy

	for {
		for ox := -strokeWidth / 2; ox <= strokeWidth/2; ox++ {
			for oy := -strokeWidth / 2; oy <= strokeWidth/2; oy++ {
				setPix(x0+ox, y0+oy)
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}
