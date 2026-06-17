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

// Compile-time check: SupabaseWorkflowStore satisfies SessionHistoryReader.
var _ SessionHistoryReader = (*SupabaseWorkflowStore)(nil)
