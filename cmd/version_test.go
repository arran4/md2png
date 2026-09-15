package cmd_test

import (
	"bytes"
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

func assertOutputLine(t *testing.T, output, prefix, expected string) {
	t.Helper()
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			if line != expected {
				t.Errorf("Expected exact line %q, got: %q", expected, line)
			}
			return
		}
	}
	t.Errorf("Could not find line starting with %q in output:\n%s", prefix, output)
}

func TestVersionMetadataInjection(t *testing.T) {
	tempDir := t.TempDir()

	tests := []string{"./cmd/md2png", "./cmd/md2view"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			binPath := buildBinary(t, tempDir, target, "-X main.version=v0.6.0", "-X main.commit=abcdef1", "-X main.date=2024-05-06T12:00:00Z")
			output := runVersionCmd(t, binPath)

			assertOutputLine(t, output, "Version:", "Version: v0.6.0")
			assertOutputLine(t, output, "Commit:", "Commit: abcdef1")
			assertOutputLine(t, output, "Date:", "Date: 2024-05-06T12:00:00Z")
		})
	}
}

func TestVersionMetadataDefault(t *testing.T) {
	tempDir := t.TempDir()

	tests := []string{"./cmd/md2png", "./cmd/md2view"}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			binPath := buildBinary(t, tempDir, target)
			output := runVersionCmd(t, binPath)

			assertOutputLine(t, output, "Version:", "Version: dev")
			assertOutputLine(t, output, "Commit:", "Commit: none")
			assertOutputLine(t, output, "Date:", "Date: unknown")
		})
	}
}
