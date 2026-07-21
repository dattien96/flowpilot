package runner

// BUG-308 residual UI: after Stop, loop stays "stopped" forever, but a plain-chat
// follow-up sets rs.status=completed. historyStatusForLiveRun /
// normalizeResumedFlowStatus must NOT force Cancelled over that completed status
// (header + history list stuck on Cancelled while the answer is already there).
//
// additive-tests-only: new file only.
// cross-provider-parity Case 1: status mapping is provider-agnostic.

import (
	"testing"
)

func TestHistoryStatusAfterStopFollowUpCompleted(t *testing.T) {
	svc := newInteractiveService(bug308CompletingCodexRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Cap: 3, Mode: "explicit", Round: 0})

	// Immediately after Stop (no follow-up yet).
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.agentStatus = string(RunStatusCancelled)
	got := svc.historyStatusForLiveRun(rs)
	svc.mu.Unlock()
	if got != RunStatusCancelled {
		t.Fatalf("after stop only: history=%q want cancelled", got)
	}

	// After plain-chat follow-up completed (emitLocked path).
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.status = RunStatusCompleted
	rs.agentStatus = string(RunStatusCompleted)
	got = svc.historyStatusForLiveRun(rs)
	svc.mu.Unlock()
	if got != RunStatusCompleted {
		t.Fatalf("after post-stop follow-up complete: history=%q want completed (loop still stopped)", got)
	}

	// Mid follow-up.
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.status = RunStatusRunning
	got = svc.historyStatusForLiveRun(rs)
	svc.mu.Unlock()
	if got != RunStatusRunning {
		t.Fatalf("mid post-stop follow-up: history=%q want running", got)
	}
}

func TestNormalizeResumedFlowStatus_StoppedLoopPrefersCompletedSession(t *testing.T) {
	st := ProviderSessionState{
		Status:    RunStatusCompleted,
		LoopState: AgentLoopState{Status: "stopped", Cap: 3, Mode: "explicit"},
	}
	if got := normalizeResumedFlowStatus(st); got != RunStatusCompleted {
		t.Fatalf("normalizeResumedFlowStatus = %q, want completed when session completed after stop", got)
	}
	st.Status = RunStatusCancelled
	if got := normalizeResumedFlowStatus(st); got != RunStatusCancelled {
		t.Fatalf("normalizeResumedFlowStatus = %q, want cancelled when still cancelled", got)
	}
}
