package cli

import (
	"testing"

	"github.com/arran4/md2png"
)

func ptrInt(v int) *int             { return &v }
func ptrFloat64(v float64) *float64 { return &v }

func TestCLIValidationBehavior(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions(nil, ptrInt(800), ptrInt(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
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

	opts2, err2 := ConvertCommandArgsToRenderOptions(nil, nil, nil, ptrFloat64(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
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
	opts, err := ConvertCommandArgsToRenderOptions(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if opts.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be false when absent")
	}

	// Disable mapping true
	opts2, err2 := ConvertCommandArgsToRenderOptions(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptrBool(true), "")
	if err2 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err2)
	}
	if !opts2.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be mapped to true")
	}

	// Disable mapping false
	opts3, err3 := ConvertCommandArgsToRenderOptions(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptrBool(false), "")
	if err3 != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err3)
	}
	if opts3.DisableHighlighting {
		t.Errorf("expected DisableHighlighting to be mapped to false")
	}
}

func TestCLITableFitWidthEquivalence(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptrBool(true), nil, "")
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if !opts.TableFitWidth {
		t.Errorf("expected TableFitWidth to be true")
	}

	opts2, err2 := ConvertCommandArgsToRenderOptions(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptrBool(false), nil, "")
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

	optsCLI, errCLI := ConvertCommandArgsToRenderOptions(nil, ptrInt(200), ptrInt(10), nil, nil, nil, nil, nil, nil, nil, nil, ptrBool(true), nil, "")
	if errCLI != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", errCLI)
	}

	resCLI, err := md2png.RenderWithDiagnostics(md, optsCLI)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if resLibrary.Image.Bounds().Dx() != resCLI.Image.Bounds().Dx() {
		t.Fatalf("Library width %d did not match CLI width %d", resLibrary.Image.Bounds().Dx(), resCLI.Image.Bounds().Dx())
	}
}
