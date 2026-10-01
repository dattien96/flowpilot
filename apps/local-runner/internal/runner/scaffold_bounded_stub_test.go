package runner

// CA-1094 (bounded stubs): at contract.freeze a declared production path that
// ALREADY exists on disk is read-only for the scaffold (tdd) writer — the
// scaffold may CREATE declared paths that do not exist yet (whitelist stubs),
// but may never modify a file that existed at freeze time. Live run-3362:
// the scaffold rewrote settings.gradle.kts / libs.versions.toml — all
// committed before freeze — while its frozen record carried the full
// declared scope with no existing-vs-new split.

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// scaffoldChainFixture is freeze -> tdd (agent.scaffold) with coder
// (agent.code) bound as the sibling writer — the vibe-sprint writer pair.
func scaffoldChainFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "tdd", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

// The scaffold's frozen record must split declared paths by existence at
// freeze: pre-existing production files land in ReadOnlyPaths; not-yet-
// existing paths stay create-only targets. The coder sibling keeps an empty
// lock — it is the writer that later fills the bodies.
func TestScaffoldBoundedStubs_FreezeSplitsExistingVsNew(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	// Written before the run is created so it lands in the flow-start
	// fingerprint (pre-existing dirt), not a planner mutation.
	writeRepoFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	svc := newFreezeTestService(t)
	edges, nodes := scaffoldChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	draft := `{"feature_key":"app","intent":"add screen","declared_paths":["existing.go","new_stub.go"]}`
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, draft) {
		t.Fatal("runContractFreezeNode returned false")
	}

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	scaffoldRec, ok, err := store.GetFrozenForStep(runID, "tdd")
	if err != nil || !ok {
		t.Fatalf("frozen record for tdd missing: ok=%v err=%v", ok, err)
	}
	if !changecontract.IsReadOnlyLockedPath(scaffoldRec, "existing.go") {
		t.Fatalf("pre-existing declared path must be read-only for the scaffold; ReadOnlyPaths=%v", scaffoldRec.ReadOnlyPaths)
	}
	if changecontract.IsReadOnlyLockedPath(scaffoldRec, "new_stub.go") {
		t.Fatal("a declared path that does not exist at freeze must NOT be locked — the scaffold creates it as a stub")
	}

	coderRec, ok, err := store.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("frozen record for coder missing: ok=%v err=%v", ok, err)
	}
	if changecontract.IsReadOnlyLockedPath(coderRec, "existing.go") {
		t.Fatal("the coder must stay free to modify pre-existing files — it fills the bodies")
	}
}

// The approval bridge must silent-deny the scaffold child's write/exec on a
// freeze-locked path, while leaving the same file writable for the coder and
// new declared files writable for the scaffold.
func TestScaffoldBoundedStubs_BridgeDeniesPreExistingWrites(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	freezeP4Contract(t, dir, parent.RunID, "tdd", head, []string{"existing.go", "new_stub.go"})
	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"existing.go", "new_stub.go"})
	if _, err := changecontract.LockReproduceTestPaths(dir, parent.RunID, "tdd", []string{"existing.go"}, time.Now().UTC()); err != nil {
		t.Fatalf("lock pre-existing path on scaffold record: %v", err)
	}

	scaffold := newReproduceChildRun(svc, "child-tdd", parent.RunID, dir, head, "tdd")
	if d, r, handled := svc.decideScaffoldPreExistingLock(scaffold, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Write"}); !handled || d != "deny" {
		t.Fatalf("scaffold write to pre-existing declared file must be denied, got handled=%v d=%q r=%q", handled, d, r)
	}
	if _, _, handled := svc.decideScaffoldPreExistingLock(scaffold, ApprovalDetails{Kind: "file", Command: "new_stub.go", Reason: "Write"}); handled {
		t.Fatal("a declared path that did not exist at freeze must stay writable for the scaffold")
	}
	// Reads stay allowed — the scaffold must read the file it integrates with.
	if _, _, handled := svc.decideScaffoldPreExistingLock(scaffold, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Read"}); handled {
		t.Fatal("reading a locked pre-existing file must stay allowed")
	}
	// Shell bypass on the locked path is denied too.
	if _, _, handled := svc.decideScaffoldPreExistingLock(scaffold, ApprovalDetails{Kind: "exec", Command: "cat > existing.go <<EOF\npackage app\nEOF", Reason: "Bash"}); !handled {
		t.Fatal("an exec mutation naming the locked file must be denied")
	}

	coder := newReproduceChildRun(svc, "child-coder", parent.RunID, dir, head, "coder")
	if _, _, handled := svc.decideScaffoldPreExistingLock(coder, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Write"}); handled {
		t.Fatal("the bounded-stub lock is scoped to the scaffold writer only — the coder fills the same files")
	}
}
