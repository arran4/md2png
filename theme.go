package md2png

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// ParseTheme parses a CSS-lite theme from a string.
func ParseTheme(input string) (Theme, error) {
	return ParseThemeBytes([]byte(input))
}

// LoadThemeFile reads and parses a CSS-lite theme from a file.
func LoadThemeFile(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, fmt.Errorf("failed to read theme file %s: %w", path, err)
	}
	th, err := ParseThemeBytes(data)
	if err != nil {
		return Theme{}, fmt.Errorf("%s: %w", path, err)
	}
	return th, nil
}

// ParseThemeBytes parses a CSS-lite theme from a byte slice.
func ParseThemeBytes(data []byte) (Theme, error) {
	p := newThemeParser(string(data))
	return p.parse()
}

type themeParser struct {
	input string
	pos   int
	line  int
	col   int
}

func newThemeParser(input string) *themeParser {
	return &themeParser{
		input: input,
		pos:   0,
		line:  1,
		col:   1,
	}
}

func (p *themeParser) errorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("line %d, col %d: %s", p.line, p.col, msg)
}

func (p *themeParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *themeParser) next() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	ch := p.input[p.pos]
	p.pos++
	if ch == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	return ch
}

func (p *themeParser) skipWhitespaceAndComments() error {
	for p.pos < len(p.input) {
		ch := p.peek()
		if unicode.IsSpace(rune(ch)) {
			p.next()
			continue
		}
		if ch == '/' && p.pos+1 < len(p.input) {
			nextCh := p.input[p.pos+1]
			if nextCh == '/' {
				// Single line comment
				p.next()
				p.next()
				for p.pos < len(p.input) && p.peek() != '\n' {
					p.next()
				}
				continue
			} else if nextCh == '*' {
				// Block comment
				p.next()
				p.next()
				closed := false
				for p.pos+1 < len(p.input) {
					if p.input[p.pos] == '*' && p.input[p.pos+1] == '/' {
						p.next()
						p.next()
						closed = true
						break
					}
					p.next()
				}
				if !closed {
					return p.errorf("unclosed block comment")
				}
				continue
			}
		}
		break
	}
	return nil
}

func (p *themeParser) parse() (Theme, error) {
	// Base theme defaults to light
	baseTheme := lightTheme
	themeName := "custom"

	for {
		if err := p.skipWhitespaceAndComments(); err != nil {
			return Theme{}, err
		}
		if p.pos >= len(p.input) {
			break
		}

		if p.peek() == '@' {
			directive, err := p.parseDirective()
			if err != nil {
				return Theme{}, err
			}
			switch directive.name {
			case "theme":
				themeName = directive.themeName
				if directive.extends != "" {
					parent, err := ThemeByName(directive.extends)
					if err != nil {
						return Theme{}, p.errorf("unknown parent theme %q to extend", directive.extends)
					}
					baseTheme = parent
				}
			case "version":
				if directive.version != "1" {
					return Theme{}, p.errorf("unsupported theme format version %q (only version 1 is supported)", directive.version)
				}
			default:
				return Theme{}, p.errorf("unknown directive @%s", directive.name)
			}
			continue
		}

		// Parse element block: element { prop: val; ... }
		elemName, err := p.parseIdentifier()
		if err != nil {
			return Theme{}, err
		}

		if err := p.skipWhitespaceAndComments(); err != nil {
			return Theme{}, err
		}

		if p.next() != '{' {
			return Theme{}, p.errorf("expected '{' after element %q", elemName)
		}

		props, err := p.parseDeclarations()
		if err != nil {
			return Theme{}, err
		}

		if err := applyElementProperties(&baseTheme, elemName, props, p); err != nil {
			return Theme{}, err
		}
	}

	_ = themeName
	return baseTheme, nil
}

type directiveInfo struct {
	name      string
	themeName string
	extends   string
	version   string
}

func (p *themeParser) parseDirective() (directiveInfo, error) {
	p.next() // consume '@'
	name, err := p.parseIdentifier()
	if err != nil {
		return directiveInfo{}, err
	}

	info := directiveInfo{name: name}
	if name == "version" {
		if err := p.skipWhitespaceAndComments(); err != nil {
			return directiveInfo{}, err
		}
		ver, err := p.parseIdentifier()
		if err != nil {
			return directiveInfo{}, err
		}
		info.version = ver
		if err := p.skipWhitespaceAndComments(); err != nil {
			return directiveInfo{}, err
		}
		if p.next() != ';' {
			return directiveInfo{}, p.errorf("expected ';' after @version directive")
		}
		return info, nil
	}
	if name == "theme" {
		if err := p.skipWhitespaceAndComments(); err != nil {
			return directiveInfo{}, err
		}
		tName, err := p.parseIdentifier()
		if err != nil {
			return directiveInfo{}, err
		}
		info.themeName = tName

		if err := p.skipWhitespaceAndComments(); err != nil {
			return directiveInfo{}, err
		}

		if p.peek() != ';' {
			word, err := p.parseIdentifier()
			if err != nil {
				return directiveInfo{}, err
			}
			if word == "extends" {
				if err := p.skipWhitespaceAndComments(); err != nil {
					return directiveInfo{}, err
				}
				parent, err := p.parseIdentifier()
				if err != nil {
					return directiveInfo{}, err
				}
				info.extends = parent
			} else {
				return directiveInfo{}, p.errorf("expected 'extends' or ';' after theme name, got %q", word)
			}
		}

		if err := p.skipWhitespaceAndComments(); err != nil {
			return directiveInfo{}, err
		}
		if p.next() != ';' {
			return directiveInfo{}, p.errorf("expected ';' after @theme directive")
		}
	}

	return info, nil
}

func (p *themeParser) parseIdentifier() (string, error) {
	if err := p.skipWhitespaceAndComments(); err != nil {
		return "", err
	}
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.peek()
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '-' || ch == '_' {
			p.next()
		} else {
			break
		}
	}
	if p.pos == start {
		return "", p.errorf("expected identifier, found %q", string(p.peek()))
	}
	return p.input[start:p.pos], nil
}

func (p *themeParser) parseDeclarations() (map[string]string, error) {
	props := make(map[string]string)
	for {
		if err := p.skipWhitespaceAndComments(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.input) {
			return nil, p.errorf("unclosed block, expected '}'")
		}
		if p.peek() == '}' {
			p.next() // consume '}'
			break
		}

		propName, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}

		if err := p.skipWhitespaceAndComments(); err != nil {
			return nil, err
		}
		if p.next() != ':' {
			return nil, p.errorf("expected ':' after property %q", propName)
		}

		val, err := p.parsePropertyValue()
		if err != nil {
			return nil, err
		}
		props[strings.ToLower(propName)] = val
	}
	return props, nil
}

func (p *themeParser) parsePropertyValue() (string, error) {
	if err := p.skipWhitespaceAndComments(); err != nil {
		return "", err
	}
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.peek()
		if ch == ';' || ch == '}' {
			break
		}
		p.next()
	}
	val := strings.TrimSpace(p.input[start:p.pos])
	if val == "" {
		return "", p.errorf("empty property value")
	}
	if p.peek() == ';' {
		p.next() // consume ';'
	}
	return val, nil
}

func applyElementProperties(th *Theme, elem string, props map[string]string, p *themeParser) error {
	elem = strings.ToLower(elem)
	for prop, rawVal := range props {
		col, err := parseColor(rawVal)
		if err != nil {
			return p.errorf("invalid color %q for %s.%s: %v", rawVal, elem, prop, err)
		}

		switch elem {
		case "document", "body", "root":
			switch prop {
			case "color", "foreground", "fg":
				th.FG = col
			case "background", "bg", "background-color":
				th.BG = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "link", "a":
			switch prop {
			case "color", "fg":
				th.Link = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "code-block", "code", "pre":
			switch prop {
			case "background", "bg", "background-color":
				th.CodeBG = col
			case "color", "fg":
				th.CodeFG = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "blockquote", "quote":
			switch prop {
			case "border-color", "bar-color", "color":
				th.QuoteBar = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "hr", "rule", "thematic-break":
			switch prop {
			case "color", "border-color", "background":
				th.HRule = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "warning", "error":
			switch prop {
			case "color", "fg":
				th.Warning = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "table":
			switch prop {
			case "border-color", "border":
				th.TableBorder = col
			case "header-background", "header-bg":
				th.TableHeader = col
			default:
				return p.errorf("unknown property %q for element %s", prop, elem)
			}
		case "syntax", "syntax-highlighting":
			switch prop {
			case "keyword":
				th.Syntax.Keyword = col
			case "string":
				th.Syntax.String = col
			case "comment":
				th.Syntax.Comment = col
			case "type":
				th.Syntax.Type = col
			case "number":
				th.Syntax.Number = col
			case "builtin":
				th.Syntax.Builtin = col
			case "punctuation":
				th.Syntax.Punctuation = col
			default:
				return p.errorf("unknown syntax token property %q", prop)
			}
		default:
			return p.errorf("unknown element %q in theme", elem)
		}
	}
	return nil
}

// parseColor parses CSS color expressions:
// - Hex: #rgb, #rgba, #rrggbb, #rrggbbaa
// - rgb(...) and rgba(...) in comma or CSS4 slash syntax: rgb(65 200 145 / 90%)
// - hsl(...) and hsla(...)
// - named colors including transparent
func parseColor(s string) (color.Color, error) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	if lower == "transparent" {
		return color.RGBA{0, 0, 0, 0}, nil
	}

	if named, ok := namedColors[lower]; ok {
		return named, nil
	}

	if strings.HasPrefix(s, "#") {
		return parseHexColor(s)
	}

	if strings.HasPrefix(lower, "rgb(") || strings.HasPrefix(lower, "rgba(") {
		return parseRGBColor(s)
	}

	if strings.HasPrefix(lower, "hsl(") || strings.HasPrefix(lower, "hsla(") {
		return parseHSLColor(s)
	}

	return nil, fmt.Errorf("unrecognized color format %q", s)
}

func parseHexColor(s string) (color.Color, error) {
	hex := strings.TrimPrefix(s, "#")
	switch len(hex) {
	case 3: // #rgb
		r, err := parseHexDigit(hex[0])
		if err != nil {
			return nil, err
		}
		g, err := parseHexDigit(hex[1])
		if err != nil {
			return nil, err
		}
		b, err := parseHexDigit(hex[2])
		if err != nil {
			return nil, err
		}
		return color.RGBA{R: r * 17, G: g * 17, B: b * 17, A: 255}, nil
	case 4: // #rgba
		r, err := parseHexDigit(hex[0])
		if err != nil {
			return nil, err
		}
		g, err := parseHexDigit(hex[1])
		if err != nil {
			return nil, err
		}
		b, err := parseHexDigit(hex[2])
		if err != nil {
			return nil, err
		}
		a, err := parseHexDigit(hex[3])
		if err != nil {
			return nil, err
		}
		return color.RGBA{R: r * 17, G: g * 17, B: b * 17, A: a * 17}, nil
	case 6: // #rrggbb
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return nil, err
		}
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
	case 8: // #rrggbbaa
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return nil, err
		}
		return color.RGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
	default:
		return nil, fmt.Errorf("invalid hex color length #%s", hex)
	}
}

func parseHexDigit(c byte) (uint8, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex digit %c", c)
	}
}

func parseRGBColor(s string) (color.Color, error) {
	// Strip rgb( or rgba( and ending )
	start := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if start == -1 || end == -1 || end <= start {
		return nil, errors.New("malformed rgb expression")
	}
	body := strings.TrimSpace(s[start+1 : end])

	var parts []string
	if strings.Contains(body, ",") {
		raw := strings.Split(body, ",")
		for _, p := range raw {
			parts = append(parts, strings.TrimSpace(p))
		}
	} else {
		// CSS4 slash syntax: r g b / a
		slashSplit := strings.Split(body, "/")
		colorPart := strings.Fields(slashSplit[0])
		parts = append(parts, colorPart...)
		if len(slashSplit) > 1 {
			parts = append(parts, strings.TrimSpace(slashSplit[1]))
		}
	}

	if len(parts) != 3 && len(parts) != 4 {
		return nil, fmt.Errorf("rgb expects 3 or 4 components, got %d", len(parts))
	}

	r, err := parseColorComponent(parts[0], 255)
	if err != nil {
		return nil, fmt.Errorf("invalid red: %w", err)
	}
	g, err := parseColorComponent(parts[1], 255)
	if err != nil {
		return nil, fmt.Errorf("invalid green: %w", err)
	}
	b, err := parseColorComponent(parts[2], 255)
	if err != nil {
		return nil, fmt.Errorf("invalid blue: %w", err)
	}

	a := uint8(255)
	if len(parts) == 4 {
		a, err = parseAlphaComponent(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid alpha: %w", err)
		}
	}

	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}

func parseColorComponent(s string, max float64) (uint8, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, err
		}
		val := math.Round((pct / 100.0) * max)
		return clampUint8(val), nil
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return clampUint8(math.Round(val)), nil
}

func parseAlphaComponent(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, err
		}
		return clampUint8(math.Round((pct / 100.0) * 255)), nil
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if val <= 1.0 {
		return clampUint8(math.Round(val * 255)), nil
	}
	return clampUint8(math.Round(val)), nil
}

func clampUint8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func parseHSLColor(s string) (color.Color, error) {
	start := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if start == -1 || end == -1 || end <= start {
		return nil, errors.New("malformed hsl expression")
	}
	body := strings.TrimSpace(s[start+1 : end])

	var parts []string
	if strings.Contains(body, ",") {
		raw := strings.Split(body, ",")
		for _, p := range raw {
			parts = append(parts, strings.TrimSpace(p))
		}
	} else {
		slashSplit := strings.Split(body, "/")
		colorPart := strings.Fields(slashSplit[0])
		parts = append(parts, colorPart...)
		if len(slashSplit) > 1 {
			parts = append(parts, strings.TrimSpace(slashSplit[1]))
		}
	}

	if len(parts) != 3 && len(parts) != 4 {
		return nil, fmt.Errorf("hsl expects 3 or 4 components, got %d", len(parts))
	}

	hStr := strings.TrimSuffix(strings.TrimSpace(parts[0]), "deg")
	h, err := strconv.ParseFloat(hStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid hue: %w", err)
	}

	sStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), "%")
	sat, err := strconv.ParseFloat(sStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid saturation: %w", err)
	}

	lStr := strings.TrimSuffix(strings.TrimSpace(parts[2]), "%")
	lit, err := strconv.ParseFloat(lStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid lightness: %w", err)
	}

	r, g, b := hslToRGB(h, sat/100.0, lit/100.0)

	a := uint8(255)
	if len(parts) == 4 {
		a, err = parseAlphaComponent(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid alpha: %w", err)
		}
	}

	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	// Normalize h to [0, 360)
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}

	return clampUint8(math.Round((r1 + m) * 255)),
		clampUint8(math.Round((g1 + m) * 255)),
		clampUint8(math.Round((b1 + m) * 255))
}

var namedColors = map[string]color.RGBA{
	"white":       {0xFF, 0xFF, 0xFF, 0xFF},
	"black":       {0x00, 0x00, 0x00, 0xFF},
	"gray":        {0x80, 0x80, 0x80, 0xFF},
	"grey":        {0x80, 0x80, 0x80, 0xFF},
	"lightgray":   {0xD3, 0xD3, 0xD3, 0xFF},
	"lightgrey":   {0xD3, 0xD3, 0xD3, 0xFF},
	"darkgray":    {0xA9, 0xA9, 0xA9, 0xFF},
	"darkgrey":    {0xA9, 0xA9, 0xA9, 0xFF},
	"red":         {0xFF, 0x00, 0x00, 0xFF},
	"green":       {0x00, 0x80, 0x00, 0xFF},
	"blue":        {0x00, 0x00, 0xFF, 0xFF},
	"yellow":      {0xFF, 0xFF, 0x00, 0xFF},
	"cyan":        {0x00, 0xFF, 0xFF, 0xFF},
	"magenta":     {0xFF, 0x00, 0xFF, 0xFF},
	"orange":      {0xFF, 0xA5, 0x00, 0xFF},
	"purple":      {0x80, 0x00, 0x80, 0xFF},
	"navy":        {0x00, 0x00, 0x80, 0xFF},
	"teal":        {0x00, 0x80, 0x80, 0xFF},
	"maroon":      {0x80, 0x00, 0x00, 0xFF},
	"silver":      {0xC0, 0xC0, 0xC0, 0xFF},
	"transparent": {0x00, 0x00, 0x00, 0x00},
}
