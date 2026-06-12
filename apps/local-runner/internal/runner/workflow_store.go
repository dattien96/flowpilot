package runner

import (
	"context"
	"sync"
)

// Phase 5 (04-05): the persistence boundary for workflow orchestration. The runner
// becomes the single backend, reading/writing run/step/log state in Supabase. The
// orchestrator depends only on this interface; the live PostgREST implementation is
// SupabaseWorkflowStore (supabase_workflow_store.go), and tests use the in-memory
// fakeWorkflowStore.
//
// All writes are idempotent (ApplyTransition is keyed by step id and overwrites; a
// retried apply converges to the same row) so a partial failure mid-progress is
// safe to retry — the concurrency requirement in 04-05 (T-42).
type WorkflowStore interface {
	// LoadRunSteps returns a run's steps in execution order.
	LoadRunSteps(ctx context.Context, runID string) ([]RuntimeWorkflowStep, error)
	// ApplyStepTransition patches one step (idempotent, last-write-wins).
	ApplyStepTransition(ctx context.Context, runID string, t WorkflowStepTransition) error
	// SetRunStatus updates the run-level status + finished_at.
	SetRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt string) error
	// AppendLog records a step log line.
	AppendLog(ctx context.Context, stepID string, log WorkflowLog) error
}

// ---- in-memory fake (tests) ------------------------------------------------

type fakeWorkflowStore struct {
	mu        sync.Mutex
	steps     map[string][]RuntimeWorkflowStep // runID -> steps
	runStatus map[string]WorkflowRunStatus
	finished  map[string]string
	logs      map[string][]WorkflowLog // stepID -> logs
	applies   int                      // total ApplyStepTransition calls (idempotency probe)
}

func newFakeWorkflowStore() *fakeWorkflowStore {
	return &fakeWorkflowStore{
		steps:     map[string][]RuntimeWorkflowStep{},
		runStatus: map[string]WorkflowRunStatus{},
		finished:  map[string]string{},
		logs:      map[string][]WorkflowLog{},
	}
}

func (f *fakeWorkflowStore) seed(runID string, steps []RuntimeWorkflowStep) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps[runID] = steps
}

func (f *fakeWorkflowStore) LoadRunSteps(_ context.Context, runID string) ([]RuntimeWorkflowStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RuntimeWorkflowStep, len(f.steps[runID]))
	copy(out, f.steps[runID])
	return out, nil
}

func (f *fakeWorkflowStore) ApplyStepTransition(_ context.Context, runID string, t WorkflowStepTransition) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies++
	steps := f.steps[runID]
	for i := range steps {
		if steps[i].ID != t.StepID {
			continue
		}
		steps[i].Status = t.Patch.Status
		if t.Patch.StartedAt != nil {
			steps[i].StartedAt = *t.Patch.StartedAt
		}
		if t.Patch.RejectionNote != nil {
			steps[i].RejectionNote = *t.Patch.RejectionNote
		}
		if t.Patch.RetryCount != nil {
			steps[i].RetryCount = *t.Patch.RetryCount
		}
		break
	}
	return nil
}

func (f *fakeWorkflowStore) SetRunStatus(_ context.Context, runID string, status WorkflowRunStatus, finishedAt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runStatus[runID] = status
	f.finished[runID] = finishedAt
	return nil
}

func (f *fakeWorkflowStore) AppendLog(_ context.Context, stepID string, log WorkflowLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs[stepID] = append(f.logs[stepID], log)
	return nil
}
