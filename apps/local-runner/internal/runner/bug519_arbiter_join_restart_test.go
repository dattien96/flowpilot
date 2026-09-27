package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-519 (live run-49109): after a runner restart a fully-finished
// tournament cohort is deliberately not reconstructed into s.runs
// (reconstructPendingChildSessions only restores live/pending members).
// tournamentJoinSatisfied's memory-only scan then waits forever on
// siblings that completed pre-restart — the arbiter parks cancelled and
// the run can never resume past it. The durable step-transition sidecar
// is the authoritative record.
func TestBug519_JoinSatisfiedViaStepLogAfterRestart(t *testing.T) {
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	// Pre-restart truth: both candidates stamped terminal in the durable
	// sidecar; NO child run exists in s.runs (post-restart memory).
	for _, id := range []string{"candidate-a", "candidate-b"} {
		if err := store.AppendStepTransition(context.Background(), parent.RunID, stepTransitionLine{
			RunID: parent.RunID, NodeID: id, Status: string(StepStatusDone), TS: "2026-09-26T11:17:44Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("join must be satisfied by durable terminal stamps when children are not in memory")
	}
}

// BUG-519 guard: a predecessor RUNNING in the sidecar with no terminal
// stamp still blocks the join — the fallback must not fail open.
func TestBug519_JoinStillBlocksOnNonTerminalStepLog(t *testing.T) {
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	// candidate-a terminal; candidate-b still RUNNING (last-wins).
	for _, l := range []stepTransitionLine{
		{RunID: parent.RunID, NodeID: "candidate-a", Status: string(StepStatusDone), TS: "2026-09-26T11:17:44Z"},
		{RunID: parent.RunID, NodeID: "candidate-b", Status: string(StepStatusRunning), TS: "2026-09-26T11:14:25Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parent.RunID, l); err != nil {
			t.Fatal(err)
		}
	}
	if svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("join must still block while a predecessor's last stamp is non-terminal")
	}
}

// BUG-519 guard: a FAILED candidate stamp satisfies the join — join:all
// collects every terminal outcome (live run-1890 contract preserved).
func TestBug519_JoinCountsFailedStepLogStamp(t *testing.T) {
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	if err := store.AppendStepTransition(context.Background(), parent.RunID, stepTransitionLine{
		RunID: parent.RunID, NodeID: "candidate-a", Status: string(StepStatusFailed), TS: "2026-09-26T11:17:44Z",
	}); err != nil {
		t.Fatal(err)
	}
	if !svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("failed-but-terminal predecessor must satisfy join:all")
	}
}
