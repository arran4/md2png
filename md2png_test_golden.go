package md2png

import (
	"bytes"
	"image"
	"image/png"
	"os"

	"testing"
)

func verifyImageMatch(t *testing.T, actual image.Image, goldenPath string) {
	t.Helper()

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		f, err := os.Create(goldenPath)
		if err != nil {
			t.Fatalf("failed to create golden file: %v", err)
		}
		defer f.Close()
		if err := png.Encode(f, actual); err != nil {
			t.Fatalf("failed to encode golden file: %v", err)
		}
		return
	}

	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file (run with UPDATE_GOLDEN=1 to update): %v", err)
	}

	var actualBuf bytes.Buffer
	if err := png.Encode(&actualBuf, actual); err != nil {
		t.Fatalf("failed to encode actual image: %v", err)
	}

	if !bytes.Equal(goldenData, actualBuf.Bytes()) {
		actualPath := goldenPath + ".actual.png"
		_ = os.WriteFile(actualPath, actualBuf.Bytes(), 0644)
		t.Errorf("image mismatch for %s. Actual image written to %s", goldenPath, actualPath)
	}
}
