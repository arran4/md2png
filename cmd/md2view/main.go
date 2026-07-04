package main

import (
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/arran4/md2png"
	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	xdraw "golang.org/x/image/draw"
	"image/draw"
)

func main() {
	in := flag.String("in", "", "Input Markdown file (default: stdin if empty)")
	width := flag.Int("width", 1024, "Output image width in pixels")
	margin := flag.Int("margin", 48, "Margin in pixels")
	pt := flag.Float64("pt", 16, "Base font size in points (paragraph)")
	theme := flag.String("theme", "light", "Theme: light|dark")
	fontRegular := flag.String("font", "", "Path to TTF for regular text (optional; default Go Regular)")
	fontBold := flag.String("fontbold", "", "Path to TTF for bold text (optional; default Go Bold)")
	fontMono := flag.String("fontmono", "", "Path to TTF for mono/code (optional; default Go Mono)")
	footnoteLinks := flag.Bool("footnote-links", true, "Add footnotes for link destinations")
	footnoteImages := flag.Bool("footnote-images", false, "Add footnotes for image destinations")
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
	var baseDir string
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
		data, err = io.ReadAll(f)
		_ = f.Close()
		if err == nil {
			baseDir, err = filepath.Abs(filepath.Dir(*in))
		}
	}
	if err != nil {
		fatal(err)
	}

	img, err := md2png.Render(data, md2png.RenderOptions{
		Width:          *width,
		Margin:         *margin,
		BaseFontSize:   *pt,
		Theme:          th,
		Fonts:          fonts,
		LinkFootnotes:  footnoteLinks,
		ImageFootnotes: footnoteImages,
		BaseDir:        baseDir,
	})
	if err != nil {
		fatal(err)
	}

	driver.Main(func(s screen.Screen) {
		w, err := s.NewWindow(&screen.NewWindowOptions{
			Title:  "MD Viewer",
			Width:  800,
			Height: 600,
		})
		if err != nil {
			log.Fatal(err)
		}
		defer w.Release()

		var sz size.Event
		var b screen.Buffer
		defer func() {
			if b != nil {
				b.Release()
			}
		}()

		zoom := 1.0
		offset := image.Point{}
		var moveStart image.Point
		var moveOffset image.Point
		moving := false

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
					b = nil
				}
				if !sz.Bounds().Empty() {
					var err error
					b, err = s.NewBuffer(sz.Size())
					if err != nil {
						log.Fatal(err)
					}
				}
				w.Send(paint.Event{})
			case paint.Event:
				if sz.Bounds().Empty() || b == nil {
					continue
				}

				// Fill background
				draw.Draw(b.RGBA(), b.RGBA().Bounds(), image.NewUniform(th.BG), image.Point{}, draw.Src)

				// Calculate scaled dimensions
				scaledW := int(float64(img.Bounds().Dx()) * zoom)
				scaledH := int(float64(img.Bounds().Dy()) * zoom)

				// Calculate position based on offset
				dr := image.Rect(offset.X, offset.Y, offset.X+scaledW, offset.Y+scaledH)

				// Draw scaled image directly to buffer
				xdraw.ApproxBiLinear.Scale(b.RGBA(), dr, img, img.Bounds(), draw.Over, nil)

				// Upload to screen
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				switch e.Button {
				case mouse.ButtonLeft:
					switch e.Direction {
					case mouse.DirPress:
						moving = true
						moveStart = image.Point{int(e.X), int(e.Y)}
						moveOffset = offset
					case mouse.DirRelease:
						moving = false
					}
				case mouse.ButtonWheelUp:
					zoom *= 1.1
					w.Send(paint.Event{})
				case mouse.ButtonWheelDown:
					zoom /= 1.1
					w.Send(paint.Event{})
				}

				if moving && e.Direction == mouse.DirNone {
					dx := int(e.X) - moveStart.X
					dy := int(e.Y) - moveStart.Y
					offset = moveOffset.Add(image.Point{dx, dy})
					w.Send(paint.Event{})
				}
			case key.Event:
				if e.Direction == key.DirPress {
					switch e.Code {
					case key.CodeEscape, key.CodeQ:
						return
					case key.CodeEqualSign: // +
						zoom *= 1.1
						w.Send(paint.Event{})
					case key.CodeHyphenMinus: // -
						zoom /= 1.1
						w.Send(paint.Event{})
					case key.CodeLeftArrow:
						offset.X -= 20
						w.Send(paint.Event{})
					case key.CodeRightArrow:
						offset.X += 20
						w.Send(paint.Event{})
					case key.CodeUpArrow:
						offset.Y -= 20
						w.Send(paint.Event{})
					case key.CodeDownArrow:
						offset.Y += 20
						w.Send(paint.Event{})
					}
				}
			}
		}
	})
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "md2view: %v\n", err)
	os.Exit(1)
}
