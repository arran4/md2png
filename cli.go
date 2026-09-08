package md2png

import (
	"flag"
)

// CLIFlags holds pointers to the parsed command-line flags.
type CLIFlags struct {
	Width          *int
	Margin         *int
	Pt             *float64
	Theme          *string
	FontRegular    *string
	FontBold       *string
	FontMono       *string
	FootnoteLinks  *bool
	FootnoteImages *bool
	MaxHeight      *int
}

// RegisterFlags registers the standard RenderOptions flags on the given FlagSet.
func RegisterFlags(fs *flag.FlagSet) *CLIFlags {
	return &CLIFlags{
		Width:          fs.Int("width", 0, "Output image width in pixels (0 for default)"),
		Margin:         fs.Int("margin", 0, "Margin in pixels (0 for default)"),
		Pt:             fs.Float64("pt", 0, "Base font size in points (paragraph) (0 for default)"),
		Theme:          fs.String("theme", "light", "Theme: light|dark"),
		FontRegular:    fs.String("font", "", "Path to TTF for regular text (optional; default Go Regular)"),
		FontBold:       fs.String("fontbold", "", "Path to TTF for bold text (optional; default Go Bold)"),
		FontMono:       fs.String("fontmono", "", "Path to TTF for mono/code (optional; default Go Mono)"),
		FootnoteLinks:  fs.Bool("footnote-links", true, "Add footnotes for link destinations"),
		FootnoteImages: fs.Bool("footnote-images", false, "Add footnotes for image destinations"),
		MaxHeight:      fs.Int("max-height", 0, "Maximum output height in pixels (0 for default)"),
	}
}

// ToRenderOptions converts parsed CLIFlags into a RenderOptions struct.
// It uses fs.Visit to detect intentional zero values (e.g. margin=0).
// baseDir should be passed explicitly.
func (f *CLIFlags) ToRenderOptions(fs *flag.FlagSet, baseDir string) (RenderOptions, error) {
	th, err := ThemeByName(*f.Theme)
	if err != nil {
		return RenderOptions{}, err
	}

	fonts, err := LoadFonts(FontConfig{
		RegularPath: *f.FontRegular,
		BoldPath:    *f.FontBold,
		MonoPath:    *f.FontMono,
		SizeBase:    *f.Pt,
	})
	if err != nil {
		return RenderOptions{}, err
	}

	marginSet := false
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "margin" {
			marginSet = true
		}
	})

	return RenderOptions{
		Width:          *f.Width,
		Margin:         *f.Margin,
		ZeroMargin:     *f.Margin == 0 && marginSet,
		BaseFontSize:   *f.Pt,
		Theme:          th,
		Fonts:          fonts,
		LinkFootnotes:  f.FootnoteLinks,
		ImageFootnotes: f.FootnoteImages,
		BaseDir:        baseDir,
		MaxHeight:      *f.MaxHeight,
	}, nil
}
