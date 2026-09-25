package runner

// Task-426 (CP-83): worktreePath is exposed read-only on the run-history and
// run-handle contracts so the embedded terminal resolves cwd from the
// persisted binding instead of recomputing .flowpilot/worktrees/<ownerID>.
// New file — additive only.

import (
	"context"
	"testing"
	"time"
)

func TestRunHistoryItem_IncludesWorktreePathWhenBound(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["run-wt"] = &interactiveRun{
		id:          "run-wt",
		projectID:   "proj-web",
		providerKey: ProviderKeyCodex,
		status:      RunStatusCompleted,
		createdAt:   now,
		updatedAt:   now,
		runKind:     "chat",
		worktree: &worktreeBinding{
			OwnerID: "cht-1",
			Path:    `C:\repo\.flowpilot\worktrees\cht-1`,
			Branch:  "flowpilot/cht-1",
			Slug:    "cht-1",
			State:   "active",
			Enabled: true,
		},
	}
	svc.mu.Unlock()

	history, _ := svc.projectRunHistory("proj-web")
	if len(history) != 1 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].WorktreePath != `C:\repo\.flowpilot\worktrees\cht-1` {
		t.Fatalf("worktreePath = %q, want the binding path", history[0].WorktreePath)
	}
	if history[0].WorktreeSlug != "cht-1" || history[0].WorktreeState != "active" {
		t.Fatalf("existing badge fields must still serialize: %+v", history[0])
	}
}

func TestRunHistoryItem_OmitsWorktreePathWhenUnbound(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["run-plain"] = &interactiveRun{
		id:          "run-plain",
		projectID:   "proj-web",
		providerKey: ProviderKeyCodex,
		status:      RunStatusCompleted,
		createdAt:   now,
		updatedAt:   now,
		runKind:     "chat",
	}
	svc.mu.Unlock()

	history, _ := svc.projectRunHistory("proj-web")
	if len(history) != 1 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].WorktreePath != "" {
		t.Fatalf("unbound run must omit worktreePath, got %q", history[0].WorktreePath)
	}
}

func TestRunHistoryItem_WorktreePathFromPersistedSessionAfterRestart(t *testing.T) {
	// The persisted session row is the post-restart source (in-memory s.runs
	// is empty). The history mapper must carry WorktreePath through it too.
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-persisted-wt",
		ProjectID:         "proj-web",
		ProviderSessionID: "session-1",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		RunKind:           "chat",
		WorktreePath:      `C:\repo\.flowpilot\worktrees\cht-9`,
		WorktreeSlug:      "cht-9",
		WorktreeState:     "active",
	}); err != nil {
		t.Fatal(err)
	}
	svc, _ := newTestServer(t)
	svc.workflowStore = store

	history, _ := svc.projectRunHistory("proj-web")
	if len(history) != 1 || history[0].RunID != "run-persisted-wt" {
		t.Fatalf("history = %+v", history)
	}
	if history[0].WorktreePath != `C:\repo\.flowpilot\worktrees\cht-9` {
		t.Fatalf("persisted worktreePath = %q", history[0].WorktreePath)
	}
}
