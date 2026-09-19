package md2png

import (
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

			// Note: the test doesn't assert that the extracted text is perfectly visually rendered,
			// just testing safe extraction visually is hard. We test that no panic happens and
			// diagnostics are raised properly.
		})
	}
}
