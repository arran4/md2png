package cli

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatch_EmptyInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunBatch(&stdout, &stderr, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error with empty inputs, got nil")
	}
	if !strings.Contains(err.Error(), "at least one input file or directory is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBatch_UnsupportedFormat(t *testing.T) {
	tempDir := t.TempDir()
	f := filepath.Join(tempDir, "test.md")
	if err := os.WriteFile(f, []byte("# Test"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	badFormat := "bmp"
	err := RunBatch(&stdout, &stderr, nil, nil, &badFormat, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, f)
	if err == nil {
		t.Fatal("expected error with unsupported format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBatch_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "docs")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f1 := filepath.Join(tempDir, "readme.md")
	f2 := filepath.Join(subDir, "guide.markdown")
	if err := os.WriteFile(f1, []byte("# Readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("# Guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "output")
	isDryRun := true
	var stdout, stderr bytes.Buffer

	err := RunBatch(&stdout, &stderr, &outDir, nil, nil, nil, &isDryRun,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, f1, subDir)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, f1+" -> "+filepath.Join(outDir, "readme.png")) {
		t.Errorf("stdout missing expected f1 mapping: %s", outStr)
	}
	if !strings.Contains(outStr, f2+" -> "+filepath.Join(outDir, "guide.png")) {
		t.Errorf("stdout missing expected f2 mapping: %s", outStr)
	}

	// Verify nothing was actually written to outDir
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("outDir should not exist after dry run")
	}
}

func TestBatch_OutputCollision(t *testing.T) {
	tempDir := t.TempDir()
	dirA := filepath.Join(tempDir, "a")
	dirB := filepath.Join(tempDir, "b")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}

	fA := filepath.Join(dirA, "doc.md")
	fB := filepath.Join(dirB, "doc.md")
	if err := os.WriteFile(fA, []byte("# A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fB, []byte("# B"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "output")
	var stdout, stderr bytes.Buffer
	// Passing two standalone files with the same basename into the same outDir causes collision
	err := RunBatch(&stdout, &stderr, &outDir, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fA, fB)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Fatalf("unexpected collision error: %v", err)
	}
}

func TestBatch_RenderDirectoryAndFiles(t *testing.T) {
	tempDir := t.TempDir()
	docDir := filepath.Join(tempDir, "docs")
	subDir := filepath.Join(docDir, "advanced")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rootFile := filepath.Join(tempDir, "index.md")
	docFile := filepath.Join(docDir, "intro.md")
	advFile := filepath.Join(subDir, "spec.markdown")

	if err := os.WriteFile(rootFile, []byte("# Root Index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docFile, []byte("# Introduction"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(advFile, []byte("# Advanced Spec"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "dist")
	concurrency := 2
	var stdout, stderr bytes.Buffer

	err := RunBatch(&stdout, &stderr, &outDir, &concurrency, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, rootFile, docDir)
	if err != nil {
		t.Fatalf("batch render failed: %v\nstderr: %s", err, stderr.String())
	}

	expectedFiles := []string{
		filepath.Join(outDir, "index.png"),
		filepath.Join(outDir, "intro.png"),
		filepath.Join(outDir, "advanced", "spec.png"),
	}

	for _, exp := range expectedFiles {
		f, err := os.Open(exp)
		if err != nil {
			t.Fatalf("expected output %s was not created: %v", exp, err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("failed to decode generated image %s: %v", exp, err)
		}
		if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
			t.Errorf("image %s has empty dimensions: %v", exp, img.Bounds())
		}
	}
}

func TestBatch_PartialFailureAndContinueOnError(t *testing.T) {
	tempDir := t.TempDir()
	goodFile := filepath.Join(tempDir, "good.md")
	badFile := filepath.Join(tempDir, "bad.md")

	if err := os.WriteFile(goodFile, []byte("# Good Content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// With strict=true and missing image, bad.md will fail
	if err := os.WriteFile(badFile, []byte("![missing](nonexistent.png)"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "dist")
	strict := true

	// Case 1: continueOnError = false -> returns error, fails batch
	{
		cont := false
		var stdout, stderr bytes.Buffer
		err := RunBatch(&stdout, &stderr, &outDir, nil, nil, &cont, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &strict, nil, nil, nil, goodFile, badFile)
		if err == nil {
			t.Fatal("expected error with strict failure, got nil")
		}
		if !strings.Contains(err.Error(), "render failure") {
			t.Errorf("unexpected error message: %v", err)
		}
	}

	// Case 2: continueOnError = true -> renders good file, logs failure on bad file, returns aggregate error
	{
		cont := true
		var stdout, stderr bytes.Buffer
		err := RunBatch(&stdout, &stderr, &outDir, nil, nil, &cont, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &strict, nil, nil, nil, goodFile, badFile)
		if err == nil {
			t.Fatal("expected error with strict failure, got nil")
		}
		if !strings.Contains(err.Error(), "1 render failure") {
			t.Errorf("unexpected error message: %v", err)
		}
		// good.png should still have been rendered
		goodImgPath := filepath.Join(outDir, "good.png")
		if _, err := os.Stat(goodImgPath); err != nil {
			t.Errorf("good.png should be created even when another file fails: %v", err)
		}
	}
}

func TestBatch_RelativeImageBaseDir(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "chapter1")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a dummy image in chapter1/
	imgFile := filepath.Join(subDir, "pic.png")
	rgba := image.NewRGBA(image.Rect(0, 0, 10, 10))
	f, err := os.Create(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, rgba); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	mdFile := filepath.Join(subDir, "chapter.md")
	if err := os.WriteFile(mdFile, []byte("# Chapter 1\n\n![diagram](pic.png)"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "out")
	strict := true
	var stdout, stderr bytes.Buffer

	// Strict=true ensures that if relative image cannot be found from the file's dir, it fails
	err = RunBatch(&stdout, &stderr, &outDir, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &strict, nil, nil, nil, subDir)
	if err != nil {
		t.Fatalf("batch render failed to resolve relative image: %v\nstderr: %s", err, stderr.String())
	}

	outPath := filepath.Join(outDir, "chapter.png")
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output %s was not created: %v", outPath, err)
	}
}
