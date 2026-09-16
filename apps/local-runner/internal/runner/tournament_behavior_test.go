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

// CP-65 P-3 (Task-370): tournament execution glue. New file — no
// pre-existing test is modified.

func tournamentBehaviorRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "tournament@test.local")
	run("config", "user.name", "tournament-test")
	run("config", "core.autocrlf", "false")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "base.txt")
	run("commit", "-m", "base")
	return dir
}

func tournamentHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func tournamentResults(aTotal, aPassed, bTotal, bPassed int) []tournament.CandidateResult {
	return []tournament.CandidateResult{
		{CandidateID: "candidate-a", ProviderKey: "claude", TotalTests: aTotal, PassedTests: aPassed},
		{CandidateID: "candidate-b", ProviderKey: "codex", TotalTests: bTotal, PassedTests: bPassed},
	}
}

func TestTournamentBehaviorsRegisteredInline(t *testing.T) {
	for _, id := range []string{"tournament.arbiter", "tournament.merge"} {
		spec, err := DefaultBehaviorRegistry().Resolve(id)
		if err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
		if spec.Scope != BehaviorScopeInline {
			t.Fatalf("%s scope = %q, want inline (deterministic, no provider call)", id, spec.Scope)
		}
	}
}

func TestBehaviorTournamentArbiterPicksWinnerFromInjectedResults(t *testing.T) {
	out, err := behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID:       "tournament_arbiter",
		WorkspaceCwd: t.TempDir(),
		Payload: map[string]any{
			"candidateResults": tournamentResults(10, 10, 10, 4),
		},
	})
	if err != nil {
		t.Fatalf("arbiter: %v", err)
	}
	if out.Status != "done" {
		t.Fatalf("status = %q, want done (merge)", out.Status)
	}
	if out.Payload["winner"] != "candidate-a" || out.Payload["action"] != tournament.TournamentActionMerge {
		t.Fatalf("payload must name winner candidate-a + merge, got %+v", out.Payload)
	}
}

func TestBehaviorTournamentArbiterRetryAndAskPaths(t *testing.T) {
	tie := tournamentResults(10, 8, 10, 8)
	retryCfg := map[string]any{"max_attempts": 2}
	// Attempt 1 of 2 with auto_pick on retries with a brief.
	out, err := behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID:       "tournament_arbiter",
		WorkspaceCwd: t.TempDir(),
		RawArgs:      map[string]any{"config": retryCfg, "attempt": 1},
		Payload:      map[string]any{"candidateResults": tie},
	})
	if err != nil {
		t.Fatalf("arbiter: %v", err)
	}
	if out.Status != "retry" {
		t.Fatalf("status = %q, want retry", out.Status)
	}
	brief, _ := out.Payload["brief"].(string)
	if brief == "" {
		t.Fatal("retry must carry a distilled brief for the fresh spawns")
	}
	if out.Payload["nextAttempt"] != 2 {
		t.Fatalf("nextAttempt = %v, want 2", out.Payload["nextAttempt"])
	}
	// Attempt 2 of 2 escalates.
	out, err = behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID:       "tournament_arbiter",
		WorkspaceCwd: t.TempDir(),
		RawArgs:      map[string]any{"config": retryCfg, "attempt": 2},
		Payload:      map[string]any{"candidateResults": tie},
	})
	if err != nil {
		t.Fatalf("arbiter: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("status = %q, want escalate (attempts exhausted)", out.Status)
	}
	// auto_pick=false escalates immediately.
	out, err = behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID:       "tournament_arbiter",
		WorkspaceCwd: t.TempDir(),
		RawArgs:      map[string]any{"config": map[string]any{"auto_pick": false, "max_attempts": 2}, "attempt": 1},
		Payload:      map[string]any{"candidateResults": tie},
	})
	if err != nil {
		t.Fatalf("arbiter: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("status = %q, want escalate (auto_pick off)", out.Status)
	}
}

func TestBehaviorTournamentArbiterRejectsBadInput(t *testing.T) {
	if _, err := behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID:  "tournament_arbiter",
		RawArgs: map[string]any{"config": map[string]any{"max_attempts": 9}},
	}); err == nil {
		t.Fatal("bad node config must fail the behavior")
	}
	if _, err := behaviorTournamentArbiter(context.Background(), BehaviorInput{NodeID: "tournament_arbiter"}); err == nil {
		t.Fatal("missing workspace must fail the behavior")
	}
	if _, err := behaviorTournamentMerge(context.Background(), BehaviorInput{NodeID: "merge_and_audit"}); err == nil {
		t.Fatal("missing winner/workspace must fail the merge behavior")
	}
}

func TestBehaviorTournamentMergeAppliesWinnerPatch(t *testing.T) {
	repo := tournamentBehaviorRepo(t)
	base := tournamentHead(t, repo)
	var mgr tournament.WorktreeManager
	pathW, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pathW, "base.txt"), []byte("v1-winner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := behaviorTournamentMerge(context.Background(), BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-a"},
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if out.Status != "done" {
		t.Fatalf("status = %q, want done", out.Status)
	}
	got, _ := os.ReadFile(filepath.Join(repo, "base.txt"))
	if string(got) != "v1-winner\n" {
		t.Fatalf("main workspace must carry the winner fix, got %q", string(got))
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".flowpilot", "worktrees", "candidate-candidate-a")); !os.IsNotExist(statErr) {
		t.Fatal("winner worktree must be cleaned at flow done")
	}
	if _, ok := out.Payload["draftSummary"].(string); !ok {
		t.Fatal("merge payload must carry an audit-draft-shaped summary")
	}
}

func TestBehaviorTournamentMergeEscalatesOnConflict(t *testing.T) {
	repo := tournamentBehaviorRepo(t)
	base := tournamentHead(t, repo)
	var mgr tournament.WorktreeManager
	pathW, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pathW, "base.txt"), []byte("v1-winner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Main workspace drifts on the same file before the merge lands.
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("v1-drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := behaviorTournamentMerge(context.Background(), BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-a"},
	})
	if err != nil {
		t.Fatalf("conflict must escalate, not error: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("status = %q, want escalate", out.Status)
	}
	patch, _ := out.Payload["patch"].(string)
	if patch == "" {
		t.Fatal("escalate payload must carry the winner patch for the human card")
	}
	paths, _ := out.Payload["conflictPaths"].([]string)
	if len(paths) != 1 || paths[0] != "base.txt" {
		t.Fatalf("conflictPaths must name base.txt, got %q", paths)
	}
	got, _ := os.ReadFile(filepath.Join(repo, "base.txt"))
	if string(got) != "v1-drifted\n" {
		t.Fatalf("failed merge must not touch the workspace, got %q", string(got))
	}
}

func TestRunGoTestJSONCountsRealSuite(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module tournamentmini\n\ngo 1.26\n")
	write("calc_test.go", "package tournamentmini\n\nimport \"testing\"\n\nfunc TestGreen(t *testing.T) {}\n\nfunc TestRed(t *testing.T) { t.Fatal(\"boom\") }\n")
	res, err := RunGoTestJSON(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("RunGoTestJSON: %v", err)
	}
	if res.TotalTests != 2 || res.PassedTests != 1 {
		t.Fatalf("counts = total:%d passed:%d, want 2/1", res.TotalTests, res.PassedTests)
	}
	if len(res.Failed) != 1 || res.Failed[0] != "TestRed" {
		t.Fatalf("failed = %q, want [TestRed]", res.Failed)
	}
	if res.CompileBroken {
		t.Fatal("a red (not broken) suite must not flag CompileBroken")
	}
}

func TestBehaviorTournamentLiveMiniRound(t *testing.T) {
	// Full live loop on a real temp module: baseline suite runs for real,
	// candidate A adds a green feature, candidate B breaks a baseline test.
	// Suite runs real; LSP/dependents are stubbed (Task-372 T-1 seam).
	repo := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module tournamentmini\n\ngo 1.26\n")
	write("calc.go", "package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n")
	write("calc_test.go", "package tournamentmini\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"add broken\") } }\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
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
	git("commit", "-m", "base")

	// Stub the non-suite probes (suite stays real).
	oldLSP, oldDeps := TournamentLSPProbe, TournamentDependentsProbe
	TournamentLSPProbe = func(context.Context, string, []string) int { return 0 }
	TournamentDependentsProbe = func(context.Context, string, []string) int { return 0 }
	t.Cleanup(func() { TournamentLSPProbe, TournamentDependentsProbe = oldLSP, oldDeps })

	// Simulate the two candidate turns (P-5 will drive these via mocks):
	// A adds a green feature, B breaks a baseline test.
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
	if err := os.WriteFile(filepath.Join(pathA, "mul.go"), []byte("package tournamentmini\n\nfunc Mul(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathA, "mul_test.go"), []byte("package tournamentmini\n\nimport \"testing\"\n\nfunc TestMul(t *testing.T) { if Mul(2, 3) != 6 { t.Fatal(\"mul broken\") } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathB, "calc.go"), []byte("package tournamentmini\n\nfunc Add(a, b int) int { return a - b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Live arbiter path: default config names candidate-a/b, real suites.
	out, err := behaviorTournamentArbiter(context.Background(), BehaviorInput{
		NodeID: "tournament_arbiter", WorkspaceCwd: repo,
		RawArgs: map[string]any{"base_commit": base},
	})
	if err != nil {
		t.Fatalf("live arbiter: %v", err)
	}
	if out.Status != "done" {
		t.Fatalf("status = %q, want done (A must win)", out.Status)
	}
	if out.Payload["winner"] != "candidate-a" {
		t.Fatalf("winner = %v, want candidate-a", out.Payload["winner"])
	}
	// Loser cleaned at verdict, winner kept for the merge behavior.
	if _, statErr := os.Stat(tournament.WorktreePath(repo, "candidate-b")); !os.IsNotExist(statErr) {
		t.Fatal("loser worktree must be cleaned at verdict")
	}
	if _, statErr := os.Stat(tournament.WorktreePath(repo, "candidate-a")); statErr != nil {
		t.Fatalf("winner worktree must survive for merge: %v", statErr)
	}
	// B broke a baseline-green test: the verdict ranking must flag it.
	verdict, ok := out.Payload["verdict"].(tournament.TournamentVerdict)
	if !ok {
		t.Fatalf("payload verdict has wrong type %T", out.Payload["verdict"])
	}
	flagged := false
	for _, s := range verdict.Ranking {
		if s.CandidateID == "candidate-b" && s.Disqualified {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("candidate-b must be disqualified (broke baseline-green TestAdd)")
	}

	// Merge behavior lands A's feature in main and cleans up.
	mout, err := behaviorTournamentMerge(context.Background(), BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-a"},
	})
	if err != nil {
		t.Fatalf("live merge: %v", err)
	}
	if mout.Status != "done" {
		t.Fatalf("merge status = %q, want done", mout.Status)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "mul.go")); statErr != nil {
		t.Fatalf("winner feature must land in main: %v", statErr)
	}
	if _, statErr := os.Stat(tournament.WorktreePath(repo, "candidate-a")); !os.IsNotExist(statErr) {
		t.Fatal("winner worktree must be cleaned at flow done")
	}
	if got := tournamentHead(t, repo); got != base {
		t.Fatalf("merge must not move HEAD: %s -> %s", base, got)
	}
}
