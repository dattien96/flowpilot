package runner

import (
	"context"
	"testing"
)

// CP-51 Task-254 (DOD-I6/Rr4). TestSnapshot_ActiveLowGenKeySurvives48TerminalKeys
// and TestSnapshot_PerNamespaceCap_NoCrossEviction already live in
// cp51_tasks_test.go; these two close the remaining §4.2 skeletons: the
// POST-TURN snapshot path (not just accept-time) and a real disk round-trip.

// TestSnapshot_PostTurnAlsoPinsActiveKey proves the fix at sessionStateOf
// (interactive_service.go) — every snapshot pins active (non-terminal) keys via
// nonTerminalIdemKeys() derived from rs.dispatch, not just the accept-time
// sessionStateOfProtectingIdem path. Before this was wired, the post-turn
// snapshot used plain durableIdempotencySnapshot with no non-terminal set, so a
// low-generation active key could be evicted by 48+ higher-generation terminal
// keys once the run had been through many resumes.
func TestSnapshot_PostTurnAlsoPinsActiveKey(t *testing.T) {
	rs := &interactiveRun{
		id:          "r1",
		idempotency: map[string]string{},
		dispatch:    map[string]*DispatchRecord{},
	}
	// 60 terminal (completed) keys in a high-generation namespace.
	for i := 1; i <= 60; i++ {
		key := "durable-r1-resume-" + itoa64(int64(i+100))
		rs.idempotency[key] = "turn-terminal-" + itoa64(int64(i))
	}
	// One active low-gen key in a DIFFERENT namespace, backed by a non-terminal
	// DispatchRecord — exactly what nonTerminalIdemKeys() reads.
	activeKey := "durable-r1-restart-1"
	rs.idempotency[activeKey] = "turn-active"
	rs.dispatch["turn-active"] = &DispatchRecord{
		TurnID:         "turn-active",
		State:          DispatchSendClaimed,
		OuterIntentKey: activeKey,
	}

	// This is the exact function the post-turn path calls (interactive_service.go
	// sessionStateOf), not a hand-rolled equivalent.
	snap := sessionStateOf(rs)

	if snap.IdempotencyKeys[activeKey] != "turn-active" {
		t.Fatalf("post-turn sessionStateOf dropped the active key: %#v", snap.IdempotencyKeys)
	}
}

// TestSnapshot_RetainedKeysReloadAndShortCircuit proves the retained active key
// survives a REAL disk round-trip (local file session store) and, after
// reconstruction, still short-circuits startTurn's duplicate-turn replay for
// the same durable idempotency key.
func TestSnapshot_RetainedKeysReloadAndShortCircuit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	rs := &interactiveRun{
		id:          "r1",
		idempotency: map[string]string{},
		dispatch:    map[string]*DispatchRecord{},
	}
	for i := 1; i <= 60; i++ {
		key := "durable-r1-resume-" + itoa64(int64(i+100))
		rs.idempotency[key] = "turn-terminal-" + itoa64(int64(i))
	}
	activeKey := "durable-r1-restart-1"
	rs.idempotency[activeKey] = "turn-active"
	rs.dispatch["turn-active"] = &DispatchRecord{
		TurnID:         "turn-active",
		State:          DispatchSendClaimed,
		OuterIntentKey: activeKey,
	}
	rs.providerKey = ProviderKeyCodex
	// Durable recovery evidence that DOES survive disk round-trip (unlike
	// rs.lastTurnID, which is RAM-only) — a pending flow-gate settle for this
	// exact turnID is exactly the recorded evidence durableIdemReplaySafe checks
	// (rs.pendingFlowGateSettle && rs.pendingFlowGateTurnID == turnID).
	rs.pendingFlowGateSettle = true
	rs.pendingFlowGateTurnID = "turn-active"

	svcForAccount := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	snap := sessionStateOf(rs)
	snap.RunID = "r1"
	snap.ProjectID = "p"
	snap.ProviderKey = ProviderKeyCodex
	// Match whatever this service resolves as the active account for Codex, so
	// startTurn's cross-account guard is a no-op here — this test is about
	// idempotency-key retention/replay, not account-switch handling.
	snap.ProviderAccountID = svcForAccount.activeAccountForProvider(ProviderKeyCodex)
	if err := store.UpsertProviderSession(context.Background(), snap); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Fresh store instance — a real reload from disk, not the in-RAM struct.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	loaded, found, err := store2.GetProviderSession(context.Background(), "r1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession: found=%v err=%v", found, err)
	}
	if loaded.IdempotencyKeys[activeKey] != "turn-active" {
		t.Fatalf("active key did not survive disk round-trip: %#v", loaded.IdempotencyKeys)
	}

	// Reconstruct into RAM and confirm the same durable key short-circuits to
	// the SAME prior turnID (no duplicate turn) using durable recovery evidence
	// that actually survives the round-trip (PendingFlowGateSettle/TurnID —
	// dispatch records themselves are deliberately never persisted, SD-24 `SB`).
	svc := svcForAccount
	reconstructed, apiErr := svc.reconstructRun(loaded)
	if apiErr != nil || reconstructed == nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[reconstructed.id] = reconstructed
	svc.mu.Unlock()

	tid, apiErr := svc.startTurn(reconstructed.id, TurnInput{StepID: "s1", Prompt: "x"}, "", activeKey)
	if apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	if tid != "turn-active" {
		t.Fatalf("reloaded active key did not short-circuit to the prior turn: got %q want turn-active", tid)
	}
}
