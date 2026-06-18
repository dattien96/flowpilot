# Task-074: Cross-PC Non-Supabase Chat Sync Test Signatures

## Metadata

- Document ID: `Task-074`
- Title: `Cross-PC Non-Supabase Chat Sync Test Signatures`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-073: Cross-PC Non-Supabase Chat Sync Definition of Done Checklist](./Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md), [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- Child Documents: `none`
- Related Documents: [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [Task-072: Cross-Account Chat Resume Test Signatures](./Task-072-Cross-Account-Chat-Resume-Test-Signatures.md), [Task-023: Sync Artifact With Google Drive](../done/Task-023-Sync-Artifact-With-Google.md)
- Replaces: `none`
- Tags: `desktop, local-runner, tests, tdd, sync, google-drive, cross-pc, ndjson`

## AI Quick View

### Summary

- This file defines the test signatures for Task-073.
- Automated tests must use temp dirs, fake Drive APIs, fake stores, and fake provider homes.
- Real Google Drive/provider-token tests are manual or opt-in only.
- Tests focus on sync index correctness, provider-file safety, restore collision behavior, typed errors, and desktop state transitions.

### Current Ask

- Implement these tests as the acceptance harness for Task-073 before calling Task-069 complete.

### Key Decisions

- `T-1` No default test may require live Google Drive, provider auth, or a second physical PC.
- `T-2` Drive upload/download behavior should use the existing fake Google Drive test pattern from artifact sync tests.
- `T-3` Provider file contents are test fixture bytes; tests must not log them.
- `T-4` Manual/e2e tests are skipped unless explicitly enabled.

### Constraints

- Keep test fixtures deterministic.
- Use `t.TempDir()` for provider homes and session stores.
- Do not make network calls in unit tests.
- Desktop tests use the existing Node test style unless a React render harness already exists.

### Open Questions

- `Q-1` Exact UI component test coverage depends on whether the repo adds a renderer harness.
- `Q-2` Live cross-PC Codex validation requires a second machine or an equivalent isolated environment.

### Source Refs

- [Task-073](./Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md)
- [10-IG](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- [Task-072](./Task-072-Cross-Account-Chat-Resume-Test-Signatures.md)
- `apps/local-runner/internal/runner/artifacts_sync_test.go`
- `apps/local-runner/internal/runner/cross_account_resume_test.go`
- `apps/desktop-flowpilot/src/state/store.test.ts`

## 1. Goal

Define the exact automated and manual tests required to validate explicit cross-PC sync and restore for non-Supabase chat runs.

## 2. Parent Links

- coding plan: [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-073 `DOD-001` through `DOD-098`

## 3. Trigger

Task-073 defines the DoD for Task-069. This document turns that checklist into testable signatures so implementation can be built against a concrete harness.

## 4. Exact Change

### Machine Identity Tests

#### [x] `TS-001` Machine identity is generated once

- target file: `apps/local-runner/internal/runner/machine_identity_test.go`
- signature: `func TestMachineIdentityGeneratedOnce(t *testing.T)`
- expected input:
  - temp `.flowpilot/chats` directory
  - call machine identity loader twice
- expected output:
  - both calls return same non-empty id
  - `machine.json` exists
- covers: `DOD-012`, `DOD-014`

#### [x] `TS-002` Machine identity contains no PII-derived fields

- target file: `apps/local-runner/internal/runner/machine_identity_test.go`
- signature: `func TestMachineIdentityDoesNotUsePII(t *testing.T)`
- expected input:
  - generated machine identity
- expected output:
  - id uses expected random prefix/format
  - file does not contain hostname, username, provider account id, or email fields
- covers: `DOD-013`

### NDJSON Metadata Tests

#### [x] `TS-003` Sync metadata round-trips through local session store

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreSyncMetadataRoundTrip(t *testing.T)`
- expected input:
  - `ProviderSessionState` with source machine/run metadata and `SyncStatus:"restored"`
  - upsert, reload, get provider session
- expected output:
  - metadata fields survive reload
- covers: `DOD-008`, `DOD-009`, `DOD-011`, `DOD-052`

#### [x] `TS-004` Legacy NDJSON without sync metadata still loads

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreLoadsLegacyRecordWithoutSyncMetadata(t *testing.T)`
- expected input:
  - old NDJSON line without source/sync fields
- expected output:
  - load succeeds
  - sync metadata fields are empty
- covers: `DOD-010`

### Remote Index Tests

#### [x] `TS-005` Drive index merge is last-wins by source identity

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestMergeChatSessionDriveIndexLastWinsBySourceIdentity(t *testing.T)`
- expected input:
  - existing index with two records
  - replacement record with same source machine/run and newer `syncedAt`
- expected output:
  - output has one record for that source identity
  - unrelated records preserved
- covers: `DOD-017`, `DOD-018`

#### [x] `TS-006` Drive index merge ignores malformed lines

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestMergeChatSessionDriveIndexIgnoresMalformedLines(t *testing.T)`
- expected input:
  - index bytes with malformed JSON lines and valid lines
- expected output:
  - valid lines merge
  - malformed lines do not fail the merge
- covers: `DOD-019`

#### [x] `TS-007` Drive path builder uses safe deterministic segments

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestChatSessionDrivePathsUseSafeSegments(t *testing.T)`
- expected input:
  - source machine id and run id containing unsafe separators
- expected output:
  - generated logical paths contain sanitized segments only
- covers: `DOD-022`, `DOD-074`

### Sync Manifest Tests

#### [x] `TS-008` Build sync manifest hashes provider file

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestBuildChatSessionSyncManifestHashesProviderFile(t *testing.T)`
- expected input:
  - temp local session store with chat run
  - fake provider home with provider session file bytes
- expected output:
  - manifest has provider key/session id/run metadata
  - file size and SHA-256 match fixture bytes
- covers: `DOD-020`, `DOD-026`, `DOD-028`

#### [x] `TS-009` Build sync manifest rejects missing run

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestBuildChatSessionSyncManifestMissingRun(t *testing.T)`
- expected input:
  - empty session store
  - build manifest for missing run
- expected output:
  - typed error code `run_not_found`
- covers: `DOD-023`, `DOD-024`

#### [x] `TS-010` Build sync manifest rejects non-chat run

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestBuildChatSessionSyncManifestRejectsNonChatRun(t *testing.T)`
- expected input:
  - session state with `RunKind:"workflow"`
- expected output:
  - typed error code `resume_unsupported`
- covers: `DOD-002`, `DOD-025`

#### [x] `TS-011` Build sync manifest rejects missing provider file

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestBuildChatSessionSyncManifestMissingProviderFile(t *testing.T)`
- expected input:
  - valid chat session pointing to absent provider file
- expected output:
  - typed error code `session_unavailable`
- covers: `DOD-027`

### Sync Upload Tests

#### [x] `TS-012` Sync uploads provider file before manifest and index

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestSyncChatRunToDriveUploadOrdering(t *testing.T)`
- expected input:
  - fake Drive API recording upsert order
  - valid local chat session and provider file
- expected output:
  - provider file upload occurs first
  - manifest upload occurs before index upload
- covers: `DOD-029`, `DOD-030`

#### [x] `TS-013` Sync updates local session sync status after success

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestSyncChatRunToDriveUpdatesLocalSyncStatus(t *testing.T)`
- expected input:
  - successful fake Drive sync
- expected output:
  - local `ProviderSessionState.SyncStatus == "synced"`
  - `SyncUpdatedAt` is non-empty
- covers: `DOD-031`

#### [x] `TS-014` Sync failure preserves prior successful metadata

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestSyncChatRunToDriveFailurePreservesPriorMetadata(t *testing.T)`
- expected input:
  - local session already marked synced
  - fake Drive fails during provider upload
- expected output:
  - previous sync metadata remains available
  - typed error returned
- covers: `DOD-032`, `DOD-033`

#### [x] `TS-015` Sync requires Google Drive connection

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestSyncChatRunToDriveRequiresGoogleDriveConnection(t *testing.T)`
- expected input:
  - no Drive connection state for project
- expected output:
  - typed error code `google_drive_not_connected`
- covers: `DOD-006`

### Remote List Tests

#### [x] `TS-016` List remote chat sessions reads Drive index

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestListRemoteChatSessionsReadsDriveIndex(t *testing.T)`
- expected input:
  - fake Drive index with two manifest summaries
- expected output:
  - returns two remote summaries
  - no provider file bytes are downloaded
- covers: `DOD-034`, `DOD-057`

#### [x] `TS-017` List remote chat sessions handles missing index as empty

- target file: `apps/local-runner/internal/runner/chat_session_sync_test.go`
- signature: `func TestListRemoteChatSessionsMissingIndexIsEmpty(t *testing.T)`
- expected input:
  - fake Drive returns not found for index
- expected output:
  - returns empty list and no error
- covers: `DOD-034`

### Restore Validation Tests

#### [x] `TS-018` Restore downloads and validates manifest

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveValidatesManifest(t *testing.T)`
- expected input:
  - fake Drive index and manifest
- expected output:
  - manifest schema/source ids/provider key accepted
- covers: `DOD-021`, `DOD-035`

#### [x] `TS-019` Restore maps missing remote files to sync_remote_not_found

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveMissingRemoteFile(t *testing.T)`
- expected input:
  - index and manifest exist
  - provider file object missing
- expected output:
  - typed error code `sync_remote_not_found`
- covers: `DOD-036`

#### [x] `TS-020` Restore verifies provider file hash

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveHashMismatch(t *testing.T)`
- expected input:
  - manifest hash differs from downloaded bytes
- expected output:
  - typed error code `sync_integrity_failed`
  - no local provider file written
- covers: `DOD-037`, `DOD-038`

#### [x] `TS-021` Restore requires cwd remap when original cwd is missing

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveRequiresCwdRemap(t *testing.T)`
- expected input:
  - manifest original cwd does not exist
  - request cwd empty
- expected output:
  - typed error code `cwd_remap_required`
- covers: `DOD-042`, `DOD-043`

#### [x] `TS-022` Restore uses request cwd when supplied

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveUsesRequestCwd(t *testing.T)`
- expected input:
  - missing original cwd
  - request cwd points to temp project
- expected output:
  - restored session `WorkingDirectory` equals request cwd
- covers: `DOD-042`, `DOD-067`

#### [x] `TS-023` Restore requires active provider account home

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveMissingActiveAccountHome(t *testing.T)`
- expected input:
  - no active account home for provider key
- expected output:
  - typed error code `account_unavailable`
- covers: `DOD-039`, `DOD-040`

#### [x] `TS-024` Restore requires active provider auth

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveMissingActiveAccountAuth(t *testing.T)`
- expected input:
  - active account home exists without auth
- expected output:
  - typed error code `account_not_signed_in`
- covers: `DOD-041`

#### [x] `TS-025` Restore refuses overwrite with different file bytes

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveRefusesOverwriteWithDifferentBytes(t *testing.T)`
- expected input:
  - target provider file already exists with different hash
- expected output:
  - typed error code `session_file_conflict`
  - existing file remains unchanged
- covers: `DOD-045`, `DOD-080`

#### [x] `TS-026` Restore accepts existing identical provider file

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveAcceptsExistingIdenticalFile(t *testing.T)`
- expected input:
  - target provider file already exists with same hash
- expected output:
  - restore succeeds without rewriting different bytes
- covers: `DOD-046`

### Run ID Collision Tests

#### [x] `TS-027` Restore uses source run id when unused

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestResolveRestoredRunIDUsesSourceWhenUnused(t *testing.T)`
- expected input:
  - local store has no `run-1`
  - manifest source run id `run-1`
- expected output:
  - restored local run id is `run-1`
- covers: `DOD-049`

#### [x] `TS-028` Restore reuses existing restored id for same source

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestResolveRestoredRunIDReusesSameSourceIdentity(t *testing.T)`
- expected input:
  - local store has restored record with same source machine/run
- expected output:
  - same local run id is returned
- covers: `DOD-050`

#### [x] `TS-029` Restore creates imported id for unrelated collision

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestResolveRestoredRunIDCreatesImportedIDForCollision(t *testing.T)`
- expected input:
  - local store already has unrelated `run-1`
  - manifest source run id `run-1`
- expected output:
  - restored id starts with `sync-`
  - local unrelated `run-1` remains unchanged
- covers: `DOD-051`, `DOD-081`

#### [x] `TS-030` Restore upserts one local provider session

- target file: `apps/local-runner/internal/runner/chat_session_restore_test.go`
- signature: `func TestRestoreChatRunFromDriveUpsertsOneLocalSession(t *testing.T)`
- expected input:
  - successful restore
- expected output:
  - one restored local `ProviderSessionState`
  - source metadata preserved
  - `SyncStatus == "restored"`
- covers: `DOD-047`, `DOD-048`, `DOD-052`, `DOD-053`

### HTTP Handler Tests

#### [x] `TS-031` Sync route returns sync result

- target file: `apps/local-runner/internal/runner/chat_session_handlers_test.go`
- signature: `func TestHandleSyncChatRunReturnsSyncResult(t *testing.T)`
- expected input:
  - HTTP `POST /client/workflow-runs/run-1/sync-chat`
- expected output:
  - status 200
  - response includes `runId`, `sourceMachineId`, `sourceRunId`, `syncStatus`
- covers: `DOD-055`, `DOD-058`

#### [x] `TS-032` Restore route returns restored local run id

- target file: `apps/local-runner/internal/runner/chat_session_handlers_test.go`
- signature: `func TestHandleRestoreChatRunReturnsLocalRunID(t *testing.T)`
- expected input:
  - HTTP `POST /client/chat-sessions/restore`
- expected output:
  - status 200
  - response includes restored local `runId`
- covers: `DOD-056`, `DOD-058`

#### [x] `TS-033` Remote list route returns summaries

- target file: `apps/local-runner/internal/runner/chat_session_handlers_test.go`
- signature: `func TestHandleListRemoteChatSessionsReturnsSummaries(t *testing.T)`
- expected input:
  - HTTP `GET /client/projects/project-1/chat-sessions/remote`
- expected output:
  - status 200
  - summaries do not include provider file bytes
- covers: `DOD-057`, `DOD-058`

### Desktop Store Tests

#### [x] `TS-034` Store syncHistoryRun calls client and marks syncing state

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `test("syncHistoryRun calls client and marks sync status", async () => { ... })`
- expected input:
  - fake client resolves sync result
  - store has local history item
- expected output:
  - client called with run id
  - history item sync status becomes `synced`
- covers: `DOD-062`, `DOD-063`, `DOD-083`

#### [x] `TS-035` Store syncHistoryRun typed error preserves current timeline

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `test("syncHistoryRun typed error preserves current timeline", async () => { ... })`
- expected input:
  - fake client rejects `session_unavailable`
  - current run/timeline set
- expected output:
  - current run/timeline unchanged
  - history item receives unavailable/sync error reason
- covers: `DOD-066`, `DOD-068`

#### [x] `TS-036` Store loadRemoteChatSessions populates remote summaries

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `test("loadRemoteChatSessions stores remote summaries", async () => { ... })`
- expected input:
  - fake client returns remote summaries
- expected output:
  - store exposes remote summaries for restore UI
- covers: `DOD-064`

#### [x] `TS-037` Store restoreRemoteChatSession adds restored run to local history

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `test("restoreRemoteChatSession adds restored run to history", async () => { ... })`
- expected input:
  - fake client returns restored run id
- expected output:
  - history includes restored run
  - selected provider matches provider key
- covers: `DOD-065`

#### [x] `TS-038` Store restoreRemoteChatSession retries cwd remap with selected project path

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `test("restoreRemoteChatSession retries cwd remap with selected project path", async () => { ... })`
- expected input:
  - fake client first rejects `cwd_remap_required`
  - selected project has path
- expected output:
  - second restore call includes selected project path
- covers: `DOD-067`

### Contract And Typecheck Tests

#### [x] `TS-039` RunnerClient includes chat sync methods

- target file: `apps/desktop-flowpilot/src/types/contract.ts`
- signature: `TypeScript compile-time contract`
- expected input:
  - `RunnerClient` implementation in `HttpWsRunnerClient`
- expected output:
  - `npm run typecheck` passes
- covers: `DOD-059`, `DOD-060`, `DOD-061`, `DOD-085`

### Manual And Opt-In E2E

#### [ ] `TS-040` Manual Codex PC1 sync to Drive

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Codex PC1 sync to Drive`
- expected input:
  - valid Google Drive connection
  - valid Codex chat run on PC1
- expected output:
  - Drive contains index, manifest, and provider file
- covers: `DOD-087`

#### [ ] `TS-041` Manual Codex PC2 restore from Drive

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Codex PC2 restore from Drive`
- expected input:
  - second machine or isolated home
  - same Drive folder
- expected output:
  - restore creates one local history item
  - restored provider file exists under active account home
- covers: `DOD-088`

#### [ ] `TS-042` Manual restored Codex follow-up

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Restored Codex run sends follow-up`
- expected input:
  - restored run from `TS-041`
  - active Codex account signed in
- expected output:
  - opening restored run and sending a prompt completes
- covers: `DOD-089`

#### [ ] `TS-043` Manual different cwd restore

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Different cwd restore`
- expected input:
  - original PC1 cwd absent on PC2
  - selected project path differs
- expected output:
  - restore uses remapped cwd
  - run opens from remapped project path
- covers: `DOD-090`

#### [ ] `TS-044` Manual missing provider or remote file greyout

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Missing provider/remote file greyout`
- expected input:
  - delete remote provider file or local restored file
  - attempt restore/open
- expected output:
  - item remains visible and disabled with reason
- covers: `DOD-091`

#### [ ] `TS-045` Manual missing active provider auth greyout

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Missing active provider auth greyout`
- expected input:
  - restore into provider home without auth
- expected output:
  - typed `account_not_signed_in` reason shown
- covers: `DOD-092`

#### [ ] `TS-046` Manual Claude provider status

- target file: `requirements/08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md`
- signature: `Manual: Claude provider status`
- expected input:
  - real Claude session file copied through sync/restore flow when available
- expected output:
  - record pass/fail/provider-untested in completion notes
- covers: `DOD-093`

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/machine_identity_test.go`
  - `apps/local-runner/internal/runner/local_file_session_store_test.go`
  - `apps/local-runner/internal/runner/chat_session_sync_test.go`
  - `apps/local-runner/internal/runner/chat_session_restore_test.go`
  - `apps/local-runner/internal/runner/chat_session_handlers_test.go`
  - `apps/desktop-flowpilot/src/state/store.test.ts`
  - `apps/desktop-flowpilot/src/types/contract.ts`
- modules:
  - `local-runner`
  - `desktop-flowpilot`
- routes:
  - `POST /client/workflow-runs/{runId}/sync-chat`
  - `POST /client/chat-sessions/restore`
  - `GET /client/projects/{projectId}/chat-sessions/remote`
- tables:
  - none

## 6. Acceptance Check

- All non-manual signatures from `TS-001` through `TS-039` are implemented or explicitly deferred with reason.
- Manual/e2e signatures from `TS-040` through `TS-046` are documented in completion notes with pass/fail/skip status.
- Normal automated tests require no live Google Drive, provider tokens, or second PC.
- Test failures identify whether the break is index merge, manifest creation, Drive upload, restore validation, collision handling, route mapping, or desktop state.

## 7. Out of Scope

- Supabase-backed sync tests.
- Workflow-run sync tests.
- Visual pixel-level QA.
- Real provider-token tests in default suites.
- Automatic sync behavior.

## 8. Completion Notes

- result:
  - Automated signatures `TS-001` through `TS-039` are implemented across runner/store tests.
  - Focused runner tests pass and the desktop store test file passes when bundled and executed under Node's test runner.
- follow-ups:
  - Manual signatures `TS-040` through `TS-046` still need real cross-PC/provider execution and result capture.
  - Desktop package does not expose a first-class `npm test` script for `src/state/store.test.ts`; verification currently uses a local `esbuild` bundle plus `node --test`.
- upstream docs updated:
  - Task-073 checklist state updated to reflect the automated coverage now in the branch.
