package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	ListWorkflows(ctx context.Context, projectID string) ([]Workflow, error)
	ListSteps(ctx context.Context, workflowID string) ([]Step, error)
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
	endpoint := s.restURL + "/projects?select=id,name,path&order=name.asc"
	var rows []Project
	if err := s.getJSON(ctx, endpoint, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SupabaseCatalogStore) ListWorkflows(ctx context.Context, projectID string) ([]Workflow, error) {
	endpoint := fmt.Sprintf("%s/workflows?project_id=eq.%s&select=id,project_id,name,description&order=name.asc", s.restURL, projectID)
	var raw []struct {
		ID          string `json:"id"`
		ProjectID   string `json:"project_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Workflow, len(raw))
	for i, r := range raw {
		out[i] = Workflow{ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Description: r.Description}
	}
	return out, nil
}

func (s *SupabaseCatalogStore) ListSteps(ctx context.Context, workflowID string) ([]Step, error) {
	endpoint := fmt.Sprintf("%s/workflow_steps?workflow_id=eq.%s&select=id,workflow_id,step_type,order_index&order=order_index.asc", s.restURL, workflowID)
	var raw []struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflow_id"`
		StepType   string `json:"step_type"`
		OrderIndex int    `json:"order_index"`
	}
	if err := s.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	out := make([]Step, len(raw))
	for i, r := range raw {
		out[i] = Step{ID: r.ID, WorkflowID: r.WorkflowID, Name: r.StepType, Order: r.OrderIndex}
	}
	return out, nil
}
