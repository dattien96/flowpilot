package runner

import (
	"context"
	"path/filepath"
	"testing"
)

// BUG-622: persistedCompletedChildExists scanned every durable child session
// by parent+label with no sprint scoping — the same foreign-sprint evidence
// hole as BUG-616, one layer up. Live run-150388: sprint-1's completed
// coder/reviewer legs (vibe_task_index=1) made pendingVibeResumeFromNode keep
// overwriting the resume target ("Resume from reviewer?" instead of "Resume
// from tdd?"), and can satisfy maybeAdvancePendingValidateAfterCoder's
// predecessor check — advancing past a leg that never ran in this sprint.
//
// Fix: only sessions belonging to the run's current vibeSprintIndex count
// (sessionBelongsToVibeSprint). Legacy index-0 sessions keep the old
// fail-open behavior; non-sprint parents keep the unscoped scan.
func TestBug622_PersistedCompletedChildIgnoresForeignSprintLegs(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	now := "2026-10-03T11:24:00Z"

	parent := &interactiveRun{
		id:              "run-hub-622",
		workspaceCwd:    t.TempDir(),
		vibeSprintIndex: 2,
	}
	svc.mu.Lock()
	svc.runs[parent.id] = parent
	svc.mu.Unlock()

	mk := func(runID, label string, status RunStatus, taskIdx int) ProviderSessionState {
		return ProviderSessionState{
			RunID: runID, ParentRunID: parent.id, Label: label,
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			Status: status, VibeTaskIndex: taskIdx,
			StartedAt: now, UpdatedAt: now,
		}
	}
	for _, session := range []ProviderSessionState{
		// Foreign-sprint completed legs — the BUG-622 repro. Must not count.
		mk("run-old-coder", "coder", RunStatusCompleted, 1),
		mk("run-old-reviewer", "reviewer", RunStatusCompleted, 1),
		// Current-sprint completed leg — still counts.
		mk("run-cur-tdd", "tdd", RunStatusCompleted, 2),
		// Legacy unstamped leg — fails open (BUG-616 contract).
		mk("run-legacy-validate", "validate", RunStatusCompleted, 0),
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	if svc.persistedCompletedChildExists(parent.id, "coder") {
		t.Fatalf("persistedCompletedChildExists(coder): sprint-1 completed leg satisfied sprint-2's node")
	}
	if svc.persistedCompletedChildExists(parent.id, "reviewer") {
		t.Fatalf("persistedCompletedChildExists(reviewer): sprint-1 completed leg satisfied sprint-2's node")
	}
	if !svc.persistedCompletedChildExists(parent.id, "tdd") {
		t.Fatalf("persistedCompletedChildExists(tdd): current-sprint completed leg must count")
	}
	if !svc.persistedCompletedChildExists(parent.id, "validate") {
		t.Fatalf("persistedCompletedChildExists(validate): legacy unstamped leg must keep counting (fail-open)")
	}
}
