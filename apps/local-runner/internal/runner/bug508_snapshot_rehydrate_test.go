package runner

// BUG-508 (live 2026-09-26, build 8c95a5bb): after a runner restart,
// GET /client/workflow-runs/{runId} returns 404 `run_not_found` for runs
// whose durable session rows still exist and still appear in project
// history. runSnapshot reads only the in-memory s.runs map; runs that the
// boot reconciler does not rehydrate (terminal/completed rows) are invisible
// to the endpoint even though their durable record is the SSOT.
// Observed live: run-6010 (task-harness, completed), run-980, run-3688 all
// returned run_not_found while /client/projects/{id}/workflow-runs listed
// them; meanwhile run-22230 (cancelled) DID resolve — an asymmetric index
// the operator cannot reason about.
//
// Fix: on an s.runs miss, runSnapshot falls back to the durable session
// store (SessionHistoryReader.GetProviderSession) and projects a read-only
// view from the persisted row — status, provider, session id, workspace.
// A run with no durable row still 404s (fail-closed).

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBug508_SnapshotFallsBackToDurableSession(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	// Persist a terminal session row the way a completed run leaves it, then
	// simulate a post-restart process where s.runs was never repopulated.
	store.sessions["run-6010"] = ProviderSessionState{
		RunID:             "run-6010",
		ProjectID:         "proj-bed",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: "01a0d-SESSION",
		WorkingDirectory:  t.TempDir(),
		Status:            RunStatusCompleted,
	}

	svc.mu.Lock()
	_, inMemory := svc.runs["run-6010"]
	svc.mu.Unlock()
	if inMemory {
		t.Fatal("precondition: run-6010 must not be in s.runs")
	}

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/run-6010", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("durable-backed run must resolve, got %d body=%s", st, body)
	}
	var view struct {
		Status      string `json:"status"`
		ProviderKey string `json:"providerKey"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != "completed" || view.ProviderKey != string(ProviderKeyGrok) {
		t.Fatalf("view must project durable row, got %+v", view)
	}
}

func TestBug508_UnknownRunStill404s(t *testing.T) {
	store := newFakeWorkflowStore()
	_, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/run-does-not-exist", nil, nil)
	if st != http.StatusNotFound {
		t.Fatalf("a run with no durable row must still 404, got %d body=%s", st, body)
	}
}

func TestBug508_InMemoryRunStillWins(t *testing.T) {
	// The in-memory view is authoritative for live runs — the durable fallback
	// must never override a resident run's (richer, fresher) state.
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	h, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	// A stale durable row claiming a different status must not shadow the
	// live in-memory view.
	store.sessions[h.RunID] = ProviderSessionState{RunID: h.RunID, Status: RunStatusCompleted, ProviderKey: ProviderKeyGrok}

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/"+h.RunID, nil, nil)
	if st != http.StatusOK {
		t.Fatalf("GET: %d body=%s", st, body)
	}
	var view struct {
		Status      string `json:"status"`
		ProviderKey string `json:"providerKey"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status == "completed" || view.ProviderKey == string(ProviderKeyGrok) {
		t.Fatalf("in-memory state must win over stale durable row, got %+v", view)
	}
}
