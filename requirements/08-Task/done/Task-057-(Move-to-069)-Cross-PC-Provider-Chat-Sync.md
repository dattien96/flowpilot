# Task-057: Cross-PC Provider Chat Sync

## Metadata

- Document ID: `Task-057`
- Title: `Cross-PC Provider Chat Sync`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [Task-023: Sync Artifact With Google Drive](../done/Task-023-Sync-Artifact-With-Google.md)
- Child Documents: `none`
- Related Documents: [Task-070: History Supabase Reader Production Fix](../done/Task-070-History-Supabase-Reader-Production-Fix.md), [BUG-060: Desktop Run History Empties After Switching Runs](../../09-BugFix/done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [Task-037: Desktop Project Run History Popover](../done/Task-037-Desktop-Project-Run-History-Popover.md)
- Replaces: `none`
- Tags: `local-runner, history, sync, google-drive, cross-pc, claude, codex, session`

## AI Quick View

### Summary

- **DEFERRED (user decision 2026-06-17):** the active cross-PC path is now Task-069 (Google Drive + local `sessions.ndjson`), which fits non-Supabase desktop installs. This Supabase-backed design is retained as the future option for Supabase users — do not implement before Task-069. The cross-cutting fallback is also updated: an unopenable synced run is shown greyed-out, not history-injected.
- Provider CLIs (Claude, Codex) store their conversation session files locally on disk (`~/.claude/`, `~/.codex/`). FlowPilot already knows the session_id / thread_id that maps to those files (stored in `workflow_provider_sessions`).
- Goal: one "Sync Chat" button uploads the relevant local provider session files to Google Drive (using existing GDrive infra from Task-023), tagged with the FlowPilot `run_id` as the portable identity. PC2 can download those files, restore them to the correct local directory, and resume the session.
- The key open question that must be validated before implementation: are provider CLI session files fully portable (self-contained JSONL — no server-side validation on resume)? This must be verified manually before coding begins.
- Task-070 (Phase 1) is a prerequisite — history must work on a single PC before cross-PC sync is meaningful.

### Current Ask

- **Step 0 (prerequisite — verify before coding):** Manual portability test: copy `~/.claude/projects/<hash>/<session_id>.jsonl` from PC1 to the same path on PC2 → run `claude --resume <session_id>` → does it resume? Repeat for Codex. Record result and decide whether same-ID resume or history-injection fallback is needed.
- **Step 1 (after portability confirmed):** Implement local file discovery: given a `provider_session_id` from Supabase, locate the matching file under `~/.claude/` or `~/.codex/`.
- **Step 2:** Upload located files to Google Drive under a `chat-sessions/<run_id>/` hierarchy, using existing GDrive artifact-sync infra.
- **Step 3:** On PC2, download and restore session files to the correct local directory, then update `workflow_provider_sessions` to point to the restored session.
- **Step 4:** Expose a "Sync Chat" button in the desktop History panel for per-run upload, and a matching "Restore on this PC" action on PC2.

### Key Decisions

- `T-1` FlowPilot's `run_id` (not the provider session_id) is the portable cross-PC identity for synced chat files. The provider session_id is machine-local and may need to change on PC2 if the provider does not accept a copied ID.
- `T-2` If provider session files are portable (same-ID resume works on PC2), restore the file and reuse the existing `provider_session_id`. If not portable, inject the conversation history as a context window for a fresh session — this fallback path degrades gracefully.
- `T-3` Reuse the Google Drive connection and folder established by Task-023 artifact sync. Do not require a separate GDrive auth for chat sync.
- `T-4` Store chat sync metadata in a new table `workflow_chat_sync_replicas` (or extend `workflow_provider_sessions` with a `gdrive_object_id` column) so PC2 can discover available synced sessions without scanning GDrive.
- `T-5` Session files are user-sensitive (contain full conversation history). Sync must be explicit (button-triggered), never automatic. Do not sync without user action.
- `T-6` Working directory path differences between PC1 and PC2 are expected. On restore, record the original cwd in metadata; on PC2, require the user to confirm or remap the cwd before resume is attempted.

### Constraints

- Task-070 must be completed first — single-PC history must be stable before adding cross-PC.
- Must not sync session files automatically — only on explicit user action per run.
- Must reuse Task-023 Google Drive connection (same OAuth, same folder structure).
- Session files may contain sensitive conversation content. Never log file contents. Treat as user data with same sensitivity as artifact content.
- Do not break or conflict with existing artifact sync behavior in `artifacts.go`.
- Portability test result (Step 0) gates the implementation design. If providers validate session IDs server-side and cross-machine resume fails, the fallback is to show the run greyed-out / disabled on PC2 with a reason (user decision 2026-06-17, supersedes the earlier history-injection idea); `T-2` / `T-5b` are updated accordingly.

### Open Questions

- `Q-1` Are Claude CLI session files (`~/.claude/projects/.../`) fully portable across machines? Does `claude --resume <session_id>` on PC2 (with the copied file) succeed without server-side validation? **Must be answered before coding Step 1.**
- `Q-2` Are Codex thread files (`~/.codex/`) similarly portable?
- `Q-3` What is the exact file path pattern for each provider? `~/.claude/projects/<cwd_hash>/<session_id>.jsonl`? Need to confirm naming convention.
- `Q-4` If the cwd hash in the file path is derived from the absolute working directory, and PC2 has the project at a different path, does the restore need to recompute the hash, or is the session_id-based lookup sufficient?
- `Q-5` What table change is least invasive for storing GDrive sync metadata per provider session: new column on `workflow_provider_sessions`, or new `workflow_chat_sync_replicas` table? Prefer new column if schema change is small; prefer new table if replica state (sync_status, object_id, checksum) mirrors Task-023 pattern.
- `Q-6` Does the "Sync Chat" action apply to all providers at once ("sync all local chats") or per-run? Start per-run for precision; a "sync all" sweep can be added later.

### Source Refs

- `apps/local-runner/internal/runner/artifacts.go` — Google Drive upload/download infra to reuse
- `apps/local-runner/internal/runner/artifact_google_drive_connection.go` — OAuth / folder state
- `apps/local-runner/internal/runner/supabase_provider_session_store.go` — `UpsertSession`, session record
- `apps/local-runner/internal/runner/workflow_store.go` — `ProviderSessionState`, `SessionHistoryReader`
- `supabase/migrations/20260615120000_add_workflow_provider_tables.sql` — `workflow_provider_sessions`
- Task-023 §9 (Multi-PC Semantics) — same cross-PC model applies to chat session files
- `08-Desktop-Chat-New-Plan.md §1.1` — resume semantics; `§1.3` — this feature listed as follow-up

## 1. Goal

Allow a FlowPilot user to upload the local provider CLI session files for a given run to Google Drive, then restore and resume that session on another PC — making chat history portable across machines in the same way artifact files already are.

## 2. Parent Links

- coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- artifact sync parent: [Task-023: Sync Artifact With Google Drive](../done/Task-023-Sync-Artifact-With-Google.md)
- specific upstream ids: Task-023 §9, BUG-060 §1 (resume semantics), `08-Desktop-Chat-New-Plan.md §1.1, §1.3`

## 3. Trigger

After Phase 1 (Task-070) stabilises single-PC history, the next user need is: "I started a chat run on my work PC and want to continue it on my laptop." Provider CLIs store their sessions locally. FlowPilot already knows the session ID from Supabase. The existing Google Drive artifact-sync infrastructure provides the upload/download plumbing. This task wires them together.

## 4. Exact Change

### Step 0 — Portability verification (before any code)

- `T-0` Manual test: on PC1, run a Claude chat session. Locate the session file at `~/.claude/projects/<hash>/<session_id>.jsonl` (or equivalent). Copy it to PC2 at the same relative path. On PC2, run `claude --resume <session_id>` from the same project cwd. Record whether resume succeeds. Repeat for Codex.
  - If **portable**: proceed with same-ID restore path (T-5a).
  - If **not portable**: implement history-injection fallback (T-5b) as primary path.

### Step 1 — Local file discovery

- `T-1` In `apps/local-runner/internal/runner/`, add `chat_session_locator.go` with `LocateProviderSessionFile(providerKey, sessionID, cwd string) (string, error)`. Returns the absolute path of the local session file for the given provider. For Claude: `~/.claude/projects/<hash(cwd)>/<session_id>.jsonl` (confirm exact pattern from Q-3/Q-4). For Codex: `~/.codex/threads/<thread_id>.json` (confirm).

### Step 2 — Upload to Google Drive

- `T-2` Add `SyncChatSessionToGoogleDrive(ctx, runID, providerKey, sessionFilePath string) error` in a new `chat_session_sync.go`. Uploads the session file to the project's configured GDrive folder under `chat-sessions/<run_id>/<provider_key>/session.jsonl`. Reuses `artifact_google_drive_connection.go` OAuth and upload primitives.
- `T-3` After a successful upload, persist the GDrive object ID and sync metadata. Decision per Q-5: add `gdrive_object_id` and `gdrive_sync_status` columns to `workflow_provider_sessions`, or create `workflow_chat_sync_replicas`. Write migration accordingly.
- `T-4` Expose a runner HTTP endpoint `POST /client/runs/{runId}/sync-chat` that triggers T-1 + T-2 for all provider sessions of that run.

### Step 3 — Download and restore on PC2

- `T-5a` (if portable — Q-1 answer YES): `RestoreChatSessionFromGoogleDrive(ctx, runID, providerKey, cwd string) error` — downloads the file from GDrive to the correct local path. Calls `UpsertSession` to record the restored session (same session_id) in `workflow_provider_sessions` for PC2.
- `T-5b` (if not portable — Q-1 answer NO): on restore, read the JSONL conversation turns, start a fresh provider session, inject prior conversation as a context preamble. Record the new session_id in `workflow_provider_sessions`.
- `T-6` Expose a runner HTTP endpoint `POST /client/runs/{runId}/restore-chat` that downloads from GDrive and restores locally.

### Step 4 — Desktop UI

- `T-7` In `RunStatus.tsx` or the History popover: add a "Sync to Drive" icon/button per history run item (only shown when GDrive artifact sync is connected for the project). Calls `sync-chat` endpoint.
- `T-8` On PC2, when opening a history run whose sessions are synced to GDrive but not locally present, show a "Restore from Drive" prompt before attempting resume. Calls `restore-chat` endpoint.
- `T-9` Show sync status (not synced / syncing / synced / failed) alongside each history run that has been synced.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/chat_session_locator.go` (new)
  - `apps/local-runner/internal/runner/chat_session_sync.go` (new)
  - `apps/local-runner/internal/runner/interactive_handlers.go` (new routes T-4, T-6)
  - `apps/local-runner/internal/runner/artifact_google_drive_connection.go` (reuse, no change expected)
  - `apps/desktop-flowpilot/src/components/RunStatus.tsx` (T-7, T-8, T-9)
  - `apps/desktop-flowpilot/src/state/store.ts` (sync/restore actions)
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts` (new endpoint calls)
  - new Supabase migration (T-3 metadata storage)
- modules: `local-runner`, `desktop-flowpilot`
- routes:
  - `POST /client/runs/{runId}/sync-chat`
  - `POST /client/runs/{runId}/restore-chat`
- tables: `workflow_provider_sessions` (extended) or new `workflow_chat_sync_replicas`

## 6. Acceptance Check

- T-0: portability verdict recorded; implementation path (T-5a or T-5b) confirmed.
- PC1: "Sync to Drive" button appears for a completed run in the History panel (when GDrive is connected).
- PC1: pressing "Sync to Drive" uploads the provider session file to GDrive and marks the run as synced.
- PC2 (fresh machine, same Supabase project, same GDrive connection): History panel shows the synced run. "Restore from Drive" prompt appears. After restore, clicking the run resumes the chat from the correct point in history.
- Sync must not happen automatically — only on explicit button press.
- Session file contents must not appear in runner logs.
- If GDrive is not connected, the sync button is not shown (no silent failure).

## 7. Out of Scope

- Automatic/background chat sync — always explicit, always per-run.
- Syncing to Supabase Storage (GDrive only for this task; Supabase path is a potential follow-up).
- Full mesh replication between every provider account — single-project, single-folder scope.
- Live-updating History panel while a run is in progress.
- Provider-aware `/skills` and `/agents` slash commands.
- Admin-web UI changes (desktop-only for this task).
- Codex thread resume if Codex portability test (T-0) shows server-side validation blocks cross-PC resume — document the limitation and leave Codex for a follow-up.

## 8. Completion Notes

- result:
- follow-ups:
  - Supabase Storage as alternative sync target (mirrors Task-023 provider duality)
  - Codex portability follow-up if T-0 shows it is not portable
  - "Sync all chat sessions for project" sweep action
  - Live History updates (separate from this task)
- upstream docs updated:
  - `08-Desktop-Chat-New-Plan.md §1.3` — mark "Cross-PC sync" as in progress / done
  - `SS-11` / `SD-12` — if new `workflow_chat_sync_replicas` table is added, flag the schema extension
