package md2png

// ConvertCommandArgsToRenderOptions tests the logic for mapping pointer arguments
// from go-subcommand into a RenderOptions struct, exactly as Md2png/Md2view do.
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
) (RenderOptions, error) {
	themeName := "light"
	if theme != nil {
		themeName = *theme
	}
	th, err := ThemeByName(themeName)
	if err != nil {
		return RenderOptions{}, err
	}

	cfg := FontConfig{}
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

	fonts, err := LoadFonts(cfg)
	if err != nil {
		return RenderOptions{}, err
	}

	opts := RenderOptions{
		Theme:   th,
		Fonts:   fonts,
		BaseDir: baseDir,
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
