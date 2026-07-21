package runner

import (
	"context"
	"strings"
	"testing"
)

// BUG-307: a cohort that finishes joining AFTER the parent loop already sealed
// (Stop, or the flow's own "done") left its "call the flow's control tool"
// synthesis note stranded in pendingAgentContext — no reinvoke will ever drain
// it (maybeAutoReinvokeHubWithNote's own status switch blocks the reinvoke but
// does not undo the append). composeAgentContextBlock (BUG-122) then silently
// prepends that stranded note to the NEXT turn on this run — including an
// ordinary chat follow-up typed after a restart — sending the model an
// instruction ("Synthesize... then call the flow's control tool") with no live
// flow left to satisfy it, which stalls the hub watchdog waiting for progress
// that can never come.
//
// Live repro: run-19500 (Review Loop, "fix bug 1+1 != 2") — Stop cancelled 2
// in-flight reviewers, both joined the cohort as "cancelled" as part of the
// Stop request itself (see TestStopAgentLoopCancelsCohortMemberAndJoins, whose
// exact setup this test mirrors), the loop was already "stopped" by then. The
// server was later restarted and a plain follow-up ("lan truoc fix gi vay")
// on the same run carried the stale join note and hung for 2m0s before the
// hub-stalled watchdog surfaced a "no progress" card.
func TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContext(t *testing.T) {
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
	c2, err2 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err1 != nil || err2 != nil {
		t.Fatalf("children: %v %v", err1, err2)
	}
	cohortID := "flow-auto-coder-round-0"
	svc.agentOrchestrator.registerChild(parent.RunID, c1.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, c2.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, cohortID, 2)
	svc.mu.Lock()
	svc.runs[c1.RunID].parentRunID = parent.RunID
	svc.runs[c1.RunID].flowCohortId = cohortID
	svc.runs[c1.RunID].label = "reviewer_correctness"
	svc.runs[c1.RunID].turnInFlight = true
	svc.runs[c1.RunID].turnCancel = func() {}
	svc.runs[c2.RunID].parentRunID = parent.RunID
	svc.runs[c2.RunID].flowCohortId = cohortID
	svc.runs[c2.RunID].label = "reviewer_security"
	svc.runs[c2.RunID].turnInFlight = true
	svc.runs[c2.RunID].turnCancel = func() {}
	svc.mu.Unlock()

	// One member completes first (like the coder-derived reviewer cohort in
	// run-19500), the other is still in-flight when Stop lands.
	svc.agentOrchestrator.appendCohortResult(parent.RunID, cohortID, cohortEntry{
		Label: "reviewer_correctness", Provider: "codex", Status: "completed", FinalMessage: "ok",
	})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusDone)

	// Stop cancels the remaining in-flight member -> cohort joins AS PART OF
	// this same Stop request, with the loop already "stopped".
	if _, apiErr := svc.stopAgentLoop(parent.RunID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("expected cohort joined after cancel of remaining member")
	}

	// BUG-233 fallback must still work: lastCohortNote is diagnostic-only
	// (never sent to a provider) and must still be retained regardless of loop
	// status.
	note := svc.lastCohortNoteFor(parent.RunID)
	if !strings.Contains(note, "cancelled") {
		t.Fatalf("lastCohortNote missing cancelled: %q", note)
	}

	// BUG-307: pendingAgentContext must NOT carry the synthesis note forward —
	// no reinvoke will ever drain it once the loop is sealed, and this is
	// exactly what composeAgentContextBlock would wrap onto the next turn.
	svc.mu.Lock()
	pending := append([]string(nil), svc.runs[parent.RunID].pendingAgentContext...)
	svc.mu.Unlock()
	for _, p := range pending {
		if strings.Contains(p, "call the flow's control tool") {
			t.Fatalf("pendingAgentContext still carries the stale synthesis note after Stop: %q", pending)
		}
	}
}

// Cross-provider parity variant of the above: the fix lives in
// loopSealedForReinvoke, called from the shared orchestration path
// (stopAgentLoop's cohort-cancel join) — the same code regardless of which
// provider drives the run or its children, so this exercises the identical
// scenario with Grok-flavored children to prove no adapter-specific branch was
// missed. newTestServer's default registry has no working Grok adapter (test
// harness limitation, "grok controlled runtime is not implemented yet"), so
// children are created via Codex and re-tagged Grok directly — the mechanism
// under test (loopSealedForReinvoke) only ever reads AgentLoopState.Status, it
// never branches on providerKey/cohortEntry.Provider.
func TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContextGrok(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].autoOrchestrate = true
	svc.runs[parent.RunID].providerKey = ProviderKeyGrok
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	c1, err1 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	c2, err2 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err1 != nil || err2 != nil {
		t.Fatalf("children: %v %v", err1, err2)
	}
	cohortID := "flow-auto-coder-round-0"
	svc.agentOrchestrator.registerChild(parent.RunID, c1.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, c2.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, cohortID, 2)
	svc.mu.Lock()
	svc.runs[c1.RunID].parentRunID = parent.RunID
	svc.runs[c1.RunID].flowCohortId = cohortID
	svc.runs[c1.RunID].label = "reviewer_correctness"
	svc.runs[c1.RunID].providerKey = ProviderKeyGrok
	svc.runs[c1.RunID].turnInFlight = true
	svc.runs[c1.RunID].turnCancel = func() {}
	svc.runs[c2.RunID].parentRunID = parent.RunID
	svc.runs[c2.RunID].flowCohortId = cohortID
	svc.runs[c2.RunID].label = "reviewer_security"
	svc.runs[c2.RunID].providerKey = ProviderKeyGrok
	svc.runs[c2.RunID].turnInFlight = true
	svc.runs[c2.RunID].turnCancel = func() {}
	svc.mu.Unlock()

	svc.agentOrchestrator.appendCohortResult(parent.RunID, cohortID, cohortEntry{
		Label: "reviewer_correctness", Provider: "grok", Status: "completed", FinalMessage: "ok",
	})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusDone)

	if _, apiErr := svc.stopAgentLoop(parent.RunID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("expected cohort joined after cancel of remaining member")
	}
	note := svc.lastCohortNoteFor(parent.RunID)
	if !strings.Contains(note, "cancelled") {
		t.Fatalf("lastCohortNote missing cancelled: %q", note)
	}
	svc.mu.Lock()
	pending := append([]string(nil), svc.runs[parent.RunID].pendingAgentContext...)
	svc.mu.Unlock()
	for _, p := range pending {
		if strings.Contains(p, "call the flow's control tool") {
			t.Fatalf("pendingAgentContext still carries the stale synthesis note after Stop (grok): %q", pending)
		}
	}
}

// Unit-pins loopSealedForReinvoke's exact status set: only "stopped"/"done" are
// sealed (no future reinvoke will ever drain a queued note). "blocked"/"paused"
// are temporary holds a later Continue/Resume legitimately drains and must NOT
// be treated as sealed, or a real in-progress operator-decision flow would
// silently lose its queued cohort note.
func TestLoopSealedForReinvoke(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := "run-loop-sealed"

	cases := []struct {
		status string
		sealed bool
	}{
		{"running", false},
		{"", false}, // never-set / post-restart zero-value must NOT be treated as sealed
		{"blocked", false},
		{"paused", false},
		{"stopped", true},
		{"done", true},
	}
	for _, tc := range cases {
		svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: tc.status})
		if got := svc.loopSealedForReinvoke(runID); got != tc.sealed {
			t.Errorf("loopSealedForReinvoke(status=%q) = %v, want %v", tc.status, got, tc.sealed)
		}
	}
}
