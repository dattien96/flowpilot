package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListRunHistory_FetchesProjectRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/projects/proj-1/workflow-runs" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"runId": "run-1", "projectId": "proj-1", "status": "completed", "lastPrompt": "hi", "runKind": "chat"},
		})
	}))
	defer srv.Close()

	items, err := New(srv.URL).ListRunHistory(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].RunID != "run-1" || items[0].LastPrompt != "hi" {
		t.Fatalf("items=%+v", items)
	}
}
