package tooling

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ToolStatus struct {
	Tool      string `json:"tool"`
	Version   string `json:"version,omitempty"`
	Status    string `json:"status"`     // "ok" | "missing" | "stale"
	CheckedAt string `json:"checked_at"` // RFC3339
}

type CapabilityProfile struct {
	HasGitNexus   bool     `json:"has_gitnexus"`
	HasRTK        bool     `json:"has_rtk"`
	HasNode       bool     `json:"has_node"`
	HasTests      bool     `json:"has_tests"`
	HasSpecs      bool     `json:"has_specs"`
	StructureTier string   `json:"structure_tier"` // "gitnexus" | "fallback"
	DecisionTier  string   `json:"decision_tier"`  // "full" | "git-only"
	Languages     []string `json:"languages"`
}

// CheckTool probes a single named tool and returns its status.
// All exec failures are non-fatal: Status="missing", Version="".
func CheckTool(name string, repoDir string) ToolStatus {
	now := time.Now().UTC().Format(time.RFC3339)
	ts := ToolStatus{Tool: name, CheckedAt: now}

	switch name {
	case "gitnexus":
		// Prefer a native binary; fall back to npx.
		if path, err := exec.LookPath("gitnexus"); err == nil && path != "" {
			out, err := exec.Command("gitnexus", "--version").Output()
			if err == nil {
				ts.Version = strings.TrimSpace(string(out))
				ts.Status = "ok"
				return ts
			}
		}
		out, err := exec.Command("npx", "gitnexus", "--version").Output()
		if err == nil {
			ts.Version = strings.TrimSpace(string(out))
			ts.Status = "ok"
			return ts
		}
		ts.Status = "missing"

	case "rtk":
		out, err := exec.Command("rtk", "--version").Output()
		if err == nil {
			ts.Version = strings.TrimSpace(string(out))
			ts.Status = "ok"
			return ts
		}
		ts.Status = "missing"

	case "node":
		out, err := exec.Command("node", "--version").Output()
		if err == nil {
			ts.Version = strings.TrimSpace(string(out))
			ts.Status = "ok"
			return ts
		}
		ts.Status = "missing"

	case "skill_pack":
		// Presence check only — no version concept.
		skillPath := filepath.Join(repoDir, ".claude", "skills", "flowpilot", "git-commit-format", "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			ts.Status = "ok"
			return ts
		}
		ts.Status = "missing"

	default:
		ts.Status = "missing"
	}

	return ts
}

// CheckAll probes all known tools and persists results to dotFlowpilotDir/tooling.json.
func CheckAll(repoDir, dotFlowpilotDir string) ([]ToolStatus, error) {
	tools := []string{"gitnexus", "rtk", "node", "skill_pack"}
	statuses := make([]ToolStatus, 0, len(tools))
	for _, t := range tools {
		statuses = append(statuses, CheckTool(t, repoDir))
	}

	if err := os.MkdirAll(dotFlowpilotDir, 0o755); err != nil {
		return statuses, err
	}
	dest := filepath.Join(dotFlowpilotDir, "tooling.json")
	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		return statuses, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return statuses, err
	}
	return statuses, nil
}

// CheckGlobal probes machine-global tooling without any project-scoped checks.
func CheckGlobal() []ToolStatus {
	tools := []string{"gitnexus", "rtk", "node"}
	statuses := make([]ToolStatus, 0, len(tools))
	for _, t := range tools {
		statuses = append(statuses, CheckTool(t, ""))
	}
	return statuses
}

// LoadToolingStatus reads dotFlowpilotDir/tooling.json and returns the slice.
func LoadToolingStatus(dotFlowpilotDir string) ([]ToolStatus, error) {
	data, err := os.ReadFile(filepath.Join(dotFlowpilotDir, "tooling.json"))
	if err != nil {
		return nil, err
	}
	var statuses []ToolStatus
	if err := json.Unmarshal(data, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// StatusOf returns the ToolStatus for the named tool, or a zero value if not found.
func StatusOf(statuses []ToolStatus, name string) ToolStatus {
	for _, s := range statuses {
		if s.Tool == name {
			return s
		}
	}
	return ToolStatus{}
}

// ComputeCapabilityProfile derives project capabilities from tool statuses and repo layout.
func ComputeCapabilityProfile(repoDir string, statuses []ToolStatus) CapabilityProfile {
	p := CapabilityProfile{}

	p.HasGitNexus = StatusOf(statuses, "gitnexus").Status == "ok"
	p.HasRTK = StatusOf(statuses, "rtk").Status == "ok"
	p.HasNode = StatusOf(statuses, "node").Status == "ok"

	p.HasTests = detectTests(repoDir)
	p.HasSpecs = detectSpecs(repoDir)

	if p.HasGitNexus {
		p.StructureTier = "gitnexus"
	} else {
		p.StructureTier = "fallback"
	}

	if p.HasSpecs {
		p.DecisionTier = "full"
	} else {
		p.DecisionTier = "git-only"
	}

	p.Languages = detectLanguages(repoDir)
	return p
}

// detectTests returns true when at least one test harness is configured in the repo.
func detectTests(repoDir string) bool {
	if _, err := os.Stat(filepath.Join(repoDir, "go.mod")); err == nil {
		return true
	}
	pkgJSON := filepath.Join(repoDir, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		// Cheap check: presence of "test" in scripts section without full JSON parse.
		if strings.Contains(string(data), `"test"`) {
			return true
		}
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pytest.ini")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		return true
	}
	return false
}

// detectSpecs returns true when at least one SS-*.md spec file exists.
func detectSpecs(repoDir string) bool {
	pattern := filepath.Join(repoDir, "requirements", "05-System-Specs", "SS-*.md")
	matches, err := filepath.Glob(pattern)
	return err == nil && len(matches) > 0
}

// detectLanguages walks the repo (up to one level of subdirs for speed) and
// returns a sorted, deduplicated list of detected language names.
func detectLanguages(repoDir string) []string {
	seen := make(map[string]bool)
	extLang := map[string]string{
		".go":  "go",
		".ts":  "typescript",
		".tsx": "typescript",
		".py":  "python",
		".rs":  "rust",
	}

	// Walk up to depth-2 to keep this fast on large repos.
	_ = filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden dirs and node_modules to avoid noise.
			base := d.Name()
			if base != "." && (strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if lang, ok := extLang[ext]; ok {
			seen[lang] = true
		}
		return nil
	})

	langs := make([]string, 0, len(seen))
	// Stable order: go, typescript, python, rust.
	for _, l := range []string{"go", "typescript", "python", "rust"} {
		if seen[l] {
			langs = append(langs, l)
		}
	}
	return langs
}
