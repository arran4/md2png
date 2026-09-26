package md2png

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestSafeHTML(t *testing.T) {
	tests := []struct {
		name       string
		md         string
		strict     bool
		wantErr    bool
		wantDiag   DiagnosticCode
		expectText []string
	}{
		{
			name:       "Inline HTML",
			md:         "Hello <span>world</span>",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"world"},
		},
		{
			name:       "Block HTML",
			md:         "<div>\nHello\n</div>",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"Hello"},
		},
		{
			name:       "HTML comments",
			md:         "Hello <!-- secret --> world",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"Hello", "world"},
		},
		{
			name:       "Line breaks",
			md:         "Line 1<br>Line 2<br/>Line 3",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"Line 1", "Line 2", "Line 3"},
		},
		{
			name:       "Script and Style tags stripped",
			md:         "<script>alert(1)</script><style>body { color: red; }</style>Content",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"Content"}, // script and style content should be ignored entirely
		},
		{
			name:       "Strict Mode HTML Failure",
			md:         "<div>fail</div>",
			strict:     true,
			wantErr:    true,
			wantDiag:   DiagRawHTML,
			expectText: nil,
		},
		{
			name:       "False prefix matching",
			md:         "<scripture>text</scripture>",
			strict:     false,
			wantErr:    false,
			wantDiag:   DiagRawHTML,
			expectText: []string{"text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := RenderOptions{}
			if tt.strict {
				opts.DiagnosticPolicy = &DiagnosticPolicy{
					FailOnRawHTML: true,
				}
			}

			res, err := RenderWithDiagnostics([]byte(tt.md), opts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			foundDiag := false
			for _, d := range res.Diagnostics {
				if d.Code == tt.wantDiag {
					foundDiag = true
					break
				}
			}
			if !foundDiag {
				t.Errorf("expected diagnostic %v, but was not found in %v", tt.wantDiag, res.Diagnostics)
			}

			r := &renderer{}
			extracted := r.safeExtractHTMLText([]byte(tt.md))

			for _, expectedText := range tt.expectText {
				found := false
				for _, actualText := range extracted {
					if strings.Contains(actualText, expectedText) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected extracted text to contain %q, but got %v", expectedText, extracted)
				}
			}
		})
	}
}

func TestRawHTMLRenderPathSuppressesSeparatedBodies(t *testing.T) {
	md := []byte("Before <script>alert('script body')</script> middle <style>style body { color: red }</style> after <!-- comment body --> last<br>tail")
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	doc := parser.Parser().Parse(text.NewReader(md))
	paragraph, ok := doc.FirstChild().(*ast.Paragraph)
	if !ok {
		t.Fatalf("expected paragraph, got %T", doc.FirstChild())
	}
	opts := RenderOptions{}
	if err := normalizeRenderOptions(&opts); err != nil {
		t.Fatal(err)
	}
	r := &renderer{c: newCanvas(opts.Width, opts.Margin, opts.Theme, opts.Fonts, opts.BaseFontSize, opts.MaxHeight)}
	var tokens []textToken
	r.collectInlineTokens(paragraph, md, opts.Fonts.Regular, opts.BaseFontSize, opts.Theme.FG, &tokens)
	var got strings.Builder
	for _, token := range tokens {
		if token.newline {
			got.WriteByte('\n')
		} else {
			got.WriteString(token.text)
		}
	}
	drawable := got.String()
	for _, forbidden := range []string{"alert('script body')", "style body", "comment body"} {
		if strings.Contains(drawable, forbidden) {
			t.Fatalf("raw HTML body leaked into drawable tokens: %q", drawable)
		}
	}
	for _, safe := range []string{"Before ", " middle ", " after ", " last", "tail"} {
		if !strings.Contains(drawable, safe) {
			t.Fatalf("safe prose %q missing from drawable tokens %q", safe, drawable)
		}
	}
	if !strings.Contains(drawable, "last\ntail") {
		t.Fatalf("expected <br> newline in drawable tokens, got %q", drawable)
	}

	block := []byte("<div>\nblock safe<br>next\n</div>\n\nAfter block")
	res, err := RenderWithDiagnostics(block, RenderOptions{})
	if err != nil || res.Image == nil {
		t.Fatalf("block HTML render = (%v, %v), want image and no error", res.Image, err)
	}
	if len(res.Diagnostics) == 0 || res.Diagnostics[0].Code != DiagRawHTML {
		t.Fatalf("expected block raw-HTML diagnostic, got %#v", res.Diagnostics)
	}
}

func TestSafeHTML_ExactExtraction(t *testing.T) {
	tests := []struct {
		name       string
		md         string
		expectText []string
	}{
		{
			name:       "Inline HTML preservation",
			md:         "Hello <span>world</span>",
			expectText: []string{"Hello world"},
		},
		{
			name:       "Block HTML text preservation",
			md:         "<div>\nHello\n</div>",
			expectText: []string{"\nHello\n"},
		},
		{
			name:       "Comment missing",
			md:         "Hello <!-- secret --> world",
			expectText: []string{"Hello  world"},
		},
		{
			name:       "BR exact extraction",
			md:         "Line 1<br>Line 2<br/>Line 3<br\t/>Line 4",
			expectText: []string{"Line 1", "\n", "Line 2", "\n", "Line 3", "\n", "Line 4"},
		},
		{
			name:       "Script and Style tags missing entirely",
			md:         "Content<script>alert(1)</script><style>body { color: red; }</style>Here",
			expectText: []string{"ContentHere"},
		},
		{
			name:       "Entities and surrounding text",
			md:         "Text &amp; <span>More</span>",
			expectText: []string{"Text & More"},
		},
		{
			name:       "Malformed HTML",
			md:         "<div>incomplete",
			expectText: []string{"incomplete"},
		},
		{
			name:       "False prefix matching treated as standard tag",
			md:         "<scripture>text</scripture><styleguide>content</styleguide><bravo>!",
			expectText: []string{"textcontent!"},
		},
		{
			name:       "Tag whitespace support",
			md:         "<script\ttype=\"text/javascript\">alert();</script\n>Safe",
			expectText: []string{"Safe"},
		},
	}

	r := &renderer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extracted := r.safeExtractHTMLText([]byte(tt.md))

			if len(extracted) != len(tt.expectText) {
				t.Fatalf("Length mismatch: expected %v but got %v", tt.expectText, extracted)
			}
			for i, actual := range extracted {
				if actual != tt.expectText[i] {
					t.Errorf("Mismatch at index %d: expected %q, got %q", i, tt.expectText[i], actual)
				}
			}
		})
	}
}

func TestHTMLResourceNetworkAccess(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Use actual markdown image parsing too just to ensure our baseline fetch tracking works!
	mdWithStandardImage := []byte(fmt.Sprintf("This is an image ![test](%s/img.png)", server.URL))

	// Enable remote policy to prove we allow standard rendering but NOT raw html evaluating
	opts := RenderOptions{
		ImagePolicy: &ImagePolicy{
			AllowRemote: true,
			HTTPClient:  server.Client(),
		},
	}

	_, _ = RenderWithDiagnostics(mdWithStandardImage, opts)
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("Expected exactly 1 request from baseline standard Markdown image rendering to prove tracking works, got %d", requestCount)
	}

	// Reset counter
	atomic.StoreInt32(&requestCount, 0)

	// Test actual raw HTML
	md := []byte(fmt.Sprintf(`This is <img src="%s/nope.png"> an image test.`, server.URL))

	res, err := RenderWithDiagnostics(md, opts)

	if err != nil {
		t.Fatalf("Unexpected render failure: %v", err)
	}

	if res.Image == nil {
		t.Fatalf("Expected an image result")
	}

	if atomic.LoadInt32(&requestCount) > 0 {
		t.Fatalf("Renderer actually tried to load the image via network! Expected no network request for raw HTML.")
	}

	foundRawDiag := false
	for _, diag := range res.Diagnostics {
		if diag.Code == DiagImageLoadFailed {
			t.Fatalf("Renderer reported DiagImageLoadFailed despite not being expected to evaluate raw HTML network resources.")
		}
		if diag.Code == DiagRawHTML {
			foundRawDiag = true
		}
	}

	if !foundRawDiag {
		t.Fatalf("Missing DiagRawHTML diagnostic")
	}
}

func createTestImage(t *testing.T, path string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	err = png.Encode(f, img)
	if err != nil {
		t.Fatalf("Failed to encode test image: %v", err)
	}
}

func TestSuppressedHTML_CompleteCoverage(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	localImagePath := filepath.Join(tempDir, "testimg.png")
	createTestImage(t, localImagePath)
	// File URLs for markdown
	localImageURL := "file://" + localImagePath

	tests := []struct {
		name                 string
		md                   string
		expectDraw           []string
		expectNoDraw         []string
		expectReqs           int32
		expectFootnotes      int
		expectLocalImageDiag bool
	}{
		{
			name:                 "Local file load check",
			md:                   fmt.Sprintf("Before <script>![leak](%s) [link](local_link)</script> After", localImageURL),
			expectDraw:           []string{"Before ", " After"},
			expectNoDraw:         []string{"leak", "link", "[1]", "[2]"},
			expectReqs:           0,
			expectFootnotes:      0,
			expectLocalImageDiag: false,
		},
		{
			name:                 "Positive local file load",
			md:                   fmt.Sprintf("Before ![valid](%s) [valid](local_link) After", localImageURL),
			expectDraw:           []string{"Before ", " After", "valid"},
			expectNoDraw:         []string{"leak"},
			expectReqs:           0, // local load
			expectFootnotes:      2,
			expectLocalImageDiag: false, // Should succeed
		},
		{
			name:            "Script context leak check",
			md:              fmt.Sprintf("Before <script>![leak](%s/img1.png) [leak](%s/link1)</script> After", server.URL, server.URL),
			expectDraw:      []string{"Before ", " After"},
			expectNoDraw:    []string{"leak", "img1", "link1", "[1]", "[2]"},
			expectReqs:      0,
			expectFootnotes: 0,
		},
		{
			name:            "Style context leak check",
			md:              fmt.Sprintf("Before <style>![leak](%s/img2.png) [leak](%s/link2)</style> After", server.URL, server.URL),
			expectDraw:      []string{"Before ", " After"},
			expectNoDraw:    []string{"leak", "img2", "link2", "[1]", "[2]"},
			expectReqs:      0,
			expectFootnotes: 0,
		},
		{
			name:            "Positive control outside suppressed tag",
			md:              fmt.Sprintf("Before ![valid](%s/valid.png) [valid](%s/validlink) After", server.URL, server.URL),
			expectDraw:      []string{"Before ", " After", "valid"},
			expectNoDraw:    []string{"leak"},
			expectReqs:      1, // The image request
			expectFootnotes: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atomic.StoreInt32(&requestCount, 0)

			mdParser := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
			doc := mdParser.Parser().Parse(text.NewReader([]byte(tt.md)))
			paragraph, ok := doc.FirstChild().(*ast.Paragraph)
			if !ok {
				t.Fatalf("expected paragraph, got %T", doc.FirstChild())
			}

			opts := RenderOptions{}
			_ = normalizeRenderOptions(&opts)

			policy := DefaultCLIImagePolicy()
			policy.AllowRemote = true
			policy.HTTPClient = server.Client()
			policy.AllowLocal = true
			policy.SandboxLocal = false // to load temp file easily

			c := newCanvas(opts.Width, opts.Margin, opts.Theme, opts.Fonts, opts.BaseFontSize, opts.MaxHeight)

			r := &renderer{
				c:              c,
				baseSize:       opts.BaseFontSize,
				linkFootnotes:  true,
				imageFootnotes: true,
				imagePolicy:    policy,
			}
			r.ensureImageResolvers()

			var tokens []textToken
			r.collectInlineTokens(paragraph, []byte(tt.md), opts.Fonts.Regular, opts.BaseFontSize, opts.Theme.FG, &tokens)

			if atomic.LoadInt32(&requestCount) != tt.expectReqs {
				t.Fatalf("Expected %d requests, got %d", tt.expectReqs, requestCount)
			}

			var got strings.Builder
			for _, token := range tokens {
				if token.text != "" {
					got.WriteString(token.text)
				}
			}
			joined := got.String()

			for _, avoid := range tt.expectNoDraw {
				if strings.Contains(joined, avoid) {
					t.Fatalf("Output text incorrectly contains suppressed content %q", avoid)
				}
			}

			for _, require := range tt.expectDraw {
				if !strings.Contains(joined, require) {
					t.Fatalf("Output text missing required content %q, got: %q", require, joined)
				}
			}

			if len(r.footnotes) != tt.expectFootnotes {
				t.Fatalf("Expected %d footnotes, got %d: %v", tt.expectFootnotes, len(r.footnotes), r.footnotes)
			}

			hasLocalDiag := false
			for _, diag := range r.diagnostics {
				if diag.Code == DiagImageLoadFailed && strings.Contains(diag.Destination, localImagePath) {
					hasLocalDiag = true
				}
			}
			if hasLocalDiag != tt.expectLocalImageDiag {
				t.Fatalf("Expected local image diagnostic to be %v, got %v", tt.expectLocalImageDiag, hasLocalDiag)
			}
		})
	}
}
