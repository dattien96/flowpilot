# Task-067: Desktop Post-Restart Run Resume Via Provider Session ID

## Metadata

- Document ID: `Task-067`
- Title: `Desktop Post-Restart Run Resume Via Provider Session ID`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md), [CA-098: Provider Session Portability Spike](../../../change-audit/CA-098-spike-provider-session-portability.md), [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [Task-057: Cross-PC Provider Chat Sync](./Task-057-Cross-PC-Provider-Chat-Sync.md), [CA-097: Fix Desktop Run History Lost On Restart No Supabase](../../../change-audit/CA-097-fix-desktop-run-history-lost-on-restart-no-supabase.md)
- Replaces: `none`
- Tags: `desktop, history, local-runner, resume, restart, provider-session, codex, claude`

## AI Quick View

### Summary

- BUG-080 fixed history **visibility** after restart — the sidebar now shows past runs from `sessions.ndjson`. But clicking one of those runs still fails with `run_not_found` because `s.runs` is empty after restart.
- `sessions.ndjson` already persists `provider_session_id` (the Codex thread ID or Claude session ID) alongside each run record. This is the key needed to reconnect to the provider session.
- Two capabilities are needed: (1) **view transcript** — load the full conversation from the provider's local files; (2) **resume** — send new messages to a run from a previous session.
- Within the same runner session (in-memory), both already work. This task covers the post-restart gap only.
- This task is the **keystone** for cross-account (Task-068) and cross-PC (Task-069) sharing: it owns the run-reconstruction + re-point engine and the canonical "greyed-out when unopenable" fallback. MVP scope = chat runs only, designed to extend to workflow runs in Phase 2.

### Current Ask

- When `resumeRun` is called for a `runId` not in `s.runs` (post-restart), look it up in `SessionHistoryReader`, reconstruct an `interactiveRun` from the persisted `ProviderSessionState`, and re-register it in `s.runs` so the existing resume path works.
- Decide and implement what "reconnect" means per provider: Codex resumes via `--continue <thread_id>`; Claude resumes via `--resume <session_id>`.
- For transcript read-back, determine whether to stream the provider session file contents back to the desktop as a synthetic event sequence, or just surface a file path for direct read.

### Key Decisions

- `T-1` `resumeRun` must fall through to a "restore from disk" path when `s.runs[runID] == nil`. The restored `interactiveRun` is seeded from `ProviderSessionState` and registered back into `s.runs` before returning the `RunHandle`.
- `T-2` `providerSessionId` stored in `sessions.ndjson` is the portable handle passed to the provider CLI on resume (`codex --continue <id>` / `claude --resume <id>`). No new fields needed in the NDJSON schema for basic resume.
- `T-3` Transcript view and message resume are two separate capabilities. Implement resume first (allows new messages); transcript view is a follow-up.
- `T-4` If the provider session file is missing or the provider rejects the stored handle, the history item is shown **greyed-out / disabled** with an explanatory label (e.g. "session data not found on this machine"), never a crash. This is the canonical "greyed-out when unopenable" fallback (user decision 2026-06-17) reused by cross-account (Task-068) and cross-PC (Task-069).
- `T-6` MVP scope is chat runs only: the synthetic chat step (`chat-<runId>`) is deterministic, so reconstruction needs no stored step list. The reconstruction code MUST be designed so workflow runs can be added in Phase 2 by re-fetching step definitions from the catalog via the persisted `workflow_id` — do not hard-code chat-only assumptions into the restore path.
- `T-5` `run_id` is the durable identity; `provider_session_id` is mutable. On the SAME PC after restart the local session file persists, so resume reuses the stored `provider_session_id`. If the provider rejects the stored handle, the resume path re-mints a new provider session and re-points the `run_id → provider_session_id` mapping in `sessions.ndjson` — the user keeps seeing the same chat. (Re-pointing logic is shared with Task-068 cross-account and Task-069 cross-PC.)

### Constraints

- Do not change `sessions.ndjson` schema for this task — `providerSessionId` is already stored.
- Do not break the existing in-memory resume path (`s.runs[runID] != nil` case).
- Provider reconnect logic must be provider-specific (separate adapters for Codex and Claude).
- Task-068 (per-account isolation) is a recommended prerequisite — restoring a run from the wrong account context is confusing.

### Open Questions

- `Q-1` Does Codex resume a session from a previous process? **Largely answered** ([CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md), 2026-06-17): `codex exec resume <id>` resolves a rollout straight from `CODEX_HOME/sessions` by id, with no `session_index.jsonl` entry required — so reconstruction across a restart is viable. Confirm with a validly-logged-in account (the spike's turn failed only on expired auth).
- `Q-2` Does Claude accept `--resume <session_id>` from a previous process? Is the session file at `~/.claude/projects/<hash>/<session_id>.jsonl` still valid after the process exits?
- `Q-3` Should the restored run appear as `RunStatusIdle` (resumable) or `RunStatusCompleted` (view-only)? For a completed run, the user may want to send a follow-up; for an interrupted run, they want to retry.
- `Q-4` (resolved for MVP via T-6) Workflow-run reconstruction in Phase 2: re-fetch step definitions from the catalog by the persisted `workflow_id` on restore (`sessions.ndjson` stores `workflow_id` and `run_kind` but not the step list). Confirm the catalog is reachable at resume time on the non-Supabase path. Chat MVP does not need this — the synthetic step is deterministic.

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go` — `resumeRun` (current in-memory-only path, lines 532-550)
- `apps/local-runner/internal/runner/interactive_service.go` — `newInteractiveService`, `startTurn`
- `apps/local-runner/internal/runner/workflow_store.go` — `ProviderSessionState`, `SessionHistoryReader`
- `apps/local-runner/internal/runner/local_file_session_store.go` — `UpsertProviderSession`, `loadFromDisk`
- Task-057 Step 0 — provider CLI portability test (must be verified before coding T-2)

## 1. Goal

After an app restart, clicking a history item from a previous session should open that run and allow the user to continue chatting — not return `run_not_found`. The `providerSessionId` stored in `sessions.ndjson` is the bridge back to the provider's own conversation files.

## 2. Parent Links

- coding plan: [CP-18](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: BUG-080 §7 F-2 (providerSessionId persisted as foundation for future resume)

## 3. Trigger

BUG-080 (CA-097) fixed the sidebar population gap but explicitly noted that post-restart resume was out of scope. The natural next step is to close that gap using the `providerSessionId` already in `sessions.ndjson`. The open questions (Q-1, Q-2) around provider CLI portability must be answered first — if a provider session is not reattachable, the run is shown greyed-out / disabled with a reason (user decision 2026-06-17); history-injection is NOT used.

## 4. Exact Change

- `T-1` In `resumeRun`, when `s.runs[runID] == nil`, call `SessionHistoryReader.ListProviderSessionsByProject` (or a new `GetProviderSession(runID)`) to load the `ProviderSessionState` from `sessions.ndjson`. If found, construct an `interactiveRun` from the state and register it in `s.runs`.
- `T-2` Add `GetProviderSession(ctx, runID) (ProviderSessionState, error)` to `SessionHistoryReader` interface (or a new `SessionStore` interface) so individual lookup by `run_id` is efficient without scanning the full project list.
- `T-3` Each provider adapter (`codex_adapter.go`, `claude_adapter.go`) gets a `ResumeSession(sessionID, cwd string) error` method that validates the session file is still accessible and sets up the adapter to resume from it.
- `T-4` If the session file is missing or unopenable, return a typed error `ErrSessionFileMissing` so the desktop renders that history item greyed-out / disabled with a reason (canonical greyout fallback, Key Decisions T-4).
- `T-5` Re-point support: a resumed run must be able to update its `provider_session_id` / `provider_account_id` while keeping `run_id` (reuse `UpsertProviderSession`, last-wins). Shared by Task-068 (cross-account) and Task-069 (cross-PC).

### Definition Of Done

- [x] `DOD-067-001` `resumeRun` falls back to persisted session lookup when the in-memory run map is empty.
- [x] `DOD-067-002` `SessionHistoryReader` supports direct `GetProviderSession(runID)` lookup.
- [x] `DOD-067-003` Persisted chat runs reconstruct into an `interactiveRun` and re-register in `s.runs`.
- [x] `DOD-067-004` Restored workflow runs are rejected with `resume_unsupported` for MVP.
- [x] `DOD-067-005` Missing session files return a typed unavailable error instead of `run_not_found`.
- [x] `DOD-067-006` Missing active account auth returns `account_not_signed_in`.
- [x] `DOD-067-007` Existing in-memory resume behavior remains unchanged.
- [x] `DOD-067-008` Desktop history entries stay visible and receive an unavailable reason on typed resume failures.
- [x] `DOD-067-009` Focused restart/cross-account resume tests exist and pass in `cross_account_resume_test.go`.
- [ ] `DOD-067-010` Transcript view for historic runs is implemented.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_handlers.go` — extend `resumeRun`
  - `apps/local-runner/internal/runner/workflow_store.go` — extend `SessionHistoryReader` interface
  - `apps/local-runner/internal/runner/local_file_session_store.go` — implement `GetProviderSession`
  - `apps/local-runner/internal/runner/codex_adapter.go` — `ResumeSession`
  - `apps/local-runner/internal/runner/claude_adapter.go` — `ResumeSession`
- modules: `local-runner`
- routes: `POST /client/workflow-runs/{runId}/resume` (existing, behavior change only)

## 6. Acceptance Check

- Start a chat run. Restart the app. Click the run in the History sidebar. It opens and allows a follow-up message without error.
- `run_not_found` must not be returned for any run visible in the History sidebar.
- If the provider session file is deleted, a clear user-facing error is shown (not a blank screen or crash).
- Existing in-memory resume (same session, `s.runs` populated) is unchanged.

## 7. Out of Scope

- Transcript view (loading the prior conversation text back into the chat UI) — separate follow-up.
- Cross-PC resume — covered by Task-069 (non-Supabase, the selected path) / Task-057 (Supabase, deferred).
- Resume for Supabase-backed sessions (they use a different code path via `SupabaseWorkflowStore`).
- Workflow (multi-step) run resume — chat runs only for this task unless the provider adapter supports it identically.

## 8. Completion Notes

- result:
  - Post-restart chat resume is implemented through persisted session reconstruction plus provider-session validation.
  - Cross-account preparation and run re-pointing are exercised in `cross_account_resume_test.go`, including greyout-safe failure modes and Codex CLI resume execution.
- follow-ups:
  - Transcript view (stream prior conversation back to desktop as synthetic events)
  - Cross-PC resume (Task-057)
- upstream docs updated:
  - Added inline DoD checklist with current implementation state.
