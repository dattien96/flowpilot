package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/tournament"
	"flowpilot-runner/internal/workingmode"
)

// ---------------------------------------------------------------------------
// BUG-412 — tournament escalation child lacks provider session identity
// ---------------------------------------------------------------------------

func TestBug412_TournamentChildGetsProviderIdentity(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	svc.mu.Lock()
	svc.activeAccountID = "acct-1"
	svc.runs[parentID].providerAccountID = "acct-1"
	svc.mu.Unlock()

	childID, err := svc.escalateToTournament(parentID, "cap reached")
	if err != nil {
		t.Fatalf("escalateToTournament: %v", err)
	}
	child := tournamentChildOf(t, svc, parentID)
	if child.id != childID {
		t.Fatalf("child id = %q, want %q", child.id, childID)
	}
	// The child must carry the same durable provider identity fields a normal
	// createRun child gets, or every subsequent turn rejects 409
	// provider_account_changed / session_unavailable.
	if strings.TrimSpace(child.providerSessionID) == "" {
		t.Fatal("tournament child must own a provider session id")
	}
	if child.providerKey != ProviderKeyCodex {
		t.Fatalf("child provider = %q, want codex", child.providerKey)
	}
	if child.providerAccountID != "acct-1" {
		t.Fatalf("child providerAccountID = %q, want acct-1 (parent's account)", child.providerAccountID)
	}
	if child.chatFlowRef != workingmode.PackPrefix+tournamentHarnessFlowID {
		t.Fatalf("child chatFlowRef = %q, want pack tournament-harness", child.chatFlowRef)
	}
	if child.status != RunStatusIdle {
		t.Fatalf("child status = %q, want idle", child.status)
	}
}

func TestBug412_TournamentChildAcceptsFirstTurn(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	svc.mu.Lock()
	svc.activeAccountID = "acct-1"
	svc.runs[parentID].providerAccountID = "acct-1"
	svc.mu.Unlock()

	if _, err := svc.escalateToTournament(parentID, "cap reached"); err != nil {
		t.Fatalf("escalateToTournament: %v", err)
	}
	child := tournamentChildOf(t, svc, parentID)

	// The child must admit its own first turn — no provider_session_unavailable
	// / provider_account_changed 409.
	_, aerr := svc.startTurn(child.id, TurnInput{
		StepID:  tournamentHarnessFlowID,
		FlowRef: workingmode.PackPrefix + tournamentHarnessFlowID,
		Prompt:  "begin tournament",
	}, "tournament", "")
	if aerr != nil {
		t.Fatalf("tournament child first turn rejected: %v", aerr)
	}
}

func TestBug412_EscalationKicksChildFirstTurn(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	if !svc.maybeEscalateCapToTournament(parentID, "cap reached") {
		t.Fatal("maybeEscalateCapToTournament must dispatch the tournament child")
	}
	child := tournamentChildOf(t, svc, parentID)

	// The child turn must dispatch asynchronously (engine entry), not sit inert.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		started := child.turnCount > 0 || child.turnInFlight
		svc.mu.Unlock()
		if started {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tournament child never received its first turn after escalation")
}

// ---------------------------------------------------------------------------
// BUG-413 — continue on a tournament_escalation parent duplicates the child
// ---------------------------------------------------------------------------

func TestBug413_FlowControlContinueOnEscalatedLoopIsIdempotent(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	if _, err := svc.escalateToTournament(parentID, "cap reached"); err != nil {
		t.Fatalf("escalateToTournament: %v", err)
	}
	first := tournamentChildOf(t, svc, parentID)

	out, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue"})
	if err != nil {
		t.Fatalf("applyFlowControl continue: %v", err)
	}
	if out.NextAction != "awaiting_tournament" {
		t.Fatalf("continue NextAction = %q, want awaiting_tournament", out.NextAction)
	}
	second := tournamentChildOf(t, svc, parentID)
	if second.id != first.id {
		t.Fatalf("repeated continue minted a second child: %q vs %q", second.id, first.id)
	}
	loop := svc.agentOrchestrator.graphSnapshot(parentID).LoopState
	if loop.Status != LoopStatusTournamentEscalation {
		t.Fatalf("loop status = %q, want tournament_escalation", loop.Status)
	}
	if !strings.Contains(loop.GateReason, "tournament escalation") {
		t.Fatalf("gate reason overwritten: %q", loop.GateReason)
	}
}

func TestBug413_RepeatedContinuesKeepSingleChild(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	if _, err := svc.escalateToTournament(parentID, "cap reached"); err != nil {
		t.Fatalf("escalateToTournament: %v", err)
	}
	first := tournamentChildOf(t, svc, parentID)

	for i := 0; i < 3; i++ {
		if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue"}); err != nil {
			t.Fatalf("continue %d: %v", i, err)
		}
		if _, err := svc.resumeFlowWithFeedback(parentID, ""); err != nil {
			t.Fatalf("resume %d: %v", i, err)
		}
	}
	svc.mu.Lock()
	var children []string
	for _, c := range svc.runs {
		if c.parentRunID == parentID {
			children = append(children, c.id)
		}
	}
	svc.mu.Unlock()
	if len(children) != 1 || children[0] != first.id {
		t.Fatalf("repeated continues must keep exactly one tournament child, got %v", children)
	}
}

// ---------------------------------------------------------------------------
// BUG-414 — tournament tie-card choice must route to merge/retry, not vanish
// ---------------------------------------------------------------------------

// bug414Fixture builds a parked tournament child whose arbiter escalated with
// a tie card; candidate-b's worktree carries a committed fix.
func bug414Fixture(t *testing.T) (*InteractiveService, string, string, string) {
	t.Helper()
	svc, parentID := tournamentEscalationFixture(t, true)
	repo := tournamentE2EBugRepo(t)
	childID, err := svc.escalateToTournament(parentID, "cap reached")
	if err != nil {
		t.Fatalf("escalateToTournament: %v", err)
	}
	child := svc.runs[childID]
	svc.mu.Lock()
	child.workspaceCwd = repo
	svc.mu.Unlock()

	// candidate-b makes a real change in its own worktree (dirty, like the
	// candidate turn would leave it).
	mgr := &tournament.WorktreeManager{}
	path, err := mgr.Create(repo, tournamentHead(t, repo), "candidate-b")
	if err != nil {
		t.Fatalf("create candidate-b worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "calc.go"),
		[]byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"candidate-a", "candidate-b"}) })

	// The child is parked with the tie card the arbiter emitted.
	svc.mu.Lock()
	child.decisionCard = &UserDecisionCard{
		Kind:     DecisionCardKindTournament,
		Question: "Tournament verdict: tie — pick a winner or retry?",
		Options: []DecisionCardOption{
			{ID: "candidate-a", Label: "Candidate A"},
			{ID: "candidate-b", Label: "Candidate B"},
			{ID: "retry", Label: "Retry"},
			{ID: "ask", Label: "Ask"},
		},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(childID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate",
		GateReason: "tournament needs a human",
	})
	return svc, parentID, childID, repo
}

func TestBug414_TieCardCandidateChoiceMerges(t *testing.T) {
	svc, parentID, childID, repo := bug414Fixture(t)

	svc.captureDecisionChoice(childID, "candidate-b")
	if _, err := svc.resumeFlowWithFeedback(childID, "candidate-b"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	// The choice must reach tournament.merge: candidate-b's diff lands on the
	// workspace checkout.
	content, rerr := os.ReadFile(filepath.Join(repo, "calc.go"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(content), "return a + b") {
		t.Fatalf("choosing candidate-b must merge its worktree diff; calc.go = %q", string(content))
	}

	// A merged tournament resumes the parent loop (winner merged path).
	loop := svc.agentOrchestrator.graphSnapshot(parentID).LoopState
	if loop.Status != "running" {
		t.Fatalf("parent loop = %q, want running after merge", loop.Status)
	}
	if !strings.Contains(loop.GateReason, "candidate-b") {
		t.Fatalf("parent gate reason should name the merged winner, got %q", loop.GateReason)
	}
}

func TestBug414_TieCardRetryChoiceReattempts(t *testing.T) {
	svc, _, childID, _ := bug414Fixture(t)

	svc.captureDecisionChoice(childID, "retry")
	if _, err := svc.resumeFlowWithFeedback(childID, "retry"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	// Retry must arm a fresh attempt on the child rather than reopening the
	// loop to re-escalate.
	svc.mu.Lock()
	attempt := svc.runs[childID].tournamentAttempt
	svc.mu.Unlock()
	if attempt != 1 {
		t.Fatalf("tournamentAttempt = %d, want 1 after retry", attempt)
	}
	loop := svc.agentOrchestrator.graphSnapshot(childID).LoopState
	if loop.Status != "running" {
		t.Fatalf("child loop = %q, want running after retry", loop.Status)
	}
}

func TestBug414_UnknownTournamentChoiceRejected(t *testing.T) {
	svc, _, childID, _ := bug414Fixture(t)

	svc.mu.Lock()
	svc.runs[childID].decisionCardChosen = "candidate-z"
	svc.mu.Unlock()
	if _, err := svc.resumeFlowWithFeedback(childID, "candidate-z"); err == nil {
		t.Fatal("an unmapped tournament choice must be rejected, not silently dropped")
	}
}

// ---------------------------------------------------------------------------
// BUG-426 — tournament-harness flow must launch & engine-drive its nodes
// ---------------------------------------------------------------------------

func TestBug426_CreateRunPackRefSynthesizesSteps(t *testing.T) {
	svc := newFreezeTestService(t)

	handle, aerr := svc.createRun(StartRunInput{
		ProjectID:  "proj-web",
		WorkflowID: workingmode.PackPrefix + tournamentHarnessFlowID,
		Cwd:        t.TempDir(),
	})
	if aerr != nil {
		t.Fatalf("pack-ref flow must not 422 with workflow_has_no_steps: %v", aerr)
	}
	if handle.RunID == "" {
		t.Fatal("createRun returned empty run id")
	}
}

// newTournamentFreezeService registers fake codex AND claude adapters — the
// declared candidates pin claude-sonnet / gpt-5.4-mini respectively, so a
// codex-only registry fails candidate-a's spawn with provider_unavailable.
func newTournamentFreezeService(t *testing.T) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude} {
		k := key
		reg.register(ProviderRegistration{
			Key: k, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coded"})
					return nil
				})
			},
		})
	}
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func TestBug426_RolloutPassthroughSpawnsCandidates(t *testing.T) {
	svc := newTournamentFreezeService(t)
	repo := tournamentE2EBugRepo(t)

	def, err := tournamentFlowDefinition()
	if err != nil {
		t.Fatalf("tournamentFlowDefinition: %v", err)
	}
	runID := newFreezeTestRun(t, svc, repo, def.Edges, def.Nodes, "")

	if !svc.tryAdvanceFlowFromNode(runID, "problem_scout", "scout complete") {
		t.Fatal("parallel_rollout passthrough must spawn the candidate cohort")
	}
	svc.mu.Lock()
	var labels []string
	for _, c := range svc.runs {
		if c.parentRunID == runID {
			labels = append(labels, c.label)
		}
	}
	svc.mu.Unlock()
	if len(labels) != 2 {
		t.Fatalf("expected candidate-a + candidate-b children, got %v", labels)
	}
	seen := map[string]bool{}
	for _, l := range labels {
		seen[l] = true
	}
	if !seen["candidate-a"] || !seen["candidate-b"] {
		t.Fatalf("candidate labels wrong: %v", labels)
	}
}

func TestBug426_CandidateJoinDispatchesArbiterAndMerge(t *testing.T) {
	svc := newFreezeTestService(t)
	repo := tournamentE2EBugRepo(t)
	stubTournamentProbes(t)

	// candidate-a wins: it carries the fix (dirty in its worktree).
	var mgr tournament.WorktreeManager
	base := tournamentHead(t, repo)
	aPath, err := mgr.Create(repo, base, "candidate-a")
	if err != nil {
		t.Fatalf("create candidate-a worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aPath, "calc.go"),
		[]byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(repo, base, "candidate-b"); err != nil {
		t.Fatalf("create candidate-b worktree: %v", err)
	} // candidate-b left untouched → empty diff, losing score
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"candidate-a", "candidate-b"}) })

	def, err := tournamentFlowDefinition()
	if err != nil {
		t.Fatalf("tournamentFlowDefinition: %v", err)
	}
	runID := newFreezeTestRun(t, svc, repo, def.Edges, def.Nodes, base)

	// Both candidates already completed → the join gate must be satisfied and
	// the arbiter must run inline.
	svc.mu.Lock()
	svc.runs["cand-a"] = &interactiveRun{id: "cand-a", parentRunID: runID, label: "candidate-a", status: RunStatusCompleted}
	svc.runs["cand-b"] = &interactiveRun{id: "cand-b", parentRunID: runID, label: "candidate-b", status: RunStatusCompleted}
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(runID, "candidate-b", "candidate-b done") {
		t.Fatal("last candidate completion must dispatch tournament_arbiter")
	}

	// Arbiter (auto_pick) → merge candidate-a → done edge → terminal.
	content, rerr := os.ReadFile(filepath.Join(repo, "calc.go"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(content), "return a + b") {
		t.Fatalf("arbiter winner must route into tournament.merge; calc.go = %q", string(content))
	}
}

// TestBug426_JoinTargetIncludesFailedMember pins the partial-cohort contract
// found in the live re-verification (run-2): candidate-a's provider failure
// must NOT route the cohort around the declared join:all target — the
// arbiter still dispatches and scores the dead candidate's empty worktree.
func TestBug426_JoinTargetIncludesFailedMember(t *testing.T) {
	def, err := tournamentFlowDefinition()
	if err != nil {
		t.Fatalf("tournamentFlowDefinition: %v", err)
	}
	entries := []cohortEntry{
		{Label: "candidate-a", Provider: "claude", Status: "failed", Err: "no connected local account"},
		{Label: "candidate-b", Provider: "codex", Status: "completed", FinalMessage: "done"},
	}
	node, ok := flowSharedInlineJoinTarget(def.Nodes, def.Edges, entries)
	if !ok {
		t.Fatal("failed cohort member must not suppress the declared join:all target")
	}
	if node.ID != "tournament_arbiter" {
		t.Fatalf("shared join target = %q, want tournament_arbiter", node.ID)
	}
}

// TestBug426_ArbiterJoinSatisfiedWithFailedMember pins the second gate found
// in live run-1890: after the cohort barrier completed (candidate-a failed,
// candidate-b completed), cohort_join_inline_dispatch fired but
// tournamentJoinSatisfied still vetoed the arbiter because it only accepted
// status==completed. join:all treats terminal failure as collected.
func TestBug426_ArbiterJoinSatisfiedWithFailedMember(t *testing.T) {
	svc := newFreezeTestService(t)
	svc.mu.Lock()
	svc.runs["parent-h"] = &interactiveRun{id: "parent-h"}
	svc.runs["child-a"] = &interactiveRun{
		id: "child-a", parentRunID: "parent-h", label: "candidate-a",
		status: RunStatusFailed,
	}
	svc.runs["child-b"] = &interactiveRun{
		id: "child-b", parentRunID: "parent-h", label: "candidate-b",
		status: RunStatusCompleted,
	}
	svc.mu.Unlock()
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	if !svc.tournamentJoinSatisfied("parent-h", edges, "tournament_arbiter") {
		t.Fatal("terminal failed member must satisfy the join:all arbiter gate")
	}
}

// TestBug414_TournamentCardParsesThroughFlowControlValidator pins the live
// defect from run-2500: the arbiter's escalate carried a decision_card whose
// options were []map[string]any, but parseUserDecisionCard asserts []any —
// the card was rejected (decision_card_invalid) and the prose fallback
// parked, so captureDecisionChoice could never record a tournament choice.
func TestBug414_TournamentCardParsesThroughFlowControlValidator(t *testing.T) {
	out := BehaviorOutput{
		Status:  "escalate",
		Summary: "tie between candidates",
		Payload: map[string]any{
			"verdict": tournament.TournamentVerdict{
				Ranking: []tournament.CandidateScore{
					{CandidateID: "candidate-a", Total: 0.99},
					{CandidateID: "candidate-b", Total: 0.99},
				},
			},
		},
	}
	cardMap := tournamentDecisionCard(out)
	card, err := parseUserDecisionCard(cardMap)
	if err != nil {
		t.Fatalf("tournament card must survive applyFlowControl validation: %v", err)
	}
	if card.Kind != DecisionCardKindTournament {
		t.Fatalf("card kind = %q, want %q", card.Kind, DecisionCardKindTournament)
	}
	ids := map[string]bool{}
	for _, o := range card.Options {
		ids[o.ID] = true
	}
	for _, want := range []string{"candidate-a", "candidate-b", "retry", "ask"} {
		if !ids[want] {
			t.Fatalf("card missing option %q (have %v)", want, ids)
		}
	}
}

// TestBug414_EscalateCarriesMergeablePatches pins the live run-3077 defect:
// the arbiter's escalate cleans every candidate worktree (no-orphan
// contract, kept), so a human's card pick previously hit "no worktree for
// owner" in tournament.merge — the choice was consumed but unmergeable.
// The escalate payload must carry each candidate's patch snapshot, and
// tournament.merge must apply it worktree-free.
func TestBug414_EscalateCarriesMergeablePatches(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(pathA, "calc.go"),
		[]byte("package tournamentmini\n\nfunc Add(a, b int) int { return a + b + 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathB, "calc.go"),
		[]byte("package tournamentmini\n\nfunc Add(a, b int) int { return a * b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := behaviorTournamentArbiter(ctx, BehaviorInput{
		NodeID: "tournament_arbiter", WorkspaceCwd: repo,
		RawArgs: map[string]any{"base_commit": base},
	})
	if err != nil {
		t.Fatalf("arbiter: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("tie must escalate, got %q", out.Status)
	}
	patches, ok := out.Payload["patches"].(map[string]string)
	if !ok || patches["candidate-b"] == "" || patches["candidate-a"] == "" {
		t.Fatalf("escalate must carry mergeable patch snapshots, got %v", out.Payload["patches"])
	}
	// The pick path after the dirs are gone: merge applies the snapshot.
	mergeOut, err := behaviorTournamentMerge(ctx, BehaviorInput{
		NodeID: "merge_and_audit", WorkspaceCwd: repo,
		RawArgs: map[string]any{"winner": "candidate-b", "patch": patches["candidate-b"]},
	})
	if err != nil {
		t.Fatalf("merge from stored patch: %v", err)
	}
	if mergeOut.Status != "done" {
		t.Fatalf("merge from stored patch must be done, got %q", mergeOut.Status)
	}
	got, _ := os.ReadFile(filepath.Join(repo, "calc.go"))
	if !strings.Contains(string(got), "return a * b") {
		t.Fatalf("candidate-b patch must land on the main workspace, got %q", got)
	}
}

// TestBug414_RetryChoiceCleansStaleWorktrees pins the retry-after-park half:
// once escalate stops cleaning, a human "retry" must clean stale candidate
// dirs before re-spawn or Create hits "already exists" and candidates fall
// back to the main workspace (the worktree fallback the live log showed).
func TestBug414_RetryChoiceCleansStaleWorktrees(t *testing.T) {
	svc := newFreezeTestService(t)
	repo := tournamentE2EBugRepo(t)
	def, err := tournamentFlowDefinition()
	if err != nil {
		t.Fatalf("tournamentFlowDefinition: %v", err)
	}
	runID := newFreezeTestRun(t, svc, repo, def.Edges, def.Nodes, "")
	stale := tournament.WorktreePath(repo, "candidate-a")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("seed stale worktree: %v", err)
	}
	marker := filepath.Join(stale, "stale-marker.txt")
	if err := os.WriteFile(marker, []byte("old round"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}
	if _, err := svc.resumeTournamentChoice(runID, "retry", ""); err != nil {
		t.Fatalf("retry choice: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("stale candidate worktree content must be cleaned before retry re-spawn")
	}
}
