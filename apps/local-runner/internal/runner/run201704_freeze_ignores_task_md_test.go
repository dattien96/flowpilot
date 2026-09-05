package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// run-201704 (CP-58 S6 / task-harness): the flow reached preflight_contract_freeze
// after plan_writer wrote requirements/08-Task/todo/Task-951-*.md, and the freeze
// step parked WAITING_USER_APPROVAL with "planner changed 1 file(s)
// (requirements/08-Task/todo/Task-951-...md); the contract planner must be
// read-only". That Task md is the plan phase's own legitimate artifact —
// task-harness freezes AFTER plan_writer, unlike rag-harness where the planner
// feeds freeze directly — never the read-only planner touching code. The
// planner-mutation guard must ignore doc/audit surfaces (requirements/**,
// change-audit/**, *.md), exactly like the gate scope path already does
// (flowgate.IsDocOrAuditFile), while a planner-written *.go still blocks.
//
// New file; no pre-existing test is modified. Provider-agnostic: the guard
// takes no providerKey, so the matrix guards future provider drift.

func TestRun201704_FreezeIgnoresTaskMdWrittenAfterFlowStart(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc := newFreezeTestServiceForProvider(t, pk)
			edges, nodes := freezeChainFixture()
			runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head) // baseline captured here
			freezeNode, _ := findFlowNode(nodes, "freeze")

			// plan_writer writes the Task md between flow start and freeze.
			taskMd := filepath.Join(dir, "requirements", "08-Task", "todo", "Task-951-gcd-provider-agnostic-euclidean.md")
			if err := os.MkdirAll(filepath.Dir(taskMd), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(taskMd, []byte("# Task-951\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
				t.Fatalf("%s: freeze must succeed when only the plan Task md was written", pk)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
				t.Fatalf("%s: loop must not block on the Task md: %s", pk, st.GateReason)
			}
			waitLoop(t, string(pk)+": coder spawned despite Task md", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "coder") == 1
			})
		})
	}
}

func TestRun201704_FreezeStillCatchesPlannerCodeMutation(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc := newFreezeTestServiceForProvider(t, pk)
			edges, nodes := freezeChainFixture()
			runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
			freezeNode, _ := findFlowNode(nodes, "freeze")

			// A real code file mutated after flow start must still be caught —
			// the doc exclusion must not weaken the guard for genuine planner
			// code mutations (CA-426 contract).
			if err := os.WriteFile(filepath.Join(dir, "planner_touched.go"), []byte("package main\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
				t.Fatalf("%s: expected true (escalated) for a real code mutation", pk)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" {
				t.Fatalf("%s: loop = %q, want blocked (escalate)", pk, st.Status)
			}
			if got := countChildrenWithLabel(svc, runID, "coder"); got != 0 {
				t.Fatalf("%s: writer must not spawn after a real planner mutation, got %d children", pk, got)
			}
		})
	}
}

func TestRun201704_WorktreeMutatedSincePathsIgnoresDocs(t *testing.T) {
	base := map[string]string{
		"src/calc.go": "a",
		"requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md": "h1",
		"change-audit/CA-999.md": "c1",
		"README.md":              "r1",
	}
	cur := map[string]string{
		"src/calc.go": "a",
		"requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md": "h2",
		"change-audit/CA-999.md": "c2",
		"README.md":              "r2",
	}
	if got := worktreeMutatedSincePaths(base, cur); len(got) != 0 {
		t.Fatalf("doc/audit churn must be ignored, got %v", got)
	}
}

func TestRun201704_WorktreeMutatedSincePathsStillCatchesCode(t *testing.T) {
	base := map[string]string{
		"src/calc.go": "a",
		"requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md": "h1",
	}
	cur := map[string]string{
		"src/calc.go": "b",
		"requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md": "h2",
	}
	got := worktreeMutatedSincePaths(base, cur)
	if len(got) != 1 || got[0] != "src/calc.go" {
		t.Fatalf("a real code change must be reported alone, got %v", got)
	}
}

func TestRun201704_BaselineFingerprintExcludesTaskMd(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	taskMd := filepath.Join(dir, "requirements", "08-Task", "todo", "Task-951-gcd-provider-agnostic-euclidean.md")
	if err := os.MkdirAll(filepath.Dir(taskMd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskMd, []byte("# Task-951\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := baselineWorktreeFingerprint(dir)
	for k := range fp {
		if k == "requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md" {
			t.Fatalf("baseline must skip the Task md, got %v", fp)
		}
	}
	if _, ok := fp["real.go"]; !ok {
		t.Fatalf("baseline must still capture real code files, got %v", fp)
	}
}
