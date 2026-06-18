package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSupabaseWorkflowStoreListProviderSessionsByProject(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotMethod, gotURL string
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		gotMethod, gotURL = method, endpoint
		resp := []map[string]any{
			{
				"workflow_run_id":     "run-1",
				"provider_key":        "claude",
				"provider_session_id": "sess-1",
				"working_directory":   "/workspace",
				"status":              "active",
				"workflow_runs": map[string]any{
					"project_id":  "proj-uuid",
					"workflow_id": "wf-uuid",
				},
			},
		}
		b, _ := json.Marshal(resp)
		return 200, b, nil
	}

	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	sessions, err := store.ListProviderSessionsByProject(context.Background(), "proj-uuid")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if !strings.Contains(gotURL, "/workflow_provider_sessions") ||
		!strings.Contains(gotURL, "workflow_runs!inner") ||
		!strings.Contains(gotURL, "proj-uuid") ||
		!strings.Contains(gotURL, "updated_at.desc") {
		t.Fatalf("endpoint = %s", gotURL)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", s.RunID)
	}
	if s.ProjectID != "proj-uuid" {
		t.Errorf("ProjectID = %q, want proj-uuid", s.ProjectID)
	}
	if s.WorkflowID != "wf-uuid" {
		t.Errorf("WorkflowID = %q, want wf-uuid", s.WorkflowID)
	}
	if s.ProviderKey != "claude" {
		t.Errorf("ProviderKey = %q, want claude", s.ProviderKey)
	}
	if s.ProviderSessionID != "sess-1" {
		t.Errorf("ProviderSessionID = %q, want sess-1", s.ProviderSessionID)
	}
	if string(s.Status) != "active" {
		t.Errorf("Status = %q, want active", s.Status)
	}
}

func TestSupabaseWorkflowStoreListProviderSessionsByProjectEmpty(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	httpRequestFn = func(_ context.Context, _, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	sessions, err := store.ListProviderSessionsByProject(context.Background(), "no-such-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("got %d sessions, want 0", len(sessions))
	}
}

func TestSupabaseWorkflowStoreGetProviderSession(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var calls int
	var gotMethod, gotURL string
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		calls++
		gotMethod, gotURL = method, endpoint
		if calls == 1 {
			resp := []map[string]any{
				{
					"workflow_run_id":     "run-1",
					"provider_key":        "codex",
					"provider_session_id": "sess-1",
					"provider_account_id": "acct-a",
					"working_directory":   "/workspace",
					"status":              "completed",
					"last_prompt":         "hello",
					"last_message":        "done",
					"started_at":          "2026-06-17T10:00:00Z",
					"updated_at":          "2026-06-17T10:05:00Z",
					"run_kind":            "chat",
					"workflow_runs": map[string]any{
						"project_id":  "proj-uuid",
						"workflow_id": "wf-uuid",
					},
				},
			}
			b, _ := json.Marshal(resp)
			return 200, b, nil
		}
		return 200, []byte("[]"), nil
	}

	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected provider session to be found")
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if !strings.Contains(gotURL, "/workflow_provider_sessions") ||
		!strings.Contains(gotURL, "workflow_run_id=eq.run-1") ||
		!strings.Contains(gotURL, "provider_account_id") ||
		!strings.Contains(gotURL, "limit=1") {
		t.Fatalf("endpoint = %s", gotURL)
	}
	if state.RunID != "run-1" || state.ProviderSessionID != "sess-1" || state.ProviderAccountID != "acct-a" {
		t.Fatalf("unexpected state: %+v", state)
	}
	if state.ProjectID != "proj-uuid" || state.WorkflowID != "wf-uuid" {
		t.Fatalf("unexpected workflow join fields: %+v", state)
	}

	state, found, err = store.GetProviderSession(context.Background(), "missing-run")
	if err != nil {
		t.Fatalf("GetProviderSession missing-run: %v", err)
	}
	if found {
		t.Fatalf("expected not found, got %+v", state)
	}
}

// Compile-time check: SupabaseWorkflowStore satisfies SessionHistoryReader.
var _ SessionHistoryReader = (*SupabaseWorkflowStore)(nil)
