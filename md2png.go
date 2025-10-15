package md2png

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionAST "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
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
	linkColor    = color.RGBA{0x06, 0x4F, 0xBD, 0xFF}
	warningColor = color.RGBA{0xD9, 0x51, 0x2C, 0xFF}
)

// ---- Font loading ----

type FontAndFace struct {
	Font     *truetype.Font
	Face     font.Face
	baseSize float64
}

type Fonts struct {
	Regular *FontAndFace
	Bold    *FontAndFace
	Mono    *FontAndFace
}

type FontConfig struct {
	RegularPath string
	BoldPath    string
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
		Font:     ft,
		Face:     face,
		baseSize: size,
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
	// Bold
	if cfg.BoldPath != "" {
		b, e := os.ReadFile(cfg.BoldPath)
		if e != nil {
			return f, e
		}
		f.Bold, err = loadFontAndFace(b, cfg.SizeBase)
		if err != nil {
			return f, err
		}
	} else {
		f.Bold, err = loadFontAndFace(gobold.TTF, cfg.SizeBase)
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
		if measureWidth(fnt, size, candidate) <= maxWidth {
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

func measureWidth(fnt *FontAndFace, size float64, s string) float64 {
	if fnt == nil || s == "" {
		return 0
	}
	// freetype.Context lacks a direct width measurement; approximate using font.Drawer
	var d font.Drawer
	d.Face = fnt.Face
	// d.Dot fixed point ignores size; face was created at size
	d.Src = image.NewUniform(color.Black)
	d.Dst = nil
	width := float64(d.MeasureString(s).Round())
	base := fnt.baseSize
	if base <= 0 {
		base = size
	}
	if base <= 0 {
		base = 1
	}
	if size <= 0 {
		size = base
	}
	if size <= 0 {
		size = 1
	}
	if size != base {
		width *= size / base
	}
	return width
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

func scaleImageToWidth(img image.Image, maxWidth int) image.Image {
	if img == nil {
		return nil
	}
	if maxWidth <= 0 {
		return img
	}
	bounds := img.Bounds()
	if bounds.Dx() <= maxWidth {
		return img
	}
	scale := float64(maxWidth) / float64(bounds.Dx())
	height := int(float64(bounds.Dy()) * scale)
	if height <= 0 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)
	return dst
}

func wrapLines(ff *FontAndFace, size float64, text string, maxWidth float64) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		ln := scanner.Text()
		if ln == "" {
			lines = append(lines, "")
			continue
		}
		if maxWidth <= 0 || measureWidth(ff, size, ln) <= maxWidth {
			lines = append(lines, ln)
			continue
		}
		wrapped := wrapLinePreservingSpaces(ff, size, ln, maxWidth)
		lines = append(lines, wrapped...)
	}
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	return lines
}

func wrapLinePreservingSpaces(ff *FontAndFace, size float64, line string, maxWidth float64) []string {
	if line == "" {
		return []string{""}
	}
	tokens := splitTextPreserveSpaces(line)
	var result []string
	var current strings.Builder
	var currentWidth float64

	flush := func() {
		result = append(result, current.String())
		current.Reset()
		currentWidth = 0
	}

	for _, token := range tokens {
		if token == "" {
			continue
		}
		tokenWidth := measureWidth(ff, size, token)
		if tokenWidth > maxWidth {
			if current.Len() > 0 {
				flush()
			}
			result = append(result, breakLongToken(ff, size, token, maxWidth)...)
			continue
		}
		if currentWidth+tokenWidth > maxWidth && current.Len() > 0 {
			flush()
		}
		current.WriteString(token)
		currentWidth += tokenWidth
	}
	if current.Len() > 0 {
		flush()
	}
	if len(result) == 0 {
		result = append(result, "")
	}
	return result
}

func breakLongToken(ff *FontAndFace, size float64, token string, maxWidth float64) []string {
	var parts []string
	var current strings.Builder
	var width float64
	for _, r := range token {
		ch := string(r)
		charWidth := measureWidth(ff, size, ch)
		if width+charWidth > maxWidth && current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
			width = 0
		}
		current.WriteString(ch)
		width += charWidth
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 {
		parts = append(parts, token)
	}
	return parts
}

// ---- Markdown -> draw ----

type renderer struct {
	c              *canvas
	baseSize       float64
	linkFootnotes  bool
	imageFootnotes bool
	footnoteIndex  map[string]int
	footnotes      []string
	baseDir        string
	imageCache     map[string]image.Image
	imageResolvers map[string]imageResolver
	httpClient     *http.Client
}

type imageResolver func(dest string) (cacheKey string, loader func() (image.Image, error), err error)

const (
	listIndentStep  = 32
	listMarkerWidth = 28
	listMarkerGap   = 8
)

type textToken struct {
	text      string
	font      *FontAndFace
	size      float64
	color     color.Color
	underline bool
	newline   bool
	image     image.Image
	center    bool
}

func (r *renderer) ensureFootnote(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	if r.footnoteIndex == nil {
		r.footnoteIndex = make(map[string]int)
	}
	if idx, ok := r.footnoteIndex[raw]; ok {
		return idx
	}
	idx := len(r.footnotes) + 1
	r.footnoteIndex[raw] = idx
	r.footnotes = append(r.footnotes, raw)
	return idx
}

func (r *renderer) appendFootnoteMarker(out *[]textToken, size float64, index int) {
	if out == nil || index <= 0 {
		return
	}
	markerSize := size * 0.75
	if markerSize <= 0 {
		markerSize = r.baseSize * 0.75
	}
	if markerSize <= 0 {
		markerSize = r.baseSize
	}
	*out = append(*out, textToken{
		text:  fmt.Sprintf("[%d]", index),
		font:  r.c.fonts.Regular,
		size:  markerSize,
		color: r.c.th.FG,
	})
}

func (r *renderer) ensureImageResolvers() {
	if r.httpClient == nil {
		r.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if r.imageResolvers != nil {
		return
	}
	r.imageResolvers = map[string]imageResolver{
		"":      r.resolveLocalImage,
		"file":  r.resolveLocalImage,
		"http":  r.resolveRemoteImage,
		"https": r.resolveRemoteImage,
	}
}

func (r *renderer) loadImage(dest string) (image.Image, error) {
	if strings.TrimSpace(dest) == "" {
		return nil, errors.New("md2png: empty image destination")
	}
	r.ensureImageResolvers()
	dest = strings.TrimSpace(dest)
	scheme := ""
	if idx := strings.Index(dest, "://"); idx != -1 {
		scheme = strings.ToLower(dest[:idx])
	}
	resolver, ok := r.imageResolvers[scheme]
	if !ok {
		if scheme != "" {
			return nil, fmt.Errorf("md2png: unsupported image scheme: %s", scheme)
		}
		return nil, fmt.Errorf("md2png: unsupported image destination: %s", dest)
	}
	cacheKey, loader, err := resolver(dest)
	if err != nil {
		return nil, err
	}
	if cacheKey == "" {
		cacheKey = dest
	}
	if r.imageCache != nil {
		if img, ok := r.imageCache[cacheKey]; ok {
			return img, nil
		}
	}
	if loader == nil {
		return nil, fmt.Errorf("md2png: resolver for %q returned nil loader", dest)
	}
	img, err := loader()
	if err != nil {
		return nil, err
	}
	if r.imageCache == nil {
		r.imageCache = make(map[string]image.Image)
	}
	r.imageCache[cacheKey] = img
	return img, nil
}

func (r *renderer) resolveLocalImage(dest string) (string, func() (image.Image, error), error) {
	path := strings.TrimSpace(dest)
	if strings.HasPrefix(path, "file://") {
		path = strings.TrimPrefix(path, "file://")
	}
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(r.baseDir)
		if base != "" {
			path = filepath.Join(base, path)
		}
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		if abs, err := filepath.Abs(cleaned); err == nil {
			cleaned = abs
		}
	}
	loader := func() (image.Image, error) {
		f, err := os.Open(cleaned)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return nil, err
		}
		return img, nil
	}
	return cleaned, loader, nil
}

func (r *renderer) resolveRemoteImage(dest string) (string, func() (image.Image, error), error) {
	url := strings.TrimSpace(dest)
	loader := func() (image.Image, error) {
		client := r.httpClient
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("md2png: fetching image %s: %s", url, resp.Status)
		}
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return nil, err
		}
		return img, nil
	}
	return url, loader, nil
}

func (r *renderer) collectInlineTokens(node ast.Node, md []byte, font *FontAndFace, size float64, color color.Color, out *[]textToken) {
	if font == nil {
		font = r.c.fonts.Regular
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			text := string(c.Segment.Value(md))
			if text != "" {
				parts := strings.Split(text, "\n")
				for i, part := range parts {
					if part != "" {
						*out = append(*out, textToken{text: part, font: font, size: size, color: color})
					}
					if i < len(parts)-1 {
						*out = append(*out, textToken{newline: true})
					}
				}
			}
			if c.SoftLineBreak() || c.HardLineBreak() {
				*out = append(*out, textToken{newline: true})
			}
		case *ast.Link:
			before := len(*out)
			r.collectInlineTokens(c, md, font, size, linkColor, out)
			for i := before; i < len(*out); i++ {
				(*out)[i].color = linkColor
				(*out)[i].underline = true
			}
			if r.linkFootnotes {
				idx := r.ensureFootnote(string(c.Destination))
				r.appendFootnoteMarker(out, size, idx)
			}
		case *ast.AutoLink:
			label := string(c.Label(md))
			if label == "" {
				label = string(c.URL(md))
			}
			if label != "" {
				*out = append(*out, textToken{text: label, font: font, size: size, color: linkColor, underline: true})
			}
			if r.linkFootnotes {
				idx := r.ensureFootnote(string(c.URL(md)))
				r.appendFootnoteMarker(out, size, idx)
			}
		case *ast.Image:
			dest := strings.TrimSpace(string(c.Destination))
			alt := strings.TrimSpace(string(c.Text(md)))
			if alt == "" {
				alt = strings.TrimSpace(string(c.Title))
			}
			if img, err := r.loadImage(dest); err == nil {
				*out = append(*out, textToken{image: img, center: true})
			} else {
				fallback := alt
				fallbackColor := r.c.th.FG
				if fallback == "" {
					fallback = dest
					fallbackColor = warningColor
				}
				if fallback != "" {
					*out = append(*out, textToken{text: fallback, font: font, size: size, color: fallbackColor})
				}
			}
			if r.imageFootnotes {
				idx := r.ensureFootnote(dest)
				r.appendFootnoteMarker(out, size, idx)
			}
		case *ast.Paragraph:
			r.collectInlineTokens(c, md, font, size, color, out)
			if child.NextSibling() != nil {
				*out = append(*out, textToken{newline: true})
			}
		case *ast.Emphasis:
			nextFont := font
			if c.Level >= 2 && r.c.fonts.Bold != nil {
				nextFont = r.c.fonts.Bold
			}
			r.collectInlineTokens(c, md, nextFont, size, color, out)
		case *ast.CodeSpan:
			mono := r.c.fonts.Mono
			if mono == nil {
				mono = font
			}
			txt := string(c.Text(md))
			if txt != "" {
				*out = append(*out, textToken{text: txt, font: mono, size: size * 0.95, color: color})
			}
		default:
			if child.HasChildren() {
				r.collectInlineTokens(child, md, font, size, color, out)
			}
		}
	}
}

type styledWord struct {
	text      string
	font      *FontAndFace
	size      float64
	color     color.Color
	underline bool
}

func splitTextPreserveSpaces(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	var current strings.Builder
	lastType := 0 // 0 unknown, 1 space, 2 non-space
	for _, r := range s {
		typ := 2
		if unicode.IsSpace(r) {
			typ = 1
		}
		if lastType == 0 {
			current.WriteRune(r)
			lastType = typ
			continue
		}
		if typ == lastType {
			current.WriteRune(r)
			continue
		}
		parts = append(parts, current.String())
		current.Reset()
		current.WriteRune(r)
		lastType = typ
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func (r *renderer) markerPositions(level int) (markerLeft, markerRight, contentLeft int) {
	markerLeft = r.c.margin + level*listIndentStep
	markerRight = markerLeft + listMarkerWidth
	contentLeft = markerRight + listMarkerGap
	return
}

func (r *renderer) drawListMarker(marker string, baseline int, markerLeft, markerRight int) {
	font := r.c.fonts.Regular
	if font == nil {
		return
	}
	r.c.setFace(font, r.c.th.FG, r.baseSize)
	width := measureWidth(font, r.baseSize, marker)
	x := markerRight - int(width)
	if x < markerLeft {
		x = markerLeft
	}
	pt := freetype.Pt(x, baseline)
	_, _ = r.c.dc.DrawString(marker, pt)
}

type lineMetric struct {
	baseline int
	height   int
}

func (c *canvas) drawTokens(tokens []textToken, left, right int) []lineMetric {
	if len(tokens) == 0 {
		return nil
	}
	maxWidth := float64(right - left)
	var line []styledWord
	var lineWidth float64
	var lineMaxSize float64
	var metrics []lineMetric

	flush := func(force bool) {
		if len(line) == 0 {
			if force {
				heightSize := lineMaxSize
				if heightSize == 0 {
					heightSize = c.ptSize
				}
				height := int(heightSize * 1.4)
				if height == 0 {
					height = int(c.ptSize * 1.4)
				}
				c.cursorY += height
			}
			return
		}
		baselineSize := lineMaxSize
		if baselineSize == 0 {
			baselineSize = c.ptSize
		}
		baseline := c.cursorY + int(baselineSize)
		x := left
		for _, w := range line {
			if w.font == nil {
				w.font = c.fonts.Regular
			}
			c.setFace(w.font, w.color, w.size)
			pt := freetype.Pt(x, baseline)
			_, _ = c.dc.DrawString(w.text, pt)
			width := int(measureWidth(w.font, w.size, w.text))
			if w.underline && width > 0 {
				underlineY := baseline + int(w.size*0.12)
				if underlineY <= baseline {
					underlineY = baseline + 1
				}
				rect := image.Rect(x, underlineY, x+width, underlineY+1)
				draw.Draw(c.img, rect, image.NewUniform(w.color), image.Point{}, draw.Src)
			}
			x += width
		}
		lineHeight := int(baselineSize * 1.4)
		if lineHeight <= 0 {
			lineHeight = int(c.ptSize * 1.4)
		}
		metrics = append(metrics, lineMetric{baseline: baseline, height: lineHeight})
		c.cursorY += lineHeight
		line = line[:0]
		lineWidth = 0
		lineMaxSize = 0
	}

	for _, tok := range tokens {
		if tok.newline {
			flush(true)
			continue
		}
		if tok.image != nil {
			flush(false)
			maxWidthInt := int(maxWidth)
			img := tok.image
			if b := img.Bounds(); maxWidthInt > 0 && b.Dx() > maxWidthInt {
				img = scaleImageToWidth(img, maxWidthInt)
			}
			bounds := img.Bounds()
			startY := c.cursorY
			drawWidth := bounds.Dx()
			drawHeight := bounds.Dy()
			x := left
			if tok.center && maxWidthInt > drawWidth {
				x += (maxWidthInt - drawWidth) / 2
			}
			rect := image.Rect(x, startY, x+drawWidth, startY+drawHeight)
			draw.Draw(c.img, rect, img, bounds.Min, draw.Over)
			baseline := startY + int(c.ptSize)
			if baseline > rect.Max.Y {
				baseline = rect.Max.Y
			}
			if baseline <= startY {
				baseline = startY + drawHeight
			}
			metrics = append(metrics, lineMetric{baseline: baseline, height: drawHeight})
			c.cursorY += drawHeight
			c.cursorY += int(c.ptSize * 0.6)
			continue
		}
		font := tok.font
		if font == nil {
			font = c.fonts.Regular
		}
		segments := splitTextPreserveSpaces(tok.text)
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			isSpace := unicode.IsSpace([]rune(seg)[0])
			segWidth := measureWidth(font, tok.size, seg)
			if isSpace {
				if len(line) == 0 {
					continue
				}
				line = append(line, styledWord{text: seg, font: font, size: tok.size, color: tok.color, underline: tok.underline})
				lineWidth += segWidth
				continue
			}
			if lineWidth+segWidth > maxWidth && len(line) > 0 {
				flush(false)
			}
			line = append(line, styledWord{text: seg, font: font, size: tok.size, color: tok.color, underline: tok.underline})
			if tok.size > lineMaxSize {
				lineMaxSize = tok.size
			}
			lineWidth += segWidth
		}
	}
	flush(false)
	return metrics
}

func (r *renderer) renderList(list *ast.List, md []byte, level int) {
	markerLeft, markerRight, contentLeft := r.markerPositions(level)
	itemSpacing := int(r.baseSize * 0.6)
	start := list.Start
	if !list.IsOrdered() || start == 0 {
		start = 1
	}
	index := 0
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		marker := "•"
		if list.IsOrdered() {
			marker = fmt.Sprintf("%d%c", start+index, list.Marker)
		}
		r.renderListItem(li, md, level, marker, markerLeft, markerRight, contentLeft)
		if item.NextSibling() != nil {
			r.c.addVSpace(itemSpacing)
		}
		index++
	}
	r.c.addVSpace(int(r.baseSize * 0.7))
}

func (r *renderer) renderListItem(li *ast.ListItem, md []byte, level int, marker string, markerLeft, markerRight, contentLeft int) {
	startY := r.c.cursorY
	markerDrawn := false
	blockSpacing := int(r.baseSize * 0.5)

	ensureMarker := func(baseline int) {
		if markerDrawn {
			return
		}
		r.drawListMarker(marker, baseline, markerLeft, markerRight)
		markerDrawn = true
	}

	inlineBlock := func(node ast.Node) {
		var tokens []textToken
		r.collectInlineTokens(node, md, r.c.fonts.Regular, r.baseSize, r.c.th.FG, &tokens)
		if len(tokens) == 0 {
			text := strings.TrimRight(string(node.Text(md)), "\n")
			if text == "" {
				return
			}
			tokens = []textToken{{text: text, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG}}
		}
		metrics := r.c.drawTokens(tokens, contentLeft, r.c.w-r.c.margin)
		if len(metrics) > 0 {
			ensureMarker(metrics[0].baseline)
		} else {
			ensureMarker(startY + int(r.baseSize))
		}
		if node.NextSibling() != nil {
			r.c.addVSpace(blockSpacing)
		}
	}

	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Paragraph:
			inlineBlock(c)
		case *ast.TextBlock:
			inlineBlock(c)
		case *ast.List:
			ensureMarker(startY + int(r.baseSize))
			r.c.addVSpace(int(r.baseSize * 0.3))
			r.renderList(c, md, level+1)
		case *ast.CodeBlock:
			ensureMarker(startY + int(r.baseSize))
			text := strings.TrimRight(string(c.Text(md)), "\n")
			r.c.addVSpace(int(r.baseSize * 0.2))
			r.c.drawCodeBlock(text, contentLeft, r.c.w-r.c.margin, r.baseSize*0.95)
			if child.NextSibling() != nil {
				r.c.addVSpace(blockSpacing)
			}
		case *ast.FencedCodeBlock:
			ensureMarker(startY + int(r.baseSize))
			text := strings.TrimRight(string(c.Text(md)), "\n")
			r.c.addVSpace(int(r.baseSize * 0.2))
			r.c.drawCodeBlock(text, contentLeft, r.c.w-r.c.margin, r.baseSize*0.95)
			if child.NextSibling() != nil {
				r.c.addVSpace(blockSpacing)
			}
		case *ast.Blockquote:
			ensureMarker(startY + int(r.baseSize))
			quoteStart := r.c.cursorY
			var tokens []textToken
			r.collectInlineTokens(c, md, r.c.fonts.Regular, r.baseSize, r.c.th.FG, &tokens)
			if len(tokens) > 0 {
				r.c.addVSpace(2)
				_ = r.c.drawTokens(tokens, contentLeft+10, r.c.w-r.c.margin)
				r.c.addVSpace(6)
				r.c.drawBlockquoteBar(quoteStart+2, r.c.cursorY-quoteStart-2)
			}
			if child.NextSibling() != nil {
				r.c.addVSpace(blockSpacing)
			}
		default:
			// Ignore unsupported inline nodes; block handlers cover known types.
		}
	}

	if !markerDrawn {
		ensureMarker(startY + int(r.baseSize))
	}
}

func (r *renderer) collectTableRow(row *extensionAST.TableRow, md []byte) [][]textToken {
	var cells [][]textToken
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if tc, ok := cell.(*extensionAST.TableCell); ok {
			var tokens []textToken
			r.collectInlineTokens(tc, md, r.c.fonts.Regular, r.baseSize, r.c.th.FG, &tokens)
			cells = append(cells, tokens)
		}
	}
	return cells
}

func (r *renderer) renderTable(tbl *extensionAST.Table, md []byte) {
	var rows [][][]textToken
	for node := tbl.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *extensionAST.TableHeader:
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if tr, ok := child.(*extensionAST.TableRow); ok {
					rows = append(rows, r.collectTableRow(tr, md))
				}
			}
		case *extensionAST.TableRow:
			rows = append(rows, r.collectTableRow(n, md))
		}
	}
	if len(rows) == 0 {
		return
	}
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if colCount == 0 {
		return
	}

	border := 1
	cellPadding := int(r.baseSize * 0.6)
	if cellPadding < 8 {
		cellPadding = 8
	}
	availableWidth := r.c.w - 2*r.c.margin
	minWidth := colCount*40 + border*(colCount+1)
	if availableWidth < minWidth {
		availableWidth = minWidth
	}
	colWidth := (availableWidth - border*(colCount+1)) / colCount
	if colWidth < 60 {
		colWidth = 60
	}
	tableWidth := colCount*colWidth + border*(colCount+1)
	if tableWidth > availableWidth {
		tableWidth = availableWidth
	}
	tableLeft := r.c.margin
	tableRight := tableLeft + tableWidth

	borderColor := image.NewUniform(r.c.th.HRule)
	r.c.addVSpace(int(r.baseSize * 0.3))
	tableTop := r.c.cursorY
	draw.Draw(r.c.img, image.Rect(tableLeft, tableTop, tableRight, tableTop+border), borderColor, image.Point{}, draw.Src)
	y := tableTop + border

	for _, row := range rows {
		rowTop := y
		maxCellHeight := 0
		for col := 0; col < colCount; col++ {
			cellLeft := tableLeft + border + col*(colWidth+border)
			cellRight := cellLeft + colWidth
			contentLeft := cellLeft + cellPadding
			contentRight := cellRight - cellPadding
			if contentRight <= contentLeft {
				contentRight = cellRight - 2
			}
			start := rowTop + cellPadding
			r.c.cursorY = start
			var tokens []textToken
			if col < len(row) {
				tokens = row[col]
			}
			metrics := r.c.drawTokens(tokens, contentLeft, contentRight)
			height := r.c.cursorY - start
			if len(metrics) == 0 && len(tokens) == 0 {
				height = int(r.baseSize * 1.1)
			}
			if height > maxCellHeight {
				maxCellHeight = height
			}
			r.c.cursorY = start
		}
		if maxCellHeight < int(r.baseSize*1.1) {
			maxCellHeight = int(r.baseSize * 1.1)
		}
		rowBottom := rowTop + maxCellHeight + 2*cellPadding
		draw.Draw(r.c.img, image.Rect(tableLeft, rowBottom, tableRight, rowBottom+border), borderColor, image.Point{}, draw.Src)
		y = rowBottom + border
	}

	tableBottom := y - border
	for col := 0; col <= colCount; col++ {
		x := tableLeft + col*(colWidth+border)
		draw.Draw(r.c.img, image.Rect(x, tableTop, x+border, tableBottom+border), borderColor, image.Point{}, draw.Src)
	}
	r.c.cursorY = tableBottom + int(r.baseSize*0.7)
}

func (r *renderer) renderUnsupported(node ast.Node) {
	if node.Type() != ast.TypeBlock {
		return
	}
	msg := fmt.Sprintf("⚠ Unsupported: %s", node.Kind().String())
	tokens := []textToken{{text: msg, font: r.c.fonts.Regular, size: r.baseSize * 0.9, color: warningColor}}
	_ = r.c.drawTokens(tokens, r.c.margin, r.c.w-r.c.margin)
	r.c.addVSpace(int(r.baseSize * 0.6))
}

func (r *renderer) drawFootnotes() {
	if len(r.footnotes) == 0 {
		return
	}
	r.c.addVSpace(int(r.baseSize * 0.4))
	noteSize := r.baseSize * 0.85
	if noteSize <= 0 {
		noteSize = r.baseSize
	}
	for i, note := range r.footnotes {
		label := fmt.Sprintf("[%d] %s", i+1, note)
		tokens := []textToken{{text: label, font: r.c.fonts.Regular, size: noteSize, color: r.c.th.FG}}
		_ = r.c.drawTokens(tokens, r.c.margin, r.c.w-r.c.margin)
	}
}

func (r *renderer) render(md []byte) error {
	mdParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	doc := mdParser.Parser().Parse(text.NewReader(md))
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
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
			var tokens []textToken
			r.collectInlineTokens(n, md, r.c.fonts.Regular, size, r.c.th.FG, &tokens)
			r.c.addVSpace(int(r.baseSize * 0.75))
			_ = r.c.drawTokens(tokens, r.c.margin, r.c.w-r.c.margin)
			r.c.addVSpace(int(r.baseSize * 0.5))
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			var tokens []textToken
			r.collectInlineTokens(n, md, r.c.fonts.Regular, r.baseSize, r.c.th.FG, &tokens)
			if len(tokens) > 0 {
				_ = r.c.drawTokens(tokens, r.c.margin, r.c.w-r.c.margin)
				r.c.addVSpace(int(r.baseSize * 0.9))
			}
			return ast.WalkSkipChildren, nil
		case *ast.List:
			r.renderList(nd, md, 0)
			return ast.WalkSkipChildren, nil
		case *extensionAST.Table:
			r.renderTable(nd, md)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			text := strings.TrimRight(string(n.Text(md)), "\n")
			r.c.addVSpace(4)
			r.c.drawCodeBlock(text, r.c.margin, r.c.w-r.c.margin, r.baseSize*0.95)
			return ast.WalkSkipChildren, nil
		case *ast.Blockquote:
			startY := r.c.cursorY
			var tokens []textToken
			r.collectInlineTokens(n, md, r.c.fonts.Regular, r.baseSize*1.0, r.c.th.FG, &tokens)
			if len(tokens) > 0 {
				r.c.addVSpace(2)
				_ = r.c.drawTokens(tokens, r.c.margin+10, r.c.w-r.c.margin)
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
			switch nd.(type) {
			case *ast.Document, *ast.ListItem:
				return ast.WalkContinue, nil
			case *extensionAST.TableHeader, *extensionAST.TableRow, *extensionAST.TableCell:
				return ast.WalkContinue, nil
			}
			r.renderUnsupported(nd)
			return ast.WalkSkipChildren, nil
		}
	}); err != nil {
		return err
	}
	r.drawFootnotes()
	return nil
}

// ---- Library entry points ----

// LightTheme and DarkTheme expose the built-in themes for convenience.
var (
	LightTheme = lightTheme
	DarkTheme  = darkTheme
)

// ThemeByName returns a built-in theme by name ("light" or "dark").
func ThemeByName(name string) (Theme, error) {
	switch strings.ToLower(name) {
	case "light", "":
		return lightTheme, nil
	case "dark":
		return darkTheme, nil
	default:
		return Theme{}, errors.New("unknown theme: " + name)
	}
}

// LoadFonts returns a Fonts set using the provided FontConfig. When no
// custom paths are supplied it falls back to Go's bundled fonts.
func LoadFonts(cfg FontConfig) (Fonts, error) {
	return loadFonts(cfg)
}

// RenderOptions configure how Markdown is rendered to an image.
type RenderOptions struct {
	Width          int
	Margin         int
	BaseFontSize   float64
	Theme          Theme
	Fonts          Fonts
	LinkFootnotes  *bool
	ImageFootnotes *bool
	BaseDir        string
}

// Render converts the provided Markdown document into a raster image using the
// supplied options. Zero values enable sensible defaults (1024px width,
// 48px margin, 16pt base font, light theme, bundled fonts).
func Render(data []byte, opts RenderOptions) (*image.RGBA, error) {
	if opts.Width <= 0 {
		opts.Width = 1024
	}
	if opts.Margin <= 0 {
		opts.Margin = 48
	}
	if opts.BaseFontSize <= 0 {
		opts.BaseFontSize = 16
	}
	if (opts.Theme == Theme{}) {
		opts.Theme = lightTheme
	}

	// Fill in missing fonts using the bundled defaults.
	if opts.Fonts.Regular == nil || opts.Fonts.Bold == nil || opts.Fonts.Mono == nil {
		fallback, err := LoadFonts(FontConfig{SizeBase: opts.BaseFontSize})
		if err != nil {
			return nil, err
		}
		if opts.Fonts.Regular == nil {
			opts.Fonts.Regular = fallback.Regular
		}
		if opts.Fonts.Bold == nil {
			opts.Fonts.Bold = fallback.Bold
		}
		if opts.Fonts.Mono == nil {
			opts.Fonts.Mono = fallback.Mono
		}
	}

	if opts.Fonts.Regular == nil || opts.Fonts.Bold == nil || opts.Fonts.Mono == nil {
		return nil, errors.New("md2png: incomplete font configuration")
	}

	linkFootnotes := true
	if opts.LinkFootnotes != nil {
		linkFootnotes = *opts.LinkFootnotes
	}
	imageFootnotes := false
	if opts.ImageFootnotes != nil {
		imageFootnotes = *opts.ImageFootnotes
	}

	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		if wd, err := os.Getwd(); err == nil {
			baseDir = wd
		}
	} else if !filepath.IsAbs(baseDir) {
		if abs, err := filepath.Abs(baseDir); err == nil {
			baseDir = abs
		}
	}

	c := newCanvas(opts.Width, opts.Margin, opts.Theme, opts.Fonts, opts.BaseFontSize)
	r := &renderer{
		c:              c,
		baseSize:       opts.BaseFontSize,
		linkFootnotes:  linkFootnotes,
		imageFootnotes: imageFootnotes,
		baseDir:        baseDir,
	}
	r.ensureImageResolvers()
	if err := r.render(data); err != nil {
		return nil, err
	}

	used := c.cursorY + opts.Margin
	if used < opts.Margin+50 {
		used = opts.Margin + 50
	}

	img := image.NewRGBA(image.Rect(0, 0, opts.Width, used))
	draw.Draw(img, img.Bounds(), c.img, image.Point{}, draw.Src)
	return img, nil
}
