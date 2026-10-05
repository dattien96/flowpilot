package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1203 (live run-183756): after the owner-debate overlay consumed tdd's
// completion, coder stayed PENDING forever — the sweep only re-drove RUNNING
// stamps, and the synthesis hub's early done hit the verdict gate which
// escalated into a WAITING park that could never self-resolve (spec_align/
// reviewer depend on validate, which depends on coder). A PENDING node whose
// direct done-edge predecessors are all terminal, on a silent run with no
// drivers, is a lost dispatch — re-drive it through the normal paths.

func ca1203SprintSlice() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

func ca1203Seed(t *testing.T) (*InteractiveService, *fakeWorkflowStore, string, []agentpack.FlowEdge, []agentpack.FlowNode) {
	t.Helper()
	edges, nodes := ca1203SprintSlice()
	svc, store, runID := seedDeadDispatchRun(t, nodes)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	// Silent run — the last provider event is far past the dispatch bound.
	rs.lastProviderEventAt = time.Now().UTC().Add(-3 * time.Minute)
	svc.mu.Unlock()
	return svc, store, runID, edges, nodes
}

// The core wedge: tdd DONE, coder PENDING, nothing in flight, run silent —
// coder's dispatch was consumed/lost. The sweep must spawn its leg.
func TestCA1203_PendingNodeWithTerminalPredRedispatches(t *testing.T) {
	svc, _, runID, _, _ := ca1203Seed(t)
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	spawned := false
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" {
			spawned = true
		}
	}
	svc.mu.Unlock()
	if !spawned {
		t.Fatal("PENDING coder with DONE predecessor on a silent run got no leg — chain stall persists")
	}
}

// A PENDING node whose predecessor is still running is correctly queued —
// the pred's own completion will fire the edge. Never double-dispatch.
func TestCA1203_PendingNodeWithLivePredSkips(t *testing.T) {
	svc, _, runID, _, _ := ca1203Seed(t)
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusRunning)
	svc.mu.Lock()
	svc.runs["run-tdd-leg"] = &interactiveRun{
		id:          "run-tdd-leg",
		parentRunID: runID,
		label:       "tdd",
		status:      RunStatusRunning,
	}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("dispatched coder while tdd leg still live — double dispatch")
	}
}

// The dispatch window is real: a pred stamped DONE seconds ago still owns the
// spawn goroutine. Only a silent run proves the dispatch was lost.
func TestCA1203_PendingNodeRecentActivitySkips(t *testing.T) {
	svc, _, runID, _, _ := ca1203Seed(t)
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
	svc.mu.Lock()
	svc.runs[runID].lastProviderEventAt = time.Now().UTC()
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("dispatched during the live dispatch window — in-flight spawn double-fired")
	}
}

// An entry-shape node (no done-edge predecessors) is the mount's own job —
// never the sweep's.
func TestCA1203_PendingEntryNodeSkips(t *testing.T) {
	svc, _, runID, _, _ := ca1203Seed(t)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "tdd" {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("entry node tdd (no done-preds) must never be sweep-dispatched")
	}
}

// The verdict-gate half of live run-183756: synthesis hub submits done while
// the reviewer's verdict is missing AND upstream work is undispatched-but-
// dispatchable. Escalating parks into a WAITING loop that can never produce
// the verdict — defer so the chain drives itself to the verdict instead.
func TestCA1203_HubDoneDefersOnDispatchablePendingUpstream(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1203SprintSlice()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	// Chain consumed up to tdd; coder pending (dispatch lost); downstream
	// pending. Reviewer never spawned, so its machine verdict is missing.
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis done")
	}
	if res.NextAction != "deferred_member_in_flight" {
		t.Fatalf("NextAction=%q, want deferred_member_in_flight — pending upstream work can still yield the verdict", res.NextAction)
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
		t.Fatalf("loop parked while dispatchable work remains: %+v", st)
	}
}

// Same shape but nothing is dispatchable (upstream mid-flight, pred RUNNING):
// the missing verdict is a real dead end — escalate stays the contract.
func TestCA1203_HubDoneEscalatesWhenNothingDispatchable(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1203SprintSlice()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	// tdd still RUNNING (leg live) → coder's pred non-terminal → nothing
	// dispatchable; reviewer missing verdict must escalate as before.
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusRunning)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)
	svc.mu.Lock()
	svc.runs["run-tdd-leg"] = &interactiveRun{
		id:          "run-tdd-leg",
		parentRunID: runID,
		label:       "tdd",
		status:      RunStatusRunning,
		turnInFlight: true,
	}
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis done")
	}
	if res.NextAction == "deferred_member_in_flight" {
		t.Fatal("deferred on no dispatchable work — the old escalate contract must hold")
	}
}
