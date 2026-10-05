package runner

// BUG-1186 — live run-204891 (CP-03 synthesis done-edge):
//
// A context-pressure advisory card (kind=context_pressure_90) fired on a
// cohort member child whose turn had already ended. The card sets
// pendingQuestionID WITHOUT gating the member's work — the child cannot
// rotate legs (contextResetEligibleLocked requires a root run) and no
// answer can yield the missing machine verdict. But every liveness check
// counted pendingQuestionID != "" as "still live":
//
//   - memberVerdictStillLive → missingVerdictLabelsWithLiveMember deferred
//     the done-edge missing-verdict escalation forever;
//   - hasActiveFlowChild → the hub watchdog never reached the wedge;
//   - the cohort stall sweep read the member as gated and skipped it;
//   - redriveQuietFlowLoop's quiet check stayed false — armed intents and
//     orphan redrives never ran.
//
// Contract:
//  a) advisory cards (context_pressure_90, usage_budget_exceeded) do NOT
//     hold member liveness — the missing verdict escalates to the designed
//     park instead of deferring forever;
//  b) real gating questions (kind "", quota_route_required, or an
//     unresolvable record) still count — the BUG-565 protection stands;
//  c) the same exclusion applies to the hub watchdog's busy/card checks.

import (
	"context"
	"testing"
	"time"
)

// setStaleStepStartedAt ages a flow step's StartedAt past the spawn grace so
// assertions rest on the child record, not a fresh RUNNING step stamp.
func setStaleStepStartedAt(t *testing.T, svc *InteractiveService, runID, nodeID string) {
	t.Helper()
	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	stale := time.Now().UTC().Add(-2 * defaultStallTimeout).Format(time.RFC3339Nano)
	steps, err := store.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range steps {
		if st.ID == nodeID || st.NodeID == nodeID {
			if err := store.ApplyStepTransition(context.Background(), runID, WorkflowStepTransition{
				StepID: st.ID, Patch: WorkflowStepPatch{StartedAt: &stale},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// bug1186PressureCard plants a pending context_pressure_90 question record
// pinned on the run — the live advisory shape (no status flip, no verdict
// path behind it).
func bug1186PressureCard(t *testing.T, svc *InteractiveService, run *interactiveRun) string {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rec := &questionRecord{
		id:        svc.nextID("q"),
		runID:     run.id,
		prompt:    "context_pressure: this session is at 91% of its context window",
		options:   []QuestionOption{{Label: "continue"}, {Label: "stop"}},
		status:    "pending",
		resolve:   make(chan questionResolveResult, 1),
		expiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		kind:      contextPressureQuestionKind,
	}
	svc.questions[rec.id] = rec
	run.pendingQuestionID = rec.id
	return rec.id
}

// (a) The live failure: a member holding only an advisory card must not
// defer the done-edge missing-verdict escalation.
func TestBug1186_AdvisoryCardMemberDoesNotDefer(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusRunning)
	bug1186PressureCard(t, svc, svc.runs[childID])
	// The member's step row is stale past spawn grace — the assertion must
	// rest on the child record, not a fresh RUNNING stamp.
	svc.mu.Lock()
	svc.runs[childID].turnInFlight = false
	svc.mu.Unlock()
	setStaleStepStartedAt(t, svc, runID, "reviewer")

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction == "deferred_member_in_flight" {
		t.Fatal("advisory context-pressure card held a zombie member 'live' — verdict defer wedged")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" {
		t.Fatalf("loop=%q, want blocked (escalate)", st.Status)
	}
}

// (b) Gating question kinds still defer — a quota_route_required card is a
// real user gate on the member's own progress.
func TestBug1186_GatingCardMemberStillDefers(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusRunning)
	svc.mu.Lock()
	child := svc.runs[childID]
	rec := &questionRecord{
		id:        svc.nextID("q"),
		runID:     child.id,
		prompt:    "quota_route_required: binding unusable",
		options:   []QuestionOption{{Label: "rotate"}, {Label: "stop"}},
		status:    "pending",
		resolve:   make(chan questionResolveResult, 1),
		expiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		kind:      quotaRouteQuestionKind,
	}
	svc.questions[rec.id] = rec
	child.pendingQuestionID = rec.id
	svc.mu.Unlock()
	setStaleStepStartedAt(t, svc, runID, "reviewer")

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction != "deferred_member_in_flight" {
		t.Fatalf("NextAction=%q — a gating quota card on the member must still defer", res.NextAction)
	}
}

// (b') An advisory question does not make the member count as an active
// flow child — the hub watchdog must see through it to the wedge.
func TestBug1186_AdvisoryCardNotActiveFlowChild(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusRunning)
	bug1186PressureCard(t, svc, svc.runs[childID])
	if svc.hasActiveFlowChild(runID) {
		t.Fatal("a member holding only an advisory card is not active work")
	}
}

// (b'') And the unresolvable-record fallback stays fail-closed: a pending
// question id with no record must still count as live (BUG-565 posture).
func TestBug1186_UnresolvableQuestionStillCountsLive(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusRunning)
	svc.mu.Lock()
	svc.runs[childID].pendingQuestionID = "q-dangling"
	svc.mu.Unlock()
	if !svc.hasActiveFlowChild(runID) {
		t.Fatal("dangling pendingQuestionID must still count live — fail closed")
	}
}
