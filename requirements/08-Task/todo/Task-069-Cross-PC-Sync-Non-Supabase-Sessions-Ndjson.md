# Task-069: Cross-PC Sync for Non-Supabase Users (sessions.ndjson + Provider Files)

## Metadata

- Document ID: `Task-069`
- Title: `Cross-PC Sync for Non-Supabase Users (sessions.ndjson + Provider Files)`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [Task-057: Cross-PC Provider Chat Sync](./Task-057-Cross-PC-Provider-Chat-Sync.md)
- Child Documents: `none`
- Related Documents: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md) (the foundation this extends to cross-PC), [CA-098: Provider Session Portability Spike](../../../change-audit/CA-098-spike-provider-session-portability.md), [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [Task-023: Sync Artifact With Google Drive](../done/Task-023-Sync-Artifact-With-Google.md), [CA-097: Fix Desktop Run History Lost On Restart No Supabase](../../../change-audit/CA-097-fix-desktop-run-history-lost-on-restart-no-supabase.md)
- Replaces: `none`
- Tags: `desktop, history, local-runner, sync, cross-pc, sessions, ndjson, codex, claude, google-drive`

## AI Quick View

### Summary

- Task-057 designed cross-PC chat sync for Supabase users: session metadata lives in `workflow_provider_sessions` (Supabase), and provider session files (Codex/Claude) are uploaded to Google Drive.
- Non-Supabase users (the majority of desktop installs) have no Supabase. Their session index lives in `.flowpilot/chats/sessions.ndjson`. To sync between PCs they must sync **both** the NDJSON file **and** the provider session files.
- The real chat content never lives in FlowPilot — it lives in the provider's own local storage (`~/.claude/`, `~/.codex/`). FlowPilot's `sessions.ndjson` is just the index (run metadata + providerSessionId pointer). Without the provider files, the index is useless for resume on PC2.
- This task defines the two-layer sync requirement and the open questions that must be answered before implementation.
- **Selected as the primary cross-PC transport (user decision 2026-06-17):** Google Drive + local `sessions.ndjson`, not Supabase. Task-057 (Supabase) is deferred. MVP scope is chat runs only; workflow runs are Phase 2.

### Current Ask

- Extend Task-057's scope to cover the non-Supabase path: use `.flowpilot/chats/sessions.ndjson` as the session index instead of Supabase; sync it to Google Drive alongside the provider session files.
- Validate Task-057 Q-1 / Q-2 portability questions (must happen before any code).
- Define the two-layer sync contract: layer 1 = `sessions.ndjson` (index), layer 2 = provider session files (content).

### Key Decisions

- `T-1` Two layers must always be synced together — syncing only the NDJSON index (without provider files) gives PC2 a history list it cannot resume; syncing only provider files (without NDJSON) gives PC2 session files it has no index for. Both layers are required.
- `T-2` For non-Supabase users, `sessions.ndjson` replaces `workflow_provider_sessions` as the session index to sync. Use the same Google Drive upload infra (Task-023 / Task-057 T-2) — upload `sessions.ndjson` to `chat-sessions/_index/sessions.ndjson` under the project's GDrive folder.
- `T-3` On PC2 restore: download `sessions.ndjson` to `.flowpilot/chats/sessions.ndjson`, then download each provider session file to its correct local path. After restore, the runner's `loadFromDisk` at next startup will pick up the index automatically.
- `T-4` The portability questions from Task-057 (Q-1: Claude files portable? Q-2: Codex files portable?) apply identically here. This task must not proceed past planning until Task-057 T-0 portability test results are known.
- `T-5` Working directory paths differ between PCs (Task-057 T-6 concern). The `working_directory` field in `sessions.ndjson` records the original cwd. On PC2, before resume, the user must confirm or remap the cwd.
- `T-6` The provider's `thread_id` / `session_id` from PC1 is typically invalid on PC2. FlowPilot keeps the stable `run_id` and re-points it to a `provider_session_id` valid on PC2 via file-restore-with-id-reuse when the provider accepts the relocated file (Q-1 = YES). If the provider rejects the relocated file (Q-1 = NO), the synced run is shown greyed-out / disabled on PC2 with a description ("can't open on a different PC") — NOT history-injected (user decision 2026-06-17). `provider_session_id` is mutable per `run_id`; when reuse works the user's view of the chat is continuous. Shared re-point + greyout mechanism with Task-068 and Task-067.

### Constraints

- Task-057 T-0 portability test is a hard prerequisite — do not implement sync before portability is confirmed.
- Non-Supabase sync must use the same Google Drive connection as artifact sync (Task-023) — no separate GDrive auth.
- `sessions.ndjson` sync must be explicit (button-triggered), never automatic — same policy as Task-057 T-5.
- Do not change the NDJSON schema for this task; `providerSessionId` and `workingDirectory` are already stored and are sufficient for the sync index.
- Task-068 (per-account isolation) is a recommended prerequisite — account-attributed runs in the NDJSON make the PC2 restore unambiguous about which provider account files to restore.
- MVP scope is chat runs only; workflow runs are Phase 2. The sync index (`sessions.ndjson`) and restore path must not hard-code chat-only assumptions — workflow runs re-fetch step definitions from the catalog by `workflow_id` (see Task-067 T-6).

### Open Questions

- `Q-1` (inherited from Task-057 Q-1/Q-2) Are Claude and Codex session files fully portable across machines? **Partial answer** ([CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md), 2026-06-17): Codex rollouts are account-agnostic and resume by id across *account homes* on one machine — cross-PC (2nd machine) is the same file relocation and is expected to work but is still unverified. Claude untested on both axes. If a provider rejects the relocated file, the synced run is shown greyed-out / disabled on PC2 (user decision 2026-06-17); history-injection is NOT used.
- `Q-2` When `sessions.ndjson` is synced to GDrive and restored on PC2, should it merge with any existing local `sessions.ndjson` on PC2, or replace it? Merge (last-wins by `run_id`) is safer but requires a merge step at restore time.
- `Q-3` What happens if PC2 already has a local run with the same `run_id` as a restored one (e.g. if the user ran the same project on both PCs)? `run_id` is a sequential counter per process (`run-1`, `run-2`…) so collisions are possible. Need a namespace strategy (e.g. `<machine_id>/<run_id>`) or accept last-write-wins.
- `Q-4` Should `sessions.ndjson` sync be per-project (scoped to a single workspace) or cross-project (the user picks which workspaces to sync)?
- `Q-5` Does the `.flowpilot/` folder live inside the project repo? If so, syncing it to GDrive may conflict with or duplicate the project's existing artifact GDrive folder. Need to confirm the GDrive folder hierarchy.

### Source Refs

- `apps/local-runner/internal/runner/local_file_session_store.go` — sessions.ndjson format and load/persist path
- `apps/local-runner/internal/runner/artifacts.go` — GDrive upload/download infra to reuse
- `apps/local-runner/internal/cli/root.go` — `storeDir = .flowpilot/chats/` path
- Task-057 §4 (Steps 0–4) — full sync implementation plan; non-Supabase path extends this plan, not replaces it
- Task-057 T-1 — `run_id` as portable identity; T-6 — cwd remapping concern

## 1. Goal

Give non-Supabase desktop users the same cross-PC chat portability that Task-057 gives Supabase users. A user clicks "Sync to Drive" on a history run; PC2 restores both the `sessions.ndjson` index and the provider session files and can open/resume that run.

## 2. Parent Links

- coding plan: [CP-18](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- sync parent: [Task-057](./Task-057-Cross-PC-Provider-Chat-Sync.md) — this task is the non-Supabase extension of Task-057
- artifact sync: [Task-023](../done/Task-023-Sync-Artifact-With-Google.md) — GDrive infra to reuse

## 3. Trigger

BUG-080 introduced `sessions.ndjson` as the session index for non-Supabase users. This makes single-PC history persistent. The natural next step — cross-PC sync — requires syncing this new file alongside the existing provider session files. Task-057 was designed before `sessions.ndjson` existed and assumes Supabase as the index; this task fills the non-Supabase gap.

## 4. Exact Change

Non-Supabase path extends Task-057 Steps 1–4. Only the index source differs:

- `T-1` **Index sync** (non-Supabase alternative to Task-057 T-3): upload `.flowpilot/chats/sessions.ndjson` to GDrive under `chat-sessions/_index/sessions.ndjson`. No Supabase table write needed.
- `T-2` **Index restore** (PC2): download `sessions.ndjson` from GDrive and merge-write to `.flowpilot/chats/sessions.ndjson` (last-wins by `run_id`, resolve Q-3 collision strategy first).
- `T-3` **Provider file sync / restore**: identical to Task-057 T-1, T-2, T-5a/b — locate provider session files by `providerSessionId`, upload, restore. Non-Supabase path uses `sessions.ndjson` as the lookup source instead of `workflow_provider_sessions`.
- `T-4` **Desktop UI**: same as Task-057 T-7, T-8, T-9 — "Sync to Drive" / "Restore from Drive" per history run. Non-Supabase users see the same button; the backend detects the storage mode and routes accordingly.
- `T-5` Resolve Q-3 `run_id` collision strategy before implementation. Recommended: prefix persisted `run_id` with a machine fingerprint (`<fingerprint>/<run_id>`) in the synced copy, preserving local IDs on disk.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/local_file_session_store.go` — index sync/restore helpers
  - `apps/local-runner/internal/runner/chat_session_sync.go` (new, from Task-057) — extended for non-Supabase path
  - `apps/local-runner/internal/runner/interactive_handlers.go` — new sync/restore routes
  - `apps/desktop-flowpilot/src/components/Navigator.tsx` or `RunStatus.tsx` — sync button
- modules: `local-runner`, `desktop-flowpilot`
- routes: `POST /client/runs/{runId}/sync-chat`, `POST /client/runs/{runId}/restore-chat` (same as Task-057)

## 6. Acceptance Check

- Task-057 T-0 portability test completed and result recorded.
- PC1 (no Supabase, GDrive connected): "Sync to Drive" on a history run uploads both `sessions.ndjson` and the provider session file to GDrive.
- PC2 (fresh machine, same GDrive folder, no Supabase): "Restore from Drive" restores `sessions.ndjson` and provider file; restarting the runner shows the synced run in History.
- `run_id` collisions between PC1 and PC2 local history do not corrupt either machine's `sessions.ndjson`.
- Sync is never automatic; only triggered by explicit user action.

## 7. Out of Scope

- Supabase-backed sync (fully covered by Task-057).
- Syncing the full `.flowpilot/` directory (artifacts, settings) — only `chats/sessions.ndjson` and provider session files are in scope.
- Automatic background sync.
- Live chat sync (while a turn is in progress).
- Conflict resolution UI (defer to a follow-up if Q-3 collision strategy proves complex).

## 8. Completion Notes

- result:
- follow-ups:
  - Merge the Supabase and non-Supabase sync paths into a unified backend once both are stable.
  - "Sync all runs for project" sweep (deferred per Task-057 §7).
- upstream docs updated:
  - Task-057 §8 follow-ups — add note about non-Supabase extension (this task)
