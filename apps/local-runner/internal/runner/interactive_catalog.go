package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Phase 2 (04-02) catalog source — the offline fake. Implements CatalogStore
// (projects/workflows/steps) so it is interchangeable with SupabaseCatalogStore:
// `CatalogStoreFor` picks this when Supabase is not configured, the live store
// otherwise (04-08 A1). It also serves the local skill list (listSkills).
type interactiveCatalog struct {
	projects  []Project
	workflows map[string][]Workflow
	steps     map[string][]Step
	skills    []ProviderSkill
}

func newInteractiveCatalog() *interactiveCatalog {
	return &interactiveCatalog{
		projects: []Project{
			{ID: "proj-web", Name: "Acme Web App", Path: "/Users/dev/acme-web", Model: "gpt-5.4", Platform: "reactjs"},
			{ID: "proj-android", Name: "Acme Android", Path: "/Users/dev/acme-android", Model: "gpt-5.4", Platform: "android"},
		},
		workflows: map[string][]Workflow{
			"proj-web": {
				{ID: "wf-feature", ProjectID: "proj-web", Name: "Implement Feature", Description: "Plan → code → test → summarize."},
				{ID: "wf-bugfix", ProjectID: "proj-web", Name: "Fix Bug", Description: "Reproduce → patch → verify."},
			},
			"proj-android": {
				{ID: "wf-screen", ProjectID: "proj-android", Name: "Build Screen", Description: "Compose UI → wire VM → test."},
			},
		},
		// BUG-229: a direct single-step launch resolves its model from the step
		// ONLY (no Project fallback), matching step_definitions.model being a
		// mandatory field on real, well-formed data (Task-183 T-2). Every fixture
		// step below carries the same model the project used to supply via the
		// now-removed fallback, so single-step launches in the tests below keep
		// resolving to the identical model/provider they did before.
		steps: map[string][]Step{
			"wf-feature": {
				{ID: "step-plan", WorkflowID: "wf-feature", Name: "Plan", Order: 1, DefaultSkill: "architect", Model: "gpt-5.4"},
				{ID: "step-code", WorkflowID: "wf-feature", Name: "Implement", Order: 2, DefaultSkill: "coder", Model: "gpt-5.4"},
				{ID: "step-test", WorkflowID: "wf-feature", Name: "Write Tests", Order: 3, Model: "gpt-5.4"},
				{ID: "step-sum", WorkflowID: "wf-feature", Name: "Summarize", Order: 4, Model: "gpt-5.4"},
			},
			"wf-bugfix": {
				{ID: "bug-repro", WorkflowID: "wf-bugfix", Name: "Reproduce", Order: 1, Model: "gpt-5.4"},
				{ID: "bug-patch", WorkflowID: "wf-bugfix", Name: "Patch", Order: 2, DefaultSkill: "coder", Model: "gpt-5.4"},
			},
			"wf-screen": {
				{ID: "scr-ui", WorkflowID: "wf-screen", Name: "Compose UI", Order: 1, Model: "gpt-5.4"},
				{ID: "scr-vm", WorkflowID: "wf-screen", Name: "Wire ViewModel", Order: 2, Model: "gpt-5.4"},
			},
		},
		skills: []ProviderSkill{
			{Name: "architect", Description: "High-level planning & design", Source: "flowpilot"},
			{Name: "coder", Description: "Implementation-focused", Source: "flowpilot"},
			{Name: "reviewer", Description: "Critical code review", Source: "flowpilot"},
			{Name: "test-writer", Description: "Generates tests", Source: "workspace"},
		},
	}
}

// CatalogStore implementation (ctx ignored — the fake is in-memory).
func (c *interactiveCatalog) ListProjects(context.Context) ([]Project, error) {
	return c.projects, nil
}

func (c *interactiveCatalog) CreateProject(_ context.Context, input CreateProjectInput) (Project, error) {
	name := strings.TrimSpace(input.Name)
	dir := strings.TrimSpace(input.DirectoryPath)
	if name == "" {
		name = "Project"
	}
	p := Project{
		ID:       fmt.Sprintf("proj-%d", len(c.projects)+1),
		Name:     name,
		Path:     dir,
		Model:    input.DefaultModel,
		Platform: input.Platform,
	}
	c.projects = append(c.projects, p)
	return p, nil
}

func (c *interactiveCatalog) ListWorkflows(context.Context) ([]Workflow, error) {
	var out []Workflow
	for _, workflows := range c.workflows {
		out = append(out, workflows...)
	}
	return out, nil
}

func (c *interactiveCatalog) ListSteps(context.Context) ([]Step, error) {
	seen := map[string]Step{}
	for _, steps := range c.steps {
		for _, step := range steps {
			if _, ok := seen[step.ID]; !ok {
				step.WorkflowID = ""
				seen[step.ID] = step
			}
		}
	}
	out := make([]Step, 0, len(seen))
	for _, step := range seen {
		step.Order = len(out) + 1
		out = append(out, step)
	}
	return out, nil
}

func (c *interactiveCatalog) ListWorkflowSteps(_ context.Context, workflowID string) ([]Step, error) {
	steps := c.steps[workflowID]
	out := make([]Step, len(steps))
	copy(out, steps)
	return out, nil
}

// listSkills returns skills for the given provider and workspace directory.
// The interactive API merges:
// - project-local provider skills:
//   - Codex/Gemini from `.agents/skills` as `flowpilot`
//   - Claude from `.claude/skills` as `workspace`
//   - Grok from `.grok/skills` as `workspace` (Grok has the same kind of
//     first-class, doc-recommended skill directory Claude does — Task-214)
//
// - provider-home skills as `provider`
// using project-local > provider precedence by skill name.
func (c *interactiveCatalog) listSkills(provider string, cwd string) []ProviderSkill {
	merged := make([]ProviderSkill, 0, 16)
	seen := make(map[string]struct{})
	appendSkills := func(skills []ProviderSkill) {
		for _, skill := range skills {
			key := strings.ToLower(strings.TrimSpace(skill.Name))
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, skill)
		}
	}

	appendSkills(discoverProjectSkills(provider, cwd))
	// Common flow-pack (.agents/skills) for all providers — BUG-062 F-2.
	// discoverProjectSkills is exclusive per provider, so opencode/claude/grok
	// would otherwise miss the common pack.
	appendSkills(providerSkillsFromDir(filepath.Join(cwd, ".agents", "skills"), "flowpilot"))
	appendSkills(discoverProviderHomeSkills(provider))

	if len(merged) == 0 {
		return slices.Clone(c.skills)
	}

	slices.SortFunc(merged, func(left, right ProviderSkill) int {
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return merged
}

func discoverProjectSkills(provider string, cwd string) []ProviderSkill {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "gemini":
		return providerSkillsFromDir(filepath.Join(cwd, ".agents", "skills"), "flowpilot")
	case "claude":
		return providerSkillsFromDir(filepath.Join(cwd, ".claude", "skills"), "workspace")
	case "grok":
		return providerSkillsFromDir(filepath.Join(cwd, ".grok", "skills"), "workspace")
	case "opencode":
		// Appended last (CP-57)
		return providerSkillsFromDir(filepath.Join(cwd, ".opencode", "skills"), "workspace")
	case "devin":
		// Appended last (CP-70)
		return providerSkillsFromDir(filepath.Join(cwd, ".devin", "skills"), "workspace")
	default:
		return nil
	}
}

func discoverProviderHomeSkills(provider string) []ProviderSkill {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return nil
	}

	homePaths, err := DiscoverProviderAccountHomes(provider)
	if err != nil {
		return nil
	}

	skills := make([]ProviderSkill, 0, 8)
	seenRoots := make(map[string]struct{})
	for _, homePath := range homePaths {
		for _, root := range providerHomeSkillDirs(provider, homePath) {
			cleanRoot := canonicalPathKey(root)
			if cleanRoot == "" {
				continue
			}
			if _, exists := seenRoots[cleanRoot]; exists {
				continue
			}
			seenRoots[cleanRoot] = struct{}{}
			skills = append(skills, providerSkillsFromDir(root, "provider")...)
		}
	}
	return skills
}

func providerHomeSkillDirs(provider string, homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return []string{
			filepath.Join(homePath, "skills"),
			filepath.Join(homePath, ".codex", "skills"),
		}
	case "claude":
		return []string{
			filepath.Join(homePath, ".claude", "skills"),
			filepath.Join(homePath, "skills"),
		}
	case "gemini":
		return []string{
			filepath.Join(homePath, ".gemini", "skills"),
			filepath.Join(homePath, "skills"),
		}
	case "grok":
		// GROK_HOME (homePath here) already points directly at the .grok
		// directory itself, same as CODEX_HOME — so skills live at
		// homePath/skills, not a nested homePath/.grok/skills (Task-214 T-6).
		return []string{
			filepath.Join(homePath, "skills"),
			filepath.Join(homePath, ".grok", "skills"),
		}
	case "opencode":
		// Appended last (CP-57): HOME is user home or config dir; check both
		isConfigDir := strings.HasSuffix(filepath.ToSlash(filepath.Clean(homePath)), ".config/opencode") || strings.HasSuffix(filepath.ToSlash(filepath.Clean(homePath)), "/opencode")
		if isConfigDir {
			return []string{
				filepath.Join(homePath, "skills"),
			}
		}
		return []string{
			filepath.Join(homePath, ".config", "opencode", "skills"),
			filepath.Join(homePath, ".opencode", "skills"),
			filepath.Join(homePath, "skills"),
		}
	case "devin":
		// Appended last (CP-70): homePath is a user home or managed .devinHomeN
		// slot; config-dir style homes resolve skills directly under them.
		isConfigDir := strings.HasSuffix(filepath.ToSlash(filepath.Clean(homePath)), ".config/devin") || strings.HasSuffix(filepath.ToSlash(filepath.Clean(homePath)), "/devin")
		if isConfigDir {
			return []string{
				filepath.Join(homePath, "skills"),
			}
		}
		return []string{
			filepath.Join(homePath, ".config", "devin", "skills"),
			filepath.Join(homePath, ".devin", "skills"),
			filepath.Join(homePath, "skills"),
		}
	default:
		return nil
	}
}

func providerSkillsFromDir(baseDir string, source string) []ProviderSkill {
	entries, err := discoverMarkdownEntries(baseDir, true)
	if err != nil {
		return nil
	}

	skills := make([]ProviderSkill, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		skills = append(skills, ProviderSkill{
			Name:        name,
			Path:        filepath.Join(baseDir, filepath.FromSlash(entry.FilePath)),
			Description: entry.Description,
			Source:      source,
		})
	}
	return skills
}

// stepExists reports whether a step id is known (turn validation).
func (c *interactiveCatalog) stepExists(stepID string) bool {
	for _, steps := range c.steps {
		for _, s := range steps {
			if s.ID == stepID {
				return true
			}
		}
	}
	return false
}
