package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func TestImageToPalettedWithTransparency_UnpremultipliedColors(t *testing.T) {
	// 4x4 test image with:
	// - (0,0): fully transparent
	// - (1,0): partially transparent (alpha < 128) -> should be transparent
	// - (2,0): partially transparent (alpha >= 128) with rich color -> must NOT be darkened
	// - (3,0): fully opaque
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))

	// (0,0): fully transparent
	img.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})

	// (1,0): alpha 100 (< 128) -> becomes transparent (index 0)
	img.SetNRGBA(1, 0, color.NRGBA{R: 100, G: 200, B: 50, A: 100})

	// (2,0): alpha 200 (>= 128) -> promoted to opaque (A=255) with R=200, G=100, B=50 preserved
	img.SetNRGBA(2, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 200})

	// (3,0): fully opaque
	img.SetNRGBA(3, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	paletted := imageToPalettedWithTransparency(img)
	if paletted == nil {
		t.Fatal("expected non-nil paletted image")
	}

	// (0,0) must have index 0 (transparent)
	if idx := paletted.ColorIndexAt(0, 0); idx != 0 {
		t.Errorf("expected index 0 at (0,0), got %d", idx)
	}

	// (1,0) must have index 0 (alpha < 128 thresholded to transparent)
	if idx := paletted.ColorIndexAt(1, 0); idx != 0 {
		t.Errorf("expected index 0 at (1,0) for alpha < 128, got %d", idx)
	}

	// (2,0) must NOT be index 0
	idx2 := paletted.ColorIndexAt(2, 0)
	if idx2 == 0 {
		t.Fatalf("expected non-zero index at (2,0)")
	}

	col2 := paletted.Palette[idx2].(color.RGBA)
	// Color must NOT be darkened by premultiplication (200*200/255 = 156)
	if col2.R != 200 || col2.G != 100 || col2.B != 50 {
		t.Errorf("expected preserved unpremultiplied RGB (200, 100, 50), got (%d, %d, %d)",
			col2.R, col2.G, col2.B)
	}
	if col2.A != 255 {
		t.Errorf("expected promoted opaque alpha 255, got %d", col2.A)
	}

	// Encode to GIF and decode back to verify GIF encoder accepts palette index 0 as transparent
	var buf bytes.Buffer
	if err := gif.Encode(&buf, paletted, nil); err != nil {
		t.Fatalf("gif encode error: %v", err)
	}

	dec, err := gif.Decode(&buf)
	if err != nil {
		t.Fatalf("gif decode error: %v", err)
	}

	_, _, _, a0 := dec.At(0, 0).RGBA()
	if a0 != 0 {
		t.Errorf("expected decoded GIF (0,0) alpha=0, got %d", a0)
	}

	r2, g2, b2, a2 := dec.At(2, 0).RGBA()
	if a2 != 0xffff {
		t.Errorf("expected decoded GIF (2,0) alpha=0xffff, got %d", a2)
	}
	if uint8(r2>>8) != 200 || uint8(g2>>8) != 100 || uint8(b2>>8) != 50 {
		t.Errorf("expected decoded GIF (2,0) RGB (200, 100, 50), got (%d, %d, %d)",
			r2>>8, g2>>8, b2>>8)
	}
}
