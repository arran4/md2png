package md2png

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var writeVisualSamples = flag.Bool("write-visual-samples", false, "write SVG and PNG samples to testdata/visual/generated")

// TestSVGVisualSamples uses one txtar file as the source of truth for several
// comparable SVG/PNG examples. The optional flag exports real renderer outputs
// for reviewers; the default test never writes files.
func TestSVGVisualSamples(t *testing.T) {
	data, err := os.ReadFile("testdata/visual/svg-cases.txtar")
	if err != nil {
		t.Fatal(err)
	}
	header := regexp.MustCompile(`(?m)^-- ([a-z-]+\.md) --\r?\n`)
	matches := header.FindAllStringSubmatchIndex(string(data), -1)
	if len(matches) < 2 {
		t.Fatalf("expected multiple Markdown samples in txtar, got %d", len(matches))
	}

	for i, match := range matches {
		name := string(data[match[2]:match[3]])
		start, end := match[1], len(data)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		source := data[start:end]
		t.Run(name, func(t *testing.T) {
			opts := RenderOptions{Width: 780, BaseDir: "testdata"}
			svgData, err := RenderSVG(source, opts)
			if err != nil {
				t.Fatalf("RenderSVG: %v", err)
			}
			raster, err := Render(source, opts)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			counts := map[string]int{}
			var svgWidth, svgHeight, viewWidth, viewHeight int
			decoder := xml.NewDecoder(bytes.NewReader(svgData))
			for {
				tok, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("SVG XML: %v", err)
				}
				start, ok := tok.(xml.StartElement)
				if !ok {
					continue
				}
				counts[start.Name.Local]++
				if start.Name.Local == "svg" {
					for _, attr := range start.Attr {
						switch attr.Name.Local {
						case "width":
							svgWidth, err = strconv.Atoi(attr.Value)
						case "height":
							svgHeight, err = strconv.Atoi(attr.Value)
						case "viewBox":
							_, err = fmt.Sscanf(attr.Value, "0 0 %d %d", &viewWidth, &viewHeight)
						}
						if err != nil {
							t.Fatalf("SVG %s attribute: %v", attr.Name.Local, err)
						}
					}
				}
			}
			if counts["svg"] != 1 || counts["text"] == 0 || counts["rect"] == 0 {
				t.Errorf("missing expected SVG structure: %#v", counts)
			}
			if name == "showcase.md" && (counts["line"] == 0 || counts["image"] == 0) {
				t.Errorf("showcase needs geometric checkbox lines and real embedded image: %#v", counts)
			}
			w, h := raster.Bounds().Dx(), raster.Bounds().Dy()
			if svgWidth != w || svgHeight != h || viewWidth != w || viewHeight != h {
				t.Errorf("SVG dimensions %dx%d and viewBox %dx%d differ from raster %dx%d",
					svgWidth, svgHeight, viewWidth, viewHeight, w, h)
			}
			hash := sha256.Sum256(svgData)
			t.Logf("input=%s width=%d height=%d text=%d rect=%d line=%d image=%d svgBytes=%d svgSHA256=%x",
				name, w, h, counts["text"], counts["rect"], counts["line"], counts["image"], len(svgData), hash)

			if *writeVisualSamples {
				dir := filepath.Join("testdata", "visual", "generated")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				stem := strings.TrimSuffix(name, ".md")
				if err := os.WriteFile(filepath.Join(dir, stem+".svg"), svgData, 0o644); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(dir, stem+".png"))
				if err != nil {
					t.Fatal(err)
				}
				encodeErr := png.Encode(file, raster)
				closeErr := file.Close()
				if encodeErr != nil {
					t.Fatal(encodeErr)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		})
	}
}
