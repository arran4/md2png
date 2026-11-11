package cli

import (
	"fmt"
	"strings"

	"github.com/arran4/md2png"
)

// CommandRenderArgs holds optional go-subcommand rendering flags. A nil pointer
// means that flag was absent; explicit false and zero values remain distinct.
// This is the internal CLI adapter input, not the public md2png.RenderOptions.
type CommandRenderArgs struct {
	Strict *bool
	Width *int
	Margin *int
	Pt *float64
	Theme *string
	ThemeFile *string
	Transparent *bool
	FontRegular *string
	FontBold *string
	FontMono *string
	FontItalic *string
	FontBoldItalic *string
	FontFallback *string
	FootnoteLinks *bool
	FootnoteImages *bool
	MaxHeight *int
	TableFitWidth *bool
	DisableHighlighting *bool
	CodeTabWidth *int
	FootnoteOrder *string
	BaseDir string
}

// ConvertCommandArgsToRenderOptions translates optional command input to the
// renderer's typed options. The type-switch currently recognizes one option
// type, CommandRenderArgs, and is intentionally extensible for future typed
// options or a go-subcommand-generated CommandRenderArgs.
//
// Zero arguments mean defaults. Repeating CommandRenderArgs replaces the
// previous complete configuration (last value wins); unknown types, including
// nil and typed-nil pointers, are rejected rather than silently ignored.
func ConvertCommandArgsToRenderOptions(options ...any) (md2png.RenderOptions, error) {
	var args CommandRenderArgs
	for _, option := range options {
		switch v := option.(type) {
		case CommandRenderArgs:
			args = v
		default:
			return md2png.RenderOptions{}, fmt.Errorf("unsupported command render option %T", option)
		}
	}
	return convertCommandRenderArgs(args)
}

// convertCommandRenderArgs contains the existing CLI-specific defaulting,
// validation and font/theme materialization logic in one place.
func convertCommandRenderArgs(a CommandRenderArgs) (md2png.RenderOptions, error) {
	var th md2png.Theme
	if a.ThemeFile != nil && *a.ThemeFile != "" {
		loaded, err := md2png.LoadThemeFile(*a.ThemeFile)
		if err != nil {
			return md2png.RenderOptions{}, err
		}
		th = loaded
	} else {
		themeName := "light"
		if a.Theme != nil {
			themeName = *a.Theme
		}
		loaded, err := md2png.ThemeByName(themeName)
		if err != nil {
			return md2png.RenderOptions{}, err
		}
		th = loaded
	}

	cfg := md2png.FontConfig{}
	if a.FontRegular != nil {
		cfg.RegularPath = *a.FontRegular
	}
	if a.FontBold != nil {
		cfg.BoldPath = *a.FontBold
	}
	if a.FontMono != nil {
		cfg.MonoPath = *a.FontMono
	}
	if a.FontItalic != nil {
		cfg.ItalicPath = *a.FontItalic
	}
	if a.FontBoldItalic != nil {
		cfg.BoldItalicPath = *a.FontBoldItalic
	}
	if a.FontFallback != nil && *a.FontFallback != "" {
		for _, p := range strings.Split(*a.FontFallback, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cfg.FallbackPaths = append(cfg.FallbackPaths, trimmed)
			}
		}
	}
	if a.Pt != nil {
		cfg.SizeBase = *a.Pt
	}

	fonts, err := md2png.LoadFonts(cfg)
	if err != nil {
		return md2png.RenderOptions{}, err
	}

	policy := md2png.DefaultCLIImagePolicy()

	diagPolicy := md2png.DiagnosticPolicy{}
	if a.Strict != nil && *a.Strict {
		diagPolicy.FailOnImageError = true
		diagPolicy.FailOnRawHTML = true
		diagPolicy.FailOnUnsupported = true
	}

	opts := md2png.RenderOptions{
		Theme:            th,
		Fonts:            fonts,
		BaseDir:          a.BaseDir,
		ImagePolicy:      &policy,
		DiagnosticPolicy: &diagPolicy,
	}

	if a.Transparent != nil && *a.Transparent {
		opts.TransparentBackground = true
	}

	if a.Width != nil {
		opts.Width = *a.Width
	}
	if a.Margin != nil {
		opts.Margin = *a.Margin
		opts.ZeroMargin = *a.Margin == 0
	}
	if a.Pt != nil {
		opts.BaseFontSize = *a.Pt
	}

	if a.FootnoteLinks != nil {
		opts.LinkFootnotes = a.FootnoteLinks
	} else {
		t := true
		opts.LinkFootnotes = &t
	}
	if a.FootnoteImages != nil {
		opts.ImageFootnotes = a.FootnoteImages
	} else {
		f := false
		opts.ImageFootnotes = &f
	}
	if a.MaxHeight != nil {
		opts.MaxHeight = *a.MaxHeight
	}
	if a.TableFitWidth != nil {
		opts.TableFitWidth = *a.TableFitWidth
	}
	if a.DisableHighlighting != nil {
		opts.DisableHighlighting = *a.DisableHighlighting
	}
	if a.CodeTabWidth != nil && *a.CodeTabWidth > 0 {
		opts.CodeTabWidth = *a.CodeTabWidth
	} else {
		opts.CodeTabWidth = 4
	}

	if a.FootnoteOrder != nil && *a.FootnoteOrder != "" {
		switch strings.ToLower(*a.FootnoteOrder) {
		case "authored-first", "authored":
			opts.FootnoteOrder = md2png.FootnoteOrderAuthoredFirst
		case "links-first", "urls-first", "links":
			opts.FootnoteOrder = md2png.FootnoteOrderLinksFirst
		default:
			return opts, fmt.Errorf("unknown footnote order %q, expected 'authored-first' or 'links-first'", *a.FootnoteOrder)
		}
	}

	return opts, nil
}
