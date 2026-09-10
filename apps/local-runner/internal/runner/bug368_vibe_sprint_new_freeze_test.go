package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/workingmode"
)

const sprint2PlannerDraft = `{"feature_key":"snake-mvp","intent":"Add tick loop and WASD input under snake/","declared_paths":["snake/loop.go","snake/input.go","snake/loop_test.go"],"source_doc_id":"Task-911"}`

func TestBUG368_AbandonLetsNextSprintFreezeNewPaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("sprint-1 freeze")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := store.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("sprint-1 active missing ok=%v err=%v", ok, err)
	}
	if len(first.DeclaredPaths) != 1 || first.DeclaredPaths[0] != "src/calc.go" {
		t.Fatalf("sprint-1 paths=%v", first.DeclaredPaths)
	}

	abandonActiveFrozenContractsForRun(dir, runID, "vibe-sprint next task")

	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = reopened.GetFrozenForStep(runID, "coder"); ok {
		t.Fatal("after abandon, sprint-1 contract must not stay active")
	}

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, sprint2PlannerDraft) {
		t.Fatal("sprint-2 freeze")
	}
	after, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, ok, err := after.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("sprint-2 active missing ok=%v err=%v", ok, err)
	}
	if second.ContractID == first.ContractID {
		t.Fatal("sprint-2 must mint a new contract, not reuse sprint-1")
	}
	joined := strings.Join(second.DeclaredPaths, ",")
	if !strings.Contains(joined, "snake/loop.go") || !strings.Contains(joined, "snake/input.go") {
		t.Fatalf("sprint-2 paths=%v want tick/input files", second.DeclaredPaths)
	}
	versions, err := after.ListVersionsForStep(runID, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) < 2 {
		t.Fatalf("want both sprint versions on disk, got %d", len(versions))
	}
}

func TestBUG368_WithoutAbandonDifferentDraftStillReuses(t *testing.T) {
	// Pins the recovery contract: freeze re-delivery with a different draft
	// must NOT mint a new version unless the previous contract was abandoned
	// (TestRecoveredFlowReusesPersistedFrozenContract). This file does not
	// edit that old test.
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("first freeze")
	}
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, sprint2PlannerDraft) {
		t.Fatal("re-delivery")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListVersionsForStep(runID, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("recovery re-delivery must still reuse, got %d versions", len(versions))
	}
}

func TestBUG368_MaybeStartNextSprintAbandonsPreviousFreeze(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, _ := newTestServer(t)
	parent, startErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui", Cwd: dir,
	})
	if startErr != nil {
		t.Fatal(startErr)
	}
	pre, storeErr := changecontract.NewFrozenStore(dir)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "snake-mvp", Intent: "core model", DeclaredPaths: []string{"src/calc.go"},
	}
	rec, err := changecontract.FreezeContract(dir, parent.RunID, "freeze", "coder", draft, head, nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := pre.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workspaceCwd = dir
	rs.workingMode = workingmode.Vibe
	rs.vibeTaskPlan = []string{
		"requirements/08-Task/todo/Task-910.md",
		"requirements/08-Task/todo/Task-911.md",
	}
	rs.vibeSprintIndex = 1
	rs.flowEngineDriven = true
	svc.mu.Unlock()

	svc.maybeStartNextVibeSprint(parent.RunID)

	store, storeErr2 := changecontract.NewFrozenStore(dir)
	if storeErr2 != nil {
		t.Fatal(storeErr2)
	}
	if _, ok, _ := store.GetFrozenForStep(parent.RunID, "coder"); ok {
		t.Fatal("starting sprint 2 must abandon sprint-1 freeze so the new planner can freeze Task-911 paths")
	}
}

func TestBUG368_ContinueBoundaryAbandonsPreviousFreeze(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, []string{
		"requirements/08-Task/todo/Task-910.md",
		"requirements/08-Task/todo/Task-911.md",
	}, 1)
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = dir
	svc.mu.Unlock()

	pre, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "snake-mvp", Intent: "core model", DeclaredPaths: []string{"src/calc.go"},
	}
	rec, err := changecontract.FreezeContract(dir, runID, "freeze", "coder", draft, head, nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := pre.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("want boundary park")
	}
	if !svc.continueVibeSprintBoundary(runID, "") {
		t.Fatal("Continue must start sprint 2")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.GetFrozenForStep(runID, "coder"); ok {
		t.Fatal("Continue to sprint 2 must abandon sprint-1 freeze")
	}
}
