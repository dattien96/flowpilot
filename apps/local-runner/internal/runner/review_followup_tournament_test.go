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
)

func TestBug522_TournamentWorktreeCreateFailureDoesNotSpawnInMainWorkspace(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	candidate := agentpack.FlowNode{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament"}
	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, []agentpack.FlowNode{candidate}, "solve")
	time.Sleep(100 * time.Millisecond)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, child := range svc.runs {
		if child.parentRunID == parent.RunID && child.label == candidate.ID {
			t.Fatalf("candidate spawned after worktree creation failed: cwd=%q", child.workspaceCwd)
		}
	}
}

func TestBug524_TournamentRetryJoinIgnoresTerminalChildrenFromPriorCohort(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatal(aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].tournamentAttempt = 1
	svc.runs["old-a"] = &interactiveRun{id: "old-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-0", status: RunStatusCompleted}
	svc.runs["old-b"] = &interactiveRun{id: "old-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-0", status: RunStatusCompleted}
	svc.runs["new-a"] = &interactiveRun{id: "new-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-1", status: RunStatusCompleted}
	svc.runs["new-b"] = &interactiveRun{id: "new-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-1", status: RunStatusRunning}
	svc.mu.Unlock()

	if svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("retry join was satisfied by terminal children from the prior cohort")
	}
}

// BUG-524 residual: a restart landing between the round-2 child spawn and its
// RUNNING sidecar stamp leaves the round-1 DONE stamp as the last durable
// record. The live current-attempt child in s.runs is authoritative — the
// sidecar fallback must not satisfy the join while it is still running.
func TestBug524_SidecarStampDoesNotBypassLiveRetryChild(t *testing.T) {
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatal(aerr)
	}
	edges := []agentpack.FlowEdge{
		{From: "candidate-a", To: "tournament_arbiter", When: "done", Kind: "forward"},
		{From: "candidate-b", To: "tournament_arbiter", When: "done", Kind: "forward"},
	}
	// Round-1 terminal stamps survived the restart; the round-2 RUNNING stamp
	// never reached the sidecar before the kill.
	for _, id := range []string{"candidate-a", "candidate-b"} {
		if err := store.AppendStepTransition(context.Background(), parent.RunID, stepTransitionLine{
			RunID: parent.RunID, NodeID: id, Status: string(StepStatusDone), TS: "2026-09-26T11:17:44Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].tournamentAttempt = 1
	svc.runs["new-a"] = &interactiveRun{id: "new-a", parentRunID: parent.RunID, label: "candidate-a", flowCohortId: "attempt-1", status: RunStatusRunning}
	svc.runs["new-b"] = &interactiveRun{id: "new-b", parentRunID: parent.RunID, label: "candidate-b", flowCohortId: "attempt-1", status: RunStatusCompleted}
	svc.mu.Unlock()

	if svc.tournamentJoinSatisfied(parent.RunID, edges, "tournament_arbiter") {
		t.Fatal("stale prior-round sidecar stamp satisfied the join while a current-attempt child is still running")
	}
}

func TestBug525_AutomaticMergeIgnoresDiffInAgentResultMessage(t *testing.T) {
	repo := tournamentE2EGreenRepo(t)
	svc, _, parentID := bug515ParkedConflict(t, repo)
	recorded := bug478RealPatch(t, repo)

	injectedPath := filepath.Join(repo, "injected.go")
	agentDiff := "diff --git a/injected.go b/injected.go\nnew file mode 100644\nindex 0000000..f48f0ad\n--- /dev/null\n+++ b/injected.go\n@@ -0,0 +1,3 @@\n+package tournamentmini\n+\n+func Injected() bool { return true }\n"

	svc.mu.Lock()
	svc.runs[parentID].tournamentWinner = "candidate-a"
	svc.runs[parentID].tournamentPatches = map[string]string{"candidate-a": recorded}
	svc.mu.Unlock()

	svc.runTournamentMergeNode(context.Background(), parentID, bug478Edges(), bug478Nodes(), bug478Nodes()[3], agentDiff)
	if _, err := os.Stat(filepath.Join(repo, "mul.go")); err != nil {
		t.Fatalf("recorded winner patch was not applied: %v", err)
	}
	if _, err := os.Stat(injectedPath); !os.IsNotExist(err) {
		t.Fatalf("agent result diff overrode the recorded winner patch: err=%v", err)
	}
}

// BUG-533: candidate-a's worktree is created, then candidate-b fails the
// provider-selectability check — the abort escalates but must also sweep the
// already-created worktree. Pre-fix it orphaned; every later retry wedged on
// "worktree already exists" (live run-1 needed manual `git worktree remove`).
func TestBug533_ProviderCheckAbortCleansEarlierWorktree(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyCodex, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	// grok deliberately unregistered — Selectable fails for candidate-b.
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	repo := tournamentE2EGreenRepo(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	candidates := []agentpack.FlowNode{
		{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "gpt-5.4-mini"},
		{ID: "candidate-b", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "grok-4.5"},
	}
	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, candidates, "solve")

	if _, err := os.Stat(tournament.WorktreePath(repo, "candidate-a")); !os.IsNotExist(err) {
		t.Fatalf("candidate-a worktree orphaned after provider-check abort: stat err=%v", err)
	}
}

// BUG-533 wedge case: a stale dir from a previously aborted spawn makes
// Create fail "already exists". The failure path must sweep the stale dir
// (no live child owns it) and retry once — self-healing the spawn instead of
// wedging the escalate/continue cycle on "already exists" forever.
func TestBug533_StaleWorktreeSweptOnCreateFailure(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyCodex, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	repo := tournamentE2EGreenRepo(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// Stale dir from an earlier aborted attempt — no live child owns it.
	var mgr tournament.WorktreeManager
	if _, err := mgr.Create(repo, "HEAD", "candidate-a"); err != nil {
		t.Fatalf("seed stale worktree: %v", err)
	}
	candidates := []agentpack.FlowNode{
		{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "gpt-5.4-mini"},
	}
	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, candidates, "solve")

	// The stale dir is swept, recreated, and the child actually spawned into
	// it — pre-fix the abort escalated before any spawn and the stale dir
	// stayed behind to wedge every retry.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, child := range svc.runs {
		if child.parentRunID == parent.RunID && child.label == "candidate-a" {
			if child.workspaceCwd != tournament.WorktreePath(repo, "candidate-a") {
				t.Fatalf("candidate child cwd=%q want worktree %q", child.workspaceCwd, tournament.WorktreePath(repo, "candidate-a"))
			}
			return
		}
	}
	t.Fatal("stale worktree wedged the spawn — no candidate child created")
}

// BUG-535: a cancelled run's leg keeps leg_state=active on the candidate
// worktree forever (live /tmp/fp-live3: three active legs on
// candidate-candidate-a). When the spawn path sweeps a stale dir it must
// also close those residue claims so no durable claim outlives the dir.
func TestBug535_WorktreeSweepClosesStaleLegClaims(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyCodex, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	repo := tournamentE2EGreenRepo(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// Dead-run residue: the old run is terminal but its leg still claims
	// the worktree dir (exactly what a kill/restart leaves behind).
	stale, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	var mgr tournament.WorktreeManager
	if _, err := mgr.Create(repo, "HEAD", "candidate-a"); err != nil {
		t.Fatalf("seed claimed worktree: %v", err)
	}
	svc.mu.Lock()
	sr := svc.runs[stale.RunID]
	sr.status = RunStatusCancelled
	sr.legState = LegStateActive
	sr.workspaceCwd = tournament.WorktreePath(repo, "candidate-a")
	svc.mu.Unlock()

	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, []agentpack.FlowNode{
		{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "gpt-5.4-mini"},
	}, "solve")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if sr.legState != LegStateClosed || sr.legClosedReason != LegClosedReasonWorktreeSwept {
		t.Fatalf("stale leg claim survives the sweep: state=%q reason=%q", sr.legState, sr.legClosedReason)
	}
	for _, child := range svc.runs {
		if child.parentRunID == parent.RunID && child.label == "candidate-a" {
			return
		}
	}
	t.Fatal("claimed-by-dead-run worktree wedged the spawn — no candidate child created")
}

// BUG-535 cross-run guard: the dir is claimed by a DIFFERENT run's live
// leg — never sweep an actively-claimed worktree out from under it
// (pre-fix liveChild only guarded same-attempt children of this parent).
func TestBug535_LiveClaimantBlocksSweep(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyCodex, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	repo := tournamentE2EGreenRepo(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// A different (still-running) run's leg owns the dir.
	rival, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	var mgr tournament.WorktreeManager
	if _, err := mgr.Create(repo, "HEAD", "candidate-a"); err != nil {
		t.Fatalf("seed claimed worktree: %v", err)
	}
	dir := tournament.WorktreePath(repo, "candidate-a")
	svc.mu.Lock()
	rr := svc.runs[rival.RunID]
	rr.status = RunStatusRunning
	rr.legState = LegStateActive
	rr.workspaceCwd = dir
	svc.mu.Unlock()

	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, []agentpack.FlowNode{
		{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "gpt-5.4-mini"},
	}, "solve")

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("live claimant's worktree was swept: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rr.legState != LegStateActive {
		t.Fatalf("live claimant leg closed by another run's spawn: %q", rr.legState)
	}
	if loop := svc.agentOrchestrator.loopStateFor(parent.RunID); loop.Status != "blocked" {
		t.Fatalf("contested worktree should park the flow, loop=%q", loop.Status)
	}
}

// BUG-536 (R6 residual): binding picked providers by connectivity only —
// a ledger-blocked account (billing_required) was re-bound every retry
// round, so each cohort's candidate vetoed on admission and re-fired the
// same quota card. Binding must skip providers whose resolved active
// account is durably hard-blocked.
func TestBug536_BindingSkipsLedgerBlockedProviderAccount(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, "grok-home")
	devinHome := filepath.Join(root, "devin-home")
	for _, dir := range []string{grokHome, devinHome} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeLinesToPath083(t, filepath.Join(grokHome, "auth.json"), []string{`{"refresh_token":"r","email":"a@b.c"}`})
	writeLinesToPath083(t, filepath.Join(devinHome, "credentials.toml"), []string{`api_key = "k"`})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "grok-blocked", ProviderKey: "grok", IsActive: true, AuthStatus: "connected", HomePath: grokHome},
		{ID: "devin-live", ProviderKey: "devin", IsActive: true, AuthStatus: "connected", HomePath: devinHome},
	})
	reg := newProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyGrok, ProviderKeyDevin} {
		key := key
		reg.register(ProviderRegistration{Key: key, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		}})
	}
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	svc.quotaRuntimePath = filepath.Join(t.TempDir(), "quota.json")
	svc.noteAccountBlockedLocked("grok", "grok-blocked", "billing_required")

	def, err := svc.bindTournamentCandidatesToAvailableProviders(tournamentFlowDefinitionMust(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range def.Nodes {
		if !strings.EqualFold(strings.TrimSpace(n.Cohort), "tournament") {
			continue
		}
		if strings.Contains(n.Model, "grok") {
			t.Fatalf("candidate %q bound to ledger-blocked grok account (model=%q)", n.ID, n.Model)
		}
	}
	// Both candidates must land on the single unblocked provider.
	bound := 0
	for _, n := range def.Nodes {
		if strings.EqualFold(strings.TrimSpace(n.Cohort), "tournament") && strings.Contains(n.Model, "devin") {
			bound++
		}
	}
	if bound != 2 {
		t.Fatalf("expected both candidates on devin, got %d", bound)
	}
}

func tournamentFlowDefinitionMust(t *testing.T) agentpack.FlowDefinition {
	t.Helper()
	def, err := tournamentFlowDefinition()
	if err != nil {
		t.Fatalf("load tournament-harness: %v", err)
	}
	return def
}

// BUG-538 residue class (live run-1651): a stranded leg belonged to a run
// that normalized to failed on restart and never reloaded into s.runs — its
// durable session row kept leg_state=active on the candidate worktree, so
// closeLegsBoundToWorktree (which only scanned memory) left the claim alive
// forever. The sweep must close durable-only active legs on the dir too.
func TestBug538_WorktreeSweepClosesDurableOnlyLegClaim(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyCodex, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), store)
	repo := tournamentE2EGreenRepo(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: repo})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	var mgr tournament.WorktreeManager
	if _, err := mgr.Create(repo, "HEAD", "candidate-a"); err != nil {
		t.Fatalf("seed claimed worktree: %v", err)
	}
	dir := tournament.WorktreePath(repo, "candidate-a")
	// Durable-only residue: failed run row with an active leg on the dir and
	// NO in-memory interactiveRun — exactly what run-1651 left behind.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:            "run-1651-dead",
		ProjectID:        "proj",
		WorkingDirectory: dir,
		Status:           RunStatusFailed,
		AgentStatus:      "failed",
		LegState:         LegStateActive,
		ParentRunID:      parent.RunID,
		FlowCohortID:     "flow-auto-parallel_rollout-attempt-0",
	}); err != nil {
		t.Fatal(err)
	}

	svc.spawnTournamentCandidates(parent.RunID, agentpack.FlowNode{ID: "parallel_rollout"}, []agentpack.FlowNode{
		{ID: "candidate-a", Run: "delegate", Behavior: "agent.delegate", Agent: "agents/coder.md", Cohort: "tournament", Model: "gpt-5.4-mini"},
	}, "solve")

	sess, ok, err := store.GetProviderSession(context.Background(), "run-1651-dead")
	if err != nil || !ok {
		t.Fatalf("GetProviderSession: ok=%v err=%v", ok, err)
	}
	if sess.LegState != LegStateClosed || sess.LegClosedReason != LegClosedReasonWorktreeSwept {
		t.Fatalf("durable-only leg claim survives the sweep: state=%q reason=%q", sess.LegState, sess.LegClosedReason)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, child := range svc.runs {
		if child.parentRunID == parent.RunID && child.label == "candidate-a" {
			return
		}
	}
	t.Fatal("dead-run durable claim wedged the spawn — no candidate child created")
}
