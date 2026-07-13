package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

func TestHandleGetCanonicalHeadReturnsFoundHead(t *testing.T) {
	workspace := t.TempDir()
	h := changecontract.CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "Divide returns an error on zero divisor",
		Status:            changecontract.HeadStatusCurrent,
		Decisions:         []changecontract.Decision{{Tried: "lookup table", Outcome: changecontract.DecisionRejected, Reason: "no negative divisor support"}},
	}
	h.IntentSignature = changecontract.ComputeSignature(h)
	if err := changecontract.SaveHead(workspace, h); err != nil {
		t.Fatalf("SaveHead: %v", err)
	}

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqURL := server.URL + "/client/projects/project-1/features/calc-core/canonical-head?workingDirectory=" + url.QueryEscape(workspace)
	status, body := doJSON(t, http.MethodGet, reqURL, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}

	var resp canonicalHeadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Found {
		t.Fatal("expected found=true for an existing Head")
	}
	if resp.BehaviorStatement != h.BehaviorStatement {
		t.Errorf("behavior_statement = %q, want %q", resp.BehaviorStatement, h.BehaviorStatement)
	}
	if len(resp.Decisions) != 1 || resp.Decisions[0].Tried != "lookup table" {
		t.Errorf("decisions = %+v, want the rejected lookup-table decision", resp.Decisions)
	}
}

func TestHandleGetCanonicalHeadNotFoundReturnsFoundFalse(t *testing.T) {
	workspace := t.TempDir()
	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqURL := server.URL + "/client/projects/project-1/features/never-seen/canonical-head?workingDirectory=" + url.QueryEscape(workspace)
	status, body := doJSON(t, http.MethodGet, reqURL, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp canonicalHeadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Found {
		t.Fatal("expected found=false when no Head exists on disk")
	}
}

func TestHandleGetStepContractReturnsFoundContract(t *testing.T) {
	workspace := t.TempDir()
	store, err := changecontract.NewStore(workspace)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(changecontract.Contract{
		RunID:         "run-1",
		StepID:        "step-1",
		FeatureKey:    "calc-core",
		Intent:        "add zero-divisor guard",
		DeclaredPaths: []string{"calc.go"},
		Confidence:    changecontract.ConfidenceDeclared,
	}); err != nil {
		t.Fatalf("Save: %v", err)
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
		t.Fatalf("decode: %v", err)
	}
	if !resp.Found {
		t.Fatal("expected found=true for an existing Contract")
	}
	if resp.Intent != "add zero-divisor guard" {
		t.Errorf("intent = %q", resp.Intent)
	}
}

func TestHandleGetStepContractUnknownRunReturns404(t *testing.T) {
	svc := NewInteractiveService()
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status, _ := doJSON(t, http.MethodGet, server.URL+"/client/workflow-runs/never-seen/steps/step-1/contract", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", status)
	}
}

// TestHandleRebaselineCanonicalHead verifies Task-186's r-attach-spec
// confirmation caller: a spec_less Head flips to current when the human
// confirms via the rebaseline endpoint, using the feature's catalog DocRefs.
func TestHandleRebaselineCanonicalHead(t *testing.T) {
	workspace := t.TempDir()

	// Governing doc on disk (resolveDocRefPath looks under requirements/05-System-Specs).
	specDir := filepath.Join(workspace, "requirements", "05-System-Specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "SS-14-Code-Context-And-Regression-Safety.md"), []byte("spec"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Catalog with the feature's DocRefs.
	catalogDir := filepath.Join(workspace, ".flowpilot", "catalog")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "features.ndjson"),
		[]byte(`{"feature_key":"calc-core","doc_refs":["SS-14-Code-Context-And-Regression-Safety"]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A spec_less Head to be rebaselined.
	head := changecontract.CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "guard zero divisor", Status: changecontract.HeadStatusSpecLess, SpecConfidence: changecontract.SpecConfidenceSpecLess}
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

	status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/p1/features/calc-core/canonical-head/rebaseline",
		map[string]any{"workingDirectory": workspace}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp canonicalHeadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != changecontract.HeadStatusCurrent || resp.SpecConfidence != changecontract.SpecConfidenceSpecBacked {
		t.Fatalf("expected current/spec_backed after rebaseline, got status=%q conf=%q", resp.Status, resp.SpecConfidence)
	}
}

func TestHandleRebaselineCanonicalHeadNoGoverningDocsReturns400(t *testing.T) {
	workspace := t.TempDir()
	head := changecontract.CanonicalHead{FeatureKey: "calc-core", Status: changecontract.HeadStatusSpecLess, SpecConfidence: changecontract.SpecConfidenceSpecLess}
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

	status, _ := doJSON(t, http.MethodPost, server.URL+"/client/projects/p1/features/calc-core/canonical-head/rebaseline",
		map[string]any{"workingDirectory": workspace}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 when no governing docs registered, got %d", status)
	}
}

// TestHandleRetireCanonicalHeadMerged verifies Task-187's retire: an explicit
// human-initiated merge copies the retiring Head's decisions into the target
// and marks the source retired.
func TestHandleRetireCanonicalHeadMerged(t *testing.T) {
	workspace := t.TempDir()
	head := changecontract.CanonicalHead{
		FeatureKey: "calc-core-old",
		Decisions:  []changecontract.Decision{{Tried: "lookup table", Outcome: changecontract.DecisionRejected, Reason: "no negatives"}},
	}
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

	status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/p1/features/calc-core-old/canonical-head/retire",
		map[string]any{"workingDirectory": workspace, "action": "merged", "targets": []string{"calc-core"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp canonicalHeadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != changecontract.RetireActionMerged {
		t.Fatalf("expected status merged, got %q", resp.Status)
	}
	// Target Head should exist and carry the folded-in decision.
	target, found, err := changecontract.LoadHead(workspace, "calc-core")
	if err != nil || !found {
		t.Fatalf("expected target Head to be created, found=%v err=%v", found, err)
	}
	if len(target.Decisions) == 0 || target.Decisions[len(target.Decisions)-1].Tried != "lookup table" {
		t.Fatalf("expected target to receive the retiring Head's decision, got %+v", target.Decisions)
	}
}

func TestHandleRetireCanonicalHeadRenameRequiresTargets(t *testing.T) {
	workspace := t.TempDir()
	head := changecontract.CanonicalHead{FeatureKey: "calc-core"}
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

	status, _ := doJSON(t, http.MethodPost, server.URL+"/client/projects/p1/features/calc-core/canonical-head/retire",
		map[string]any{"workingDirectory": workspace, "action": "renamed", "targets": []string{}}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 when rename has no targets, got %d", status)
	}
}

// TestDetectRetirePending verifies the conservative retire nudge: a spec-backed
// Head whose key is absent from FEATURE-KEYS.md flags pending; spec_less and
// still-known keys do not.
func TestDetectRetirePending(t *testing.T) {
	workspace := t.TempDir()

	specBackedGone := changecontract.CanonicalHead{FeatureKey: "gone-feature", Status: changecontract.HeadStatusCurrent, SpecConfidence: changecontract.SpecConfidenceSpecBacked}
	specBackedGone.IntentSignature = changecontract.ComputeSignature(specBackedGone)
	if err := changecontract.SaveHead(workspace, specBackedGone); err != nil {
		t.Fatal(err)
	}
	if !detectRetirePending(workspace, []string{"still-here"}) {
		t.Fatal("expected pending=true for a spec-backed Head whose key vanished from known keys")
	}
	if detectRetirePending(workspace, []string{"gone-feature"}) {
		t.Fatal("expected pending=false when the key is still known")
	}

	// A spec_less Head with an unknown key must NOT flag (may be an inferred key).
	workspace2 := t.TempDir()
	specLess := changecontract.CanonicalHead{FeatureKey: "inferred-key", Status: changecontract.HeadStatusSpecLess, SpecConfidence: changecontract.SpecConfidenceSpecLess}
	specLess.IntentSignature = changecontract.ComputeSignature(specLess)
	if err := changecontract.SaveHead(workspace2, specLess); err != nil {
		t.Fatal(err)
	}
	if detectRetirePending(workspace2, []string{"something-else"}) {
		t.Fatal("expected pending=false for a spec_less Head (exempt from retire detection)")
	}
}
