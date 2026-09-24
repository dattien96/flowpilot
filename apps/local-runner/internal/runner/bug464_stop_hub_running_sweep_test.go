package runner

import (
	"context"
	"testing"
)

// BUG-464: Stop terminalizes cohort-member nodes (m.label → CANCELED) but never
// sweeps non-member flow steps — the inline hub node keeps whatever status the
// last cohort-join stamped. Live repro: run-47170 (vibe-ingest) — SIGKILL mid-
// ss_converter child turn, resume re-drove the converter (run-47655), the
// converter-completion join stamped hub `ss_validator` RUNNING (loopIsAdvancing
// was still true at that instant), then Stop landed: run → cancelled, member
// rows normalized, but `ss_validator` stayed RUNNING in both the in-memory
// step-runtime and the durable step-transition log forever — the desktop
// sidebar shows a permanently spinning node on a dead run.
//
// Fix contract: on Stop, every non-terminal step row of a flow-engine-driven
// run normalizes — RUNNING/WAITING_* → CANCELED (in-flight work dies with the
// run), while PENDING stays PENDING (it never ran, same convention as the
// restart evidence-walk) and terminal rows are untouched.
func TestStopAgentLoopSweepsNonMemberRunningSteps(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].autoOrchestrate = true
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	c1, err1 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err1 != nil {
		t.Fatalf("child: %v", err1)
	}
	cohortID := "flow-auto-coder-round-0"
	svc.agentOrchestrator.registerChild(parent.RunID, c1.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, cohortID, 1)
	svc.mu.Lock()
	svc.runs[c1.RunID].parentRunID = parent.RunID
	svc.runs[c1.RunID].flowCohortId = cohortID
	svc.runs[c1.RunID].label = "reviewer_correctness"
	svc.runs[c1.RunID].turnInFlight = true
	svc.runs[c1.RunID].turnCancel = func() {}
	svc.mu.Unlock()

	// Live shape at Stop time: one member DONE, one member in-flight, hub
	// stamped RUNNING by the earlier join (loopIsAdvancing), last node PENDING.
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusRunning)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	if _, apiErr := svc.stopAgentLoop(parent.RunID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}

	steps, loadErr := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	if loadErr != nil {
		t.Fatalf("LoadRunSteps: %v", loadErr)
	}
	if st, ok := stepByID(steps, "synthesis"); !ok || st.Status != StepStatusCanceled {
		t.Fatalf("hub node after Stop = %+v (ok=%v), want CANCELED — ghost RUNNING on a cancelled run", st, ok)
	}
	if st, _ := stepByID(steps, "reviewer_security"); st.Status != StepStatusPending {
		t.Fatalf("never-started node after Stop = %q, want PENDING (untouched)", st.Status)
	}
	if st, _ := stepByID(steps, "coder"); st.Status != StepStatusDone {
		t.Fatalf("terminal node after Stop = %q, want DONE (untouched)", st.Status)
	}
}
