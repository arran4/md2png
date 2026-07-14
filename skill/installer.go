package skill

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallFiles safely copies files from a source fs.FS (e.g. from local disk, or embedded) to destDir.
// It guards against path traversal, absolute paths, and ensures no executable bits are set.
// It also checks that a SKILL.md file exists at the root of the source FS.
func InstallFiles(src fs.FS, destDir string) error {
	// First, verify SKILL.md exists
	if _, err := fs.Stat(src, "SKILL.md"); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("SKILL.md not found in source")
		}
		return fmt.Errorf("error checking for SKILL.md: %w", err)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." || path == "" {
			return nil
		}

		// Security: clean the path and prevent traversal out of destDir
		cleanPath := filepath.Clean(filepath.ToSlash(path))
		if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || filepath.IsAbs(cleanPath) {
			return fmt.Errorf("illegal path traversal detected in source: %s", path)
		}

		destPath := filepath.Join(destDir, cleanPath)

		// Extra safety: ensure destPath is actually inside destDir
		absDestDir, _ := filepath.Abs(destDir)
		absDestPath, _ := filepath.Abs(destPath)
		if !strings.HasPrefix(absDestPath, absDestDir) {
			return fmt.Errorf("illegal path traversal detected in source: %s", path)
		}

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// It's a file
		srcFile, err := src.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = srcFile.Close() }()

		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer func() { _ = destFile.Close() }()

		if _, err := io.Copy(destFile, srcFile); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// ComputeDigest calculates the sha256 of all files in a skill directory (excluding metadata)
// and returns the hex encoded string. Useful to check for local modifications.
func ComputeDigest(dir string) (string, error) {
	hasher := sha256.New()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Base(path) == ".skill-metadata.json" {
			return nil // Skip metadata
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Write the relative path to the hasher to detect file renaming or swapping
		hasher.Write([]byte(filepath.ToSlash(relPath)))

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		if _, err := io.Copy(hasher, file); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// extractZip extracts files from a zip archive data into destDir.
func extractZip(data []byte, skillSubDir, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	skillFound := false
	var prefix string

	// 1. Find the root of the archive (usually repository name)
	// Or find the specific skillSubDir if provided.
	if skillSubDir != "" {
		// e.g. "skills/foo"
		// GitHub archives are prefixed with repo-name-branch/
		// We need to locate "repo-name-branch/skills/foo/SKILL.md"
		for _, f := range r.File {
			if strings.HasSuffix(f.Name, "/"+skillSubDir+"/SKILL.md") || f.Name == skillSubDir+"/SKILL.md" {
				prefix = filepath.Dir(f.Name)
				skillFound = true
				break
			}
		}
	} else {
		for _, f := range r.File {
			if strings.HasSuffix(f.Name, "/SKILL.md") || f.Name == "SKILL.md" {
				prefix = filepath.Dir(f.Name)
				skillFound = true
				break
			}
		}
	}

	if !skillFound {
		return fmt.Errorf("SKILL.md not found in remote archive")
	}

	if prefix == "." {
		prefix = ""
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}

		relPath := strings.TrimPrefix(f.Name, prefix)
		if relPath == "" || relPath == "." {
			continue
		}

		cleanPath := filepath.Clean(filepath.ToSlash(relPath))
		if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || filepath.IsAbs(cleanPath) {
			return fmt.Errorf("illegal path traversal detected in source: %s", relPath)
		}

		destPath := filepath.Join(destDir, cleanPath)
		absDestDir, _ := filepath.Abs(destDir)
		absDestPath, _ := filepath.Abs(destPath)
		if !strings.HasPrefix(absDestPath, absDestDir) {
			return fmt.Errorf("illegal path traversal detected in source: %s", relPath)
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}

		_ = os.MkdirAll(filepath.Dir(destPath), 0755)

		srcFile, err := f.Open()
		if err != nil {
			return err
		}

		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) // enforce safe permissions
		if err != nil {
			_ = srcFile.Close()
			return err
		}

		_, err = io.Copy(destFile, srcFile)

		_ = destFile.Close()
		_ = srcFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
}
