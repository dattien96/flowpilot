package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-580 (live run-100368): reviewer verdict loss, two stacked layers.
//
// Layer A — snapshotReviewCohortVerdictsLocked replaced the WHOLE
// lastReviewCohortVerdicts map on every cohort join. A reviewer verdict
// recorded at the review-cohort join was wiped by a LATER owner-debate join
// (whose members carry their own labels), so the synthesis hub gate read
// missing forever.
//
// Layer B — appendCohortResult's drained-cohort tombstone dropped a member
// re-completion whole: the redrive turn's submit_review_outcome had already
// been consumed out of pendingReviewVerdictByLabel into the entry's
// MachineVerdict, the entry was dropped, and the verdict vanished from both
// places.

// TestBUG580SnapshotJoinPreservesOtherCohortVerdicts pins layer A: a cohort
// join must merge per-label into the verdict view, not replace it — a
// non-review cohort's join (owner_debate) must not erase the reviewer's
// recorded verdict.
func TestBUG580SnapshotJoinPreservesOtherCohortVerdicts(t *testing.T) {
	svc := bug580Svc(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
	})
	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
		{Label: "owner_1", Status: "completed", MachineVerdict: "continue"},
		{Label: "owner_2", Status: "completed", MachineVerdict: "continue"},
	})
	svc.mu.Lock()
	got := svc.runs[parent.RunID].lastReviewCohortVerdicts
	svc.mu.Unlock()
	if got["reviewer"] != "approved" {
		t.Fatalf("reviewer verdict wiped by a later non-review cohort join: %v", got)
	}
	if got["owner_1"] != "continue" || got["owner_2"] != "continue" {
		t.Fatalf("owner verdicts missing after their own join: %v", got)
	}
}

// TestBUG580SnapshotVerdictlessRejoinClearsStale pins the other half of the
// merge contract: a member re-joining WITHOUT a machine verdict (reprompt
// cap exhausted) must clear its own stale verdict — otherwise a superseded
// approval satisfies the gate for a round that produced nothing. Labels not
// present in the join are untouched.
func TestBUG580SnapshotVerdictlessRejoinClearsStale(t *testing.T) {
	svc := bug580Svc(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
		{Label: "reviewer_b", Status: "completed", MachineVerdict: "approved"},
	})
	// Round 2 re-join: reviewer completed verdict-less; reviewer_b did not
	// rejoin at all in this drain — only reviewer's own verdict may clear.
	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
		{Label: "reviewer", Status: "completed"},
	})
	svc.mu.Lock()
	got := svc.runs[parent.RunID].lastReviewCohortVerdicts
	svc.mu.Unlock()
	if v, ok := got["reviewer"]; ok {
		t.Fatalf("stale verdict survived a verdict-less re-join for the same label: %q", v)
	}
	if got["reviewer_b"] != "approved" {
		t.Fatalf("unrelated label cleared by another member's re-join: %v", got)
	}
}

// TestBUG580DrainedCohortReCompletionRestoresVerdict pins layer B: a member
// leg re-driven AFTER its cohort drained (bug-565 redrive path) must not lose
// the verdict it just recorded — the entry is correctly dropped from the dead
// barrier, but the verdict must be restored to pendingReviewVerdictByLabel so
// mergePendingReviewVerdictsLocked surfaces it to the hub gate.
func TestBUG580DrainedCohortReCompletionRestoresVerdict(t *testing.T) {
	svc := bug580Svc(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// The review cohort already delivered and tombstoned — exactly the state a
	// bug-565 verdict-deficient redrive completes into.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "review", 1)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "review", cohortEntry{Label: "reviewer", Status: "completed"})
	svc.agentOrchestrator.drainCohort(parent.RunID, "review")
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.autoOrchestrate = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	prs.activeFlowAcceptanceNodes = []string{"synthesis"}
	// The redriven turn's submit_review_outcome recorded into the pending map —
	// settle will consume it into the cohort entry that then gets dropped.
	prs.pendingReviewVerdictByLabel = map[string]string{"reviewer": "approved"}
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "reviewer"
	rs.flowCohortId = "review"
	rs.status = RunStatusCompleted
	svc.settleFlowChildTurnCompletedLocked(rs, "reviewed again", ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed again"})
	restored := prs.pendingReviewVerdictByLabel["reviewer"]
	svc.mu.Unlock()
	if restored != "approved" {
		t.Fatalf("verdict from a drained-cohort re-completion was lost: pendingReviewVerdictByLabel[reviewer]=%q", restored)
	}
}

func bug580Svc(t *testing.T) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}
