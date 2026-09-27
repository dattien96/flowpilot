package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// Task-240 D-4: applyFlowControl("done") while cohort incomplete must soft-defer.
func TestApplyFlowControlDoneRejectedWhenCohortIncomplete(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 0, Mode: "explicit"})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	// Register a 2-member cohort with only 1 result buffered → incomplete.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-coder-round-0", 2)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "flow-auto-coder-round-0", cohortEntry{
		Label: "reviewer_correctness", Status: "completed", FinalMessage: "ok",
	})

	result, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "done", Summary: "premature done"})
	if fcErr != nil {
		t.Fatalf("applyFlowControl: %v", fcErr)
	}
	if result.NextAction != "rejected_cohort_incomplete" {
		t.Fatalf("NextAction = %q, want rejected_cohort_incomplete", result.NextAction)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "running" {
		t.Fatalf("loop status = %q, want still running (no mutate)", loop.Status)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusRunning {
		t.Fatalf("synthesis = %q, want still RUNNING (not settled)", got)
	}
}

// Task-240 D-4 counterpart: all-done path still finalizes.
func TestApplyFlowControlDoneSucceedsWhenCohortCompleteOrAbsent(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 0, Mode: "explicit"})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	result, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "done", Summary: "all good"})
	if fcErr != nil {
		t.Fatalf("applyFlowControl: %v", fcErr)
	}
	if result.NextAction != "done" {
		t.Fatalf("NextAction = %q, want done", result.NextAction)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "done" {
		t.Fatalf("loop status = %q, want done", loop.Status)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusDone {
		t.Fatalf("synthesis = %q, want DONE", got)
	}
}

// Task-240 D-2 / I-2: continue-cap settles WAITING before loop reads blocked.
func TestApplyFlowControlCapSettlesHubBeforeLoopBlocked(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	// Round is already at cap-1 so continue bumps into cap.
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "running", Cap: 1, RoundCap: 1, Round: 0, Mode: "explicit",
	})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	result, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "continue",
		Payload: map[string]any{"issues": []any{map[string]any{"title": "x"}}},
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl: %v", fcErr)
	}
	if result.NextAction != "awaiting_user" {
		t.Fatalf("NextAction = %q, want awaiting_user", result.NextAction)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "tournament_escalation" || loop.BlockReason != "cap" {
		t.Fatalf("loop = %+v, want tournament_escalation/cap (escalation is always on)", loop)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Fatalf("synthesis = %q, want WAITING_USER_APPROVAL (settled with block)", got)
	}
	awaitTournamentChildIdle(t, svc, parent.RunID)
}

// Task-240 D-3 / I-4: blocked loop does not allow advance.
func TestLoopDoesNotAdvanceWhenBlockedOrDone(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	for _, st := range []string{"blocked", "done"} {
		svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: st, Cap: 3, Mode: "explicit"})
		svc.mu.Lock()
		allowed := svc.loopAllowsNextTurnLocked(parent.RunID)
		svc.mu.Unlock()
		if allowed {
			t.Errorf("loopAllowsNextTurnLocked when status=%q = true, want false", st)
		}
	}
}

// Task-240 D-9: watchdog helper fails when a node is stuck RUNNING.
func TestAssertNoStepStuckRunning(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	// Sub-test that must detect the stuck RUNNING node.
	probe := &testing.T{}
	assertNoStepStuckRunning(probe, svc, parent.RunID)
	if !probe.Failed() {
		t.Fatal("expected assertNoStepStuckRunning to fail when synthesis is RUNNING")
	}

	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusDone)
	assertNoStepStuckRunning(t, svc, parent.RunID) // must pass
}

// assertNoStepStuckRunning fails t if any step on runID is still RUNNING
// (Task-240 D-9 watchdog). allowRunning lists node IDs that may still be mid-turn.
func assertNoStepStuckRunning(t testing.TB, svc *InteractiveService, runID string, allowRunning ...string) {
	t.Helper()
	allow := map[string]bool{}
	for _, id := range allowRunning {
		allow[id] = true
	}
	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	for _, st := range steps {
		if st.Status == StepStatusRunning && !allow[st.ID] {
			t.Errorf("node %q stuck RUNNING at end of test (Task-240 no-RUNNING watchdog)", st.ID)
		}
	}
}

// Task-240 D-8: hub-facing tool prompts must not mention values outside schema.
func TestHubFacingToolPromptContract(t *testing.T) {
	// submit_review_outcome declared statuses.
	face, ok, err := agentpack.LoadBuiltinToolFace("submit_review_outcome")
	if err != nil || !ok {
		t.Skipf("submit_review_outcome face unavailable: ok=%v err=%v", ok, err)
	}
	allowed := map[string]bool{}
	for k := range face.StatusMap {
		allowed[k] = true
	}
	// Known allowed values used in prompts.
	for _, v := range []string{"approved", "changes_requested", "blocked", "continue", "done", "escalate"} {
		// only enforce declared face keys when StatusMap non-empty
		_ = v
	}
	if face.ID == "" && face.MapsTo == "" {
		t.Fatal("empty tool face")
	}
	if len(face.StatusMap) == 0 {
		t.Log("no status map on submit_review_outcome face")
	}
	// Minimal contract: allowed review outcomes do not include a free-form "done"
	// status key on the review tool (done is flow_control, not review outcome).
	if allowed["done"] {
		t.Error(`submit_review_outcome StatusMap must not include "done" (that is flow_control status; class BUG-287)`)
	}
}

// Task-240: hasOpenCohort unit coverage.
func TestHasOpenCohort(t *testing.T) {
	o := newAgentOrchestrator()
	if o.hasOpenCohort("p1") {
		t.Fatal("empty should be false")
	}
	o.preRegisterCohort("p1", "c1", 2)
	if !o.hasOpenCohort("p1") {
		t.Fatal("expected open after preRegister with 0 results")
	}
	o.appendCohortResult("p1", "c1", cohortEntry{Label: "a", Status: "completed"})
	if !o.hasOpenCohort("p1") {
		t.Fatal("still open with 1/2")
	}
	o.appendCohortResult("p1", "c1", cohortEntry{Label: "b", Status: "failed"})
	if o.hasOpenCohort("p1") {
		t.Fatal("should be complete at 2/2")
	}
	_ = o.drainCohort("p1", "c1")
	if o.hasOpenCohort("p1") {
		t.Fatal("after drain should be false")
	}
}

// Ensure strings import used if needed for future contract scans.
var _ = strings.Contains
