package md2png

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 200, G: 50, B: 50, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return buf.Bytes()
}

func writeTestImage(t *testing.T, path string, width, height int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	data := createTestPNG(t, width, height)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test image: %v", err)
	}
}

func TestPolicy_LocalImagesDisabled(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "local.png")
	writeTestImage(t, imgPath, 20, 20)

	opts := RenderOptions{
		BaseDir: dir,
		ImagePolicy: &ImagePolicy{
			AllowLocal: false,
		},
	}
	_, err := Render([]byte("![img](local.png)"), opts)
	if err == nil {
		t.Fatalf("expected error when local images are disabled, got nil")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected errors.Is(err, ErrPolicyDenied), got: %v", err)
	}
}

func TestPolicy_RemoteImagesDisabled(t *testing.T) {
	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: false,
		},
	}
	_, err := Render([]byte("![img](http://127.0.0.1:9999/image.png)"), opts)
	if err == nil {
		t.Fatalf("expected error when remote images are disabled, got nil")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected errors.Is(err, ErrPolicyDenied), got: %v", err)
	}
}

func TestPolicy_SandboxTraversalEscapeRejected(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.png")
	writeTestImage(t, outsideFile, 20, 20)

	opts := RenderOptions{
		BaseDir: baseDir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:   true,
			SandboxLocal: true,
		},
	}

	testCases := []string{
		"![img](../secret.png)",
		"![img](../../secret.png)",
		"![img](sub/../../secret.png)",
		"![img](sub/../..//../secret.png)",
	}

	for _, tc := range testCases {
		_, err := Render([]byte(tc), opts)
		if err == nil {
			t.Fatalf("expected sandbox violation for %q, got nil", tc)
		}
		if !errors.Is(err, ErrSandboxViolation) {
			t.Fatalf("expected errors.Is(err, ErrSandboxViolation) for %q, got: %v", tc, err)
		}
	}
}

func TestPolicy_SandboxAbsoluteLocalPathRejected(t *testing.T) {
	baseDir := t.TempDir()
	opts := RenderOptions{
		BaseDir: baseDir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:   true,
			SandboxLocal: true,
		},
	}

	testCases := []string{
		"![img](/etc/passwd)",
		"![img](/tmp/secret.png)",
	}

	for _, tc := range testCases {
		_, err := Render([]byte(tc), opts)
		if err == nil {
			t.Fatalf("expected sandbox violation for %q, got nil", tc)
		}
		if !errors.Is(err, ErrSandboxViolation) {
			t.Fatalf("expected errors.Is(err, ErrSandboxViolation) for %q, got: %v", tc, err)
		}
	}
}

func TestPolicy_SandboxFileURIBypassRejected(t *testing.T) {
	baseDir := t.TempDir()
	opts := RenderOptions{
		BaseDir: baseDir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:   true,
			SandboxLocal: true,
		},
	}

	testCases := []string{
		"![img](file:///etc/passwd)",
		"![img](file:///tmp/secret.png)",
		"![img](file://localhost/etc/passwd)",
		"![img](file://localhost/tmp/secret.png)",
	}

	for _, tc := range testCases {
		_, err := Render([]byte(tc), opts)
		if err == nil {
			t.Fatalf("expected sandbox violation for %q, got nil", tc)
		}
		if !errors.Is(err, ErrSandboxViolation) {
			t.Fatalf("expected errors.Is(err, ErrSandboxViolation) for %q, got: %v", tc, err)
		}
	}
}

func TestPolicy_SandboxSymlinkEscapeRejected(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideImg := filepath.Join(outsideDir, "secret.png")
	writeTestImage(t, outsideImg, 20, 20)

	symlinkPath := filepath.Join(baseDir, "symlink.png")
	if err := os.Symlink(outsideImg, symlinkPath); err != nil {
		t.Skipf("skipping symlink test (symlinks not supported): %v", err)
	}

	opts := RenderOptions{
		BaseDir: baseDir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			SandboxLocal:   true,
			MaxImageWidth:  1000,
			MaxImageHeight: 1000,
			MaxImagePixels: 1000000,
		},
	}

	_, err := Render([]byte("![img](symlink.png)"), opts)
	if err == nil {
		t.Fatalf("expected sandbox violation for symlink escape, got nil")
	}
	if !errors.Is(err, ErrSandboxViolation) {
		t.Fatalf("expected errors.Is(err, ErrSandboxViolation), got: %v", err)
	}
}

func TestPolicy_ValidFileInsideBaseDir(t *testing.T) {
	baseDir := t.TempDir()
	imgPath := filepath.Join(baseDir, "images", "valid.png")
	writeTestImage(t, imgPath, 20, 20)

	opts := RenderOptions{
		BaseDir: baseDir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			SandboxLocal:   true,
			MaxImageWidth:  1000,
			MaxImageHeight: 1000,
			MaxImagePixels: 1000000,
		},
	}

	img, err := Render([]byte("![valid](images/valid.png)"), opts)
	if err != nil {
		t.Fatalf("expected success for valid sandboxed file, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected non-nil image returned")
	}
}

func TestPolicy_RemoteRequestUsingSuppliedHTTPClient(t *testing.T) {
	pngData := createTestPNG(t, 20, 20)
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	customClient := server.Client()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     customClient,
			MaxImageWidth:  1000,
			MaxImageHeight: 1000,
			MaxImagePixels: 1000000,
			MaxRemoteBytes: 1024 * 1024,
		},
	}

	img, err := Render([]byte(fmt.Sprintf("![remote](%s/test.png)", server.URL)), opts)
	if err != nil {
		t.Fatalf("expected success with custom HTTP client, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected non-nil image returned")
	}
	if atomic.LoadInt32(&requestCount) == 0 {
		t.Fatalf("expected custom HTTP client to be invoked")
	}
}

func TestPolicy_RedirectToDisallowedDestinationRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer server.Close()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     server.Client(),
			MaxRemoteBytes: 1024 * 1024,
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![redirect](%s/img.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected error for disallowed redirect scheme, got nil")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected errors.Is(err, ErrPolicyDenied), got: %v", err)
	}
}

func TestPolicy_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(createTestPNG(t, 20, 20))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before rendering

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			HTTPClient:  server.Client(),
			Context:     ctx,
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![img](%s/test.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got: %v", err)
	}
}

func TestPolicy_ContextDeadlineTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(createTestPNG(t, 20, 20))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			HTTPClient:  server.Client(),
			Context:     ctx,
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![img](%s/test.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected errors.Is(err, context.DeadlineExceeded), got: %v", err)
	}
}

func TestPolicy_ResponseBodyExceedingMaxRemoteBytes(t *testing.T) {
	pngData := createTestPNG(t, 100, 100) // ~few KB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     server.Client(),
			MaxRemoteBytes: 50, // limit is 50 bytes, PNG is much larger
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![img](%s/test.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for excessive bytes, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}

func TestPolicy_ExcessiveWidth(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "wide.png")
	writeTestImage(t, imgPath, 200, 20)

	opts := RenderOptions{
		BaseDir: dir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			MaxImageWidth:  100,
			MaxImageHeight: 1000,
			MaxImagePixels: 1000000,
		},
	}

	_, err := Render([]byte("![wide](wide.png)"), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for excessive width, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}

func TestPolicy_ExcessiveHeight(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "tall.png")
	writeTestImage(t, imgPath, 20, 200)

	opts := RenderOptions{
		BaseDir: dir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			MaxImageWidth:  1000,
			MaxImageHeight: 100,
			MaxImagePixels: 1000000,
		},
	}

	_, err := Render([]byte("![tall](tall.png)"), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for excessive height, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}

func TestPolicy_ExcessiveTotalPixels(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "square.png")
	writeTestImage(t, imgPath, 100, 100) // 10,000 pixels

	opts := RenderOptions{
		BaseDir: dir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			MaxImageWidth:  500,
			MaxImageHeight: 500,
			MaxImagePixels: 5000, // limit is 5000 pixels
		},
	}

	_, err := Render([]byte("![square](square.png)"), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for excessive pixels, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}

func TestPolicy_NormalLocalImageBelowLimits(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "normal.png")
	writeTestImage(t, imgPath, 50, 50)

	opts := RenderOptions{
		BaseDir: dir,
		ImagePolicy: &ImagePolicy{
			AllowLocal:     true,
			SandboxLocal:   true,
			MaxImageWidth:  100,
			MaxImageHeight: 100,
			MaxImagePixels: 10000,
		},
	}

	img, err := Render([]byte("![normal](normal.png)"), opts)
	if err != nil {
		t.Fatalf("expected success for image below limits, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected non-nil image returned")
	}
}

func TestPolicy_NormalRemoteImageBelowLimits(t *testing.T) {
	pngData := createTestPNG(t, 50, 50)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     server.Client(),
			MaxRemoteBytes: 100 * 1024,
			MaxImageWidth:  100,
			MaxImageHeight: 100,
			MaxImagePixels: 10000,
		},
	}

	img, err := Render([]byte(fmt.Sprintf("![normal](%s/normal.png)", server.URL)), opts)
	if err != nil {
		t.Fatalf("expected success for remote image below limits, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected non-nil image returned")
	}
}

func TestPolicy_BoundedCache(t *testing.T) {
	dir := t.TempDir()
	writeTestImage(t, filepath.Join(dir, "img1.png"), 10, 10)
	writeTestImage(t, filepath.Join(dir, "img2.png"), 10, 10)
	writeTestImage(t, filepath.Join(dir, "img3.png"), 10, 10)

	policy := ImagePolicy{
		AllowLocal:     true,
		MaxCacheItems:  2,
		MaxImageWidth:  100,
		MaxImageHeight: 100,
		MaxImagePixels: 10000,
	}

	fonts, err := LoadFonts(FontConfig{SizeBase: 16})
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	c := newCanvas(640, 48, lightTheme, fonts, 16, 32768)
	r := &renderer{
		c:           c,
		baseSize:    16,
		baseDir:     dir,
		imagePolicy: policy,
	}
	r.ensureImageResolvers()

	// Load img1
	if _, err := r.loadImage("img1.png"); err != nil {
		t.Fatalf("loadImage 1 failed: %v", err)
	}
	// Load img2
	if _, err := r.loadImage("img2.png"); err != nil {
		t.Fatalf("loadImage 2 failed: %v", err)
	}
	if len(r.imageCache) != 2 {
		t.Fatalf("expected 2 items in cache, got %d", len(r.imageCache))
	}

	// Load img3: should evict img1 (FIFO)
	if _, err := r.loadImage("img3.png"); err != nil {
		t.Fatalf("loadImage 3 failed: %v", err)
	}
	if len(r.imageCache) != 2 {
		t.Fatalf("expected cache size to stay at 2, got %d", len(r.imageCache))
	}
	key1 := filepath.Join(dir, "img1.png")
	if _, ok := r.imageCache[key1]; ok {
		t.Fatalf("expected img1 to be evicted from cache")
	}
	key2 := filepath.Join(dir, "img2.png")
	key3 := filepath.Join(dir, "img3.png")
	if _, ok := r.imageCache[key2]; !ok {
		t.Fatalf("expected img2 to be retained in cache")
	}
	if _, ok := r.imageCache[key3]; !ok {
		t.Fatalf("expected img3 to be retained in cache")
	}
}

func TestPolicy_TrustedMissingImageFallback(t *testing.T) {
	markdown := "Paragraph with ![missing](non-existent-image-12345.png)."
	img, err := Render([]byte(markdown), RenderOptions{})
	if err != nil {
		t.Fatalf("expected non-fatal fallback in default trusted mode, got error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected image rendered with fallback text")
	}
}

func TestPolicy_StrictImagePolicy(t *testing.T) {
	strict := StrictImagePolicy()
	if strict.AllowLocal {
		t.Errorf("StrictImagePolicy should have AllowLocal = false")
	}
	if strict.AllowRemote {
		t.Errorf("StrictImagePolicy should have AllowRemote = false")
	}
	if !strict.SandboxLocal {
		t.Errorf("StrictImagePolicy should have SandboxLocal = true")
	}

	opts := RenderOptions{ImagePolicy: &strict}
	_, err := Render([]byte("![local](foo.png)"), opts)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected ErrPolicyDenied for local image under strict policy, got: %v", err)
	}

	_, err = Render([]byte("![remote](https://example.com/foo.png)"), opts)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected ErrPolicyDenied for remote image under strict policy, got: %v", err)
	}
}

func TestPolicy_CallerSuppliedCheckRedirectPreserved(t *testing.T) {
	errCustomRedirect := errors.New("custom redirect denied")
	var redirectChecked int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(createTestPNG(t, 20, 20))
	}))
	defer server.Close()

	customClient := server.Client()
	customClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		atomic.AddInt32(&redirectChecked, 1)
		return errCustomRedirect
	}

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			HTTPClient:  customClient,
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![img](%s/start)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected error from caller-supplied CheckRedirect, got nil")
	}
	if !errors.Is(err, errCustomRedirect) {
		t.Fatalf("expected error to wrap errCustomRedirect, got: %v", err)
	}
	if atomic.LoadInt32(&redirectChecked) == 0 {
		t.Fatalf("expected caller CheckRedirect to be invoked")
	}
}

func TestPolicy_CallerHTTPClientNotMutated(t *testing.T) {
	origClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	origCheckRedirect := origClient.CheckRedirect

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			HTTPClient:  origClient,
		},
	}

	_, _ = Render([]byte("![test](http://127.0.0.1:99999/dummy.png)"), opts)

	if origClient.CheckRedirect != nil && origCheckRedirect == nil {
		t.Fatalf("caller-owned http.Client was mutated: CheckRedirect was overwritten")
	}
}

func TestPolicy_ChunkedResponseExceedingMaxRemoteBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use chunked encoding (no Content-Length header)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("expected ResponseWriter to be Flusher")
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		// Write 50 bytes and flush
		chunk1 := bytes.Repeat([]byte("A"), 50)
		_, _ = w.Write(chunk1)
		flusher.Flush()
		// Write another 50 bytes and flush
		chunk2 := bytes.Repeat([]byte("B"), 50)
		_, _ = w.Write(chunk2)
		flusher.Flush()
	}))
	defer server.Close()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     server.Client(),
			MaxRemoteBytes: 60, // Total sent is 100 bytes, limit is 60
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![chunked](%s/chunked.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for chunked stream exceeding MaxRemoteBytes, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}

func TestPolicy_RemoteImageExcessiveDimensions(t *testing.T) {
	pngData := createTestPNG(t, 200, 200) // 200x200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote:    true,
			HTTPClient:     server.Client(),
			MaxRemoteBytes: 1024 * 1024,
			MaxImageWidth:  50, // limit is 50px
		},
	}

	_, err := Render([]byte(fmt.Sprintf("![wide](%s/wide.png)", server.URL)), opts)
	if err == nil {
		t.Fatalf("expected resource limit error for remote image exceeding width, got nil")
	}
	if !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("expected errors.Is(err, ErrResourceLimit), got: %v", err)
	}
}
