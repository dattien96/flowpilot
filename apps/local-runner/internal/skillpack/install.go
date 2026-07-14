package skillpack

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed flow-pack
var flowPackFS embed.FS

const PackVersion = 6

// commonGroup is always installed regardless of project platform.
const commonGroup = "common"

type installRoot struct {
	Provider string
	RootPath string
}

var installRoots = []installRoot{
	{Provider: "claude", RootPath: filepath.Join(".claude", "skills")},
	{Provider: "agents", RootPath: filepath.Join(".agents", "skills")},
	{Provider: "grok", RootPath: filepath.Join(".grok", "skills")},
}

var providerStatuses = []installRoot{
	{Provider: "claude", RootPath: filepath.Join(".claude", "skills")},
	{Provider: "codex", RootPath: filepath.Join(".agents", "skills")},
	{Provider: "gemini", RootPath: filepath.Join(".agents", "skills")},
	{Provider: "grok", RootPath: filepath.Join(".grok", "skills")},
}

// skillRef identifies one embedded skill by its flow-pack group and skill name.
type skillRef struct {
	Group string
	Name  string
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

// normalizePlatform lower-cases and trims a raw platform value so unknown,
// empty, and "none" platforms all collapse to common-only behavior.
func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

// platformGroups returns the ordered, de-duplicated set of flow-pack groups to
// install for a given project platform. The common group is always first; a
// recognised platform contributes its same-named group, except Kotlin
// Multiplatform (kmm) which additionally pulls the android and ios groups.
func platformGroups(platform string) []string {
	groups := []string{commonGroup}

	switch normalizePlatform(platform) {
	case "kmm":
		groups = append(groups, "kmm", "android", "ios")
	case "android", "ios", "react-native", "flutter", "reactjs", "vuejs",
		"angularjs", "golang", "java", "python", "nodejs":
		groups = append(groups, normalizePlatform(platform))
	}

	return dedupeStrings(groups)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// skillsForPlatform returns the embedded skills to install for the platform,
// across the common group plus any platform-specific groups.
func skillsForPlatform(platform string) ([]skillRef, error) {
	refs := make([]skillRef, 0)
	for _, group := range platformGroups(platform) {
		entries, err := fs.ReadDir(flowPackFS, "flow-pack/"+group)
		if err != nil {
			// A mapped group should always have an embedded folder; tolerate a
			// missing one rather than failing the whole install.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("skillpack: read embedded group %q: %w", group, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				refs = append(refs, skillRef{Group: group, Name: entry.Name()})
			}
		}
	}
	return refs, nil
}

// Install copies every embedded SKILL.md for the project's platform into:
// - <targetRepoDir>/.claude/skills/<skill>/SKILL.md
// - <targetRepoDir>/.agents/skills/<skill>/SKILL.md
// - <targetRepoDir>/.grok/skills/<skill>/SKILL.md
// The common group is always installed; a platform contributes its own group
// (kmm also pulls android + ios). An existing file is skipped when it already
// declares the current PackVersion. All errors are collected and returned in
// InstallResult.Errors; the function never panics.
func Install(targetRepoDir string, platform string) (InstallResult, error) {
	result := InstallResult{Target: targetRepoDir}

	refs, err := skillsForPlatform(platform)
	if err != nil {
		return result, err
	}

	for _, ref := range refs {
		srcPath := "flow-pack/" + ref.Group + "/" + ref.Name + "/SKILL.md"

		srcBytes, err := flowPackFS.ReadFile(srcPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %v", srcPath, err))
			continue
		}

		for _, root := range installRoots {
			destDir := filepath.Join(targetRepoDir, root.RootPath, ref.Name)
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

// IsInstalled returns true when the primary sentinel file exists in each
// provider dir. The sentinel is a common-group skill, so it is present for
// every platform once the pack has been installed.
func IsInstalled(targetRepoDir string) bool {
	sentinels := []string{
		filepath.Join(targetRepoDir, ".claude", "skills", "git-commit-format", "SKILL.md"),
		filepath.Join(targetRepoDir, ".agents", "skills", "git-commit-format", "SKILL.md"),
		filepath.Join(targetRepoDir, ".grok", "skills", "git-commit-format", "SKILL.md"),
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

// SkillNames returns the de-duplicated skill names that apply to the given
// platform (common skills plus any platform-specific skills).
func SkillNames(platform string) ([]string, error) {
	refs, err := skillsForPlatform(platform)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return dedupeStrings(names), nil
}

func Status(targetRepoDir string, platform string) (PackStatus, error) {
	refs, err := skillsForPlatform(platform)
	if err != nil {
		return PackStatus{}, err
	}

	status := PackStatus{
		PackVersion: PackVersion,
		Installed:   true,
		Current:     true,
		Skills:      make([]SkillStatus, 0, len(refs)),
	}

	for _, ref := range refs {
		skill := SkillStatus{Name: ref.Name}
		for _, provider := range providerStatuses {
			path := filepath.Join(targetRepoDir, provider.RootPath, ref.Name, "SKILL.md")
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
