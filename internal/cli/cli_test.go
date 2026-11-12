package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/arran4/md2png"
)

func ptrInt(v int) *int             { return &v }
func ptrFloat64(v float64) *float64 { return &v }
func ptrString(s string) *string    { return &s }

func TestCLIValidationBehavior(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{Width: ptrInt(800), Margin: ptrInt(0)})
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if opts.Width != 800 {
		t.Errorf("expected width 800, got %d", opts.Width)
	}
	if opts.Margin != 0 {
		t.Errorf("expected margin 0, got %d", opts.Margin)
	}
	if !opts.ZeroMargin {
		t.Errorf("expected ZeroMargin to be true")
	}

	img, err := md2png.Render([]byte("test"), opts)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected image, got nil")
	}

	opts2, err2 := ConvertCommandArgsToRenderOptions(CommandRenderArgs{Pt: ptrFloat64(0)})
	if err2 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err2)
	}

	img2, err2 := md2png.Render([]byte("test"), opts2)
	if err2 != nil {
		t.Fatalf("unexpected render error for explicit 0: %v", err2)
	}
	if img2 == nil {
		t.Fatalf("expected image, got nil")
	}
}

func TestCLIDisableHighlighting(t *testing.T) {
	// Default absent behavior
	opts, err := ConvertCommandArgsToRenderOptions()
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if opts.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be false when absent")
	}

	// Disable mapping true
	opts2, err2 := ConvertCommandArgsToRenderOptions(CommandRenderArgs{DisableHighlighting: ptrBool(true)})
	if err2 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err2)
	}
	if !opts2.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be mapped to true")
	}

	// Disable mapping false
	opts3, err3 := ConvertCommandArgsToRenderOptions(CommandRenderArgs{DisableHighlighting: ptrBool(false)})
	if err3 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err3)
	}
	if opts3.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be mapped to false")
	}
}

func TestCLITableFitWidthEquivalence(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{TableFitWidth: ptrBool(true)})
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if !opts.TableFitWidth {
		t.Errorf("expected TableFitWidth to be true")
	}

	opts2, err2 := ConvertCommandArgsToRenderOptions(CommandRenderArgs{TableFitWidth: ptrBool(false)})
	if err2 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err2)
	}
	if opts2.TableFitWidth {
		t.Errorf("expected TableFitWidth to be false")
	}
}

func ptrBool(b bool) *bool { return &b }

func TestCLITableFitWidthEndToEnd(t *testing.T) {
	md := []byte(`
| C1 | C2 | C3 | C4 | C5 | C6 | C7 | C8 | C9 | C10 |
| -- | -- | -- | -- | -- | -- | -- | -- | -- | --- |
| 1  | 2  | 3  | 4  | 5  | 6  | 7  | 8  | 9  | 10  |
`)

	opts := md2png.RenderOptions{Width: 200, Margin: 10, TableFitWidth: true}
	resLibrary, err := md2png.RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	optsCLI, errCLI := ConvertCommandArgsToRenderOptions(CommandRenderArgs{Width: ptrInt(200), Margin: ptrInt(10), TableFitWidth: ptrBool(true)})
	if errCLI != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", errCLI)
	}

	resCLI, err := md2png.RenderWithDiagnostics(md, optsCLI)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if resLibrary.Image.Bounds().Dx() != resCLI.Image.Bounds().Dx() {
		t.Fatalf("CLI output width %d did not match Library render width %d", resLibrary.Image.Bounds().Dx(), resCLI.Image.Bounds().Dx())
	}
}

func TestConvertCommandArgsToRenderOptions_CodeTabWidth(t *testing.T) {
	tests := []struct {
		name         string
		codeTabWidth *int
		expected     int
	}{
		{"nil defaults to 4", nil, 4},
		{"zero defaults to 4", ptrInt(0), 4},
		{"negative defaults to 4", ptrInt(-1), 4},
		{"positive uses value", ptrInt(8), 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{CodeTabWidth: tt.codeTabWidth})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.CodeTabWidth != tt.expected {
				t.Errorf("expected CodeTabWidth %d, got %d", tt.expected, opts.CodeTabWidth)
			}
		})
	}
}

func TestConvertCommandArgsToRenderOptions_ThemeFile(t *testing.T) {
	tempDir := t.TempDir()
	themePath := filepath.Join(tempDir, "brand.css")
	css := `
@theme brand extends light;
document {
  color: #123456;
  background: #f0f0f0;
}
link {
  color: #ff0055;
}
`
	if err := os.WriteFile(themePath, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}

	themeName := "dark"
	// --theme-file overrides --theme "dark"
	opts, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{Theme: &themeName, ThemeFile: &themePath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, g, b, _ := opts.Theme.Link.RGBA()
	if uint8(r>>8) != 0xff || uint8(g>>8) != 0x00 || uint8(b>>8) != 0x55 {
		t.Errorf("expected link color #ff0055 from theme-file, got rgba(%d, %d, %d)", r>>8, g>>8, b>>8)
	}

	// Invalid theme file error
	badPath := filepath.Join(tempDir, "bad.css")
	if err := os.WriteFile(badPath, []byte("unknown { bad: color; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ConvertCommandArgsToRenderOptions(CommandRenderArgs{ThemeFile: &badPath})
	if err == nil {
		t.Fatal("expected error with bad theme file, got nil")
	}
}

func TestConvertCommandArgsToRenderOptions_FootnoteOrder(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.FootnoteOrder != md2png.FootnoteOrderAuthoredFirst {
		t.Errorf("expected default AuthoredFirst, got %v", opts.FootnoteOrder)
	}

	opts2, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{FootnoteOrder: ptrString("authored-first")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts2.FootnoteOrder != md2png.FootnoteOrderAuthoredFirst {
		t.Errorf("expected AuthoredFirst, got %v", opts2.FootnoteOrder)
	}

	opts3, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{FootnoteOrder: ptrString("links-first")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts3.FootnoteOrder != md2png.FootnoteOrderLinksFirst {
		t.Errorf("expected LinksFirst, got %v", opts3.FootnoteOrder)
	}

	_, err = ConvertCommandArgsToRenderOptions(CommandRenderArgs{FootnoteOrder: ptrString("invalid-order")})
	if err == nil {
		t.Fatalf("expected error for invalid footnote order, got nil")
	}
}

// The type-switched variadic boundary accepts a complete typed struct while
// preserving omitted vs explicit values, and rejects unknown option types.
func TestConvertCommandArgsToRenderOptions_TypeSwitchedStruct(t *testing.T) {
	t.Run("no arguments defaults", func(t *testing.T) {
		got, err := ConvertCommandArgsToRenderOptions()
		if err != nil {
			t.Fatal(err)
		}
		if !*got.LinkFootnotes || *got.ImageFootnotes || got.CodeTabWidth != 4 {
			t.Fatalf("unexpected default options: %+v", got)
		}
	})
	t.Run("typed struct with explicit false and zero", func(t *testing.T) {
		got, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{
			Margin: ptrInt(0),
			TableFitWidth: ptrBool(false),
			FootnoteLinks: ptrBool(false),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !got.ZeroMargin || got.TableFitWidth || *got.LinkFootnotes {
			t.Fatalf("explicit false/zero was not preserved: %+v", got)
		}
	})
	t.Run("last struct wins", func(t *testing.T) {
		got, err := ConvertCommandArgsToRenderOptions(
			CommandRenderArgs{TableFitWidth: ptrBool(true)},
			CommandRenderArgs{Margin: ptrInt(0)},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.TableFitWidth || !got.ZeroMargin {
			t.Fatalf("last config did not replace first: %+v", got)
		}
	})
	for _, option := range []any{nil, 3, "unknown", (*CommandRenderArgs)(nil)} {
		t.Run(fmt.Sprintf("unsupported-%T", option), func(t *testing.T) {
			if _, err := ConvertCommandArgsToRenderOptions(option); err == nil {
				t.Fatalf("expected unsupported option error for %T", option)
			}
		})
	}
}
