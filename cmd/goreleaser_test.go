package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGoReleaserSnapshotConfiguration(t *testing.T) {
	// The prompt requests "Where practical within the repository's existing tooling, add a GoReleaser
	// configuration/snapshot smoke check that exercises or validates the metadata
	// ldflags without publishing anything."

	// We check if `.goreleaser.yaml` is using `.Tag` for version, `.Commit` for commit, etc.
	// We read the actual file rather than running `go run github.com/goreleaser/goreleaser/v2@latest`
	// to avoid non-deterministic network dependency from `@latest` in the standard test suite.

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

	for _, flag := range requiredLDFlags {
		if !strings.Contains(config, flag) {
			t.Errorf("GoReleaser config is missing required ldflag: %s", flag)
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
		// goreleaser check might return exit status 2 if it's "configuration is valid, but uses deprecated properties"
		if strings.Contains(outStr, "configuration is valid, but uses deprecated properties") {
			t.Logf("Configuration is valid, but has deprecations.\n%s", outStr)
		} else {
			t.Fatalf("goreleaser check failed: %v\nOutput: %s", err, outStr)
		}
	}
}
