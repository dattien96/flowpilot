package runner

import (
	"net/http"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// BUG-637 (live run-306526): parked inside a mounted owner-debate sub-flow,
// agent-loop/amend iterated flowAgentCodeWriterNodes(activeFlowNodes) — the
// debate graph has no agent.code node, so zero contracts were found and the
// endpoint 404'd no_frozen_contract even though the sprint's tdd/coder
// contracts were active in the store. The RESCOPE verdict — "declare
// CMakeLists.txt in the parent sprint contract" — was unactionable through
// the only operator surface that can widen scope. The amend target set must
// be every active frozen contract OF THE RUN, not writer nodes of the
// currently-mounted flow.

func TestBug637_AmendReachesParentContractDuringNestedDebatePark(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	// The mounted flow is the owner-debate graph — zero agent.code nodes.
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "owner_1", Behavior: "hub.inline"},
		{ID: "owner_2", Behavior: "hub.inline"},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	// The sprint's contracts exist in the store — one scope level up.
	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"src/calc.go"})
	freezeP4Contract(t, dir, parent.RunID, "tdd", head, []string{"src/calc_test.go"})

	// Park the flow exactly like a RESCOPE debate verdict does.
	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "escalate",
		Summary: "resolving in favor of owner_1's RESCOPE — contract declared only src/calc.go but the task intent requires wiring it via src/build.mk",
	}); err != nil {
		t.Fatalf("applyFlowControl escalate: %v", err)
	}

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/build.mk"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("amend must reach the sprint contracts while parked in the nested debate: status=%d body=%s", status, body)
	}

	// Both active sprint contracts widen — union semantics preserved.
	reopened, serr := changecontract.NewFrozenStore(dir)
	if serr != nil {
		t.Fatal(serr)
	}
	for _, step := range []string{"coder", "tdd"} {
		rec, ok, serr := reopened.GetFrozenForStep(parent.RunID, step)
		if serr != nil || !ok {
			t.Fatalf("GetFrozenForStep(%s) ok=%v err=%v", step, ok, serr)
		}
		found := false
		for _, p := range rec.DeclaredPaths {
			if p == "src/build.mk" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s contract must declare src/build.mk: %v", step, rec.DeclaredPaths)
		}
	}

	snap := srvClientSnapshot(t, srv, parent.RunID)
	if snap.LoopState.Status == "blocked" {
		t.Fatal("amend must resume the parked flow")
	}
}

// A run with genuinely no contracts still 404s — the fix must not swallow
// the honest "nothing to widen" answer.
func TestBug637_AmendStill404WhenNoContractExists(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "owner_1", Behavior: "hub.inline"},
	}
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "escalate",
		Summary: "debate escalate with no contract anywhere",
	}); err != nil {
		t.Fatalf("applyFlowControl escalate: %v", err)
	}

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/build.mk"}}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("contractless run must still 404: status=%d body=%s", status, body)
	}
}
