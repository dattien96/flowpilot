package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// run-243681 (CP-43 F2 live): contract.freeze's planner-mutation guard false
// flagged FlowPilot's OWN runtime metadata — .flowpilot/ledger/chat_summary.ndjson
// and .flowpilot/manifest.json are rewritten by the runner every turn, so the
// freeze step permanently blocked with "planner changed 2 file(s) ... must be
// read-only" and the flow looped at "ask continue". The guard must only ever
// attribute PROJECT files to the planner, never runner/tool-owned state.
//
// Additive file only — the legacy flow_contract_freeze_test.go suite is
// untouched. Provider-agnostic (no ProviderKey branch in the freeze dispatch).

func TestRun243681_FreezeIgnoresFlowpilotStateChanges(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	// Runner-owned state present (and dirty) before the flow starts.
	ledger := filepath.Join(dir, ".flowpilot", "ledger")
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "chat_summary.ndjson"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".flowpilot", "manifest.json"), []byte("{\"v\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head) // baseline captured here
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// The runner rewrites its own state between flow start and freeze — exactly
	// what happens on every real turn.
	if err := os.WriteFile(filepath.Join(ledger, "chat_summary.ndjson"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".flowpilot", "manifest.json"), []byte("{\"v\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("freeze must succeed when only .flowpilot runtime state changed")
	}
	waitLoop(t, "coder spawned despite .flowpilot churn", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

func TestRun243681_FreezeStillCatchesPlannerProjectFileMutation(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// A real project file mutated after flow start must still be caught.
	if err := os.WriteFile(filepath.Join(dir, "planner_touched.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (escalated) for a real project mutation")
	}
	time.Sleep(50 * time.Millisecond)
	if got := countChildrenWithLabel(svc, runID, "coder"); got != 0 {
		t.Fatalf("writer must not spawn after a real planner mutation, got %d children", got)
	}
}

func TestWorktreeMutatedSincePathsIgnoresFlowpilotAndGitNexus(t *testing.T) {
	base := map[string]string{"src/calc.go": "a", ".flowpilot/ledger/chat_summary.ndjson": "h1", ".gitnexus/meta.json": "i1"}
	cur := map[string]string{"src/calc.go": "a", ".flowpilot/ledger/chat_summary.ndjson": "h2", ".gitnexus/meta.json": "i2"}
	if got := worktreeMutatedSincePaths(base, cur); len(got) != 0 {
		t.Fatalf("runner-owned runtime churn must be ignored, got %v", got)
	}
}

func TestWorktreeMutatedSincePathsStillCatchesProjectFile(t *testing.T) {
	base := map[string]string{"src/calc.go": "a"}
	cur := map[string]string{"src/calc.go": "b"}
	got := worktreeMutatedSincePaths(base, cur)
	if len(got) != 1 || got[0] != "src/calc.go" {
		t.Fatalf("a real project file change must be reported, got %v", got)
	}
}

func TestBaselineWorktreeFingerprintExcludesFlowpilotAndGitNexus(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	// Runner-owned + tool-owned dirty paths (untracked in a fresh repo).
	for _, p := range []string{".flowpilot/ledger/chat_summary.ndjson", ".flowpilot/manifest.json", ".gitnexus/meta.json"} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Plus a real project file that SHOULD be captured.
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := baselineWorktreeFingerprint(dir)
	for _, excluded := range []string{".flowpilot/ledger/chat_summary.ndjson", ".flowpilot/manifest.json", ".gitnexus/meta.json"} {
		if _, ok := fp[excluded]; ok {
			t.Fatalf("baseline must not fingerprint runner/tool-owned path %q", excluded)
		}
	}
	if _, ok := fp["real.go"]; !ok {
		t.Fatalf("baseline must fingerprint the real project file real.go, got %v", fp)
	}
}
