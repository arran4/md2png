package cli

import (
	"github.com/arran4/md2png"
)

// ConvertCommandArgsToRenderOptions tests the logic for mapping pointer arguments
// from go-subcommand into a md2png.RenderOptions struct, exactly as Md2png/Md2view do.
func ConvertCommandArgsToRenderOptions(
	width *int,
	margin *int,
	pt *float64,
	theme *string,
	fontRegular *string,
	fontBold *string,
	fontMono *string,
	footnoteLinks *bool,
	footnoteImages *bool,
	maxHeight *int,
	baseDir string,
) (md2png.RenderOptions, error) {
	themeName := "light"
	if theme != nil {
		themeName = *theme
	}
	th, err := md2png.ThemeByName(themeName)
	if err != nil {
		return md2png.RenderOptions{}, err
	}

	cfg := md2png.FontConfig{}
	if fontRegular != nil {
		cfg.RegularPath = *fontRegular
	}
	if fontBold != nil {
		cfg.BoldPath = *fontBold
	}
	if fontMono != nil {
		cfg.MonoPath = *fontMono
	}
	if pt != nil {
		cfg.SizeBase = *pt
	}

	fonts, err := md2png.LoadFonts(cfg)
	if err != nil {
		return md2png.RenderOptions{}, err
	}

	policy := md2png.DefaultCLIImagePolicy()
	opts := md2png.RenderOptions{
		Theme:       th,
		Fonts:       fonts,
		BaseDir:     baseDir,
		ImagePolicy: &policy,
	}

	if width != nil {
		opts.Width = *width
	}
	if margin != nil {
		opts.Margin = *margin
		opts.ZeroMargin = *margin == 0
	}
	if pt != nil {
		opts.BaseFontSize = *pt
	}

	if footnoteLinks != nil {
		opts.LinkFootnotes = footnoteLinks
	} else {
		t := true
		opts.LinkFootnotes = &t
	}
	if footnoteImages != nil {
		opts.ImageFootnotes = footnoteImages
	} else {
		f := false
		opts.ImageFootnotes = &f
	}
	if maxHeight != nil {
		opts.MaxHeight = *maxHeight
	}

	return opts, nil
}
