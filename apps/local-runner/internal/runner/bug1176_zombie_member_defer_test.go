package runner

// BUG-1176 — live run-183756 (PrivateVault Task-038 sprint boundary):
//
// The cohort children's turns ended without a durable terminal stamp —
// run records stuck RunStatusRunning with turnInFlight=false, no pending
// turn prompt, no user gate, no armed reprompt/resume intent: zombies.
// missingVerdictLabelsWithLiveMember counted bare non-terminal status as
// "live", so every synthesis hub-done returned deferred_member_in_flight.
// The defer waits for "the member's settle/join to re-invoke the hub" —
// a zombie has no settle left to fire — so the loop sat RUNNING with no
// active turns until the run was cancelled (transitions: synthesis
// RUNNING 13:04 → all nodes CANCELED 13:56). The sprint-boundary
// auto-advance never ran and Task-039 never started: the no-auto-next-
// task regression.
//
// Contract:
//  a) a zombie member (non-terminal status but no live verdict path) must
//     NOT hold the defer — the missing verdict escalates to the designed
//     park and Continue redrives the member (CA-1098 D6).
//  b) genuinely live member shapes still defer (BUG-565 protection):
//     in-flight turn, queued turn prompt, pending user gate, armed
//     reprompt/resume intent, Starting status.
//  c) a RUNNING flow step with NO child record counts live only inside
//     the spawn grace — a stale RUNNING step with no child is a dead
//     dispatch and escalates.
//  d) a terminal child must not suppress the step channel: a re-dispatch
//     already stamped RUNNING with the new child row not yet inserted
//     still defers.

import (
	"context"
	"testing"
	"time"
)

// bug1176ZombieReviewer registers a reviewer-labeled child whose run record
// is stuck non-terminal with NO live verdict path — the live zombie shape.
func bug1176ZombieReviewer(t *testing.T, svc *InteractiveService, runID string) string {
	t.Helper()
	childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusRunning)
	svc.mu.Lock()
	c := svc.runs[childID]
	if c.turnInFlight || c.pendingTurnPrompt != "" || c.pendingApprovalID != "" ||
		c.pendingQuestionID != "" || c.pendingGateRepromptPrompt != "" ||
		c.pendingGateRepromptStepID != "" || c.pendingResumePrompt != "" ||
		c.postTurnGateCancel != nil {
		t.Fatal("fixture is not a zombie")
	}
	svc.mu.Unlock()
	return childID
}

// (a) The live failure: zombie member + missing verdict must escalate, not
// defer forever.
func TestBug1176_ZombieMemberDoesNotDefer(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	bug1176ZombieReviewer(t, svc, runID)
	svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction == "deferred_member_in_flight" {
		t.Fatal("deferred on a zombie member — no settle will ever re-invoke the hub")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" {
		t.Fatalf("loop=%q, want blocked (escalate) — a truly missing verdict must surface", st.Status)
	}
}

// (a) A zombie member must also not shield the stale RUNNING step row:
// the child record is authoritative over the step stamp.
func TestBug1176_ZombieChildStaleStepStillEscalates(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	bug1176ZombieReviewer(t, svc, runID)
	// The zombie's own step row also stuck RUNNING — exactly the live pair.
	svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)
	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	// Age the step stamp past the spawn grace so even the no-child channel
	// could not count it live — the assertion must rest on the child.
	stale := time.Now().UTC().Add(-2 * defaultStallTimeout).Format(time.RFC3339Nano)
	steps, err := store.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range steps {
		if st.ID == "reviewer" || st.NodeID == "reviewer" {
			if err := store.ApplyStepTransition(context.Background(), runID, WorkflowStepTransition{
				StepID: st.ID, Patch: WorkflowStepPatch{StartedAt: &stale},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction == "deferred_member_in_flight" {
		t.Fatal("deferred on zombie child — stale step stamp must not resurrect liveness")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" {
		t.Fatalf("loop=%q, want blocked", st.Status)
	}
}

// (b) Every genuinely-live member shape still defers — the BUG-565 contract
// (a mid-flight member must never be escalated/cancelled out from under
// its own verdict).
func TestBug1176_LiveMemberShapesStillDefer(t *testing.T) {
	cases := []struct {
		name   string
		status RunStatus
		mutate func(c *interactiveRun)
	}{
		{"turn in flight", RunStatusRunning, func(c *interactiveRun) { c.turnInFlight = true }},
		{"queued turn", RunStatusRunning, func(c *interactiveRun) { c.pendingTurnPrompt = "retry" }},
		{"approval gate", RunStatusWaitingApproval, func(c *interactiveRun) { c.pendingApprovalID = "ap-1" }},
		{"question gate", RunStatusWaitingQuestion, func(c *interactiveRun) { c.pendingQuestionID = "q-1" }},
		{"reprompt armed", RunStatusRunning, func(c *interactiveRun) { c.pendingGateRepromptPrompt = "verdict reprompt" }},
		{"reprompt step armed", RunStatusRunning, func(c *interactiveRun) { c.pendingGateRepromptStepID = "reviewer" }},
		{"resume armed", RunStatusRunning, func(c *interactiveRun) { c.pendingResumePrompt = "resume" }},
		{"starting", RunStatusStarting, func(c *interactiveRun) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, runID := newFlowTestRun(t)
			edges, nodes := ca1098VerdictTopology()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			childID := bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, tc.status)
			svc.mu.Lock()
			tc.mutate(svc.runs[childID])
			svc.mu.Unlock()

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
			if !handled {
				t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
			}
			if res.NextAction != "deferred_member_in_flight" {
				t.Fatalf("NextAction=%q, want deferred_member_in_flight — %s is still live", res.NextAction, tc.name)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
				t.Fatalf("loop parked while member live (%s)", tc.name)
			}
		})
	}
}

// (c) No child record at all: a fresh RUNNING step marks the dispatch
// window and defers; a RUNNING step older than the spawn grace is a dead
// dispatch and escalates.
func TestBug1176_NoChildStepSpawnGrace(t *testing.T) {
	t.Run("fresh running step defers", func(t *testing.T) {
		svc, runID := newFlowTestRun(t)
		edges, nodes := ca1098VerdictTopology()
		setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
		svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

		res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
		if !handled {
			t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
		}
		if res.NextAction != "deferred_member_in_flight" {
			t.Fatalf("NextAction=%q, want deferred_member_in_flight (dispatch in flight)", res.NextAction)
		}
	})
	t.Run("stale running step escalates", func(t *testing.T) {
		svc, runID := newFlowTestRun(t)
		edges, nodes := ca1098VerdictTopology()
		setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
		svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)
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
			if st.ID == "reviewer" || st.NodeID == "reviewer" {
				if err := store.ApplyStepTransition(context.Background(), runID, WorkflowStepTransition{
					StepID: st.ID, Patch: WorkflowStepPatch{StartedAt: &stale},
				}); err != nil {
					t.Fatal(err)
				}
			}
		}

		res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
		if !handled {
			t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
		}
		if res.NextAction == "deferred_member_in_flight" {
			t.Fatal("deferred on a dead dispatch — stale RUNNING step with no child is not live")
		}
		if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" {
			t.Fatalf("loop=%q, want blocked", st.Status)
		}
	})
}

// (d) A terminal member child must not suppress the step channel: its
// settle already ran, and a fresh RUNNING step means a re-dispatch is in
// flight whose child row has not landed yet.
func TestBug1176_TerminalChildDoesNotBlockStepChannel(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	bug565ReviewerChild(t, svc, runID, ProviderKeyClaude, RunStatusCompleted)
	svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction != "deferred_member_in_flight" {
		t.Fatalf("NextAction=%q, want deferred_member_in_flight (re-dispatch in flight)", res.NextAction)
	}
}
