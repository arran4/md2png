package cli

import (
	"bytes"
	"image"
	"os"
	"sync"
	"time"
)

// Reloader coordinates file watching, debouncing, and retaining the last good frame on render failure.
type Reloader struct {
	mu            sync.RWMutex
	filePath      string
	debounceDur   time.Duration
	pollInterval  time.Duration
	renderFunc    func([]byte) (image.Image, error)
	onFrameUpdate func(image.Image)

	lastData      []byte
	lastModTime   time.Time
	lastSize      int64
	lastGoodImage image.Image
	lastError     error

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewReloader creates a new Reloader instance.
func NewReloader(filePath string, initialData []byte, initialImg image.Image, renderFunc func([]byte) (image.Image, error), onFrameUpdate func(image.Image)) *Reloader {
	var modTime time.Time
	var size int64
	if fi, err := os.Stat(filePath); err == nil {
		modTime = fi.ModTime()
		size = fi.Size()
	}

	return &Reloader{
		filePath:      filePath,
		debounceDur:   60 * time.Millisecond,
		pollInterval:  80 * time.Millisecond,
		renderFunc:    renderFunc,
		onFrameUpdate: onFrameUpdate,
		lastData:      initialData,
		lastModTime:   modTime,
		lastSize:      size,
		lastGoodImage: initialImg,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// SetDebounce sets the debounce duration for testing or customization.
func (r *Reloader) SetDebounce(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debounceDur = d
}

// SetPollInterval sets the polling interval.
func (r *Reloader) SetPollInterval(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pollInterval = d
}

// LastGoodImage returns the current successfully rendered frame.
func (r *Reloader) LastGoodImage() image.Image {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastGoodImage
}

// LastError returns the error from the most recent failed reload attempt, if any.
func (r *Reloader) LastError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastError
}

// ReloadNow attempts an immediate reload from the file.
func (r *Reloader) ReloadNow() (image.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := readFileWithRetry(r.filePath, 5, 10*time.Millisecond)
	if err != nil {
		r.lastError = err
		return r.lastGoodImage, err
	}

	if fi, err := os.Stat(r.filePath); err == nil {
		r.lastModTime = fi.ModTime()
		r.lastSize = fi.Size()
	}

	newImg, err := r.renderFunc(data)
	if err != nil {
		r.lastError = err
		// Keep last good image visible when re-rendering fails
		return r.lastGoodImage, err
	}

	r.lastData = data
	r.lastGoodImage = newImg
	r.lastError = nil
	if r.onFrameUpdate != nil {
		r.onFrameUpdate(newImg)
	}
	return newImg, nil
}

// Start begins background file watching.
func (r *Reloader) Start() {
	if r.filePath == "" {
		return
	}
	go r.watchLoop()
}

// Stop cleanly terminates background watching.
func (r *Reloader) Stop() {
	select {
	case <-r.stopCh:
		return
	default:
		close(r.stopCh)
		<-r.doneCh
	}
}

func (r *Reloader) watchLoop() {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	var pendingSince time.Time
	var pending bool

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.mu.RLock()
			lastMod := r.lastModTime
			lastSz := r.lastSize
			debounce := r.debounceDur
			r.mu.RUnlock()

			fi, err := os.Stat(r.filePath)
			if err != nil {
				// File may be in the middle of atomic rename/replace; retry on next tick
				continue
			}

			if fi.ModTime() != lastMod || fi.Size() != lastSz {
				if !pending {
					pending = true
					pendingSince = time.Now()
				}
			}

			if pending && time.Since(pendingSince) >= debounce {
				pending = false
				_, _ = r.ReloadNow()
			}
		}
	}
}

// readFileWithRetry reads a file, retrying briefly if the file is momentarily unavailable (atomic save pattern).
func readFileWithRetry(path string, maxAttempts int, retryDelay time.Duration) ([]byte, error) {
	var err error
	var data []byte
	for i := 0; i < maxAttempts; i++ {
		data, err = os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if i < maxAttempts-1 {
			time.Sleep(retryDelay)
		}
	}
	if err == nil {
		return data, nil
	}
	return nil, err
}

// CheckIfDataChanged reports whether newData differs from oldData.
func CheckIfDataChanged(oldData, newData []byte) bool {
	return !bytes.Equal(oldData, newData)
}
