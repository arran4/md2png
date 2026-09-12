package cli

import (
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

func encodeImage(w io.Writer, img image.Image, format string) error {
	switch format {
	case "png":
		return png.Encode(w, img)
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
	case "gif":
		return gif.Encode(w, img, nil)
	default:
		return errors.New("unsupported output format: " + format)
	}
}

func encodeToFile(outPath string, img image.Image, format string) error {
	dir := filepath.Dir(outPath)
	tmpFile, err := os.CreateTemp(dir, "md2png-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmpFile.Name()

	// Ensure cleanup if things fail. If rename succeeds, this will try to remove a non-existent file, which is safe to ignore error.
	defer os.Remove(tmpName)

	if err := encodeImage(tmpFile, img, format); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode image: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("failed to replace destination file: %w", err)
	}

	return nil
}
