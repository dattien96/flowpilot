package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/tournament"
)

type idleOnlySessionStore struct {
	*fakeWorkflowStore
}

func (s *idleOnlySessionStore) UpsertProviderSession(ctx context.Context, state ProviderSessionState) error {
	if state.Status != RunStatusIdle {
		return nil
	}
	return s.fakeWorkflowStore.UpsertProviderSession(ctx, state)
}

func TestBug523_QuotaRespawnPreservesTournamentWorktreeAndCohort(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyGrok, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	mainCwd := t.TempDir()
	candidateCwd := t.TempDir()
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok, Cwd: mainCwd})
	if aerr != nil {
		t.Fatal(aerr)
	}
	child, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok, Cwd: candidateCwd})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.mu.Lock()
	cr := svc.runs[child.RunID]
	cr.parentRunID = parent.RunID
	cr.label = "candidate-a"
	cr.agentName = "coder"
	cr.flowCohortId = "attempt-1"
	cr.lastFullPrompt = "finish candidate"
	svc.mu.Unlock()

	if err := svc.respawnChildOnRoute(context.Background(), cr, &RouteCandidate{ProviderKey: ProviderKeyGrok, Model: "grok-4.5"}, "finish candidate"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, run := range svc.runs {
		if run.id != child.RunID && run.parentRunID == parent.RunID && run.label == "candidate-a" {
			if run.workspaceCwd != candidateCwd {
				t.Fatalf("replacement cwd=%q want tournament worktree %q", run.workspaceCwd, candidateCwd)
			}
			if run.flowCohortId != "attempt-1" {
				t.Fatalf("replacement cohort=%q want attempt-1", run.flowCohortId)
			}
			return
		}
	}
	t.Fatal("replacement candidate was not spawned")
}

func TestBug526_TournamentEscalationUsesAvailableProviders(t *testing.T) {
	writeProviderAccountsConfig(t, filepath.Join(t.TempDir(), "accounts.json"), []ProviderAccount{
		{ID: "devin-1", ProviderKey: string(ProviderKeyDevin), AuthStatus: "connected"},
		{ID: "grok-1", ProviderKey: string(ProviderKeyGrok), AuthStatus: "connected"},
	})
	reg := newProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyDevin, ProviderKeyGrok} {
		key := key
		reg.register(ProviderRegistration{Key: key, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		}})
	}
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	t.Cleanup(func() { awaitTournamentChildIdle(t, svc, parent.RunID) })
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: "cap", Cap: 3, RoundCap: 3})
	childID, err := svc.escalateToTournament(parent.RunID, "cap")
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	nodes := append([]agentpack.FlowNode(nil), svc.runs[childID].activeFlowNodes...)
	svc.mu.Unlock()
	got := map[ProviderKey]bool{}
	for _, node := range nodes {
		if node.Cohort == "tournament" {
			if key, ok := providerKeyFromModel(node.Model); ok {
				got[key] = true
			}
		}
	}
	if !got[ProviderKeyDevin] || !got[ProviderKeyGrok] {
		t.Fatalf("candidate providers=%v want available devin+grok", got)
	}
	arbiter, ok := findFlowNode(nodes, "tournament_arbiter")
	if !ok {
		t.Fatal("tournament arbiter missing")
	}
	if _, err := tournament.ParseTournamentConfig(tournamentNodeConfig(nodes, arbiter)); err != nil {
		t.Fatalf("available provider config rejected by arbiter: %v", err)
	}
}

// When exactly one provider is connected (e.g. the other account is out of
// quota), both candidates must bind to it rather than silently keeping the
// pack's claude/codex defaults, which can never dispatch on this machine.
func TestBug526_SingleConnectedProviderBindsBothCandidates(t *testing.T) {
	writeProviderAccountsConfig(t, filepath.Join(t.TempDir(), "accounts.json"), []ProviderAccount{
		{ID: "devin-1", ProviderKey: string(ProviderKeyDevin), AuthStatus: "connected"},
	})
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyDevin, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
		return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
			b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
			return nil
		})
	}})
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	t.Cleanup(func() { awaitTournamentChildIdle(t, svc, parent.RunID) })
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: "cap", Cap: 3, RoundCap: 3})
	childID, err := svc.escalateToTournament(parent.RunID, "cap")
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	nodes := append([]agentpack.FlowNode(nil), svc.runs[childID].activeFlowNodes...)
	svc.mu.Unlock()
	count := 0
	for _, node := range nodes {
		if node.Cohort != "tournament" {
			continue
		}
		count++
		if key, ok := providerKeyFromModel(node.Model); !ok || key != ProviderKeyDevin {
			t.Fatalf("candidate node %q model=%q must bind to devin when it is the only connected provider", node.ID, node.Model)
		}
	}
	if count < 2 {
		t.Fatalf("expected 2 tournament candidate nodes, got %d", count)
	}
}

func TestBug527_DurableRunSnapshotIncludesWorktreeProjection(t *testing.T) {
	fws := newFakeWorkflowStore()
	fws.sessions["run-wt"] = ProviderSessionState{
		RunID: "run-wt", ProjectID: "p", Status: RunStatusCompleted,
		WorktreeOwnerID: "chat-1", WorktreePath: "/tmp/wt", WorktreeBranch: "fp/chat-1",
		WorktreeBaseCommit: "abc", WorktreeSlug: "chat-1", WorktreeState: "merge_pending", WorktreeEnabled: true,
	}
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}
	view, err := svc.runSnapshot("run-wt")
	if err != nil {
		t.Fatal(err)
	}
	if view.Worktree == nil || view.Worktree.Path != "/tmp/wt" || view.Worktree.State != "merge_pending" {
		t.Fatalf("durable worktree projection=%+v", view.Worktree)
	}
}

func TestBug528_CorruptQuotaLedgerFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_STATE_FILE", path)
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-q", providerKey: ProviderKeyCodex, providerAccountID: "acct-1"}
	svc.runs[rs.id] = rs
	if got := svc.pinnedAccountHardVeto(context.Background(), rs); got != "quota_state_corrupt" {
		t.Fatalf("corrupt ledger veto=%q want quota_state_corrupt", got)
	}
}

func TestBug529_TournamentEscalationPersistsChildBeforeDispatch(t *testing.T) {
	writeProviderAccountsConfig(t, filepath.Join(t.TempDir(), "accounts.json"), []ProviderAccount{
		{ID: "codex-1", ProviderKey: string(ProviderKeyCodex), AuthStatus: "connected"},
		{ID: "grok-1", ProviderKey: string(ProviderKeyGrok), AuthStatus: "connected"},
	})
	store := &idleOnlySessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	reg := newProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyCodex, ProviderKeyGrok} {
		key := key
		reg.register(ProviderRegistration{Key: key, Status: ProviderStatusAvailable, newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(context.Context, TurnRequest, TurnBridge) error { return nil })
		}})
	}
	svc, _ := newTestServerWith(t, reg, newInteractiveCatalog(), store)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	t.Cleanup(func() { awaitTournamentChildIdle(t, svc, parent.RunID) })
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: "cap", Cap: 3, RoundCap: 3})
	childID, err := svc.escalateToTournament(parent.RunID, "cap")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, found, _ := store.GetProviderSession(context.Background(), childID); !found {
		t.Fatalf("tournament child %q was not durable after rejected async dispatch", childID)
	}
}

func TestBug530_CodexDifferentScopesDoNotTearDownEachOther(t *testing.T) {
	original := commandContextFn
	defer func() { commandContextFn = original }()
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		script := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{}}`) + "sleep 10\n"
		return testShellCommand(ctx, script)
	}
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.CleanupSessions()
	h1, err := r.ensureCodexAppServer(context.Background(), "acct-a", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	h1.dispatcher.mu.Lock()
	h1.dispatcher.threadSubs["active-thread"] = make(chan codexNotification)
	h1.dispatcher.mu.Unlock()
	if _, err := r.ensureCodexAppServer(context.Background(), "acct-b", t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	if h1.dispatcher.isClosed() {
		t.Fatal("starting account B tore down account A's live app-server")
	}
}

// BUG-534 (live run-1269): the quota-route commit closed the child's leg
// BEFORE the replacement spawn was known to succeed — the parent loop was
// blocked on the arbiter card, spawnChildRun refused (BUG-432), and the run
// ended with a committed route + durably closed leg + no successor that
// nothing retried. A refused respawn must leave the leg open so the veto
// refires on the next admission, and emit no route_committed record.
func TestBug534_RefusedRespawnKeepsLegOpen(t *testing.T) {
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
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	child, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok, Cwd: t.TempDir()})
	if aerr != nil {
		t.Fatal(aerr)
	}
	svc.mu.Lock()
	cr := svc.runs[child.RunID]
	cr.parentRunID = parent.RunID
	cr.label = "candidate-a"
	cr.agentName = "coder"
	cr.flowCohortId = "attempt-1"
	cr.lastFullPrompt = "finish candidate"
	svc.mu.Unlock()
	// Parent parked on a decision card — spawnChildRun refuses (BUG-432).
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: "escalate"})

	err := svc.commitQuotaRotation(context.Background(), QuotaResolution{
		Demand:   ExecutionDemand{RunID: child.RunID, RequestedProvider: ProviderKeyGrok},
		Selected: &RouteCandidate{ProviderKey: ProviderKeyDevin, Model: "devin/swe-2-high", AccountID: "devin-1"},
	})
	if err == nil {
		t.Fatal("respawn under a blocked parent loop must surface the refusal")
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if cr.legState == LegStateClosed {
		t.Fatal("refused respawn closed the leg — committed route left no successor")
	}
	for _, e := range cr.events {
		if e.Type == EventQuotaRouteCommitted {
			t.Fatal("route_committed emitted for a respawn that never happened")
		}
	}
	for _, run := range svc.runs {
		if run.id != child.RunID && run.parentRunID == parent.RunID && run.label == "candidate-a" {
			t.Fatalf("unexpected replacement spawned under blocked parent: %q", run.id)
		}
	}
}
