package client

// Task-437: ResolveWorktree endpoint/body + APIError.Details must carry the
// 409 evidence body (requiresConfirm/conflictPaths/patchArtifactRef) so the
// TUI can render the confirm hint and the fail-closed conflict guidance.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ResolveWorktreePostsModeAndConfirm(t *testing.T) {
	var gotPath, gotMode string
	var gotConfirm bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			Mode    string `json:"mode"`
			Confirm bool   `json:"confirm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotMode, gotConfirm = body.Mode, body.Confirm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"worktreeState": "kept_branch", "branch": "fp/run-1",
		})
	}))
	defer srv.Close()

	res, err := New(srv.URL).ResolveWorktree(context.Background(), "run-1", "keep_branch", true)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	if gotPath != "/client/workflow-runs/run-1/worktree/resolve" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotMode != "keep_branch" || !gotConfirm {
		t.Fatalf("body mode=%q confirm=%v — confirm flag must reach the wire", gotMode, gotConfirm)
	}
	if res.WorktreeState != "kept_branch" || res.Branch != "fp/run-1" {
		t.Fatalf("result=%+v", res)
	}
}

func TestClient_ResolveWorktree409EvidenceBodySurvives(t *testing.T) {
	// The runner writes the evidence map bare (no error{} wrapper) for
	// conflict/confirm 409s — Details must preserve it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requiresConfirm": true, "uncommitted": []string{"a.txt", "b.txt"},
			"worktreeState": "active", "mode": "keep_branch",
		})
	}))
	defer srv.Close()

	_, err := New(srv.URL).ResolveWorktree(context.Background(), "run-1", "keep_branch", false)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 409 {
		t.Fatalf("err=%v — want *APIError 409", err)
	}
	if apiErr.Details["requiresConfirm"] != true {
		t.Fatalf("Details missing requiresConfirm: %v", apiErr.Details)
	}
	files, _ := apiErr.Details["uncommitted"].([]any)
	if len(files) != 2 {
		t.Fatalf("uncommitted=%v", apiErr.Details["uncommitted"])
	}
}

func TestClient_ResolveWorktree409ConflictDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conflict": true, "conflictPaths": []string{"CONFLICT.txt"},
			"patchArtifactRef": "C:/tmp/x.patch", "reason": "patch does not apply",
			"worktreeState": "merge_pending",
		})
	}))
	defer srv.Close()

	_, err := New(srv.URL).ResolveWorktree(context.Background(), "run-1", "apply_patch", false)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 409 {
		t.Fatalf("err=%v", err)
	}
	// Wire-truth: no code field on evidence bodies — detection keys off
	// conflict:true in Details.
	if apiErr.Details["conflict"] != true {
		t.Fatalf("Details missing conflict flag: %v", apiErr.Details)
	}
	if apiErr.Details["patchArtifactRef"] != "C:/tmp/x.patch" {
		t.Fatalf("patchArtifactRef=%v", apiErr.Details["patchArtifactRef"])
	}
}
