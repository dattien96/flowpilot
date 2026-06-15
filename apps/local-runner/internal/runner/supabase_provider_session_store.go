package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Phase 5 / 07: durable persistence for the (run, cwd, provider session) mapping that
// backs resume + audit. Writes the workflow_provider_sessions row added by
// 20260615120000_add_workflow_provider_tables.sql via PostgREST upsert, mirroring
// SupabaseWorkflowStore / SupabaseCatalogStore. Request shaping is tested over the
// mocked httpRequestFn; end-to-end against a real Supabase is the deferred live step.

// ProviderSessionRecord is the durable mapping the runner persists when a provider
// session id is established (Codex thread id / real Claude session_id).
type ProviderSessionRecord struct {
	WorkflowRunID     string
	WorkflowStepRunID string
	ProviderKey       string
	ProviderSessionID string
	ProviderThreadID  string
	WorkingDirectory  string
	ModelName         string
	Status            string
}

// ProviderSessionStore is the write boundary for provider-session persistence. The
// no-op store is used when Supabase is not configured (offline/demo/tests), exactly
// like the fake catalog — so the adapter code path is identical either way.
type ProviderSessionStore interface {
	UpsertSession(ctx context.Context, rec ProviderSessionRecord) error
}

type noopProviderSessionStore struct{}

func (noopProviderSessionStore) UpsertSession(context.Context, ProviderSessionRecord) error {
	return nil
}

// SupabaseProviderSessionStore upserts workflow_provider_sessions via PostgREST.
type SupabaseProviderSessionStore struct {
	restURL string // "{apiUrl}/rest/v1"
	apiKey  string
}

func NewSupabaseProviderSessionStore(cfg SupabaseWorkspaceConfig, apiKey string) *SupabaseProviderSessionStore {
	base := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	return &SupabaseProviderSessionStore{restURL: base + "/rest/v1", apiKey: apiKey}
}

// ProviderSessionStoreFor returns the live Supabase store when the runner has a
// Supabase workspace config (API URL + a usable key), else a no-op store. Prefers the
// service-role key, falling back to the anon key — same switch as CatalogStoreFor.
func ProviderSessionStoreFor(r *Runner) ProviderSessionStore {
	if r == nil {
		return noopProviderSessionStore{}
	}
	resp, err := r.LoadSupabaseWorkspaceConfigWithSecret()
	if err != nil {
		return noopProviderSessionStore{}
	}
	key := strings.TrimSpace(resp.ServiceRoleKey)
	if key == "" {
		key = strings.TrimSpace(resp.AnonKey)
	}
	if strings.TrimSpace(resp.APIURL) == "" || key == "" {
		return noopProviderSessionStore{}
	}
	return NewSupabaseProviderSessionStore(resp.SupabaseWorkspaceConfig, key)
}

func (s *SupabaseProviderSessionStore) headers(prefer string) map[string]string {
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

// UpsertSession inserts-or-merges the (run, provider, cwd) mapping. on_conflict targets
// the plain-column unique index so a re-captured session_id updates the same row.
func (s *SupabaseProviderSessionStore) UpsertSession(ctx context.Context, rec ProviderSessionRecord) error {
	if strings.TrimSpace(rec.WorkflowRunID) == "" || strings.TrimSpace(rec.ProviderKey) == "" {
		return fmt.Errorf("provider session upsert requires workflow_run_id and provider_key")
	}
	row := map[string]any{
		"workflow_run_id":   rec.WorkflowRunID,
		"provider_key":      rec.ProviderKey,
		"working_directory": rec.WorkingDirectory,
		"status":            stringDefault(rec.Status, "active"),
		"updated_at":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	// Optional fields: only send when set (uuid/text columns; empty uuids would error).
	if rec.WorkflowStepRunID != "" {
		row["workflow_step_run_id"] = rec.WorkflowStepRunID
	}
	if rec.ProviderSessionID != "" {
		row["provider_session_id"] = rec.ProviderSessionID
	}
	if rec.ProviderThreadID != "" {
		row["provider_thread_id"] = rec.ProviderThreadID
	}
	if rec.ModelName != "" {
		row["model_name"] = rec.ModelName
	}

	body, err := json.Marshal(row)
	if err != nil {
		return err
	}
	endpoint := s.restURL + "/workflow_provider_sessions?on_conflict=workflow_run_id,provider_key,working_directory"
	status, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=merge-duplicates,return=minimal"), body)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("supabase upsert provider session failed: status %d: %s", status, string(respBody))
	}
	return nil
}
