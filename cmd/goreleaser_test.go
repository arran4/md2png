package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGoReleaserSnapshotConfiguration(t *testing.T) {
	configBytes, err := os.ReadFile("../.goreleaser.yaml")
	if err != nil {
		t.Fatalf("Failed to read .goreleaser.yaml: %v", err)
	}
	config := string(configBytes)

	requiredLDFlags := []string{
		"-X main.version={{.Tag}}",
		"-X main.commit={{.Commit}}",
		"-X main.date={{.Date}}",
	}

	// Make sure we validate ldflags for both md2png and md2view independently
	buildSections := []string{
		"  - id: md2png",
		"  - id: md2view",
	}

	for _, sectionHeader := range buildSections {
		startIdx := strings.Index(config, sectionHeader)
		if startIdx == -1 {
			t.Fatalf("Could not find section %q in .goreleaser.yaml", sectionHeader)
		}

		// Find where this section roughly ends (e.g., the next `  - id:` or `archives:`)
		restOfConfig := config[startIdx+len(sectionHeader):]
		endIdx := strings.Index(restOfConfig, "\n  - id:")
		if endIdx == -1 {
			endIdx = strings.Index(restOfConfig, "\narchives:")
		}
		if endIdx == -1 {
			endIdx = len(restOfConfig)
		}

		section := restOfConfig[:endIdx]

		for _, flag := range requiredLDFlags {
			if !strings.Contains(section, flag) {
				t.Errorf("Build section %q is missing required ldflag: %s", sectionHeader, flag)
			}
		}
	}

	// Additionally, assert we don't accidentally use `.Version` instead of `.Tag`
	if strings.Contains(config, "-X main.version={{.Version}}") {
		t.Errorf("GoReleaser config incorrectly uses {{.Version}} instead of {{.Tag}} for main.version")
	}

	// Also test using a local build snapshot if goreleaser is available locally in CI (optional but robust)
	cmd := exec.Command("goreleaser", "check")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			t.Log("goreleaser binary not found, skipping full configuration parse check")
			return
		}

		outStr := out.String()
		if strings.Contains(outStr, "configuration is valid, but uses deprecated properties") {
			t.Logf("Configuration is valid, but has deprecations.\n%s", outStr)
		} else {
			t.Fatalf("goreleaser check failed: %v\nOutput: %s", err, outStr)
		}
	}
}
