# md2png Agent Skill

This skill provides operational guidance for AI coding agents on how to use `md2png` and `md2view` effectively.

## Core Commands

`md2png` transforms Markdown into raster images. It does not use Node or headless browsers.

* **Render to PNG:** `md2png -in input.md -out output.png`
* **Render to JPG:** `md2png -in input.md -out output.jpg`
* **Render to GIF:** `md2png -in input.md -out output.gif`

If `-in` is omitted or empty, it reads from `stdin`. E.g., `echo "# Hello" | md2png -out out.png`.
The output format is determined by the `-out` extension.

`md2view` is the GUI viewer for Markdown with the exact same rendering engine.
* **View Markdown:** `md2view <file.md>`
* It accepts the same flags as `md2png` (e.g., `-theme`, `-width`), but doesn't write an image file.

## Operational Quirks & Pitfalls

1. **Wait for output:** Rendering is fast, but it's not instantaneous. Ensure the command exits with `0` before assuming the image is ready.
2. **Missing Input:** If `md2png` or `md2view` blocks unexpectedly, you likely forgot `-in` or the positional argument, and it is waiting for `stdin`.
3. **Paths:** Ensure `-in` and `-out` paths exist. `md2png` does not create parent directories for `-out`.
4. **Themes:** Supports `-theme light` (default) and `-theme dark`. Use `-theme dark` for terminal-like aesthetics.
5. **Image Width:** Use `-width 1024` (default) or set higher (e.g., `1920`) for 1080p outputs.
6. **Margin & Font:** Use `-margin` (default 48) and `-pt` (default 16) to tweak the layout.
7. **No HTML:** The parser is strictly Markdown. Inline HTML is mostly ignored or rendered as raw text.

## Validating Changes

If you modify the project's source code, you **must** verify the changes.
The `AGENTS.md` explicitly requires running:

```bash
go run . -in README.md -out output.png
```

Check `output.png` to confirm the rendering looks correct.
For `md2view`, you might not be able to easily verify the GUI in a headless environment, so test the rendering logic via `md2png`.

## Code Architecture

* `github.com/yuin/goldmark`: Parses Markdown to an AST.
* `golang.org/x/image/draw`: Rasterizes the output.
* `freetype`: Used for font rendering.

Do not attempt to add headless Chrome or Puppeteer. This is a strict pure-Go rasterizer.
