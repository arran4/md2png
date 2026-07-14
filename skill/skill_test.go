package skill

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillLocal(t *testing.T) {
	tmp, err := os.MkdirTemp("", "skill_test_local")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Create a dummy local skill
	srcDir := filepath.Join(tmp, "src_skill")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# Test Skill\n"), 0644)

	originalWd, _ := os.Getwd()
	os.Chdir(tmp) // Make sure ScopeProject uses our tmp dir
	defer os.Chdir(originalWd)

	err = InstallSkill(srcDir, "test_skill", ScopeProject, "")
	if err != nil {
		t.Fatalf("Failed to install local skill: %v", err)
	}

	destDir := filepath.Join(tmp, ".agents", "skills", "test_skill")
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); os.IsNotExist(err) {
		t.Errorf("Installed SKILL.md not found")
	}
	if _, err := os.Stat(filepath.Join(destDir, ".skill-metadata.json")); os.IsNotExist(err) {
		t.Errorf("Metadata manifest not found")
	}
}

func TestUpdateConflict(t *testing.T) {
	tmp, err := os.MkdirTemp("", "skill_test_update")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	srcDir := filepath.Join(tmp, "src_skill")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# Test Skill\n"), 0644)

	originalWd, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(originalWd)

	InstallSkill(srcDir, "test_skill", ScopeProject, "")

	destDir := filepath.Join(tmp, ".agents", "skills", "test_skill")

	// Modify the skill locally
	os.WriteFile(filepath.Join(destDir, "SKILL.md"), []byte("# Modified Skill\n"), 0644)

	// Try to update, should fail
	err = UpdateSkill("test_skill", ScopeProject, "", false)
	if err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("Expected update to fail due to local modifications, got %v", err)
	}

	// Try with force, should succeed
	err = UpdateSkill("test_skill", ScopeProject, "", true)
	if err != nil {
		t.Fatalf("Expected force update to succeed, got %v", err)
	}
}

func TestPathTraversalZip(t *testing.T) {
	// Create a malicious zip file
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Put SKILL.md in root to satisfy requirement
	f2, _ := w.Create("SKILL.md")
	f2.Write([]byte("ok"))

	f, _ := w.Create("../../../malicious.txt")
	f.Write([]byte("bad"))

	w.Close()

	tmp, err := os.MkdirTemp("", "skill_test_traversal")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	_, err = extractZip(buf.Bytes(), "", tmp)
	if err == nil || !strings.Contains(err.Error(), "illegal path traversal") {
		t.Fatalf("Expected path traversal error, got %v", err)
	}
}
