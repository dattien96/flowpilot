# 10 - Implementation Guide: Cross-PC Chat Sync for Non-Supabase Users

> **Audience:** an implementing agent that will write the code directly.
> **Goal:** let a user explicitly sync a local chat run from PC1 to Google Drive,
> restore it on PC2, and continue the chat when the provider session file is portable.
>
> This guide extends [Task-069](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md).
> It uses the cross-account resume foundation from
> [09-IG](./09-Cross-Account-Chat-Resume-Implementation-Guide.md), especially
> provider session file location, relocation, typed greyout errors, and Codex CLI resume.

Related task docs:
[Task-069](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md),
[Task-073](../../08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md),
[Task-074](../../08-Task/todo/Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md),
[Task-071](../../08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md),
[Task-072](../../08-Task/todo/Task-072-Cross-Account-Chat-Resume-Test-Signatures.md),
[Task-023](../../08-Task/done/Task-023-Sync-Artifact-With-Google.md),
[CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md).

---

## 0. Proven facts and scope boundary

- Non-Supabase desktop history is indexed by `.flowpilot/chats/sessions.ndjson`.
- Provider conversation content is not in FlowPilot. It lives in provider files under the provider account home.
- Codex rollout files are proven portable across accounts on one PC. Cross-PC is expected but still requires manual validation on a second machine.
- Claude portability remains unverified. Treat Claude restore as best-effort; if resume cannot be prepared, keep the synced run visible but disabled with a reason.
- Supabase-backed cross-PC sync remains Task-057. This guide is only for the local-file, non-Supabase path.

---

## 1. Scope

### In scope

- Chat runs only (`runKind == "chat"`).
- Explicit per-run "Sync to Drive" from PC1.
- Explicit "Restore from Drive" on PC2.
- Google Drive as the transport, reusing the artifact Google Drive connection from Task-023.
- Syncing two layers together:
  - the run index record derived from `sessions.ndjson`
  - the provider session file needed to resume the run
- Local run id collision handling during restore.
- Working-directory remap before resume if PC2's project path differs from PC1.
- Greyout-safe failure for unportable or missing provider data.

### Out of scope

- Automatic/background sync.
- Sync-all-runs sweep.
- Workflow-run reconstruction.
- Supabase table sync.
- Transcript injection fallback.
- Editing provider session file contents.

---

## 2. Decisions

- **D-1** Sync is explicit and per-run. No background sync, no sync-on-turn, no startup restore.
- **D-2** The Google Drive folder layout is deterministic:
  - `chat-sessions/_index/sessions.ndjson`
  - `chat-sessions/runs/<source_machine_id>/<source_run_id>/manifest.json`
  - `chat-sessions/runs/<source_machine_id>/<source_run_id>/<provider_key>/<provider_session_file>`
- **D-3** Do not upload the entire local `sessions.ndjson` blindly. Build a normalized one-line sync index record for the selected run, then merge it into the Drive index last-wins by `(source_machine_id, source_run_id)`.
- **D-4** Restore does not overwrite unrelated local run ids. If PC2 already has the same `run_id` from a different source, import as `sync-<short_machine_id>-<source_run_id>`.
- **D-5** Add backward-compatible sync metadata fields to the NDJSON record. Old records still load.
- **D-6** Provider session file bytes are copied as opaque bytes. FlowPilot may compute size/hash but never logs or parses conversation content.
- **D-7** Restore requires a valid active provider account on PC2. If auth is missing, return `account_not_signed_in`.
- **D-8** Restore requires a valid PC2 cwd. If the original cwd does not exist, return `cwd_remap_required` and let the desktop send the selected project path.
- **D-9** If provider file restore succeeds but provider resume later rejects it, the item remains visible and disabled with a server-provided reason.
- **D-10** The source run identity is preserved in metadata; the local restored run id may differ only to avoid collision.

---

## 3. Data model

### 3.1 NDJSON record extension

File: `apps/local-runner/internal/runner/local_file_session_store.go`

Add optional fields to `ndjsonSessionRecord` and `ProviderSessionState`:

```go
SourceMachineID string `json:"source_machine_id,omitempty"`
SourceRunID     string `json:"source_run_id,omitempty"`
RestoredFrom    string `json:"restored_from,omitempty"` // "google_drive" when imported
SyncStatus      string `json:"sync_status,omitempty"`   // local_only | synced | failed | restored
SyncUpdatedAt   string `json:"sync_updated_at,omitempty"`
```

Rules:

- For local-only runs, these fields are empty except `SyncStatus` may be omitted or `local_only`.
- When syncing a local run, source identity is:
  - `SourceMachineID = localMachineID`
  - `SourceRunID = RunID`
- When restoring on PC2:
  - `SourceMachineID` and `SourceRunID` come from the manifest
  - `RunID` is the local restored run id after collision resolution
  - `RestoredFrom = "google_drive"`
  - `SyncStatus = "restored"`

### 3.2 Machine id

New file: `apps/local-runner/internal/runner/machine_identity.go`

Create or read `.flowpilot/chats/machine.json`:

```json
{
  "machine_id": "mch_<random>",
  "created_at": "2026-06-17T00:00:00Z"
}
```

Requirements:

- Generate once with cryptographic random bytes.
- Keep stable across runner restarts on the same project.
- Never derive from hostnames, usernames, emails, or provider account ids.

### 3.3 Drive manifest

New file: `apps/local-runner/internal/runner/chat_session_sync.go`

```go
type ChatSessionSyncManifest struct {
    SchemaVersion       int               `json:"schemaVersion"`
    SourceMachineID     string            `json:"sourceMachineId"`
    SourceRunID         string            `json:"sourceRunId"`
    ProjectID           string            `json:"projectId"`
    WorkflowID          string            `json:"workflowId,omitempty"`
    ProviderKey         ProviderKey       `json:"providerKey"`
    ProviderSessionID   string            `json:"providerSessionId"`
    ProviderAccountID   string            `json:"providerAccountId,omitempty"`
    RunKind             string            `json:"runKind"`
    OriginalCwd         string            `json:"originalCwd,omitempty"`
    LastPrompt          string            `json:"lastPrompt,omitempty"`
    LastMessage         string            `json:"lastMessage,omitempty"`
    StartedAt           string            `json:"startedAt,omitempty"`
    UpdatedAt           string            `json:"updatedAt,omitempty"`
    SyncedAt            string            `json:"syncedAt"`
    ProviderFile        ChatSessionFile   `json:"providerFile"`
    Extra               map[string]string `json:"extra,omitempty"`
}

type ChatSessionFile struct {
    RelativePath   string `json:"relativePath"`
    DriveObjectID  string `json:"driveObjectId,omitempty"`
    SizeBytes      int64  `json:"sizeBytes"`
    SHA256         string `json:"sha256"`
}
```

The manifest is the authoritative remote description for one synced run.

---

## 4. Runner implementation phases

### Phase A - Drive client primitives

Files:

- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/artifact_cloud_storage.go`
- `apps/local-runner/internal/runner/artifact_google_drive_connection.go`

Tasks:

- Reuse `GetGoogleDriveArtifactConnectionStatus(projectID, "")`.
- Reuse `loadGoogleDriveCredentialByProject`.
- Reuse `refreshGoogleDriveAccessToken`.
- Reuse `ensureGoogleDriveFolderPath`, `findGoogleDriveFile`, `upsertGoogleDriveFile`, `downloadGoogleDriveFileByID`, and folder listing helpers.
- Add chat-session-specific wrappers:
  - `ensureChatSessionDriveRoot(ctx, projectID) (rootFolderID string, accessToken string, error)`
  - `uploadChatSessionFile(accessToken, parentID, name string, bytes []byte, appProperties map[string]string) (googleDriveFile, error)`
  - `downloadChatSessionFile(ctx, accessToken, objectID string) ([]byte, error)`

Do not duplicate OAuth state or create a separate Google Drive connection.

### Phase B - Local sync source

Files:

- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/session_file_locator.go`
- `apps/local-runner/internal/runner/interactive_resume.go`

Tasks:

- Add `BuildChatSessionSyncManifest(ctx, runID string) (ChatSessionSyncManifest, string, error)`.
- Load the run from `SessionHistoryReader.GetProviderSession`.
- Require `RunKind == "chat"`.
- Resolve provider account home with the same helper used by resume.
- Locate the provider session file with `LocateSessionFile`.
- Read file bytes only for hashing/upload; never log contents.
- Compute SHA-256 and file size.
- Use local machine id and source run id in the manifest.

Error mapping:

- missing run: `run_not_found`
- non-chat run: `resume_unsupported`
- missing provider file: `session_unavailable`
- missing active account home/auth when needed: existing account errors

### Phase C - Sync to Drive

Function:

```go
func (s *InteractiveService) syncChatRunToDrive(ctx context.Context, runID string, req ChatSessionSyncRequest) (ChatSessionSyncResult, *apiErr)
```

Request:

```go
type ChatSessionSyncRequest struct {
    GoogleDriveProjectID string `json:"googleDriveProjectId,omitempty"`
    GoogleDriveFolderID  string `json:"googleDriveFolderId,omitempty"`
}
```

Result:

```go
type ChatSessionSyncResult struct {
    RunID           string `json:"runId"`
    SourceMachineID string `json:"sourceMachineId"`
    SourceRunID     string `json:"sourceRunId"`
    SyncStatus      string `json:"syncStatus"`
    SyncedAt        string `json:"syncedAt"`
    RemotePath      string `json:"remotePath"`
}
```

Steps:

1. Build manifest and provider file bytes.
2. Ensure Drive folders:
   - `chat-sessions`
   - `chat-sessions/runs/<source_machine_id>/<source_run_id>/<provider_key>`
3. Upload provider file first.
4. Write object id, size, hash, and relative path into manifest.
5. Upload `manifest.json`.
6. Merge a one-line manifest summary into `chat-sessions/_index/sessions.ndjson`.
7. Upsert local `ProviderSessionState` with `SyncStatus = "synced"` and `SyncUpdatedAt`.

Index merge rule:

- Download existing index if present.
- Parse line by line.
- Drop malformed lines.
- Last-wins by `(source_machine_id, source_run_id)`.
- Replace only the selected run's record.
- Upload the full merged index atomically via Drive upsert.

### Phase D - Restore from Drive

Function:

```go
func (s *InteractiveService) restoreChatRunFromDrive(ctx context.Context, req ChatSessionRestoreRequest) (ChatSessionRestoreResult, *apiErr)
```

Request:

```go
type ChatSessionRestoreRequest struct {
    ProjectID        string `json:"projectId"`
    SourceMachineID string `json:"sourceMachineId"`
    SourceRunID     string `json:"sourceRunId"`
    Cwd             string `json:"cwd,omitempty"`
}
```

Result:

```go
type ChatSessionRestoreResult struct {
    RunID           string      `json:"runId"`
    SourceMachineID string      `json:"sourceMachineId"`
    SourceRunID     string      `json:"sourceRunId"`
    ProviderKey     ProviderKey `json:"providerKey"`
    RestoreStatus   string      `json:"restoreStatus"`
}
```

Steps:

1. Read `chat-sessions/_index/sessions.ndjson`.
2. Find the selected `(source_machine_id, source_run_id)`.
3. Download `manifest.json`.
4. Validate manifest schema, provider key, source ids, file hash metadata.
5. Resolve the target cwd:
   - use request cwd if non-empty
   - else use original cwd only if it exists on PC2
   - else return `cwd_remap_required`
6. Resolve active provider account home on PC2.
7. Download provider session file.
8. Verify SHA-256 and size.
9. Write file into the active account home using provider-specific restore target rules.
10. Refuse overwrite unless the existing file has the same hash.
11. Resolve local run id:
    - if source run id is unused locally, use it
    - if the same source identity was already restored, reuse that local run id
    - otherwise use `sync-<short_machine_id>-<source_run_id>`
12. Upsert `ProviderSessionState` into local store.
13. Return restored local run id.

Error mapping:

- missing Drive connection: `google_drive_not_connected`
- missing remote index/manifest/file: `sync_remote_not_found`
- cwd missing: `cwd_remap_required`
- active provider account missing: `account_unavailable`
- active provider auth missing: `account_not_signed_in`
- hash mismatch: `sync_integrity_failed`
- provider file write conflict: `session_file_conflict`

### Phase E - HTTP routes

File: `apps/local-runner/internal/runner/interactive_handlers.go`

Add routes:

```go
mux.HandleFunc("POST /client/workflow-runs/{runId}/sync-chat", s.handleSyncChatRun)
mux.HandleFunc("POST /client/chat-sessions/restore", s.handleRestoreChatRun)
mux.HandleFunc("GET /client/projects/{projectId}/chat-sessions/remote", s.handleListRemoteChatSessions)
```

The remote list route reads the Drive index and returns summaries for the desktop restore picker.

### Phase F - Desktop integration

Files:

- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/styles.css`

Tasks:

- Add contract DTOs for sync request/result, restore request/result, and remote chat session summary.
- Add `RunnerClient.syncChatRun`, `restoreChatRun`, and `listRemoteChatSessions`.
- Add store actions:
  - `syncHistoryRun(runId)`
  - `restoreRemoteChatSession(summary, cwd?)`
  - `loadRemoteChatSessions(projectId)`
- Add per-history-run sync action in the history list.
- Add restore view/action for remote sessions not present locally.
- On `cwd_remap_required`, use the selected project path as the first retry cwd when available.
- Show typed errors as disabled/unavailable reasons; keep current run/timeline intact.

---

## 5. Safety rules

- Never log provider session file bytes or manifest `lastPrompt` / `lastMessage` as raw content.
- Do not parse provider conversation JSONL except for provider-neutral metadata already used by locators.
- Do not overwrite local provider files unless hash matches.
- Do not replace the whole local `sessions.ndjson` during restore; always merge via `UpsertProviderSession`.
- Do not mutate unrelated history records during sync or restore.
- Do not add account grouping/badges to the history list.

---

## 6. Validation sequence

1. Unit tests for machine id generation and restored run id collision handling.
2. Unit tests for NDJSON sync metadata round-trip and backward compatibility.
3. Unit tests for Drive index merge.
4. Unit tests for sync manifest creation without logging file contents.
5. Unit tests for restore manifest validation, hash mismatch, cwd remap, overwrite conflict, and local store upsert.
6. HTTP handler tests for typed error mapping.
7. Desktop store tests for sync/restore actions and typed error behavior.
8. Manual same-account Google Drive flow:
   - PC1 sync Codex run.
   - PC2 restore into a different project path.
   - open restored run and send follow-up.
9. Manual unsupported-provider flow:
   - simulate missing provider file or rejected file.
   - verify disabled history item with reason.

---

## 7. Implementation order

1. Add machine identity and NDJSON metadata fields.
2. Add sync manifest and local run id collision helpers.
3. Add Drive index read/merge/write helpers using fakeable Drive primitives.
4. Add `syncChatRunToDrive`.
5. Add `restoreChatRunFromDrive`.
6. Add HTTP routes and DTOs.
7. Add desktop client/store/actions.
8. Add UI actions and disabled states.
9. Add focused tests from Task-074.
10. Run focused Go and desktop tests, then manual Drive validation when credentials are available.
