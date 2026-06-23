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

const PackVersion = 2

type installRoot struct {
	Provider string
	RootPath string
}

var installRoots = []installRoot{
	{Provider: "claude", RootPath: filepath.Join(".claude", "skills")},
	{Provider: "agents", RootPath: filepath.Join(".agents", "skills")},
}

var providerStatuses = []installRoot{
	{Provider: "claude", RootPath: filepath.Join(".claude", "skills")},
	{Provider: "codex", RootPath: filepath.Join(".agents", "skills")},
	{Provider: "gemini", RootPath: filepath.Join(".agents", "skills")},
}

// InstallResult summarises what Install did for each file it encountered.
type InstallResult struct {
	Target    string
	Installed []string
	Skipped   []string
	Errors    []string
}

type ProviderStatus struct {
	Provider string `json:"provider"`
	Path     string `json:"path"`
	Present  bool   `json:"present"`
	Current  bool   `json:"current"`
}

type SkillStatus struct {
	Name      string           `json:"name"`
	Providers []ProviderStatus `json:"providers"`
}

type PackStatus struct {
	PackVersion int           `json:"packVersion"`
	Installed   bool          `json:"installed"`
	Current     bool          `json:"current"`
	Skills      []SkillStatus `json:"skills"`
}

// Install copies every embedded SKILL.md into:
// - <targetRepoDir>/.claude/skills/<skill>/SKILL.md
// - <targetRepoDir>/.agents/skills/<skill>/SKILL.md
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

		for _, root := range installRoots {
			destDir := filepath.Join(targetRepoDir, root.RootPath, skillName)
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
	sentinels := []string{
		filepath.Join(targetRepoDir, ".claude", "skills", "git-commit-format", "SKILL.md"),
		filepath.Join(targetRepoDir, ".agents", "skills", "git-commit-format", "SKILL.md"),
	}
	for _, sentinel := range sentinels {
		if _, err := os.Stat(sentinel); err != nil {
			return false
		}
	}
	return true
}

func ProviderDirs() []string {
	out := make([]string, 0, len(installRoots))
	for _, root := range installRoots {
		out = append(out, root.RootPath)
	}
	return out
}

func SkillNames() ([]string, error) {
	entries, err := fs.ReadDir(flowPackFS, "flow-pack")
	if err != nil {
		return nil, fmt.Errorf("skillpack: read embedded flow-pack dir: %w", err)
	}

	skills := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			skills = append(skills, entry.Name())
		}
	}
	return skills, nil
}

func Status(targetRepoDir string) (PackStatus, error) {
	skills, err := SkillNames()
	if err != nil {
		return PackStatus{}, err
	}

	status := PackStatus{
		PackVersion: PackVersion,
		Installed:   true,
		Current:     true,
		Skills:      make([]SkillStatus, 0, len(skills)),
	}

	for _, skillName := range skills {
		skill := SkillStatus{Name: skillName}
		for _, provider := range providerStatuses {
			path := filepath.Join(targetRepoDir, provider.RootPath, skillName, "SKILL.md")
			_, statErr := os.Stat(path)
			present := statErr == nil
			current := present && fileMatchesVersion(path, PackVersion)
			skill.Providers = append(skill.Providers, ProviderStatus{
				Provider: provider.Provider,
				Path:     path,
				Present:  present,
				Current:  current,
			})
			if !present {
				status.Installed = false
			}
			if !current {
				status.Current = false
			}
		}
		status.Skills = append(status.Skills, skill)
	}

	return status, nil
}

// fileMatchesVersion reads a top-of-file YAML frontmatter version when present,
// falling back to the legacy first-line "version: <v>" format. Returns false on
// any read error.
func fileMatchesVersion(path string, v int) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	first := firstNonEmptyLine(lines)
	if first == "" {
		return false
	}

	if first == "---" {
		return readFrontmatterVersion(lines) == v
	}

	if parsed, ok := parseVersionLine(first); ok {
		return parsed == v
	}
	return false
}

func firstNonEmptyLine(lines []string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func readFrontmatterVersion(lines []string) int {
	started := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" && !started {
			continue
		}
		if !started {
			if line != "---" {
				return 0
			}
			started = true
			continue
		}
		if line == "---" {
			return 0
		}
		if parsed, ok := parseVersionLine(line); ok {
			return parsed
		}
	}
	return 0
}

func parseVersionLine(line string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	if strings.TrimSpace(parts[0]) != "version" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, false
	}
	return n, true
}
