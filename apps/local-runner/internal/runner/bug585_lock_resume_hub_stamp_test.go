package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-585 (live run-139670): confirming the CP Preview & Lock gate called
// resumeFlowWithFeedback, whose generic "un-park the escalated hub" stamp
// re-stamped the ALREADY-DONE cp_validator node RUNNING (esc field empty is
// read as "the hub itself parked", but a vibe_lock park parks cp_lock — a
// user.confirm node — and resumeVibeLock advances without any hub turn).
// The stamped RUNNING had no leg behind it and never settled, leaving
// cp_validator RUNNING forever in the timeline with no card to answer.
// The hub stamp must only fire when the hub step is actually parked
// WAITING_USER_APPROVAL (escalate/cap path), not for gates parked on other
// nodes.
func TestBUG585_VibeLockResumeDoesNotRestampDoneHubRunning(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	cwd := t.TempDir()
	cpDir := filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cpPath := "requirements/07-Coding-Plan/todo/CP-02-core.md"
	if err := os.WriteFile(filepath.Join(cwd, cpPath), []byte("---\nid: CP-02\nstatus: todo\n---\n\n- Status: `todo`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "cp_validator", Behavior: "hub.inline"},
		{ID: "cp_lock", Behavior: "user.confirm"},
		{ID: "task_plan_reader", Behavior: "agent.delegate"},
	}
	edges := []agentpack.FlowEdge{
		{From: "cp_validator", To: "cp_lock", When: "done", Kind: "forward"},
		{From: "cp_lock", To: "task_plan_reader", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeAwaitingLock = true
	rs.vibeLockNodeID = vibeCpLockNodeID
	rs.vibeLockPath = cpPath
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	// Live shape: cp_validator already DONE before the lock gate parked.
	svc.setFlowStepStatus(context.Background(), runID, "cp_validator", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "cp_lock", StepStatusWaitingUserApr)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeLockBlockReason
		st.GateReason = "CP Preview & Lock"
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	st, _ := stepByID(steps, "cp_validator")
	if st.Status != StepStatusDone {
		t.Fatalf("cp_validator re-stamped %s on a cp_lock gate resume — a DONE hub node must stay DONE; the lock park never parked the hub", st.Status)
	}
}

// Guard the contract the stamp exists for (BUG-231/BUG-233): an escalate that
// parked the hub itself leaves the hub step WAITING_USER_APPROVAL — resume
// must flip it back to RUNNING so the timeline does not render it still
// waiting while the reinvoked hub turn runs.
func TestBUG585_EscalateResumeStillUnparksWaitingHub(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "synthesis", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.activeHubNodeID = "synthesis"
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusWaitingUserApr)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = "needs a decision"
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	st, _ := stepByID(steps, "synthesis")
	if st.Status != StepStatusRunning {
		t.Fatalf("escalated hub stayed %s after resume — the WAITING hub must flip back to RUNNING", st.Status)
	}
}
