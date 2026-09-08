package cli

import (
	"testing"

	"github.com/arran4/md2png"
)

func ptrInt(v int) *int { return &v }
func ptrFloat64(v float64) *float64 { return &v }

func TestCLIValidationBehavior(t *testing.T) {
	opts, err := ConvertCommandArgsToRenderOptions(ptrInt(800), ptrInt(0), nil, nil, nil, nil, nil, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err)
	}
	if opts.Width != 800 { t.Errorf("expected width 800, got %d", opts.Width) }
	if opts.Margin != 0 { t.Errorf("expected margin 0, got %d", opts.Margin) }
	if !opts.ZeroMargin { t.Errorf("expected ZeroMargin to be true") }

	img, err := md2png.Render([]byte("test"), opts)
	if err != nil { t.Fatalf("unexpected render error: %v", err) }
	if img == nil { t.Fatalf("expected image, got nil") }

	opts2, err2 := ConvertCommandArgsToRenderOptions(nil, nil, ptrFloat64(0), nil, nil, nil, nil, nil, nil, nil, "")
	if err2 != nil { t.Fatalf("unexpected ConvertCommandArgsToRenderOptions error: %v", err2) }

	img2, err2 := md2png.Render([]byte("test"), opts2)
	if err2 != nil { t.Fatalf("unexpected render error for explicit 0: %v", err2) }
	if img2 == nil { t.Fatalf("expected image, got nil") }
}
