package md2png

import (
	"errors"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Md2png is a subcommand md2png that renders Markdown to an image
func Md2png(
	in *string, // flag: --in Input Markdown file (default: stdin if empty)
	out *string, // flag: --out Output image file (.png, .jpg, or .gif) (default: "out.png")
	width *int, // flag: --width Output image width in pixels (0 for default)
	margin *int, // flag: --margin Margin in pixels (0 for default)
	pt *float64, // flag: --pt Base font size in points (paragraph) (0 for default)
	theme *string, // flag: --theme Theme: light|dark (default: "light")
	fontRegular *string, // flag: --font Path to TTF for regular text (optional; default Go Regular)
	fontBold *string, // flag: --fontbold Path to TTF for bold text (optional; default Go Bold)
	fontMono *string, // flag: --fontmono Path to TTF for mono/code (optional; default Go Mono)
	footnoteLinks *bool, // flag: --footnote-links Add footnotes for link destinations (default: true)
	footnoteImages *bool, // flag: --footnote-images Add footnotes for image destinations (default: false)
	maxHeight *int, // flag: --max-height Maximum output height in pixels (0 for default)
) error {
	var data []byte
	var baseDir string
	var err error

	inPath := ""
	if in != nil {
		inPath = *in
	}
	outPath := "out.png"
	if out != nil {
		outPath = *out
	}

	if inPath == "" {
		data, err = io.ReadAll(os.Stdin)
		if err == nil {
			baseDir, err = os.Getwd()
		}
	} else {
		var f *os.File
		f, err = os.Open(inPath)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		data, err = io.ReadAll(f)
		if err == nil {
			baseDir, err = filepath.Abs(filepath.Dir(inPath))
		}
	}
	if err != nil {
		return err
	}

	opts, err := ConvertCommandArgsToRenderOptions(width, margin, pt, theme, fontRegular, fontBold, fontMono, footnoteLinks, footnoteImages, maxHeight, baseDir)
	if err != nil {
		return err
	}

	img, err := Render(data, opts)
	if err != nil {
		return err
	}

	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	ext := strings.ToLower(filepath.Ext(outPath))
	switch ext {
	case ".png":
		if err := png.Encode(file, img); err != nil {
			return err
		}
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 92}); err != nil {
			return err
		}
	case ".gif":
		if err := gif.Encode(file, img, nil); err != nil {
			return err
		}
	default:
		return errors.New("unsupported output extension: " + ext)
	}

	return nil
}
