package md2png

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strings"
	"unicode"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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
	Link     color.Color
	Warning  color.Color
}

var (
	// Light theme defaults
	lightTheme = Theme{
		BG:       color.RGBA{0xFF, 0xFF, 0xFF, 0xFF},
		FG:       color.RGBA{0x11, 0x11, 0x11, 0xFF},
		CodeBG:   color.RGBA{0xF5, 0xF5, 0xF7, 0xFF},
		QuoteBar: color.RGBA{0xCC, 0xCC, 0xCC, 0xFF},
		HRule:    color.RGBA{0xDD, 0xDD, 0xDD, 0xFF},
		Link:     color.RGBA{0x00, 0x55, 0xCC, 0xFF},
		Warning:  color.RGBA{0xCC, 0x55, 0x00, 0xFF},
	}
	// Dark theme defaults
	darkTheme = Theme{
		BG:       color.RGBA{0x12, 0x12, 0x14, 0xFF},
		FG:       color.RGBA{0xEE, 0xEE, 0xF0, 0xFF},
		CodeBG:   color.RGBA{0x1E, 0x1E, 0x22, 0xFF},
		QuoteBar: color.RGBA{0x44, 0x44, 0x48, 0xFF},
		HRule:    color.RGBA{0x33, 0x33, 0x36, 0xFF},
		Link:     color.RGBA{0x4A, 0x90, 0xE2, 0xFF},
		Warning:  color.RGBA{0xF2, 0x91, 0x5B, 0xFF},
	}
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
		tokens := splitTextPreserveSpaces(ln)
		var builder strings.Builder
		var width float64
		for _, tok := range tokens {
			if tok == "" {
				continue
			}
			w := measureWidth(ff, size, tok)
			if width+w > maxWidth && builder.Len() > 0 {
				lines = append(lines, builder.String())
				builder.Reset()
				width = 0
			}
			builder.WriteString(tok)
			width += w
		}
		lines = append(lines, builder.String())
	}
	return lines
}

// ---- Markdown -> draw ----

type renderer struct {
	c         *canvas
	baseSize  float64
	listStack []*listContext
	itemStack []*listItemContext
}

type textToken struct {
	text      string
	font      *FontAndFace
	size      float64
	color     color.Color
	newline   bool
	underline bool
}

type listContext struct {
	indent      int
	contentLeft int
	markerArea  int
	isOrdered   bool
	counter     int
	tight       bool
}

type listItemContext struct {
	marker string
	drawn  bool
}

func (r *renderer) collectInlineTokens(node ast.Node, md []byte, font *FontAndFace, size float64, color color.Color, underline bool, out *[]textToken) {
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
						*out = append(*out, textToken{text: part, font: font, size: size, color: color, underline: underline})
					}
					if i < len(parts)-1 {
						*out = append(*out, textToken{newline: true})
					}
				}
			}
			if c.SoftLineBreak() || c.HardLineBreak() {
				*out = append(*out, textToken{newline: true})
			}
		case *ast.Paragraph:
			r.collectInlineTokens(c, md, font, size, color, underline, out)
			if child.NextSibling() != nil {
				*out = append(*out, textToken{newline: true})
			}
		case *ast.Emphasis:
			nextFont := font
			if c.Level >= 2 && r.c.fonts.Bold != nil {
				nextFont = r.c.fonts.Bold
			}
			r.collectInlineTokens(c, md, nextFont, size, color, underline, out)
		case *ast.Link:
			r.collectInlineTokens(c, md, font, size, r.c.th.Link, true, out)
		case *ast.CodeSpan:
			mono := r.c.fonts.Mono
			if mono == nil {
				mono = font
			}
			txt := string(c.Text(md))
			if txt != "" {
				*out = append(*out, textToken{text: txt, font: mono, size: size * 0.95, color: color, underline: underline})
			}
		default:
			if child.HasChildren() {
				r.collectInlineTokens(child, md, font, size, color, underline, out)
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

func (c *canvas) drawTokens(tokens []textToken, left, right int) {
	if len(tokens) == 0 {
		return
	}
	maxWidth := float64(right - left)
	var line []styledWord
	var lineWidth float64
	var lineMaxSize float64

	flush := func(force bool) {
		if len(line) == 0 {
			if force {
				heightSize := lineMaxSize
				if heightSize == 0 {
					heightSize = c.ptSize
				}
				height := int(heightSize*1.5 + 0.5)
				if height <= 0 {
					height = int(c.ptSize*1.5 + 0.5)
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
			advance := int(measureWidth(w.font, w.size, w.text))
			if w.underline && strings.TrimSpace(w.text) != "" {
				underlineY := baseline + int(w.size*0.2)
				rect := image.Rect(x, underlineY, x+advance, underlineY+1)
				draw.Draw(c.img, rect, image.NewUniform(w.color), image.Point{}, draw.Src)
			}
			x += advance
		}
		lineHeight := int(baselineSize*1.5 + 0.5)
		if lineHeight <= 0 {
			lineHeight = int(c.ptSize*1.5 + 0.5)
		}
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
				if tok.size > lineMaxSize {
					lineMaxSize = tok.size
				}
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
}

func (r *renderer) currentListContext() *listContext {
	if len(r.listStack) == 0 {
		return nil
	}
	return r.listStack[len(r.listStack)-1]
}

func (r *renderer) currentItemContext() *listItemContext {
	if len(r.itemStack) == 0 {
		return nil
	}
	return r.itemStack[len(r.itemStack)-1]
}

func (r *renderer) pushListContext(list *ast.List) {
	level := len(r.listStack)
	indentStep := int(r.baseSize * 1.8)
	markerArea := int(r.baseSize * 2.8)
	indent := r.c.margin + level*indentStep
	ctx := &listContext{
		indent:      indent,
		contentLeft: indent + markerArea,
		markerArea:  markerArea,
		isOrdered:   list.IsOrdered(),
		counter:     list.Start,
		tight:       list.IsTight,
	}
	if ctx.counter <= 0 {
		ctx.counter = 1
	}
	r.listStack = append(r.listStack, ctx)
	r.c.addVSpace(int(r.baseSize * 0.3))
}

func (r *renderer) popListContext() {
	if len(r.listStack) == 0 {
		return
	}
	r.listStack = r.listStack[:len(r.listStack)-1]
	r.c.addVSpace(int(r.baseSize * 0.5))
}

func (r *renderer) beginListItem() {
	ctx := r.currentListContext()
	if ctx == nil {
		return
	}
	marker := "•"
	if ctx.isOrdered {
		marker = fmt.Sprintf("%d.", ctx.counter)
		ctx.counter++
	}
	r.itemStack = append(r.itemStack, &listItemContext{marker: marker})
}

func (r *renderer) endListItem() {
	if len(r.itemStack) == 0 {
		return
	}
	ctx := r.currentListContext()
	item := r.currentItemContext()
	if ctx != nil && item != nil && !item.drawn {
		r.drawListMarker(item.marker, ctx)
		r.c.cursorY += int(r.baseSize * 1.2)
	}
	r.itemStack = r.itemStack[:len(r.itemStack)-1]
	spacing := int(r.baseSize * 0.75)
	if ctx != nil && ctx.tight {
		spacing = int(r.baseSize * 0.45)
	}
	r.c.addVSpace(spacing)
}

func (r *renderer) drawListMarker(marker string, ctx *listContext) {
	if ctx == nil {
		return
	}
	start := r.c.cursorY
	tokens := []textToken{{text: marker, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG}}
	r.c.drawTokens(tokens, ctx.indent, ctx.indent+ctx.markerArea)
	r.c.cursorY = start
}

func (r *renderer) ensureListMarker() {
	ctx := r.currentListContext()
	item := r.currentItemContext()
	if ctx == nil || item == nil || item.drawn {
		return
	}
	r.drawListMarker(item.marker, ctx)
	item.drawn = true
}

func (r *renderer) renderUnsupportedNode(kind string, left int) {
	tokens := []textToken{{text: "⚠ unsupported: " + kind, font: r.c.fonts.Regular, size: r.baseSize * 0.9, color: r.c.th.Warning}}
	r.c.drawTokens(tokens, left, r.c.w-r.c.margin)
	r.c.addVSpace(int(r.baseSize))
}

func (r *renderer) render(md []byte) error {
	mdParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	doc := mdParser.Parser().Parse(text.NewReader(md))
	return ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n.(type) {
		case *ast.List:
			if entering {
				r.pushListContext(n.(*ast.List))
			} else {
				r.popListContext()
			}
			return ast.WalkContinue, nil
		case *ast.ListItem:
			if entering {
				r.beginListItem()
			} else {
				r.endListItem()
			}
			return ast.WalkContinue, nil
		}

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
			left := r.c.margin
			if ctx := r.currentListContext(); ctx != nil {
				item := r.currentItemContext()
				if item != nil && !item.drawn {
					gap := strings.Repeat(" ", 6)
					tokens = append(tokens, textToken{text: item.marker, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG})
					tokens = append(tokens, textToken{text: gap, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG})
					left = ctx.indent
					item.drawn = true
				} else if ctx != nil {
					left = ctx.contentLeft
				}
			}
			r.collectInlineTokens(n, md, r.c.fonts.Regular, size, r.c.th.FG, false, &tokens)
			r.c.addVSpace(int(r.baseSize * 0.6))
			r.c.drawTokens(tokens, left, r.c.w-r.c.margin)
			r.c.addVSpace(int(r.baseSize * 0.75))
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			var tokens []textToken
			left := r.c.margin
			if ctx := r.currentListContext(); ctx != nil {
				item := r.currentItemContext()
				if item != nil && !item.drawn {
					gap := strings.Repeat(" ", 6)
					tokens = append(tokens, textToken{text: item.marker, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG})
					tokens = append(tokens, textToken{text: gap, font: r.c.fonts.Regular, size: r.baseSize, color: r.c.th.FG})
					left = ctx.indent
					item.drawn = true
				} else {
					left = ctx.contentLeft
				}
			}
			r.collectInlineTokens(n, md, r.c.fonts.Regular, r.baseSize, r.c.th.FG, false, &tokens)
			if len(tokens) > 0 {
				r.c.drawTokens(tokens, left, r.c.w-r.c.margin)
				if r.currentListContext() != nil {
					r.c.addVSpace(int(r.baseSize * 0.6))
				} else {
					r.c.addVSpace(int(r.baseSize * 1.5))
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			text := string(n.Text(md))
			left := r.c.margin
			if ctx := r.currentListContext(); ctx != nil {
				r.ensureListMarker()
				left = ctx.contentLeft
			}
			r.c.addVSpace(4)
			r.c.drawCodeBlock(text, left, r.c.w-r.c.margin, r.baseSize*0.95)
			return ast.WalkSkipChildren, nil
		case *ast.Blockquote:
			startY := r.c.cursorY
			var tokens []textToken
			left := r.c.margin
			if ctx := r.currentListContext(); ctx != nil {
				r.ensureListMarker()
				left = ctx.contentLeft
			}
			r.collectInlineTokens(n, md, r.c.fonts.Regular, r.baseSize*1.0, r.c.th.FG, false, &tokens)
			if len(tokens) > 0 {
				r.c.addVSpace(2)
				r.c.drawTokens(tokens, left+10, r.c.w-r.c.margin)
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
		case *extast.Table:
			r.renderTable(nd, md)
			return ast.WalkSkipChildren, nil
		default:
			if n.Type() == ast.TypeInline {
				return ast.WalkContinue, nil
			}
			if n.Kind().String() == "Document" {
				return ast.WalkContinue, nil
			}
			left := r.c.margin
			if ctx := r.currentListContext(); ctx != nil {
				r.ensureListMarker()
				left = ctx.contentLeft
			}
			r.renderUnsupportedNode(n.Kind().String(), left)
			return ast.WalkSkipChildren, nil
		}
	})
}

func (r *renderer) renderTable(tbl *extast.Table, md []byte) {
	left := r.c.margin
	if ctx := r.currentListContext(); ctx != nil {
		r.ensureListMarker()
		left = ctx.contentLeft
	}

	header, rows := r.extractTableRows(tbl, md)
	columnCount := len(header)
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return
	}

	widths := make([]int, columnCount)
	for i := 0; i < columnCount; i++ {
		if i < len(header) {
			if w := runeCount(header[i]); w > widths[i] {
				widths[i] = w
			}
		}
		for _, row := range rows {
			if i < len(row) {
				if w := runeCount(row[i]); w > widths[i] {
					widths[i] = w
				}
			}
		}
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	mono := r.c.fonts.Mono
	if mono == nil {
		mono = r.c.fonts.Regular
	}

	r.c.addVSpace(int(r.baseSize * 0.6))
	if len(header) > 0 {
		r.drawTableLine(header, widths, left, mono, true)
		r.drawTableSeparator(widths, left, mono)
	}
	for _, row := range rows {
		r.drawTableLine(row, widths, left, mono, false)
	}
	r.c.addVSpace(int(r.baseSize * 0.8))
}

func (r *renderer) drawTableLine(cells []string, widths []int, left int, font *FontAndFace, bold bool) {
	var builder strings.Builder
	builder.WriteString("| ")
	for i, width := range widths {
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		builder.WriteString(cell)
		pad := width - runeCount(cell)
		if pad > 0 {
			builder.WriteString(strings.Repeat(" ", pad))
		}
		builder.WriteString(" | ")
	}
	tokens := []textToken{{text: builder.String(), font: font, size: r.baseSize * 0.95, color: r.c.th.FG}}
	if bold && r.c.fonts.Bold != nil {
		tokens[0].font = r.c.fonts.Bold
	}
	r.c.drawTokens(tokens, left, r.c.w-r.c.margin)
}

func (r *renderer) drawTableSeparator(widths []int, left int, font *FontAndFace) {
	var builder strings.Builder
	builder.WriteString("| ")
	for _, width := range widths {
		builder.WriteString(strings.Repeat("-", width))
		builder.WriteString(" | ")
	}
	tokens := []textToken{{text: builder.String(), font: font, size: r.baseSize * 0.9, color: r.c.th.FG}}
	r.c.drawTokens(tokens, left, r.c.w-r.c.margin)
}

func (r *renderer) extractTableRows(tbl *extast.Table, md []byte) ([]string, [][]string) {
	var header []string
	var rows [][]string
	for child := tbl.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *extast.TableHeader:
			header = r.extractTableRow(c, md, true)
		case *extast.TableRow:
			rows = append(rows, r.extractTableRow(c, md, false))
		}
	}
	return header, rows
}

func (r *renderer) extractTableRow(node ast.Node, md []byte, header bool) []string {
	var cells []string
	for cell := node.FirstChild(); cell != nil; cell = cell.NextSibling() {
		tokens := r.tableCellTokens(cell, md, header)
		cells = append(cells, tokensToString(tokens))
	}
	return cells
}

func (r *renderer) tableCellTokens(node ast.Node, md []byte, header bool) []textToken {
	font := r.c.fonts.Regular
	if header && r.c.fonts.Bold != nil {
		font = r.c.fonts.Bold
	}
	var tokens []textToken
	r.collectInlineTokens(node, md, font, r.baseSize, r.c.th.FG, false, &tokens)
	return tokens
}

func tokensToString(tokens []textToken) string {
	var builder strings.Builder
	for _, tok := range tokens {
		if tok.newline {
			if builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			continue
		}
		builder.WriteString(tok.text)
	}
	return strings.TrimSpace(builder.String())
}

func runeCount(s string) int {
	return len([]rune(s))
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
	Width        int
	Margin       int
	BaseFontSize float64
	Theme        Theme
	Fonts        Fonts
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

	c := newCanvas(opts.Width, opts.Margin, opts.Theme, opts.Fonts, opts.BaseFontSize)
	r := &renderer{c: c, baseSize: opts.BaseFontSize}
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
