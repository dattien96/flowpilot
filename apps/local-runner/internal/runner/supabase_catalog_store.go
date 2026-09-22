package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
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

type ProjectCreatorStore interface {
	CreateProject(ctx context.Context, input CreateProjectInput) (Project, error)
}

type CreateProjectInput struct {
	Name          string `json:"name"`
	DirectoryPath string `json:"directoryPath"`
	Platform      string `json:"platform,omitempty"`
	DefaultModel  string `json:"defaultModel,omitempty"`
	Description   string `json:"description,omitempty"`
	RepositoryURL string `json:"repositoryUrl,omitempty"`
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
	endpoint := s.restURL + "/projects?select=id,name,directory_path,default_model,platform,project_workspace_bindings(local_path)&order=name.asc"
	var raw []struct {
		ID                    string  `json:"id"`
		Name                  string  `json:"name"`
		DirectoryPath         string  `json:"directory_path"`
		DefaultModel          *string `json:"default_model"`
		Platform              *string `json:"platform"`
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
		platform := ""
		if r.Platform != nil {
			platform = *r.Platform
		}
		out[i] = Project{ID: r.ID, Name: r.Name, Path: chooseUsableProjectPath(candidates), Model: model, Platform: platform}
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
	endpoint := s.restURL + "/step_definitions?select=step_type,name,model,yolo_mode,node_id,behavior_id,agent_ref&order=name.asc"
	var raw []struct {
		StepType   string  `json:"step_type"`
		Name       string  `json:"name"`
		Model      *string `json:"model"`
		YoloMode   bool    `json:"yolo_mode"`
		NodeID     *string `json:"node_id"`
		BehaviorID *string `json:"behavior_id"`
		AgentRef   *string `json:"agent_ref"`
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
		if r.NodeID != nil {
			out[i].NodeID = *r.NodeID
		}
		if r.BehaviorID != nil {
			out[i].BehaviorID = *r.BehaviorID
		}
		if r.AgentRef != nil {
			out[i].AgentRef = *r.AgentRef
		}
	}
	return out, nil
}

func (s *SupabaseCatalogStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]Step, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_steps?workflow_id=eq.%s&is_enabled=is.true&select=id,workflow_id,step_type,order_index,step_definitions(name,required_skills,model,yolo_mode,node_id,agent_ref)&order=order_index.asc",
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
			NodeID         *string  `json:"node_id"`
			AgentRef       *string  `json:"agent_ref"`
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
		nodeID := ""
		agentRef := ""
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
			if r.StepDefinitions.NodeID != nil {
				nodeID = *r.StepDefinitions.NodeID
			}
			if r.StepDefinitions.AgentRef != nil {
				agentRef = *r.StepDefinitions.AgentRef
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
			NodeID:       nodeID,
			AgentRef:     agentRef,
		}
	}
	return out, nil
}

// newProjectLegacyID returns the `project_<18 hex>` legacy text id every
// projects insert must carry — the same shape the TS repositories generate
// with `project_${crypto.randomUUID().replaceAll("-","").slice(0,18)}`.
func newProjectLegacyID() string {
	var b [9]byte // 9 bytes -> 18 hex chars
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is near-impossible; fall back to a timestamp
		// suffix so the unique constraint still has distinct input.
		return fmt.Sprintf("project_%018d", time.Now().UnixNano())
	}
	return "project_" + hex.EncodeToString(b[:])
}

func (s *SupabaseCatalogStore) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	name := strings.TrimSpace(input.Name)
	dir := strings.TrimSpace(input.DirectoryPath)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	if dir == "" {
		return Project{}, fmt.Errorf("directory path is required")
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = "generic"
	}
	defaultModel := strings.TrimSpace(input.DefaultModel)

	bodyMap := map[string]any{
		"name":                        name,
		"directory_path":              dir,
		"platform":                    platform,
		"status":                      "active",
		"artifact_storage_preference": "supabase",
		// projects.legacy_id / created_by / description / repository_url are
		// NOT NULL with no DB default — same contract the TS insert paths
		// fixed in BUG-135 / BUG-136. Empty string satisfies the constraint.
		"legacy_id":      newProjectLegacyID(),
		"created_by":     "supabase-admin",
		"description":    strings.TrimSpace(input.Description),
		"repository_url": strings.TrimSpace(input.RepositoryURL),
	}
	if defaultModel != "" {
		bodyMap["default_model"] = defaultModel
	}

	bodyJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return Project{}, err
	}

	headers := s.headers()
	headers["Content-Type"] = "application/json"
	headers["Prefer"] = "return=representation"

	status, respBody, err := httpRequestFn(ctx, http.MethodPost, s.restURL+"/projects", headers, bodyJSON)
	if err != nil {
		return Project{}, fmt.Errorf("create project failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return Project{}, fmt.Errorf("create project failed: status %d: %s", status, string(respBody))
	}

	var created []struct {
		ID            string  `json:"id"`
		Name          string  `json:"name"`
		DirectoryPath string  `json:"directory_path"`
		DefaultModel  *string `json:"default_model"`
		Platform      *string `json:"platform"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return Project{}, fmt.Errorf("parse created project response: %w", err)
	}
	if len(created) == 0 {
		return Project{}, fmt.Errorf("create project response contained no project row (status %d)", status)
	}

	projectID := created[0].ID

	// Save workspace binding for local path. A binding failure is NOT fatal —
	// the project exists in Supabase and chat works this session — but it must
	// never be silent: the TUI reports "created and bound", and a missing
	// binding means the project will not match this path on future sessions.
	bindingMap := map[string]any{
		"project_id": projectID,
		"local_path": dir,
		"label":      "Primary",
	}
	bindingJSON, err := json.Marshal(bindingMap)
	if err != nil {
		log.Printf("[supabase] project %q created but binding payload marshal failed: %v", projectID, err)
	} else if bindStatus, bindBody, bindErr := httpRequestFn(ctx, http.MethodPost, s.restURL+"/project_workspace_bindings", headers, bindingJSON); bindErr != nil || bindStatus < 200 || bindStatus >= 300 {
		log.Printf("[supabase] project %q created but workspace binding failed (status=%d): %v %s", projectID, bindStatus, bindErr, string(bindBody))
	}

	return Project{
		ID:       projectID,
		Name:     name,
		Path:     dir,
		Model:    defaultModel,
		Platform: platform,
	}, nil
}
