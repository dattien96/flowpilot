# Task-073: Cross-PC Non-Supabase Chat Sync Definition of Done Checklist

## Metadata

- Document ID: `Task-073`
- Title: `Cross-PC Non-Supabase Chat Sync Definition of Done Checklist`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- Child Documents: [Task-074: Cross-PC Non-Supabase Chat Sync Test Signatures](./Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md)
- Related Documents: [Task-071: Cross-Account Chat Resume Definition of Done Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md), [Task-072: Cross-Account Chat Resume Test Signatures](./Task-072-Cross-Account-Chat-Resume-Test-Signatures.md), [Task-023: Sync Artifact With Google Drive](../done/Task-023-Sync-Artifact-With-Google.md), [CA-098: Provider Session Portability Spike](../../../change-audit/CA-098-spike-provider-session-portability.md)
- Replaces: `none`
- Tags: `desktop, local-runner, sync, google-drive, cross-pc, sessions, ndjson, dod`

## AI Quick View

### Summary

- This checklist defines what must be true before Task-069 can be called complete.
- Scope is non-Supabase chat sync only: local `sessions.ndjson` index plus provider session file bytes through Google Drive.
- It depends on the same-machine resume and session-file relocation foundation from Task-071.
- Full Supabase sync, workflow-run sync, and automatic background sync are out of scope.

### Current Ask

- Use this checklist while implementing the cross-PC non-Supabase sync feature and during final review.

### Key Decisions

- `T-1` Sync is explicit and per-run.
- `T-2` Sync uploads both index metadata and provider session file bytes.
- `T-3` Restore merges into local `sessions.ndjson`; it never replaces the whole file.
- `T-4` Run id collisions are resolved deterministically during restore.
- `T-5` Unopenable restored runs remain visible and disabled with a typed reason.

### Constraints

- Do not log provider session file contents.
- Do not require Supabase.
- Do not auto-sync or auto-restore.
- Do not overwrite unrelated local provider files.
- Keep old `sessions.ndjson` records loadable.

### Open Questions

- `Q-1` Cross-PC Codex live validation requires a second machine or an equivalent isolated environment.
- `Q-2` Claude portability remains provider-untested until a real Claude session can be copied across machines.
- `Q-3` Whether the restore picker should appear in the existing history list or a small adjacent remote-history panel is a UI implementation detail; store and client behavior must support either.

### Source Refs

- [Task-069](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- [10-IG](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- [Task-023](../done/Task-023-Sync-Artifact-With-Google.md)
- [Task-071](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md)
- [CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md)

## 1. Goal

Provide a reviewable completion checklist for explicit Google Drive sync and restore of non-Supabase chat runs across PCs.

## 2. Parent Links

- coding plan: [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-069 `T-1` through `T-5`, Task-023 Google Drive artifact sync, Task-071 resume foundation

## 3. Trigger

Task-069 chose Google Drive plus local `sessions.ndjson` as the primary cross-PC path for non-Supabase desktop installs. The feature needs a concrete DoD checklist before implementation starts.

## 4. Exact Change

### Scope And Preconditions

- [x] `DOD-001` Feature applies only to non-Supabase local-file session storage.
- [x] `DOD-002` Feature applies only to chat runs for MVP.
- [x] `DOD-003` Sync action is explicit per run and never automatic.
- [x] `DOD-004` Restore action is explicit and never automatic on startup.
- [x] `DOD-005` Existing cross-account resume behavior from Task-071 remains unchanged.
- [x] `DOD-006` Missing Google Drive connection returns `google_drive_not_connected`.
- [x] `DOD-007` Provider session file contents are never logged.

### Local Data Model

- [x] `DOD-008` `ProviderSessionState` supports optional source machine/run metadata.
- [x] `DOD-009` `ndjsonSessionRecord` persists optional source machine/run metadata with `omitempty`.
- [x] `DOD-010` Old `sessions.ndjson` records without sync fields still load.
- [x] `DOD-011` Local sync status is represented without breaking history rendering.
- [x] `DOD-012` Machine identity is generated once per local project store.
- [x] `DOD-013` Machine identity uses random bytes and no PII.
- [x] `DOD-014` Machine identity survives runner restart.

### Remote Manifest And Drive Layout

- [x] `DOD-015` Sync writes `chat-sessions/runs/<source_machine_id>/<source_run_id>/manifest.json`.
- [x] `DOD-016` Sync writes provider files under the run's provider folder in Drive.
- [x] `DOD-017` Sync writes or updates `chat-sessions/_index/sessions.ndjson`.
- [x] `DOD-018` Remote index merge is last-wins by `(source_machine_id, source_run_id)`.
- [x] `DOD-019` Malformed remote index lines are ignored, not fatal.
- [x] `DOD-020` Manifest includes provider key, provider session id, run kind, original cwd, timestamps, and file hash.
- [x] `DOD-021` Manifest schema version is validated on restore.
- [x] `DOD-022` Remote paths are deterministic and use safe path segments.

### Sync To Drive

- [x] `DOD-023` Sync loads the selected run from `SessionHistoryReader.GetProviderSession`.
- [x] `DOD-024` Sync rejects missing runs with `run_not_found`.
- [x] `DOD-025` Sync rejects non-chat runs with `resume_unsupported`.
- [x] `DOD-026` Sync locates provider session files through the Task-071 locator.
- [x] `DOD-027` Sync maps missing provider files to `session_unavailable`.
- [x] `DOD-028` Sync computes SHA-256 and size for the provider file.
- [x] `DOD-029` Sync uploads provider file before manifest.
- [x] `DOD-030` Sync uploads manifest before updating the remote index.
- [x] `DOD-031` Sync updates local session sync status after successful Drive writes.
- [x] `DOD-032` Sync failure preserves prior successful sync metadata where present.
- [x] `DOD-033` Sync does not mutate unrelated local history records.

### Restore From Drive

- [x] `DOD-034` Restore can list remote chat sessions for a project from the Drive index.
- [x] `DOD-035` Restore downloads and validates the selected manifest.
- [x] `DOD-036` Restore maps missing remote index/manifest/file to `sync_remote_not_found`.
- [x] `DOD-037` Restore verifies provider file SHA-256 before writing.
- [x] `DOD-038` Restore maps hash mismatch to `sync_integrity_failed`.
- [x] `DOD-039` Restore requires an active provider account for the provider key.
- [x] `DOD-040` Restore maps missing active provider account home to `account_unavailable`.
- [x] `DOD-041` Restore maps missing active provider auth to `account_not_signed_in`.
- [x] `DOD-042` Restore requires a valid PC2 cwd.
- [x] `DOD-043` Restore maps missing cwd/remap requirement to `cwd_remap_required`.
- [x] `DOD-044` Restore writes provider file into the active account home.
- [x] `DOD-045` Restore refuses to overwrite a different local provider file.
- [x] `DOD-046` Restore accepts an existing identical provider file by hash.
- [x] `DOD-047` Restore upserts one local `ProviderSessionState`.
- [x] `DOD-048` Restore never replaces the whole local `sessions.ndjson`.

### Run Identity And Collision Handling

- [x] `DOD-049` Restore uses the source run id when it is unused locally.
- [x] `DOD-050` Restore reuses an existing restored local id for the same source identity.
- [x] `DOD-051` Restore creates `sync-<short_machine_id>-<source_run_id>` when the source run id collides with unrelated local history.
- [x] `DOD-052` Restored records preserve `SourceMachineID` and `SourceRunID`.
- [x] `DOD-053` Restored records keep provider session id mutable for later account re-point.
- [x] `DOD-054` History list can display restored runs without account grouping.

### HTTP And API Contracts

- [x] `DOD-055` Runner exposes `POST /client/workflow-runs/{runId}/sync-chat`.
- [x] `DOD-056` Runner exposes `POST /client/chat-sessions/restore`.
- [x] `DOD-057` Runner exposes `GET /client/projects/{projectId}/chat-sessions/remote`.
- [x] `DOD-058` All new routes return typed API errors using the existing error envelope.
- [x] `DOD-059` Desktop contract includes sync, restore, and remote summary DTOs.
- [x] `DOD-060` `RunnerClient` includes sync, restore, and list remote chat session methods.
- [x] `DOD-061` `HttpWsRunnerClient` implements the new methods.

### Desktop Behavior

- [x] `DOD-062` History item can trigger sync for a local chat run.
- [x] `DOD-063` Sync progress/failure/success is represented in store state.
- [x] `DOD-064` Restore can show remote sessions not present locally.
- [x] `DOD-065` Restore success adds or updates the restored run in local history.
- [x] `DOD-066` Typed sync/restore errors do not clear the current timeline.
- [x] `DOD-067` `cwd_remap_required` can retry with selected project path when available.
- [x] `DOD-068` Restored unopenable runs are disabled with the server reason.
- [x] `DOD-069` No account grouping, badges, or filters are added to history.

### Security And Privacy

- [x] `DOD-070` Provider session bytes are treated as sensitive user data.
- [x] `DOD-071` Logs include only ids, provider key, status, and safe remote path metadata.
- [x] `DOD-072` Drive appProperties do not exceed Google Drive limits.
- [x] `DOD-073` Drive appProperties do not include prompt/message content.
- [x] `DOD-074` Restore validates relative paths and rejects traversal.

### Tests And Verification

- [x] `DOD-075` Unit tests cover machine identity generation and persistence.
- [x] `DOD-076` Unit tests cover NDJSON sync metadata backward compatibility.
- [x] `DOD-077` Unit tests cover remote index merge last-wins behavior.
- [x] `DOD-078` Unit tests cover sync manifest construction and provider file hashing.
- [x] `DOD-079` Unit tests cover sync upload ordering.
- [x] `DOD-080` Unit tests cover restore hash mismatch and overwrite conflict.
- [x] `DOD-081` Unit tests cover restored run id collision handling.
- [x] `DOD-082` Handler tests cover typed sync/restore errors.
- [x] `DOD-083` Desktop store tests cover sync and restore actions.
- [x] `DOD-084` Focused Go tests pass.
- [x] `DOD-085` Desktop typecheck passes.
- [x] `DOD-086` No default automated test requires real Google Drive or provider tokens.

### Manual End-To-End

- [ ] `DOD-087` Manual Codex PC1 sync to Drive succeeds.
- [ ] `DOD-088` Manual Codex PC2 restore from Drive succeeds.
- [ ] `DOD-089` Manual restored Codex run opens and can send a follow-up.
- [ ] `DOD-090` Manual different-cwd restore path is verified.
- [ ] `DOD-091` Manual missing provider file or missing remote file greyout is verified.
- [ ] `DOD-092` Manual missing active provider auth greyout is verified.
- [ ] `DOD-093` Manual Claude behavior is recorded as pass/fail/provider-untested.

### Final Review Gate

- [x] `DOD-094` Implementation follows 10-IG without redesigning the sync model.
- [x] `DOD-095` All HIGH/CRITICAL GitNexus impact warnings are reviewed before edits proceed. (Waived — GitNexus MCP tools not exposed in this environment; no warnings could be generated. Consistent with Task-071 `DOD-94`.)
- [x] `DOD-096` `gitnexus_detect_changes()` is run before commit or final handoff when available. (Waived — not available in this environment and no CLI equivalent exists; scope verified via `go test` + `git diff`.)
- [x] `DOD-097` Any unimplemented item is moved to a named follow-up with reason.
- [x] `DOD-098` Task-069 and Task-074 completion notes are updated when implementation status changes.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/local_file_session_store.go`
  - `apps/local-runner/internal/runner/machine_identity.go`
  - `apps/local-runner/internal/runner/chat_session_sync.go`
  - `apps/local-runner/internal/runner/session_file_locator.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules:
  - `local-runner`
  - `desktop-flowpilot`
- routes:
  - `POST /client/workflow-runs/{runId}/sync-chat`
  - `POST /client/chat-sessions/restore`
  - `GET /client/projects/{projectId}/chat-sessions/remote`
- tables:
  - none; non-Supabase path uses local NDJSON and Google Drive

## 6. Acceptance Check

- Every checklist item from `DOD-001` through `DOD-098` is either checked or explicitly moved to a follow-up with reason.
- PC1 sync uploads both manifest/index metadata and provider session file bytes.
- PC2 restore merges one local session record without corrupting existing history.
- Restored run can resume when the provider accepts the copied file.
- Unsupported or missing data cases stay visible and disabled with typed reasons.

## 7. Out of Scope

- Supabase-backed sync.
- Workflow-run sync.
- Automatic sync or restore.
- Sync-all-runs.
- Transcript injection fallback.
- Admin-web UI.

## 8. Completion Notes

- result:
  - Backend sync/restore flow, Drive manifest/index handling, machine identity persistence, and desktop sync/restore wiring are implemented.
  - Focused runner tests pass, desktop store tests pass via bundled Node execution, and desktop typecheck passes.
- follow-ups:
  - `DOD-087` through `DOD-093`: manual cross-PC/provider validation remains pending.
  - `DOD-095` and `DOD-096`: waived — GitNexus MCP tools are not exposed in this environment and the GitNexus CLI has no `detect-changes` equivalent; change scope was verified via `go test` and `git diff` instead.
- upstream docs updated:
  - Updated Task-069 completion notes/status.
  - Updated Task-074 completion notes/status.
