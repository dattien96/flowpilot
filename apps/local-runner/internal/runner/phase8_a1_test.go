package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// customCatalogStore is a test CatalogStore returning canned data or an error.
type customCatalogStore struct {
	projects      []Project
	workflows     []Workflow
	steps         []Step
	workflowSteps map[string][]Step
	err           error
}

type failingWorkflowStore struct{}

func (c customCatalogStore) ListProjects(context.Context) ([]Project, error) {
	return c.projects, c.err
}
func (c customCatalogStore) ListWorkflows(context.Context) ([]Workflow, error) {
	return c.workflows, c.err
}
func (c customCatalogStore) ListSteps(context.Context) ([]Step, error) {
	return c.steps, c.err
}
func (c customCatalogStore) ListWorkflowSteps(_ context.Context, workflowID string) ([]Step, error) {
	return c.workflowSteps[workflowID], c.err
}

func (f failingWorkflowStore) LoadRunSteps(context.Context, string) ([]RuntimeWorkflowStep, error) {
	return nil, context.DeadlineExceeded
}

func (f failingWorkflowStore) ApplyStepTransition(context.Context, string, WorkflowStepTransition) error {
	return context.DeadlineExceeded
}

func (f failingWorkflowStore) SetRunStatus(context.Context, string, WorkflowRunStatus, string) error {
	return context.DeadlineExceeded
}

func (f failingWorkflowStore) AppendLog(context.Context, string, WorkflowLog) error {
	return context.DeadlineExceeded
}

func newCatalogTestServer(t *testing.T, catalog CatalogStore) *httptest.Server {
	t.Helper()
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, nil)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// The service serves projects from the injected CatalogStore, not the fake catalog.
func TestServiceUsesInjectedCatalogStore(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{
		projects: []Project{{ID: "p9", Name: "My Real Project", Path: "/work/real"}},
	})
	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("projects status=%d", status)
	}
	if !strings.Contains(string(body), "My Real Project") || strings.Contains(string(body), "Acme Web App") {
		t.Fatalf("expected injected projects (not the fake catalog), got %s", body)
	}
}

// The desktop workflow selector mirrors Admin Web's /workflows screen: workflows
// are a global catalog and are not scoped to the selected project.
func TestServiceListsInjectedWorkflowsGlobally(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{
		workflows: []Workflow{{ID: "wf9", Name: "Global Workflow"}},
	})
	status, body := doJSON(t, "GET", srv.URL+"/client/workflows", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Global Workflow") {
		t.Fatalf("expected injected global workflows, got status=%d body=%s", status, body)
	}
}

// The desktop single-step selector mirrors Admin Web's /workflow-steps screen:
// step definitions are global and not scoped to a selected workflow.
func TestServiceListsInjectedStepsGlobally(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{
		steps: []Step{{ID: "analysis", Name: "Analysis"}},
	})
	status, body := doJSON(t, "GET", srv.URL+"/client/steps", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Analysis") {
		t.Fatalf("expected injected global steps, got status=%d body=%s", status, body)
	}
}

// A catalog read error surfaces as a typed 502 catalog_unavailable.
func TestServiceCatalogErrorSurfaces(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{err: context.DeadlineExceeded})
	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusBadGateway || !strings.Contains(string(body), "catalog_unavailable") {
		t.Fatalf("expected 502 catalog_unavailable, got status=%d body=%s", status, body)
	}
}

// Skills still come from the local catalog regardless of the projects store.
func TestSkillsServedLocallyWithInjectedStore(t *testing.T) {
	srv := newCatalogTestServer(t, customCatalogStore{})
	status, body := doJSON(t, "GET", srv.URL+"/client/provider-skills", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "architect") {
		t.Fatalf("skills status=%d body=%s", status, body)
	}
}

func TestSkillsMergeClaudeProjectAndProviderHomeWithPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", "")

	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write claude config: %v", err)
	}

	projectDir := t.TempDir()
	writeSkillFile := func(baseDir string, folder string, frontMatterName string, description string) {
		t.Helper()
		skillDir := filepath.Join(baseDir, folder)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", skillDir, err)
		}
		body := "---\nname: " + frontMatterName + "\ndescription: " + description + "\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write skill %s: %v", skillDir, err)
		}
	}

	writeSkillFile(filepath.Join(homeDir, ".claude", "skills"), "shared", "shared", "provider shared")
	writeSkillFile(filepath.Join(homeDir, ".claude", "skills"), "provider-only", "provider-only", "provider only")
	writeSkillFile(filepath.Join(projectDir, ".agents", "skills"), "shared", "shared", "flowpilot shared")
	writeSkillFile(filepath.Join(projectDir, ".agents", "skills"), "flowpilot-only", "flowpilot-only", "flowpilot only")
	writeSkillFile(filepath.Join(projectDir, ".claude", "skills"), "shared", "shared", "workspace shared")
	writeSkillFile(filepath.Join(projectDir, ".claude", "skills"), "workspace-only", "workspace-only", "workspace only")

	srv := newCatalogTestServer(t, customCatalogStore{})
	status, body := doJSON(t, "GET", srv.URL+"/client/provider-skills?provider=claude&cwd="+url.QueryEscape(projectDir), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("skills status=%d body=%s", status, body)
	}

	var skills []ProviderSkill
	if err := json.Unmarshal(body, &skills); err != nil {
		t.Fatalf("decode skills: %v", err)
	}

	byName := make(map[string]ProviderSkill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}

	if got := byName["shared"]; got.Source != "workspace" || got.Description != "workspace shared" {
		t.Fatalf("shared skill = %+v, want workspace precedence", got)
	}
	if got := byName["provider-only"]; got.Source != "provider" {
		t.Fatalf("provider-only skill = %+v, want provider source", got)
	}
	if _, ok := byName["workspace-only"]; !ok {
		t.Fatalf("workspace-only skill missing: %+v", skills)
	}
	if _, ok := byName["flowpilot-only"]; ok {
		t.Fatalf("flowpilot-only should not load for claude project skills: %+v", skills)
	}
}

func TestSkillsMergeCodexProjectAgentsAndProviderHomeWithPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", "")
	codexHome := filepath.Join(homeDir, ".codexHome")

	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("[core]\nmodel = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}

	projectDir := t.TempDir()
	writeSkillFile := func(baseDir string, folder string, frontMatterName string, description string) {
		t.Helper()
		skillDir := filepath.Join(baseDir, folder)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", skillDir, err)
		}
		body := "---\nname: " + frontMatterName + "\ndescription: " + description + "\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write skill %s: %v", skillDir, err)
		}
	}

	writeSkillFile(filepath.Join(codexHome, "skills"), "shared", "shared", "provider shared")
	writeSkillFile(filepath.Join(codexHome, "skills"), "provider-only", "provider-only", "provider only")
	writeSkillFile(filepath.Join(projectDir, ".agents", "skills"), "shared", "shared", "project shared")
	writeSkillFile(filepath.Join(projectDir, ".agents", "skills"), "project-only", "project-only", "project only")
	writeSkillFile(filepath.Join(projectDir, ".codex", "skills"), "ignored-local", "ignored-local", "ignored local")

	srv := newCatalogTestServer(t, customCatalogStore{})
	status, body := doJSON(t, "GET", srv.URL+"/client/provider-skills?provider=codex&cwd="+url.QueryEscape(projectDir), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("skills status=%d body=%s", status, body)
	}

	var skills []ProviderSkill
	if err := json.Unmarshal(body, &skills); err != nil {
		t.Fatalf("decode skills: %v", err)
	}

	byName := make(map[string]ProviderSkill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}

	if got := byName["shared"]; got.Source != "flowpilot" || got.Description != "project shared" {
		t.Fatalf("shared skill = %+v, want .agents/skills precedence", got)
	}
	if got := byName["project-only"]; got.Source != "flowpilot" {
		t.Fatalf("project-only skill = %+v, want flowpilot source", got)
	}
	if got := byName["provider-only"]; got.Source != "provider" {
		t.Fatalf("provider-only skill = %+v, want provider source", got)
	}
	if _, ok := byName["ignored-local"]; ok {
		t.Fatalf(".codex/skills should not load for codex project skills: %+v", skills)
	}
}

func TestSkillsMergeGeminiProjectAgentsAndProviderHomeWithPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", "")

	if err := os.MkdirAll(filepath.Join(homeDir, ".gemini"), 0o755); err != nil {
		t.Fatalf("mkdir gemini config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".gemini", "settings.json"), []byte(`{"user":"test"}`), 0o644); err != nil {
		t.Fatalf("write gemini settings: %v", err)
	}

	projectDir := t.TempDir()
	writeSkillFile := func(baseDir string, folder string, frontMatterName string, description string) {
		t.Helper()
		skillDir := filepath.Join(baseDir, folder)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", skillDir, err)
		}
		body := "---\nname: " + frontMatterName + "\ndescription: " + description + "\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write skill %s: %v", skillDir, err)
		}
	}

	writeSkillFile(filepath.Join(homeDir, ".gemini", "skills"), "provider-only", "provider-only", "provider only")
	writeSkillFile(filepath.Join(projectDir, ".agents", "skills"), "project-only", "project-only", "project only")
	writeSkillFile(filepath.Join(projectDir, ".gemini", "skills"), "ignored-local", "ignored-local", "ignored local")

	srv := newCatalogTestServer(t, customCatalogStore{})
	status, body := doJSON(t, "GET", srv.URL+"/client/provider-skills?provider=gemini&cwd="+url.QueryEscape(projectDir), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("skills status=%d body=%s", status, body)
	}

	var skills []ProviderSkill
	if err := json.Unmarshal(body, &skills); err != nil {
		t.Fatalf("decode skills: %v", err)
	}

	byName := make(map[string]ProviderSkill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}

	if got := byName["project-only"]; got.Source != "flowpilot" {
		t.Fatalf("project-only skill = %+v, want flowpilot source", got)
	}
	if got := byName["provider-only"]; got.Source != "provider" {
		t.Fatalf("provider-only skill = %+v, want provider source", got)
	}
	if _, ok := byName["ignored-local"]; ok {
		t.Fatalf(".gemini/skills should not load for gemini project skills: %+v", skills)
	}
}

// CatalogStoreFor falls back to the fake catalog with no runner / no Supabase config.
func TestCatalogStoreForFallsBackToFake(t *testing.T) {
	if _, ok := CatalogStoreFor(nil).(*interactiveCatalog); !ok {
		t.Fatal("nil runner should yield the fake catalog")
	}
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := CatalogStoreFor(r).(*interactiveCatalog); !ok {
		t.Fatal("a runner with no Supabase config should yield the fake catalog")
	}
}

// CatalogStoreFor returns the live Supabase store once a workspace config is present.
func TestCatalogStoreForUsesSupabaseWhenConfigured(t *testing.T) {
	tmp := t.TempDir()
	r, err := New(tmp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Write a minimal config with an anon key (no secret needed for the anon path).
	cfg := SupabaseWorkspaceConfig{Version: 1, APIURL: "https://proj.supabase.co", AnonKey: "anon-key"}
	raw, _ := json.Marshal(cfg)
	dir := filepath.Join(tmp, ".flowpilot", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "supabase-config.json"), raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, ok := CatalogStoreFor(r).(*SupabaseCatalogStore); !ok {
		t.Fatalf("a configured runner should yield SupabaseCatalogStore, got %T", CatalogStoreFor(r))
	}
}

func TestStartTurnSurfacesWorkflowStateStoreErrors(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), failingWorkflowStore{})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	runID := startRun(t, srv.URL)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/turns",
		map[string]any{"stepId": "step-plan", "prompt": "hi", "scenario": "normal"}, nil)
	if status != http.StatusBadGateway || !strings.Contains(string(body), "workflow_state_unavailable") {
		t.Fatalf("expected 502 workflow_state_unavailable, got status=%d body=%s", status, body)
	}
}

func TestStartRunResolvesWorkflowLaunchToFirstWorkflowStep(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), customCatalogStore{
		projects:  []Project{{ID: "proj-web", Name: "Acme Web App", Path: "/Users/dev/acme-web", Model: "gpt-5.4"}},
		workflows: []Workflow{{ID: "wf-live", ProjectID: "proj-web", Name: "WF", Model: "gpt-5.4"}},
		workflowSteps: map[string][]Step{
			"wf-live": {
				{ID: "ws-plan", WorkflowID: "wf-live", Name: "plan", Order: 1},
				{ID: "ws-code", WorkflowID: "wf-live", Name: "code", Order: 2},
			},
		},
	}, nil)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:  "proj-web",
		WorkflowID: "wf-live",
		StepID:     "wf-live",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if handle.StepID != "ws-plan" {
		t.Fatalf("handle step id = %q, want ws-plan", handle.StepID)
	}

	store := svc.workflowStore.(*fakeWorkflowStore)
	store.mu.Lock()
	defer store.mu.Unlock()
	steps := store.steps[handle.RunID]
	if len(steps) != 2 || steps[0].ID != "ws-plan" || steps[1].ID != "ws-code" {
		t.Fatalf("seeded steps = %+v, want workflow steps", steps)
	}
}
