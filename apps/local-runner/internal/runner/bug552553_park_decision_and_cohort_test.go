package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-552: the escalate park's gate-decision routing ACKed `routed_to` and
// rode the resume prompt only — any re-park cancelling the carrying turn
// (or an orphan-redrive answering with its own generic prompt) swallowed
// the operator's decision permanently. The routed decision must be
// recorded in durable pendingAgentContext BEFORE any dispatch.

// bug552EscalatedParent builds a flow parent whose loop is blocked on an
// escalate park, with one parked orphan child (waiting_user_approval +
// pending gate debt) — the shape that takes the orphan-redrive early
// return in resumeFlowWithFeedback, where the decision used to vanish.
func bug552EscalatedParent(svc *InteractiveService, runID, childID string) {
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		providerKey:      ProviderKeyCodex,
		flowEngineDriven: true,
		autoOrchestrate:  true,
		status:           RunStatusRunning,
		activeFlowNodes:  []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code"}, {ID: "synthesis", Behavior: "hub.inline"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                   childID,
		parentRunID:          runID,
		providerKey:          ProviderKeyCodex,
		label:                "coder",
		status:               RunStatusWaitingUserApr,
		pendingGateCodePaths: []string{"src/tool.go"},
		subs:                 map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, childID)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.Round = 3
		st.Cap = 5
		return st
	})
}

func TestBug552_RoutedDecisionRecordedDurably(t *testing.T) {
	svc := bug289Service(t)
	runID, childID := "run-552a", "child-552a"
	bug552EscalatedParent(svc, runID, childID)

	const decision = "User decision on the gate/decision card: amend the requirement/contract (suggest-requirement-change)."
	if _, err := svc.resumeFlowWithFeedback(runID, decision); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	ctx := svc.runs[runID].pendingAgentContext
	svc.mu.Unlock()
	found := false
	for _, note := range ctx {
		if strings.Contains(note, "suggest-requirement-change") {
			found = true
		}
	}
	if !found {
		t.Fatalf("routed gate-decision never recorded — pendingAgentContext=%v (the escalate-repark shape that swallowed it live)", ctx)
	}
}

// The durable note must survive a re-park so the next real turn still
// consumes it — parkFlowForAwaitingUser drops auto-intents but must not
// wipe pendingAgentContext.
func TestBug552_DecisionSurvivesSubsequentPark(t *testing.T) {
	svc := bug289Service(t)
	runID, childID := "run-552b", "child-552b"
	bug552EscalatedParent(svc, runID, childID)

	const decision = "User decision on the gate/decision card: keep the test and fix the code (keep-test-fix-code)."
	if _, err := svc.resumeFlowWithFeedback(runID, decision); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	// Re-park cancels whatever the resume dispatched — the durable decision
	// must still be buffered for the next turn.
	svc.parkFlowForAwaitingUser(runID)

	svc.mu.Lock()
	ctx := svc.runs[runID].pendingAgentContext
	svc.mu.Unlock()
	found := false
	for _, note := range ctx {
		if strings.Contains(note, "keep-test-fix-code") {
			found = true
		}
	}
	if !found {
		t.Fatalf("re-park wiped the routed decision — pendingAgentContext=%v", ctx)
	}
}

// BUG-553: park cancelled in-flight children but never buffered their
// cohort results — unlike the Stop path (Task-241 B1). The frozen member's
// seat stayed held: buffer short of expected forever, hasOpenCohort true
// forever, flow-control done soft-deferred indefinitely (live run-15525).

// bug553CohortParent builds a parent with a 2-member cohort: member-a
// already completed (result buffered), member-b still live when the park
// freezes it.
func bug553CohortParent(svc *InteractiveService, runID, childA, childB, cohortID string) {
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		providerKey:      ProviderKeyCodex,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		activeFlowNodes:  []agentpack.FlowNode{{ID: "owner_1"}, {ID: "owner_2"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childA] = &interactiveRun{
		id:           childA,
		parentRunID:  runID,
		providerKey:  ProviderKeyCodex,
		label:        "owner_1",
		flowCohortId: cohortID,
		status:       RunStatusCompleted,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.runs[childB] = &interactiveRun{
		id:           childB,
		parentRunID:  runID,
		providerKey:  ProviderKeyCodex,
		label:        "owner_2",
		flowCohortId: cohortID,
		status:       RunStatusRunning,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, childA)
	svc.agentOrchestrator.registerChild(runID, childB)
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "owner_1", Provider: "codex", Status: "completed",
	})
}

func TestBug553_ParkReleasesFrozenCohortSeats(t *testing.T) {
	svc := bug289Service(t)
	runID, childA, childB, cohortID := "run-553a", "child-a", "child-b", "cohort-553"
	bug553CohortParent(svc, runID, childA, childB, cohortID)

	if !svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("precondition: cohort should be open before park (1/2 buffered)")
	}
	svc.parkFlowForAwaitingUser(runID)

	if svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("park left the frozen member's cohort seat held — hasOpenCohort still true, flow-control done soft-defers forever")
	}
}

// The live residue shape: a child parked `waiting_user_approval` by an
// EARLIER park — not running, so the park's running→waiting stamp skips it —
// still held its cohort seat on the next park.
func TestBug553_StaleWaitingChildSeatAlsoReleased(t *testing.T) {
	svc := bug289Service(t)
	runID, childA, childB, cohortID := "run-553b", "child-a2", "child-b2", "cohort-553b"
	bug553CohortParent(svc, runID, childA, childB, cohortID)
	svc.mu.Lock()
	svc.runs[childB].status = RunStatusWaitingUserApr
	svc.mu.Unlock()

	svc.parkFlowForAwaitingUser(runID)

	if svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("stale waiting_user_approval child still counted as an open cohort member")
	}
}

// A member revived after the park (reprompt/resume intent, e.g. the
// CA-1062 gated-child path) must be able to REPLACE its released-seat
// placeholder with a real result — cancelled is a placeholder, not a
// dedup wall.
func TestBug553_RevivedMemberReplacesCancelledSeat(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-553c"
	cohortID := "cohort-553c"
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "owner_1", Provider: "codex", Status: "completed",
	})
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "owner_2", Provider: "codex", Status: "cancelled",
	})

	// owner_2 revived and produced a real result.
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "owner_2", Provider: "codex", Status: "completed", FinalMessage: "reprompted verdict",
	})

	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("revived member's real result was deduped away — barrier never joins")
	}
	entries := svc.agentOrchestrator.drainCohort(runID, cohortID)
	var got *cohortEntry
	for i := range entries {
		if entries[i].Label == "owner_2" {
			got = &entries[i]
		}
	}
	if got == nil || got.Status != "completed" || got.FinalMessage != "reprompted verdict" {
		t.Fatalf("revived member entry = %+v, want the real completed result, not the cancelled placeholder", got)
	}
}
