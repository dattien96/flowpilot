package runner

// CP live-test R.2 item 5: after a runner restart, mutation endpoints answered
// `run_not_found` for a run whose durable session row still exists — the
// BUG-508 read fallback covered GET, but continue / flow-control /
// gate-decision read only the in-memory s.runs map. Live evidence: run-1 was
// durably `blocked` pre-restart and `POST agent-loop/continue` 404'd.
//
// Contract: on an s.runs miss the endpoint resolves the run through
// loadPersistedRun (durable row → reconstructRun); a genuinely absent row
// still 404s and an unreadable store returns workflow_state_unavailable —
// never a misleading not-found.

import (
	"net/http"
	"testing"
)

func seedBlockedDurableRun(t *testing.T, store *fakeWorkflowStore, runID string) {
	t.Helper()
	store.sessions[runID] = ProviderSessionState{
		RunID:            runID,
		ProjectID:        "proj-bed",
		ProviderKey:      ProviderKeyCodex,
		WorkingDirectory: t.TempDir(),
		RunKind:          "chat",
		Status:           RunStatusRunning,
		AgentStatus:      string(RunStatusRunning),
		LoopState:        AgentLoopState{Status: "blocked", Mode: "explicit", BlockReason: "cap", Cap: 1, RoundCap: 1, Round: 1, ExtendBy: 1},
	}
}

// POST continue on a restart-orphaned but durably-blocked run must unblock the
// loop instead of 404ing.
func TestContinueResolvesDurablyBlockedRunPostRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)
	seedBlockedDurableRun(t, store, "run-durblocked")

	svc.mu.Lock()
	_, inMemory := svc.runs["run-durblocked"]
	svc.mu.Unlock()
	if inMemory {
		t.Fatal("precondition: run must not be resident")
	}

	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/run-durblocked/agent-loop/continue",
		map[string]any{"feedback": ""}, nil)
	if st != http.StatusOK {
		t.Fatalf("continue on durable-blocked run = %d body=%s, want 200 (run exists durably)", st, body)
	}
	if got := svc.agentOrchestrator.loopStateFor("run-durblocked").Status; got == "blocked" || got == "" {
		t.Fatalf("loop state = %q, want unblocked after continue", got)
	}
}

// POST flow-control on a durable-but-not-resident run must reach domain logic,
// not run_not_found.
func TestFlowControlResolvesDurableRunPostRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)
	seedBlockedDurableRun(t, store, "run-durfc")

	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/run-durfc/flow-control",
		map[string]any{"status": "continue"}, nil)
	if st == http.StatusNotFound {
		t.Fatalf("flow-control on durable run = 404 body=%s — run exists in the session store", body)
	}
	svc.mu.Lock()
	resident := svc.runs["run-durfc"] != nil
	svc.mu.Unlock()
	if !resident {
		t.Fatal("durable run was not made resident by the mutation")
	}
}

// POST gate-decision on a durable-but-not-resident run must resolve the run —
// the domain answer may be a gate-state error, but never run_not_found.
func TestGateDecisionResolvesDurableRunPostRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	_, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)
	seedBlockedDurableRun(t, store, "run-durgate")

	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/run-durgate/gate-decision",
		map[string]any{"option": "ok"}, nil)
	if st == http.StatusNotFound {
		t.Fatalf("gate-decision on durable run = 404 body=%s — run exists in the session store", body)
	}
}

// Fail-closed contract kept: a run with NO durable row still 404s on mutation.
func TestMutationUnknownRunStill404s(t *testing.T) {
	store := newFakeWorkflowStore()
	_, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/run-ghost/agent-loop/continue",
		map[string]any{"feedback": ""}, nil)
	if st != http.StatusNotFound {
		t.Fatalf("continue on unknown run = %d body=%s, want 404", st, body)
	}
	st, body = doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/run-ghost/flow-control",
		map[string]any{"status": "continue"}, nil)
	if st != http.StatusNotFound {
		t.Fatalf("flow-control on unknown run = %d body=%s, want 404", st, body)
	}
}
