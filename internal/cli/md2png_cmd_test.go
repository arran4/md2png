package cli

import (
	"bytes"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
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
		{"file no format", "out.png", nil, "png", false},
		{"file with format agree", "out.png", ptr("png"), "png", false},
		{"file with format disagree", "out.png", ptr("jpeg"), "", true},
		{"file with jpg mapped to jpeg", "out.jpg", nil, "jpeg", false},
		{"file with jpg format jpeg flag", "out.jpg", ptr("jpeg"), "jpeg", false},
		{"file with jpg flag", "out.png", ptr("jpg"), "", true}, // "jpg" is a valid format passed by flag, it is compared exactly to ext "png". So disagree. However, we map jpg ext to jpeg. Should we map jpg flag to jpeg too? Currently we don't.
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
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0644); err != nil {
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
				defer f.Close()
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
				defer f.Close()
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
				defer f.Close()
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

func TestMd2png_Stdout(t *testing.T) {
	tempDir := t.TempDir()

	inPath := filepath.Join(tempDir, "in.md")
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0644); err != nil {
		t.Fatal(err)
	}

	// Intercept stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outArg := "-"
	err := Md2png(&inPath, &outArg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr("png"))

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("Md2png() error = %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if buf.Len() == 0 {
		t.Fatal("expected stdout output")
	}

	if !bytes.HasPrefix(buf.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("expected PNG signature in stdout")
	}
}

func TestMd2png_PreserveDestination(t *testing.T) {
	tempDir := t.TempDir()

	inPath := filepath.Join(tempDir, "in.md")
	if err := os.WriteFile(inPath, []byte("# Hello World"), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tempDir, "out.png")
	if err := os.WriteFile(outPath, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	// This should fail because format disagree
	err := Md2png(&inPath, &outPath, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr("jpeg"))
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	// Check that the original file is intact
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original content" {
		t.Errorf("expected original content, got %s", content)
	}

	// Check that there are no temporary files left in the directory
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// We should only have in.md and out.png
	if len(files) != 2 {
		t.Errorf("expected 2 files in temp dir, got %d", len(files))
	}

	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}

	if !strings.Contains(strings.Join(names, " "), "out.png") {
		t.Errorf("expected out.png in temp dir, got %v", names)
	}
}

func TestMd2png_StdinToStdout(t *testing.T) {
	// Setup stdin
	oldStdin := os.Stdin
	rStdin, wStdin, _ := os.Pipe()
	os.Stdin = rStdin

	wStdin.Write([]byte("# Stdin test\n"))
	wStdin.Close()

	// Setup stdout
	oldStdout := os.Stdout
	rStdout, wStdout, _ := os.Pipe()
	os.Stdout = wStdout

	outArg := "-"
	err := Md2png(nil, &outArg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ptr("png"))

	wStdout.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	if err != nil {
		t.Fatalf("Md2png() error = %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(rStdout)

	if buf.Len() == 0 {
		t.Fatal("expected stdout output")
	}

	if !bytes.HasPrefix(buf.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("expected PNG signature in stdout")
	}
}
