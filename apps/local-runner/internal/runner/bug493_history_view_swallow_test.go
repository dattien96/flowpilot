package runner

// BUG-493: history/timeline endpoints swallowed session-store read errors —
// chat timeline omitted persisted legs, run timeline skipped the persisted
// fallback, and projectRunHistory silently dropped every non-resident run
// (and all syncStatus fields). Clients received a 200 with a wrong-but-
// plausible view. Contract (BUG-475 precedent): an unreadable authority
// returns a typed error, not a partial view.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

var errBug493StoreFault = errors.New("session store unreadable")

type bug493Store struct {
	*fakeWorkflowStore
	failByChat    bool
	failByProject bool
	failGet       bool
}

func (s *bug493Store) ListProviderSessionsByChat(ctx context.Context, chatID string) ([]ProviderSessionState, error) {
	if s.failByChat {
		return nil, errBug493StoreFault
	}
	return s.fakeWorkflowStore.ListProviderSessionsByChat(ctx, chatID)
}

func (s *bug493Store) ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error) {
	if s.failByProject {
		return nil, errBug493StoreFault
	}
	return s.fakeWorkflowStore.ListProviderSessionsByProject(ctx, projectID)
}

func (s *bug493Store) GetProviderSession(ctx context.Context, runID string) (ProviderSessionState, bool, error) {
	if s.failGet {
		return ProviderSessionState{}, false, errBug493StoreFault
	}
	return s.fakeWorkflowStore.GetProviderSession(ctx, runID)
}

func TestBUG493_ProjectRunHistoryStoreErrorPropagates(t *testing.T) {
	fws := newFakeWorkflowStore()
	fws.sessions["run-persisted"] = ProviderSessionState{RunID: "run-persisted", ProjectID: "p1", RunKind: "chat"}
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{},
		workflowStore: &bug493Store{fakeWorkflowStore: fws, failByProject: true},
	}
	_, err := svc.projectRunHistory("p1")
	if err == nil {
		t.Fatal("history built on an unreadable store must surface an error, not silently drop persisted runs")
	}
}

func TestBUG493_ProjectRunHistoryHealthyIncludesPersisted(t *testing.T) {
	fws := newFakeWorkflowStore()
	fws.sessions["run-persisted"] = ProviderSessionState{RunID: "run-persisted", ProjectID: "p1", RunKind: "chat"}
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{},
		workflowStore: &bug493Store{fakeWorkflowStore: fws},
	}
	items, err := svc.projectRunHistory("p1")
	if err != nil {
		t.Fatalf("healthy store must not error: %v", err)
	}
	if len(items) != 1 || items[0].RunID != "run-persisted" {
		t.Fatalf("items = %+v, want run-persisted", items)
	}
}

func TestBUG493_ChatTimelineStoreErrorReturns502(t *testing.T) {
	fws := newFakeWorkflowStore()
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{"run-a": {id: "run-a", runKind: "chat", chatID: "cht_1", legSeq: 0}},
		workflowStore: &bug493Store{fakeWorkflowStore: fws, failByChat: true},
		chatRuns:      newChatRunRegistry(),
	}
	svc.chatRuns.register("run-a", "cht_1")

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st, _ := doJSON(t, "GET", srv.URL+"/client/chats/cht_1/timeline", nil, nil)
	if st == http.StatusOK {
		t.Fatal("timeline on an unreadable store must not return a 200 partial view")
	}
}

func TestBUG493_RunTimelineFallbackStoreErrorReturns502(t *testing.T) {
	fws := newFakeWorkflowStore()
	fws.sessions["run-x"] = ProviderSessionState{RunID: "run-x", RunKind: "chat", ChatID: "cht_9"}
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{}, // run-x NOT resident → fallback path
		workflowStore: &bug493Store{fakeWorkflowStore: fws, failGet: true},
		chatRuns:      newChatRunRegistry(),
	}

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st, _ := doJSON(t, "GET", srv.URL+"/client/workflow-runs/run-x/timeline", nil, nil)
	if st == http.StatusOK {
		t.Fatal("run timeline fallback on an unreadable store must not return a 200 empty view")
	}
}
