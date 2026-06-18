package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalFileSessionStoreProviderAccountIDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	input := ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-1",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), input); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if got.ProviderAccountID != "acct-a" {
		t.Fatalf("ProviderAccountID = %q, want acct-a", got.ProviderAccountID)
	}
	if got.ProviderSessionID != "rollout-1" {
		t.Fatalf("ProviderSessionID = %q, want rollout-1", got.ProviderSessionID)
	}
}

func TestLocalFileSessionStoreLoadsLegacyRecordWithoutProviderAccountID(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	line := `{"run_id":"run-legacy","project_id":"project-1","provider_key":"codex","provider_session_id":"rollout-1","status":"completed","updated_at":"` + now + `","run_kind":"chat"}`
	if err := os.WriteFile(filepath.Join(dir, "sessions.ndjson"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	got, found, err := store.GetProviderSession(context.Background(), "run-legacy")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("expected legacy session to be found")
	}
	if got.ProviderAccountID != "" {
		t.Fatalf("ProviderAccountID = %q, want empty", got.ProviderAccountID)
	}
}

func TestLocalFileSessionStoreLastWinsRepointsProviderSessionAndAccount(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	first := ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderSessionID: "old-session",
		ProviderAccountID: "acct-a",
		Status:            RunStatusRunning,
		UpdatedAt:         now,
	}
	second := first
	second.ProviderSessionID = "new-session"
	second.ProviderAccountID = "acct-b"
	second.Status = RunStatusCompleted
	if err := store.UpsertProviderSession(context.Background(), first); err != nil {
		t.Fatalf("UpsertProviderSession first: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), second); err != nil {
		t.Fatalf("UpsertProviderSession second: %v", err)
	}

	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	got, found, err := reloaded.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", got, found, err)
	}
	if got.ProviderSessionID != "new-session" || got.ProviderAccountID != "acct-b" {
		t.Fatalf("unexpected last-wins session: %+v", got)
	}
}

func TestLocalFileSessionStoreGetProviderSessionNotFound(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	got, found, err := store.GetProviderSession(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if found {
		t.Fatalf("expected not found, got %+v", got)
	}
}

func TestSessionStateOfUsesRealProviderSessionIDWhenKnown(t *testing.T) {
	state := sessionStateOf(&interactiveRun{
		id:                    "run-1",
		projectID:             "project-1",
		providerKey:           ProviderKeyClaude,
		providerSessionID:     "thread-1",
		realProviderSessionID: "claude-real-1",
		providerAccountID:     "acct-a",
		workspaceCwd:          "/repo",
		runKind:               "chat",
	})
	if state.ProviderSessionID != "claude-real-1" {
		t.Fatalf("ProviderSessionID = %q, want claude-real-1", state.ProviderSessionID)
	}
	if state.ProviderAccountID != "acct-a" {
		t.Fatalf("ProviderAccountID = %q, want acct-a", state.ProviderAccountID)
	}
}

func TestSessionStateOfFallsBackToSyntheticProviderSessionID(t *testing.T) {
	state := sessionStateOf(&interactiveRun{
		id:                "run-1",
		projectID:         "project-1",
		providerKey:       ProviderKeyClaude,
		providerSessionID: "thread-1",
		providerAccountID: "acct-a",
		workspaceCwd:      "/repo",
		runKind:           "chat",
	})
	if state.ProviderSessionID != "thread-1" {
		t.Fatalf("ProviderSessionID = %q, want thread-1", state.ProviderSessionID)
	}
}

func TestResumeRunReconstructsChatRunFromDisk(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-a")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "rollout-1", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-1",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		LastPrompt:        "hello",
		LastMessage:       "done",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		RunKind:           "chat",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-a"
	if rs, err := svc.loadPersistedRun("run-1"); err != nil {
		t.Fatalf("loadPersistedRun: %v", err)
	} else if rs.realProviderSessionID != "rollout-1" {
		t.Fatalf("realProviderSessionID = %q, want rollout-1", rs.realProviderSessionID)
	}

	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.RunID != "run-1" || handle.StepID != "chat-run-1" {
		t.Fatalf("unexpected handle: %+v", handle)
	}
}

func TestResumeRunUsesInMemoryRunBeforeDiskLookup(t *testing.T) {
	store := newFakeWorkflowStore()
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-a")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "in-memory-session", "/repo", time.Now().UTC())
	store.sessions["run-1"] = ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "disk-session",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-a"
	svc.runs["run-1"] = &interactiveRun{
		id:                "run-1",
		projectID:         "project-1",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "in-memory-session",
		providerAccountID: "acct-a",
		workspaceCwd:      "/repo",
		status:            RunStatusRunning,
		runKind:           "chat",
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}

	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.ProviderSessionID != "in-memory-session" {
		t.Fatalf("ProviderSessionID = %q, want in-memory-session", handle.ProviderSessionID)
	}
	if svc.runs["run-1"].providerSessionID != "in-memory-session" {
		t.Fatalf("run providerSessionID = %q, want in-memory-session", svc.runs["run-1"].providerSessionID)
	}
}

func TestResumeRunRestoredWorkflowRunUnsupportedForMVP(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:     "run-workflow",
		ProjectID: "project-1",
		Status:    RunStatusCompleted,
		RunKind:   "workflow",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("run-workflow"); apiErr == nil || apiErr.code != "resume_unsupported" {
		t.Fatalf("resumeRun error = %#v, want resume_unsupported", apiErr)
	}
}

func TestLocateSessionFileCodexFindsRolloutBySessionID(t *testing.T) {
	home := t.TempDir()
	path := writeCodexRollout(t, home, "rollout-abc", "/repo", time.Now().UTC())
	got, found := LocateSessionFile(ProviderKeyCodex, home, "rollout-abc", "/repo")
	if !found {
		t.Fatal("expected rollout to be found")
	}
	if got != path {
		t.Fatalf("LocateSessionFile = %q, want %q", got, path)
	}
}

func TestDiscoverCodexRolloutSessionIDChoosesNewestMatchingCWD(t *testing.T) {
	home := t.TempDir()
	old := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	writeCodexRollout(t, home, "rollout-old", "/repo", old)
	writeCodexRollout(t, home, "rollout-new", "/repo", newer)
	writeCodexRollout(t, home, "rollout-other", "/other", newer.Add(time.Minute))
	got, found := DiscoverCodexRolloutSessionID(home, "/repo")
	if !found {
		t.Fatal("expected rollout discovery to succeed")
	}
	if got != "rollout-new" {
		t.Fatalf("DiscoverCodexRolloutSessionID = %q, want rollout-new", got)
	}
}

func TestRelocateSessionFileCodexCopiesRolloutWithoutOverwrite(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	src := writeCodexRollout(t, srcHome, "rollout-abc", "/repo", time.Now().UTC())
	unrelated := filepath.Join(targetHome, "sessions", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dst, err := RelocateSessionFile(ProviderKeyCodex, src, targetHome, "rollout-abc", "/repo")
	if err != nil {
		t.Fatalf("RelocateSessionFile: %v", err)
	}
	srcBytes, _ := os.ReadFile(src)
	dstBytes, _ := os.ReadFile(dst)
	if string(srcBytes) != string(dstBytes) {
		t.Fatalf("destination bytes differ from source")
	}
	if raw, _ := os.ReadFile(unrelated); string(raw) != "keep" {
		t.Fatalf("unrelated target file changed: %q", raw)
	}
}

func TestRelocateSessionFileDoesNotOverwriteExistingSessionFile(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	src := writeCodexRollout(t, srcHome, "rollout-abc", "/repo", time.Now().UTC())
	dst, err := relocationTargetPath(ProviderKeyCodex, src, targetHome, "rollout-abc", "/repo")
	if err != nil {
		t.Fatalf("relocationTargetPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(dst, []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := RelocateSessionFile(ProviderKeyCodex, src, targetHome, "rollout-abc", "/repo"); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	raw, _ := os.ReadFile(dst)
	if string(raw) != "target" {
		t.Fatalf("target bytes changed: %q", raw)
	}
}

func TestRelocateSessionFileAcceptsExistingIdenticalSessionFile(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	src := writeCodexRollout(t, srcHome, "rollout-abc", "/repo", time.Now().UTC())
	dst, err := relocationTargetPath(ProviderKeyCodex, src, targetHome, "rollout-abc", "/repo")
	if err != nil {
		t.Fatalf("relocationTargetPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile src: %v", err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := RelocateSessionFile(ProviderKeyCodex, src, targetHome, "rollout-abc", "/repo")
	if err != nil {
		t.Fatalf("RelocateSessionFile identical existing: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(dst) {
		t.Fatalf("got = %q, want %q", got, dst)
	}
}

func TestRelocateSessionFileClaudePreservesProjectHashDirectory(t *testing.T) {
	srcHome := t.TempDir()
	targetHome := t.TempDir()
	src := filepath.Join(srcHome, ".claude", "projects", "source-hash", "claude-real-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(src, []byte("claude"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dst, err := RelocateSessionFile(ProviderKeyClaude, src, targetHome, "claude-real-1", "/repo")
	if err != nil {
		t.Fatalf("RelocateSessionFile: %v", err)
	}
	want := filepath.Join(targetHome, ".claude", "projects", "source-hash", "claude-real-1.jsonl")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	raw, _ := os.ReadFile(dst)
	if string(raw) != "claude" {
		t.Fatalf("unexpected dst bytes: %q", raw)
	}
}

// TestRelocateSessionFileSameHomeCodexIsNoop verifies that when srcPath and the
// computed dstPath are the same file (both accounts share the same home directory),
// RelocateSessionFile returns srcPath without error — no copy is attempted.
func TestRelocateSessionFileSameHomeCodexIsNoop(t *testing.T) {
	sharedHome := t.TempDir()
	src := writeCodexRollout(t, sharedHome, "rollout-abc", "/repo", time.Now().UTC())
	got, err := RelocateSessionFile(ProviderKeyCodex, src, sharedHome, "rollout-abc", "/repo")
	if err != nil {
		t.Fatalf("RelocateSessionFile same-home: unexpected error: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(src) {
		t.Fatalf("got = %q, want %q (same file)", got, src)
	}
}

// TestRelocateSessionFileSameHomeClaudeIsNoop verifies same no-op for Claude sessions.
func TestRelocateSessionFileSameHomeClaudeIsNoop(t *testing.T) {
	sharedHome := t.TempDir()
	src := filepath.Join(sharedHome, ".claude", "projects", "hash1", "session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := RelocateSessionFile(ProviderKeyClaude, src, sharedHome, "session-1", "/repo")
	if err != nil {
		t.Fatalf("RelocateSessionFile same-home: unexpected error: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(src) {
		t.Fatalf("got = %q, want %q (same file)", got, src)
	}
}

// TestResumeRunSameHomeAccountRebindsProviderAccountID verifies that when a run was
// stamped with account "acct-old" but the active account "acct-new" resolves to the
// same home directory, resumeRun succeeds and rebinds the stored providerAccountID
// to "acct-new" so future resumes go through the same-account fast path.
func TestResumeRunSameHomeAccountRebindsProviderAccountID(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	sharedHome := filepath.Join(root, "shared-home")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-old", ProviderKey: "codex", HomePath: sharedHome, SlotIndex: 0, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-new", ProviderKey: "codex", HomePath: sharedHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, sharedHome)
	writeCodexRollout(t, sharedHome, "rollout-xyz", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-xyz",
		ProviderAccountID: "acct-old",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("run-1"); apiErr != nil {
		t.Fatalf("resumeRun: unexpected error: %v", apiErr)
	}
	// After success the store must have reboundProviderAccountID to "acct-new".
	st, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", st, found, err)
	}
	if st.ProviderAccountID != "acct-new" {
		t.Fatalf("ProviderAccountID = %q, want acct-new", st.ProviderAccountID)
	}
}

func TestPrepareCrossAccountResumeActiveAccountNotSignedIn(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-b")
	// acct-b is the active Codex account for this run, but its local auth is
	// missing on disk — so resume must refuse with account_not_signed_in even
	// though the run's session (rollout) file is present.
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexRollout(t, acctHome, "rollout-abc", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-abc",
		ProviderAccountID: "acct-b",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	if _, apiErr := svc.resumeRun("run-1"); apiErr == nil || apiErr.code != "account_not_signed_in" {
		t.Fatalf("resumeRun error = %#v, want account_not_signed_in", apiErr)
	}
}

func TestPrepareCrossAccountResumeMissingSourceFile(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctAHome)
	writeCodexAuth(t, acctBHome)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-missing",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	if _, apiErr := svc.resumeRun("run-1"); apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("resumeRun error = %#v, want session_unavailable", apiErr)
	}
}

func TestPrepareCrossAccountResumeSkippedForSameAccount(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-a")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "rollout-abc", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-abc",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-a"
	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.RunID != "run-1" {
		t.Fatalf("unexpected handle: %+v", handle)
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-a" {
		t.Fatalf("ProviderAccountID = %q, want acct-a", state.ProviderAccountID)
	}
}

func TestPrepareCrossAccountResumeRelocatesAndRepointsRun(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctAHome)
	writeCodexAuth(t, acctBHome)
	src := writeCodexRollout(t, acctAHome, "rollout-xyz", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-xyz",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.RunID != "run-1" {
		t.Fatalf("unexpected handle: %+v", handle)
	}

	dst, found := LocateSessionFile(ProviderKeyCodex, acctBHome, "rollout-xyz", "/repo")
	if !found {
		t.Fatal("expected relocated rollout in target home")
	}
	if dst == src {
		t.Fatal("expected relocation into target home")
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-b" {
		t.Fatalf("ProviderAccountID = %q, want acct-b", state.ProviderAccountID)
	}
}

func TestPrepareCrossAccountResumeRelocationFailureKeepsHistoryVisible(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctAHome)
	writeCodexAuth(t, acctBHome)
	src := writeCodexRollout(t, acctAHome, "rollout-dup", "/repo", time.Now().UTC())
	dst, err := relocationTargetPath(ProviderKeyCodex, src, acctBHome, "rollout-dup", "/repo")
	if err != nil {
		t.Fatalf("relocationTargetPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(dst, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-dup",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	if _, apiErr := svc.resumeRun("run-1"); apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("resumeRun error = %#v, want session_unavailable", apiErr)
	}
	history := svc.projectRunHistory("project-1")
	if len(history) != 1 || history[0].RunID != "run-1" {
		t.Fatalf("history = %+v, want run-1 visible", history)
	}
}

func TestPrepareCrossAccountResumeAcceptsExistingIdenticalTargetFile(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctAHome)
	writeCodexAuth(t, acctBHome)
	src := writeCodexRollout(t, acctAHome, "rollout-dup", "/repo", time.Now().UTC())
	dst, err := relocationTargetPath(ProviderKeyCodex, src, acctBHome, "rollout-dup", "/repo")
	if err != nil {
		t.Fatalf("relocationTargetPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile src: %v", err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-dup",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.RunID != "run-1" {
		t.Fatalf("unexpected handle: %+v", handle)
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-b" {
		t.Fatalf("ProviderAccountID = %q, want acct-b", state.ProviderAccountID)
	}
}

func TestPrepareCrossAccountResumeSupportsSwitchingBackAndForth(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "provider-accounts.json")
	acctAHome := filepath.Join(root, "acct-a")
	acctBHome := filepath.Join(root, "acct-b")
	writeProviderAccountsConfig(t, configPath, []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctAHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctBHome, SlotIndex: 2, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctAHome)
	writeCodexAuth(t, acctBHome)
	_ = writeCodexRollout(t, acctAHome, "rollout-xyz", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-xyz",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	runner := &Runner{}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.AttachRunner(runner)

	if _, err := runner.ActivateProviderAccount("acct-b"); err != nil {
		t.Fatalf("ActivateProviderAccount acct-b: %v", err)
	}
	if _, apiErr := svc.resumeRun("run-1"); apiErr != nil {
		t.Fatalf("resumeRun on acct-b: %v", apiErr)
	}
	state, found, err := store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession after acct-b = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-b" {
		t.Fatalf("ProviderAccountID after acct-b = %q, want acct-b", state.ProviderAccountID)
	}

	if _, err := runner.ActivateProviderAccount("acct-a"); err != nil {
		t.Fatalf("ActivateProviderAccount acct-a: %v", err)
	}
	if _, apiErr := svc.resumeRun("run-1"); apiErr != nil {
		t.Fatalf("resumeRun back on acct-a: %v", apiErr)
	}
	state, found, err = store.GetProviderSession(context.Background(), "run-1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession after acct-a = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderAccountID != "acct-a" {
		t.Fatalf("ProviderAccountID after acct-a = %q, want acct-a", state.ProviderAccountID)
	}
}

func TestRestoredCodexRunUsesCLIResumePath(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-b")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 2, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "rollout-abc", workspace, time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-abc",
		ProviderAccountID: "acct-b",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	var fallbackCalls atomic.Int32
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(context.Context, TurnRequest, TurnBridge) error {
				fallbackCalls.Add(1)
				return nil
			})
		},
	})

	envCapture := filepath.Join(root, "codex-home.txt")
	var gotArgs []string
	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		script := `out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; printf "%s" "$CODEX_HOME" > ` + strconv.Quote(envCapture) + `; [ -n "$out" ] && printf "cli final\n" > "$out"`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		cmd := exec.CommandContext(ctx, "sh", cmdArgs...)
		return cmd
	}
	defer func() { commandContextFn = originalCmd }()

	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	handle, apiErr := svc.resumeRun("run-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if _, apiErr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "continue"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "restored codex turn to finish")

	if fallbackCalls.Load() != 0 {
		t.Fatalf("expected restored run to bypass registry adapter SendTurn, got %d calls", fallbackCalls.Load())
	}
	if strings.Join(gotArgs[:3], " ") != "exec resume rollout-abc" {
		t.Fatalf("unexpected codex resume args: %v", gotArgs)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "continue") {
		t.Fatalf("expected prompt in args, got %v", gotArgs)
	}
	raw, err := os.ReadFile(envCapture)
	if err != nil {
		t.Fatalf("ReadFile env capture: %v", err)
	}
	if strings.TrimSpace(string(raw)) != acctHome {
		t.Fatalf("CODEX_HOME = %q, want %q", strings.TrimSpace(string(raw)), acctHome)
	}
}

// TestRestoredCodexRunSecondTurnStillUsesResumeSessionID verifies that the session id
// passed to `codex exec resume` remains stable across multiple turns of a resumed run —
// i.e. the second turn reuses the same rollout id, not a freshly-discovered one.
func TestRestoredCodexRunSecondTurnStillUsesResumeSessionID(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-b")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 2, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "rollout-abc", workspace, time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-multiturn",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-abc",
		ProviderAccountID: "acct-b",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Capture the session id arg from each turn invocation.
	var mu sync.Mutex
	var capturedSessionIDs []string

	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		// args[0]="exec" args[1]="resume" args[2]=<sessionID>
		if len(args) >= 3 && args[0] == "exec" && args[1] == "resume" {
			mu.Lock()
			capturedSessionIDs = append(capturedSessionIDs, args[2])
			mu.Unlock()
		}
		// Write a non-empty output so the adapter emits EventMessageCompleted.
		script := `out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; [ -n "$out" ] && printf "ok\n" > "$out"`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(context.Context, TurnRequest, TurnBridge) error { return nil })
		},
	})

	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"

	handle, apiErr := svc.resumeRun("run-multiturn")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}

	// Turn 1.
	if _, apiErr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "first turn"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn (turn 1): %v", apiErr)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "turn 1 to finish")

	// Turn 2 — must still resume the same rollout id.
	if _, apiErr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "second turn"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn (turn 2): %v", apiErr)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "turn 2 to finish")

	mu.Lock()
	ids := append([]string{}, capturedSessionIDs...)
	mu.Unlock()

	// The retry budget (maxTurnAttempts=3) may produce >1 exec call per turn when the
	// shell mock exits non-zero; the important invariant is that every call uses the
	// same session id and both turns fired at least once.
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 codex exec resume calls (one per turn), got %d", len(ids))
	}
	for i, id := range ids {
		if id != "rollout-abc" {
			t.Fatalf("call %d used session id %q, want rollout-abc (stable across turns)", i+1, id)
		}
	}
}

func TestRestoredCodexRunKeepsStablePersistedResumeIDAndLogsNewRollout(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-b")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-b", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 2, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	baseTime := time.Now().UTC().Add(-time.Hour)
	writeCodexRollout(t, acctHome, "rollout-abc", workspace, baseTime)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-stable",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-abc",
		ProviderAccountID: "acct-b",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-b"
	rs := &interactiveRun{
		id:                     "run-stable",
		projectID:              "project-1",
		providerKey:            ProviderKeyCodex,
		providerSessionID:      "rollout-abc",
		realProviderSessionID:  "rollout-abc",
		lastCodexTurnSessionID: "rollout-abc",
		providerAccountID:      "acct-b",
		workspaceCwd:           workspace,
		status:                 RunStatusCompleted,
		runKind:                "chat",
		createdAt:              baseTime.Format(time.RFC3339Nano),
		updatedAt:              baseTime.Format(time.RFC3339Nano),
	}
	writeCodexRollout(t, acctHome, "rollout-new", workspace, baseTime.Add(2*time.Hour))

	svc.mu.Lock()
	newSessionID := svc.refreshResumeHandleLocked(rs, nil)
	snap := sessionStateOf(rs)
	svc.mu.Unlock()
	if newSessionID != "rollout-new" {
		t.Fatalf("refreshResumeHandleLocked returned %q, want rollout-new", newSessionID)
	}
	if err := store.UpsertProviderSession(context.Background(), snap); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), rs.id, turnLogLine{Kind: turnLogKindCodexSession, SessionID: newSessionID}); err != nil {
		t.Fatalf("AppendTurnLog: %v", err)
	}

	state, found, err := store.GetProviderSession(context.Background(), "run-stable")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", state, found, err)
	}
	if state.ProviderSessionID != "rollout-abc" {
		t.Fatalf("ProviderSessionID = %q, want stable rollout-abc", state.ProviderSessionID)
	}
	entries, err := store.ReadTurnLog(context.Background(), "run-stable")
	if err != nil {
		t.Fatalf("ReadTurnLog: %v", err)
	}
	foundNew := false
	for _, entry := range entries {
		if entry.Kind == turnLogKindCodexSession && entry.SessionID == "rollout-new" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("expected turn log to include rollout-new, got %+v", entries)
	}
}

// TestRestoreTargetPathRejectsTraversal verifies that restoreTargetPath refuses
// relative paths that could escape the target home or point to the wrong provider
// subtree. The security property is that every dangerous path returns an error
// (regardless of the specific message, which varies by platform path separator).
func TestRestoreTargetPathRejectsTraversal(t *testing.T) {
	home := t.TempDir()
	reject := []struct {
		provider ProviderKey
		relPath  string
	}{
		// traversal — caught by the explicit "../" guard on POSIX, or by the provider
		// prefix check on Windows (where filepath.Clean converts "/" → "\").
		{ProviderKeyCodex, "../evil"},
		{ProviderKeyClaude, "../../etc/passwd"},
		{ProviderKeyCodex, ".."},
		// wrong subtree for provider
		{ProviderKeyCodex, ".claude/projects/abc/session.jsonl"},
		{ProviderKeyClaude, "sessions/2024/01/01/rollout-xyz.jsonl"},
		// empty / dot
		{ProviderKeyCodex, ""},
		{ProviderKeyCodex, "."},
	}
	for _, tc := range reject {
		_, err := restoreTargetPath(tc.provider, home, tc.relPath, "session-id", "/cwd")
		if err == nil {
			t.Errorf("restoreTargetPath(%q, %q): expected error, got nil", tc.provider, tc.relPath)
		}
	}

	// Positive cases — valid paths must succeed.
	accept := []struct {
		provider ProviderKey
		relPath  string
	}{
		{ProviderKeyCodex, "sessions/2024/01/01/rollout-abc.jsonl"},
		{ProviderKeyClaude, ".claude/projects/abc123/session.jsonl"},
	}
	for _, tc := range accept {
		_, err := restoreTargetPath(tc.provider, home, tc.relPath, "session-id", "/cwd")
		if err != nil {
			t.Errorf("restoreTargetPath(%q, %q): unexpected error: %v", tc.provider, tc.relPath, err)
		}
	}
}

// TS-011: resumeRun seeds a chat step into the workflow store after reconstructing from disk.
func TestResumeRunReconstructsChatRunSeedsChatStep(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	root := t.TempDir()
	acctHome := filepath.Join(root, "acct-a")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	writeCodexAuth(t, acctHome)
	writeCodexRollout(t, acctHome, "rollout-1", "/repo", time.Now().UTC())
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-1",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "rollout-1",
		ProviderAccountID: "acct-a",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-a"
	if _, apiErr := svc.resumeRun("run-1"); apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}

	steps, err := store.LoadRunSteps(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("LoadRunSteps = %d steps, want 1", len(steps))
	}
	if steps[0].ID != "chat-run-1" {
		t.Fatalf("step.ID = %q, want chat-run-1", steps[0].ID)
	}
	if steps[0].StepType != "chat" {
		t.Fatalf("step.StepType = %q, want chat", steps[0].StepType)
	}
}

// TS-013: resumeRun returns run_not_found when no persisted session exists.
func TestResumeRunMissingPersistedSessionReturnsRunNotFound(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	_, apiErr := svc.resumeRun("missing-run")
	if apiErr == nil {
		t.Fatal("expected error, got nil")
	}
	if apiErr.code != "run_not_found" {
		t.Fatalf("apiErr.code = %q, want run_not_found", apiErr.code)
	}

	svc.mu.Lock()
	_, exists := svc.runs["missing-run"]
	svc.mu.Unlock()
	if exists {
		t.Fatal("expected no run inserted into s.runs")
	}
}

// TS-015: resolveAccountHome returns the configured home path for an explicit account ID.
func TestResolveAccountHomeExplicitAccount(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "codex-a")
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	home, ok := svc.resolveAccountHome(ProviderKeyCodex, "acct-a")
	if !ok {
		t.Fatal("resolveAccountHome('acct-a'): expected ok=true")
	}
	if home != acctHome {
		t.Fatalf("home = %q, want %q", home, acctHome)
	}
}

// TS-016: resolveAccountHome falls back to DetectDefaultAccountHomePath when accountID is "" or "default".
func TestResolveAccountHomeDefaultAccountFallback(t *testing.T) {
	root := t.TempDir()
	// getPossibleHomeDirs() reads HOME/USERPROFILE; defaultAuthCandidates("codex", dir) checks
	// dir/.codex/auth.json. Writing auth into root/.codex/auth.json ensures the fallback finds it.
	writeCodexAuth(t, root)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	// No accounts in registry for codex — forces the "" / "default" fallback path.
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	home, ok := svc.resolveAccountHome(ProviderKeyCodex, "")
	if !ok || home == "" {
		t.Fatalf("resolveAccountHome(''): expected default codex home via DetectDefaultAccountHomePath, got (%q, %v)", home, ok)
	}
	home2, ok2 := svc.resolveAccountHome(ProviderKeyCodex, "default")
	if !ok2 || home2 == "" {
		t.Fatalf("resolveAccountHome('default'): expected default codex home via DetectDefaultAccountHomePath, got (%q, %v)", home2, ok2)
	}
}

// TS-017: resolveAccountHome returns ("", false) for an account ID that is neither in the registry
// nor the "" / "default" fallback sentinel.
func TestResolveAccountHomeMissingAccount(t *testing.T) {
	root := t.TempDir()
	writeProviderAccountsConfig(t, filepath.Join(root, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-a", ProviderKey: "codex", HomePath: filepath.Join(root, "acct-a"), SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	home, ok := svc.resolveAccountHome(ProviderKeyClaude, "acct-missing")
	if ok || home != "" {
		t.Fatalf("resolveAccountHome('acct-missing') = (%q, %v), want ('', false)", home, ok)
	}
}

// TS-019: LocateSessionFile returns ("", false) when no matching Codex rollout exists.
func TestLocateSessionFileCodexMissing(t *testing.T) {
	home := t.TempDir()
	path, found := LocateSessionFile(ProviderKeyCodex, home, "missing-id", "/repo")
	if found || path != "" {
		t.Fatalf("LocateSessionFile = (%q, %v), want ('', false)", path, found)
	}
}

// TS-020: LocateSessionFile finds a Claude project session file by session ID.
func TestLocateSessionFileClaudeFindsProjectSession(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".claude", "projects", "project-hash", "claude-real-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	path, found := LocateSessionFile(ProviderKeyClaude, home, "claude-real-1", "/repo")
	if !found {
		t.Fatal("expected session file to be found")
	}
	want := filepath.Join(".claude", "projects", "project-hash", "claude-real-1.jsonl")
	if !strings.HasSuffix(path, want) {
		t.Fatalf("path = %q, want suffix %q", path, want)
	}
}

type fakeAdapterFunc func(context.Context, TurnRequest, TurnBridge) error

func (f fakeAdapterFunc) Key() ProviderKey { return ProviderKeyCodex }
func (f fakeAdapterFunc) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, Resume: true}
}
func (f fakeAdapterFunc) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	return f(ctx, req, bridge)
}

func writeProviderAccountsConfig(t *testing.T, path string, accounts []ProviderAccount) {
	t.Helper()
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", path)
	// Isolate HOME so syncProviderAccounts does not inject the developer's real
	// ~/.codex (or ~/.claude) default account into the resolved list — without
	// this, ResolveProviderAccount("",provider) can return a real account and a
	// cross-account resume would relocate session files into the real home.
	t.Setenv("HOME", filepath.Dir(path))
	t.Setenv("USERPROFILE", filepath.Dir(path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	payload, err := json.Marshal(providerAccountState{Accounts: accounts})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func writeCodexAuth(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"id_token":"token"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func writeCodexRollout(t *testing.T, home, sessionID, cwd string, ts time.Time) string {
	t.Helper()
	dir := filepath.Join(home, "sessions", ts.Format("2006"), ts.Format("01"), ts.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+ts.Format("20060102T150405")+"-"+sessionID+".jsonl")
	line := map[string]any{
		"payload": map[string]any{
			"id":        sessionID,
			"timestamp": ts.Format(time.RFC3339Nano),
			"cwd":       cwd,
		},
	}
	raw, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}
