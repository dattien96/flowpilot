package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestLocalFileSessionStoreRestoredMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	sess := ProviderSessionState{
		RunID:           "run-restore",
		ProjectID:       "proj-1",
		ProviderKey:     "codex",
		Status:          "completed",
		RunKind:         "chat",
		SourceMachineID: "mch-source",
		SourceRunID:     "run-source",
		RestoredFrom:    "google_drive",
		SyncStatus:      "restored",
		SyncUpdatedAt:   "2026-06-17T10:10:00Z",
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-restore")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected restored session to be found")
	}
	if got.SourceMachineID != sess.SourceMachineID || got.SourceRunID != sess.SourceRunID {
		t.Fatalf("unexpected source metadata: %+v", got)
	}
	if got.RestoredFrom != sess.RestoredFrom || got.SyncStatus != sess.SyncStatus || got.SyncUpdatedAt != sess.SyncUpdatedAt {
		t.Fatalf("unexpected sync metadata: %+v", got)
	}
}

func TestLocalFileSessionStoreAgentMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	sess := ProviderSessionState{
		RunID:       "run-child",
		ProjectID:   "proj-1",
		ProviderKey: "codex",
		Status:      "running",
		StartedAt:   "2026-06-19T10:00:00Z",
		UpdatedAt:   "2026-06-19T10:01:00Z",
		ParentRunID: "run-parent",
		AgentName:   "coder",
		Role:        "coder",
		DependsOn:   []string{"run-abc"},
		AgentStatus: "spawned",
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-child")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected child session to be found")
	}
	if got.ParentRunID != "run-parent" || got.AgentName != "coder" || got.Role != "coder" || got.AgentStatus != "spawned" {
		t.Fatalf("unexpected agent metadata: %+v", got)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "run-abc" {
		t.Fatalf("dependsOn = %v, want [run-abc]", got.DependsOn)
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
		RunID:       "run-A",
		ProjectID:   "proj-X",
		Status:      "completed",
		LastPrompt:  "my question",
		LastMessage: "my answer",
		StartedAt:   now,
		UpdatedAt:   now,
		RunKind:     "chat",
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

func TestLocalFileSessionStoreSyncMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	session := ProviderSessionState{
		RunID:           "run-sync",
		ProjectID:       "proj-sync",
		ProviderKey:     "codex",
		Status:          "completed",
		RunKind:         "chat",
		SourceMachineID: "mch_source",
		SourceRunID:     "run-source",
		RestoredFrom:    "google_drive",
		SyncStatus:      "restored",
		SyncUpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := store.UpsertProviderSession(context.Background(), session); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-sync")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected reloaded session to exist")
	}
	if got.SourceMachineID != session.SourceMachineID || got.SourceRunID != session.SourceRunID {
		t.Fatalf("source metadata mismatch: got %#v want %#v", got, session)
	}
	if got.SyncStatus != "restored" || got.RestoredFrom != "google_drive" || got.SyncUpdatedAt == "" {
		t.Fatalf("sync metadata not preserved: %#v", got)
	}
}

func TestLocalFileSessionStoreLoadsLegacyRecordWithoutSyncMetadata(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	line := `{"run_id":"legacy-run","project_id":"legacy-project","provider_key":"codex","status":"completed","updated_at":"` + now + `","run_kind":"chat"}`
	if err := os.WriteFile(filepath.Join(dir, "sessions.ndjson"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	got, found, err := store.GetProviderSession(context.Background(), "legacy-run")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected legacy record to load")
	}
	if got.SourceMachineID != "" || got.SourceRunID != "" || got.SyncStatus != "" || got.RestoredFrom != "" {
		t.Fatalf("legacy record unexpectedly populated sync metadata: %#v", got)
	}
}

func TestLoopStateRoundTripsThroughSessionStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	original := ProviderSessionState{
		RunID:     "run-flow-1",
		ProjectID: "proj-x",
		RunKind:   "chat",
		Status:    "completed",
		UpdatedAt: now,
		LoopState: AgentLoopState{
			Mode:        "explicit",
			Round:       2,
			Cap:         5,
			RoundCap:    5,
			ExtendCount: 1,
			ActiveNode:  "coder",
		},
	}
	if err := store.UpsertProviderSession(context.Background(), original); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Reload from disk to confirm persistence.
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-flow-1")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("session not found after reload")
	}
	ls := got.LoopState
	if ls.Mode != "explicit" || ls.Round != 2 || ls.Cap != 5 || ls.ExtendCount != 1 || ls.ActiveNode != "coder" {
		t.Errorf("LoopState = %+v, want Mode=explicit Round=2 Cap=5 ExtendCount=1 ActiveNode=coder", ls)
	}
}

func TestZeroLoopStateOmittedFromDisk(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	plain := ProviderSessionState{RunID: "run-plain", ProjectID: "p", RunKind: "chat", Status: "completed", UpdatedAt: now}
	if err := store.UpsertProviderSession(context.Background(), plain); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sessions.ndjson"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "loop_state") {
		t.Error("sessions.ndjson should not contain loop_state for a plain chat run")
	}
}

func TestFlowRunRestoresSeedsOrchestratorLoopState(t *testing.T) {
	// Simulate a runner restart: persist a flow run, then reconstruct it and
	// verify the orchestrator loop state is restored (Task-085 T-4).
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Simulate explicit-mode flow run at round 2.
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Mode:     "explicit",
		Round:    2,
		Cap:      5,
		RoundCap: 5,
	})
	svc.persistParentSession(parent.RunID)

	// Rebuild service from the same store (simulates restart).
	store := svc.workflowStore
	svc2 := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	reader, ok := store.(SessionHistoryReader)
	if !ok {
		t.Fatal("store does not implement SessionHistoryReader")
	}
	st, found, stErr := reader.GetProviderSession(context.Background(), parent.RunID)
	if stErr != nil || !found {
		t.Fatalf("session not found after persist: found=%v err=%v", found, stErr)
	}
	if _, recErr := svc2.reconstructRun(st); recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	snap := svc2.agentGraphSnapshot(parent.RunID)
	if snap.LoopState.Mode != "explicit" || snap.LoopState.Round != 2 || snap.LoopState.Cap != 5 {
		t.Errorf("LoopState after restart = %+v, want Mode=explicit Round=2 Cap=5", snap.LoopState)
	}
}

func TestAutoOrchestrateRoundTripsThroughSessionStore(t *testing.T) {
	// Verify that autoOrchestrate=true survives a write/reload cycle so a restarted
	// runner correctly resumes hub auto-reinvocation (Task-093 / CP-36 P-7).
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	original := ProviderSessionState{
		RunID: "run-ao-1", ProjectID: "proj", RunKind: "chat", Status: "completed",
		UpdatedAt:       now,
		AutoOrchestrate: true,
	}
	if err := store.UpsertProviderSession(context.Background(), original); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Reload from disk.
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, found, stErr := reloaded.GetProviderSession(context.Background(), "run-ao-1")
	if stErr != nil {
		t.Fatalf("GetProviderSession: %v", stErr)
	}
	if !found {
		t.Fatal("session not found after reload")
	}
	if !got.AutoOrchestrate {
		t.Error("AutoOrchestrate should be true after round-trip through sessions.ndjson")
	}
}

func TestZeroAutoOrchestrateOmittedFromDisk(t *testing.T) {
	// A plain chat run (autoOrchestrate=false) must not write the field to disk.
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	plain := ProviderSessionState{RunID: "run-plain-ao", ProjectID: "p", RunKind: "chat", Status: "completed", UpdatedAt: now}
	if err := store.UpsertProviderSession(context.Background(), plain); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sessions.ndjson"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "auto_orchestrate") {
		t.Error("sessions.ndjson should not contain auto_orchestrate for a plain chat run")
	}
}

func TestFlowRunRestoresSeedsAutoOrchestrate(t *testing.T) {
	// Simulate a runner restart: persist a flow run with autoOrchestrate=true, then
	// reconstruct it and verify the flag is restored on the interactiveRun (Task-093).
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].autoOrchestrate = true
	svc.mu.Unlock()
	svc.persistParentSession(parent.RunID)

	// Rebuild service from the same store (simulates restart).
	store := svc.workflowStore
	svc2 := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	reader, ok := store.(SessionHistoryReader)
	if !ok {
		t.Fatal("store does not implement SessionHistoryReader")
	}
	st, found, stErr := reader.GetProviderSession(context.Background(), parent.RunID)
	if stErr != nil || !found {
		t.Fatalf("session not found after persist: found=%v err=%v", found, stErr)
	}
	rs2, recErr := svc2.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if !rs2.autoOrchestrate {
		t.Error("autoOrchestrate should be true after run reconstruction from disk")
	}
}

// BUG-fix (Finding 5): flowCohortId must survive a session store round-trip so that
// a restored child run can still enter the cohort barrier on completion.
func TestFlowCohortIDRoundTripsThroughSessionStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess := ProviderSessionState{
		RunID:        "child-1",
		ProjectID:    "proj",
		ProviderKey:  ProviderKeyCodex,
		UpdatedAt:    now,
		FlowCohortID: "review-round-1",
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Reload from a fresh store instance (full disk round-trip).
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, found, getErr := reloaded.GetProviderSession(context.Background(), "child-1")
	if getErr != nil || !found {
		t.Fatalf("GetProviderSession: found=%v err=%v", found, getErr)
	}
	if got.FlowCohortID != "review-round-1" {
		t.Errorf("FlowCohortID = %q after round-trip, want %q", got.FlowCohortID, "review-round-1")
	}
}

func TestZeroFlowCohortIDOmittedFromDisk(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	sess := ProviderSessionState{RunID: "plain-1", ProjectID: "proj", ProviderKey: ProviderKeyCodex}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// The NDJSON line must NOT contain "flow_cohort_id" when the field is empty.
	data, readErr := os.ReadFile(filepath.Join(dir, "sessions.ndjson"))
	if readErr != nil {
		t.Fatalf("read file: %v", readErr)
	}
	if strings.Contains(string(data), "flow_cohort_id") {
		t.Error("expected flow_cohort_id to be omitted when empty, but found it on disk")
	}
}

// TestLocalFileSessionStoreFlowEventsDurability verifies that CP-41 flow events
// (EventFlowContextPackage) survive a simulated process restart: after creating
// a new store instance from the same dataDir, LoadFlowEvents returns the
// persisted event, and FindFlowContextPackage succeeds on the reloaded events.
func TestLocalFileSessionStoreFlowEventsDurability(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	// Build a minimal FlowContextPackage and emit an EventFlowContextPackage.
	pkg := FlowContextPackage{
		PackageID:     "pkg-durability-1",
		WorkflowRunID: "run-dur",
		FeatureKey:    "agent-flow-engine",
	}
	ev := ProviderEvent{
		Type:               EventFlowContextPackage,
		WorkflowRunID:      "run-dur",
		WorkflowStepRunID:  "step-plan-1",
		FlowContextPackage: &pkg,
	}
	if err := store1.AppendEvent(context.Background(), ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// Simulate a process restart by creating a new store instance.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore (restart): %v", err)
	}

	// LoadFlowEvents must return the persisted event.
	evs, err := store2.LoadFlowEvents(context.Background(), "run-dur")
	if err != nil {
		t.Fatalf("LoadFlowEvents: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("LoadFlowEvents: got %d events, want 1", len(evs))
	}
	if evs[0].Type != EventFlowContextPackage {
		t.Errorf("event type = %q, want %q", evs[0].Type, EventFlowContextPackage)
	}

	// FindFlowContextPackage must succeed on the reloaded events.
	found, ok := FindFlowContextPackage(evs, "step-plan-1")
	if !ok {
		t.Fatal("FindFlowContextPackage: not found after restart")
	}
	if found.PackageID != "pkg-durability-1" {
		t.Errorf("PackageID = %q, want pkg-durability-1", found.PackageID)
	}
}

// TestLocalFileSessionStoreFlowEventsNonCp41NotPersisted verifies that
// non-CP-41 event types (e.g. EventTurnStarted) are NOT written to the
// flow-events sidecar.
func TestLocalFileSessionStoreFlowEventsNonCp41NotPersisted(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	_ = store.AppendEvent(context.Background(), ProviderEvent{
		Type:          EventTurnStarted,
		WorkflowRunID: "run-x",
		Prompt:        "hello",
	})

	evs, _ := store.LoadFlowEvents(context.Background(), "run-x")
	if len(evs) != 0 {
		t.Errorf("expected 0 flow events for non-CP-41 type, got %d", len(evs))
	}
}

// TestLocalFileSessionStoreDeleteFlowEvents verifies that DeleteFlowEvents
// removes the sidecar and subsequent LoadFlowEvents returns empty.
func TestLocalFileSessionStoreDeleteFlowEvents(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	pkg := FlowContextPackage{PackageID: "pkg-del", WorkflowRunID: "run-del"}
	_ = store.AppendEvent(context.Background(), ProviderEvent{
		Type:               EventFlowContextPackage,
		WorkflowRunID:      "run-del",
		WorkflowStepRunID:  "step-plan",
		FlowContextPackage: &pkg,
	})

	if err := store.DeleteFlowEvents(context.Background(), "run-del"); err != nil {
		t.Fatalf("DeleteFlowEvents: %v", err)
	}

	// No-op on missing file.
	if err := store.DeleteFlowEvents(context.Background(), "run-del"); err != nil {
		t.Fatalf("DeleteFlowEvents (2nd): %v", err)
	}

	evs, _ := store.LoadFlowEvents(context.Background(), "run-del")
	if len(evs) != 0 {
		t.Errorf("expected 0 events after delete, got %d", len(evs))
	}
}

