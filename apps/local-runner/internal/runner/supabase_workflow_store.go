package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Phase 5 (04-05): the live Supabase-backed WorkflowStore. The runner reads/writes
// run/step/log state directly via PostgREST (`{apiUrl}/rest/v1/...`), replacing the
// Admin-Web-server / edge-function path so orchestration is not split across tiers.
//
// Trust model: the runner is a local process; it authenticates to Supabase with a
// key held in the OS secret store (supabase:workspace:service-role-key). 04-05
// prefers a least-privilege / RLS-scoped key over a broad service-role key — the
// store takes whatever key it is handed, so the deployment chooses. The request
// shaping (URL, headers, body, PostgREST filters) is exercised by tests over a
// mocked httpRequestFn; end-to-end reads/writes against a real Supabase instance
// (and golden parity vs the TS path) are deferred to a live environment per the
// 04-05 DOD.
type SupabaseWorkflowStore struct {
	restURL string // "{apiUrl}/rest/v1"
	apiKey  string
}

// NewSupabaseWorkflowStore builds the store from the workspace config + the resolved
// API key (service-role or RLS-scoped).
func NewSupabaseWorkflowStore(cfg SupabaseWorkspaceConfig, apiKey string) *SupabaseWorkflowStore {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	return &SupabaseWorkflowStore{restURL: base + "/rest/v1", apiKey: apiKey}
}

func (s *SupabaseWorkflowStore) headers(prefer string) map[string]string {
	h := map[string]string{
		"apikey":        s.apiKey,
		"Authorization": "Bearer " + s.apiKey,
		"Content-Type":  "application/json",
	}
	if prefer != "" {
		h["Prefer"] = prefer
	}
	return h
}

// dbStep is the PostgREST row shape for workflow_run_steps. requires_approval lives
// on the joined workflow_steps definition, embedded via the select.
type dbStep struct {
	ID            string  `json:"id"`
	StepType      string  `json:"step_type"`
	Status        string  `json:"status"`
	StartedAt     *string `json:"started_at"`
	RetryCount    int     `json:"retry_count"`
	RejectionNote *string `json:"rejection_note"`
	WorkflowSteps *struct {
		RequiresApproval bool `json:"requires_approval"`
	} `json:"workflow_steps"`
}

// LoadRunSteps reads a run's steps in execution order.
func (s *SupabaseWorkflowStore) LoadRunSteps(ctx context.Context, runID string) ([]RuntimeWorkflowStep, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_run_steps?workflow_run_id=eq.%s&order=execution_order_index.asc&select=id,step_type,status,started_at,retry_count,rejection_note,workflow_steps(requires_approval)",
		s.restURL, runID,
	)
	status, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("supabase load steps failed: status %d: %s", status, string(body))
	}
	var rows []dbStep
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase load steps decode: %w", err)
	}
	out := make([]RuntimeWorkflowStep, len(rows))
	for i, r := range rows {
		step := RuntimeWorkflowStep{
			ID:         r.ID,
			StepType:   r.StepType,
			Status:     RuntimeWorkflowStepStatus(r.Status),
			RetryCount: r.RetryCount,
		}
		if r.StartedAt != nil {
			step.StartedAt = *r.StartedAt
		}
		if r.RejectionNote != nil {
			step.RejectionNote = *r.RejectionNote
		}
		if r.WorkflowSteps != nil {
			step.RequiresApproval = r.WorkflowSteps.RequiresApproval
		}
		out[i] = step
	}
	return out, nil
}

// nilIfEmpty maps the "" == null sentinel to a JSON null.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// buildStepPatchBody renders a WorkflowStepPatch to a PostgREST PATCH body. Only
// fields the patch sets are included; a non-nil pointer to "" becomes JSON null.
func buildStepPatchBody(p WorkflowStepPatch) map[string]any {
	body := map[string]any{"status": string(p.Status)}
	if p.StartedAt != nil {
		body["started_at"] = nilIfEmpty(*p.StartedAt)
	}
	if p.FinishedAt != nil {
		body["finished_at"] = nilIfEmpty(*p.FinishedAt)
	}
	if p.RejectionNote != nil {
		body["rejection_note"] = nilIfEmpty(*p.RejectionNote)
	}
	if p.RetryCount != nil {
		body["retry_count"] = *p.RetryCount
	}
	return body
}

// ApplyStepTransition patches one step row (idempotent: re-applying the same patch
// converges to the same row).
func (s *SupabaseWorkflowStore) ApplyStepTransition(ctx context.Context, _ string, t WorkflowStepTransition) error {
	endpoint := fmt.Sprintf("%s/workflow_run_steps?id=eq.%s", s.restURL, t.StepID)
	payload, err := json.Marshal(buildStepPatchBody(t.Patch))
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase apply transition failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// SetRunStatus patches the run-level status + finished_at.
func (s *SupabaseWorkflowStore) SetRunStatus(ctx context.Context, runID string, runStatus WorkflowRunStatus, finishedAt string) error {
	endpoint := fmt.Sprintf("%s/workflow_runs?id=eq.%s", s.restURL, runID)
	payload, err := json.Marshal(map[string]any{
		"status":      string(runStatus),
		"finished_at": nilIfEmpty(finishedAt),
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPatch, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase set run status failed: status %d: %s", status, string(respBody))
	}
	return nil
}

// AppendLog inserts a step log row.
func (s *SupabaseWorkflowStore) AppendLog(ctx context.Context, stepID string, log WorkflowLog) error {
	endpoint := s.restURL + "/workflow_run_logs"
	payload, err := json.Marshal(map[string]any{
		"workflow_run_step_id": stepID,
		"log_level":            string(log.LogLevel),
		"message":              log.Message,
	})
	if err != nil {
		return err
	}
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("return=minimal"), payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase append log failed: status %d: %s", status, string(respBody))
	}
	return nil
}
