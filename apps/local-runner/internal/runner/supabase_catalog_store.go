package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Phase 5 (04-05): the navigator catalog read-path over Supabase. P2 served
// projects/workflows/steps from a fake catalog; this is the live PostgREST read path
// that replaces it at cut-over. Request shaping (URL, filters, select, auth) is
// tested over a mocked transport; end-to-end against a real Supabase is the deferred
// live step.
//
// CatalogStore is the read boundary; the fake catalog (interactive_catalog.go) and
// this Supabase store both satisfy the navigator's needs.
type CatalogStore interface {
	ListProjects(ctx context.Context) ([]Project, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	ListSteps(ctx context.Context) ([]Step, error)
}

type WorkflowStepCatalogStore interface {
	ListWorkflowSteps(ctx context.Context, workflowID string) ([]Step, error)
}

// CatalogStoreFor returns the live SupabaseCatalogStore when the runner has a
// Supabase workspace config (API URL + a usable key), else the offline fake catalog
// (04-08 A1). Prefers the service-role key (server-side reads); falls back to the
// anon key. This is the single switch that turns the real project list on.
func CatalogStoreFor(r *Runner) CatalogStore {
	if r == nil {
		return newInteractiveCatalog()
	}
	resp, err := r.LoadSupabaseWorkspaceConfigWithSecret()
	if err != nil {
		return newInteractiveCatalog()
	}
	key := strings.TrimSpace(resp.ServiceRoleKey)
	if key == "" {
		key = strings.TrimSpace(resp.AnonKey)
	}
	if strings.TrimSpace(resp.APIURL) == "" || key == "" {
		return newInteractiveCatalog()
	}
	return NewSupabaseCatalogStore(resp.SupabaseWorkspaceConfig, key)
}

// SupabaseCatalogStore reads the navigator catalog from Supabase via PostgREST.
type SupabaseCatalogStore struct {
	restURL string
	apiKey  string
}

func NewSupabaseCatalogStore(cfg SupabaseWorkspaceConfig, apiKey string) *SupabaseCatalogStore {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	return &SupabaseCatalogStore{restURL: base + "/rest/v1", apiKey: apiKey}
}

func (s *SupabaseCatalogStore) headers() map[string]string {
	return map[string]string{
		"apikey":        s.apiKey,
		"Authorization": "Bearer " + s.apiKey,
		"Accept":        "application/json",
	}
}

func (s *SupabaseCatalogStore) getJSON(ctx context.Context, endpoint string, out any) error {
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(), nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase catalog read failed: status %d: %s", status, string(body))
	}
	return json.Unmarshal(body, out)
}

func (s *SupabaseCatalogStore) ListProjects(ctx context.Context) ([]Project, error) {
	endpoint := s.restURL + "/projects?select=id,name,directory_path,default_model,project_workspace_bindings(local_path)&order=name.asc"
	var raw []struct {
		ID                    string  `json:"id"`
		Name                  string  `json:"name"`
		DirectoryPath         string  `json:"directory_path"`
		DefaultModel          *string `json:"default_model"`
		ProjectWorkspaceBinds []struct {
			LocalPath string `json:"local_path"`
		} `json:"project_workspace_bindings"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Project, len(raw))
	for i, r := range raw {
		candidates := make([]string, 0, len(r.ProjectWorkspaceBinds)+1)
		for _, binding := range r.ProjectWorkspaceBinds {
			candidates = append(candidates, binding.LocalPath)
		}
		candidates = append(candidates, r.DirectoryPath)
		model := ""
		if r.DefaultModel != nil {
			model = *r.DefaultModel
		}
		out[i] = Project{ID: r.ID, Name: r.Name, Path: chooseUsableProjectPath(candidates), Model: model}
	}
	return out, nil
}

func chooseUsableProjectPath(candidates []string) string {
	fallback := ""
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		if fallback == "" {
			fallback = path
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return fallback
}

func (s *SupabaseCatalogStore) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	endpoint := s.restURL + "/workflows?created_by=neq.flowpilot-runtime&select=id,project_id,name,description,model_override,yolo_mode&order=created_at.desc"
	var raw []struct {
		ID            string  `json:"id"`
		ProjectID     string  `json:"project_id"`
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		ModelOverride *string `json:"model_override"`
		YoloMode      bool    `json:"yolo_mode"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Workflow, len(raw))
	for i, r := range raw {
		model := ""
		if r.ModelOverride != nil {
			model = *r.ModelOverride
		}
		out[i] = Workflow{ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Description: r.Description, Model: model, YoloMode: r.YoloMode}
	}
	return out, nil
}

func (s *SupabaseCatalogStore) ListSteps(ctx context.Context) ([]Step, error) {
	endpoint := s.restURL + "/step_definitions?select=step_type,name,model,yolo_mode&order=name.asc"
	var raw []struct {
		StepType string  `json:"step_type"`
		Name     string  `json:"name"`
		Model    *string `json:"model"`
		YoloMode bool    `json:"yolo_mode"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Step, len(raw))
	for i, r := range raw {
		model := ""
		if r.Model != nil {
			model = *r.Model
		}
		out[i] = Step{ID: r.StepType, Name: r.Name, Order: i + 1, Model: model, YoloMode: r.YoloMode}
	}
	return out, nil
}

func (s *SupabaseCatalogStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]Step, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_steps?workflow_id=eq.%s&is_enabled=is.true&select=id,workflow_id,step_type,order_index,step_definitions(name,required_skills,model,yolo_mode)&order=order_index.asc",
		s.restURL,
		workflowID,
	)
	var raw []struct {
		ID              string `json:"id"`
		WorkflowID      string `json:"workflow_id"`
		StepType        string `json:"step_type"`
		OrderIndex      int    `json:"order_index"`
		StepDefinitions *struct {
			Name           string   `json:"name"`
			RequiredSkills []string `json:"required_skills"`
			Model          *string  `json:"model"`
			YoloMode       bool     `json:"yolo_mode"`
		} `json:"step_definitions"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Step, len(raw))
	for i, r := range raw {
		name := r.StepType
		defaultSkill := ""
		model := ""
		if r.StepDefinitions != nil {
			if strings.TrimSpace(r.StepDefinitions.Name) != "" {
				name = r.StepDefinitions.Name
			}
			if len(r.StepDefinitions.RequiredSkills) > 0 {
				defaultSkill = r.StepDefinitions.RequiredSkills[0]
			}
			if r.StepDefinitions.Model != nil {
				model = *r.StepDefinitions.Model
			}
		}
		out[i] = Step{
			ID:           r.ID,
			WorkflowID:   r.WorkflowID,
			Name:         name,
			Order:        r.OrderIndex + 1,
			DefaultSkill: defaultSkill,
			Model:        model,
			YoloMode:     r.StepDefinitions != nil && r.StepDefinitions.YoloMode,
		}
	}
	return out, nil
}
