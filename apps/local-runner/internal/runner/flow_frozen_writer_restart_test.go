package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// TestRestartBetweenFreezeAndCoderPreservesContract simulates a runner
// restart landing between the planner freezing a contract and the coder
// child ever running: a fresh runContractFreezeNode call with the IDENTICAL
// planner draft (as a recovered/replayed flow would deliver) must reuse the
// already-frozen contract rather than mint a second version or re-run
// freeze's own side effects — CP-55 spec 3.17's "duplicate freeze delivery
// with identical input returns the existing contract."
func TestRestartBetweenFreezeAndCoderPreservesContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("first freeze: expected true (handled)")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := store.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("expected a frozen record after the first freeze, ok=%v err=%v", ok, err)
	}

	// Simulate the restart: a fresh FrozenStore instance (as a recovered
	// process would open) sees the same durable record; re-delivering the
	// identical planner draft must not mint a second version.
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("replayed freeze: expected true (handled)")
	}
	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, ok, err := reopened.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("expected the frozen record to still exist after replay, ok=%v err=%v", ok, err)
	}
	if second.ContractID != first.ContractID || second.Version != first.Version {
		t.Fatalf("a replayed identical freeze must reuse the existing contract, not mint a new one: first=%+v second=%+v", first, second)
	}
	versions, err := reopened.ListVersionsForStep(runID, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected exactly 1 version after a replayed identical freeze, got %d: %+v", len(versions), versions)
	}
}

// TestRestartBetweenCoderAndValidationPreservesPendingCanonical simulates a
// runner restart landing right after a frozen writer stages a pending
// Canonical Head update but before the Flow reaches terminal acceptance: a
// fresh PendingCanonicalStore instance (as a recovered process would open)
// must still see the staged update — it is durable, not held only in the
// staging call's own in-memory state.
func TestRestartBetweenCoderAndValidationPreservesPendingCanonical(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	// Simulate the restart: open a completely fresh store instance.
	reopened, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	pending, ok, err := reopened.GetPending(parentID, "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected the pending canonical update to survive a restart, ok=%v err=%v", ok, err)
	}
	if pending.Head.BehaviorStatement != "fix rounding" {
		t.Fatalf("BehaviorStatement = %q, want fix rounding", pending.Head.BehaviorStatement)
	}
	// The real Canonical Head must still be absent — recovery must not have
	// been misread as terminal acceptance.
	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || found {
		t.Fatalf("Canonical Head must remain absent across the simulated restart, found=%v err=%v", found, err)
	}
}

// TestRestartDuringTerminalFinalizationConverges simulates a crash landing
// mid-finalize for a frozen writer's flow: one feature's real Head is
// written but the pending-store status update for it never lands (process
// died between SaveHead and AppendStatus). A retried finalize call — as a
// recovered process would issue — must converge safely: the already-written
// feature is not re-written incorrectly, and every other staged feature for
// the same run still finalizes. CP-55 spec 3.17's "pending finalization
// interrupted between SaveHead and pending-status update must converge
// safely on retry."
func TestRestartDuringTerminalFinalizationConverges(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	// Simulate the crash: the real Head got written directly (as finalize's
	// own first phase would have done), but the pending-store status update
	// never landed — GetPending would still report it "pending".
	if err := changecontract.SaveHead(dir, changecontract.CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "fix rounding"}); err != nil {
		t.Fatal(err)
	}

	// Retry: applyFlowControl(done) re-invokes finalizePendingCanonicalHeadsForRun.
	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done) retry after simulated crash: %v", err)
	}
	finalHead, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("expected Canonical Head to still exist after the retried finalize, found=%v err=%v", found, err)
	}
	if finalHead.BehaviorStatement != "fix rounding" {
		t.Fatalf("BehaviorStatement = %q, want fix rounding", finalHead.BehaviorStatement)
	}
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "done" {
		t.Fatalf("loop status = %q, want done after the retried finalize converges", got)
	}
}

