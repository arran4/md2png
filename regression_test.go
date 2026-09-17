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
		os.WriteFile(actualPath, actualBytes, 0644)
		t.Errorf("Image mismatch for %s. Actual image written to %s", expectedPath, actualPath)
	}
}

func getDeterministicOptions() RenderOptions {
	fonts, _ := LoadFonts(FontConfig{SizeBase: 16})
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
			opts := getDeterministicOptions()
			opts.LinkFootnotes = &yes
			opts.ImageFootnotes = &yes
			return &opts
		}()},
		{"unsupported_nodes", "<div>Raw HTML is unsupported natively</div>", nil},
		{"dark_theme", "# Dark Theme\n\nThis is dark theme.", func() *RenderOptions {
			opts := getDeterministicOptions()
			opts.Theme = darkTheme
			return &opts
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := getDeterministicOptions()
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
	f.Add([]byte("# Hello\n\nWorld"))
	f.Add([]byte("```\ncode\n```"))
	f.Add([]byte("- list\n- items"))
	f.Add([]byte("> quote\n> block"))
	f.Add([]byte("| A | B |\n|---|---|\n| C | D |"))
	f.Add([]byte("This is a very long text block that should trigger word wrapping multiple times to test the line breaking logic when strings exceed the maximum width allowed by the canvas."))

	f.Fuzz(func(t *testing.T, data []byte) {
		opts := getDeterministicOptions()
		opts.Width = 200
		opts.MaxHeight = 1000
		opts.ImagePolicy = &ImagePolicy{
			AllowLocal:  false,
			AllowRemote: false,
		}
		_, _ = Render(data, opts)
	})
}
