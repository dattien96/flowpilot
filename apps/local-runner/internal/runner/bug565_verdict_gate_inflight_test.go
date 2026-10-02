package runner

// BUG-565 — live run-69320 (PrivateVault Task-024 sprint, 2026-10-02):
//
// Failure A (14:28:30→14:28:53): the reviewer leg spawned 23s before the
// synthesis hub submitted done. The verdict gate (hubDoneVerdictError /
// synthesisDoneVerdictError) saw a missing machine verdict and escalated —
// the park CANCELLED the still-running reviewer mid-turn, so its verdict
// could never arrive. The gate had no "member still in flight" defer — the
// same class BUG-560/561 already fixed on the audit side
// (flow_audit_deferred_open_cohort / hasRunningSprintStep).
//
// Failure B (14:01→14:03): when the verdict was missing because the member
// had NEVER spawned (stale-hub-done swallowed the dispatch), Continue
// fell through resumeVerdictDeficientMembers → no live child maps → generic
// hub resume → identical reject → re-park, 16s later, forever.
//
// Contract:
//  a) a missing verdict whose member leg is still live is PROVISIONAL — defer
//     (unstamp the decision, no escalate, no park, member not cancelled);
//     the member's settle/join re-invokes the hub.
//  b) Continue on a verdict park whose deficient member was never spawned
//     spawns that member leg — a hub re-prompt can never produce the verdict.
//  c) a missing verdict with NO live member and none spawned still escalates
//     (fail-closed) — defer is only for genuinely in-flight work.

import (
	"context"
	"testing"
)

// bug565ReviewerChild registers a reviewer-labeled child in the given status.
func bug565ReviewerChild(t *testing.T, svc *InteractiveService, parentID string, pk ProviderKey, status RunStatus) string {
	t.Helper()
	childID := parentID + "-rev-live"
	svc.mu.Lock()
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		label:             "reviewer",
		role:              "reviewer",
		agentName:         "reviewer",
		status:            status,
		agentStatus:       string(status),
		providerKey:       pk,
		providerAccountID: svc.runs[parentID].providerAccountID,
		flowCohortId:      "flow-auto-validate-round-0",
		stepID:            "reviewer",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	return childID
}

// (a) hubDoneVerdictError path: hub done with the reviewer leg still running
// must defer — never escalate/park/cancel the member mid-turn.
func TestBug565_HubDoneDefersWhileReviewerInFlight(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := newFlowTestRun(t)
			edges, nodes := ca1098VerdictTopology()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			childID := bug565ReviewerChild(t, svc, runID, pk, RunStatusRunning)
			svc.mu.Lock()
			svc.runs[childID].turnInFlight = true
			svc.mu.Unlock()
			svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
			if !handled {
				t.Fatalf("%s: advanceHubDoneThroughEdge must take over synthesis->audit", pk)
			}
			if res.NextAction != "deferred_member_in_flight" {
				t.Fatalf("%s: NextAction=%q, want deferred_member_in_flight (member still live)", pk, res.NextAction)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
				t.Fatalf("%s: loop parked while reviewer still in flight — the verdict can never arrive", pk)
			}
			svc.mu.Lock()
			childStatus := svc.runs[childID].status
			svc.mu.Unlock()
			if childStatus != RunStatusRunning {
				t.Fatalf("%s: in-flight reviewer child %q after defer — park must not cancel it", pk, childStatus)
			}
		})
	}
}

// (a) same defer applies on the synthesisDoneVerdictError (acceptance-gate)
// path — hub submits via submit_review_outcome while the member runs.
func TestBug565_SynthesisDoneDefersWhileReviewerInFlight(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := newFlowTestRun(t)
			edges, nodes := ca1098VerdictTopology()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			svc.mu.Lock()
			svc.runs[runID].activeFlowAcceptanceNodes = []string{"synthesis"}
			svc.mu.Unlock()
			childID := bug565ReviewerChild(t, svc, runID, pk, RunStatusRunning)
			svc.mu.Lock()
			svc.runs[childID].turnInFlight = true
			svc.mu.Unlock()
			svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

			res, err := svc.applyFlowControl(runID, FlowControlInput{
				Status: "done", Summary: "approved",
				viaReviewOutcome: true, reviewOutcomeStatus: "approved",
			})
			if err != nil {
				t.Fatalf("%s: in-flight member defer must not error: %v", pk, err)
			}
			if res.NextAction != "deferred_member_in_flight" {
				t.Fatalf("%s: NextAction=%q, want deferred_member_in_flight", pk, res.NextAction)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
				t.Fatalf("%s: loop parked while reviewer still in flight", pk)
			}
		})
	}
}

// (c) no member live and none spawned → still escalates (fail-closed), so a
// genuinely lost verdict surfaces as an actionable park, not a silent defer.
func TestBug565_HubDoneEscalatesWhenMemberNeverSpawned(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction == "deferred_member_in_flight" {
		t.Fatal("deferred with no live member — defer requires in-flight work")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" {
		t.Fatalf("loop=%q, want blocked (escalate) when the verdict is truly unproduced", st.Status)
	}
}

// (b) Continue on a verdict park where the deficient member was NEVER
// spawned must spawn that member leg — the hub cannot produce the verdict.
func TestBug565_ContinueSpawnsNeverSpawnedReviewer(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = ca1098MissingVerdictGateReason
		return st
	})
	// Mirror resumeFlowWithFeedback's ordering: the loop is unblocked BEFORE
	// the verdict-deficient member re-drive runs (BUG-432 spawn guard refuses
	// children on a blocked loop — legit dispatch paths unblock first).
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		return st
	})

	if !svc.resumeVerdictDeficientMembers(runID) {
		t.Fatal("resumeVerdictDeficientMembers must handle a never-spawned deficient member")
	}

	svc.mu.Lock()
	spawned := ""
	for _, childID := range svc.agentOrchestrator.listChildren(runID) {
		if c := svc.runs[childID]; c != nil && c.label == "reviewer" {
			spawned = childID
		}
	}
	svc.mu.Unlock()
	if spawned == "" {
		t.Fatal("no reviewer member leg spawned — Continue would loop on the identical park")
	}
	if st := svc.lookupFlowStepStatus(runID, "reviewer"); st != StepStatusRunning {
		t.Fatalf("reviewer step = %q, want RUNNING after member spawn", st)
	}
}
