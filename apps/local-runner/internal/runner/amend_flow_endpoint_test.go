package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// amendFlowParkedFixture parks a freeze-fixture run with a frozen contract for
// the coder writer, matching the state a scope-drift block leaves behind
// (flow parked blocked, contract v1 declaring src/calc.go only).
func amendFlowParkedFixture(t *testing.T) (*InteractiveService, *httptest.Server, string, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	_, nodes := freezeChainFixture()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = nil
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"src/calc.go"})

	// Park the flow as a scope-drift block would.
	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "escalate",
		Summary: "flow gate block: flow scope drift: wrote outside the frozen contract's declared paths: src/user.go",
	}); err != nil {
		t.Fatalf("applyFlowControl escalate: %v", err)
	}
	return svc, srv, dir, parent.RunID
}

func TestAmendFlow_WidensContractAndResumesParkedFlow(t *testing.T) {
	_, srv, dir, runID := amendFlowParkedFixture(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/user.go"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("amend status=%d body=%s", status, body)
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
		t.Fatalf("Version = %d, want 2 (amended)", active.Version)
	}
	foundUser := false
	foundCalc := false
	for _, p := range active.DeclaredPaths {
		if p == "src/user.go" {
			foundUser = true
		}
		if p == "src/calc.go" {
			foundCalc = true
		}
	}
	if !foundUser {
		t.Fatalf("amended DeclaredPaths must include src/user.go: %v", active.DeclaredPaths)
	}
	if !foundCalc {
		t.Fatalf("amended contract must keep the original declared path src/calc.go: %v", active.DeclaredPaths)
	}

	// The park must be lifted so the retried writer can run.
	snap := srvClientSnapshot(t, srv, runID)
	if snap.LoopState.Status == "blocked" {
		t.Fatal("amend must resume the parked flow")
	}
}

func srvClientSnapshot(t *testing.T, srv *httptest.Server, runID string) AgentGraphSnapshot {
	t.Helper()
	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+runID+"/agent-graph", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", status, body)
	}
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	return snap
}

func TestAmendFlow_RejectsWhenFlowNotParked(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	_, nodes := freezeChainFixture()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = nodes
	prs.workspaceCwd = dir
	svc.mu.Unlock()
	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"src/calc.go"})

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/user.go"}}, nil)
	if status != http.StatusConflict {
		t.Fatalf("running flow must reject amend: status=%d body=%s", status, body)
	}
}

func TestAmendFlow_RejectsEmptyAndMissingPaths(t *testing.T) {
	_, srv, _, runID := amendFlowParkedFixture(t)
	for name, body := range map[string]any{
		"missing": map[string]any{},
		"empty":   map[string]any{"paths": []string{"  "}},
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend", body, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s, want 400", status, resp)
			}
		})
	}
}

func TestAmendFlow_RejectsNonConcretePath(t *testing.T) {
	_, srv, _, runID := amendFlowParkedFixture(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"Makefile"}}, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("non-concrete path must fail loudly: status=%d body=%s", status, body)
	}
}

func TestAmendFlow_NoWideningNeededReturns404(t *testing.T) {
	_, srv, _, runID := amendFlowParkedFixture(t)
	// The frozen contract already declares src/calc.go — amending with the same
	// path adds nothing, so no contract is minted and the endpoint says so.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/calc.go"}}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("already-covered amend must 404: status=%d body=%s", status, body)
	}
}

func TestAmendFlow_UnknownRunReturns404(t *testing.T) {
	_, srv, _, _ := amendFlowParkedFixture(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/run-missing/agent-loop/amend",
		map[string]any{"paths": []string{"src/user.go"}}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", status, body)
	}
}