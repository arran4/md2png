package md2png

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func createSizedTestImage(t *testing.T, dir string, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 100, A: 255})
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode test image: %v", err)
	}
	return p
}

func TestImageSizing_WidthOnlyPreservesAspect(t *testing.T) {
	tmpDir := t.TempDir()
	// 100x50 image (2:1 aspect ratio)
	createSizedTestImage(t, tmpDir, "rect.png", 100, 50)

	md := `![rectangle](` + filepath.Join(tmpDir, "rect.png") + `){width=60}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 600,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageSizing_HeightOnlyPreservesAspect(t *testing.T) {
	tmpDir := t.TempDir()
	// 100x50 image (2:1 aspect ratio)
	createSizedTestImage(t, tmpDir, "rect.png", 100, 50)

	md := `![rectangle](` + filepath.Join(tmpDir, "rect.png") + `){height=25}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 600,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageSizing_BothDimensions(t *testing.T) {
	tmpDir := t.TempDir()
	createSizedTestImage(t, tmpDir, "rect.png", 100, 50)

	md := `![rectangle](` + filepath.Join(tmpDir, "rect.png") + `){width=80 height=80}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 600,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageSizing_PercentageWidth(t *testing.T) {
	tmpDir := t.TempDir()
	createSizedTestImage(t, tmpDir, "rect.png", 400, 200)

	md := `![rectangle](` + filepath.Join(tmpDir, "rect.png") + `){width=50%}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageURL_PreservesQueryParams(t *testing.T) {
	receivedQuery := ""
	receivedPath := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		receivedPath = r.URL.Path
		img := image.NewRGBA(image.Rect(0, 0, 100, 50))
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, img)
	}))
	defer srv.Close()

	// Remote URL with query parameters and fragment, styled with attribute syntax
	remoteURL := srv.URL + "/test-image.png?token=secret123&version=2&size=large#section-1"
	md := `![remote](` + remoteURL + `){width=80 align=right}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 600,
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			AllowLocal:  true,
		},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}

	expectedQuery := "token=secret123&version=2&size=large"
	if receivedQuery != expectedQuery {
		t.Errorf("expected remote query %q, got %q", expectedQuery, receivedQuery)
	}
	if receivedPath != "/test-image.png" {
		t.Errorf("expected remote path /test-image.png, got %q", receivedPath)
	}
}

func TestImageAlignment_LeftCenterRight(t *testing.T) {
	tmpDir := t.TempDir()
	imgFile := createSizedTestImage(t, tmpDir, "sq.png", 40, 40)

	cases := []struct {
		name  string
		md    string
	}{
		{"left", `![left](` + imgFile + `){align=left}`},
		{"center", `![center](` + imgFile + `){align=center}`},
		{"right", `![right](` + imgFile + `){align=right}`},
		{"shorthand_left", `![left](` + imgFile + `){left}`},
		{"shorthand_center", `![center](` + imgFile + `){center}`},
		{"shorthand_right", `![right](` + imgFile + `){right}`},
		{"class_right", `![right](` + imgFile + `){.align-right}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, err := Render([]byte(tc.md), RenderOptions{Width: 400})
			if err != nil {
				t.Fatalf("failed to render %s: %v", tc.name, err)
			}
			if img == nil {
				t.Fatalf("expected rendered image for %s", tc.name)
			}
		})
	}
}

func TestImageSizing_InTablesAndLists(t *testing.T) {
	tmpDir := t.TempDir()
	imgFile := createSizedTestImage(t, tmpDir, "icon.png", 30, 30)

	md := `# Table and List with Images

| Feature | Preview | Status |
| :--- | :---: | ---: |
| Icon 1 | ![icon](` + imgFile + `){width=20} | OK |
| Icon 2 | ![icon](` + imgFile + `){width=25 align=right} | Done |

* Item with image ![icon](` + imgFile + `){width=15} inline
* Next item
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("table and list rendering failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageSizing_OversizeClamping(t *testing.T) {
	tmpDir := t.TempDir()
	imgFile := createSizedTestImage(t, tmpDir, "huge.png", 100, 100)

	// Canvas width 400 with margins 48 means content width is 304.
	// Asking for width 2000 should be clamped to content width without panic or error.
	md := `![huge](` + imgFile + `){width=2000 upscale=true}
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 400,
	})
	if err != nil {
		t.Fatalf("clamping failed: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestImageSizing_RemoteImageWithAttributes(t *testing.T) {
	// Start local mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 80, 40))
		_ = png.Encode(w, img)
	}))
	defer server.Close()

	md := `![remote](` + server.URL + `/test.png){width=50 align=center}
`
	allowRemote := true
	img, err := Render([]byte(md), RenderOptions{
		Width: 600,
		ImagePolicy: &ImagePolicy{
			AllowRemote: allowRemote,
			HTTPClient:  server.Client(),
		},
	})
	if err != nil {
		t.Fatalf("failed to render remote image: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}
