package cli

import (
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arran4/md2png"
	xdraw "golang.org/x/image/draw"
)

func TestViewer_FullSessionEvidence(t *testing.T) {
	outDir := filepath.Join("..", "..", "testdata", "viewer")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("failed to create viewer output dir: %v", err)
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "demo.md")
	initialMD := []byte("# Live Viewer Session\n\nWelcome to **md2view**.\n- Live file reload\n- Viewport zoom/pan\n- Resilient recovery\n")
	if err := os.WriteFile(filePath, initialMD, 0o644); err != nil {
		t.Fatalf("failed to write initial markdown: %v", err)
	}

	opts := md2png.RenderOptions{Width: 600}
	initialImg, err := md2png.Render(initialMD, opts)
	if err != nil {
		t.Fatalf("initial render failed: %v", err)
	}

	winWidth, winHeight := 800, 600
	winBuf := image.NewRGBA(image.Rect(0, 0, winWidth, winHeight))
	vp := NewViewport(initialImg.Bounds().Size(), image.Point{X: winWidth, Y: winHeight})

	var currentImg image.Image = initialImg

	paint := func() *image.RGBA {
		draw.Draw(winBuf, winBuf.Bounds(), image.White, image.Point{}, draw.Src)
		if currentImg != nil {
			dr := vp.DestRect()
			xdraw.ApproxBiLinear.Scale(winBuf, dr, currentImg, currentImg.Bounds(), draw.Src, nil)
		}
		snap := image.NewRGBA(winBuf.Bounds())
		draw.Draw(snap, snap.Bounds(), winBuf, image.Point{}, draw.Src)
		return snap
	}

	savePNG := func(name string, img image.Image) {
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("failed to encode %s: %v", name, err)
		}
	}

	// 1. Initial view (centered / top)
	snap1 := paint()
	savePNG("session_1_initial.png", snap1)

	// 2. Viewport fit-to-width and zoom
	vp.FitWidthMode()
	vp.ZoomAt(1.25, image.Point{X: winWidth / 2, Y: winHeight / 2})
	snap2 := paint()
	savePNG("session_2_fit_zoom.png", snap2)

	// 3. Live reload on disk edit
	var failRender int32
	updatedCh := make(chan struct{}, 10)
	reloader := NewReloader(
		filePath,
		initialMD,
		initialImg,
		func(data []byte) (image.Image, error) {
			if atomic.LoadInt32(&failRender) == 1 {
				return nil, errors.New("simulated render error on invalid input")
			}
			return md2png.Render(data, opts)
		},
		func(newImg image.Image) {
			currentImg = newImg
			vp.SetImageSize(newImg.Bounds().Size())
			select {
			case updatedCh <- struct{}{}:
			default:
			}
		},
	)
	reloader.SetPollInterval(10 * time.Millisecond)
	reloader.SetDebounce(10 * time.Millisecond)
	reloader.Start()
	defer reloader.Stop()

	// Write updated file content
	updatedMD := []byte("# Live Reload Completed\n\nContent was edited and saved by editor at live timestamp.\n\n- Zoom preserved\n- Frame cleanly swapped\n")
	if err := os.WriteFile(filePath, updatedMD, 0o644); err != nil {
		t.Fatalf("failed to update markdown: %v", err)
	}

	select {
	case <-updatedCh:
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for live reload")
	}

	snap3 := paint()
	savePNG("session_3_reloaded.png", snap3)

	// 4. Failure recovery (broken save retains last good image)
	atomic.StoreInt32(&failRender, 1)
	if err := os.WriteFile(filePath, []byte("# Corrupted syntax"), 0o644); err != nil {
		t.Fatalf("failed to write corrupted markdown: %v", err)
	}

	// Wait for poll pass
	time.Sleep(50 * time.Millisecond)

	// Reloader must have retained the valid frame from step 3
	lastGood := reloader.LastGoodImage()
	if lastGood == nil || lastGood.Bounds().Empty() {
		t.Fatalf("expected last good image retained on failure")
	}
	currentImg = lastGood

	snap4 := paint()
	savePNG("session_4_failure_recovered.png", snap4)
}
