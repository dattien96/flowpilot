package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFileSessionStoreUpsertAndList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	sess := ProviderSessionState{
		RunID:       "run-1",
		ProjectID:   "proj-1",
		ProviderKey: "claude",
		Status:      "completed",
		LastPrompt:  "hello",
		LastMessage: "hi",
		StartedAt:   "2026-06-17T10:00:00Z",
		UpdatedAt:   "2026-06-17T10:01:00Z",
		RunKind:     "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	got, err := store.ListProviderSessionsByProject(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	g := got[0]
	if g.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", g.RunID)
	}
	if string(g.ProviderKey) != "claude" {
		t.Errorf("ProviderKey = %q, want claude", g.ProviderKey)
	}
	if string(g.Status) != "completed" {
		t.Errorf("Status = %q, want completed", g.Status)
	}
	if g.LastPrompt != "hello" {
		t.Errorf("LastPrompt = %q, want hello", g.LastPrompt)
	}
	if g.LastMessage != "hi" {
		t.Errorf("LastMessage = %q, want hi", g.LastMessage)
	}
	if g.RunKind != "chat" {
		t.Errorf("RunKind = %q, want chat", g.RunKind)
	}
}

func TestLocalFileSessionStoreRestart(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	store1, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store1.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:      "run-A",
		ProjectID:  "proj-X",
		Status:     "completed",
		LastPrompt: "my question",
		LastMessage: "my answer",
		StartedAt:  now,
		UpdatedAt:  now,
		RunKind:    "chat",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Simulate process restart: new store instance from same directory.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore (restart): %v", err)
	}

	got, err := store2.ListProviderSessionsByProject(context.Background(), "proj-X")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject after restart: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after restart: got %d sessions, want 1", len(got))
	}
	g := got[0]
	if g.RunID != "run-A" {
		t.Errorf("RunID = %q, want run-A", g.RunID)
	}
	if g.LastPrompt != "my question" {
		t.Errorf("LastPrompt = %q, want my question", g.LastPrompt)
	}
	if g.LastMessage != "my answer" {
		t.Errorf("LastMessage = %q, want my answer", g.LastMessage)
	}
	if g.RunKind != "chat" {
		t.Errorf("RunKind = %q, want chat", g.RunKind)
	}
}

func TestLocalFileSessionStoreMultipleRunsRestart(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	store1, _ := NewLocalFileSessionStore(dir)
	sessions := []ProviderSessionState{
		{RunID: "run-1", ProjectID: "proj-A", Status: "completed", LastPrompt: "p1", UpdatedAt: now, RunKind: "chat"},
		{RunID: "run-2", ProjectID: "proj-A", Status: "completed", LastPrompt: "p2", UpdatedAt: now, RunKind: "chat"},
		{RunID: "run-3", ProjectID: "proj-B", Status: "running", LastPrompt: "p3", UpdatedAt: now, RunKind: "chat"},
	}
	for _, sess := range sessions {
		if err := store1.UpsertProviderSession(context.Background(), sess); err != nil {
			t.Fatalf("UpsertProviderSession %s: %v", sess.RunID, err)
		}
	}

	store2, _ := NewLocalFileSessionStore(dir)

	gotA, _ := store2.ListProviderSessionsByProject(context.Background(), "proj-A")
	if len(gotA) != 2 {
		t.Errorf("proj-A after restart: got %d, want 2", len(gotA))
	}
	gotB, _ := store2.ListProviderSessionsByProject(context.Background(), "proj-B")
	if len(gotB) != 1 {
		t.Errorf("proj-B after restart: got %d, want 1", len(gotB))
	}
}

func TestLocalFileSessionStoreLastWins(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store, _ := NewLocalFileSessionStore(dir)

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-1", ProjectID: "proj-1", Status: "running", LastPrompt: "first", UpdatedAt: now,
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-1", ProjectID: "proj-1", Status: "completed", LastPrompt: "second", UpdatedAt: now,
	})

	// Restart: only last state for run-1 should be loaded.
	store2, _ := NewLocalFileSessionStore(dir)
	got, _ := store2.ListProviderSessionsByProject(context.Background(), "proj-1")
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if string(got[0].Status) != "completed" {
		t.Errorf("Status = %q, want completed (last-wins)", got[0].Status)
	}
	if got[0].LastPrompt != "second" {
		t.Errorf("LastPrompt = %q, want second (last-wins)", got[0].LastPrompt)
	}
}

func TestLocalFileSessionStoreProjectFilter(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store, _ := NewLocalFileSessionStore(dir)

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "r1", ProjectID: "proj-A", UpdatedAt: now, Status: "completed"})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "r2", ProjectID: "proj-B", UpdatedAt: now, Status: "completed"})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "r3", ProjectID: "proj-A", UpdatedAt: now, Status: "running"})

	gotA, _ := store.ListProviderSessionsByProject(context.Background(), "proj-A")
	if len(gotA) != 2 {
		t.Errorf("proj-A: got %d, want 2", len(gotA))
	}
	gotB, _ := store.ListProviderSessionsByProject(context.Background(), "proj-B")
	if len(gotB) != 1 {
		t.Errorf("proj-B: got %d, want 1", len(gotB))
	}
	gotC, _ := store.ListProviderSessionsByProject(context.Background(), "proj-C")
	if len(gotC) != 0 {
		t.Errorf("proj-C: got %d, want 0", len(gotC))
	}
}

func TestLocalFileSessionStorePruning(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewLocalFileSessionStore(dir)

	oldTS := time.Now().UTC().Add(-91 * 24 * time.Hour).Format(time.RFC3339Nano)
	recentTS := time.Now().UTC().Format(time.RFC3339Nano)

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "old-run", ProjectID: "proj-1", UpdatedAt: oldTS, Status: "completed"})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "new-run", ProjectID: "proj-1", UpdatedAt: recentTS, Status: "completed"})

	// Restart: old-run should be pruned, new-run should survive.
	store2, _ := NewLocalFileSessionStore(dir)
	got, _ := store2.ListProviderSessionsByProject(context.Background(), "proj-1")
	if len(got) != 1 {
		t.Fatalf("after pruning: got %d sessions, want 1", len(got))
	}
	if got[0].RunID != "new-run" {
		t.Errorf("surviving RunID = %q, want new-run", got[0].RunID)
	}
}

func TestLocalFileSessionStoreMissingFile(t *testing.T) {
	dir := t.TempDir()
	// No file written yet — first call to ListProviderSessionsByProject should
	// succeed and return empty, not error.
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	got, err := store.ListProviderSessionsByProject(context.Background(), "proj-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d sessions from empty store, want 0", len(got))
	}
}

func TestLocalFileSessionStoreMalformedLines(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Write a file with a bad line before a good one; bad line must be skipped.
	filePath := filepath.Join(dir, "sessions.ndjson")
	goodLine := `{"run_id":"good","project_id":"proj-1","status":"completed","updated_at":"` + now + `"}`
	content := "{bad json}\n" + goodLine + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, _ := NewLocalFileSessionStore(dir)
	got, err := store.ListProviderSessionsByProject(context.Background(), "proj-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d sessions, want 1 (malformed line must be skipped)", len(got))
	}
}

func TestLocalFileSessionStoreWorkflowStoreDelegation(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewLocalFileSessionStore(dir)
	ctx := context.Background()

	runID := "run-wf"
	store.seed(runID, []RuntimeWorkflowStep{{ID: "step-1", Status: StepStatusPending}})

	steps, err := store.LoadRunSteps(ctx, runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "step-1" {
		t.Fatalf("LoadRunSteps: got %v", steps)
	}

	if err := store.ApplyStepTransition(ctx, runID, WorkflowStepTransition{
		StepID: "step-1",
		Patch:  WorkflowStepPatch{Status: StepStatusDone},
	}); err != nil {
		t.Errorf("ApplyStepTransition: %v", err)
	}

	if err := store.SetRunStatus(ctx, runID, RunStatusEngineDone, ""); err != nil {
		t.Errorf("SetRunStatus: %v", err)
	}

	if err := store.AppendLog(ctx, "step-1", WorkflowLog{LogLevel: LogInfo, Message: "done"}); err != nil {
		t.Errorf("AppendLog: %v", err)
	}
}

func TestLocalFileSessionStoreInteractiveStateDelegation(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewLocalFileSessionStore(dir)
	ctx := context.Background()

	// AppendEvent, UpsertApproval, UpsertQuestion must work (delegated to fake).
	if err := store.AppendEvent(ctx, ProviderEvent{WorkflowRunID: "run-1", Type: EventTurnStarted}); err != nil {
		t.Errorf("AppendEvent: %v", err)
	}
	if err := store.UpsertApproval(ctx, ProviderApprovalState{ApprovalID: "apr-1", RunID: "run-1"}); err != nil {
		t.Errorf("UpsertApproval: %v", err)
	}
	if err := store.UpsertQuestion(ctx, ProviderQuestionState{QuestionID: "q-1", RunID: "run-1"}); err != nil {
		t.Errorf("UpsertQuestion: %v", err)
	}
}

func TestLocalFileSessionStoreNewSubdir(t *testing.T) {
	base := t.TempDir()
	// Pass a non-existent subdir; NewLocalFileSessionStore should create it.
	subDir := filepath.Join(base, "nested", "flowpilot")
	store, err := NewLocalFileSessionStore(subDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore with nested subdir: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "r1", ProjectID: "p1", Status: "completed", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	if _, err := os.Stat(filepath.Join(subDir, "sessions.ndjson")); err != nil {
		t.Errorf("sessions.ndjson not created: %v", err)
	}
}

func TestLocalFileSessionStoreSessionHistoryReaderInterface(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewLocalFileSessionStore(dir)

	// Verify the store satisfies both key interfaces via type assertions.
	var _ WorkflowStore = store
	var _ InteractiveStateStore = store
	var _ SessionHistoryReader = store
}
