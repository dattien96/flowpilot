package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// contextCodingReviewSynthesisTestNodesAndEdges mirrors context-coding-
// review-synthesis.yaml's shape (context -> coder -> reviewer cohort ->
// synthesis -> tele-step -> done, with synthesis's back edge to coder), the
// exact topology BUG-286 was diagnosed against. coder.DependsOn is set
// explicitly to mirror BUG-282's edge-derived value a Supabase-mirrored flow
// actually carries at load time (forwardEdgeSources derives it from the
// context->coder forward edge) — the precise condition that made
// entryDelegateNodes (and therefore flowEntryNodeID) exclude "coder" and
// return "" for this shape before the fix.
func contextCodingReviewSynthesisTestNodesAndEdges() ([]agentpack.FlowNode, []agentpack.FlowEdge) {
	nodes := []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md", DependsOn: []string{"context"}},
		{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
		{ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "tele-step", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "context", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
		{From: "reviewer_correctness", To: "synthesis", When: "done", Kind: "forward"},
		{From: "reviewer_security", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
		{From: "synthesis", To: "tele-step", When: "done", Kind: "forward"},
		{From: "tele-step", To: "done", When: "done", Kind: "forward"},
	}
	return nodes, edges
}

// TestForwardReachableNodeIDsExcludesUpstreamNodesAndBackEdges is the direct
// unit-level proof for BUG-286's core primitive.
func TestForwardReachableNodeIDsExcludesUpstreamNodesAndBackEdges(t *testing.T) {
	_, edges := contextCodingReviewSynthesisTestNodesAndEdges()
	reachable := forwardReachableNodeIDs(edges, "coder")

	for _, id := range []string{"reviewer_correctness", "reviewer_security", "synthesis", "tele-step"} {
		if !reachable[id] {
			t.Errorf("expected %q forward-reachable from coder, got reachable=%v", id, reachable)
		}
	}
	if reachable["context"] {
		t.Error("context is UPSTREAM of coder (only reachable via context->coder, the wrong direction) and must not appear")
	}
	if reachable["coder"] {
		t.Error("the start node itself must never be included in its own reachable set")
	}
	if reachable["done"] || reachable["ask_user"] {
		t.Error("terminal pseudo-nodes must never be included")
	}
}

// TestForwardReachableNodeIDsIgnoresBackEdges proves a back edge (synthesis's
// own re-entry edge to coder) is never walked as forward reachability — if it
// were, the loop's own re-entry point would be wrongly treated as "part of
// what resets for a new round" territory rather than the anchor a caller
// marks RUNNING.
func TestForwardReachableNodeIDsIgnoresBackEdges(t *testing.T) {
	_, edges := contextCodingReviewSynthesisTestNodesAndEdges()
	reachable := forwardReachableNodeIDs(edges, "synthesis")
	if reachable["coder"] {
		t.Error("synthesis->coder is a BACK edge and must not be walked as forward reachability")
	}
	if !reachable["tele-step"] {
		t.Error("expected tele-step forward-reachable from synthesis via its own forward edge")
	}
}

// TestApplyFlowControlContinueResetsOnlyForwardReachableNodesKeepingUpstreamEntryDone
// is the direct regression proof for BUG-286 (observed live as run-9225: the
// desktop showed both "context" and "coder" stuck PENDING through an entire
// review round, only flipping once coder itself finished). Before the fix,
// the reset used flowEntryNodeID (entryDelegateNodes), which excludes any
// node with a non-empty DependsOn — true for "coder" on every Supabase-
// mirrored flow (BUG-282's edge-derived DependsOn) — so it returned "",
// making setFlowStepStatus(parentRunID, "", RUNNING) a silent no-op and
// resetting EVERY other node (including the once-only "context" entry,
// upstream of the loop and never meant to re-run) to PENDING.
func TestApplyFlowControlContinueResetsOnlyForwardReachableNodesKeepingUpstreamEntryDone(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes, edges := contextCodingReviewSynthesisTestNodesAndEdges()
	svc.mu.Lock()
	svc.runs[runID].activeFlowNodes = nodes
	svc.runs[runID].activeFlowEdges = edges
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)

	// Simulate round 0 having fully completed.
	for _, id := range []string{"context", "coder", "reviewer_correctness", "reviewer_security", "synthesis"} {
		svc.setFlowStepStatus(context.Background(), runID, id, StepStatusDone)
	}

	if _, err := svc.applyFlowControl(runID, FlowControlInput{
		Status: "continue", Summary: "reviewers requested changes",
		Payload: map[string]any{"issues": []any{"x"}},
	}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	steps, loadErr := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if loadErr != nil {
		t.Fatalf("LoadRunSteps: %v", loadErr)
	}

	// The core regression: an upstream, once-only entry node must keep DONE.
	if st, _ := stepByID(steps, "context"); st.Status != StepStatusDone {
		t.Fatalf("context = %q, want still DONE (upstream once-only entry node must not reset)", st.Status)
	}
	// The back-edge's own re-entry target must be marked RUNNING, not left as
	// a silent no-op.
	if st, _ := stepByID(steps, "coder"); st.Status != StepStatusRunning {
		t.Fatalf("coder = %q, want RUNNING (the re-entry node for this new round)", st.Status)
	}
	// Everything actually downstream of the re-entry node resets for the new
	// round, including a hub.notify successor chained after synthesis.
	for _, id := range []string{"reviewer_correctness", "reviewer_security", "synthesis", "tele-step"} {
		if st, _ := stepByID(steps, id); st.Status != StepStatusPending {
			t.Errorf("%s = %q, want PENDING (forward-reachable from the re-entry node)", id, st.Status)
		}
	}
}

// TestApplyFlowControlContinueFallsBackToEntryNodeWithoutBackEdge proves the
// defensive fallback: a flow with no declared "continue" back-edge (shouldn't
// occur for anything reaching NextAction=="looping" today, but must not
// crash or silently do nothing) still resolves a re-entry node via the
// pre-existing flowEntryNodeID path.
func TestApplyFlowControlContinueFallsBackToEntryNodeWithoutBackEdge(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := reviewLoopTestNodes() // coder has no DependsOn in this fixture
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
		{From: "reviewer_correctness", To: "synthesis", When: "done", Kind: "forward"},
		{From: "reviewer_security", To: "synthesis", When: "done", Kind: "forward"},
		// Deliberately no synthesis->coder back edge.
	}
	svc.mu.Lock()
	svc.runs[runID].activeFlowNodes = nodes
	svc.runs[runID].activeFlowEdges = edges
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	for _, id := range []string{"coder", "reviewer_correctness", "reviewer_security", "synthesis"} {
		svc.setFlowStepStatus(context.Background(), runID, id, StepStatusDone)
	}

	if _, err := svc.applyFlowControl(runID, FlowControlInput{
		Status: "continue", Summary: "x", Payload: map[string]any{"issues": []any{"x"}},
	}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if st, _ := stepByID(steps, "coder"); st.Status != StepStatusRunning {
		t.Fatalf("coder = %q, want RUNNING via the flowEntryNodeID fallback", st.Status)
	}
}
