package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/tournament"
)

// CP-65 P-5 (Task-372): tournament end-to-end verification. New file — no
// pre-existing test is modified.
//
// The driver mirrors the tournament-harness node order
// (scout → rollout → arbiter → merge) at behavior-chain level with REAL git
// worktrees and REAL suite runs; candidate turns and LSP/dependents are the
// documented seams (mock file writes like P-5 will drive via mock turns;
// stubbed aggregated metrics per Task-372 T-1). Complements the P-3 live
// mini-round (green baseline + disqualification win) with a RED baseline +
// score win here.

// tournamentE2EBugRepo builds a real Go module repo with one genuine bug:
// Add subtracts, and TestAdd (red at baseline) proves it.
func tournamentE2EBugRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module tournamentmini\n\ngo 1.26\n")
	write("calc.go", "package tournamentmini\n\nfunc Add(a, b int) int { return a - b }\n")
	write("calc_test.go", "package tournamentmini\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"add broken\") } }\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	git("init")
	git("config", "user.email", "tournament@test.local")
	git("config", "user.name", "tournament-test")
	git("config", "core.autocrlf", "false")
	git("config", "commit.gpgsign", "false")
	git("add", ".")
	git("commit", "-m", "buggy base")
	return dir
}

// tournamentE2EGreenRepo is tournamentE2EBugRepo's twin with a GREEN
// baseline: Add is correct and TestAdd passes — the shape an empty-diff
// tournament winner exploits (BUG-459 live run-20041).
func tournamentE2EGreenRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module tournamentmini\n\ngo 1.26\n")
	write("calc.go", "package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n")
	write("calc_test.go", "package tournamentmini\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"add broken\") } }\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	git("init")
	git("config", "user.email", "tournament@test.local")
	git("config", "user.name", "tournament-test")
	git("config", "core.autocrlf", "false")
	git("config", "commit.gpgsign", "false")
	git("add", ".")
	git("commit", "-m", "green base")
	return dir
}

func stubTournamentProbes(t *testing.T) {
	t.Helper()
	oldLSP, oldDeps := TournamentLSPProbe, TournamentDependentsProbe
	TournamentLSPProbe = func(context.Context, string, []string) int { return 0 }
	TournamentDependentsProbe = func(context.Context, string, []string) int { return 0 }
	t.Cleanup(func() { TournamentLSPProbe, TournamentDependentsProbe = oldLSP, oldDeps })
}

func tournamentWorktreeList(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	return string(out)
}

// Scenario: A fixes the bug (green), B fixes it wrong (red). The arbiter
// must pick A by score, the merge must land A's fix green in main, and no
// worktree may survive the done flow.
func TestTournamentEndToEndWinnerSelectedAndMerged(t *testing.T) {
	repo := tournamentE2EBugRepo(t)
	stubTournamentProbes(t)
	ctx := context.Background()
	var mgr tournament.WorktreeManager
	base := tournamentHead(t, repo)

	// Mock candidate turns (P-5 drives these via mock provider turns).
	pathA, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	pathB, err := mgr.Create(repo, base, "candidate-b")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"candidate-a", "candidate-b"}) })
	if err := os.WriteFile(filepath.Join(pathA, "calc.go"), []byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathB, "calc.go"), []byte("package tournamentmini\n\nfunc Add(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// tournament_arbiter node, live collection, real suites.
	arbOut, err := behaviorTournamentArbiter(ctx, BehaviorInput{
		NodeID: "tournament_arbiter", WorkspaceCwd: repo,
		RawArgs: map[string]any{"base_commit": base},
	})
	if err != nil {
		t.Fatalf("live arbiter: %v", err)
	}
	if arbOut.Status != "done" {
		t.Fatalf("arbiter status = %q, want done (A must win by score)", arbOut.Status)
	}
	if arbOut.Payload["winner"] != "candidate-a" {
		t.Fatalf("winner = %v, want candidate-a", arbOut.Payload["winner"])
	}
	verdict, ok := arbOut.Payload["verdict"].(tournament.TournamentVerdict)
	if !ok || verdict.NeedsHumanDecision {
		t.Fatalf("winning verdict must not need a human: %+v", arbOut.Payload["verdict"])
	}

	// merge_and_audit node.
	mergeOut, err := behaviorTournamentMerge(ctx, BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-a"},
	})
	if err != nil {
		t.Fatalf("live merge: %v", err)
	}
	if mergeOut.Status != "done" {
		t.Fatalf("merge status = %q, want done", mergeOut.Status)
	}

	// AC-1/AC-2: main carries A's fix, suite green for real, nothing left.
	got, _ := os.ReadFile(filepath.Join(repo, "calc.go"))
	if !strings.Contains(string(got), "return a + b") {
		t.Fatalf("main workspace must carry A's fix, got %q", string(got))
	}
	suite, err := RunGoTestJSON(ctx, repo, "")
	if err != nil {
		t.Fatalf("verify suite: %v", err)
	}
	if suite.TotalTests != 1 || suite.PassedTests != 1 {
		t.Fatalf("main suite must be green 1/1, got %d/%d", suite.TotalTests, suite.PassedTests)
	}
	if list := tournamentWorktreeList(t, repo); strings.Contains(list, "candidate-") {
		t.Fatalf("no worktree may survive the done flow:\n%s", list)
	}
	if got := tournamentHead(t, repo); got != base {
		t.Fatalf("merge must not move HEAD: %s -> %s", base, got)
	}
}

// Scenario (CP-65 §7 failure case): both candidates stay red with equal
// scores — the tournament fails closed to a human with the ranking card,
// touches nothing, and still cleans up.
func TestTournamentTieRequiresHumanDecision(t *testing.T) {
	repo := tournamentE2EBugRepo(t)
	stubTournamentProbes(t)
	ctx := context.Background()
	var mgr tournament.WorktreeManager
	base := tournamentHead(t, repo)

	pathA, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	pathB, err := mgr.Create(repo, base, "candidate-b")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"candidate-a", "candidate-b"}) })
	// Both try, both stay red (different wrong guesses, same score shape).
	if err := os.WriteFile(filepath.Join(pathA, "calc.go"), []byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b + 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathB, "calc.go"), []byte("package tournamentmini\n\nfunc Add(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	arbOut, err := behaviorTournamentArbiter(ctx, BehaviorInput{
		NodeID: "tournament_arbiter", WorkspaceCwd: repo,
		RawArgs: map[string]any{"base_commit": base},
	})
	if err != nil {
		t.Fatalf("live arbiter: %v", err)
	}
	// Default max_attempts=1: no retry leg — straight to human.
	if arbOut.Status != "escalate" {
		t.Fatalf("tied tournament must escalate, got %q", arbOut.Status)
	}
	verdict, ok := arbOut.Payload["verdict"].(tournament.TournamentVerdict)
	if !ok {
		t.Fatalf("escalate payload must carry the verdict card, got %T", arbOut.Payload["verdict"])
	}
	if !verdict.NeedsHumanDecision || verdict.WinnerCandidateID != "" {
		t.Fatalf("tie verdict must need a human with no winner: %+v", verdict)
	}
	if len(verdict.Ranking) != 2 {
		t.Fatalf("ranking card must show both candidates, got %d", len(verdict.Ranking))
	}
	// AC-3: main workspace untouched by the failed tournament.
	got, _ := os.ReadFile(filepath.Join(repo, "calc.go"))
	if !strings.Contains(string(got), "return a - b") {
		t.Fatalf("failed tournament must not touch main, got %q", string(got))
	}
	if list := tournamentWorktreeList(t, repo); strings.Contains(list, "candidate-") {
		t.Fatalf("failed tournament must still clean up:\n%s", list)
	}
}
