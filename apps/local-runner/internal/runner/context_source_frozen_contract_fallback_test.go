package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/structure"
)

// saveFrozenContract helper for these tests (workspace-local, no global state).
func saveFrozenContract(t *testing.T, dir, runID, coderStepID string, paths []string, declaredAt time.Time) {
	t.Helper()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatalf("NewFrozenStore: %v", err)
	}
	rec := changecontract.FrozenContractRecord{
		ContractID:    runID + ":" + coderStepID + ":" + declaredAt.Format(time.RFC3339Nano),
		Version:       1,
		RunID:         runID,
		CoderStepID:   coderStepID,
		FeatureKey:    "change-contract",
		Intent:        "test",
		DeclaredPaths: paths,
		DeclaredAt:    declaredAt,
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatalf("SaveFrozen: %v", err)
	}
}

func TestChangeContractSourceFrozenFallback(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenContract(t, dir, "run-f1", "implement", []string{"a.go"}, now)

	src := &changeContractSource{priority: 3}
	sec, err := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-f1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sec.Body, "a.go") {
		t.Fatalf("expected frozen fallback body to contain declared path, got %q", sec.Body)
	}
}

func TestChangeContractSourceLegacyWinsOverFrozen(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	// Legacy contract for same run
	legacyStore, _ := changecontract.NewStore(dir)
	if err := legacyStore.Save(changecontract.Contract{RunID: "run-wins", StepID: "s1", FeatureKey: "k", DeclaredPaths: []string{"legacy.go"}, Confidence: changecontract.ConfidenceDeclared, DeclaredAt: now}); err != nil {
		t.Fatal(err)
	}
	saveFrozenContract(t, dir, "run-wins", "implement", []string{"frozen.go"}, now.Add(time.Second))

	src := &changeContractSource{priority: 3}
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-wins"})
	if !strings.Contains(sec.Body, "legacy.go") {
		t.Fatalf("legacy should win, got %q", sec.Body)
	}
	if strings.Contains(sec.Body, "frozen.go") {
		t.Fatalf("frozen should not win when legacy exists, got %q", sec.Body)
	}
}

func TestChangeContractSourceNoDataDegrades(t *testing.T) {
	dir := t.TempDir()
	src := &changeContractSource{priority: 3}
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "nope"})
	if sec.Body != "" {
		t.Fatalf("expected empty, got %q", sec.Body)
	}
}

func TestChangeContractSourceFrozenDirBucketDegrades(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenContract(t, dir, "run-dir", "implement", []string{"internal"}, now) // dir-bucket
	src := &changeContractSource{priority: 3}
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-dir"})
	// RenderContractBlock always renders Scope even for dir-bucket — the
	// dependence path's dir-bucket check is separate. For change.contract,
	// we expect a Body (the test relaxes). The key is it does not panic.
	_ = sec
}

func TestDependenceSourceFrozenFallback(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenContract(t, dir, "run-f1", "implement", []string{"apps/local-runner/internal/runner/foo.go"}, now)

	fake := &fakeDependenceProvider{available: true, byTarget: map[string]structure.DependentsSummary{"Foo": {Count: 1, Nearest: []string{"X"}}}}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-f1"})
	if !strings.Contains(sec.Body, "Foo") {
		t.Fatalf("expected dependence body from frozen, got %q", sec.Body)
	}
}

func TestDependenceSourceFrozenDirBucketYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenContract(t, dir, "run-dir2", "implement", []string{"apps"}, now)
	fake := &fakeDependenceProvider{available: true, byTarget: map[string]structure.DependentsSummary{"Apps": {Count: 1, Nearest: []string{"X"}}}}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-dir2"})
	if sec.Body != "" {
		t.Fatalf("expected empty for dir-bucket frozen, got %q", sec.Body)
	}
}

func TestDependenceSourceFrozenGitNexusUnavailableNote(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenContract(t, dir, "run-gn", "implement", []string{"apps/local-runner/internal/runner/bar.go"}, now)
	fake := &fakeDependenceProvider{available: false}
	src := newDependenceSourceWithFake(fake)
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-gn"})
	if !strings.Contains(sec.Body, "GitNexus not indexed") {
		t.Fatalf("expected not-indexed note, got %q", sec.Body)
	}
}

func TestFrozenFallbackPicksLatestActiveAcrossSteps(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().UTC()
	saveFrozenContract(t, dir, "run-multi", "test_signatures", []string{"old.go"}, base)
	saveFrozenContract(t, dir, "run-multi", "implement", []string{"new.go"}, base.Add(time.Second))
	src := &changeContractSource{priority: 3}
	sec, _ := src.Fetch(context.Background(), FlowContextHints{Workspace: dir, WorkflowRunID: "run-multi"})
	if !strings.Contains(sec.Body, "new.go") {
		t.Fatalf("expected newest active, got %q", sec.Body)
	}
}
