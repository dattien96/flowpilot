package runner

// BUG-060: run history empties after InteractiveService recreation.
//
// Root cause: projectRunHistory reads only the in-memory s.runs map, which is
// initialized empty on every NewInteractiveService() call and never rehydrated
// from the persisted session store. Any runner/app-server restart therefore
// drops all history even though sessions were persisted.
//
// This test confirms the bug by simulating a runner restart (new service, same
// persisted store). It is expected to FAIL until F-1/F-2 in BUG-060 are
// implemented (rehydration from the persisted store).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServerWith(t *testing.T, registry *ProviderRegistry, catalog CatalogStore, workflowStore WorkflowStore) (*InteractiveService, *httptest.Server) {
	t.Helper()
	svc := newInteractiveService(registry, catalog, workflowStore)
	fakeAdapterDelay = 0
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

// TestRunHistoryEmptiesAfterServiceRecreation reproduces BUG-060.
//
// Expected (pre-fix): GET history after recreation returns 0 items — the bug.
// Expected (post-fix): GET history after recreation returns 2 items.
func TestRunHistoryEmptiesAfterServiceRecreation(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()

	// --- Phase 1: start two runs on the original service instance. ---
	_, srv1 := newTestServerWith(t, registry, catalog, store)

	startProjectRun(t, srv1.URL, "proj-web", "wf-feature")
	startProjectRun(t, srv1.URL, "proj-web", "wf-other")

	// Confirm both runs appear in history on the original service.
	st, body := doJSON(t, "GET", srv1.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("pre-restart history status=%d body=%s", st, body)
	}
	var before []runHistoryItem
	if err := json.Unmarshal(body, &before); err != nil {
		t.Fatalf("decode pre-restart history: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("pre-restart history len=%d, want 2 — test setup broken", len(before))
	}

	// --- Phase 2: simulate a runner / app-server restart by creating a new
	// InteractiveService backed by the SAME persisted store (workflowStore).
	// This is the scenario that occurs when the Codex app-server is recreated
	// on active-account change, or when the runner process restarts during dev. ---
	_, srv2 := newTestServerWith(t, registry, catalog, store)

	st, body = doJSON(t, "GET", srv2.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("post-restart history status=%d body=%s", st, body)
	}
	var after []runHistoryItem
	if err := json.Unmarshal(body, &after); err != nil {
		t.Fatalf("decode post-restart history: %v", err)
	}

	// BUG-060: this assertion currently fails (after == 0 because s.runs is
	// empty on the new service). Once F-1/F-2 are implemented, it must pass.
	if len(after) != 2 {
		t.Errorf("BUG-060: post-restart history len=%d, want 2 (history lost after service recreation)", len(after))
	}
	for _, item := range after {
		if item.ProjectID != "proj-web" {
			t.Errorf("BUG-060: post-restart history includes wrong project: %+v", item)
		}
	}
}
