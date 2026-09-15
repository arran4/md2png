package cmd_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestGoReleaserSnapshotConfiguration(t *testing.T) {
	cmd := exec.Command("go", "run", "github.com/goreleaser/goreleaser/v2@latest", "check")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		outStr := out.String()
		// goreleaser check might return exit status 2 if it's "configuration is valid, but uses deprecated properties"
		if strings.Contains(outStr, "configuration is valid, but uses deprecated properties") {
			t.Logf("Configuration is valid, but has deprecations.\n%s", outStr)
		} else {
			t.Fatalf("goreleaser check failed: %v\nOutput: %s", err, outStr)
		}
	}
}
