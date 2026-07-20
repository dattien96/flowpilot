package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CP-51 A1 residual durability: continue-delegate suppression must clear
// PendingGateReprompt* on disk (with HubContinueDelegatedTurnID), so a
// crash/restart cannot revive a root hub gate reprompt while the delegated
// writer path is still active (run-9437 class).
//
// Uses reconstructRun (session-store contract) rather than resumeRun, which
// also requires provider-home transcript material and fails with
// session_unavailable when only session metadata is seeded.
func TestRun9437ContinueDelegateSuppressSurvivesRestartAndIdleFlush(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dir := filepath.Join(root, ".flowpilot", "chats")
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	const (
		hubID    = "run-9437-hub-restart"
		coderID  = "run-9437-coder-restart"
		turnID   = "turn-9437-synthesis-continue"
		reprompt = "Flow gate: create missing requirements/09-BugFix/BUG-908.md"
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Crash window: disk still has a root gate reprompt from post-turn gate
	// persist, plus the durable continue-delegate marker.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:                      hubID,
		ProjectID:                  "proj-9437",
		ProviderKey:                ProviderKeyCodex,
		ProviderSessionID:          "sess-hub-9437",
		WorkingDirectory:           cwd,
		Status:                     RunStatusRunning,
		RunKind:                    "chat",
		AutoOrchestrate:            true,
		ActiveFlowNodes:            []agentpack.FlowNode{{ID: "coder"}, {ID: "synthesis"}},
		PendingGateRepromptPrompt:  reprompt,
		PendingGateRepromptStepID:  "synthesis",
		PendingGateRepromptGen:     7,
		HubContinueDelegatedTurnID: turnID,
		// Gate turn identity so reconstruct/flush can match turn-scoped suppress.
		PendingFlowGateTurnID: turnID,
		LastTurnStepID:        "synthesis",
		StepID:                "synthesis",
		StartedAt:             now,
		UpdatedAt:             now,
		LoopState:             AgentLoopState{Status: "running", Round: 1, Cap: 3, Mode: "explicit"},
	}); err != nil {
		t.Fatalf("Upsert hub session: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             coderID,
		ProjectID:         "proj-9437",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "sess-coder-9437",
		ParentRunID:       hubID,
		AgentName:         "coder",
		Label:             "coder",
		Role:              "coder",
		AgentStatus:       string(RunStatusRunning),
		Status:            RunStatusRunning,
		WorkingDirectory:  cwd,
		StartedAt:         now,
		UpdatedAt:         now,
		RunKind:           "chat",
	}); err != nil {
		t.Fatalf("Upsert coder session: %v", err)
	}

	// Prove crash-window shape is on disk before any reconstruct/flush.
	pre, ok, getErr := store.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("pre-restart GetProviderSession: ok=%v err=%v", ok, getErr)
	}
	if pre.PendingGateRepromptPrompt != reprompt || pre.HubContinueDelegatedTurnID != turnID {
		t.Fatalf("pre-restart disk shape wrong: prompt=%q marker=%q",
			pre.PendingGateRepromptPrompt, pre.HubContinueDelegatedTurnID)
	}

	// Restart: new process loads store from disk and reconstructs the hub.
	// reconstructRun calls flushDurableTurnIntents, which must durable-suppress
	// the forbidden root reprompt under HubContinueDelegatedTurnID.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	restarted := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store2)
	st, ok, getErr := store2.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("GetProviderSession hub after reload: ok=%v err=%v", ok, getErr)
	}
	if st.PendingGateRepromptPrompt != reprompt {
		t.Fatalf("disk lost PendingGateRepromptPrompt before reconstruct: %q", st.PendingGateRepromptPrompt)
	}
	if st.HubContinueDelegatedTurnID != turnID {
		t.Fatalf("disk lost HubContinueDelegatedTurnID before reconstruct: %q", st.HubContinueDelegatedTurnID)
	}

	// Keep delegated writer active for park acceptance after reconstruct.
	// Register child before reconstruct so any concurrent flush sees park.
	restarted.mu.Lock()
	restarted.runs[coderID] = &interactiveRun{
		id:           coderID,
		parentRunID:  hubID,
		label:        "coder",
		role:         "coder",
		status:       RunStatusRunning,
		turnInFlight: true,
		subs:         map[int64]chan ProviderEvent{},
	}
	restarted.mu.Unlock()
	restarted.agentOrchestrator.registerChild(hubID, coderID)
	restarted.agentOrchestrator.mutateLoop(hubID, func(loop AgentLoopState) AgentLoopState {
		loop.Status = "running"
		loop.Cap = 3
		return loop
	})

	hubRS, aerr := restarted.reconstructRun(st)
	if aerr != nil {
		t.Fatalf("reconstructRun hub: code=%s msg=%s", aerr.code, aerr.msg)
	}
	// reconstructRun flushes durable intents: same-turn suppress clears the
	// forbidden reprompt and consumes the marker. Follow-up flush must not
	// start a hub turn either.
	_ = hubRS
	restarted.flushDurableTurnIntents(hubID)
	time.Sleep(80 * time.Millisecond)

	restarted.mu.Lock()
	hub := restarted.runs[hubID]
	if hub.turnInFlight {
		restarted.mu.Unlock()
		t.Fatal("hub turnInFlight after flush: continue-delegate must not auto-reprompt hub")
	}
	if hub.pendingGateRepromptPrompt != "" {
		restarted.mu.Unlock()
		t.Fatalf("pendingGateRepromptPrompt still in RAM after suppress: %q", hub.pendingGateRepromptPrompt)
	}
	// Same-turn suppress consumes the marker so N+1 gates are not blocked.
	if hub.hubContinueDelegatedTurnID != "" {
		restarted.mu.Unlock()
		t.Fatalf("marker still set after same-turn suppress consume: %q", hub.hubContinueDelegatedTurnID)
	}
	restarted.mu.Unlock()

	loaded, ok, getErr := store2.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("GetProviderSession hub after suppress: ok=%v err=%v", ok, getErr)
	}
	if loaded.PendingGateRepromptPrompt != "" || loaded.PendingGateRepromptStepID != "" || loaded.PendingGateRepromptGen != 0 {
		t.Fatalf("disk still has PendingGateReprompt* after suppress: prompt=%q step=%q gen=%d",
			loaded.PendingGateRepromptPrompt, loaded.PendingGateRepromptStepID, loaded.PendingGateRepromptGen)
	}
	if loaded.HubContinueDelegatedTurnID != "" {
		t.Fatalf("disk marker not consumed after suppress: %q", loaded.HubContinueDelegatedTurnID)
	}

	// Second process restart must still not revive the reprompt.
	store3, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("second reload store: %v", err)
	}
	restarted2 := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store3)
	st2, ok, getErr := store3.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("second GetProviderSession: ok=%v err=%v", ok, getErr)
	}
	if st2.PendingGateRepromptPrompt != "" {
		t.Fatalf("second load revived PendingGateRepromptPrompt: %q", st2.PendingGateRepromptPrompt)
	}
	if st2.HubContinueDelegatedTurnID != "" {
		t.Fatalf("second load revived marker: %q", st2.HubContinueDelegatedTurnID)
	}
	hub2, aerr := restarted2.reconstructRun(st2)
	if aerr != nil {
		t.Fatalf("second reconstructRun: code=%s msg=%s", aerr.code, aerr.msg)
	}
	if hub2.pendingGateRepromptPrompt != "" {
		t.Fatalf("second reconstruct revived pendingGateRepromptPrompt: %q", hub2.pendingGateRepromptPrompt)
	}

	restarted2.flushDurableTurnIntents(hubID)
	time.Sleep(50 * time.Millisecond)
	restarted2.mu.Lock()
	if restarted2.runs[hubID].turnInFlight {
		restarted2.mu.Unlock()
		t.Fatal("second flush started hub turn after durable suppress")
	}
	restarted2.mu.Unlock()
}

func TestRun9437StampContinueDelegateClearsAndPersistsReprompt(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	const hubID = "run-9437-stamp"
	const turnID = "turn-stamp-1"

	svc.mu.Lock()
	svc.runs[hubID] = &interactiveRun{
		id:                        hubID,
		flowEngineDriven:          true,
		status:                    RunStatusRunning,
		currentTurnID:             turnID,
		pendingGateRepromptPrompt: "create missing BUG doc",
		pendingGateRepromptStepID: "synthesis",
		pendingGateRepromptGen:    2,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(hubID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	svc.stampHubContinueDelegatedDurable(hubID)

	svc.mu.Lock()
	rs := svc.runs[hubID]
	if rs.hubContinueDelegatedTurnID != turnID {
		svc.mu.Unlock()
		t.Fatalf("marker = %q, want %q", rs.hubContinueDelegatedTurnID, turnID)
	}
	if rs.pendingGateRepromptPrompt != "" {
		svc.mu.Unlock()
		t.Fatalf("RAM reprompt not cleared: %q", rs.pendingGateRepromptPrompt)
	}
	svc.mu.Unlock()

	loaded, ok, getErr := store.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("GetProviderSession: ok=%v err=%v", ok, getErr)
	}
	if loaded.HubContinueDelegatedTurnID != turnID {
		t.Fatalf("disk marker = %q", loaded.HubContinueDelegatedTurnID)
	}
	if loaded.PendingGateRepromptPrompt != "" || loaded.PendingGateRepromptGen != 0 {
		t.Fatalf("disk still has reprompt after stamp: prompt=%q gen=%d", loaded.PendingGateRepromptPrompt, loaded.PendingGateRepromptGen)
	}
}
