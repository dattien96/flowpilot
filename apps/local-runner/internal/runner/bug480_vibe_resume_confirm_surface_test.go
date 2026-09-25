package runner

import (
	"errors"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-480 (live run-37268): a mounted vibeResumeConfirm gate owns the run's
// decision surface, but the generic /agent-loop/continue path
// (resumeFlowWithFeedback) never checked it — it unblocked the loop and
// re-invoked the hub instead of consuming the gate. The fix routes an
// unambiguous Continue through the SAME gate consumer as gate-decision;
// ambiguous prose gets a typed pending_gate_decision conflict and nothing
// is unblocked.

func bug480MountedResumeConfirm(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = "tdd"
	rs.vibeCheckpointNode = "tdd"
	rs.workspaceCwd = cwd
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3})
	return parent.RunID
}

func bug480ConfirmMounted(svc *InteractiveService, runID string) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	return rs != nil && rs.vibeResumeConfirm
}

// An unambiguous generic Continue (empty feedback — what the client sends for
// the single "Continue" button) must consume the mounted resume-confirm gate
// through its canonical consumer, not unblock the loop around it.
func TestBUG480_GenericContinueConsumesMountedResumeConfirm(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := bug480MountedResumeConfirm(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	if bug480ConfirmMounted(svc, runID) {
		t.Fatal("generic Continue left vibeResumeConfirm mounted — the gate was bypassed, not consumed (BUG-480)")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status == "blocked" {
		t.Fatalf("consumed gate must resume the loop, still blocked (reason=%q)", st.BlockReason)
	}
}

// "cancel" feedback maps onto the gate's own cancel option: the gate stays
// mounted and the loop stays parked — an honest decline, never a resume.
func TestBUG480_GenericContinueCancelFeedbackStaysParked(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := bug480MountedResumeConfirm(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, "cancel"); err != nil {
		t.Fatalf("resumeFlowWithFeedback(cancel): %v", err)
	}
	if !bug480ConfirmMounted(svc, runID) {
		t.Fatal("cancel must keep the resume-confirm gate mounted (stay parked)")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" {
		t.Fatalf("cancel must not unblock the loop, got status=%q", st.Status)
	}
}

// Prose feedback is ambiguous: it is neither an option id nor a label on the
// resume card. The endpoint must NOT guess — return a typed
// pending_gate_decision conflict and leave the gate + block untouched.
func TestBUG480_ProseContinueReturnsPendingGateDecision(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := bug480MountedResumeConfirm(t, svc)

	_, err := svc.resumeFlowWithFeedback(runID, "rewrite the plan first")
	if err == nil {
		t.Fatal("ambiguous prose on a mounted resume gate must not silently resume")
	}
	var ae *apiErr
	if !errors.As(err, &ae) || ae.status != 409 || ae.code != "pending_gate_decision" {
		t.Fatalf("want typed 409 pending_gate_decision, got err=%v", err)
	}
	if !bug480ConfirmMounted(svc, runID) {
		t.Fatal("rejected prose must leave the gate mounted")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" {
		t.Fatalf("rejected prose must not unblock the loop, got status=%q", st.Status)
	}
}

// Once the gate is consumed, a repeat generic Continue hits the normal
// resume path (no gate mounted) — the consumed decision is not replayed.
func TestBUG480_DuplicateContinueAfterConsumeDoesNotReplay(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := bug480MountedResumeConfirm(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, "ok"); err != nil {
		t.Fatalf("first continue: %v", err)
	}
	if bug480ConfirmMounted(svc, runID) {
		t.Fatal("gate should be consumed")
	}
	// Second continue: gate gone — must not error or re-drive a resume.
	if _, err := svc.resumeFlowWithFeedback(runID, "ok"); err != nil {
		t.Fatalf("duplicate continue after consume: %v", err)
	}
}
