package runner

// BUG-384: saving a Supabase workspace config via PUT /supabase-config persists
// the new credentials but the already-running InteractiveService keeps the
// catalog store captured at boot (CatalogStoreFor resolved once in
// newRunnerCommand). /client/projects therefore keeps returning the stale
// (usually empty/anon) list until the runner is restarted.
//
// Fix: SetCatalogStore atomically swaps the catalog the service reads; the CLI
// save/delete handlers re-resolve CatalogStoreFor(instance) after a successful
// mutation. This test pins the swap semantics on the service seam.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type stubCatalogStore struct {
	projects []Project
}

func (s *stubCatalogStore) ListProjects(context.Context) ([]Project, error) {
	return s.projects, nil
}

func (s *stubCatalogStore) ListWorkflows(context.Context) ([]Workflow, error) {
	return nil, nil
}

func (s *stubCatalogStore) ListSteps(context.Context) ([]Step, error) {
	return nil, nil
}

// TestCatalogSwapReflectedInClientProjects asserts the post-save contract:
// after SetCatalogStore, GET /client/projects serves the NEW store without a
// service restart.
func TestCatalogSwapReflectedInClientProjects(t *testing.T) {
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, newFakeWorkflowStore())

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("pre-swap list status=%d body=%s", st, body)
	}
	var before []Project
	if err := json.Unmarshal(body, &before); err != nil {
		t.Fatalf("decode pre-swap list: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("pre-swap projects len=%d, want 0", len(before))
	}

	svc.SetCatalogStore(&stubCatalogStore{projects: []Project{
		{ID: "proj-live", Name: "Live Project", Path: "/tmp/proj-live"},
	}})

	st, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("post-swap list status=%d body=%s", st, body)
	}
	var after []Project
	if err := json.Unmarshal(body, &after); err != nil {
		t.Fatalf("decode post-swap list: %v", err)
	}
	if len(after) != 1 || after[0].ID != "proj-live" {
		t.Fatalf("post-swap projects=%+v, want exactly proj-live", after)
	}
}

// TestCatalogSwapNilFallsBackToFake pins the reset path: a nil store (config
// deleted) falls back to the offline fake catalog instead of going dark.
func TestCatalogSwapNilFallsBackToFake(t *testing.T) {
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{
		projects: []Project{{ID: "proj-live", Name: "Live Project"}},
	}, newFakeWorkflowStore())

	svc.SetCatalogStore(nil)

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("post-reset list status=%d body=%s", st, body)
	}
	var projects []Project
	if err := json.Unmarshal(body, &projects); err != nil {
		t.Fatalf("decode post-reset list: %v", err)
	}
	if len(projects) == 0 {
		t.Fatalf("post-reset projects empty — nil store must fall back to the fake catalog")
	}
}
