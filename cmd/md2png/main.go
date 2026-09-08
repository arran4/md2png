package main

import (
	"errors"
	"flag"
	"image/gif"
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
	out := flag.String("out", "out.png", "Output image file (.png, .jpg, or .gif)")
	cliOpts := md2png.RegisterFlags(flag.CommandLine)
	flag.Parse()

	var data []byte
	var baseDir string
	var err error
	if *in == "" {
		data, err = io.ReadAll(os.Stdin)
		if err == nil {
			baseDir, err = os.Getwd()
		}
	} else {
		var f *os.File
		f, err = os.Open(*in)
		if err != nil {
			fatal(err)
		}
		defer func() { _ = f.Close() }()
		data, err = io.ReadAll(f)
		if err == nil {
			baseDir, err = filepath.Abs(filepath.Dir(*in))
		}
	}
	if err != nil {
		fatal(err)
	}

	opts, err := cliOpts.ToRenderOptions(flag.CommandLine, baseDir)
	if err != nil {
		fatal(err)
	}

	img, err := md2png.Render(data, opts)
	if err != nil {
		fatal(err)
	}

	file, err := os.Create(*out)
	if err != nil {
		fatal(err)
	}
	defer func() { _ = file.Close() }()

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
	case ".gif":
		if err := gif.Encode(file, img, nil); err != nil {
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
