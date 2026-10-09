package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// BUG-663 (live PrivateVault run-641691): owner legs aborted by tool
// permission denials resolve NO verdict rows, so the BUG-652 breaker (which
// only compares non-empty signatures) stays blind and every remount
// re-debates vacuously — 7 owner legs on one card. Consecutive
// empty-verdict debates for the same entity must escalate like an
// identical repeat; a single empty resolution still proceeds (preserves
// legitimate verdict-less restores).

func bug663Fixture(t *testing.T) (*InteractiveService, *interactiveRun, *interactiveRun) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parent := &interactiveRun{
		id:                "run-663-parent",
		projectID:         "proj",
		status:            RunStatusRunning,
		agentStatus:       string(RunStatusRunning),
		workingMode:       workingmode.Vibe,
		chatFlowRef:       workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:   true,
		flowEngineDriven:  true,
		workspaceCwd:      "/tmp/bug663",
		activeFlowNodes:   bug567DebateGraph(),
		vibeParkedNodes:   bug567SprintGraph(),
		vibeParkedFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
		vibeParkedGatedRunIDs: []string{
			"run-663-child",
		},
		lastFlowVerdicts:      nil, // aborted legs: no verdict rows at all
		vibeDebateVerdictSigs: map[string]string{},
	}
	child := &interactiveRun{
		id:             "run-663-child",
		parentRunID:    parent.id,
		label:          "reviewer",
		status:         RunStatusCompleted,
		lastTurnStepID: "reviewer",
		workspaceCwd:   "/tmp/bug663",
	}
	svc.mu.Lock()
	svc.runs[parent.id] = parent
	svc.runs[child.id] = child
	svc.mu.Unlock()
	return svc, parent, child
}

func TestBug663_FirstEmptyVerdictProceeds(t *testing.T) {
	svc, parent, child := bug663Fixture(t)

	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("first empty restore must handle the parked overlay")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	svc.mu.Unlock()
	if reprompt == "" {
		t.Fatal("first empty-verdict restore must arm the reprompt (preserves verdict-less restore behavior)")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.id)
	if st.Status == "blocked" {
		t.Fatalf("first empty verdict must not trip the breaker, got blocked: %q", st.GateReason)
	}
}

func TestBug663_SecondConsecutiveEmptyVerdictEscalates(t *testing.T) {
	svc, parent, child := bug663Fixture(t)

	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("first empty restore must handle the parked overlay")
	}
	// Re-seed the parked claim as a remount would: the debate parked again
	// on the same card with still no verdicts.
	svc.mu.Lock()
	parent.vibeParkedNodes = bug567SprintGraph()
	parent.vibeParkedGatedRunIDs = []string{"run-663-child"}
	parent.vibeParkedFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	// Simulate the remount arming a fresh reprompt first (cleared below).
	child.pendingGateRepromptPrompt = ""
	svc.mu.Unlock()
	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("second empty restore must handle the parked overlay")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	svc.mu.Unlock()
	if reprompt != "" {
		t.Fatal("second consecutive empty verdict must not arm another reprompt")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.id)
	if st.Status != "blocked" {
		t.Fatalf("second consecutive empty verdict must block for human decision, got status=%q", st.Status)
	}
	if !strings.Contains(st.GateReason, "circuit breaker") {
		t.Fatalf("gate reason must name the circuit breaker, got %q", st.GateReason)
	}
}

func TestBug663_EmptyThenRealVerdictProceeds(t *testing.T) {
	svc, parent, child := bug663Fixture(t)

	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("first empty restore must handle the parked overlay")
	}
	// A real verdict after an empty round is progress, not a repeat.
	parent.lastFlowVerdicts = []VerdictRow{{ACID: "ac-1", Verdict: "fail"}}
	// Re-seed the parked claim as a remount would.
	parent.vibeParkedNodes = bug567SprintGraph()
	parent.vibeParkedGatedRunIDs = []string{"run-663-child"}
	if !svc.restoreVibeFlowAfterDebate(parent.id) {
		t.Fatal("real-verdict restore must handle the parked overlay")
	}
	svc.mu.Lock()
	reprompt := child.pendingGateRepromptPrompt
	svc.mu.Unlock()
	if reprompt == "" {
		t.Fatal("real verdict after empty must arm the normal gate reprompt")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.id)
	if st.Status == "blocked" && strings.Contains(st.GateReason, "circuit breaker") {
		t.Fatalf("real verdict after empty must not trip the breaker, got blocked: %q", st.GateReason)
	}
}
