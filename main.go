package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

// Simple, dependency-light Markdown -> raster image renderer.
// Goals:
//  - Pure Go binary (no JS, no Python)
//  - Reasonable rendering: headings, paragraphs, lists, code blocks, hr, blockquotes
//  - Word wrap by width; adjustable width, margins, fonts, theme
//  - Export PNG or JPG based on output extension
//
// Not a full HTML renderer; keep expectations practical.

// ---- Styles & theme ----

type Theme struct {
	BG       color.Color
	FG       color.Color
	CodeBG   color.Color
	QuoteBar color.Color
	HRule    color.Color
}

var (
	// Light theme defaults
	lightTheme = Theme{
		BG:       color.RGBA{0xFF, 0xFF, 0xFF, 0xFF},
		FG:       color.RGBA{0x11, 0x11, 0x11, 0xFF},
		CodeBG:   color.RGBA{0xF5, 0xF5, 0xF7, 0xFF},
		QuoteBar: color.RGBA{0xCC, 0xCC, 0xCC, 0xFF},
		HRule:    color.RGBA{0xDD, 0xDD, 0xDD, 0xFF},
	}
	// Dark theme defaults
	darkTheme = Theme{
		BG:       color.RGBA{0x12, 0x12, 0x14, 0xFF},
		FG:       color.RGBA{0xEE, 0xEE, 0xF0, 0xFF},
		CodeBG:   color.RGBA{0x1E, 0x1E, 0x22, 0xFF},
		QuoteBar: color.RGBA{0x44, 0x44, 0x48, 0xFF},
		HRule:    color.RGBA{0x33, 0x33, 0x36, 0xFF},
	}
)

// ---- Font loading ----

type FontAndFace struct {
	Font *truetype.Font
	Face font.Face
}

type Fonts struct {
	Regular *FontAndFace
	Mono    *FontAndFace
}

type FontConfig struct {
	RegularPath string
	MonoPath    string
	SizeBase    float64 // paragraph font size in pt
}

func loadFontAndFace(ttfBytes []byte, size float64) (*FontAndFace, error) {
	ft, err := truetype.Parse(ttfBytes)
	if err != nil {
		return nil, err
	}
	face := truetype.NewFace(ft, &truetype.Options{Size: size, DPI: 96, Hinting: font.HintingFull})
	return &FontAndFace{
		Font: ft,
		Face: face,
	}, err
}

func loadFonts(cfg FontConfig) (Fonts, error) {
	var f Fonts
	var err error

	// RegularFace
	if cfg.RegularPath != "" {
		b, e := os.ReadFile(cfg.RegularPath)
		if e != nil {
			return f, e
		}
		f.Regular, err = loadFontAndFace(b, cfg.SizeBase)
		if err != nil {
			return f, err
		}
	} else {
		f.Regular, err = loadFontAndFace(goregular.TTF, cfg.SizeBase)
		if err != nil {
			return f, err
		}
	}
	// Mono
	if cfg.MonoPath != "" {
		b, e := os.ReadFile(cfg.MonoPath)
		if e != nil {
			return f, e
		}
		f.Mono, err = loadFontAndFace(b, cfg.SizeBase)
		if err != nil {
			return f, err
		}
	} else {
		f.Mono, err = loadFontAndFace(gomono.TTF, cfg.SizeBase)
		if err != nil {
			return f, err
		}
	}
	return f, nil
}

// ---- Layout primitives ----

type canvas struct {
	img     *image.RGBA
	dc      *freetype.Context
	w, h    int
	margin  int
	cursorY int
	lineGap int // pixels between text lines
	th      Theme
	fonts   Fonts
	ptSize  float64
}

func newCanvas(width int, margin int, th Theme, fonts Fonts, ptSize float64) *canvas {
	// Start tall; we'll crop later
	img := image.NewRGBA(image.Rect(0, 0, width, 4096*2))
	dc := freetype.NewContext()
	dc.SetDPI(96)
	dc.SetClip(img.Bounds())
	dc.SetDst(img)
	dc.SetSrc(image.NewUniform(th.FG))
	dc.SetFont(nil)
	dc.SetFontSize(ptSize)

	// Fill BG
	draw.Draw(img, img.Bounds(), image.NewUniform(th.BG), image.Point{}, draw.Src)

	return &canvas{
		img:     img,
		dc:      dc,
		w:       width,
		h:       img.Bounds().Dy(),
		margin:  margin,
		cursorY: margin,
		lineGap: 4,
		th:      th,
		fonts:   fonts,
		ptSize:  ptSize,
	}
}

func (c *canvas) setFace(fnt *FontAndFace, color color.Color, size float64) {
	c.dc.SetFontSize(size)
	c.dc.SetSrc(image.NewUniform(color))
	c.dc.SetFont(fnt.Font)
}

func (c *canvas) drawTextWrapped(fnt *FontAndFace, col color.Color, size float64, text string, left int, right int) int {
	c.setFace(fnt, col, size)

	maxWidth := float64(right - left)
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0
	}
	// Build lines
	var lines []string
	var line string
	for _, w := range words {
		candidate := strings.TrimSpace(line + " " + w)
		if measureWidth(fnt, candidate) <= maxWidth {
			line = candidate
		} else {
			if line != "" {
				lines = append(lines, line)
			}
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}

	lineHeight := int(size * 1.4) // simple
	for _, ln := range lines {
		pt := freetype.Pt(left, c.cursorY+int(size))
		_, _ = c.dc.DrawString(ln, pt)
		c.cursorY += lineHeight
	}
	return len(lines)
}

func measureWidth(fnt *FontAndFace, s string) float64 {
	// freetype.Context lacks a direct width measurement; approximate using font.Drawer
	var d font.Drawer
	d.Face = fnt.Face
	// d.Dot fixed point ignores size; face was created at size
	d.Src = image.NewUniform(color.Black)
	d.Dst = nil
	return float64(d.MeasureString(s).Round())
}

func (c *canvas) addVSpace(px int) { c.cursorY += px }

func (c *canvas) drawHRule() {
	y := c.cursorY + 4
	rect := image.Rect(c.margin, y, c.w-c.margin, y+2)
	draw.Draw(c.img, rect, image.NewUniform(c.th.HRule), image.Point{}, draw.Src)
	c.cursorY = y + 10
}

func (c *canvas) drawBlockquoteBar(topY, height int) {
	x0 := c.margin
	rect := image.Rect(x0, topY, x0+4, topY+height)
	draw.Draw(c.img, rect, image.NewUniform(c.th.QuoteBar), image.Point{}, draw.Src)
}

func (c *canvas) drawCodeBlock(text string, left, right int, size float64) {
	pad := 10
	top := c.cursorY
	// measure height by counting wrapped lines
	mono := c.fonts.Mono
	lines := wrapLines(mono, size, text, float64(right-left-2*pad))
	lineHeight := int(size * 1.4)
	height := len(lines)*lineHeight + 2*pad + 6
	// bg
	rect := image.Rect(left, top, right, top+height)
	draw.Draw(c.img, rect, image.NewUniform(c.th.CodeBG), image.Point{}, draw.Src)

	// draw text
	c.setFace(mono, c.th.FG, size)
	y := top + pad + int(size)
	for _, ln := range lines {
		pt := freetype.Pt(left+pad, y)
		_, _ = c.dc.DrawString(ln, pt)
		y += lineHeight
	}
	c.cursorY = top + height + 6
}

func wrapLines(ff *FontAndFace, size float64, text string, maxWidth float64) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		ln := scanner.Text()
		if strings.TrimSpace(ln) == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(ln)
		var line string
		for _, w := range words {
			candidate := strings.TrimSpace(line + " " + w)
			if measureWidth(ff, candidate) <= maxWidth {
				line = candidate
			} else {
				if line != "" {
					lines = append(lines, line)
				}
				line = w
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// ---- Markdown -> draw ----

type renderer struct {
	c        *canvas
	baseSize float64
}

func (r *renderer) render(md []byte) error {
	mdParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	doc := mdParser.Parser().Parse(text.NewReader(md))
	return ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nd := n.(type) {
		case *ast.Heading:
			lvl := nd.Level
			var size float64
			switch lvl { // simple scale
			case 1:
				size = r.baseSize * 1.9
			case 2:
				size = r.baseSize * 1.6
			case 3:
				size = r.baseSize * 1.4
			case 4:
				size = r.baseSize * 1.25
			default:
				size = r.baseSize * 1.15
			}
			text := strings.TrimSpace(string(n.Text(md)))
			r.c.addVSpace(8)
			r.c.drawTextWrapped(r.c.fonts.Regular, r.c.th.FG, size, text, r.c.margin, r.c.w-r.c.margin)
			r.c.addVSpace(6)
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			text := strings.TrimSpace(string(n.Text(md)))
			if text != "" {
				r.c.drawTextWrapped(r.c.fonts.Regular, r.c.th.FG, r.baseSize, text, r.c.margin, r.c.w-r.c.margin)
				r.c.addVSpace(8)
			}
			return ast.WalkSkipChildren, nil
		case *ast.List:
			// Render each item with bullet/number
			for it := n.FirstChild(); it != nil; it = it.NextSibling() {
				if li, ok := it.(*ast.ListItem); ok {
					marker := "• "
					if nd.IsOrdered() {
						marker = "1. "
					} // naive numbering
					content := strings.TrimSpace(string(li.Text(md)))
					if content != "" {
						// draw marker
						r.c.drawTextWrapped(r.c.fonts.Regular, r.c.th.FG, r.baseSize, marker+content, r.c.margin, r.c.w-r.c.margin)
						r.c.addVSpace(4)
					}
				}
			}
			r.c.addVSpace(6)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			text := strings.TrimRight(string(n.Text(md)), "\n")
			r.c.addVSpace(4)
			r.c.drawCodeBlock(text, r.c.margin, r.c.w-r.c.margin, r.baseSize*0.95)
			return ast.WalkSkipChildren, nil
		case *ast.Blockquote:
			startY := r.c.cursorY
			inner := strings.TrimSpace(string(n.Text(md)))
			if inner != "" {
				r.c.addVSpace(2)
				r.c.drawTextWrapped(r.c.fonts.Regular, r.c.th.FG, r.baseSize*1.0, inner, r.c.margin+10, r.c.w-r.c.margin)
				r.c.addVSpace(6)
				r.c.drawBlockquoteBar(startY+2, r.c.cursorY-startY-2)
			}
			return ast.WalkSkipChildren, nil
		case *ast.ThematicBreak:
			r.c.drawHRule()
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			// Handled by parents (Paragraph/List/Heading)
			return ast.WalkContinue, nil
		default:
			return ast.WalkContinue, nil
		}
	})
}

// ---- I/O & main ----

func readAll(r io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

func chooseTheme(name string) (Theme, error) {
	switch strings.ToLower(name) {
	case "light", "":
		return lightTheme, nil
	case "dark":
		return darkTheme, nil
	default:
		return Theme{}, errors.New("unknown theme: " + name)
	}
}

func main() {
	in := flag.String("in", "", "Input Markdown file (default: stdin if empty)")
	out := flag.String("out", "out.png", "Output image file (.png or .jpg)")
	width := flag.Int("width", 1024, "Output image width in pixels")
	margin := flag.Int("margin", 48, "Margin in pixels")
	pt := flag.Float64("pt", 16, "Base font size in points (paragraph)")
	theme := flag.String("theme", "light", "Theme: light|dark")
	fontRegular := flag.String("font", "", "Path to TTF for regular text (optional; default Go RegularFace)")
	fontMono := flag.String("fontmono", "", "Path to TTF for mono/code (optional; default Go Mono)")
	flag.Parse()

	th, err := chooseTheme(*theme)
	if err != nil {
		fatal(err)
	}

	fc := FontConfig{RegularPath: *fontRegular, MonoPath: *fontMono, SizeBase: *pt}
	fonts, err := loadFonts(fc)
	if err != nil {
		fatal(err)
	}

	var data []byte
	if *in == "" {
		data, err = readAll(os.Stdin)
	} else {
		f, e := os.Open(*in)
		if e != nil {
			fatal(e)
		}
		defer f.Close()
		data, err = readAll(f)
	}
	if err != nil {
		fatal(err)
	}

	c := newCanvas(*width, *margin, th, fonts, *pt)
	r := &renderer{c: c, baseSize: *pt}
	if err := r.render(data); err != nil {
		fatal(err)
	}

	// Crop to used height
	used := c.cursorY + *margin
	if used < *margin+50 {
		used = *margin + 50
	}
	crop := image.NewRGBA(image.Rect(0, 0, *width, used))
	draw.Draw(crop, crop.Bounds(), c.img, image.Point{}, draw.Src)

	// Encode
	ext := strings.ToLower(filepath.Ext(*out))
	var w io.Writer = mustCreate(*out)
	defer func() { _ = w.(io.WriteCloser).Close() }()
	switch ext {
	case ".png":
		if err := png.Encode(w, crop); err != nil {
			fatal(err)
		}
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(w, crop, &jpeg.Options{Quality: 92}); err != nil {
			fatal(err)
		}
	default:
		fatal(errors.New("unsupported output extension: " + ext))
	}
}

func mustCreate(p string) io.WriteCloser {
	f, err := os.Create(p)
	if err != nil {
		fatal(err)
	}
	return f
}

func fatal(err error) {
	_, _ = os.Stderr.WriteString("md2img: " + err.Error() + "\n")
	os.Exit(1)
}
