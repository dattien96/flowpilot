package runner

// BUG-490: chat worktree binding reads in run_worktree.go swallowed
// ListProviderSessionsByChat errors — findChatWorktreeBindingLocked returned
// "no binding" on a store fault (a new leg could then provision a second
// worktree for a chat with a live binding), and markChatWorktreeState left
// persisted legs on stale lifecycle state with no report. Contract: binding
// decisions must not run on a provably-partial store view.

import (
	"context"
	"errors"
	"testing"
)

// bug490ErrStore fails ListProviderSessionsByChat — the BUG-485 partial-load
// condition reaching the worktree binding authority.
type bug490ErrStore struct {
	*fakeWorkflowStore
}

func (s *bug490ErrStore) ListProviderSessionsByChat(context.Context, string) ([]ProviderSessionState, error) {
	return nil, errors.New("sessions shard unreadable")
}

func TestBUG490_BindingLookupStoreErrorPropagates(t *testing.T) {
	fws := newFakeWorkflowStore()
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-leg", RunKind: "chat", ChatID: "cht_w",
		WorktreeState: "active", WorktreePath: "/wt/live",
	})
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: &bug490ErrStore{fws}}

	binding, err := svc.findChatWorktreeBindingLocked("cht_w")
	if err == nil {
		t.Fatal("store error must propagate — 'no binding' on fault is a split-brain worktree risk")
	}
	if binding != nil {
		t.Fatal("binding must be nil when the read failed")
	}
}

func TestBUG490_BindingLookupHealthyFindsPersistedBinding(t *testing.T) {
	fws := newFakeWorkflowStore()
	_ = fws.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-leg", RunKind: "chat", ChatID: "cht_w", LegSeq: 0,
		WorktreeState: "active", WorktreePath: "/wt/live",
	})
	svc := &InteractiveService{runs: map[string]*interactiveRun{}, workflowStore: fws}

	binding, err := svc.findChatWorktreeBindingLocked("cht_w")
	if err != nil {
		t.Fatalf("healthy read must not error: %v", err)
	}
	if binding == nil || binding.Path != "/wt/live" {
		t.Fatalf("binding = %+v, want /wt/live", binding)
	}
}

func TestBUG490_MarkStateStoreErrorReported(t *testing.T) {
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{},
		workflowStore: &bug490ErrStore{newFakeWorkflowStore()},
	}
	if err := svc.markChatWorktreeState("cht_w", "discarded"); err == nil {
		t.Fatal("list error must surface — silently skipping persisted legs leaves stale lifecycle state")
	}
}
