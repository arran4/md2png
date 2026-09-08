//go:build ignore
// +build ignore

package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	cmd := exec.Command("go", "run", "github.com/arran4/go-subcommand/cmd/gosubc@v0.0.29", "generate", "--timestamp=false", "--project-provenance=false")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("go-subcommand generation failed: %v", err)
	}

	// Remove generated tests that contain syntax errors due to integer flags
	os.Remove("cmd/md2png/root_test.go")
	os.Remove("cmd/md2view/root_test.go")
}
