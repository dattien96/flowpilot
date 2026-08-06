package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

func TestHandleListProjectFeaturesMergesCatalogLedgerAndCanonical(t *testing.T) {
	workspace := t.TempDir()
	dotFP := filepath.Join(workspace, ".flowpilot")
	catalogDir := filepath.Join(dotFP, "catalog")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "features.ndjson"),
		[]byte(`{"feature_key":"from-catalog"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalDir := filepath.Join(dotFP, "canonical")
	if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	head := changecontract.CanonicalHead{FeatureKey: "from-head", BehaviorStatement: "x", Status: changecontract.HeadStatusCurrent}
	head.IntentSignature = changecontract.ComputeSignature(head)
	if err := changecontract.SaveHead(workspace, head); err != nil {
		t.Fatal(err)
	}

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqURL := server.URL + "/client/projects/p1/features?workingDirectory=" + url.QueryEscape(workspace)
	status, body := doJSON(t, http.MethodGet, reqURL, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp featureListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.FeatureKeys) < 2 {
		t.Fatalf("feature_keys = %v, want at least catalog + head", resp.FeatureKeys)
	}
}

func TestHandleGetStepContractIncludesScopeDiff(t *testing.T) {
	workspace := t.TempDir()
	runGitInDir(t, workspace, "init")
	runGitInDir(t, workspace, "config", "user.email", "t@example.com")
	runGitInDir(t, workspace, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(workspace, "calc.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInDir(t, workspace, "add", "calc.go")
	runGitInDir(t, workspace, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(workspace, "user.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := changecontract.NewStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{
		RunID: "run-1", StepID: "step-1", FeatureKey: "calc-core",
		DeclaredPaths: []string{"calc.go"}, Confidence: changecontract.ConfidenceDeclared,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewInteractiveService()
	svc.mu.Lock()
	svc.runs["run-1"] = &interactiveRun{id: "run-1", workspaceCwd: workspace}
	svc.mu.Unlock()
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status, body := doJSON(t, http.MethodGet, server.URL+"/client/workflow-runs/run-1/steps/step-1/contract", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp contractResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.OutOfScopePaths) == 0 || resp.OutOfScopePaths[0] != "user.go" {
		t.Fatalf("out_of_scope_paths = %v, want [user.go]", resp.OutOfScopePaths)
	}
	if len(resp.InScopePaths) != 0 {
		t.Fatalf("in_scope_paths = %v, want empty (only user.go touched)", resp.InScopePaths)
	}
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
