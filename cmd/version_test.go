package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func buildBinary(t *testing.T, dir, target string, ldflags ...string) string {
	t.Helper()
	outPath := dir + "/md2png-test-bin"
	args := []string{"build"}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, "-o", outPath, target)

	cmd := exec.Command("go", args...)
	cmd.Dir = ".."

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build %s: %v\nOutput: %s", target, err, output)
	}
	return outPath
}

func runVersionCmd(t *testing.T, binPath string) string {
	t.Helper()
	cmd := exec.Command(binPath, "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run version command: %v\nOutput: %s", err, out.String())
	}
	return out.String()
}

func TestVersionMetadataInjection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "md2png-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []string{"./cmd/md2png", "./cmd/md2view"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			binPath := buildBinary(t, tempDir, target, "-X main.version=v0.6.0", "-X main.commit=abcdef1", "-X main.date=2024-05-06T12:00:00Z")
			output := runVersionCmd(t, binPath)

			if !strings.Contains(output, "Version: v0.6.0") {
				t.Errorf("Expected Version: v0.6.0, got: %s", output)
			}
			if !strings.Contains(output, "Commit: abcdef1") {
				t.Errorf("Expected Commit: abcdef1, got: %s", output)
			}
			if !strings.Contains(output, "Date: 2024-05-06T12:00:00Z") {
				t.Errorf("Expected Date: 2024-05-06T12:00:00Z, got: %s", output)
			}
		})
	}
}

func TestVersionMetadataRegressionProtectTag(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "md2png-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []string{"./cmd/md2png", "./cmd/md2view"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			// Deliberately injecting a non-v-prefixed string, to ensure it reports EXACTLY what was injected
			// and hasn't had the v stripped, but also to confirm that it's what goreleaser's .Tag provides.
			// By injecting v0.6.0 we expect exactly v0.6.0, not 0.6.0.
			binPath := buildBinary(t, tempDir, target, "-X main.version=v0.6.0", "-X main.commit=abcdef1", "-X main.date=2024-05-06T12:00:00Z")
			output := runVersionCmd(t, binPath)

			if strings.Contains(output, "Version: 0.6.0") {
				t.Errorf("Regression: Version stripped leading 'v'. Expected Version: v0.6.0, got: %s", output)
			}
		})
	}
}

func TestVersionMetadataDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "md2png-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []string{"./cmd/md2png", "./cmd/md2view"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			binPath := buildBinary(t, tempDir, target)
			output := runVersionCmd(t, binPath)

			if !strings.Contains(output, "Version: dev") {
				t.Errorf("Expected Version: dev, got: %s", output)
			}
			if !strings.Contains(output, "Commit: none") {
				t.Errorf("Expected Commit: none, got: %s", output)
			}
			if !strings.Contains(output, "Date: unknown") {
				t.Errorf("Expected Date: unknown, got: %s", output)
			}
		})
	}
}
