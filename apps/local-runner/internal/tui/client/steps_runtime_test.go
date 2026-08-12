package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestGetWorkflowStepsRuntime_ParsesSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/workflow-runs/run-1/steps-runtime" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runId": "run-1",
			"steps": []map[string]any{
				{"stepId": "s1", "stepType": "flow-agent-delegate", "status": "RUNNING", "nodeId": "coder", "retryCount": 0},
			},
		})
	}))
	defer srv.Close()
	snap, err := client.New(srv.URL).GetWorkflowStepsRuntime(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Steps) != 1 || snap.Steps[0].NodeID != "coder" {
		t.Fatalf("snap=%+v", snap)
	}
}
