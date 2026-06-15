package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// 07 persistence: the workflow_provider_sessions upsert request shaping, exercised over
// the mocked httpRequestFn (end-to-end against a real Supabase is the deferred live step).

func TestSupabaseProviderSessionStoreUpsert(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	var gotMethod, gotURL, gotPrefer string
	var gotBody map[string]any
	httpRequestFn = func(_ context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		gotMethod, gotURL, gotPrefer = method, endpoint, headers["Prefer"]
		_ = json.Unmarshal(body, &gotBody)
		return 201, []byte(""), nil
	}

	store := NewSupabaseProviderSessionStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	err := store.UpsertSession(context.Background(), ProviderSessionRecord{
		WorkflowRunID: "run-uuid", ProviderKey: "claude", WorkingDirectory: "/w",
		ProviderSessionID: "sess-1", ProviderThreadID: "sess-1", Status: "active",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if !strings.Contains(gotURL, "/workflow_provider_sessions") ||
		!strings.Contains(gotURL, "on_conflict=workflow_run_id,provider_key,working_directory") {
		t.Fatalf("endpoint = %s", gotURL)
	}
	if !strings.Contains(gotPrefer, "merge-duplicates") {
		t.Fatalf("Prefer = %q, want merge-duplicates", gotPrefer)
	}
	if gotBody["workflow_run_id"] != "run-uuid" || gotBody["provider_session_id"] != "sess-1" || gotBody["working_directory"] != "/w" {
		t.Fatalf("body = %+v", gotBody)
	}
	// workflow_step_run_id is intentionally omitted (FK uuid; not sent by the adapter).
	if _, present := gotBody["workflow_step_run_id"]; present {
		t.Fatalf("workflow_step_run_id should be omitted, body = %+v", gotBody)
	}
}

func TestSupabaseProviderSessionStoreUpsertRequiresKeys(t *testing.T) {
	store := NewSupabaseProviderSessionStore(SupabaseWorkspaceConfig{APIURL: "https://x"}, "k")
	if err := store.UpsertSession(context.Background(), ProviderSessionRecord{}); err == nil {
		t.Fatalf("expected an error when workflow_run_id / provider_key are missing")
	}
}

func TestProviderSessionStoreForNilRunner(t *testing.T) {
	store := ProviderSessionStoreFor(nil)
	if _, ok := store.(noopProviderSessionStore); !ok {
		t.Fatalf("nil runner should yield the no-op store, got %T", store)
	}
	if err := store.UpsertSession(context.Background(), ProviderSessionRecord{}); err != nil {
		t.Fatalf("no-op upsert should return nil, got %v", err)
	}
}
