package cli

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		name       string
		outPath    string
		formatFlag *string
		wantFormat string
		wantErr    bool
	}{
		{"stdout no format", "-", nil, "", true},
		{"stdout with format", "-", ptr("png"), "png", false},
		{"stdout unsupported format", "-", ptr("webp"), "", true},
		{"file no format", "out.png", nil, "png", false},
		{"file unsupported extension", "out.webp", nil, "", true},
		{"file with format agree", "out.png", ptr("png"), "png", false},
		{"file with format disagree", "out.png", ptr("jpeg"), "", true},
		{"file with jpg mapped to jpeg", "out.jpg", nil, "jpeg", false},
		{"file with jpg format jpeg flag", "out.jpg", ptr("jpeg"), "jpeg", false},
		{"file with jpg alias", "out.jpg", ptr("jpg"), "jpeg", false},
		{"format matching is case insensitive", "out.PNG", ptr("PNG"), "png", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFormat(tc.outPath, tc.formatFlag)
			if (err != nil) != tc.wantErr {
				t.Errorf("resolveFormat() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.wantFormat {
				t.Errorf("resolveFormat() = %v, want %v", got, tc.wantFormat)
			}
		})
	}
}

func TestMd2png_Integration(t *testing.T) {
	tempDir := t.TempDir()

	inPath := filepath.Join(tempDir, "in.md")
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		outName    string
		formatFlag *string
		wantErr    bool
		check      func(t *testing.T, outPath string)
	}{
		{
			name:    "png output",
			outName: "out.png",
			wantErr: false,
			check: func(t *testing.T, outPath string) {
				f, err := os.Open(outPath)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = f.Close() }()
				if _, err := png.Decode(f); err != nil {
					t.Errorf("failed to decode as PNG: %v", err)
				}
			},
		},
		{
			name:    "jpeg output",
			outName: "out.jpg",
			wantErr: false,
			check: func(t *testing.T, outPath string) {
				f, err := os.Open(outPath)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = f.Close() }()
				if _, err := jpeg.Decode(f); err != nil {
					t.Errorf("failed to decode as JPEG: %v", err)
				}
			},
		},
		{
			name:    "gif output",
			outName: "out.gif",
			wantErr: false,
			check: func(t *testing.T, outPath string) {
				f, err := os.Open(outPath)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = f.Close() }()
				if _, err := gif.Decode(f); err != nil {
					t.Errorf("failed to decode as GIF: %v", err)
				}
			},
		},
		{
			name:       "format flag disagree",
			outName:    "out.png",
			formatFlag: ptr("jpeg"),
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outPath := filepath.Join(tempDir, tc.outName)
			err := Md2png(&inPath, &outPath, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, tc.formatFlag)
			if (err != nil) != tc.wantErr {
				t.Errorf("Md2png() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.check != nil {
				tc.check(t, outPath)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	runErr := fn()
	closeErr := w.Close()
	os.Stdout = oldStdout
	if closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}
	defer func() { _ = r.Close() }()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return buf.Bytes(), runErr
}

func TestMd2png_StdoutFormats(t *testing.T) {
	tempDir := t.TempDir()
	inPath := filepath.Join(tempDir, "in.md")
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		format    string
		signature []byte
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n")},
		{"jpeg", []byte("\xff\xd8\xff")},
		{"gif", []byte("GIF8")},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			outArg := "-"
			output, err := captureStdout(t, func() error {
				return Md2png(&inPath, &outArg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr(tc.format))
			})
			if err != nil {
				t.Fatalf("Md2png() error = %v", err)
			}
			if !bytes.HasPrefix(output, tc.signature) {
				t.Fatalf("stdout does not start with expected %s signature", tc.format)
			}
		})
	}
}

func TestMd2png_PreserveDestination(t *testing.T) {
	tempDir := t.TempDir()

	inPath := filepath.Join(tempDir, "in.md")
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tempDir, "out.png")
	if err := os.WriteFile(outPath, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Md2png(&inPath, &outPath, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr("jpeg"))
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original content" {
		t.Errorf("expected original content, got %s", content)
	}

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files in temp dir, got %d", len(files))
	}
}

func TestEncodeToFileFailurePreservesDestination(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "out.png")
	if err := os.WriteFile(outPath, []byte("original content"), 0o640); err != nil {
		t.Fatal(err)
	}

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	forcedErr := errors.New("forced encode failure")
	err := encodeToFileWithEncoder(outPath, img, "png", func(w io.Writer, _ image.Image, _ string) error {
		if _, err := w.Write([]byte("partial output")); err != nil {
			return err
		}
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Fatalf("expected forced encode failure, got %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original content" {
		t.Fatalf("destination was modified after failed encode: %q", content)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "md2png-tmp-") {
			t.Fatalf("temporary file was not cleaned up: %s", entry.Name())
		}
	}
}

func TestEncodeToFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not portable to Windows")
	}

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tempDir := t.TempDir()

	t.Run("new output", func(t *testing.T) {
		outPath := filepath.Join(tempDir, "new.png")
		if err := encodeToFile(outPath, img, "png"); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Fatalf("new output mode = %04o, want 0644", got)
		}
	})

	t.Run("replacement preserves mode", func(t *testing.T) {
		outPath := filepath.Join(tempDir, "existing.png")
		if err := os.WriteFile(outPath, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(outPath, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := encodeToFile(outPath, img, "png"); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("replacement mode = %04o, want 0640", got)
		}
	})
}

func TestMd2png_StdinToStdout(t *testing.T) {
	oldStdin := os.Stdin
	rStdin, wStdin, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = rStdin
	defer func() {
		os.Stdin = oldStdin
		_ = rStdin.Close()
	}()

	if _, err := wStdin.Write([]byte("# Stdin test\n")); err != nil {
		t.Fatal(err)
	}
	if err := wStdin.Close(); err != nil {
		t.Fatal(err)
	}

	outArg := "-"
	output, err := captureStdout(t, func() error {
		return Md2png(nil, &outArg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr("png"))
	})
	if err != nil {
		t.Fatalf("Md2png() error = %v", err)
	}
	if !bytes.HasPrefix(output, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("expected PNG signature in stdout")
	}
}
