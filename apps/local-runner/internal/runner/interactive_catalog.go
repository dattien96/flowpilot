package runner

import "context"

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

func (c *interactiveCatalog) ListWorkflows(_ context.Context, projectID string) ([]Workflow, error) {
	if w, ok := c.workflows[projectID]; ok {
		return w, nil
	}
	return []Workflow{}, nil
}

func (c *interactiveCatalog) ListSteps(_ context.Context, workflowID string) ([]Step, error) {
	if s, ok := c.steps[workflowID]; ok {
		return s, nil
	}
	return []Step{}, nil
}

func (c *interactiveCatalog) listSkills() []ProviderSkill { return c.skills }

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
