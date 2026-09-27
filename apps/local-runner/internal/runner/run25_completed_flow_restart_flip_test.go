package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-25 (live CP-64 drill, 2026-09-26): a sealed flow run persisted
// `status=completed, loop=done` at 14:05:20.407 — then applyFlowControl's
// "Flow completed." note ran appendPendingAgentContextLocked, whose async
// persist carried sessionStateOf(parent) with NO LoopState attached
// (sessionStateOf never carries it; the orchestrator owns the loop). The
// goroutine landed AFTER the seal row, so last-wins replay exposed
// `completed + loop=None + pending_agent_context=[note]` — and
// resumedFlowRunIncomplete classified the run "incomplete", flipping it to
// cancelled in RAM (14:10:02 row: cancelled + blocked/hub_stalled after the
// stall watchdog persisted it). A completed run must survive kill/restart.

func newRun25Service(t *testing.T, store *localFileSessionStore) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), store)
}

// TestRun25_AppendPendingContextPreservesTerminalLoopOnDisk reproduces the
// durable leak itself: a flow run persisted with loop=done, then a plain
// pending-context append, must leave the last-wins session row carrying the
// terminal loop — not a loop=None overwrite.
func TestRun25_AppendPendingContextPreservesTerminalLoopOnDisk(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newRun25Service(t, store)

	run, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("createRun: %s", aerr.msg)
	}
	nodes := []agentpack.FlowNode{
		{ID: "scout", Behavior: "agent.delegate", Agent: "agents/coder.md"},
	}
	// Post-seal shape: run completed, loop done — the state run-25 was in at
	// 14:05:20.407 when the seal path persisted it.
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.status = RunStatusCompleted
	rs.activeFlowNodes = nodes
	rs.autoOrchestrate = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Mode: "explicit", Round: 2, Cap: 3, RoundCap: 3})

	// Row 1 (the seal write): carries loop=done.
	if err := svc.persistProviderSession(svc.snapshotWithLoop(rs)); err != nil {
		t.Fatalf("seal persist: %v", err)
	}
	// Row 2 (the leak): applyFlowControl("done") appends the completion note —
	// the exact call that wrote the loop=None row at 14:05:20.338-order-last.
	svc.appendPendingAgentContext(run.RunID, "Flow completed. The merge phase is complete.")

	// Wait for the async persist goroutine to land the note row.
	var sess ProviderSessionState
	var ok bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sess, ok, err = store.GetProviderSession(context.Background(), run.RunID)
		if err != nil {
			t.Fatalf("GetProviderSession: %v", err)
		}
		if ok && len(sess.PendingAgentContext) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok || len(sess.PendingAgentContext) == 0 {
		t.Fatalf("note row never landed (sess=%+v)", sess)
	}
	if sess.LoopState.Status != "done" {
		t.Fatalf("last-wins session row lost the terminal loop: LoopState=%+v (BUG run-25: reconstruct reads loop=None → cancelled)", sess.LoopState)
	}
}

// TestRun25_RestartKeepsFlowRunCompleted drives the second half of the chain:
// the durable row a restart reconstructs from must keep the run `completed`,
// not flip to `cancelled`.
func TestRun25_RestartKeepsFlowRunCompleted(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newRun25Service(t, store)

	run, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("createRun: %s", aerr.msg)
	}
	nodes := []agentpack.FlowNode{
		{ID: "scout", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", DependsOn: []string{"scout"}},
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.status = RunStatusCompleted
	rs.activeFlowNodes = nodes
	rs.autoOrchestrate = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Mode: "explicit", Round: 2, Cap: 3, RoundCap: 3})

	if err := svc.persistProviderSession(svc.snapshotWithLoop(rs)); err != nil {
		t.Fatalf("seal persist: %v", err)
	}
	svc.appendPendingAgentContext(run.RunID, "Flow completed. The merge phase is complete.")

	var sess ProviderSessionState
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var ok bool
		sess, ok, err = store.GetProviderSession(context.Background(), run.RunID)
		if err != nil {
			t.Fatalf("GetProviderSession: %v", err)
		}
		if ok && len(sess.PendingAgentContext) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(sess.PendingAgentContext) == 0 {
		t.Fatalf("note row never landed (sess=%+v)", sess)
	}

	// Simulate restart: a fresh service reconstructs strictly from the
	// last-wins durable row.
	svc2 := newRun25Service(t, store)
	recon, apiErr := svc2.reconstructRun(sess)
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if recon.status == RunStatusCancelled {
		t.Fatalf("sealed flow run flipped to cancelled on restart (run-25); durable row: %+v", sess.LoopState)
	}
	if recon.status != RunStatusCompleted {
		t.Fatalf("sealed flow run must stay completed, got %q", recon.status)
	}
}
