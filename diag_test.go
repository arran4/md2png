package md2png

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

var kindTestUnsupportedBlock = ast.NewNodeKind("TestUnsupportedBlock")

type testUnsupportedBlock struct{ ast.BaseBlock }

func (n *testUnsupportedBlock) Kind() ast.NodeKind { return kindTestUnsupportedBlock }

func (n *testUnsupportedBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

func TestDiagnostics(t *testing.T) {
	md := []byte("Testing <br> unsupported and missing image ![alt](missing.png)")

	t.Run("Best-effort local/remote image failure and offsets", func(t *testing.T) {
		// Include enough text before the image to have a non-zero, predictable offset.
		// "1234567890" is 10 bytes. The space is 11. "![missing]" starts at index 11.
		mdText := []byte("1234567890 ![missing](missing.png) ![malformed](data:image/png;base64,bad-data) ![denied](http://example.com/denied.png)")

		opts := RenderOptions{
			ImagePolicy: &ImagePolicy{
				AllowLocal:  true,
				AllowRemote: false, // will cause policy denial for the http image
			},
		}
		res, err := RenderWithDiagnostics(mdText, opts)

		if err != nil {
			t.Fatalf("Expected no error in best-effort mode, got %v", err)
		}
		if res.Image == nil {
			t.Fatal("Expected image in best-effort mode, got nil")
		}

		if len(res.Diagnostics) != 3 {
			t.Fatalf("Expected 3 diagnostics, got %d: %v", len(res.Diagnostics), res.Diagnostics)
		}

		for _, diag := range res.Diagnostics {
			if diag.Code != DiagImageLoadFailed {
				t.Errorf("Expected DiagImageLoadFailed, got %v", diag.Code)
			}
			if diag.Severity != SeverityWarning {
				t.Errorf("Expected SeverityWarning, got %v", diag.Severity)
			}
		}

		if !strings.Contains(res.Diagnostics[0].Error.Error(), "no such file or directory") {
			t.Errorf("Expected local missing error, got %v", res.Diagnostics[0].Error)
		}

		// The 3rd diag is the remote denied
		if !strings.Contains(res.Diagnostics[2].Error.Error(), "denied by policy") {
			t.Errorf("Expected policy denied error, got %v", res.Diagnostics[2].Error)
		}
	})

	t.Run("Strict HTML failure", func(t *testing.T) {
		opts := RenderOptions{
			DiagnosticPolicy: &DiagnosticPolicy{
				FailOnRawHTML: true,
			},
		}
		res, err := RenderWithDiagnostics(md, opts)
		if err == nil {
			t.Fatal("Expected error due to strict raw HTML policy, got nil")
		}
		if len(res.Diagnostics) == 0 {
			t.Fatal("Expected diagnostics on strict raw HTML policy failure, got none")
		}
		if !strings.Contains(err.Error(), "Raw HTML is intentionally stripped") {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("Strict image failure", func(t *testing.T) {
		mdImg := []byte("![alt](missing.png)")
		opts := RenderOptions{
			DiagnosticPolicy: &DiagnosticPolicy{
				FailOnImageError: true,
			},
		}
		res, err := RenderWithDiagnostics(mdImg, opts)
		if err == nil {
			t.Fatal("Expected error due to strict image error policy, got nil")
		}
		if len(res.Diagnostics) == 0 {
			t.Fatal("Expected diagnostics on strict image error policy failure, got none")
		}
		if !strings.Contains(err.Error(), "no such file or directory") && !strings.Contains(err.Error(), "Failed to load") {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("Diagnostic order and codes", func(t *testing.T) {
		mdHtmlImg := []byte("A <br> B ![alt](missing.png)")
		opts := RenderOptions{}
		res, err := RenderWithDiagnostics(mdHtmlImg, opts)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(res.Diagnostics) != 2 {
			t.Fatalf("Expected exactly 2 diagnostics, got %d", len(res.Diagnostics))
		}

		if res.Diagnostics[0].Code != DiagRawHTML {
			t.Errorf("Expected first diagnostic to be DiagRawHTML, got %v", res.Diagnostics[0].Code)
		}
		if res.Diagnostics[0].Offset <= 0 {
			t.Errorf("Expected DiagRawHTML to have a positive source offset, got %d", res.Diagnostics[0].Offset)
		}

		if res.Diagnostics[1].Code != DiagImageLoadFailed {
			t.Errorf("Expected second diagnostic to be DiagImageLoadFailed, got %v", res.Diagnostics[1].Code)
		}
	})
}

func TestRenderSetupRegression(t *testing.T) {
	md := []byte("Hello")

	t.Run("Empty RenderOptions does not panic", func(t *testing.T) {
		opts := RenderOptions{}
		_, err := RenderWithDiagnostics(md, opts)
		if err != nil {
			t.Fatalf("RenderWithDiagnostics with empty opts failed: %v", err)
		}

		_, err = Render(md, opts)
		if err != nil {
			t.Fatalf("Render with empty opts failed: %v", err)
		}
	})

	t.Run("Partial Fonts Fallback", func(t *testing.T) {
		baseFont, _ := LoadFonts(FontConfig{})
		opts := RenderOptions{
			Fonts: Fonts{
				Regular: baseFont.Regular,
				// Missing Bold and Mono should be filled in by setup path
			},
		}

		_, err := RenderWithDiagnostics(md, opts)
		if err != nil {
			t.Fatalf("RenderWithDiagnostics failed with partial fonts: %v", err)
		}
	})
}

func TestDiagnosticIgnoreCodes(t *testing.T) {
	md := []byte("![missing](missing.png) <div>html</div>")

	opts := RenderOptions{
		DiagnosticPolicy: &DiagnosticPolicy{
			IgnoreCodes: map[DiagnosticCode]bool{
				DiagRawHTML: true,
			},
		},
	}

	res, err := RenderWithDiagnostics(md, opts)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	for _, diag := range res.Diagnostics {
		if diag.Code == DiagRawHTML {
			t.Fatalf("Expected DiagRawHTML to be suppressed, but it was found in diagnostics")
		}
	}

	// Test that we CANNOT suppress fatal errors
	optsFatal := RenderOptions{
		DiagnosticPolicy: &DiagnosticPolicy{
			FailOnImageError: true,
			IgnoreCodes: map[DiagnosticCode]bool{
				DiagImageLoadFailed: true,
			},
		},
	}

	resFatal, errFatal := RenderWithDiagnostics(md, optsFatal)
	if errFatal == nil {
		t.Fatalf("Expected error due to strict image error policy, got nil")
	}

	foundFatalDiag := false
	for _, diag := range resFatal.Diagnostics {
		if diag.Code == DiagImageLoadFailed {
			foundFatalDiag = true
		}
	}

	if !foundFatalDiag {
		t.Fatalf("Expected DiagImageLoadFailed to NOT be suppressed since it caused a fatal error, but it was suppressed.")
	}
}

func TestRenderUnsupportedHelperStrict(t *testing.T) {
	// This is deliberately a helper unit test. ast.Document is normally
	// supported by the renderer walk, so it is not used as an unsupported node.
	node := &testUnsupportedBlock{}

	opts := RenderOptions{
		DiagnosticPolicy: &DiagnosticPolicy{
			FailOnUnsupported: true,
		},
	}
	_ = normalizeRenderOptions(&opts)

	r := &renderer{
		diagPolicy: *opts.DiagnosticPolicy,
		c:          newCanvas(100, 10, opts.Theme, opts.Fonts, 16, 100),
	}
	r.renderUnsupported(node)

	if r.renderErr == nil {
		t.Fatal("Expected error when FailOnUnsupported is true")
	}
	if len(r.diagnostics) == 0 {
		t.Fatal("Expected diagnostics to be preserved on strict unsupported failure")
	}
	if r.diagnostics[0].Code != DiagUnsupportedNode {
		t.Errorf("Expected DiagUnsupportedNode, got %v", r.diagnostics[0].Code)
	}
	if r.diagnostics[0].Severity != SeverityError {
		t.Errorf("Expected SeverityError, got %v", r.diagnostics[0].Severity)
	}
}

func TestStrictUnsupportedNodeThroughTraversal(t *testing.T) {
	opts := RenderOptions{DiagnosticPolicy: &DiagnosticPolicy{FailOnUnsupported: true}}
	if err := normalizeRenderOptions(&opts); err != nil {
		t.Fatal(err)
	}
	r := &renderer{
		c:          newCanvas(opts.Width, opts.Margin, opts.Theme, opts.Fonts, opts.BaseFontSize, opts.MaxHeight),
		baseSize:   opts.BaseFontSize,
		diagPolicy: *opts.DiagnosticPolicy,
	}
	doc := ast.NewDocument()
	doc.AppendChild(doc, &testUnsupportedBlock{})
	err := r.renderDocument(nil, doc)
	if err == nil {
		t.Fatal("expected strict traversal error for unsupported node")
	}
	if len(r.diagnostics) != 1 || r.diagnostics[0].Code != DiagUnsupportedNode || r.diagnostics[0].Severity != SeverityError {
		t.Fatalf("expected retained fatal unsupported-node diagnostic, got %#v", r.diagnostics)
	}
}

func TestImageDiagnosticsMalformedFileAndHTTPFailure(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "malformed.png")
	if err := os.WriteFile(badPath, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "deterministic image failure", http.StatusTeapot)
	}))
	defer server.Close()

	md := []byte(fmt.Sprintf("![local fallback](%s) ![remote fallback](%s/broken.png)", filepath.Base(badPath), server.URL))
	base := RenderOptions{BaseDir: filepath.Dir(badPath), ImagePolicy: &ImagePolicy{AllowLocal: true, AllowRemote: true, HTTPClient: server.Client()}}

	res, err := RenderWithDiagnostics(md, base)
	if err != nil || res.Image == nil {
		t.Fatalf("best-effort render = (%v, %v), want image and no error", res.Image, err)
	}
	if len(res.Diagnostics) != 2 {
		t.Fatalf("got diagnostics %#v, want local decode and HTTP failures", res.Diagnostics)
	}
	for _, diag := range res.Diagnostics {
		if diag.Code != DiagImageLoadFailed || diag.Severity != SeverityWarning || diag.Error == nil || diag.Offset != -1 {
			t.Fatalf("unexpected best-effort diagnostic %#v", diag)
		}
	}
	if !strings.Contains(res.Diagnostics[0].Error.Error(), "image: unknown format") {
		t.Fatalf("expected malformed image decode cause, got %v", res.Diagnostics[0].Error)
	}
	if !strings.Contains(res.Diagnostics[1].Error.Error(), "418") {
		t.Fatalf("expected HTTP status cause, got %v", res.Diagnostics[1].Error)
	}

	base.DiagnosticPolicy = &DiagnosticPolicy{FailOnImageError: true}
	strict, strictErr := RenderWithDiagnostics(md, base)
	if strictErr == nil || len(strict.Diagnostics) != 1 {
		t.Fatalf("strict result = (%v, %#v), want error and first causal diagnostic", strictErr, strict.Diagnostics)
	}
	if !errors.Is(strictErr, strict.Diagnostics[0].Error) && strictErr.Error() != strict.Diagnostics[0].Error.Error() {
		t.Fatalf("strict error %v did not retain diagnostic cause %v", strictErr, strict.Diagnostics[0].Error)
	}
}

func TestRawHTMLDiagnosticOffsetAndImageOffsetUnavailable(t *testing.T) {
	md := []byte("first line\nsecond <br> line\n![missing](missing.png)")
	res, err := RenderWithDiagnostics(md, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantRawOffset := strings.Index(string(md), "<br>")
	var sawRaw, sawImage bool
	for _, diag := range res.Diagnostics {
		switch diag.Code {
		case DiagRawHTML:
			sawRaw = diag.Offset == wantRawOffset
		case DiagImageLoadFailed:
			sawImage = diag.Offset == -1
		}
	}
	if !sawRaw || !sawImage {
		t.Fatalf("expected raw offset %d and unavailable image offset, got %#v", wantRawOffset, res.Diagnostics)
	}
}

func TestDiagnosticOrderAndCodesPrecise(t *testing.T) {
	mdHtmlImg := []byte("![missing](missing.png)\n<br>\n![missing2](missing2.png)")
	opts := RenderOptions{}
	res, err := RenderWithDiagnostics(mdHtmlImg, opts)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(res.Diagnostics) != 3 {
		t.Fatalf("Expected exactly 3 diagnostics, got %d", len(res.Diagnostics))
	}

	if res.Diagnostics[0].Code != DiagImageLoadFailed {
		t.Errorf("Expected first diagnostic to be DiagImageLoadFailed, got %v", res.Diagnostics[0].Code)
	}
	if res.Diagnostics[1].Code != DiagRawHTML {
		t.Errorf("Expected second diagnostic to be DiagRawHTML, got %v", res.Diagnostics[1].Code)
	}
	if res.Diagnostics[2].Code != DiagImageLoadFailed {
		t.Errorf("Expected third diagnostic to be DiagImageLoadFailed, got %v", res.Diagnostics[2].Code)
	}
}
