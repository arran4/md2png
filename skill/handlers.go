package skill

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallSkill implements the `install` subcommand logic.
// It handles installing from local paths, remote github repositories, or the builtin official skill.
func InstallSkill(source, name string, scope Scope, agentTarget string) error {
	destRoot, err := TargetDirectory(scope, agentTarget)
	if err != nil {
		return err
	}

	if name == "" {
		// Try to infer name from source
		parts := strings.Split(filepath.ToSlash(source), "/")
		name = parts[len(parts)-1]
		if name == "" || name == "." || name == ".." {
			return fmt.Errorf("must specify a skill name")
		}
	}

	destDir := filepath.Join(destRoot, name)

	var revision string
	var digest string

	// Handle local path
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || filepath.IsAbs(source) {
		absSrc, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		digest, err = InstallFiles(os.DirFS(absSrc), destDir)
		if err != nil {
			return err
		}
		revision = "local"
	} else if IsRemoteOwnerRepo(source) {
		// Handle remote GitHub repository
		remote := NewRemoteSource()
		data, branch, err := remote.FetchGitHubZip(source)
		if err != nil {
			return err
		}
		digest, err = extractZip(data, "", destDir) // Try root
		if err != nil {
			// If not found in root, try skills/<name>
			digest, err = extractZip(data, "skills/"+name, destDir)
			if err != nil {
				return err
			}
		}

		// Try to get actual commit SHA for exact revision tracking
		sha, shaErr := remote.FetchGitHubCommitSHA(source, branch)
		if shaErr == nil {
			revision = sha
		} else {
			revision = branch
		}
	} else if source == "md2png" { // Built-in official skill
		// We'll need a way to pass the FS down, but for now just mock the builtin
		return fmt.Errorf("builtin skill installation requires injection from main package")
	} else {
		return fmt.Errorf("unknown source format: %s", source)
	}

	manifest := &SkillManifest{
		Name:        name,
		Source:      source,
		Path:        "",
		Revision:    revision,
		InstallTime: time.Now(),
		Scope:       scope,
		AgentTarget: agentTarget,
		Digest:      digest,
	}

	return SaveManifest(destDir, manifest)
}

// Injectable function to install the builtin skill.
func InstallBuiltinSkill(skillFS fs.FS, name string, scope Scope, agentTarget string) error {
	destRoot, err := TargetDirectory(scope, agentTarget)
	if err != nil {
		return err
	}

	if name == "" {
		name = "md2png"
	}

	destDir := filepath.Join(destRoot, name)
	digest, err := InstallFiles(skillFS, destDir)
	if err != nil {
		return err
	}

	manifest := &SkillManifest{
		Name:        name,
		Source:      "md2png",
		Path:        "skills/md2png",
		Revision:    "builtin",
		InstallTime: time.Now(),
		Scope:       scope,
		AgentTarget: agentTarget,
		Digest:      digest,
	}

	return SaveManifest(destDir, manifest)
}

func UpdateSkill(name string, scope Scope, agent string, force bool) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}

	destDir := filepath.Join(destRoot, name)
	manifest, err := LoadManifest(destDir)
	if err != nil {
		return fmt.Errorf("no source metadata is available, so this skill cannot be updated automatically")
	}

	if !force {
		currentDigest, err := ComputeDigest(destDir)
		if err != nil {
			return err
		}
		if currentDigest != manifest.Digest {
			return fmt.Errorf("installed skill has local modifications; rerun with --force to replace")
		}
	}

	// Just re-installing using the manifest source.
	if manifest.Source == "md2png" {
		return fmt.Errorf("builtin skill update must be done via application upgrade")
	}

	fmt.Printf("Updating skill '%s' from '%s'...\n", name, manifest.Source)
	return InstallSkill(manifest.Source, name, scope, agent)
}

func UpdateAllSkills(scope Scope, agent string, force bool) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(destRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var errs []string
	for _, entry := range entries {
		if entry.IsDir() {
			if err := UpdateSkill(entry.Name(), scope, agent, force); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", entry.Name(), err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors updating skills:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

func RemoveSkill(name string, scope Scope, agent string) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}
	destDir := filepath.Join(destRoot, name)

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		return fmt.Errorf("skill '%s' not found", name)
	}

	return os.RemoveAll(destDir)
}

func ListSkills(scope Scope, agent string, format string) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(destRoot)
	if err != nil {
		if os.IsNotExist(err) {
			if format == "json" {
				fmt.Println("[]")
			} else {
				fmt.Println("No skills installed.")
			}
			return nil
		}
		return err
	}

	var skills []*SkillManifest
	for _, entry := range entries {
		if entry.IsDir() {
			manifest, err := LoadManifest(filepath.Join(destRoot, entry.Name()))
			if err == nil {
				skills = append(skills, manifest)
			}
		}
	}

	if format == "json" {
		data, _ := json.MarshalIndent(skills, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, s := range skills {
			fmt.Printf("%-20s %-20s %s\n", s.Name, s.Source, s.Revision)
		}
	}
	return nil
}

func InspectSkill(name string, scope Scope, agent string) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}
	destDir := filepath.Join(destRoot, name)

	manifest, err := LoadManifest(destDir)
	if err != nil {
		return fmt.Errorf("failed to load skill manifest: %w", err)
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	fmt.Println(string(data))
	return nil
}

func DoctorSkills(scope Scope, agent string) error {
	destRoot, err := TargetDirectory(scope, agent)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(destRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	issues := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		destDir := filepath.Join(destRoot, entry.Name())

		if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); os.IsNotExist(err) {
			fmt.Printf("⚠ %s is missing SKILL.md\n", entry.Name())
			issues++
		}

		manifest, err := LoadManifest(destDir)
		if err != nil {
			fmt.Printf("⚠ %s is missing metadata (%v)\n", entry.Name(), err)
			issues++
			continue
		}

		digest, err := ComputeDigest(destDir)
		if err == nil && digest != manifest.Digest {
			fmt.Printf("⚠ %s has local modifications\n", entry.Name())
			issues++
		}
	}

	if issues > 0 {
		return fmt.Errorf("found %d issues", issues)
	}
	fmt.Println("✔ All skills healthy")
	return nil
}
