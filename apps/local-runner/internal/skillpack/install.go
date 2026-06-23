package skillpack

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed flow-pack
var flowPackFS embed.FS

const PackVersion = 1

// providerDirs are the AI tool directories that each receive the skill files.
var providerDirs = []string{".claude", ".codex", ".gemini"}

// InstallResult summarises what Install did for each file it encountered.
type InstallResult struct {
	Target    string
	Installed []string
	Skipped   []string
	Errors    []string
}

// Install copies every embedded SKILL.md into <targetRepoDir>/<providerDir>/skills/flowpilot/<skill>/SKILL.md.
// An existing file is skipped when its first line already declares the current PackVersion.
// All errors are collected and returned in InstallResult.Errors; the function never panics.
func Install(targetRepoDir string) (InstallResult, error) {
	result := InstallResult{Target: targetRepoDir}

	entries, err := fs.ReadDir(flowPackFS, "flow-pack")
	if err != nil {
		return result, fmt.Errorf("skillpack: read embedded flow-pack dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		srcPath := "flow-pack/" + skillName + "/SKILL.md"

		srcBytes, err := flowPackFS.ReadFile(srcPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %v", srcPath, err))
			continue
		}

		for _, providerDir := range providerDirs {
			destDir := filepath.Join(targetRepoDir, providerDir, "skills", "flowpilot", skillName)
			destFile := filepath.Join(destDir, "SKILL.md")

			if fileMatchesVersion(destFile, PackVersion) {
				result.Skipped = append(result.Skipped, destFile)
				continue
			}

			if mkErr := os.MkdirAll(destDir, 0o755); mkErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("mkdir %s: %v", destDir, mkErr))
				continue
			}

			if writeErr := os.WriteFile(destFile, srcBytes, 0o644); writeErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("write %s: %v", destFile, writeErr))
				continue
			}

			result.Installed = append(result.Installed, destFile)
		}
	}

	return result, nil
}

// IsInstalled returns true when the primary sentinel file exists in the .claude provider dir.
func IsInstalled(targetRepoDir string) bool {
	sentinel := filepath.Join(targetRepoDir, ".claude", "skills", "flowpilot", "git-commit-format", "SKILL.md")
	_, err := os.Stat(sentinel)
	return err == nil
}

// fileMatchesVersion reads the first non-empty line of path and checks whether
// it equals "version: <v>". Returns false on any read error.
func fileMatchesVersion(path string, v int) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.SplitN(string(data), "\n", 10) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return false
		}
		if strings.TrimSpace(parts[0]) != "version" {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return false
		}
		return n == v
	}
	return false
}
