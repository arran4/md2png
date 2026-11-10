package cli

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestReloader_NormalFileEdit(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.md")
	if err := os.WriteFile(filePath, []byte("# Hello 1"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	initialImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var reloadCount int32
	updatedCh := make(chan struct{}, 10)

	reloader := NewReloader(
		filePath,
		[]byte("# Hello 1"),
		initialImg,
		func(data []byte) (image.Image, error) {
			atomic.AddInt32(&reloadCount, 1)
			return image.NewRGBA(image.Rect(0, 0, 120, 120)), nil
		},
		func(img image.Image) {
			select {
			case updatedCh <- struct{}{}:
			default:
			}
		},
	)
	reloader.SetPollInterval(15 * time.Millisecond)
	reloader.SetDebounce(20 * time.Millisecond)
	reloader.Start()
	defer reloader.Stop()

	// Wait brief moment, then modify file
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("# Hello 2"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	select {
	case <-updatedCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for reloader update on normal write")
	}

	if atomic.LoadInt32(&reloadCount) < 1 {
		t.Fatalf("expected at least 1 reload, got %d", atomic.LoadInt32(&reloadCount))
	}
	if reloader.LastGoodImage().Bounds().Dx() != 120 {
		t.Fatalf("expected updated image width 120, got %d", reloader.LastGoodImage().Bounds().Dx())
	}
}

func TestReloader_AtomicReplaceSave(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.md")
	if err := os.WriteFile(filePath, []byte("Version 1"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	initialImg := image.NewRGBA(image.Rect(0, 0, 50, 50))
	updatedCh := make(chan struct{}, 10)

	reloader := NewReloader(
		filePath,
		[]byte("Version 1"),
		initialImg,
		func(data []byte) (image.Image, error) {
			return image.NewRGBA(image.Rect(0, 0, 80, 80)), nil
		},
		func(img image.Image) {
			select {
			case updatedCh <- struct{}{}:
			default:
			}
		},
	)
	reloader.SetPollInterval(15 * time.Millisecond)
	reloader.SetDebounce(20 * time.Millisecond)
	reloader.Start()
	defer reloader.Stop()

	time.Sleep(30 * time.Millisecond)

	// Simulate atomic save: write to temp file, then rename over target
	tempPath := filepath.Join(tmpDir, "doc.md.tmp")
	if err := os.WriteFile(tempPath, []byte("Version 2 from atomic save"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		t.Fatalf("failed to rename temp file: %v", err)
	}

	select {
	case <-updatedCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for atomic replace reload")
	}

	if reloader.LastGoodImage().Bounds().Dx() != 80 {
		t.Fatalf("expected last good image to be updated to 80, got %d", reloader.LastGoodImage().Bounds().Dx())
	}
}

func TestReloader_DebounceRapidSaves(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.md")
	if err := os.WriteFile(filePath, []byte("Initial"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	var renderCalls int32
	initialImg := image.NewRGBA(image.Rect(0, 0, 50, 50))

	reloader := NewReloader(
		filePath,
		[]byte("Initial"),
		initialImg,
		func(data []byte) (image.Image, error) {
			atomic.AddInt32(&renderCalls, 1)
			return image.NewRGBA(image.Rect(0, 0, 60, 60)), nil
		},
		nil,
	)
	reloader.SetPollInterval(10 * time.Millisecond)
	reloader.SetDebounce(60 * time.Millisecond)
	reloader.Start()
	defer reloader.Stop()

	time.Sleep(20 * time.Millisecond)

	// Perform 5 rapid writes within 30ms (shorter than debounce duration)
	for i := 1; i <= 5; i++ {
		_ = os.WriteFile(filePath, []byte("Quick burst"), 0644)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for debounce period to settle
	time.Sleep(120 * time.Millisecond)

	calls := atomic.LoadInt32(&renderCalls)
	// Rapid burst should be debounced into 1 (or at most 2) render calls, not 5
	if calls > 2 {
		t.Fatalf("expected debounced render calls <= 2, got %d", calls)
	}
}

func TestReloader_RetainLastGoodImageOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "doc.md")
	if err := os.WriteFile(filePath, []byte("Good syntax"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	goodImg := image.NewRGBA(image.Rect(0, 0, 150, 150))
	shouldFail := false

	reloader := NewReloader(
		filePath,
		[]byte("Good syntax"),
		goodImg,
		func(data []byte) (image.Image, error) {
			if shouldFail {
				return nil, errors.New("simulated parser/render error")
			}
			return image.NewRGBA(image.Rect(0, 0, 200, 200)), nil
		},
		nil,
	)

	// Trigger a failed reload
	shouldFail = true
	_ = os.WriteFile(filePath, []byte("Broken syntax"), 0644)
	img, err := reloader.ReloadNow()

	if err == nil {
		t.Fatalf("expected error on failed reload")
	}
	// The returned image MUST be the last good image, not nil!
	if img == nil || img.Bounds().Dx() != 150 {
		t.Fatalf("expected last good image (150px) retained on render failure, got: %v", img)
	}
	if reloader.LastGoodImage() != goodImg {
		t.Fatalf("expected reloader.LastGoodImage to retain original good image")
	}

	// Now fix the file and recover
	shouldFail = false
	_ = os.WriteFile(filePath, []byte("Fixed syntax"), 0644)
	recoveredImg, err := reloader.ReloadNow()
	if err != nil {
		t.Fatalf("unexpected error after recovery: %v", err)
	}
	if recoveredImg.Bounds().Dx() != 200 {
		t.Fatalf("expected recovered image 200px, got %d", recoveredImg.Bounds().Dx())
	}
	if reloader.LastError() != nil {
		t.Fatalf("expected last error cleared after successful reload")
	}
}
