package md2png

import (
	"strings"
	"testing"
)

func TestAuthoredFootnotes_BasicAndNamed(t *testing.T) {
	md := `# Notes Test

Here is a statement with a numeric footnote[^1] and a named footnote[^custom].

[^1]: This is the numeric footnote definition.
[^custom]: This is the named footnote definition with **bold** text and ` + "`code`" + `.
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("unexpected error rendering authored footnotes: %v", err)
	}
	if img == nil || img.Bounds().Dy() == 0 {
		t.Fatalf("expected non-empty image")
	}
}

func TestAuthoredFootnotes_RepeatedReferences(t *testing.T) {
	md := `First reference[^1], second reference to same note[^1], and third[^2].

[^1]: Definition of note 1.
[^2]: Definition of note 2.
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestAuthoredFootnotes_MultilineAndFormatting(t *testing.T) {
	md := `Document body with footnote[^complex].

[^complex]: First paragraph of complex footnote.

    Second paragraph with [a link](https://example.com) and *italic* styling.

    * Footnote list item A
    * Footnote list item B

` + "```" + `
code in footnote
` + "```" + `
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("failed rendering multiline footnote: %v", err)
	}
	if img.Bounds().Dy() < 100 {
		t.Fatalf("expected significant height for multiline footnote, got %d", img.Bounds().Dy())
	}
}

func TestAuthoredFootnotes_MissingAndUnreferenced(t *testing.T) {
	// Missing definition: [^missing] has no definition. Unreferenced definition: [^unused] is never referenced.
	md := `Text with missing footnote reference[^missing].

[^unused]: Unused footnote definition.
`
	img, err := Render([]byte(md), RenderOptions{
		Width: 800,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatalf("expected rendered image")
	}
}

func TestAuthoredFootnotes_CoexistenceWithLinkFootnotes(t *testing.T) {
	md := `Text with an [example link](https://golang.org) and an authored note[^1].

[^1]: Authored note content.
`
	yes := true
	imgWithLinkFootnotes, err := Render([]byte(md), RenderOptions{
		Width:         800,
		LinkFootnotes: &yes,
	})
	if err != nil {
		t.Fatalf("render with link footnotes failed: %v", err)
	}

	no := false
	imgWithoutLinkFootnotes, err := Render([]byte(md), RenderOptions{
		Width:         800,
		LinkFootnotes: &no,
	})
	if err != nil {
		t.Fatalf("render without link footnotes failed: %v", err)
	}

	// LinkFootnotes: true should draw both the authored footnote [1] AND link footnote [2] https://golang.org
	// So imgWithLinkFootnotes should be taller than imgWithoutLinkFootnotes.
	if imgWithLinkFootnotes.Bounds().Dy() <= imgWithoutLinkFootnotes.Bounds().Dy() {
		t.Fatalf("expected imgWithLinkFootnotes (%d) to be taller than imgWithoutLinkFootnotes (%d)",
			imgWithLinkFootnotes.Bounds().Dy(), imgWithoutLinkFootnotes.Bounds().Dy())
	}
}

func TestAuthoredFootnotes_LinksInsideFootnoteDefinitions(t *testing.T) {
	md := `Text with authored footnote[^note].

[^note]: Footnote containing [external site](https://example.com/site).
`
	yes := true
	img, err := Render([]byte(md), RenderOptions{
		Width:         800,
		LinkFootnotes: &yes,
	})
	if err != nil {
		t.Fatalf("failed to render footnote containing link: %v", err)
	}
	if img == nil {
		t.Fatalf("expected valid image")
	}
}

func TestAuthoredFootnotes_ResourceLimitsParticipation(t *testing.T) {
	longFootnote := strings.Repeat("Long footnote line that wraps across multiple lines and expands the canvas height significantly.\n\n", 50)
	md := `A short paragraph[^long].

[^long]: ` + longFootnote

	// Render with a small MaxHeight should trigger MaxHeight exceeded error
	_, err := Render([]byte(md), RenderOptions{
		Width:     800,
		MaxHeight: 200,
	})
	if err == nil {
		t.Fatalf("expected MaxHeight error for massive footnote exceeding canvas limit")
	}
	if !strings.Contains(err.Error(), "maximum output height limit exceeded") {
		t.Fatalf("expected height limit error, got: %v", err)
	}
}

func TestAuthoredFootnotes_Deterministic(t *testing.T) {
	md := `# Title

Body referencing note A[^a] and note B[^b].

[^a]: Definition A.
[^b]: Definition B.
`
	img1, err := Render([]byte(md), RenderOptions{Width: 600})
	if err != nil {
		t.Fatalf("img1 failed: %v", err)
	}
	img2, err := Render([]byte(md), RenderOptions{Width: 600})
	if err != nil {
		t.Fatalf("img2 failed: %v", err)
	}

	b1 := img1.Bounds()
	b2 := img2.Bounds()
	if b1 != b2 {
		t.Fatalf("bounds mismatch: %v vs %v", b1, b2)
	}
	rgba1 := img1
	rgba2 := img2
	for i := range rgba1.Pix {
		if rgba1.Pix[i] != rgba2.Pix[i] {
			t.Fatalf("pixel mismatch at index %d", i)
		}
	}
}
