package runner

// BUG-500: SupabaseWorkflowStore wrote approvals/questions but never
// implemented the reader interfaces — every resume on that backend saw an
// empty pending-gate set (the `ok` guard turned the missing implementation
// into "no pending gate", structurally promoting waiting nodes).

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func bug500Store(t *testing.T, resp []map[string]any) *SupabaseWorkflowStore {
	t.Helper()
	original := httpRequestFn
	t.Cleanup(func() { httpRequestFn = original })
	httpRequestFn = func(_ context.Context, method, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		if method != http.MethodGet {
			t.Fatalf("reader must not write: %s %s", method, endpoint)
		}
		b, _ := json.Marshal(resp)
		return 200, b, nil
	}
	return NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
}

func TestBUG500_SupabaseImplementsGateReaders(t *testing.T) {
	var store interface{} = &SupabaseWorkflowStore{}
	if _, ok := store.(ApprovalHistoryReader); !ok {
		t.Fatal("SupabaseWorkflowStore must implement ApprovalHistoryReader")
	}
	if _, ok := store.(QuestionHistoryReader); !ok {
		t.Fatal("SupabaseWorkflowStore must implement QuestionHistoryReader")
	}
	if _, ok := store.(ChatSessionReader); !ok {
		t.Fatal("SupabaseWorkflowStore must implement ChatSessionReader")
	}
}

func TestBUG500_ListApprovalsByRunDecodesRows(t *testing.T) {
	// Two row shapes: the real migration schema (request_payload_json +
	// selected_decision + requested_at) and a drifted flat-column variant.
	store := bug500Store(t, []map[string]any{
		{
			"id": "ap-1", "workflow_run_id": "run-9", "provider_key": "claude",
			"status": "pending", "selected_decision": "approve",
			"requested_at": "2026-01-01T00:00:00Z", "expires_at": "2026-01-02T00:00:00Z",
			"request_payload_json": map[string]any{
				"command": "rm -rf x", "cwd": "/w", "reason": "danger",
				"policy": "untrusted", "resolved_choices": []string{"approve"},
			},
		},
		{
			"id": "ap-2", "workflow_run_id": "run-9", "provider_key": "codex",
			"command": "ls", "status": "pending", "decision": "deny",
		},
	})
	out, err := store.ListApprovalsByRun(context.Background(), "run-9")
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 approvals, got %+v", out)
	}
	if out[0].ApprovalID != "ap-1" || out[0].Command != "rm -rf x" || out[0].Decision != "approve" ||
		out[0].Policy != "untrusted" || out[0].CreatedAt == "" || len(out[0].ResolvedChoices) != 1 {
		t.Fatalf("migration-shape approval decoded wrong: %+v", out[0])
	}
	if out[1].ApprovalID != "ap-2" || out[1].Command != "ls" || out[1].Decision != "deny" {
		t.Fatalf("flat-shape approval decoded wrong: %+v", out[1])
	}
}

func TestBUG500_ListQuestionsByRunDecodesRows(t *testing.T) {
	store := bug500Store(t, []map[string]any{
		{
			"id": "q-1", "workflow_run_id": "run-9", "prompt": "pick",
			"options_json": []map[string]any{{"id": "a", "label": "A"}},
			"multi_select": false, "status": "waiting",
			"selected_choice_json": []string{"a"}, "requested_at": "2026-01-01T00:00:00Z",
		},
		{
			"id": "q-2", "workflow_run_id": "run-9", "prompt": "pick2",
			"options": []map[string]any{{"id": "b", "label": "B"}},
			"status":  "pending", "choice": []string{"b"},
		},
	})
	out, err := store.ListQuestionsByRun(context.Background(), "run-9")
	if err != nil {
		t.Fatalf("list questions: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 questions, got %+v", out)
	}
	if out[0].QuestionID != "q-1" || len(out[0].Options) != 1 ||
		len(out[0].Choice) != 1 || out[0].Choice[0] != "a" || out[0].CreatedAt == "" {
		t.Fatalf("migration-shape question decoded wrong: %+v", out[0])
	}
	if out[1].QuestionID != "q-2" || len(out[1].Options) != 1 || out[1].Choice[0] != "b" {
		t.Fatalf("flat-shape question decoded wrong: %+v", out[1])
	}
}

func TestBUG500_ListApprovalsByRunErrorPropagates(t *testing.T) {
	original := httpRequestFn
	t.Cleanup(func() { httpRequestFn = original })
	httpRequestFn = func(context.Context, string, string, map[string]string, []byte) (int, []byte, error) {
		return 500, []byte(`{"message":"boom"}`), nil
	}
	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	if _, err := store.ListApprovalsByRun(context.Background(), "run-9"); err == nil {
		t.Fatal("non-2xx must surface an error, not an empty list")
	}
}

func TestBUG500_QueryShapeUsesRunIDFilter(t *testing.T) {
	original := httpRequestFn
	t.Cleanup(func() { httpRequestFn = original })
	var gotURL string
	httpRequestFn = func(_ context.Context, _, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		gotURL = endpoint
		return 200, []byte("[]"), nil
	}
	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co/"}, "k")
	if _, err := store.ListQuestionsByRun(context.Background(), "run-9"); err != nil {
		t.Fatalf("list questions: %v", err)
	}
	if !strings.Contains(gotURL, "workflow_provider_questions") || !strings.Contains(gotURL, "workflow_run_id=eq.run-9") {
		t.Fatalf("bad query shape: %s", gotURL)
	}
}
