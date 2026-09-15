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

func outputFileMode(outPath string) (os.FileMode, error) {
	info, err := os.Stat(outPath)
	if err == nil {
		return info.Mode().Perm(), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("failed to inspect destination file: %w", err)
	}
	return 0o644, nil
}

func encodeToFile(outPath string, img image.Image, format string) error {
	return encodeToFileWithEncoder(outPath, img, format, encodeImage)
}

func encodeToFileWithEncoder(
	outPath string,
	img image.Image,
	format string,
	encoder func(io.Writer, image.Image, string) error,
) error {
	mode, err := outputFileMode(outPath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(outPath)
	tmpFile, err := os.CreateTemp(dir, "md2png-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmpFile.Name()

	// Ensure cleanup if anything fails before the rename completes.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to set temporary file permissions: %w", err)
	}

	if err := encoder(tmpFile, img, format); err != nil {
		_ = tmpFile.Close()
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
