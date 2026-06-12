package runner

import (
	"fmt"
	"sync"
	"time"
)

// Phase 4 (04-04): the finalizer hook.
//
// After `turn_completed`, the finalizer runs the post-turn pipeline. The full
// artifact/summary/RAG/GDrive logic is ported in Phase 5; here we establish:
//   - the hook (invoked once per completed turn),
//   - a local final-response + changed-files/diff snapshot artifact,
//   - finalize state tracked SEPARATELY from the turn so a finalize failure never
//     erases the completed turn — it is left retryable,
//   - idempotency: a retry must not double-write artifacts (keyed by run+turn id).
//
// The heavy lifting (summary, Supabase RAG, GDrive, step-status, audit trail) is
// pluggable via `run`; the default produces the local snapshot only.

type finalizeStatus string

const (
	finalizePending finalizeStatus = "pending"
	finalizeDone    finalizeStatus = "done"
	finalizeFailed  finalizeStatus = "failed"
)

// finalizeInput is the post-turn context handed to the finalizer.
type finalizeInput struct {
	RunID        string
	TurnID       string
	FinalMessage string
	ChangedFiles []string
}

type finalizeState struct {
	runID     string
	turnID    string
	status    finalizeStatus
	attempts  int
	lastError string
	artifacts []Artifact
}

// finalizer owns finalize state for all runs. It is safe for concurrent use.
type finalizer struct {
	mu     sync.Mutex
	states map[string]*finalizeState // key = runID + ":" + turnID

	// run does the actual post-turn work. Pluggable so Phase 5 can swap in the real
	// summary/RAG/GDrive pipeline and tests can inject a failing run (T-27/T-28).
	run func(finalizeInput) ([]Artifact, error)

	clock func() time.Time
}

func newFinalizer() *finalizer {
	f := &finalizer{states: map[string]*finalizeState{}, clock: time.Now}
	f.run = f.localSnapshot
	return f
}

func finalizeKey(runID, turnID string) string { return runID + ":" + turnID }

// Finalize runs the finalizer for a completed turn. It is idempotent: once a turn
// has finalized successfully, subsequent calls are no-ops (no double-write). A
// failed attempt leaves the state retryable via the same call.
func (f *finalizer) Finalize(in finalizeInput) error {
	f.mu.Lock()
	key := finalizeKey(in.RunID, in.TurnID)
	st := f.states[key]
	if st == nil {
		st = &finalizeState{runID: in.RunID, turnID: in.TurnID, status: finalizePending}
		f.states[key] = st
	}
	if st.status == finalizeDone {
		f.mu.Unlock()
		return nil // idempotent: already finalized, never re-write
	}
	st.attempts++
	f.mu.Unlock()

	artifacts, err := f.run(in)

	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		st.status = finalizeFailed
		st.lastError = err.Error()
		return err
	}
	st.status = finalizeDone
	st.lastError = ""
	st.artifacts = artifacts // overwrite (not append) so a retry can't duplicate
	return nil
}

// state returns a copy of the finalize state for a turn, if any.
func (f *finalizer) state(runID, turnID string) (finalizeState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.states[finalizeKey(runID, turnID)]
	if st == nil {
		return finalizeState{}, false
	}
	return *st, true
}

// artifactsForRun returns the artifacts of the most recent finalized turn of a run.
func (f *finalizer) artifactsForRun(runID string) ([]Artifact, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []Artifact
	for _, st := range f.states {
		if st.runID == runID && st.status == finalizeDone {
			found = st.artifacts // last writer wins; one turn at a time per run (409)
		}
	}
	return found, found != nil
}

// localSnapshot is the default Phase-4 finalize: a final-response artifact plus a
// changed-files/diff snapshot. Phase 5 replaces this with summary + RAG + GDrive.
func (f *finalizer) localSnapshot(in finalizeInput) ([]Artifact, error) {
	now := f.clock().UTC().Format(time.RFC3339Nano)
	artifacts := []Artifact{
		{
			ID:        in.RunID + ":" + in.TurnID + ":final",
			RunID:     in.RunID,
			Kind:      "final_response",
			Name:      "final-response.md",
			Preview:   in.FinalMessage,
			CreatedAt: now,
		},
		{
			ID:        in.RunID + ":" + in.TurnID + ":diff",
			RunID:     in.RunID,
			Kind:      "diff_snapshot",
			Name:      "changes.diff",
			Preview:   fmt.Sprintf("%d file(s) changed", len(in.ChangedFiles)),
			CreatedAt: now,
		},
	}
	return artifacts, nil
}
