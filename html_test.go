package md2png

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
