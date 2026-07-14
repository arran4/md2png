package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// SkillManifest represents the metadata stored alongside an installed skill.
type SkillManifest struct {
	Name        string    `json:"name"`
	Source      string    `json:"source"`       // e.g. owner/repo, ./local-path, or virtual-source
	Path        string    `json:"path"`         // path within the source repository
	Revision    string    `json:"revision"`     // git commit hash, digest, or equivalent
	InstallTime time.Time `json:"install_time"` // when it was installed
	Scope       Scope     `json:"scope"`
	AgentTarget string    `json:"agent_target"` // e.g. "" for default, "codex" for specific
	Digest      string    `json:"digest"`       // sha256 of the installed files to detect local modifications
}

// TargetDirectory determines where skills should be installed based on scope and agent.
func TargetDirectory(scope Scope, agent string) (string, error) {
	agentPath := ".agents/skills" // default common convention
	if agent != "" {
		agentPath = fmt.Sprintf(".%s/skills", agent)
	}

	if scope == ScopeProject {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not get current working directory: %w", err)
		}
		return filepath.Join(cwd, agentPath), nil
	}

	// User scope
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %w", err)
	}

	// Use generic user-level config paths for default
	if agent == "" {
		return filepath.Join(home, ".config", "agents", "skills"), nil
	}

	// If a specific agent is requested, attempt to use its conventional home path
	return filepath.Join(home, fmt.Sprintf(".%s", agent), "skills"), nil
}

// LoadManifest reads the manifest for a given installed skill directory.
func LoadManifest(dir string) (*SkillManifest, error) {
	manifestPath := filepath.Join(dir, ".skill-metadata.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid skill metadata: %w", err)
	}
	return &manifest, nil
}

// SaveManifest writes the manifest to the given installed skill directory.
func SaveManifest(dir string, manifest *SkillManifest) error {
	manifestPath := filepath.Join(dir, ".skill-metadata.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0644)
}
