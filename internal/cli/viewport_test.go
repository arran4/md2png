package cli

import (
	"image"
	"math"
	"testing"
)

func TestViewport_InitialState(t *testing.T) {
	imgSize := image.Point{X: 800, Y: 1200}
	winSize := image.Point{X: 1000, Y: 800}
	vp := NewViewport(imgSize, winSize)

	if vp.Zoom != 1.0 {
		t.Fatalf("expected zoom 1.0, got %f", vp.Zoom)
	}
	if vp.Fit != FitNone {
		t.Fatalf("expected FitNone, got %v", vp.Fit)
	}
	dr := vp.DestRect()
	if dr.Dx() != 800 || dr.Dy() != 1200 {
		t.Fatalf("expected dest rect 800x1200, got %dx%d", dr.Dx(), dr.Dy())
	}
}

func TestViewport_FitWidth(t *testing.T) {
	imgSize := image.Point{X: 800, Y: 1200}
	winSize := image.Point{X: 400, Y: 600}
	vp := NewViewport(imgSize, winSize)

	vp.FitWidthMode()
	if vp.Fit != FitWidth {
		t.Fatalf("expected FitWidth mode")
	}
	expectedZoom := 400.0 / 800.0
	if math.Abs(vp.Zoom-expectedZoom) > 0.001 {
		t.Fatalf("expected zoom %f, got %f", expectedZoom, vp.Zoom)
	}
	dr := vp.DestRect()
	if dr.Dx() != 400 {
		t.Fatalf("expected scaled width 400, got %d", dr.Dx())
	}

	// Resize window should automatically recompute fit width
	vp.SetWindowSize(image.Point{X: 600, Y: 800})
	expectedZoom2 := 600.0 / 800.0
	if math.Abs(vp.Zoom-expectedZoom2) > 0.001 {
		t.Fatalf("expected recomputed zoom %f, got %f", expectedZoom2, vp.Zoom)
	}
}

func TestViewport_FitWindow(t *testing.T) {
	imgSize := image.Point{X: 1000, Y: 2000}
	winSize := image.Point{X: 500, Y: 500}
	vp := NewViewport(imgSize, winSize)

	vp.FitWindowMode()
	if vp.Fit != FitWindow {
		t.Fatalf("expected FitWindow mode")
	}
	// 500/1000 = 0.5, 500/2000 = 0.25 -> zoom should be 0.25
	expectedZoom := 0.25
	if math.Abs(vp.Zoom-expectedZoom) > 0.001 {
		t.Fatalf("expected zoom %f, got %f", expectedZoom, vp.Zoom)
	}
	dr := vp.DestRect()
	if dr.Dy() != 500 {
		t.Fatalf("expected scaled height 500, got %d", dr.Dy())
	}
	if dr.Dx() != 250 {
		t.Fatalf("expected scaled width 250, got %d", dr.Dx())
	}
	// Centered horizontally: (500 - 250) / 2 = 125
	if dr.Min.X != 125 {
		t.Fatalf("expected horizontal centering at 125, got %d", dr.Min.X)
	}
}

func TestViewport_ResetAndScrollToTop(t *testing.T) {
	imgSize := image.Point{X: 800, Y: 1200}
	winSize := image.Point{X: 800, Y: 600}
	vp := NewViewport(imgSize, winSize)

	vp.ZoomAt(2.5, image.Point{X: 400, Y: 300})
	vp.Pan(image.Point{X: -200, Y: -500})

	vp.Reset()
	if vp.Zoom != 1.0 {
		t.Fatalf("expected zoom 1.0 after reset, got %f", vp.Zoom)
	}
	if vp.Offset.Y != 0 {
		t.Fatalf("expected offset Y = 0 after reset, got %d", vp.Offset.Y)
	}

	vp.Pan(image.Point{Y: -300})
	if vp.Offset.Y >= 0 {
		t.Fatalf("expected negative Y after pan down")
	}
	vp.ScrollToTop()
	if vp.Offset.Y != 0 {
		t.Fatalf("expected offset Y = 0 after ScrollToTop, got %d", vp.Offset.Y)
	}
}

func TestViewport_ZoomAtAnchor(t *testing.T) {
	imgSize := image.Point{X: 800, Y: 800}
	winSize := image.Point{X: 800, Y: 800}
	vp := NewViewport(imgSize, winSize)
	vp.Offset = image.Point{X: 0, Y: 0}

	// Zoom in 2x anchored at (200, 200)
	anchor := image.Point{X: 200, Y: 200}
	vp.ZoomAt(2.0, anchor)

	// Point (200, 200) was in image space at (200, 200).
	// Under 2x zoom and new offset, (imgX * 2.0) + Offset.X should equal anchor.X:
	// 200 * 2.0 + Offset.X = 200 => Offset.X = -200.
	if vp.Offset.X != -200 || vp.Offset.Y != -200 {
		t.Fatalf("expected offset (-200, -200), got (%d, %d)", vp.Offset.X, vp.Offset.Y)
	}
}

func TestViewport_ZoomClamping(t *testing.T) {
	imgSize := image.Point{X: 500, Y: 500}
	winSize := image.Point{X: 500, Y: 500}
	vp := NewViewport(imgSize, winSize)

	// Zoom in repeatedly
	for i := 0; i < 50; i++ {
		vp.ZoomAt(1.5, image.Point{X: 250, Y: 250})
	}
	if vp.Zoom > MaxZoom {
		t.Fatalf("expected zoom capped at %f, got %f", MaxZoom, vp.Zoom)
	}

	// Zoom out repeatedly
	for i := 0; i < 100; i++ {
		vp.ZoomAt(0.5, image.Point{X: 250, Y: 250})
	}
	if vp.Zoom < MinZoom {
		t.Fatalf("expected zoom floor at %f, got %f", MinZoom, vp.Zoom)
	}
}

func TestViewport_PanConstrainedNotLostOffScreen(t *testing.T) {
	imgSize := image.Point{X: 400, Y: 400}
	winSize := image.Point{X: 500, Y: 500}
	vp := NewViewport(imgSize, winSize)

	// Pan extremely far to the left
	vp.Pan(image.Point{X: -10000, Y: -10000})

	dr := vp.DestRect()
	// Must still have some intersection with window [0, 500] x [0, 500]
	intersect := dr.Intersect(image.Rect(0, 0, 500, 500))
	if intersect.Empty() {
		t.Fatalf("expected image to remain constrained within view, but destRect %v does not intersect window", dr)
	}
}
