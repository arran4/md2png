package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/arran4/md2png"
)

// Batch is a subcommand `md2png batch` that batch renders Markdown files or directories to images
//
// Flags:
//
//	outDir: --out-dir -o Output directory for rendered images (default: "dist")
//	concurrency: --concurrency -c Maximum number of concurrent renders (default: 4)
//	format: --format Output format: png, jpeg, or gif (default: "png")
//	continueOnError: --continue-on-error Continue batch rendering even if individual files fail (default: false)
//	dryRun: --dry-run List planned input and output mappings without rendering
//	width: (from parent) Output image width in pixels (0 for default)
//	margin: (from parent) Margin in pixels (0 for default)
//	pt: (from parent) Base font size in points (paragraph) (0 for default)
//	theme: (from parent) Theme: light|dark (default: "light")
//	fontRegular: (from parent) Path to TTF for regular text (optional; default Go Regular)
//	fontBold: (from parent) Path to TTF for bold text (optional; default Go Bold)
//	fontMono: (from parent) Path to TTF for mono/code (optional; default Go Mono)
//	fontItalic: (from parent) Path to TTF for italic text (optional; default Go Italic)
//	fontBoldItalic: (from parent) Path to TTF for bold italic text (optional; default Go Bold Italic)
//	fontFallback: (from parent) Comma separated paths to fallback TTF/OTF fonts
//	footnoteLinks: (from parent) Add footnotes for link destinations (default: true)
//	footnoteImages: (from parent) Add footnotes for image destinations (default: false)
//	maxHeight: (from parent) Maximum output height in pixels (0 for default)
//	strict: (from parent) Enable strict rendering (fail on warnings)
//	tableFitWidth: (from parent) Expand canvas width to fit tables
//	disableHighlighting: (from parent) Disable syntax highlighting
//	codeTabWidth: (from parent) Tab width in code blocks (default: 4)
//	inputs: ... Input Markdown files and/or directories
func Batch(
	outDir *string,
	concurrency *int,
	format *string,
	continueOnError *bool,
	dryRun *bool,
	width *int,
	margin *int,
	pt *float64,
	theme *string,
	fontRegular *string,
	fontBold *string,
	fontMono *string,
	fontItalic *string,
	fontBoldItalic *string,
	fontFallback *string,
	footnoteLinks *bool,
	footnoteImages *bool,
	maxHeight *int,
	strict *bool,
	tableFitWidth *bool,
	disableHighlighting *bool,
	codeTabWidth *int,
	inputs ...string,
) error {
	return RunBatch(os.Stdout, os.Stderr, outDir, concurrency, format, continueOnError, dryRun,
		width, margin, pt, theme, fontRegular, fontBold, fontMono, fontItalic, fontBoldItalic, fontFallback, footnoteLinks, footnoteImages,
		maxHeight, strict, tableFitWidth, disableHighlighting, codeTabWidth, inputs...)
}

// BatchTask represents a single Markdown file render task.
type BatchTask struct {
	InputPath  string
	OutputPath string
	BaseDir    string
}

// RunBatch executes the batch rendering with injected stdout and stderr writers.
func RunBatch(
	stdout io.Writer,
	stderr io.Writer,
	outDir *string,
	concurrency *int,
	format *string,
	continueOnError *bool,
	dryRun *bool,
	width *int,
	margin *int,
	pt *float64,
	theme *string,
	fontRegular *string,
	fontBold *string,
	fontMono *string,
	fontItalic *string,
	fontBoldItalic *string,
	fontFallback *string,
	footnoteLinks *bool,
	footnoteImages *bool,
	maxHeight *int,
	strict *bool,
	tableFitWidth *bool,
	disableHighlighting *bool,
	codeTabWidth *int,
	inputs ...string,
) error {
	if len(inputs) == 0 {
		return errors.New("md2png batch: at least one input file or directory is required")
	}

	targetDir := "dist"
	if outDir != nil && *outDir != "" {
		targetDir = *outDir
	}

	resolvedFormat := "png"
	if format != nil && *format != "" {
		norm := strings.ToLower(strings.TrimPrefix(*format, "."))
		switch norm {
		case "png", "jpg", "jpeg", "gif":
			if norm == "jpg" {
				norm = "jpeg"
			}
			resolvedFormat = norm
		default:
			return fmt.Errorf("md2png batch: unsupported format %q (expected png, jpeg, or gif)", *format)
		}
	}

	ext := "." + resolvedFormat
	if resolvedFormat == "jpeg" {
		ext = ".jpg"
	}

	tasks, err := discoverBatchTasks(inputs, targetDir, ext)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return errors.New("md2png batch: no Markdown files found in specified inputs")
	}

	isDryRun := dryRun != nil && *dryRun
	if isDryRun {
		for _, task := range tasks {
			if _, err := fmt.Fprintf(stdout, "%s -> %s\n", task.InputPath, task.OutputPath); err != nil {
				return fmt.Errorf("md2png batch: write dry-run output: %w", err)
			}
		}
		return nil
	}

	// Validate render options once upfront before starting workers
	if _, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{
		Strict: strict,
		Width: width,
		Margin: margin,
		Pt: pt,
		Theme: theme,
		FontRegular: fontRegular,
		FontBold: fontBold,
		FontMono: fontMono,
		FontItalic: fontItalic,
		FontBoldItalic: fontBoldItalic,
		FontFallback: fontFallback,
		FootnoteLinks: footnoteLinks,
		FootnoteImages: footnoteImages,
		MaxHeight: maxHeight,
		TableFitWidth: tableFitWidth,
		DisableHighlighting: disableHighlighting,
		CodeTabWidth: codeTabWidth,
	}); err != nil {
		return fmt.Errorf("md2png batch: option error: %w", err)
	}

	numWorkers := 4
	if concurrency != nil && *concurrency > 0 {
		numWorkers = *concurrency
	}
	if numWorkers > 32 {
		numWorkers = 32
	}
	if numWorkers > len(tasks) {
		numWorkers = len(tasks)
	}

	shouldContinue := continueOnError != nil && *continueOnError

	taskCh := make(chan BatchTask, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var taskErrors []string
	var abort bool

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerOpts, err := ConvertCommandArgsToRenderOptions(CommandRenderArgs{
				Strict: strict,
				Width: width,
				Margin: margin,
				Pt: pt,
				Theme: theme,
				FontRegular: fontRegular,
				FontBold: fontBold,
				FontMono: fontMono,
				FontItalic: fontItalic,
				FontBoldItalic: fontBoldItalic,
				FontFallback: fontFallback,
				FootnoteLinks: footnoteLinks,
				FootnoteImages: footnoteImages,
				MaxHeight: maxHeight,
				TableFitWidth: tableFitWidth,
				DisableHighlighting: disableHighlighting,
				CodeTabWidth: codeTabWidth,
			})
			if err != nil {
				errMu.Lock()
				taskErrors = append(taskErrors, fmt.Sprintf("options: %v", err))
				abort = true
				errMu.Unlock()
				return
			}
			for task := range taskCh {
				errMu.Lock()
				if abort && !shouldContinue {
					errMu.Unlock()
					return
				}
				errMu.Unlock()

				taskErr := renderBatchTask(task, workerOpts, resolvedFormat, stderr)
				if taskErr != nil {
					errMu.Lock()
					taskErrors = append(taskErrors, fmt.Sprintf("%s: %v", task.InputPath, taskErr))
					if !shouldContinue {
						abort = true
					}
					errMu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if len(taskErrors) > 0 {
		if _, err := fmt.Fprintf(stderr, "md2png batch: %d of %d files failed\n", len(taskErrors), len(tasks)); err != nil {
			return fmt.Errorf("md2png batch: write failure report: %w", err)
		}
		for _, e := range taskErrors {
			if _, err := fmt.Fprintf(stderr, "  - %s\n", e); err != nil {
				return fmt.Errorf("md2png batch: write failure detail: %w", err)
			}
		}
		return fmt.Errorf("md2png batch: %d render failure(s)", len(taskErrors))
	}

	return nil
}

func discoverBatchTasks(inputs []string, outDir string, ext string) ([]BatchTask, error) {
	var tasks []BatchTask
	seenOutputs := make(map[string]string) // outputPath -> inputPath

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("md2png batch: cannot access %s: %w", input, err)
		}

		if !info.IsDir() {
			absIn, err := filepath.Abs(input)
			if err != nil {
				return nil, err
			}
			baseName := filepath.Base(input)
			nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
			outPath := filepath.Join(outDir, nameWithoutExt+ext)
			cleanOut := filepath.Clean(outPath)

			if orig, exists := seenOutputs[cleanOut]; exists {
				return nil, fmt.Errorf("md2png batch: output collision for %s (from %s and %s)", cleanOut, orig, input)
			}
			seenOutputs[cleanOut] = input

			tasks = append(tasks, BatchTask{
				InputPath:  input,
				OutputPath: cleanOut,
				BaseDir:    filepath.Dir(absIn),
			})
			continue
		}

		// Directory input: recursively walk
		absInputDir, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}

		err = filepath.Walk(input, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if f.IsDir() {
				return nil
			}
			lower := strings.ToLower(f.Name())
			if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
				return nil
			}

			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(absInputDir, absPath)
			if err != nil {
				return err
			}

			relWithoutExt := strings.TrimSuffix(rel, filepath.Ext(rel))
			outPath := filepath.Join(outDir, relWithoutExt+ext)
			cleanOut := filepath.Clean(outPath)

			if orig, exists := seenOutputs[cleanOut]; exists {
				return fmt.Errorf("md2png batch: output collision for %s (from %s and %s)", cleanOut, orig, path)
			}
			seenOutputs[cleanOut] = path

			tasks = append(tasks, BatchTask{
				InputPath:  path,
				OutputPath: cleanOut,
				BaseDir:    filepath.Dir(absPath),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("md2png batch: failed walking %s: %w", input, err)
		}
	}

	return tasks, nil
}

func renderBatchTask(task BatchTask, baseOpts md2png.RenderOptions, format string, stderr io.Writer) error {
	data, err := os.ReadFile(task.InputPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(task.OutputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", task.OutputPath, err)
	}

	opts := baseOpts
	opts.BaseDir = task.BaseDir

	res, err := md2png.RenderWithDiagnostics(data, opts)
	if len(res.Diagnostics) > 0 && stderr != nil {
		for _, diag := range res.Diagnostics {
			if _, err := fmt.Fprintf(stderr, "md2png [%s]: %s\n", task.InputPath, diag.String()); err != nil {
				return fmt.Errorf("md2png batch: write diagnostics: %w", err)
			}
		}
	}
	if err != nil {
		return err
	}

	return encodeToFile(task.OutputPath, res.Image, format)
}
