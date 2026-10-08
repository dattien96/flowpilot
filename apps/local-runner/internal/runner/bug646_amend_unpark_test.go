package runner

import (
	"net/http"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-646 (live run-523131 leg run-525391): POST agent-loop/amend minted the
// widened contract and resumed the run, but the parked leg's
// pendingFlowGateSettle stayed armed — the leg sat waiting_user_approval
// until a second, manual agent-loop/continue. One operator call must
// suffice: amend = the gate decision, so the owed settle must discharge.

func TestBug646_AmendUnparksGatedLeg(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	prs.workspaceCwd = dir
	prs.flowEngineDriven = true
	prs.autoOrchestrate = true
	svc.mu.Unlock()

	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"src/calc.go"})

	// The parked leg shape: gate armed pendingFlowGateSettle for the drifted
	// write, status waiting_user_approval.
	child := newP4ChildRun(svc, "leg-1", parent.RunID, dir, head)
	p4WriteFile(t, dir, "src/build.mk", "all: calc\n")
	svc.mu.Lock()
	child.stepID = "coder"
	child.status = RunStatusWaitingUserApr
	child.pendingFlowGateSettle = true
	child.pendingFlowGateTurnID = "turn-9"
	child.pendingFlowGateFinalMsg = "wrote build file"
	child.pendingGateChangedFiles = []string{"src/build.mk"}
	svc.agentOrchestrator.registerChild(parent.RunID, child.id)
	svc.mu.Unlock()

	// Park the parent loop — the scope-drift card.
	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "escalate",
		Summary: "flow scope drift: wrote outside the frozen contract's declared paths: src/build.mk",
	}); err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/amend",
		map[string]any{"paths": []string{"src/build.mk"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("amend must succeed: status=%d body=%s", status, body)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		settle := svc.runs["leg-1"].pendingFlowGateSettle
		svc.mu.Unlock()
		if !settle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("leg's pendingFlowGateSettle still armed after amend — second manual continue still required")
}
