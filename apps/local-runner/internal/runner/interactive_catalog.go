package runner

import (
	"context"
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
			{ID: "proj-web", Name: "Acme Web App", Path: "/Users/dev/acme-web"},
			{ID: "proj-android", Name: "Acme Android", Path: "/Users/dev/acme-android"},
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
		steps: map[string][]Step{
			"wf-feature": {
				{ID: "step-plan", WorkflowID: "wf-feature", Name: "Plan", Order: 1, DefaultSkill: "architect"},
				{ID: "step-code", WorkflowID: "wf-feature", Name: "Implement", Order: 2, DefaultSkill: "coder"},
				{ID: "step-test", WorkflowID: "wf-feature", Name: "Write Tests", Order: 3},
				{ID: "step-sum", WorkflowID: "wf-feature", Name: "Summarize", Order: 4},
			},
			"wf-bugfix": {
				{ID: "bug-repro", WorkflowID: "wf-bugfix", Name: "Reproduce", Order: 1},
				{ID: "bug-patch", WorkflowID: "wf-bugfix", Name: "Patch", Order: 2, DefaultSkill: "coder"},
			},
			"wf-screen": {
				{ID: "scr-ui", WorkflowID: "wf-screen", Name: "Compose UI", Order: 1},
				{ID: "scr-vm", WorkflowID: "wf-screen", Name: "Wire ViewModel", Order: 2},
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
