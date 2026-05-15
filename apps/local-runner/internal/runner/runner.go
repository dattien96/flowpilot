package runner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const Version = "dev"

type Runner struct {
	workspace string
	startedAt time.Time
}

func New(workspace string) (*Runner, error) {
	resolved, err := ResolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	return &Runner{
		workspace: resolved,
		startedAt: time.Now().UTC(),
	}, nil
}

func (r *Runner) Health() Health {
	return Health{
		Status:        "online",
		RunnerVersion: Version,
		Cwd:           r.workspace,
		Os:            runtime.GOOS,
		StartedAt:     r.startedAt.Format(time.RFC3339Nano),
	}
}

func (r *Runner) DetectProviders(ctx context.Context) ([]Provider, error) {
	specs := []providerSpec{
		{
			Key:         "codex",
			Label:       "Codex",
			BinaryName:  "codex",
			InstallHint: "Install the Codex CLI, log in, and restart the runner.",
		},
		{
			Key:         "claude",
			Label:       "Claude Code",
			BinaryName:  "claude",
			InstallHint: "Install the Claude Code CLI, log in, and restart the runner.",
		},
		{
			Key:         "gemini",
			Label:       "Gemini",
			BinaryName:  "gemini",
			InstallHint: "Install the Gemini CLI, log in, and restart the runner.",
		},
	}

	providers := make([]Provider, 0, len(specs))
	for _, spec := range specs {
		provider := Provider{
			Key:         spec.Key,
			Label:       spec.Label,
			AuthStatus:  "unknown",
			InstallHint: spec.InstallHint,
		}

		binaryPath, err := exec.LookPath(spec.BinaryName)
		if err != nil {
			provider.BinaryPath = ""
			provider.Installed = false
			providers = append(providers, provider)
			continue
		}

		provider.Installed = true
		provider.BinaryPath = binaryPath
		provider.Version = resolveVersion(ctx, binaryPath)
		providers = append(providers, provider)
	}

	return providers, nil
}

func (r *Runner) ListSkills() ([]Skill, error) {
	entries, err := discoverMarkdownEntries(filepath.Join(r.workspace, ".agents", "skills"), true)
	if err != nil {
		return nil, err
	}

	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		skills = append(skills, Skill{
			ID:          entry.ID,
			Name:        entry.Name,
			FilePath:    entry.FilePath,
			Description: entry.Description,
			Tags:        entry.Tags,
		})
	}

	return skills, nil
}

func (r *Runner) ListFlows() ([]Flow, error) {
	entries, err := discoverMarkdownEntries(filepath.Join(r.workspace, ".agents", "flows"), false)
	if err != nil {
		return nil, err
	}

	flows := make([]Flow, 0, len(entries))
	for _, entry := range entries {
		flows = append(flows, Flow{
			ID:          entry.ID,
			Name:        entry.Name,
			FilePath:    entry.FilePath,
			Description: entry.Description,
			Steps:       entry.Steps,
		})
	}

	return flows, nil
}

type providerSpec struct {
	Key         string
	Label       string
	BinaryName  string
	InstallHint string
}

type markdownEntry struct {
	ID          string
	Name        string
	FilePath    string
	Description string
	Tags        []string
	Steps       []string
}

func ResolveWorkspace(workspace string) (string, error) {
	if workspace != "" {
		return filepath.Abs(workspace)
	}

	if envWorkspace := os.Getenv("FLOWPILOT_WORKSPACE"); envWorkspace != "" {
		return filepath.Abs(envWorkspace)
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}

	resolved, err := resolveByWalkingUp(current)
	if err != nil {
		return filepath.Abs(current)
	}

	return resolved, nil
}

func resolveByWalkingUp(start string) (string, error) {
	current := start

	for {
		agentsPath := filepath.Join(current, ".agents")
		if info, err := os.Stat(agentsPath); err == nil && info.IsDir() {
			return filepath.Abs(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("unable to resolve workspace root")
		}
		current = parent
	}
}

func resolveVersion(ctx context.Context, binaryPath string) string {
	command := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := command.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}

	version := strings.TrimSpace(string(output))
	if version == "" && err != nil {
		return ""
	}

	return version
}

func discoverMarkdownEntries(baseDir string, skills bool) ([]markdownEntry, error) {
	info, err := os.Stat(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []markdownEntry{}, nil
		}
		return nil, err
	}

	if !info.IsDir() {
		return []markdownEntry{}, nil
	}

	entries := make([]markdownEntry, 0)
	walkErr := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		name := strings.ToLower(d.Name())
		if skills && name != "skill.md" {
			return nil
		}
		if !skills && !strings.HasSuffix(name, ".md") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		entry := parseMarkdownEntry(baseDir, path, string(raw))
		entries = append(entries, entry)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name < entries[right].Name
	})

	return entries, nil
}

func parseMarkdownEntry(baseDir, path, contents string) markdownEntry {
	id := strings.TrimSuffix(filepath.Base(filepath.Dir(path)), filepath.Ext(filepath.Base(path)))
	if id == "" || strings.EqualFold(id, "skills") || strings.EqualFold(id, "flows") {
		id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	}

	name := ""
	description := ""
	bullets := make([]string, 0, 8)

	lines := strings.Split(contents, "\n")
	inFrontMatter := false
	frontMatterDone := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index == 0 && trimmed == "---" {
			inFrontMatter = true
			continue
		}
		if inFrontMatter && trimmed == "---" {
			inFrontMatter = false
			frontMatterDone = true
			continue
		}
		if inFrontMatter {
			if value, ok := strings.CutPrefix(trimmed, "name:"); ok && name == "" {
				name = strings.TrimSpace(value)
				continue
			}
			if value, ok := strings.CutPrefix(trimmed, "description:"); ok && description == "" {
				description = strings.TrimSpace(value)
				continue
			}
		}

		if !frontMatterDone {
			if strings.HasPrefix(trimmed, "# ") && name == "" {
				name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
				continue
			}
			if strings.HasPrefix(trimmed, "- ") && len(bullets) < 6 {
				bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			}
			continue
		}

		if name == "" && strings.HasPrefix(trimmed, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if description == "" && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") {
			description = trimmed
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && len(bullets) < 6 {
			bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}

	if name == "" {
		name = strings.ReplaceAll(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "-", " ")
	}

	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		relPath = path
	}

	return markdownEntry{
		ID:          id,
		Name:        name,
		FilePath:    filepath.ToSlash(relPath),
		Description: description,
		Tags:        deriveTags(relPath),
		Steps:       bullets,
	}
}

func deriveTags(relPath string) []string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimSuffix(part, ".md"))
		if part == "" || strings.EqualFold(part, "skill") {
			continue
		}
		tags = append(tags, part)
	}
	return tags
}
