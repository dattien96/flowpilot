package runner

import (
	"context"
	"testing"
)

// CP-59 Q-4 / TUI grouping: history must carry chatId/legSeq for both live
// and persisted rows, otherwise the TUI picker would list each leg as its own
// chat after a restart (the bug seen on cht_a8d253c2fe6f).
// These tests lock the persisted echo that the TUI grouping depends on.

// TestProjectHistoryLiveBranchCarriesChatId verifies the in-memory branch of
// projectRunHistory echoes ChatID/LegSeq (live runs before any restart).
func TestProjectHistoryLiveBranchCarriesChatId(t *testing.T) {
	store := newFakeWorkflowStore()
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	_ = store // keep reference

	// Seed two live runs of the same chat (simulating a switch).
	svc.mu.Lock()
	svc.runs["run-1"] = &interactiveRun{
		id:        "run-1",
		projectID: "proj-chat",
		runKind:   "chat",
		chatID:    "cht_a8d253c2fe6f",
		legSeq:    0,
		providerKey: ProviderKeyOpencode,
		status:    RunStatusCompleted,
		createdAt: "2026-08-31T00:00:00Z",
		updatedAt: "2026-08-31T00:00:00Z",
	}
	svc.runs["run-2"] = &interactiveRun{
		id:        "run-2",
		projectID: "proj-chat",
		runKind:   "chat",
		chatID:    "cht_a8d253c2fe6f",
		legSeq:    1,
		providerKey: ProviderKeyGrok,
		status:    RunStatusCompleted,
		createdAt: "2026-08-31T00:01:00Z",
		updatedAt: "2026-08-31T00:01:00Z",
	}
	svc.runs["run-wf"] = &interactiveRun{
		id:        "run-wf",
		projectID: "proj-chat",
		runKind:   "workflow",
		providerKey: ProviderKeyCodex,
		status:    RunStatusCompleted,
		createdAt: "2026-08-31T00:02:00Z",
		updatedAt: "2026-08-31T00:02:00Z",
	}
	svc.mu.Unlock()

	history := svc.projectRunHistory("proj-chat")
	byID := map[string]runHistoryItem{}
	for _, it := range history {
		byID[it.RunID] = it
	}

	// Both legs must carry the same chatId with distinct legSeq.
	for _, runID := range []string{"run-1", "run-2"} {
		it, ok := byID[runID]
		if !ok {
			t.Fatalf("live history missing %q", runID)
		}
		if it.ChatID != "cht_a8d253c2fe6f" {
			t.Fatalf("%q ChatID=%q want cht_a8d253c2fe6f", runID, it.ChatID)
		}
	}
	if byID["run-1"].LegSeq != 0 || byID["run-2"].LegSeq != 1 {
		t.Fatalf("legSeq mismatch: run-1=%d run-2=%d", byID["run-1"].LegSeq, byID["run-2"].LegSeq)
	}
	// Workflow run must not gain a chatId.
	if wf := byID["run-wf"]; wf.ChatID != "" {
		t.Fatalf("workflow run should have empty ChatID, got %q", wf.ChatID)
	}
}

// TestProjectHistoryPersistedBranchCarriesChatId verifies the persisted-only
// branch (BUG-060 F-1) echoes ChatID/LegSeq. This is the restart gap that broke
// grouping before this CP: the store held chatId but projectRunHistory dropped it.
func TestProjectHistoryPersistedBranchCarriesChatId(t *testing.T) {
	store := newFakeWorkflowStore()
	// Persist two legs directly to the store (no live s.runs entry — simulates
	// a fresh process after restart where the in-memory map is empty).
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-1",
		ProjectID:   "proj-chat",
		ProviderKey: ProviderKeyOpencode,
		RunKind:     "chat",
		ChatID:      "cht_a8d253c2fe6f",
		LegSeq:      0,
		StartedAt:   "2026-08-31T00:00:00Z",
		UpdatedAt:   "2026-08-31T00:00:00Z",
		Status:      RunStatusCompleted,
	}); err != nil {
		t.Fatalf("seed run-1: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-2",
		ProjectID:   "proj-chat",
		ProviderKey: ProviderKeyGrok,
		RunKind:     "chat",
		ChatID:      "cht_a8d253c2fe6f",
		LegSeq:      1,
		StartedAt:   "2026-08-31T00:01:00Z",
		UpdatedAt:   "2026-08-31T00:01:00Z",
		Status:      RunStatusCompleted,
	}); err != nil {
		t.Fatalf("seed run-2: %v", err)
	}
	// Also persist a legacy untagged row (pre-CP-59) — must stay empty.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-legacy",
		ProjectID:   "proj-chat",
		ProviderKey: ProviderKeyClaude,
		RunKind:     "chat",
		StartedAt:   "2026-08-31T00:02:00Z",
		UpdatedAt:   "2026-08-31T00:02:00Z",
		Status:      RunStatusCompleted,
	}); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	svc2 := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	history := svc2.projectRunHistory("proj-chat")
	byID := map[string]runHistoryItem{}
	for _, it := range history {
		byID[it.RunID] = it
	}

	for _, runID := range []string{"run-1", "run-2"} {
		it, ok := byID[runID]
		if !ok {
			t.Fatalf("persisted history missing %q", runID)
		}
		if it.ChatID != "cht_a8d253c2fe6f" {
			t.Fatalf("persisted %q ChatID=%q want cht_a8d253c2fe6f", runID, it.ChatID)
		}
	}
	if byID["run-1"].LegSeq != 0 || byID["run-2"].LegSeq != 1 {
		t.Fatalf("persisted legSeq mismatch: run-1=%d run-2=%d", byID["run-1"].LegSeq, byID["run-2"].LegSeq)
	}
	if leg := byID["run-legacy"]; leg.ChatID != "" {
		t.Fatalf("legacy row should have empty ChatID, got %q", leg.ChatID)
	}
}

// TestLocalFileSessionStoreRoundTripsChatId verifies the file store's NDJSON
// record conversion preserves ChatID/LegSeq across a simulated restart.
func TestLocalFileSessionStoreRoundTripsChatId(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-1",
		ProjectID:   "proj-file",
		ProviderKey: ProviderKeyOpencode,
		RunKind:     "chat",
		ChatID:      "cht_file",
		LegSeq:      2,
		LegState:    LegStateActive,
		StartedAt:   "2026-08-31T00:00:00Z",
		UpdatedAt:   "2026-08-31T00:01:00Z",
		Status:      RunStatusCompleted,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Simulate restart: new store reading the same file.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("new store2: %v", err)
	}
	sessions, err := store2.ListProviderSessionsByProject(context.Background(), "proj-file")
	if err != nil {
		t.Fatalf("list after restart: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d want 1", len(sessions))
	}
	if sessions[0].ChatID != "cht_file" || sessions[0].LegSeq != 2 || sessions[0].LegState != LegStateActive {
		t.Fatalf("round-trip chat fields lost: %+v", sessions[0])
	}
}
