package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// run-151954 (CP-43 F2 live, gate-sandbox): contract.freeze's planner-mutation
// guard false flagged skillpack-installed project scaffolding written MID-flow —
// the gitnexus skill pack wrote 7 .claude/skills/gitnexus/*/SKILL.md files plus
// AGENTS.md, CLAUDE.md and .gitignore at 06:58:50, 17s after the flow-start
// baseline was captured (06:58:33) and 13s before the freeze check (06:59:03).
// The runner logged "planner changed 9 file(s) ... the contract planner must be
// read-only" and parked preflight_contract_freeze at WAITING_USER_APPROVAL even
// though the planner never wrote anything. The guard must only ever attribute
// PROJECT files to the planner, never tool-owned skill/scaffold surfaces.
//
// Additive file only — the legacy flow_contract_freeze_test.go suite and
// run243681_freeze_planner_metadata_exclusion_test.go are untouched.
// Provider-agnostic (no ProviderKey branch in the freeze dispatch).

func TestIsFlowPlannerExcludedPathCoversSkillpackScaffold(t *testing.T) {
	excluded := []string{
		// Runner/tool-owned state already excluded before CA-645.
		".flowpilot", ".flowpilot/ledger/chat_summary.ndjson", ".flowpilot/manifest.json",
		".gitnexus", ".gitnexus/meta.json",
		// Skillpack/scaffold surfaces (CA-645) — the run-151954 false positive.
		".claude", ".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".claude/skills/gitnexus/gitnexus-debugging/SKILL.md",
		".agents", ".agents/skills/flow-mode-orchestrator/SKILL.md",
		".grok", ".grok/skills/x/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	}
	for _, p := range excluded {
		if !isFlowPlannerExcludedPath(p) {
			t.Errorf("isFlowPlannerExcludedPath(%q) = false, want true (tool-owned surface)", p)
		}
	}
	notExcluded := []string{
		"src/calc.go", "calc.go", "calc_test.go", "user_test.go",
		// docs/report.go — not .md: e39d261d intentionally excludes all *.md
		// (plan Task md lands mid-flow legitimately); a code file under docs/
		// is still a real project file the planner must not touch.
		"planner_touched.go", "docs/report.go", "format.go",
	}
	for _, p := range notExcluded {
		if isFlowPlannerExcludedPath(p) {
			t.Errorf("isFlowPlannerExcludedPath(%q) = true, want false (real project file)", p)
		}
	}
}

func TestWorktreeMutatedSincePathsIgnoresSkillpackScaffold(t *testing.T) {
	base := map[string]string{"src/calc.go": "a"}
	cur := map[string]string{
		"src/calc.go": "a",
		// Mid-flow skillpack install — must be invisible to the planner guard.
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md":   "s1",
		".claude/skills/gitnexus/gitnexus-guide/SKILL.md": "s2",
		"AGENTS.md":  "a1",
		"CLAUDE.md":  "a2",
		".gitignore": "i1",
	}
	if got := worktreeMutatedSincePaths(base, cur); len(got) != 0 {
		t.Fatalf("skillpack scaffold churn must be ignored, got %v", got)
	}
}

func TestWorktreeMutatedSincePathsStillCatchesProjectFileAlongsideScaffold(t *testing.T) {
	base := map[string]string{"src/calc.go": "a"}
	cur := map[string]string{
		"src/calc.go": "b", // real project change — must still be reported
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md": "s1",
		"AGENTS.md": "a1",
	}
	got := worktreeMutatedSincePaths(base, cur)
	if len(got) != 1 || got[0] != "src/calc.go" {
		t.Fatalf("real project change must be reported alongside ignored scaffold, got %v", got)
	}
}

func TestBaselineWorktreeFingerprintExcludesSkillpackScaffold(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	for _, p := range []string{
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".agents/skills/flow-mode-orchestrator/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := baselineWorktreeFingerprint(dir)
	for _, excluded := range []string{
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".agents/skills/flow-mode-orchestrator/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	} {
		if _, ok := fp[excluded]; ok {
			t.Fatalf("baseline must not fingerprint tool-owned scaffold path %q", excluded)
		}
	}
	if _, ok := fp["real.go"]; !ok {
		t.Fatalf("baseline must fingerprint the real project file real.go, got %v", fp)
	}
}

// TestRun151954_FreezeIgnoresSkillpackScaffoldMidFlowWrites is the exact
// run-151954 repro: skill files do NOT exist at flow start (baseline capture),
// then the skillpack installs them during the planner turn. The freeze must
// succeed and the coder must spawn.
func TestRun151954_FreezeIgnoresSkillpackScaffoldMidFlowWrites(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head) // baseline captured here
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// Skillpack install lands between flow start and freeze (06:58:50 repro).
	for _, p := range []string{
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".claude/skills/gitnexus/gitnexus-debugging/SKILL.md",
		".claude/skills/gitnexus/gitnexus-exploring/SKILL.md",
		".claude/skills/gitnexus/gitnexus-guide/SKILL.md",
		".claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md",
		".claude/skills/gitnexus/gitnexus-refactoring/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("freeze must succeed when only skillpack scaffold files appeared mid-flow")
	}
	waitLoop(t, "coder spawned despite skillpack scaffold writes", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

// TestRun151954_FreezeStillCatchesPlannerProjectFileMutation keeps the guard
// intact: a real .go file appearing after flow start must still hard-block.
func TestRun151954_FreezeStillCatchesPlannerProjectFileMutation(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

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
