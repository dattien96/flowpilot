package runner

// BUG-491: three enumeration sites swallowed SessionIndexReader errors —
// foreignProviderSessionIDs (pinning checks then ran on memory-only data),
// collectDeleteRunTree (delete walked a silently-partial tree), and
// resumedFlowStepRows (resume rebuilt cohort rows from a partial view).
// Contract: an unreadable session index must fail closed, never feed a
// "looks complete" empty view into pinning, deletion, or reconstruction.

import (
	"context"
	"errors"
	"testing"
)

// bug491ErrStore fails ListAllProviderSessions while every other method
// delegates to the fake store.
type bug491ErrStore struct {
	*fakeWorkflowStore
}

func (s *bug491ErrStore) ListAllProviderSessions(context.Context) ([]ProviderSessionState, error) {
	return nil, errors.New("session index shard unreadable")
}

func TestBUG491_ForeignSessionEnumerationFailsClosed(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug491ErrStore{newFakeWorkflowStore()}}
	rs := &interactiveRun{id: "run-a", projectID: "p1", workspaceCwd: "/ws"}

	// An unverifiable id must be treated as foreign (refuse), not auto-accepted.
	if !svc.isForeignProviderSessionID(rs, "sess-unverifiable") {
		t.Fatal("unreadable index must fail closed: unverifiable id treated as foreign")
	}
}

func TestBUG491_FilterOwnedEnumerationErrorKeepsOnlyOwn(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug491ErrStore{newFakeWorkflowStore()}}
	rs := &interactiveRun{id: "run-a", providerSessionID: "sess-own", projectID: "p1", workspaceCwd: "/ws"}

	out := svc.filterOwnedProviderSessionIDs(rs, []string{"sess-own", "sess-unverifiable"})
	for _, id := range out {
		if id != "sess-own" {
			t.Fatalf("unverifiable id %q passed the filter on a store fault", id)
		}
	}
}

func TestBUG491_DeleteTreeEnumerationErrorAborts(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug491ErrStore{newFakeWorkflowStore()}}
	_, _, err := svc.collectDeleteRunTree("run-x", ProviderSessionState{RunID: "run-x"})
	if err == nil {
		t.Fatal("delete-tree enumeration on a broken index must not proceed silently")
	}
}

func TestBUG491_DeleteTreeHealthyUnchanged(t *testing.T) {
	fws := newFakeWorkflowStore()
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-p"})
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-c", ParentRunID: "run-p"})
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}
	order, byRun, err := svc.collectDeleteRunTree("run-p", ProviderSessionState{RunID: "run-p"})
	if err != nil {
		t.Fatalf("healthy enumeration must not error: %v", err)
	}
	if len(order) != 2 || byRun["run-c"].RunID != "run-c" {
		t.Fatalf("tree = %v, want parent+child", order)
	}
}
