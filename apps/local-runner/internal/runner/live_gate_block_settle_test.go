package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type liveGateBlockTerminalAdapter struct {
	key    ProviderKey
	dir    string
	edited bool
}

func (a *liveGateBlockTerminalAdapter) Key() ProviderKey { return a.key }

func (a *liveGateBlockTerminalAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}

// SendTurn simulates the hub's synthesis turn: production code changes during
// the turn (dirtying the workspace relative to runTurn's own turn-start
// snapshot) with no accompanying change-audit note, then reports completion —
// this is what a real coder edit inside the turn would leave behind for the
// post-turn gate to observe.
func (a *liveGateBlockTerminalAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	_ = os.WriteFile(filepath.Join(a.dir, "main.go"), []byte("package main\n// edited without a change-audit note\n"), 0o644)
	a.edited = true
	// r-ca's "code_changed" trigger keys off WrittenPaths (tool-called edits),
	// not the raw git diff — an EventFileChanged is what actually populates
	// finalizeInput.ChangedFiles, so the rule engine needs this event, not
	// just the on-disk edit above, to recognize the AI touched main.go.
	bridge.Emit(ProviderEvent{Type: EventFileChanged, Path: "main.go", ChangeType: "modified"})
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "changes requested, reinvoking coder"})
	return nil
}

func liveGateBlockRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// TestLiveGateBlockSchedulesSettleDisposition reproduces a real production
// symptom (run-18997 / turn-19161, CP-51 A6 test): a flow-engine-driven hub
// run whose post-turn gate BLOCKS/reprompts (e.g. "declared bug mode but no
// bugfix document found" / r-ca missing change-audit note) left its dispatch
// record stuck at settle_phase=settle_pending forever — confirmed against the
// real dispatch.ndjson, which showed exactly 4 records for that turn
// (prepared/send_claimed/send_started/terminal_completed) and never advanced,
// while a sibling turn in the same project reached every settle phase within
// milliseconds.
//
// Root cause: resumePendingFlowGate's block branch (boot/resume path) calls
// scheduleSettleAfterGateBlock after persisting the reprompt checkpoint, but
// the LIVE post-turn-gate block branch in runTurn's tail never had the
// matching call — only the LIVE gate-PASS branch called
// scheduleSettleAfterGatePass. Any turn whose gate passes settles correctly;
// any turn whose gate blocks/reprompts (exactly what CP-51 A6 exercises)
// never does, live, without a server restart.
func TestLiveGateBlockSchedulesSettleDisposition(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	liveGateBlockRunGit(t, dir, "init")
	liveGateBlockRunGit(t, dir, "config", "user.email", "t@t")
	liveGateBlockRunGit(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	liveGateBlockRunGit(t, dir, "add", "main.go")
	liveGateBlockRunGit(t, dir, "commit", "-m", "init")
	settings := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settings, "flow-rules.json"), []byte(`{"mode":"enforce","rules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store := NewMemoryDispatchStore()
	svc, _ := newTestServer(t)
	svc.dispatchStore = store

	handle, createErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: dir})
	if createErr != nil {
		t.Fatalf("createRun: %v", createErr)
	}
	const turnID = "turn-live-gate-block"
	record := testPrepared(handle.RunID, turnID)
	record.SettleOwed = true
	if err := store.CreatePrepared(ctx, record, testEnvelope(handle.RunID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	_, revision, getErr := store.Get(ctx, handle.RunID, turnID)
	if getErr != nil {
		t.Fatalf("Get prepared: %v", getErr)
	}
	// Only advance to send_started (NOT terminal): runTurn's own
	// linearizeSendStarted must see a non-terminal record to proceed past its
	// own dispatch fence and reach the post-turn gate. Pre-committing all the
	// way to terminal_completed here (as if simulating an already-finished
	// turn) makes linearizeSendStarted's "already terminal" short-circuit
	// silently bail runTurn out before the gate ever runs.
	if _, err := store.CASAdvance(ctx, handle.RunID, turnID, revision, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatalf("send_claimed: %v", err)
	}

	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	rs.currentTurnID = turnID
	rs.turnInFlight = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "running", Round: 1, Cap: 3, Mode: "explicit"})

	adapter := &liveGateBlockTerminalAdapter{key: ProviderKeyCodex, dir: dir}
	svc.runTurn(ctx, rs, adapter, TurnInput{Prompt: "complete"}, "normal", turnID, nil)
	if !adapter.edited {
		t.Fatal("fake adapter never ran — runTurn bailed out before SendTurn")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		settled, _, err := store.Get(ctx, handle.RunID, turnID)
		if err == nil && settled.SettlePhase.IsSettleFinal() {
			// Must specifically be the gate-blocked/reprompt disposition, not a
			// gate-PASS finalize — a passing gate already had the settle call
			// wired (scheduleSettleAfterGatePass) before this fix, so accepting
			// any final phase here would pass regardless of whether the fix for
			// the BLOCK branch is present.
			if settled.SettlePhase != SettleSupersededReprompt {
				t.Fatalf("settle phase = %s, want %s (gate blocked/reprompt) — gate did not actually block in this test setup", settled.SettlePhase, SettleSupersededReprompt)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	settled, _, err := store.Get(ctx, handle.RunID, turnID)
	if err != nil {
		t.Fatalf("Get settled record: %v", err)
	}
	t.Fatalf("settle phase = %s, want %s — stuck at settle_pending like run-18997/turn-19161", settled.SettlePhase, SettleSupersededReprompt)
}
