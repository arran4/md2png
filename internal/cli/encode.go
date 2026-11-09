package cli

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

func hasTransparency(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}

func imageToPalettedWithTransparency(img image.Image) *image.Paletted {
	bounds := img.Bounds()
	paletteMap := make(map[color.RGBA]uint8)
	var pal color.Palette
	// Index 0 is transparent
	pal = append(pal, color.RGBA{0, 0, 0, 0})

	paletted := image.NewPaletted(bounds, nil)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			_, _, _, a := c.RGBA()
			if a < 128*257 {
				continue
			}
			r, g, b, _ := c.RGBA()
			rgba := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
			if _, ok := paletteMap[rgba]; !ok && len(pal) < 256 {
				paletteMap[rgba] = uint8(len(pal))
				pal = append(pal, rgba)
			}
		}
	}
	paletted.Palette = pal

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			_, _, _, a := c.RGBA()
			if a < 128*257 {
				paletted.SetColorIndex(x, y, 0)
			} else {
				r, g, b, _ := c.RGBA()
				rgba := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
				if idx, ok := paletteMap[rgba]; ok {
					paletted.SetColorIndex(x, y, idx)
				} else {
					paletted.SetColorIndex(x, y, uint8(pal.Index(rgba)))
				}
			}
		}
	}
	return paletted
}

func encodeImage(w io.Writer, img image.Image, format string) error {
	switch format {
	case "png":
		return png.Encode(w, img)
	case "jpg", "jpeg":
		if hasTransparency(img) {
			return errors.New("cannot encode transparent image as JPEG: JPEG format does not support transparency")
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
	case "gif":
		if hasTransparency(img) {
			pal := imageToPalettedWithTransparency(img)
			return gif.Encode(w, pal, nil)
		}
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
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(mode); err != nil {
		return fmt.Errorf("failed to apply destination mode to temporary file: %w", err)
	}

	if err := encoder(tmpFile, img, format); err != nil {
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("failed to commit rendered output: %w", err)
	}

	cleanup = false
	return nil
}
