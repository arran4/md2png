package md2png

import (
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"image/draw"
)

// Md2view is a subcommand md2view that provides a GUI viewer for Markdown
func Md2view(
	in *string, // flag: --in Input Markdown file (default: stdin if empty)
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
	args ...string,
) error {
	var err error
	var data []byte
	var baseDir string

	inPath := ""
	if in != nil {
		inPath = *in
	}

	if inPath == "" && len(args) > 0 {
		inPath = args[0]
	}

	if inPath == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err = io.ReadAll(os.Stdin)
			if err == nil {
				baseDir, err = os.Getwd()
			}
		} else {
			return fmt.Errorf("no input file specified and no data piped to stdin")
		}
	} else {
		var f *os.File
		f, err = os.Open(inPath)
		if err != nil {
			return err
		}
		data, err = io.ReadAll(f)
		_ = f.Close()
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

	driver.Main(func(s screen.Screen) {
		w, err := s.NewWindow(&screen.NewWindowOptions{
			Title:  "md2view",
			Width:  img.Bounds().Dx(),
			Height: 800,
		})
		if err != nil {
			log.Fatalf("new window: %v", err)
		}
		defer w.Release()

		var b screen.Buffer
		defer func() {
			if b != nil {
				b.Release()
			}
		}()

		var sz size.Event
		offset := image.Point{}
		zoom := 1.0

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case size.Event:
				sz = e
				if b != nil {
					b.Release()
				}
				b, err = s.NewBuffer(sz.Size())
				if err != nil {
					log.Fatal(err)
				}
				w.Send(paint.Event{})
			case paint.Event:
				if sz.Bounds().Empty() || b == nil {
					continue
				}

				// Fill background
				draw.Draw(b.RGBA(), b.RGBA().Bounds(), image.NewUniform(opts.Theme.BG), image.Point{}, draw.Src)

				// Calculate scaled dimensions
				scaledW := int(float64(img.Bounds().Dx()) * zoom)
				scaledH := int(float64(img.Bounds().Dy()) * zoom)

				// Calculate position based on offset
				dr := image.Rect(offset.X, offset.Y, offset.X+scaledW, offset.Y+scaledH)

				// Draw the scaled image onto the buffer
				xdraw.ApproxBiLinear.Scale(b.RGBA(), dr, img, img.Bounds(), draw.Src, nil)

				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Direction == mouse.DirStep {
					if e.Button == mouse.ButtonWheelUp {
						offset.Y += 40
						w.Send(paint.Event{})
					} else if e.Button == mouse.ButtonWheelDown {
						offset.Y -= 40
						w.Send(paint.Event{})
					}
				}
			case key.Event:
				if e.Direction == key.DirPress {
					switch e.Code {
					case key.CodeEscape:
						return
					case key.CodeDownArrow:
						offset.Y -= 40
						w.Send(paint.Event{})
					case key.CodeUpArrow:
						offset.Y += 40
						w.Send(paint.Event{})
					case key.CodeLeftArrow:
						offset.X += 40
						w.Send(paint.Event{})
					case key.CodeRightArrow:
						offset.X -= 40
						w.Send(paint.Event{})
					case key.CodeEqualSign: // '+'
						zoom *= 1.1
						w.Send(paint.Event{})
					case key.CodeHyphenMinus: // '-'
						zoom /= 1.1
						w.Send(paint.Event{})
					case key.Code0: // reset zoom and offset
						zoom = 1.0
						offset = image.Point{}
						w.Send(paint.Event{})
					}
				}
			case error:
				log.Print(e)
			}
		}
	})
	return nil
}
