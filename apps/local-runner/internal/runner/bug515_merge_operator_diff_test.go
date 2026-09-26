package runner

// BUG-515 (deep review B-5 second half, 2026-09-26): the merge-stage
// decision card labels the conflicted winner honestly
// ("picked winner — patch conflicts") but re-selecting it re-applies the
// SAME stored patch, fails identically, and parks again — live run-3688
// showed re-pick → no-progress → discard. There was no path for the
// operator to submit the diff they resolved by hand.
//
// Fix: the decision's free-form feedback carries an optional unified diff —
// when it parses, runTournamentMergeNode applies the operator's resolution
// instead of the recorded snapshot. Plain-prose feedback keeps the prior
// behavior (stored patch; conflict re-parks honestly).

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func bug515ParkedConflict(t *testing.T, repo string) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.workspaceCwd = repo
	prs.activeFlowNodes = bug478Nodes()
	prs.activeFlowEdges = bug478Edges()
	prs.tournamentWinner = "candidate-b"
	svc.mu.Unlock()
	return svc, parent.RunID
}

func bug515Git(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestBug515_OperatorResolvedDiffOverridesStoredPatch parks the merge card
// on a winner whose recorded patch conflicts (the workspace drifted: the
// file the patch creates already exists), then the operator re-picks the
// winner carrying their hand-merged diff as feedback.
func TestBug515_OperatorResolvedDiffOverridesStoredPatch(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, parentID := bug515ParkedConflict(t, repo)

	// candidate-a's recorded patch creates mul.go.
	patchA := bug478RealPatch(t, repo)

	// Workspace drift: mul.go now exists with conflicting content — the
	// recorded create-patch can never apply cleanly again.
	os.WriteFile(filepath.Join(repo, "mul.go"),
		[]byte("package tournamentmini\n\nfunc Mul(a, b int) int { return a * b + 1 }\n"), 0o644)
	bug515Git(t, repo, "add", "mul.go")
	bug515Git(t, repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "drift")

	svc.mu.Lock()
	svc.runs[parentID].tournamentPatches = map[string]string{
		"candidate-b": "",
		"candidate-a": patchA,
	}
	svc.mu.Unlock()

	// First pick attempt with plain feedback → stored patch conflicts →
	// the merge card re-parks (prior behavior preserved).
	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], "")
	svc.mu.Lock()
	svc.runs[parentID].decisionCardChosen = "candidate-a"
	svc.mu.Unlock()
	if _, err := svc.resumeTournamentChoice(parentID, "candidate-a", "just merge it"); err != nil {
		t.Fatalf("resumeTournamentChoice(candidate-a, prose): %v", err)
	}
	svc.mu.Lock()
	card := svc.runs[parentID].decisionCard
	svc.mu.Unlock()
	if card == nil {
		t.Fatal("conflict on the stored patch must re-park the merge card")
	}

	// The operator resolves the conflict by hand: append a resolved
	// function to the drifted mul.go and submit that diff as feedback.
	os.WriteFile(filepath.Join(repo, "mul.go"),
		[]byte("package tournamentmini\n\nfunc Mul(a, b int) int { return a * b + 1 }\n\nfunc Mul3(a, b, c int) int { return a * b * c }\n"), 0o644)
	opDiff := bug515Git(t, repo, "diff")
	bug515Git(t, repo, "checkout", "--", "mul.go")
	if !strings.Contains(opDiff, "diff --git") {
		t.Fatalf("operator diff fixture must be a unified diff, got: %s", opDiff)
	}

	svc.mu.Lock()
	svc.runs[parentID].decisionCardChosen = "candidate-a"
	svc.mu.Unlock()
	if _, err := svc.resumeTournamentChoice(parentID, "candidate-a", "resolved by hand:\n"+opDiff); err != nil {
		t.Fatalf("resumeTournamentChoice(candidate-a, diff): %v", err)
	}

	got, err := os.ReadFile(filepath.Join(repo, "mul.go"))
	if err != nil {
		t.Fatalf("operator diff must apply: %v", err)
	}
	if !strings.Contains(string(got), "Mul3") {
		t.Fatalf("operator's resolved content must land in the workspace, got:\n%s", got)
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID); st.Status != "done" {
		t.Fatalf("loop status = %q, want done after the operator-resolved merge", st.Status)
	}
}

// TestBug515_ProseFeedbackKeepsStoredPatch guards the fallback: feedback
// that carries no unified diff must not perturb the merge — the recorded
// snapshot stays the source and a conflict re-parks honestly.
func TestBug515_ProseFeedbackKeepsStoredPatch(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, parentID := bug515ParkedConflict(t, repo)
	patchA := bug478RealPatch(t, repo)

	svc.mu.Lock()
	svc.runs[parentID].tournamentPatches = map[string]string{"candidate-a": patchA}
	svc.runs[parentID].tournamentWinner = "candidate-a"
	svc.mu.Unlock()

	// No conflict on this path — the clean stored patch applies and the
	// prose feedback (a human note) is ignored for merge purposes.
	svc.mu.Lock()
	svc.runs[parentID].decisionCardChosen = "candidate-a"
	svc.mu.Unlock()
	if _, err := svc.resumeTournamentChoice(parentID, "candidate-a", "looks good, ship it"); err != nil {
		t.Fatalf("resumeTournamentChoice: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err != nil {
		t.Fatalf("stored patch must apply when feedback carries no diff: %v", err)
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID); st.Status != "done" {
		t.Fatalf("loop status = %q, want done", st.Status)
	}
}
