package md2png

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTheme_Basic(t *testing.T) {
	input := `
/* Forest theme extending dark */
@theme forest extends dark;

document {
  color: #edf7f1;
  background: #0f1c15;
}

link {
  color: rgb(65 200 145 / 90%);
}

code-block {
  background: #19342acc;
  color: #d1f0e2;
}

blockquote {
  border-color: #2e5945;
}

hr {
  border-color: #244234;
}

warning {
  color: #ff9966;
}

table {
  border-color: #244234;
  header-background: #19342a;
}

syntax {
  keyword: #4ade80;
  string: #86efac;
  comment: #4b7a60;
  number: #a7f3d0;
}
`

	th, err := ParseTheme(input)
	if err != nil {
		t.Fatalf("unexpected ParseTheme error: %v", err)
	}

	// Check document
	r, g, b, a := th.FG.RGBA()
	if uint8(r>>8) != 0xed || uint8(g>>8) != 0xf7 || uint8(b>>8) != 0xf1 || uint8(a>>8) != 0xff {
		t.Errorf("expected FG #edf7f1, got rgba(%d, %d, %d, %d)", r>>8, g>>8, b>>8, a>>8)
	}

	// Check link with alpha
	lr, lg, lb, la := th.Link.RGBA()
	if uint8(lr>>8) != 65 || uint8(lg>>8) != 200 || uint8(lb>>8) != 145 {
		t.Errorf("expected Link rgb(65 200 145), got rgba(%d, %d, %d, %d)", lr>>8, lg>>8, lb>>8, la>>8)
	}
	// 90% of 255 = ~230
	if uint8(la>>8) != 230 {
		t.Errorf("expected Link alpha ~230, got %d", la>>8)
	}

	// Check syntax overrides and inherited properties
	kr, kg, kb, _ := th.Syntax.Keyword.RGBA()
	if uint8(kr>>8) != 0x4a || uint8(kg>>8) != 0xde || uint8(kb>>8) != 0x80 {
		t.Errorf("expected Keyword #4ade80, got %d, %d, %d", kr>>8, kg>>8, kb>>8)
	}

	// Inherited builtin from darkTheme
	darkBuiltinR, darkBuiltinG, darkBuiltinB, _ := darkTheme.Syntax.Builtin.RGBA()
	br, bg, bb, _ := th.Syntax.Builtin.RGBA()
	if br != darkBuiltinR || bg != darkBuiltinG || bb != darkBuiltinB {
		t.Errorf("expected inherited builtin syntax color from dark theme")
	}
}

func TestParseTheme_ColorFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected color.RGBA
	}{
		{"hex 3", "document { color: #f0a; }", color.RGBA{0xff, 0x00, 0xaa, 0xff}},
		{"hex 4", "document { color: #f0a8; }", color.RGBA{0xff, 0x00, 0xaa, 0x88}},
		{"hex 6", "document { color: #123456; }", color.RGBA{0x12, 0x34, 0x56, 0xff}},
		{"hex 8", "document { color: #12345678; }", color.RGBA{0x12, 0x34, 0x56, 0x78}},
		{"rgb comma", "document { color: rgb(10, 20, 30); }", color.RGBA{10, 20, 30, 255}},
		{"rgba comma float", "document { color: rgba(10, 20, 30, 0.5); }", color.RGBA{10, 20, 30, 128}},
		{"rgba comma pct", "document { color: rgba(100%, 0%, 50%, 50%); }", color.RGBA{255, 0, 128, 128}},
		{"rgb space slash", "document { color: rgb(100 150 200 / 0.8); }", color.RGBA{100, 150, 200, 204}},
		{"hsl simple", "document { color: hsl(0, 100%, 50%); }", color.RGBA{255, 0, 0, 255}},
		{"hsl green", "document { color: hsl(120, 100%, 50%); }", color.RGBA{0, 255, 0, 255}},
		{"hsla slash", "document { color: hsl(240deg 100% 50% / 50%); }", color.RGBA{0, 0, 255, 128}},
		{"named transparent", "document { background: transparent; }", color.RGBA{0, 0, 0, 0}},
		{"named white", "document { color: white; }", color.RGBA{255, 255, 255, 255}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th, err := ParseTheme(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got color.Color
			if strings.Contains(tc.input, "background:") {
				got = th.BG
			} else {
				got = th.FG
			}
			r, g, b, a := got.RGBA()
			gotRGBA := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			if gotRGBA != tc.expected {
				t.Errorf("got %v, want %v", gotRGBA, tc.expected)
			}
		})
	}
}

func TestParseTheme_Errors(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		errSubstr string
	}{
		{"unknown element", "foo { color: red; }", "unknown element \"foo\""},
		{"unknown property", "document { foobar: red; }", "unknown property \"foobar\""},
		{"invalid color", "document { color: notacolor; }", "unrecognized color format"},
		{"unclosed block", "document { color: red;", "unclosed block"},
		{"unclosed comment", "/* unfinished", "unclosed block comment"},
		{"unknown parent", "@theme test extends nonexistent; document { color: red; }", "unknown parent theme"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTheme(tc.input)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
			}
			if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Errorf("expected error to contain %q, got: %v", tc.errSubstr, err)
			}
			// Verify error includes line and col
			if !strings.Contains(err.Error(), "line ") || !strings.Contains(err.Error(), "col ") {
				t.Errorf("expected line/col in error message: %v", err)
			}
		})
	}
}

func TestParseTheme_LoadFile(t *testing.T) {
	tempDir := t.TempDir()
	themeFile := filepath.Join(tempDir, "custom.css")
	css := `
@theme custom extends light;
document {
  color: #333333;
  background: #fafafa;
}
link {
  color: #0070f3;
}
`
	if err := os.WriteFile(themeFile, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}

	th, err := LoadThemeFile(themeFile)
	if err != nil {
		t.Fatalf("unexpected LoadThemeFile error: %v", err)
	}

	r, g, b, _ := th.Link.RGBA()
	if uint8(r>>8) != 0x00 || uint8(g>>8) != 0x70 || uint8(b>>8) != 0xf3 {
		t.Errorf("expected link color #0070f3, got %x %x %x", r>>8, g>>8, b>>8)
	}
}

func TestBuiltInLinkContrast(t *testing.T) {
	// Dark theme contrast: #58a6ff on #121214
	dr, dg, db, _ := darkTheme.BG.RGBA()
	lr, lg, lb, _ := darkTheme.Link.RGBA()

	bgLum := relativeLuminance(float64(dr>>8)/255, float64(dg>>8)/255, float64(db>>8)/255)
	linkLum := relativeLuminance(float64(lr>>8)/255, float64(lg>>8)/255, float64(lb>>8)/255)

	contrast := (linkLum + 0.05) / (bgLum + 0.05)
	if contrast < 4.5 {
		t.Errorf("dark theme link contrast ratio too low: %.2f (want >= 4.5)", contrast)
	}
}

func TestParseTheme_VersionDirective(t *testing.T) {
	// Valid version 1 directive
	v1Input := `@version 1;
@theme v1test extends light;
document { color: #111; }
`
	th, err := ParseTheme(v1Input)
	if err != nil {
		t.Fatalf("expected valid @version 1 to succeed, got %v", err)
	}
	r, _, _, _ := th.FG.RGBA()
	if uint8(r>>8) != 0x11 {
		t.Errorf("expected parsed color #111, got %x", r>>8)
	}

	// Unsupported version directive
	v2Input := `@version 2;
@theme v2test extends light;
`
	_, err = ParseTheme(v2Input)
	if err == nil {
		t.Fatalf("expected error for @version 2, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported theme format version") {
		t.Errorf("expected unsupported version error, got %v", err)
	}

	// Malformed version directive without semicolon
	malformed := `@version 1`
	_, err = ParseTheme(malformed)
	if err == nil {
		t.Fatalf("expected error for unclosed @version, got nil")
	}
}

func relativeLuminance(r, g, b float64) float64 {
	toLinear := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}
