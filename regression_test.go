package md2png

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden determines whether golden files should be updated.
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func compareImage(t *testing.T, expectedPath string, actualImg image.Image) {
	t.Helper()
	var actualBuf bytes.Buffer
	if err := png.Encode(&actualBuf, actualImg); err != nil {
		t.Fatalf("Failed to encode actual image: %v", err)
	}
	actualBytes := actualBuf.Bytes()

	if updateGolden {
		if err := os.MkdirAll(filepath.Dir(expectedPath), 0755); err != nil {
			t.Fatalf("Failed to create golden directory: %v", err)
		}
		if err := os.WriteFile(expectedPath, actualBytes, 0644); err != nil {
			t.Fatalf("Failed to write golden file: %v", err)
		}
		return
	}

	expectedBytes, err := os.ReadFile(expectedPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Golden file %s not found. Run with UPDATE_GOLDEN=1 to create it.", expectedPath)
		}
		t.Fatalf("Failed to read golden file %s: %v", expectedPath, err)
	}

	if !bytes.Equal(expectedBytes, actualBytes) {
		actualPath := expectedPath + ".actual.png"
		_ = os.WriteFile(actualPath, actualBytes, 0644)
		t.Errorf("Image mismatch for %s. Actual image written to %s", expectedPath, actualPath)
	}
}

func getDeterministicOptions(t *testing.T) RenderOptions {
	fonts, err := LoadFonts(FontConfig{SizeBase: 16})
	if err != nil {
		if t != nil {
			t.Fatalf("load fonts: %v", err)
		}
	}
	return RenderOptions{
		Width:        600,
		BaseFontSize: 16,
		Margin:       20,
		Theme:        lightTheme,
		Fonts:        fonts,
		MaxHeight:    8000,
	}
}

func TestRendererDeterministicRegression(t *testing.T) {
	cases := []struct {
		name string
		md   string
		opts *RenderOptions
	}{
		{"headings_paragraphs", "# Heading 1\n\n## Heading 2\n\n### Heading 3\n\nParagraph 1.\n\nParagraph 2.", nil},
		{"nested_lists", "- Item 1\n  - Item 1.1\n  - Item 1.2\n- Item 2\n  1. Ordered 1\n  2. Ordered 2", nil},
		{"inline_styling", "Normal **bold** *italic* ~~strikethrough~~ `code` [link](https://example.com)", nil},
		{"code_blocks", "```go\nfunc main() {\n\tprintln(\"Hello\")\n}\n```\n\n    Plain code block", nil},
		{"blockquotes_hr", "> Quote 1\n> Quote 2\n\n---\n\n> Quote 3", nil},
		{"tables", "| Col 1 | Col 2 |\n|---|---|\n| Val 1 | Val 2 |\n| Val 3 | Val 4 |", nil},
		{"local_images", "![local image](testdata/test_image.png)", nil},
		{"footnotes", "Here is a [link](https://example.com) and an ![image](testdata/test_image.png).", func() *RenderOptions {
			yes := true
			opts := getDeterministicOptions(nil)
			opts.LinkFootnotes = &yes
			opts.ImageFootnotes = &yes
			return &opts
		}()},
		{"unsupported_nodes", "<div>Raw HTML is unsupported natively</div>", nil},
		{"dark_theme", "# Dark Theme\n\nThis is dark theme.", func() *RenderOptions {
			opts := getDeterministicOptions(nil)
			opts.Theme = darkTheme
			return &opts
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := getDeterministicOptions(t)
			if tc.opts != nil {
				opts = *tc.opts
			}
			img, err := Render([]byte(tc.md), opts)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			goldenPath := filepath.Join("testdata", "golden", tc.name+".png")
			compareImage(t, goldenPath, img)
		})
	}
}

func FuzzRenderer(f *testing.F) {
	f.Add([]byte("# Hello\n\nWorld"), 200, 1000)
	f.Add([]byte("```\ncode\n```"), 400, 2000)
	f.Add([]byte("- list\n- items"), 300, 1500)
	f.Add([]byte("> quote\n> block"), 500, 2000)
	f.Add([]byte("| A | B |\n|---|---|\n| C | D |"), 400, 1200)
	f.Add([]byte("This is a very long text block that should trigger word wrapping multiple times to test the line breaking logic when strings exceed the maximum width allowed by the canvas."), 150, 1500)
	f.Add([]byte("![img](http://example.com/a.png)"), 200, 1000) // remote parsing mock
	f.Add([]byte("[link](http://example.com)"), 200, 1000)

	// deeply nested
	f.Add([]byte(">>>>>>>>>> deeply nested quote"), 300, 3000)
	f.Add([]byte("- - - - - - deeply nested list"), 300, 3000)

	fonts, err := LoadFonts(FontConfig{SizeBase: 16})

	if err != nil {
		f.Fatalf("failed to load fonts: %v", err)
	}
	f.Fuzz(func(t *testing.T, data []byte, width int, maxHeight int) {
		// Cap inputs to reasonable bounds to avoid fuzzing hanging
		if width < 10 || width > 2000 {
			width = 400
		}
		if maxHeight < 100 || maxHeight > 4000 {
			maxHeight = 2000
		}

		opts := RenderOptions{
			Width:        width,
			BaseFontSize: 16,
			Margin:       10,
			Theme:        lightTheme,
			Fonts:        fonts,
			MaxHeight:    maxHeight,
			ImagePolicy: &ImagePolicy{
				AllowLocal:  false,
				AllowRemote: false,
			},
		}

		img, err := Render(data, opts)

		if err == nil {
			if img == nil {
				t.Fatalf("Render returned nil image and nil error")
			}
			bounds := img.Bounds()
			if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
				t.Fatalf("Render returned image with invalid bounds: %v", bounds)
			}
			if bounds.Dy() > maxHeight {
				t.Fatalf("Render returned image exceeding max height (%d > %d)", bounds.Dy(), maxHeight)
			}
		}

		// Run a second time to ensure deterministic error/success behaviour
		img2, err2 := Render(data, opts)
		if (err == nil) != (err2 == nil) {
			t.Fatalf("Deterministic behaviour failure: err1=%v, err2=%v", err, err2)
		}
		if err != nil && err2 != nil && err.Error() != err2.Error() {
			t.Fatalf("Deterministic behaviour failure on error: %v != %v", err, err2)
		}
		if img != nil && img2 != nil {
			if img.Bounds() != img2.Bounds() {
				t.Fatalf("Deterministic behaviour failure on bounds: %v != %v", img.Bounds(), img2.Bounds())
			}
		}
	})
}
