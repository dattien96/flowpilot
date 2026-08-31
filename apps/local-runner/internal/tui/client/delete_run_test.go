package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestDeleteRun_SendsDeleteToWorkflowRuns(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"deleted"}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	if err := c.DeleteRun(context.Background(), "run-abc 123"); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method=%q want DELETE", gotMethod)
	}
	// Path must be escaped.
	if gotPath != "/client/workflow-runs/run-abc%20123" {
		t.Fatalf("path=%q want escaped", gotPath)
	}
}

func TestDeleteRun_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "run_not_found",
			"message": "workflow run not found",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	err := c.DeleteRun(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	api, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("err type %T want *APIError", err)
	}
	if api.Code != "run_not_found" {
		t.Fatalf("code=%q", api.Code)
	}
}
