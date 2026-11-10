package cli

import (
	"github.com/arran4/md2png"

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
	themeFile *string, // flag: --theme-file Path to CSS theme file
	transparent *bool, // flag: --transparent Explicit transparent page background
	fontRegular *string, // flag: --font Path to TTF for regular text (optional; default Go Regular)
	fontBold *string, // flag: --fontbold Path to TTF for bold text (optional; default Go Bold)
	fontMono *string, // flag: --fontmono Path to TTF for mono/code (optional; default Go Mono)
	fontItalic *string, // flag: --fontitalic Path to TTF for italic text (optional; default Go Italic)
	fontBoldItalic *string, // flag: --fontbolditalic Path to TTF for bold italic text (optional; default Go Bold Italic)
	fontFallback *string, // flag: --font-fallback Comma separated paths to fallback TTF/OTF fonts
	footnoteLinks *bool, // flag: --footnote-links Add footnotes for link destinations (default: true)
	footnoteImages *bool, // flag: --footnote-images Add footnotes for image destinations (default: false)
	maxHeight *int, // flag: --max-height Maximum output height in pixels (0 for default)
	tableFitWidth *bool, // flag: --table-fit-width Expand canvas width to fit tables
	disableHighlighting *bool, // flag: --disable-highlighting Disable syntax highlighting
	codeTabWidth *int, // flag: --code-tab-width Tab width in code blocks (default: 4)
	footnoteOrder *string, // flag: --footnote-order Ordering of authored vs link footnotes
	args ...string, // arg: [args]
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

	opts, err := ConvertCommandArgsToRenderOptions(
		nil, width, margin, pt, theme, themeFile, transparent,
		fontRegular, fontBold, fontMono, fontItalic, fontBoldItalic, fontFallback,
		footnoteLinks, footnoteImages, maxHeight, tableFitWidth, disableHighlighting, codeTabWidth,
		footnoteOrder,
		baseDir,
	)
	if err != nil {
		return err
	}

	img, err := md2png.Render(data, opts)
	if err != nil {
		return err
	}

	driver.Main(func(s screen.Screen) {
		winWidth := img.Bounds().Dx()
		if winWidth <= 0 {
			winWidth = 800
		}
		winHeight := 800
		w, err := s.NewWindow(&screen.NewWindowOptions{
			Title:  "md2view",
			Width:  winWidth,
			Height: winHeight,
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

		vp := NewViewport(img.Bounds().Size(), image.Point{X: winWidth, Y: winHeight})

		reloader := NewReloader(
			inPath,
			data,
			img,
			func(updatedData []byte) (image.Image, error) {
				return md2png.Render(updatedData, opts)
			},
			func(newImg image.Image) {
				vp.SetImageSize(newImg.Bounds().Size())
				w.Send(paint.Event{})
			},
		)
		reloader.Start()
		defer reloader.Stop()

		var sz size.Event

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
				vp.SetWindowSize(sz.Size())
				w.Send(paint.Event{})
			case paint.Event:
				if sz.Bounds().Empty() || b == nil {
					continue
				}

				// Fill background
				draw.Draw(b.RGBA(), b.RGBA().Bounds(), image.NewUniform(opts.Theme.BG), image.Point{}, draw.Src)

				currentImg := reloader.LastGoodImage()
				if currentImg != nil {
					dr := vp.DestRect()
					xdraw.ApproxBiLinear.Scale(b.RGBA(), dr, currentImg, currentImg.Bounds(), draw.Src, nil)
				}

				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Direction == mouse.DirStep {
					center := image.Point{X: int(e.X), Y: int(e.Y)}
					if e.Modifiers&key.ModControl != 0 {
						switch e.Button {
						case mouse.ButtonWheelUp:
							vp.ZoomAt(1.15, center)
							w.Send(paint.Event{})
						case mouse.ButtonWheelDown:
							vp.ZoomAt(1.0/1.15, center)
							w.Send(paint.Event{})
						}
					} else {
						switch e.Button {
						case mouse.ButtonWheelUp:
							vp.Pan(image.Point{Y: 40})
							w.Send(paint.Event{})
						case mouse.ButtonWheelDown:
							vp.Pan(image.Point{Y: -40})
							w.Send(paint.Event{})
						}
					}
				}
			case key.Event:
				if e.Direction == key.DirPress {
					center := image.Point{X: sz.WidthPx / 2, Y: sz.HeightPx / 2}
					switch e.Code {
					case key.CodeEscape, key.CodeQ:
						return
					case key.CodeDownArrow:
						vp.Pan(image.Point{Y: -40})
						w.Send(paint.Event{})
					case key.CodeUpArrow:
						vp.Pan(image.Point{Y: 40})
						w.Send(paint.Event{})
					case key.CodeLeftArrow:
						vp.Pan(image.Point{X: 40})
						w.Send(paint.Event{})
					case key.CodeRightArrow:
						vp.Pan(image.Point{X: -40})
						w.Send(paint.Event{})
					case key.CodeEqualSign: // '+'
						vp.ZoomAt(1.15, center)
						w.Send(paint.Event{})
					case key.CodeHyphenMinus: // '-'
						vp.ZoomAt(1.0/1.15, center)
						w.Send(paint.Event{})
					case key.Code0, key.CodeKeypad0: // reset zoom to 100%
						vp.Reset()
						w.Send(paint.Event{})
					case key.CodeW: // fit width
						vp.FitWidthMode()
						w.Send(paint.Event{})
					case key.CodeF: // fit window
						vp.FitWindowMode()
						w.Send(paint.Event{})
					case key.CodeC: // recenter
						vp.Recenter()
						w.Send(paint.Event{})
					case key.CodeT, key.CodeHome: // scroll to top
						vp.ScrollToTop()
						w.Send(paint.Event{})
					case key.CodeR: // manual reload
						_, _ = reloader.ReloadNow()
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
