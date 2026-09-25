package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/tournament"
)

// BUG-478 (live run-23455): an auto-picked winner whose recorded patch is
// empty parks the merge node with only a raw tournament_merge payload — the
// still-valid alternate candidate patches in rs.tournamentPatches are
// unreachable, and the operator's only way out is abandoning the run. The
// merge-stage escalation must carry a structured decision card offering
// every successfully captured patch plus retry/discard/ask, and each choice
// must be durable and idempotent across restart.

func bug478Nodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "candidate-a", Cohort: "tournament"},
		{ID: "candidate-b", Cohort: "tournament"},
		{ID: "tournament_arbiter", Run: "inline", Behavior: "tournament.arbiter", Join: "all"},
		{ID: "merge_and_audit", Run: "inline", Behavior: "tournament.merge"},
	}
}

func bug478Edges() []agentpack.FlowEdge {
	return []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "tournament_arbiter", To: "merge_and_audit", When: "done", Kind: "forward"},
	}
}

// bug478ParkedMerge puts the parent run in the exact live run-23455 shape:
// arbiter auto-picked candidate-b whose snapshot is empty; candidate-a's
// stored patch is still valid.
func bug478ParkedMerge(t *testing.T, repo string) (*InteractiveService, string) {
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
	prs.tournamentPatches = map[string]string{
		"candidate-b": "", // recorded-but-empty: the picked candidate changed nothing
	}
	svc.mu.Unlock()
	return svc, parent.RunID
}

func bug478RealPatch(t *testing.T, repo string) string {
	t.Helper()
	var mgr tournament.WorktreeManager
	base := tournamentHead(t, repo)
	pathA, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"candidate-a"}) })
	if err := os.WriteFile(filepath.Join(pathA, "mul.go"),
		[]byte("package tournamentmini\n\nfunc Mul(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := mgr.Diff(repo, "candidate-a")
	if err != nil {
		t.Fatalf("Diff A: %v", err)
	}
	if strings.TrimSpace(string(patch)) == "" {
		t.Fatal("candidate-a must produce a non-empty patch")
	}
	return string(patch)
}

func bug478OptionIDs(card *UserDecisionCard) map[string]bool {
	out := map[string]bool{}
	if card == nil {
		return out
	}
	for _, o := range card.Options {
		out[o.ID] = true
	}
	return out
}

func TestBUG478_MergeEscalateOffersValidAlternatePatches(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, parentID := bug478ParkedMerge(t, repo)
	patch := bug478RealPatch(t, repo)

	svc.mu.Lock()
	svc.runs[parentID].tournamentPatches["candidate-a"] = patch
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], "")

	svc.mu.Lock()
	card := svc.runs[parentID].decisionCard
	svc.mu.Unlock()
	if card == nil {
		t.Fatal("merge-stage escalate parked with no decision card — alternate patches unreachable (BUG-478)")
	}
	if card.Kind != DecisionCardKindTournament {
		t.Fatalf("card kind = %q, want tournament so choices route into the merge path", card.Kind)
	}
	opts := bug478OptionIDs(card)
	if !opts["candidate-a"] {
		t.Fatalf("valid alternate candidate-a missing from merge card options: %+v", card.Options)
	}
	if opts["candidate-b"] {
		t.Fatal("empty/failed snapshot candidate-b must never be offered as mergeable")
	}
	for _, want := range []string{"retry", "discard", "ask"} {
		if !opts[want] {
			t.Fatalf("merge card missing %q option: %+v", want, card.Options)
		}
	}
}

func TestBUG478_CandidateChoiceAppliesStoredPatch(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, parentID := bug478ParkedMerge(t, repo)
	patch := bug478RealPatch(t, repo)

	svc.mu.Lock()
	svc.runs[parentID].tournamentPatches["candidate-a"] = patch
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], "")

	// Human picks the still-valid alternate off the merge card.
	svc.mu.Lock()
	svc.runs[parentID].decisionCardChosen = "candidate-a"
	svc.mu.Unlock()
	if _, err := svc.resumeTournamentChoice(parentID, "candidate-a", ""); err != nil {
		t.Fatalf("resumeTournamentChoice(candidate-a): %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err != nil {
		t.Fatalf("alternate patch not applied — mul.go missing: %v", err)
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID); st.Status != "done" {
		t.Fatalf("loop status = %q, want done after alternate merged", st.Status)
	}
}

func TestBUG478_DiscardTerminalizesHonestly(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, parentID := bug478ParkedMerge(t, repo)
	patch := bug478RealPatch(t, repo)

	svc.mu.Lock()
	svc.runs[parentID].tournamentPatches["candidate-a"] = patch
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], "")

	svc.mu.Lock()
	svc.runs[parentID].decisionCardChosen = "discard"
	svc.mu.Unlock()
	if _, err := svc.resumeTournamentChoice(parentID, "discard", ""); err != nil {
		t.Fatalf("resumeTournamentChoice(discard): %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err == nil {
		t.Fatal("discard must not apply any patch")
	}
	svc.mu.Lock()
	left := svc.runs[parentID].tournamentPatches
	svc.mu.Unlock()
	if len(left) != 0 {
		t.Fatalf("discard must clear stored snapshots, left %d", len(left))
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID); st.Status != "done" {
		t.Fatalf("loop status = %q, want honest done after discard", st.Status)
	}
}

func TestBUG478_RestartPreservesMergeCardAndAppliesOnce(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	patch := bug478RealPatch(t, repo)
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.workspaceCwd = repo
	prs.runKind = "workflow" // synthetic thread-* session skips provider-resume readiness
	prs.activeFlowNodes = bug478Nodes()
	prs.activeFlowEdges = bug478Edges()
	prs.tournamentWinner = "candidate-b"
	prs.tournamentPatches = map[string]string{"candidate-b": "", "candidate-a": patch}
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parent.RunID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], "")

	// Persist exactly what a graceful shutdown would, then rebuild the run
	// in a fresh service over the same store — the simulated restart.
	svc.mu.Lock()
	snap := sessionStateOf(svc.runs[parent.RunID])
	svc.mu.Unlock()
	if err := svc.persistProviderSession(snap); err != nil {
		t.Fatalf("persistProviderSession: %v", err)
	}
	restarted := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := restarted.resumeRun(parent.RunID); apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}

	restarted.mu.Lock()
	rrs := restarted.runs[parent.RunID]
	card := rrs.decisionCard
	patches := rrs.tournamentPatches
	restarted.mu.Unlock()
	if card == nil {
		t.Fatal("restart dropped the parked merge decision card")
	}
	if !bug478OptionIDs(card)["candidate-a"] {
		t.Fatal("restart lost the alternate candidate option")
	}
	if strings.TrimSpace(patches["candidate-a"]) == "" {
		t.Fatal("restart lost the stored patch snapshots")
	}

	if _, err := restarted.resumeTournamentChoice(parent.RunID, "candidate-a", ""); err != nil {
		t.Fatalf("post-restart resumeTournamentChoice(candidate-a): %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err != nil {
		t.Fatalf("alternate patch not applied after restart: %v", err)
	}
	// Consumed once: the card must be gone so a replayed choice cannot
	// re-apply the patch.
	restarted.mu.Lock()
	cardAfter := restarted.runs[parent.RunID].decisionCard
	restarted.mu.Unlock()
	if cardAfter != nil {
		t.Fatal("consumed decision card must clear — a replay could double-apply the patch")
	}
}
