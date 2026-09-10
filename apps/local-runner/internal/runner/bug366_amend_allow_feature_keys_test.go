package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

func TestBUG366_AmendFlowAllowsFeatureKeysAndResumes(t *testing.T) {
	_, srv, dir, runID := amendFlowParkedFixture(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"change-audit/FEATURE-KEYS.md"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("Allow FEATURE-KEYS.md status=%d body=%s, want 200 (not 422 amend_failed)", status, body)
	}

	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	active, ok, err := reopened.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("GetFrozenForStep ok=%v err=%v", ok, err)
	}
	if active.Version != 2 {
		t.Fatalf("Version = %d, want 2", active.Version)
	}
	found := false
	for _, p := range active.AllowedExtraPaths {
		if p == "change-audit/FEATURE-KEYS.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("amended AllowedExtraPaths = %v, want FEATURE-KEYS.md", active.AllowedExtraPaths)
	}

	snap := srvClientSnapshot(t, srv, runID)
	if snap.LoopState.Status == "blocked" {
		t.Fatal("Allow FEATURE-KEYS.md must resume the parked flow")
	}
}

func TestBUG366_AmendFlowMixedCodeAndFeatureKeys(t *testing.T) {
	_, srv, dir, runID := amendFlowParkedFixture(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/user.go", "change-audit/FEATURE-KEYS.md"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("mixed Allow status=%d body=%s", status, body)
	}

	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	active, ok, err := reopened.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatal(err)
	}
	hasUser := false
	for _, p := range active.DeclaredPaths {
		if p == "src/user.go" {
			hasUser = true
		}
	}
	if !hasUser {
		t.Fatalf("DeclaredPaths missing src/user.go: %v", active.DeclaredPaths)
	}
	hasKeys := false
	for _, p := range active.AllowedExtraPaths {
		if p == "change-audit/FEATURE-KEYS.md" {
			hasKeys = true
		}
	}
	if !hasKeys {
		t.Fatalf("AllowedExtraPaths missing FEATURE-KEYS.md: %v", active.AllowedExtraPaths)
	}
}

func TestBUG366_FeatureKeysAllowDoesNotRepark(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rec := freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "# keys\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "change-audit/FEATURE-KEYS.md"},
	}, 0)
	if !blocked {
		t.Fatal("first write of FEATURE-KEYS.md must still park (BUG-278 / BUG-327)")
	}

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := changecontract.AmendFrozenContractForAllow(store, dir, rec, []string{"change-audit/FEATURE-KEYS.md"}, time.Now().UTC()); err != nil {
		t.Fatalf("ForAllow: %v", err)
	}

	blocked = svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-2", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "change-audit/FEATURE-KEYS.md"},
	}, 0)
	if blocked {
		snap := svc.agentGraphSnapshot(parentID)
		t.Fatalf("after Allow, FEATURE-KEYS.md must not re-park; gate=%q", snap.LoopState.GateReason)
	}
}

func TestBUG366_AmendFlowMakefileStill422(t *testing.T) {
	_, srv, _, runID := amendFlowParkedFixture(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"Makefile"}}, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("Makefile must still 422 (CA-427): status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "not a concrete code target") {
		t.Fatalf("body=%s, want concrete-code-target error", body)
	}
}

func TestBUG366_AmendFlowJSONRoundTripAllowedExtras(t *testing.T) {
	// Guard the persisted shape: AllowedExtraPaths must survive store reopen
	// (the TUI Allow path reads the next gate pass from disk).
	_, srv, dir, runID := amendFlowParkedFixture(t)
	status, _ := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"docs/readme.md"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	active, ok, err := reopened.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatal(err)
	}
	raw, err := json.Marshal(active)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "allowed_extra_paths") {
		t.Fatalf("persisted record missing allowed_extra_paths: %s", raw)
	}
}
