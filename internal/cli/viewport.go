package cli

import (
	"image"
	"math"
)

// FitMode represents the automatic fitting mode of the viewport.
type FitMode int

const (
	FitNone FitMode = iota
	FitWidth
	FitWindow
)

const (
	MinZoom = 0.05
	MaxZoom = 20.0
)

// Viewport handles zoom, pan, fit modes, and coordinate math for md2view.
type Viewport struct {
	ImageSize  image.Point
	WindowSize image.Point
	Zoom       float64
	Offset     image.Point
	Fit        FitMode
}

// NewViewport constructs an initialized Viewport.
func NewViewport(imgSize, winSize image.Point) *Viewport {
	vp := &Viewport{
		ImageSize:  imgSize,
		WindowSize: winSize,
		Zoom:       1.0,
		Fit:        FitNone,
	}
	vp.Constrain()
	return vp
}

// SetWindowSize updates the viewport window dimensions and recomputes fit modes.
func (vp *Viewport) SetWindowSize(winSize image.Point) {
	if winSize.X <= 0 || winSize.Y <= 0 {
		return
	}
	vp.WindowSize = winSize
	switch vp.Fit {
	case FitWidth:
		vp.applyFitWidth()
	case FitWindow:
		vp.applyFitWindow()
	default:
		vp.Constrain()
	}
}

// SetImageSize updates the backing image dimensions and recomputes fit modes.
func (vp *Viewport) SetImageSize(imgSize image.Point) {
	if imgSize.X <= 0 || imgSize.Y <= 0 {
		return
	}
	vp.ImageSize = imgSize
	switch vp.Fit {
	case FitWidth:
		vp.applyFitWidth()
	case FitWindow:
		vp.applyFitWindow()
	default:
		vp.Constrain()
	}
}

func (vp *Viewport) applyFitWidth() {
	if vp.ImageSize.X <= 0 || vp.WindowSize.X <= 0 {
		return
	}
	vp.Zoom = float64(vp.WindowSize.X) / float64(vp.ImageSize.X)
	vp.clampZoom()
	scaledW := int(float64(vp.ImageSize.X) * vp.Zoom)
	if vp.WindowSize.X > scaledW {
		vp.Offset.X = (vp.WindowSize.X - scaledW) / 2
	} else {
		vp.Offset.X = 0
	}
	vp.Constrain()
}

func (vp *Viewport) applyFitWindow() {
	if vp.ImageSize.X <= 0 || vp.ImageSize.Y <= 0 || vp.WindowSize.X <= 0 || vp.WindowSize.Y <= 0 {
		return
	}
	zoomX := float64(vp.WindowSize.X) / float64(vp.ImageSize.X)
	zoomY := float64(vp.WindowSize.Y) / float64(vp.ImageSize.Y)
	vp.Zoom = math.Min(zoomX, zoomY)
	vp.clampZoom()

	scaledW := int(float64(vp.ImageSize.X) * vp.Zoom)
	scaledH := int(float64(vp.ImageSize.Y) * vp.Zoom)
	vp.Offset.X = (vp.WindowSize.X - scaledW) / 2
	vp.Offset.Y = (vp.WindowSize.Y - scaledH) / 2
	vp.Constrain()
}

// FitWidthMode activates fit-to-width mode.
func (vp *Viewport) FitWidthMode() {
	vp.Fit = FitWidth
	vp.applyFitWidth()
}

// FitWindowMode activates fit-to-window mode.
func (vp *Viewport) FitWindowMode() {
	vp.Fit = FitWindow
	vp.applyFitWindow()
}

// Reset resets zoom to 100% and scrolls to the top.
func (vp *Viewport) Reset() {
	vp.Fit = FitNone
	vp.Zoom = 1.0
	scaledW := vp.ImageSize.X
	if vp.WindowSize.X > scaledW {
		vp.Offset.X = (vp.WindowSize.X - scaledW) / 2
	} else {
		vp.Offset.X = 0
	}
	vp.Offset.Y = 0
	vp.Constrain()
}

// Recenter centers the document within the current window.
func (vp *Viewport) Recenter() {
	scaledW := int(float64(vp.ImageSize.X) * vp.Zoom)
	scaledH := int(float64(vp.ImageSize.Y) * vp.Zoom)
	vp.Offset.X = (vp.WindowSize.X - scaledW) / 2
	vp.Offset.Y = (vp.WindowSize.Y - scaledH) / 2
	vp.Constrain()
}

// ScrollToTop scrolls to the top of the document.
func (vp *Viewport) ScrollToTop() {
	vp.Offset.Y = 0
	vp.Constrain()
}

// ZoomAt scales the viewport around an anchor point (e.g. cursor or window center).
func (vp *Viewport) ZoomAt(factor float64, anchor image.Point) {
	if factor <= 0 {
		return
	}
	vp.Fit = FitNone
	oldZoom := vp.Zoom
	newZoom := oldZoom * factor
	if newZoom < MinZoom {
		newZoom = MinZoom
	}
	if newZoom > MaxZoom {
		newZoom = MaxZoom
	}
	if newZoom == oldZoom {
		return
	}

	// Anchor coordinate in unscaled image space
	imgX := float64(anchor.X-vp.Offset.X) / oldZoom
	imgY := float64(anchor.Y-vp.Offset.Y) / oldZoom

	vp.Zoom = newZoom
	vp.Offset.X = anchor.X - int(math.Round(imgX*newZoom))
	vp.Offset.Y = anchor.Y - int(math.Round(imgY*newZoom))
	vp.Constrain()
}

// Pan offsets the viewport by the given delta.
func (vp *Viewport) Pan(delta image.Point) {
	vp.Fit = FitNone
	vp.Offset.X += delta.X
	vp.Offset.Y += delta.Y
	vp.Constrain()
}

func (vp *Viewport) clampZoom() {
	if vp.Zoom < MinZoom {
		vp.Zoom = MinZoom
	}
	if vp.Zoom > MaxZoom {
		vp.Zoom = MaxZoom
	}
}

// Constrain clamps zoom and ensures the document cannot be panned completely off screen.
func (vp *Viewport) Constrain() {
	vp.clampZoom()

	if vp.WindowSize.X <= 0 || vp.WindowSize.Y <= 0 || vp.ImageSize.X <= 0 || vp.ImageSize.Y <= 0 {
		return
	}

	scaledW := int(float64(vp.ImageSize.X) * vp.Zoom)
	scaledH := int(float64(vp.ImageSize.Y) * vp.Zoom)

	// Keep at least a margin of the document visible inside the window
	marginH := 40
	if vp.WindowSize.X < 80 {
		marginH = vp.WindowSize.X / 2
	}
	marginV := 40
	if vp.WindowSize.Y < 80 {
		marginV = vp.WindowSize.Y / 2
	}

	minX := -scaledW + marginH
	maxX := vp.WindowSize.X - marginH
	if minX > maxX {
		vp.Offset.X = (vp.WindowSize.X - scaledW) / 2
	} else {
		if vp.Offset.X < minX {
			vp.Offset.X = minX
		}
		if vp.Offset.X > maxX {
			vp.Offset.X = maxX
		}
	}

	minY := -scaledH + marginV
	maxY := vp.WindowSize.Y - marginV
	if minY > maxY {
		vp.Offset.Y = (vp.WindowSize.Y - scaledH) / 2
	} else {
		if vp.Offset.Y < minY {
			vp.Offset.Y = minY
		}
		if vp.Offset.Y > maxY {
			vp.Offset.Y = maxY
		}
	}
}

// DestRect returns the target draw rectangle for the scaled image in the window buffer.
func (vp *Viewport) DestRect() image.Rectangle {
	scaledW := int(float64(vp.ImageSize.X) * vp.Zoom)
	scaledH := int(float64(vp.ImageSize.Y) * vp.Zoom)
	return image.Rect(vp.Offset.X, vp.Offset.Y, vp.Offset.X+scaledW, vp.Offset.Y+scaledH)
}
