package runner

// BUG-309: project history under-reports Drive sync status for runs still
// resident in the in-memory s.runs map.
//
// Root cause: projectRunHistory built the in-memory-branch runHistoryItem
// without SyncStatus/SourceMachineID/SourceRunID -- interactiveRun never
// carries those fields at all; they are written straight to the persisted
// store by updateLocalSessionSyncStatus, asynchronously, well after a run's
// turn lifecycle ends. Any run created in the current process's lifetime
// (i.e. still in s.runs) therefore always reported syncStatus="" here, no
// matter how many times it was actually synced to Drive, making the desktop
// "Sync all" chip perpetually re-target already-synced chats.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestProjectHistoryReportsSyncStatusForInMemoryRun reproduces BUG-309.
//
// Expected (pre-fix): the in-memory run's syncStatus comes back "" even
// though the persisted store already has SyncStatus="synced" for it.
// Expected (post-fix): the in-memory run reports the real persisted
// SyncStatus/SourceMachineID/SourceRunID.
func TestProjectHistoryReportsSyncStatusForInMemoryRun(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()

	svc, srv := newTestServerWith(t, registry, catalog, store)

	syncedRunID := startProjectRun(t, srv.URL, "proj-drive", "wf-synced")
	unsyncedRunID := startProjectRun(t, srv.URL, "proj-drive", "wf-unsynced")

	// Both runs are still resident in svc.runs (no restart) -- this is the
	// exact in-memory-branch scenario projectRunHistory mishandled. Seed the
	// persisted store directly (bypassing the real Drive round-trip, which is
	// already covered elsewhere) to simulate "syncedRunID was already synced".
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:           syncedRunID,
		ProjectID:       "proj-drive",
		ProviderKey:     ProviderKeyClaude,
		RunKind:         "chat",
		SourceMachineID: "machine-A",
		SourceRunID:     syncedRunID,
		SyncStatus:      "synced",
	}); err != nil {
		t.Fatalf("seed synced session: %v", err)
	}

	st, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-drive/workflow-runs", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("history status=%d body=%s", st, body)
	}
	var items []runHistoryItem
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("decode history: %v", err)
	}

	byID := map[string]runHistoryItem{}
	for _, it := range items {
		byID[it.RunID] = it
	}

	synced, ok := byID[syncedRunID]
	if !ok {
		t.Fatalf("BUG-309: synced run %q missing from history entirely", syncedRunID)
	}
	if synced.SyncStatus != "synced" {
		t.Errorf("BUG-309: in-memory run reports syncStatus=%q, want \"synced\" (persisted store already has it)", synced.SyncStatus)
	}
	if synced.SourceMachineID != "machine-A" {
		t.Errorf("BUG-309: in-memory run reports sourceMachineId=%q, want \"machine-A\"", synced.SourceMachineID)
	}
	if synced.SourceRunID != syncedRunID {
		t.Errorf("BUG-309: in-memory run reports sourceRunId=%q, want %q", synced.SourceRunID, syncedRunID)
	}

	// A sibling run with no persisted sync record must NOT pick up the other
	// run's fields -- this catches a lookup keyed wrong (e.g. by project
	// instead of by run id) that would cross-contaminate every row.
	unsynced, ok := byID[unsyncedRunID]
	if !ok {
		t.Fatalf("unsynced run %q missing from history", unsyncedRunID)
	}
	if unsynced.SyncStatus != "" {
		t.Errorf("unsynced sibling run reports syncStatus=%q, want \"\" (must not inherit another run's sync state)", unsynced.SyncStatus)
	}

	_ = svc // keep svc referenced for readability/parity with other bug tests
}
