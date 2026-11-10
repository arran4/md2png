package md2png_test

import (
	"bytes"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arran4/md2png"
	"github.com/arran4/md2png/internal/cli"
)

func TestTransparency_RenderOptions(t *testing.T) {
	md := []byte("# Transparent Document\n\nSome paragraph text.\n\n```go\nfmt.Println(\"code\")\n```\n")

	opts := md2png.RenderOptions{
		TransparentBackground: true,
	}

	res, err := md2png.RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	img := res.Image
	bounds := img.Bounds()

	// (0,0) in the margin must be fully transparent
	_, _, _, aTopLeft := img.At(0, 0).RGBA()
	if aTopLeft != 0 {
		t.Errorf("expected pixel (0,0) alpha=0, got %d", aTopLeft)
	}

	// Find non-transparent pixel in rendered text region
	hasOpaque := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				hasOpaque = true
				break
			}
		}
		if hasOpaque {
			break
		}
	}
	if !hasOpaque {
		t.Fatal("expected rendered text to have non-transparent pixels")
	}
}

func TestTransparency_ThemeFileAlpha(t *testing.T) {
	themeCSS := `
@theme transparent-test extends light;
document {
  background: transparent;
  color: #111111;
}
code-block {
  background: #f0f0f0;
}
`
	th, err := md2png.ParseTheme(themeCSS)
	if err != nil {
		t.Fatalf("parse theme error: %v", err)
	}

	r, g, b, a := th.BG.RGBA()
	if a != 0 {
		t.Errorf("expected theme BG alpha 0, got %d (%d, %d, %d)", a, r, g, b)
	}

	md := []byte("# Heading\n")
	res, err := md2png.RenderWithDiagnostics(md, md2png.RenderOptions{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	_, _, _, aMargin := res.Image.At(0, 0).RGBA()
	if aMargin != 0 {
		t.Errorf("expected margin alpha=0 from theme background: transparent, got %d", aMargin)
	}
}

func TestTransparency_PNG_RoundTrip(t *testing.T) {
	md := []byte("# PNG Transparency\n")
	opts := md2png.RenderOptions{TransparentBackground: true}
	res, err := md2png.RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, res.Image); err != nil {
		t.Fatalf("png encode error: %v", err)
	}

	decoded, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("png decode error: %v", err)
	}

	_, _, _, a := decoded.At(0, 0).RGBA()
	if a != 0 {
		t.Errorf("expected decoded PNG (0,0) alpha=0, got %d", a)
	}
}

func TestTransparency_JPEG_Rejection(t *testing.T) {
	tempDir := t.TempDir()
	inPath := filepath.Join(tempDir, "in.md")
	outPath := filepath.Join(tempDir, "out.jpg")
	if err := os.WriteFile(inPath, []byte("# Transparent"), 0o644); err != nil {
		t.Fatal(err)
	}

	format := "jpeg"
	transparent := true
	err := cli.Md2png(&inPath, &outPath, nil, nil, nil, nil, nil, &transparent, nil, nil, nil, nil, nil, nil, nil, nil, nil, &format, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error encoding transparent image as JPEG, got nil")
	}
	if !strings.Contains(err.Error(), "JPEG") || !strings.Contains(err.Error(), "transparency") {
		t.Fatalf("expected JPEG transparency error, got: %v", err)
	}
}

func TestTransparency_GIF_Encoding(t *testing.T) {
	tempDir := t.TempDir()
	inPath := filepath.Join(tempDir, "in.md")
	outPath := filepath.Join(tempDir, "out.gif")
	if err := os.WriteFile(inPath, []byte("# Transparent GIF"), 0o644); err != nil {
		t.Fatal(err)
	}

	format := "gif"
	transparent := true
	err := cli.Md2png(&inPath, &outPath, nil, nil, nil, nil, nil, &transparent, nil, nil, nil, nil, nil, nil, nil, nil, nil, &format, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GIF transparent render failed: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	decGIF, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatalf("failed to decode generated GIF: %v", err)
	}
	if len(decGIF.Image) != 1 {
		t.Errorf("expected 1 frame in GIF, got %d", len(decGIF.Image))
	}

	// Verify transparent pixel at (0,0)
	_, _, _, a := decGIF.Image[0].At(0, 0).RGBA()
	if a != 0 {
		t.Errorf("expected GIF (0,0) alpha=0, got %d", a)
	}
}
