package runner

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestOrchestratorProgressSequencesAndPersists(t *testing.T) {
	store := newFakeWorkflowStore()
	store.seed("run-1", []RuntimeWorkflowStep{
		step("s1", "plan", StepStatusPending, false),
		step("s2", "build", StepStatusPending, false),
	})
	o := NewWorkflowOrchestrator(store)

	plan, err := o.Progress(context.Background(), "run-1", false)
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if plan.RunStatus != RunStatusEngineDone {
		t.Fatalf("run status = %s, want DONE", plan.RunStatus)
	}
	if store.runStatus["run-1"] != RunStatusEngineDone {
		t.Fatalf("store run status not persisted: %s", store.runStatus["run-1"])
	}
	steps, _ := store.LoadRunSteps(context.Background(), "run-1")
	for _, s := range steps {
		if s.Status != StepStatusDone {
			t.Fatalf("step %s not DONE: %s", s.ID, s.Status)
		}
	}
}

// T-42 (idempotency): re-running Progress on an already-finished run applies no new
// transitions — the state machine skips terminal steps, so writes converge.
func TestOrchestratorProgressIdempotentReRun(t *testing.T) {
	store := newFakeWorkflowStore()
	store.seed("run-1", []RuntimeWorkflowStep{step("s1", "plan", StepStatusPending, false)})
	o := NewWorkflowOrchestrator(store)

	_, _ = o.Progress(context.Background(), "run-1", false)
	appliesAfterFirst := store.applies

	_, _ = o.Progress(context.Background(), "run-1", false)
	if store.applies != appliesAfterFirst {
		t.Fatalf("re-run applied %d more transitions; should be 0 (terminal steps skipped)", store.applies-appliesAfterFirst)
	}
}

// T-42 (isolation): distinct runs progress concurrently without corrupting each
// other's state. Run with -race to catch data races on the store/locks.
func TestOrchestratorConcurrentRunsIsolated(t *testing.T) {
	store := newFakeWorkflowStore()
	const n = 20
	for i := 0; i < n; i++ {
		runID := "run-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		store.seed(runID, []RuntimeWorkflowStep{step("s1", "plan", StepStatusPending, false)})
	}
	o := NewWorkflowOrchestrator(store)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		runID := "run-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := o.Progress(context.Background(), runID, false); err != nil {
				t.Errorf("progress %s: %v", runID, err)
			}
		}()
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		runID := "run-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if store.runStatus[runID] != RunStatusEngineDone {
			t.Fatalf("run %s status = %s, want DONE", runID, store.runStatus[runID])
		}
	}
}

// per-run locking: a different run must be lockable while one run's lock is held.
func TestPerRunLocksDistinctRunsDoNotBlock(t *testing.T) {
	locks := newPerRunLocks()
	u1 := locks.lock("a")
	defer u1()

	done := make(chan struct{})
	go func() {
		locks.lock("b")() // lock + immediately unlock run "b" while "a" is held
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("locking a distinct run blocked while another run's lock was held")
	}
}
