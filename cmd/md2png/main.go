package main

import (
	"errors"
	"flag"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/arran4/md2png"
)

func main() {
	in := flag.String("in", "", "Input Markdown file (default: stdin if empty)")
	out := flag.String("out", "out.png", "Output image file (.png or .jpg)")
	width := flag.Int("width", 1024, "Output image width in pixels")
	margin := flag.Int("margin", 48, "Margin in pixels")
	pt := flag.Float64("pt", 16, "Base font size in points (paragraph)")
	theme := flag.String("theme", "light", "Theme: light|dark")
	fontRegular := flag.String("font", "", "Path to TTF for regular text (optional; default Go Regular)")
	fontBold := flag.String("fontbold", "", "Path to TTF for bold text (optional; default Go Bold)")
	fontMono := flag.String("fontmono", "", "Path to TTF for mono/code (optional; default Go Mono)")
	flag.Parse()

	th, err := md2png.ThemeByName(*theme)
	if err != nil {
		fatal(err)
	}

	fonts, err := md2png.LoadFonts(md2png.FontConfig{
		RegularPath: *fontRegular,
		BoldPath:    *fontBold,
		MonoPath:    *fontMono,
		SizeBase:    *pt,
	})
	if err != nil {
		fatal(err)
	}

	var data []byte
	if *in == "" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		var f *os.File
		f, err = os.Open(*in)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		data, err = io.ReadAll(f)
	}
	if err != nil {
		fatal(err)
	}

	img, err := md2png.Render(data, md2png.RenderOptions{
		Width:        *width,
		Margin:       *margin,
		BaseFontSize: *pt,
		Theme:        th,
		Fonts:        fonts,
	})
	if err != nil {
		fatal(err)
	}

	file, err := os.Create(*out)
	if err != nil {
		fatal(err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(*out))
	switch ext {
	case ".png":
		if err := png.Encode(file, img); err != nil {
			fatal(err)
		}
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 92}); err != nil {
			fatal(err)
		}
	default:
		fatal(errors.New("unsupported output extension: " + ext))
	}
}

func fatal(err error) {
	_, _ = os.Stderr.WriteString("md2png: " + err.Error() + "\n")
	os.Exit(1)
}
