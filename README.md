# md2png – Markdown to Image (Go CLI & Library)

`md2png` renders Markdown into PNG, JPG, or GIF files using only Go. It ships as a CLI and as a library so you can call it from your own code. No Node, headless browsers, or helper scripts.

---

## What it does

- Parses Markdown with `goldmark` and draws the result straight to an image buffer.
- Handles headings (H1–H5), paragraphs, ordered and unordered lists, bold text, code blocks, block quotes, tables, and horizontal rules.
- Dark and light themes, adjustable width, margin, and point size.
- Optional custom fonts: `--font`, `--fontbold`, `--fontmono`.
- Output format follows the `-out` extension.

---

## Install

Clone and build:

```bash
git clone https://github.com/arran4/md2png.git
cd md2png
go build ./cmd/md2png
```

Dependencies are pure Go packages:

```bash
go get github.com/yuin/goldmark@v1.7.4 \
       github.com/golang/freetype@v0.0.0-20170609003504-e2365dfdc4a0 \
       golang.org/x/image@latest
```

Requires Go 1.22 or newer.

---

## CLI usage

```bash
./md2png -in README.md -out out.png
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-in` | Markdown input file, or stdin when empty | — |
| `-out` | Output image (`.png`, `.jpg`, `.gif`) | `out.png` |
| `-width` | Image width in pixels | 1024 |
| `-margin` | Margin in pixels | 48 |
| `-pt` | Base font size (points) | 16 |
| `-theme` | `light` or `dark` | `light` |
| `-font` | Regular font TTF path | built-in Go Regular |
| `-fontbold` | Bold font TTF path | built-in Go Bold |
| `-fontmono` | Monospace font TTF path | built-in Go Mono |
| `-footnote-links` | Emit link targets as numbered footnotes | `true` |
| `-footnote-images` | Emit image targets as numbered footnotes | `false` |

### Examples

Render Markdown from disk:

```bash
./md2png -in example.md -out example.png
```

Dark theme, wider frame, larger type:

```bash
./md2png -in blogpost.md -out post.png -theme dark -width 1400 -pt 18
```

Produce an animated GIF (palette handled for you):

```bash
./md2png -in slides.md -out slides.gif
```

Use your own fonts:

```bash
./md2png -in notes.md -out notes.jpg \
  -font /usr/share/fonts/TTF/DejaVuSans.ttf \
  -fontmono /usr/share/fonts/TTF/DejaVuSansMono.ttf
```

From stdin:

```bash
echo "# Hello\nThis came from stdin!" | ./md2png -out hello.png
```

---

## Library usage

```go
package main

import (
        "image/png"
        "os"

        "github.com/arran4/md2png"
)

func main() {
        img, err := md2png.Render([]byte("# Hello\nRendered inside Go!"), md2png.RenderOptions{})
        if err != nil {
                panic(err)
        }

        f, err := os.Create("hello.png")
        if err != nil {
                panic(err)
        }
        defer f.Close()

        if err := png.Encode(f, img); err != nil {
                panic(err)
        }
}
```

`RenderOptions` exposes the same knobs as the CLI. Set custom dimensions, swap themes, toggle link or image footnotes, or pass a font set created with `md2png.LoadFonts`.

---

## Output

Light theme:

![Light example](examples/light-example.png)

Dark theme:

![Dark example](examples/dark-example.png)

---

## How it works

1. Parse Markdown with [`yuin/goldmark`](https://github.com/yuin/goldmark).
2. Walk the AST and draw elements onto an RGBA image with [`freetype`](https://pkg.go.dev/github.com/golang/freetype).
3. Wrap text, handle indentation, block quotes, code blocks, and tables.
4. Encode the result as PNG, JPEG, or GIF based on the `-out` extension.

Everything happens in memory; there is no HTML renderer or external process.

---

## Roadmap

- [x] Tables
- [x] Inline images
- [ ] Syntax highlighting
- [ ] SVG output
- [ ] Configurable themes via YAML/JSON

---

## License

`md2png` is available under the [MIT License](LICENSE).
