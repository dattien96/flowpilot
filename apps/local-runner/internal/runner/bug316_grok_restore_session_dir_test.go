package runner

// BUG-316: a Grok chat restored from Drive opened fine, but the very NEXT
// turn failed with a recoverable-error retry loop then a hard "Path not
// found." — Grok's own ACP `session/load` returned FS_NOT_FOUND. Root cause:
// Grok's session/load call reads the WHOLE session directory (events.jsonl,
// updates.jsonl, prompt_context.json, system_prompt.txt, summary.json,
// signals.json, resources_state.json, rewind_points.jsonl,
// announcement_state.json), not just chat_history.jsonl — but the sync
// manifest (and restore) only ever carried that one file. Confirmed live:
// comparing a session directory that was never deleted (14 files) against one
// rebuilt by restore (1 file) on the same real account/cwd. Deleting a chat
// deletes its provider session directory entirely, so ANY restored Grok chat
// hits this on its second turn.
//
// Fix: resolveGrokSessionSidecarFiles collects every other regular file in
// the session directory (skipping *.lock runtime markers); the manifest
// carries them in the new ProviderFiles field (Grok-only, empty for
// Codex/Claude); restore rewrites them all into the same directory as
// chat_history.jsonl.
//
// additive-tests-only: new file only, no existing test touched.
// cross-provider-parity: TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude
// proves the fix is scoped to Grok and does not change Codex/Claude sync at all.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeFullGrokSessionFixture creates chat_history.jsonl plus every sidecar
// file a real Grok ACP session directory has (matched against a live,
// never-deleted session directory during BUG-316 investigation), plus a
// zero-byte .lock marker that must NEVER be synced. Returns the sidecar
// bodies keyed by filename (chat_history.jsonl included) for later
// byte-comparison against the restored copy.
func writeFullGrokSessionFixture(t *testing.T, grokHome, cwd, sessionID string) map[string][]byte {
	t.Helper()
	dir := grokSessionDirPath(grokHome, cwd, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir grok session dir: %v", err)
	}
	files := map[string][]byte{
		"chat_history.jsonl":      []byte(`{"type":"user","content":[{"type":"text","text":"hi"}]}` + "\n"),
		"events.jsonl":            []byte(`{"type":"turn_start"}` + "\n"),
		"updates.jsonl":           []byte(`{"type":"agent_message_chunk","text":"hi"}` + "\n"),
		"prompt_context.json":     []byte(`{"contextTokens":123}`),
		"system_prompt.txt":       []byte("You are Grok."),
		"summary.json":            []byte(`{"turns":1}`),
		"signals.json":            []byte(`{"idle":false}`),
		"resources_state.json":    []byte(`{"resources":[]}`),
		"rewind_points.jsonl":     []byte(`{"turn":1}` + "\n"),
		"announcement_state.json": []byte(`{"seen":["1"]}`),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// Zero-byte runtime lock marker -- must never be synced (BUG-316 exclusion).
	if err := os.WriteFile(filepath.Join(dir, "chat_history.jsonl.lock"), nil, 0o644); err != nil {
		t.Fatalf("write lock marker: %v", err)
	}
	return files
}

// lastUploadedManifestFileID returns the most recently uploaded manifest.json
// file id (fake ids are sequential "chat-drive-N", assigned by nextID, so the
// highest N is the newest). Unlike remoteManifestFileID (which returns an
// arbitrary match), this is safe when one shared fakeChatDriveAPI accumulates
// manifests from more than one sync call in the same test (each run's
// manifest.json lives under its own run-id folder, so multiple same-named
// files coexist and uploadOrder tracks names, not ids).
func lastUploadedManifestFileID(api *fakeChatDriveAPI) string {
	bestID, bestNum := "", -1
	for id, f := range api.files {
		if f.Name != "manifest.json" {
			continue
		}
		num, err := strconv.Atoi(strings.TrimPrefix(id, "chat-drive-"))
		if err != nil {
			continue
		}
		if num > bestNum {
			bestNum, bestID = num, id
		}
	}
	return bestID
}

func registerGrokAccountForSync(t *testing.T, instance *Runner, workspace string) (grokHome, accountID string) {
	t.Helper()
	grokHome = filepath.Join(workspace, "grok-home")
	if err := os.MkdirAll(grokHome, 0o755); err != nil {
		t.Fatalf("mkdir grok home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grokHome, "auth.json"), []byte(`{"issuer::user":{"refresh_token":"rt","email":"g@example.com"}}`), 0o644); err != nil {
		t.Fatalf("write grok auth: %v", err)
	}
	accountID = "acct-grok-316"
	if err := instance.saveProviderAccountState(providerAccountState{Accounts: []ProviderAccount{
		{ID: "acct-sync", ProviderKey: "codex", DisplayName: "Account 1", HomePath: filepath.Join(workspace, "..", "codex-home"), SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: accountID, ProviderKey: "grok", DisplayName: "Grok 1", HomePath: grokHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}
	return grokHome, accountID
}

func TestSyncChatRunToDriveManifestCarriesGrokSessionSidecarFiles(t *testing.T) {
	svc, instance, store, api, workspace, _ := newChatSyncService(t)
	grokHome, grokAccountID := registerGrokAccountForSync(t, instance, workspace)

	const runID = "run-316-sync"
	const sessionID = "019f9000-0000-7000-8000-000000000001"
	writeFullGrokSessionFixture(t, grokHome, workspace, sessionID)

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: runID, ProjectID: "project-1", ProviderKey: ProviderKeyGrok,
		ProviderSessionID: sessionID, ProviderAccountID: grokAccountID,
		WorkingDirectory: workspace, RunKind: "chat", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T10:05:00Z",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	if _, apiErr := svc.syncChatRunToDrive(context.Background(), runID, ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	manifestID := remoteManifestFileID(api)
	if manifestID == "" {
		t.Fatal("expected manifest upload")
	}
	var manifest map[string]any
	if err := json.Unmarshal(api.files[manifestID].Content, &manifest); err != nil {
		t.Fatalf("manifest json invalid: %v", err)
	}
	rawFiles, ok := manifest["providerFiles"].([]any)
	if !ok {
		t.Fatalf("BUG-316: manifest providerFiles = %#v, want an array of sidecar files", manifest["providerFiles"])
	}
	// 9 sidecars: every file above except chat_history.jsonl (own top-level
	// providerFile field) and chat_history.jsonl.lock (excluded runtime lock).
	if len(rawFiles) != 9 {
		t.Fatalf("BUG-316: manifest providerFiles has %d entries, want 9 (got %#v)", len(rawFiles), rawFiles)
	}
	manifestJSON := string(api.files[manifestID].Content)
	if strings.Contains(manifestJSON, "chat_history.jsonl.lock") {
		t.Fatal("BUG-316: manifest must never carry the .lock runtime marker")
	}
	seenNames := map[string]bool{}
	for _, raw := range rawFiles {
		entry, _ := raw.(map[string]any)
		relPath, _ := entry["relativePath"].(string)
		if relPath == "" {
			t.Fatalf("sidecar entry missing relativePath: %#v", entry)
		}
		if strings.HasSuffix(relPath, "chat_history.jsonl") {
			t.Fatalf("BUG-316: chat_history.jsonl must not be duplicated into providerFiles: %q", relPath)
		}
		seenNames[filepath.Base(relPath)] = true
	}
	for _, want := range []string{"events.jsonl", "updates.jsonl", "prompt_context.json", "system_prompt.txt", "summary.json", "signals.json", "resources_state.json", "rewind_points.jsonl", "announcement_state.json"} {
		if !seenNames[want] {
			t.Fatalf("BUG-316: manifest providerFiles missing %q, got %v", want, seenNames)
		}
	}
}

// TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles is the live-repro
// round trip: sync a full Grok session directory, delete the ENTIRE directory
// (matching what a real chat delete does — confirmed live during BUG-316
// investigation, not just chat_history.jsonl), restore onto the same account
// under a different target cwd, and assert every sidecar file exists at the
// restored location byte-identical to the original. Byte-identical sidecars
// are exactly what makes Grok's own session/load able to resume — this is the
// property whose absence produced the live "Path not found." failure.
func TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles(t *testing.T) {
	svc, instance, store, _, workspace, _ := newChatSyncService(t)
	grokHome, grokAccountID := registerGrokAccountForSync(t, instance, workspace)

	const runID = "run-316-restore"
	const sessionID = "019f9000-0000-7000-8000-000000000002"
	sourceCwd := filepath.Join(workspace, "source-project")
	if err := os.MkdirAll(sourceCwd, 0o755); err != nil {
		t.Fatalf("mkdir source cwd: %v", err)
	}
	original := writeFullGrokSessionFixture(t, grokHome, sourceCwd, sessionID)

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: runID, ProjectID: "project-1", ProviderKey: ProviderKeyGrok,
		ProviderSessionID: sessionID, ProviderAccountID: grokAccountID,
		WorkingDirectory: sourceCwd, RunKind: "chat", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T10:05:00Z",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	result, apiErr := svc.syncChatRunToDrive(context.Background(), runID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Simulate a real delete: the whole session directory is gone, exactly
	// like FlowPilot's own chat-delete (confirmed live during investigation).
	if err := os.RemoveAll(grokSessionDirPath(grokHome, sourceCwd, sessionID)); err != nil {
		t.Fatalf("RemoveAll session dir: %v", err)
	}
	if err := store.DeleteProviderSession(context.Background(), runID); err != nil {
		t.Fatalf("DeleteProviderSession: %v", err)
	}

	targetCwd := filepath.Join(workspace, "target-project")
	if err := os.MkdirAll(targetCwd, 0o755); err != nil {
		t.Fatalf("mkdir target cwd: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID: "project-1", SourceMachineID: result.SourceMachineID, SourceRunID: result.SourceRunID,
		Cwd: targetCwd,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %#v", apiErr)
	}
	got, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%s): found=%v err=%v", restored.RunID, found, err)
	}

	restoredDir := grokSessionDirPath(grokHome, targetCwd, got.ProviderSessionID)
	for name, wantBody := range original {
		gotBody, readErr := os.ReadFile(filepath.Join(restoredDir, name))
		if readErr != nil {
			t.Fatalf("BUG-316: restored session dir missing %q (%v) — this is exactly what makes Grok's session/load fail FS_NOT_FOUND on the next turn", name, readErr)
		}
		if string(gotBody) != string(wantBody) {
			t.Fatalf("BUG-316: restored %q = %q, want %q", name, gotBody, wantBody)
		}
	}
	if _, err := os.Stat(filepath.Join(restoredDir, "chat_history.jsonl.lock")); err == nil {
		t.Fatal("BUG-316: restore must never recreate the .lock runtime marker")
	}
}

// TestRestoreChatRunFromDriveGrokSidecarConflictsWithDifferentLocalCopy proves
// restore never silently clobbers a sidecar file that already exists locally
// with different content (mirrors the primary file's own BUG-091 strictness).
func TestRestoreChatRunFromDriveGrokSidecarConflictsWithDifferentLocalCopy(t *testing.T) {
	svc, instance, store, _, workspace, _ := newChatSyncService(t)
	grokHome, grokAccountID := registerGrokAccountForSync(t, instance, workspace)

	const runID = "run-316-conflict"
	const sessionID = "019f9000-0000-7000-8000-000000000003"
	sourceCwd := filepath.Join(workspace, "source-project-2")
	if err := os.MkdirAll(sourceCwd, 0o755); err != nil {
		t.Fatalf("mkdir source cwd: %v", err)
	}
	writeFullGrokSessionFixture(t, grokHome, sourceCwd, sessionID)

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: runID, ProjectID: "project-1", ProviderKey: ProviderKeyGrok,
		ProviderSessionID: sessionID, ProviderAccountID: grokAccountID,
		WorkingDirectory: sourceCwd, RunKind: "chat", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T10:05:00Z",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	result, apiErr := svc.syncChatRunToDrive(context.Background(), runID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Restoring onto the SAME machine/cwd (no delete) means the target
	// directory already has every sidecar file with IDENTICAL content -- this
	// must succeed as a no-op, not a conflict.
	sameRestore, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID: "project-1", SourceMachineID: result.SourceMachineID, SourceRunID: result.SourceRunID,
		Cwd: sourceCwd,
	})
	if apiErr != nil {
		t.Fatalf("same-content restore should succeed as a no-op, got: %#v", apiErr)
	}
	if sameRestore.RunID == "" {
		t.Fatal("expected a restored run id")
	}

	// Now corrupt one sidecar file locally (as if a live Grok process wrote
	// something different there) and restore again -- must hard-conflict, not
	// silently overwrite a file a live process might be using.
	dir := grokSessionDirPath(grokHome, sourceCwd, sessionID)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.txt"), []byte("DIFFERENT local content"), 0o644); err != nil {
		t.Fatalf("corrupt sidecar: %v", err)
	}
	if err := store.DeleteProviderSession(context.Background(), sameRestore.RunID); err != nil {
		t.Fatalf("DeleteProviderSession: %v", err)
	}
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID: "project-1", SourceMachineID: result.SourceMachineID, SourceRunID: result.SourceRunID,
		Cwd: sourceCwd,
	})
	if apiErr == nil {
		t.Fatal("BUG-316: restore must conflict when a local sidecar file differs from the remote one, not silently overwrite it")
	}
	if apiErr.code != "session_file_conflict" {
		t.Fatalf("apiErr.code = %q, want session_file_conflict", apiErr.code)
	}
}

// TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude is the
// cross-provider-parity guard: BUG-316 is Grok-only (Codex/Claude resume via
// a single file FlowPilot itself replays, not a provider-owned stateful
// directory) — their manifests must never populate ProviderFiles.
func TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude(t *testing.T) {
	svc, instance, store, api, workspace, codexAccountHome := newChatSyncService(t)
	claudeHome := filepath.Join(filepath.Dir(codexAccountHome), "claude-home-316")
	if err := os.MkdirAll(filepath.Join(claudeHome, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir claude home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o644); err != nil {
		t.Fatalf("write claude credentials: %v", err)
	}
	if err := instance.saveProviderAccountState(providerAccountState{Accounts: []ProviderAccount{
		{ID: "acct-sync", ProviderKey: "codex", DisplayName: "Account 1", HomePath: codexAccountHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "acct-claude-316", ProviderKey: "claude", DisplayName: "Claude 1", HomePath: claudeHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}

	// Codex: seedLocalChatRun already builds a valid rollout-shaped session
	// under codexAccountHome for account "acct-sync".
	seedLocalChatRun(t, store, codexAccountHome, workspace, "run-316-codex-noside", []byte("transcript body"))
	// Claude: a session file under <home>/.claude/projects/<any>/<id>.jsonl
	// (mirrors LocateSessionFile's ProviderKeyClaude walk).
	const claudeSessionID = "316-claude-session"
	claudeProjectDir := filepath.Join(claudeHome, ".claude", "projects", "project")
	if err := os.MkdirAll(claudeProjectDir, 0o755); err != nil {
		t.Fatalf("mkdir claude project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeProjectDir, claudeSessionID+".jsonl"), []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write claude transcript: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-316-claude-noside", ProjectID: "project-1", ProviderKey: ProviderKeyClaude,
		ProviderSessionID: claudeSessionID, ProviderAccountID: "acct-claude-316",
		WorkingDirectory: workspace, RunKind: "chat", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T10:05:00Z",
	}); err != nil {
		t.Fatalf("UpsertProviderSession(claude): %v", err)
	}

	for _, tc := range []struct {
		provider ProviderKey
		runID    string
	}{
		{ProviderKeyCodex, "run-316-codex-noside"},
		{ProviderKeyClaude, "run-316-claude-noside"},
	} {
		t.Run(string(tc.provider), func(t *testing.T) {
			if _, apiErr := svc.syncChatRunToDrive(context.Background(), tc.runID, ChatSessionSyncRequest{}); apiErr != nil {
				t.Fatalf("syncChatRunToDrive(%s) failed: %v", tc.provider, apiErr)
			}
			manifestID := lastUploadedManifestFileID(api)
			if manifestID == "" {
				t.Fatal("expected manifest upload")
			}
			var manifest map[string]any
			if err := json.Unmarshal(api.files[manifestID].Content, &manifest); err != nil {
				t.Fatalf("manifest json invalid: %v", err)
			}
			if _, present := manifest["providerFiles"]; present {
				t.Fatalf("BUG-316 cross-provider guard: %s manifest must never carry providerFiles, got %#v", tc.provider, manifest["providerFiles"])
			}
		})
	}
}
