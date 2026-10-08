package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// BUG-652 (live run-523131): an owner debate resolving a gate produced the
// SAME verdict signature for the same gated entity three times — each round
// reprompted the gated child, the child re-submitted, the gate re-fired, and
// a new debate cohort mounted. Nothing in the loop noticed the verdict was
// already adjudicated, so the run burned three full debate cohorts on one
// card.
//
// Fix: a durable per-entity verdict-signature ledger. When the debate's
// resolved verdicts for a gated entity are byte-identical to that entity's
// previous resolved verdicts, the restore path escalates to a human instead
// of arming another reprompt round. Distinct verdicts still reprompt.

func bug652Fixture(t *testing.T, priorSig string) (*InteractiveService, *interactiveRun, *interactiveRun) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	verdicts := []VerdictRow{
		{ACID: "ac-1", Verdict: "fail", Note: "missing null-guard"},
		{ACID: "ac-2", Verdict: "pass"},
	}
	parent := &interactiveRun{
		id:                "run-652-parent",
		projectID:         "proj",
		status:            RunStatusRunning,
		agentStatus:       string(RunStatusRunning),
		workingMode:       workingmode.Vibe,
		chatFlowRef:       workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:   true,
		flowEngineDriven:  true,
		workspaceCwd:      "/tmp/bug652",
		activeFlowNodes:   bug567DebateGraph(),
		vibeParkedNodes:   bug567SprintGraph(),
		vibeParkedFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
		vibeParkedGatedRunIDs: []string{
			"run-652-child",
		},
		lastFlowVerdicts:      verdicts,
		vibeDebateVerdictSigs: map[string]string{},
	}
	if priorSig != "" {
		parent.vibeDebateVerdictSigs["node:reviewer"] = priorSig
	}
	child := &interactiveRun{
		id:             "run-652-child",
		parentRunID:    parent.id,
		label:          "reviewer",
		status:         RunStatusCompleted,
		lastTurnStepID: "reviewer",
		workspaceCwd:   "/tmp/bug652",
	}
	svc.mu.Lock()
	svc.runs[parent.id] = parent
	svc.runs[child.id] = child
	svc.mu.Unlock()
	return svc, parent, child
}

func TestBug652_IdenticalVerdictTwiceEscalates(t *testing.T) {
	verdicts := []VerdictRow{
		{ACID: "ac-1", Verdict: "fail"},
		{ACID: "ac-2", Verdict: "pass"},
	}
	svc, parent, child := bug652Fixture(t, vibeDebateVerdictSignature(verdicts))

	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("restoreVibeFlowAfterDebate must handle the parked overlay")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	svc.mu.Unlock()
	if reprompt != "" {
		t.Fatal("identical verdict must not arm another reprompt on the gated child")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.id)
	if st.Status != "blocked" {
		t.Fatalf("same-verdict repeat must block the loop for human decision, got status=%q", st.Status)
	}
	if !strings.Contains(st.GateReason, "same-verdict") && !strings.Contains(st.GateReason, "identical verdict") {
		t.Fatalf("gate reason must name the same-verdict breaker, got %q", st.GateReason)
	}
}

func TestBug652_DistinctVerdictsAllowed(t *testing.T) {
	// Prior round resolved with a different signature — a fresh verdict on
	// the same entity is progress, not a loop.
	svc, parent, child := bug652Fixture(t, "ac-1=pass|ac-2=pass")

	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("restoreVibeFlowAfterDebate must handle the parked overlay")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	svc.mu.Unlock()
	if reprompt == "" {
		t.Fatal("distinct verdict must arm the normal gate reprompt on the gated child")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.id)
	if st.Status == "blocked" {
		t.Fatalf("distinct verdict must not trip the breaker, got blocked: %q", st.GateReason)
	}
}

func TestBug652_VerdictLedgerSurvivesRestart(t *testing.T) {
	// Round 1: distinct verdict records the signature into the ledger. A
	// restart replay (same parked state, same verdicts) must then trip the
	// breaker — the ledger is the only memory across the restart.
	svc, parent, child := bug652Fixture(t, "")
	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("first restore must proceed normally")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	sig := parent.vibeDebateVerdictSigs["node:reviewer"]
	svc.mu.Unlock()
	if reprompt == "" {
		t.Fatal("first restore must arm the reprompt")
	}
	if sig == "" {
		t.Fatal("first restore must record the verdict signature per entity")
	}

	// Simulate restart: the debate mounted again for the same entity and
	// resolved with the identical verdict rows.
	svc2, parent2, child2 := bug652Fixture(t, sig)
	if !svc2.restoreVibeFlowAfterDebate(parent2.id) {
		t.Fatal("second restore must handle the parked overlay")
	}
	svc2.mu.Lock()
	reprompt2 := child2.pendingGateRepromptPrompt
	svc2.mu.Unlock()
	if reprompt2 != "" {
		t.Fatal("identical post-restart verdict must not arm a third reprompt")
	}
	st := svc2.agentOrchestrator.loopStateFor(parent2.id)
	if st.Status != "blocked" {
		t.Fatalf("post-restart identical verdict must escalate, got status=%q", st.Status)
	}
}

