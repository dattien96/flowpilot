package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// D2 (live run-2062497): the frozen-scope drift gate went unwinnable on a
// pre-existing vendored submodule. Two latent bugs stacked:
//
//   1. baselineWorktreeFingerprint stored "" for a directory (os.ReadFile
//      fails on dirs), while worktreeFileFingerprint returns a deterministic
//      dir hash — "" never matches, so the baseline subtraction can never
//      cover the path.
//   2. For regular files the baseline stored sha256(content) (64 hex chars)
//      while the gate compares against a 16-byte truncated
//      content|size|mode hash (32 hex chars) — formats never equal, so the
//      freeze-baseline subtraction was dead code for every path type.
//
// The live sequence that exposed it: a coder leg `git commit`ed mid-flow,
// moving the staged boringssl gitlink into the committed-since-base diff.
// Committed paths are not covered by the turn-start snapshot (uncommitted
// only in live usage), so the frozen baseline was the only possible cover —
// and it could never match.

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"HOME="+dir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newNestedGitRepo creates a nested git repository inside dir/relPath —
// the shape a vendored submodule directory takes on disk (a directory that
// is itself a repository HEAD).
func newNestedGitRepo(t *testing.T, dir, relPath string) {
	t.Helper()
	sub := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, sub, "init")
	runGitForTest(t, sub, "config", "user.email", "t@e")
	runGitForTest(t, sub, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(sub, "dep.c"), []byte("int dep;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, sub, "add", "-A")
	runGitForTest(t, sub, "commit", "-m", "pin")
}

func TestBaselineFingerprint_GitlinkAndFileMatchGateFingerprint(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	newNestedGitRepo(t, dir, "third_party/dep")
	p4WriteFile(t, dir, "stale.go", "package stale\n")

	baseline := baselineWorktreeFingerprint(dir)
	if baseline == nil {
		t.Fatal("baseline must cover the dirty tree")
	}
	for _, p := range []string{"third_party/dep", "stale.go"} {
		base, ok := baseline[p]
		if !ok {
			t.Fatalf("baseline missing %q: %+v", p, baseline)
		}
		cur := worktreeFileFingerprint(dir, p)
		if base == "" {
			t.Fatalf("baseline[%q] empty — unreadable-at-freeze must not be a silent pass/fail", p)
		}
		if base != cur {
			t.Fatalf("baseline[%q]=%q vs gate fingerprint %q — same path must fingerprint identically on both sides", p, base, cur)
		}
	}
}

func TestWorktreeFileFingerprint_DirUsesGitHead(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	newNestedGitRepo(t, dir, "third_party/dep")
	want, err := exec.Command("git", "-C", filepath.Join(dir, "third_party/dep"), "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	fp := worktreeFileFingerprint(dir, "third_party/dep")
	if !strings.Contains(fp, strings.TrimSpace(string(want))) {
		t.Fatalf("dir fingerprint %q must embed the nested repo HEAD %q", fp, want)
	}
	// Deterministic across calls.
	if worktreeFileFingerprint(dir, "third_party/dep") != fp {
		t.Fatal("dir fingerprint must be deterministic")
	}
}

func TestRun2062497_CommittedPreexistingPathsDoNotDrift(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)

	// Pre-existing dirt BEFORE freeze: a vendored nested repo (gitlink shape)
	// and an untracked regular file — exactly the live boringssl + leftovers.
	newNestedGitRepo(t, dir, "third_party/dep")
	p4WriteFile(t, dir, "stale.go", "package stale\n")

	// Freeze with a real baseline over the dirty tree.
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	baseline := baselineWorktreeFingerprint(dir)
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"},
	}
	rec, err := changecontract.FreezeContract(dir, parentID, "planner", "coder", draft, head, baseline, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	if pr := svc.runs[parentID]; pr != nil {
		pr.workspaceCwd = dir
	}
	svc.mu.Unlock()

	// Live repro: the leg committed mid-flow — the gitlink + stale file move
	// into committed-since-base, where ONLY the frozen baseline can cover them.
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "leg checkpoint")

	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	rs.turnStartWorktree = nil // no snapshot → baseline is the only cover (legacy path)
	violations := make(chan ProviderEvent, 32)
	rs.subs[7] = violations

	// Writer touches only its declared file.
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go"},
	}, 0)
	// The mid-turn commit itself is a designed block (Task-242 D-7: commits
	// are reserved for the audit step). The D2 regression is when the SAME
	// turn escalates on "scope drift" for the committed pre-existing paths —
	// drift must pass first so only the commit guard fires.
	if !blocked {
		t.Fatal("a coding child that committed mid-turn must still block on the commit-reserved guard")
	}
	var msgs []string
	for {
		select {
		case ev := <-violations:
			if ev.Type == EventFlowGateViolation {
				msgs = append(msgs, ev.Error)
			}
		default:
			goto drained
		}
	}
drained:
	for _, m := range msgs {
		if strings.Contains(m, "scope drift") {
			t.Fatalf("committed pre-existing paths must not scope-drift; violations=%v", msgs)
		}
	}
	if len(msgs) == 0 || !strings.Contains(msgs[0], "git commit") {
		t.Fatalf("expected the commit-reserved violation, got %v", msgs)
	}
}

// Same repro, still dirty (uncommitted): baseline cover must work too —
// proves the fix heals the unwinnable live loop itself.
func TestRun2062497_UncommittedGitlinkDoesNotDrift(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)

	newNestedGitRepo(t, dir, "third_party/dep")
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	baseline := baselineWorktreeFingerprint(dir)
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"},
	}
	rec, err := changecontract.FreezeContract(dir, parentID, "planner", "coder", draft, head, baseline, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	if pr := svc.runs[parentID]; pr != nil {
		pr.workspaceCwd = dir
	}
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	rs.turnStartWorktree = nil
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go"},
	}, 0)
	if blocked {
		t.Fatal("unchanged vendored submodule at freeze must not scope-drift (live D2 loop)")
	}
}

// D10 (live run-2062497): an operator edited a CMake file outside the leg's
// tool calls mid-turn — the diff counts it as drift and the gate reason said
// "the leg wrote outside scope", which sent the operator hunting for a rogue
// leg write instead of amending the contract. The gate must still fail
// closed (a bash-redirect leg write is indistinguishable), but the reason
// must name which drifted paths did NOT come through the leg's tool calls
// and point at amend as the sanction path.
func TestRun2062497_ExternalWriteDriftHintsAmend(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"},
	}
	rec, err := changecontract.FreezeContract(dir, parentID, "planner", "coder", draft, head, nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	if pr := svc.runs[parentID]; pr != nil {
		pr.workspaceCwd = dir
	}
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	rs.turnStartWorktree = nil
	violations := make(chan ProviderEvent, 32)
	rs.subs[9] = violations

	// Leg writes its declared file via tool calls; an out-of-scope cpp file
	// lands in the diff WITHOUT a leg tool write (operator edit / bash
	// redirect — indistinguishable in the diff).
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "src/ops_extra.cpp", "int ops;\n")
	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go"},
	}, 0)
	if !blocked {
		t.Fatal("out-of-scope writes must still scope-drift and block")
	}
	var msgs []string
	for {
		select {
		case ev := <-violations:
			if ev.Type == EventFlowGateViolation {
				msgs = append(msgs, ev.Error)
			}
		default:
			goto drained
		}
	}
drained:
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "scope drift") {
			found = true
			if !strings.Contains(m, "src/ops_extra.cpp") {
				t.Fatalf("drift reason must name the out-of-scope path, got %q", m)
			}
			if !strings.Contains(m, "not written via this leg's tool calls") {
				t.Fatalf("drift reason must flag paths that bypassed leg tool calls (operator edits? → amend), got %q", m)
			}
		}
	}
	if !found {
		t.Fatalf("expected a scope-drift violation, got %v", msgs)
	}
}
