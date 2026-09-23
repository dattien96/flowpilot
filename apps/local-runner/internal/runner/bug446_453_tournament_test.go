package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"flowpilot-runner/internal/tournament"
)

// BUG-446: escalateToTournament allocates the child id inside s.mu but never
// re-checks for an existing tournament_escalation child inside that same
// critical section. Two triggers that both pass the outer scan can spawn
// -tournament and -tournament-2. The insertion lock itself must be the
// authoritative dedup point.
func TestBug446SecondEscalationIsRefused(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	first, err := svc.escalateToTournament(parentID, "review cap 3 reached")
	if err != nil {
		t.Fatalf("first escalation: %v", err)
	}
	if _, err := svc.escalateToTournament(parentID, "review cap 3 reached"); err == nil {
		t.Fatal("second escalation must be refused — a tournament child already exists")
	}
	svc.mu.Lock()
	count := 0
	for _, rs := range svc.runs {
		if rs.parentRunID == parentID && rs.label == "tournament_escalation" {
			count++
		}
	}
	svc.mu.Unlock()
	if count != 1 {
		t.Fatalf("exactly one tournament child must exist, got %d (first=%q)", count, first)
	}
}

// BUG-446: concurrent cap/stall triggers racing through the dedup scan must
// still produce exactly one durable child and one dispatched first turn.
func TestBug446ConcurrentEscalationSingleChild(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	const triggers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < triggers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			svc.maybeEscalateCapToTournament(parentID, "review cap 3 reached")
		}()
	}
	close(start)
	wg.Wait()
	svc.mu.Lock()
	var children []string
	for _, rs := range svc.runs {
		if rs.parentRunID == parentID && rs.label == "tournament_escalation" {
			children = append(children, rs.id)
		}
	}
	svc.mu.Unlock()
	if len(children) != 1 {
		t.Fatalf("concurrent triggers must yield exactly 1 tournament child, got %v", children)
	}
}

// BUG-453: the arbiter ignored mgr.Diff errors, so a candidate whose patch
// snapshot failed could still be offered on the escalate decision card after
// its worktree was cleaned — the pick then hits "no worktree for owner".
// Snapshot failure must fail closed like the sibling probe errors: error +
// full candidate cleanup, never an unmergeable card.
func TestBug453SnapshotFailureFailsClosed(t *testing.T) {
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
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(filepath.Join(p, "calc.go"),
			[]byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Poison candidate-a's worktree index so `git add -N` inside Diff fails
	// while `git status` and the test suite still work — a Diff-only fault.
	gitFile, err := os.ReadFile(filepath.Join(pathA, ".git"))
	if err != nil {
		t.Fatalf("read worktree .git: %v", err)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFile), "gitdir:"))
	if gitDir == "" || gitDir == string(gitFile) {
		t.Fatalf("unexpected .git file content: %q", gitFile)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), []byte("held"), 0o644); err != nil {
		t.Fatalf("seed index.lock: %v", err)
	}
	// Confirm the injection really breaks only the diff path.
	if _, derr := mgr.Diff(repo, "candidate-a"); derr == nil {
		t.Fatal("index.lock must make Diff fail for candidate-a")
	}

	_, err = behaviorTournamentArbiter(ctx, BehaviorInput{
		NodeID: "tournament_arbiter", WorkspaceCwd: repo,
		RawArgs: map[string]any{"base_commit": base},
	})
	if err == nil {
		t.Fatal("arbiter must fail closed when a candidate patch snapshot fails")
	}
	if !strings.Contains(err.Error(), "candidate-a") {
		t.Fatalf("error must name the failed candidate, got %v", err)
	}
	// No candidate worktree may survive a failed arbiter — same no-orphan
	// contract as the sibling ensure/metrics errors.
	for _, id := range []string{"candidate-a", "candidate-b"} {
		if fi, statErr := os.Stat(tournament.WorktreePath(repo, id)); statErr == nil && fi.IsDir() {
			t.Fatalf("failed arbiter must clean worktree %q", id)
		}
	}
}

// BUG-453 (merge half): a recorded-but-empty patch snapshot means the picked
// candidate changed nothing — an explicit escalate, not a "no worktree for
// owner" error against a cleaned directory. An ABSENT patch key keeps the
// legacy MergeWinner path (arbiter-picked winner retains its worktree).
func TestBug453EmptyStoredPatchEscalatesExplicitly(t *testing.T) {
	repo := tournamentE2EBugRepo(t)
	out, err := behaviorTournamentMerge(context.Background(), BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-a", "patch": ""},
	})
	if err != nil {
		t.Fatalf("empty recorded patch must escalate, not error: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("empty recorded patch must escalate, got %q", out.Status)
	}
	if reason, _ := out.Payload["reason"].(string); reason != "empty_patch" {
		t.Fatalf("escalate must mark reason=empty_patch, got %+v", out.Payload)
	}
}
