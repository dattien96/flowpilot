package runner

// BUG-492: restoreChatRunTreeFromDrive swallowed session-store read errors —
// resolveRestoredRunID returned sourceRunID verbatim on ListAllProviderSessions
// failure (colliding with an unrelated existing record → clobber), the
// candidate collision-check treated read errors as "free", and the
// localAhead metadata merge treated a read error as "no local record" →
// remote-stale fields overwrote live local metadata (BUG-091 regression).
// Contract: restore must not write while the local authority is unreadable.

import (
	"context"
	"errors"
	"testing"
)

var errBug492StoreFault = errors.New("session store unreadable")

// bug492IndexErrStore fails only ListAllProviderSessions.
type bug492IndexErrStore struct {
	*fakeWorkflowStore
}

func (s *bug492IndexErrStore) ListAllProviderSessions(context.Context) ([]ProviderSessionState, error) {
	return nil, errBug492StoreFault
}

// bug492HistoryOnlyStore implements SessionHistoryReader but NOT
// SessionIndexReader — drives resolveRestoredRunID's fallback branch.
type bug492HistoryOnlyStore struct {
	getErr  bool
	byRunID map[string]ProviderSessionState
}

func (s *bug492HistoryOnlyStore) LoadRunSteps(context.Context, string) ([]RuntimeWorkflowStep, error) {
	return nil, nil
}
func (s *bug492HistoryOnlyStore) ApplyStepTransition(context.Context, string, WorkflowStepTransition) error {
	return nil
}
func (s *bug492HistoryOnlyStore) SetRunStatus(context.Context, string, WorkflowRunStatus, string) error {
	return nil
}
func (s *bug492HistoryOnlyStore) AppendLog(context.Context, string, WorkflowLog) error { return nil }
func (s *bug492HistoryOnlyStore) ListProviderSessionsByProject(context.Context, string) ([]ProviderSessionState, error) {
	return nil, nil
}
func (s *bug492HistoryOnlyStore) GetProviderSession(_ context.Context, runID string) (ProviderSessionState, bool, error) {
	if s.getErr {
		return ProviderSessionState{}, false, errBug492StoreFault
	}
	sess, ok := s.byRunID[runID]
	return sess, ok, nil
}

func TestBUG492_ResolveRestoredIDIndexErrorPropagates(t *testing.T) {
	svc := &InteractiveService{workflowStore: &bug492IndexErrStore{newFakeWorkflowStore()}}
	_, err := svc.resolveRestoredRunID(context.Background(), "machine-a", "run-src")
	if err == nil {
		t.Fatal("index read error must propagate — returning sourceRunID silently can clobber an unrelated record")
	}
}

func TestBUG492_ResolveRestoredIDFallbackGetErrorPropagates(t *testing.T) {
	store := &bug492HistoryOnlyStore{getErr: true}
	svc := &InteractiveService{workflowStore: store}
	_, err := svc.resolveRestoredRunID(context.Background(), "machine-a", "run-src")
	if err == nil {
		t.Fatal("GetProviderSession error in the fallback path must propagate")
	}
}

func TestBUG492_ResolveRestoredIDCollisionCheckErrorPropagates(t *testing.T) {
	// sourceRunID exists but belongs to a different source → the code enters
	// firstFreeRestoredRunID, whose predicate hits a store fault.
	store := &bug492HistoryOnlyStore{byRunID: map[string]ProviderSessionState{
		"run-src": {RunID: "run-src", SourceMachineID: "machine-B", SourceRunID: "run-other"},
	}}
	svc := &InteractiveService{workflowStore: store}
	// First probe (sourceRunID) reads fine; predicate probes must propagate
	// errors rather than treating faulted reads as "free".
	got, err := svc.resolveRestoredRunID(context.Background(), "machine-a", "run-src")
	if err != nil {
		t.Fatalf("unexpected error with healthy collision probes: %v", err)
	}
	if got == "" {
		t.Fatal("expected a derived candidate id")
	}

	// Now fail the store mid-way: candidate probes must not be treated free.
	store.getErr = true
	if _, err := svc.resolveRestoredRunID(context.Background(), "machine-a", "run-src"); err == nil {
		t.Fatal("collision-probe store error must propagate, not mint a clobbering id")
	}
}

func TestBUG492_LocalAheadReadErrorAborts(t *testing.T) {
	svc := &InteractiveService{workflowStore: &bug492HistoryOnlyStore{getErr: true}}
	sess := &ProviderSessionState{RunID: "local-1", LastPrompt: "remote-old"}
	err := svc.applyLocalAheadSessionFields(context.Background(), sess, "local-1")
	if err == nil {
		t.Fatal("localAhead merge must abort on read error — remote stale fields would overwrite live local metadata")
	}
}

func TestBUG492_LocalAheadHealthyPreservesLocal(t *testing.T) {
	store := &bug492HistoryOnlyStore{byRunID: map[string]ProviderSessionState{
		"local-1": {RunID: "local-1", LastPrompt: "local-new", LastMessage: "local-msg", Status: RunStatusCompleted},
	}}
	svc := &InteractiveService{workflowStore: store}
	sess := &ProviderSessionState{RunID: "local-1", LastPrompt: "remote-old", TurnCount: 1}
	if err := svc.applyLocalAheadSessionFields(context.Background(), sess, "local-1"); err != nil {
		t.Fatalf("healthy local read must not error: %v", err)
	}
	if sess.LastPrompt != "local-new" || sess.LastMessage != "local-msg" || sess.Status != RunStatusCompleted {
		t.Fatalf("local fields not preserved: %+v", sess)
	}
}
