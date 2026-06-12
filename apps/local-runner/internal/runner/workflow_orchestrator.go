package runner

import (
	"context"
	"sync"
	"time"
)

// Phase 5 (04-05): the workflow orchestrator — the run/step sequencing engine
// ported from the Admin Web server tier into the Go runner, so Desktop and Web are
// both thin clients of the same backend.
//
// This component owns sequencing + persistence + per-run locking. It applies the
// pure state machine (PlanWorkflowProgress) under a per-run lock and persists the
// resulting transitions to the WorkflowStore. Many runs/workspaces are served
// concurrently: distinct runs never serialize against each other (per-run mutex,
// not a global one), and writes are idempotent so a partial failure is retryable
// without corrupting state (T-42).
//
// The live turn execution (send-with-retry, provider-session sync, cross-machine
// recovery) and Supabase edge-function reconciliation are tracked as deferred in
// the 04-05 DOD — they need a live provider + Supabase. The sequencing core, the
// state machine, and the prompt assembly are implemented and tested here.

// perRunLocks hands out a mutex per run id so concurrent runs don't serialize.
type perRunLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newPerRunLocks() *perRunLocks {
	return &perRunLocks{locks: map[string]*sync.Mutex{}}
}

// lock acquires the run's mutex and returns its unlock func.
func (p *perRunLocks) lock(runID string) func() {
	p.mu.Lock()
	m := p.locks[runID]
	if m == nil {
		m = &sync.Mutex{}
		p.locks[runID] = m
	}
	p.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// WorkflowOrchestrator sequences runs over a WorkflowStore.
type WorkflowOrchestrator struct {
	store WorkflowStore
	locks *perRunLocks
	clock func() time.Time
}

func NewWorkflowOrchestrator(store WorkflowStore) *WorkflowOrchestrator {
	return &WorkflowOrchestrator{store: store, locks: newPerRunLocks(), clock: time.Now}
}

// Progress advances a run: load steps → plan → persist transitions + logs → set run
// status. Held under the run's lock so concurrent calls for the same run are
// serialized while different runs proceed in parallel. Returns the applied plan.
func (o *WorkflowOrchestrator) Progress(ctx context.Context, runID string, yoloMode bool) (WorkflowProgressPlan, error) {
	unlock := o.locks.lock(runID)
	defer unlock()

	steps, err := o.store.LoadRunSteps(ctx, runID)
	if err != nil {
		return WorkflowProgressPlan{}, err
	}

	now := o.clock().UTC().Format(time.RFC3339Nano)
	plan := PlanWorkflowProgress(steps, yoloMode, now)

	for _, t := range plan.StepTransitions {
		if err := o.store.ApplyStepTransition(ctx, runID, t); err != nil {
			return plan, err // idempotent: a retry re-applies cleanly
		}
		for _, l := range t.Logs {
			if err := o.store.AppendLog(ctx, t.StepID, l); err != nil {
				return plan, err
			}
		}
	}

	if err := o.store.SetRunStatus(ctx, runID, plan.RunStatus, plan.RunFinishedAt); err != nil {
		return plan, err
	}
	return plan, nil
}
