package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// newContractFreezeTestRepo returns a workspace with one commit, so
// captureGitHead/changedFilesSince have a real HEAD to diff against.
func newContractFreezeTestRepo(t *testing.T) (dir, headSHA string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "seed.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "seed")
	headSHA, err := captureGitHead(dir)
	if err != nil || headSHA == "" {
		t.Fatalf("captureGitHead: %q, %v", headSHA, err)
	}
	return dir, headSHA
}

// freezeChainFixture returns freeze -> coder (agent.code), no intermediate hop.
func freezeChainFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

// freezeChainWithContextFixture returns freeze -> context.produce -> coder.
func freezeChainWithContextFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "context", When: "done", Kind: "forward"},
		{From: "context", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

const validPlannerDraft = `{"feature_key":"calc-core","intent":"fix rounding","declared_paths":["src/calc.go"]}`

func newFreezeTestService(t *testing.T) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coded"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func newFreezeTestRun(t *testing.T, svc *InteractiveService, workspace string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, headSHA string) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = workspace
	rs.flowStartGitHead = headSHA
	// Mirrors startResolvedFlow's real capture (flow_executor.go): the
	// dirty-file baseline as it stands at "flow start," i.e. right now, before
	// the test's own planner-completion call — any pre-existing dirty file at
	// this point must NOT later read as a planner mutation.
	rs.flowStartWorktreeFingerprint = baselineWorktreeFingerprint(workspace)
	svc.mu.Unlock()
	return parent.RunID
}

// --- runContractFreezeNode: success path -------------------------------------

func TestRunContractFreezeNodePersistsBeforeCoderSpawn(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected runContractFreezeNode to return true (handled)")
	}

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("expected a frozen record for (runID, coder): ok=%v err=%v", ok, err)
	}
	if rec.FeatureKey != "calc-core" {
		t.Fatalf("FeatureKey = %q, want calc-core", rec.FeatureKey)
	}

	waitLoop(t, "coder spawned", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

func TestRunContractFreezeNodeBindsContractToCoderStep(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft)

	store, _ := changecontract.NewFrozenStore(dir)
	rec, ok, _ := store.GetFrozenForStep(runID, "coder")
	if !ok || rec.CoderStepID != "coder" {
		t.Fatalf("CoderStepID = %q, ok=%v, want coder/true", rec.CoderStepID, ok)
	}
	if rec.PlannerStepID != "freeze" {
		t.Fatalf("PlannerStepID = %q, want freeze", rec.PlannerStepID)
	}
}

func TestRunContractFreezeNodeRecordsBaselineSHA(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft)

	store, _ := changecontract.NewFrozenStore(dir)
	rec, ok, _ := store.GetFrozenForStep(runID, "coder")
	if !ok || rec.BaseSHA != head {
		t.Fatalf("BaseSHA = %q, want flow-start HEAD %q (ok=%v)", rec.BaseSHA, head, ok)
	}
}

func TestRunContractFreezeNodeReloadsDurableRecordBeforeAdvance(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft)

	// A brand-new store instance (not the one the function used internally)
	// must independently see the record — proves it hit disk, not just an
	// in-memory cache.
	fresh, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := fresh.GetFrozenForStep(runID, "coder"); !ok {
		t.Fatal("a fresh FrozenStore instance must see the persisted record")
	}
}

// --- runContractFreezeNode: chain advancement --------------------------------

func TestRunContractFreezeNodeAdvancesToContextProduce(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainWithContextFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (handled)")
	}
	waitLoop(t, "coder spawned past context.produce hop", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

func TestInlineChainAdvancesFreezeThenContextThenWriter(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainWithContextFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// Drive it through the same switch tryAdvanceFlowFromNode/tryAdvanceFlowThroughInline
	// use, not runContractFreezeNode directly — proves the wiring at the switch level.
	if !svc.tryAdvanceFlowThroughInline(runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected tryAdvanceFlowThroughInline to handle contract.freeze")
	}
	waitLoop(t, "coder spawned via tryAdvanceFlowThroughInline", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

// --- resolveFreezeWriterTarget: bounded chain, pure logic --------------------

func TestInlineChainStopsAtHopLimit(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "h1", When: "done", Kind: "forward"},
		{From: "h1", To: "h2", When: "done", Kind: "forward"},
		{From: "h2", To: "h3", When: "done", Kind: "forward"},
		{From: "h3", To: "h4", When: "done", Kind: "forward"},
		{From: "h4", To: "h5", When: "done", Kind: "forward"},
		{From: "h5", To: "h6", When: "done", Kind: "forward"},
		{From: "h6", To: "h7", When: "done", Kind: "forward"},
		{From: "h7", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "h1", Behavior: "context.produce"}, {ID: "h2", Behavior: "context.produce"},
		{ID: "h3", Behavior: "context.produce"}, {ID: "h4", Behavior: "context.produce"},
		{ID: "h5", Behavior: "context.produce"}, {ID: "h6", Behavior: "context.produce"},
		{ID: "h7", Behavior: "context.produce"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit); ok {
		t.Fatal("a chain longer than the hop limit must fail closed (ok=false)")
	}
}

// freezeChainWithNIntermediates builds freeze -> h1 -> ... -> hN -> coder, so
// the exact hop-limit boundary can be tested precisely: each loop iteration
// in resolveFreezeWriterTarget consumes one edge, including the final edge
// into the writer, so N intermediates require exactly N+1 iterations.
func freezeChainWithNIntermediates(n int) ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	var edges []agentpack.FlowEdge
	nodes := []agentpack.FlowNode{{ID: "freeze", Behavior: "contract.freeze"}}
	prev := "freeze"
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("h%d", i)
		edges = append(edges, agentpack.FlowEdge{From: prev, To: id, When: "done", Kind: "forward"})
		nodes = append(nodes, agentpack.FlowNode{ID: id, Behavior: "context.produce"})
		prev = id
	}
	edges = append(edges, agentpack.FlowEdge{From: prev, To: "coder", When: "done", Kind: "forward"})
	nodes = append(nodes, agentpack.FlowNode{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"})
	return edges, nodes
}

func TestInlineChainAcceptsExactlyAtHopLimitBoundary(t *testing.T) {
	// flowInlineChainHopLimit-1 intermediates require exactly
	// flowInlineChainHopLimit iterations — the last one admitted.
	edges, nodes := freezeChainWithNIntermediates(flowInlineChainHopLimit - 1)
	writer, path, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit)
	if !ok {
		t.Fatalf("%d intermediates (exactly at the boundary) must be accepted", flowInlineChainHopLimit-1)
	}
	if writer.ID != "coder" || len(path) != flowInlineChainHopLimit-1 {
		t.Fatalf("writer=%q pathLen=%d, want coder/%d", writer.ID, len(path), flowInlineChainHopLimit-1)
	}
}

func TestInlineChainRejectsOneMoreThanHopLimitBoundary(t *testing.T) {
	// flowInlineChainHopLimit intermediates require flowInlineChainHopLimit+1
	// iterations — one past what's admitted.
	edges, nodes := freezeChainWithNIntermediates(flowInlineChainHopLimit)
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit); ok {
		t.Fatalf("%d intermediates (one past the boundary) must be rejected", flowInlineChainHopLimit)
	}
}

func TestInlineChainRejectsNonContextProduceIntermediateHop(t *testing.T) {
	// CA-426 C-3: an inline behavior other than context.produce reached
	// mid-chain (e.g. command.validate) must not be silently skipped/admitted
	// — it would otherwise be marked Done without ever actually running.
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit); ok {
		t.Fatal("a non-context.produce inline intermediate must be rejected, not silently skipped")
	}
}

func TestInlineChainRejectsNonAgentCodeTerminal(t *testing.T) {
	// CA-426 M-5: the terminal target must be agent.code specifically, not
	// any non-inline node (e.g. a plain agent.delegate reviewer).
	edges := []agentpack.FlowEdge{{From: "freeze", To: "reviewer", When: "done", Kind: "forward"}}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
	}
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit); ok {
		t.Fatal("a non-agent.code terminal target must be rejected")
	}
}

func TestInlineChainDetectsCycle(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "a", When: "done", Kind: "forward"},
		{From: "a", To: "b", When: "done", Kind: "forward"},
		{From: "b", To: "a", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "a", Behavior: "context.produce"},
		{ID: "b", Behavior: "context.produce"},
	}
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "freeze", flowInlineChainHopLimit); ok {
		t.Fatal("a cycle among inline nodes must fail closed (ok=false)")
	}
}

func TestInlineChainDoesNotDispatchWriterAfterFreezeFailure(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, "not valid json at all") {
		t.Fatal("expected true (escalated/handled)")
	}
	// Give any errant async spawn a moment to (not) happen.
	time.Sleep(50 * time.Millisecond)
	if got := countChildrenWithLabel(svc, runID, "coder"); got != 0 {
		t.Fatalf("writer must not be spawned after a freeze failure, got %d children", got)
	}
	if store, err := changecontract.NewFrozenStore(dir); err == nil {
		if _, ok, _ := store.GetFrozenForStep(runID, "coder"); ok {
			t.Fatal("no frozen record should exist after an invalid-draft failure")
		}
	}
}

// --- runContractFreezeNode: rejection paths ----------------------------------

func TestRunContractFreezeNodeRejectsInvalidDraft(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, `{"feature_key":"x"}`) {
		t.Fatal("expected true (escalated) for a draft missing intent/paths")
	}
	if got := svc.agentOrchestrator.loopStateFor(runID).Status; got != "blocked" {
		t.Fatalf("loop status = %q, want blocked (escalate)", got)
	}
}

func TestRunContractFreezeNodeRejectsPlannerChangedFiles(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head) // captures the (clean) flow-start baseline
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// The mutation happens AFTER flow start (i.e. during the planner's own
	// turn) — this is what must be caught, as distinct from a file that was
	// already dirty before the flow began (see the companion test below).
	if err := os.WriteFile(filepath.Join(dir, "planner_touched.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (escalated) when the planner changed files")
	}
	if store, err := changecontract.NewFrozenStore(dir); err == nil {
		if _, ok, _ := store.GetFrozenForStep(runID, "coder"); ok {
			t.Fatal("no frozen record should exist when the planner mutated the workspace")
		}
	}
}

// TestRunContractFreezeNodeAllowsPreExistingDirtyWorktree is the direct
// regression test for CA-426 C-1: a file that was already uncommitted BEFORE
// the flow started (the normal state of a real developer's workspace) must
// not be misread as a planner mutation just because the tree is dirty.
func TestRunContractFreezeNodeAllowsPreExistingDirtyWorktree(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	// Dirty BEFORE the flow starts / baseline is captured.
	if err := os.WriteFile(filepath.Join(dir, "already_dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head) // baseline now includes already_dirty.go
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (handled)")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.GetFrozenForStep(runID, "coder"); !ok {
		t.Fatal("a pre-existing dirty file (present before flow start) must not block the freeze")
	}
}

func TestRunContractFreezeNodeRejectsUnknownCoderTarget(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	// freeze has no outgoing edge at all — a dead end.
	nodes := []agentpack.FlowNode{{ID: "freeze", Behavior: "contract.freeze"}}
	runID := newFreezeTestRun(t, svc, dir, nil, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, nil, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (escalated) for an unresolvable writer target")
	}
	if _, err := os.Stat(filepath.Join(dir, ".flowpilot", "contracts", "frozen_contracts.ndjson")); !os.IsNotExist(err) {
		t.Fatalf("no frozen_contracts.ndjson should be created when no writer target resolves (stat err=%v)", err)
	}
}

// --- runContractFreezeNode: recovery / duplicate delivery --------------------

func TestRecoveredFlowReusesPersistedFrozenContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// A prior process (before a restart) already froze this exact
	// (runID, coderStepID) — pre-seed the store directly, simulating recovery
	// finding a durable record that already exists for this run.
	pre, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "original intent", DeclaredPaths: []string{"src/calc.go"}}
	rec, err := changecontract.FreezeContract(dir, runID, "freeze", "coder", draft, head, nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := pre.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	// The recovered flow re-delivers the freeze node completion with a
	// DIFFERENT planner draft (e.g. the planner ran again post-restart) — the
	// already-frozen version must win, not the new draft.
	changedDraft := `{"feature_key":"calc-core","intent":"a completely different intent","declared_paths":["src/other.go"]}`
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, changedDraft) {
		t.Fatal("expected true (handled)")
	}

	// Read back via a FRESH store instance, not `pre` (opened before this
	// call): FrozenStore loads its in-memory maps once at construction and
	// never re-reads disk, so asserting against `pre` here would pass
	// regardless of whether runContractFreezeNode actually reused the
	// existing record or minted a second one — `pre`'s cached view predates
	// the call under test either way.
	verify, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := verify.ListVersionsForStep(runID, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected exactly 1 version (reused, not re-frozen), got %d", len(versions))
	}
	if versions[0].Intent != "original intent" {
		t.Fatalf("Intent = %q, want the original frozen intent to have been reused, not the new draft", versions[0].Intent)
	}
}

func TestDuplicateFreezeDeliveryReturnsExistingContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("first call: expected true")
	}
	waitLoop(t, "first coder spawn", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})

	afterFirst, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, ok, _ := afterFirst.GetFrozenForStep(runID, "coder")
	if !ok {
		t.Fatal("expected a frozen record after the first call")
	}

	// Duplicate delivery of the SAME freeze node completion (e.g. an
	// at-least-once retry after a crash/reconnect).
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("second (duplicate) call: expected true")
	}

	// A store instance opened AFTER both calls, not `afterFirst` (opened
	// before the second call and never re-read from disk) — see the
	// recovery test above for why this distinction is load-bearing.
	afterSecond, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := afterSecond.ListVersionsForStep(runID, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("duplicate delivery must not mint a second version, got %d", len(versions))
	}
	if versions[0].ContractID != first.ContractID {
		t.Fatalf("ContractID changed across duplicate delivery: %q vs %q", versions[0].ContractID, first.ContractID)
	}
}

// --- baselineWorktreeFingerprint ----------------------------------------------

func TestBaselineWorktreeFingerprintEmptyWhenClean(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	if got := baselineWorktreeFingerprint(dir); got != nil {
		t.Fatalf("clean worktree must yield nil, got %v", got)
	}
}

func TestBaselineWorktreeFingerprintCapturesDirtyContent(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "dirty.go"), []byte("package main // dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := baselineWorktreeFingerprint(dir)
	sum, ok := got["dirty.go"]
	if !ok || sum == "" {
		t.Fatalf("expected a non-empty fingerprint entry for dirty.go, got %v", got)
	}
}
