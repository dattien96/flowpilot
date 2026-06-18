---
Document ID: Task-077
Title: Delete Chat History — Remove Local Record and Provider Session File
Phase: task
Status: done
Owner: DatNguyen
Reviewers: —
Created: 2026-06-18
Last Updated: 2026-06-18
Parent Documents: CP-17-Workflow-Chat-And_Session.md, requirements/10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md
Child Documents: —
Related Documents: BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md, CA-100-delete-chat-history.md, Task-066-Desktop-History-Just-Done-Tick-Indicator.md, Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md
Replaces: —
Tags: chat-history, delete, provider-session, local-runner, desktop
---

## AI Quick View

### Summary

- Adds a delete action for chat history entries that removes: (1) the FlowPilot NDJSON record from `sessions.ndjson`, (2) the provider session file on disk (Claude `.jsonl` or Codex rollout `.jsonl`), and (3) the in-memory run entry.
- A chat belongs to exactly one provider (Claude or Codex) — never both — but due to cross-account copy/relocation, its session file may exist in multiple account home directories for the same provider. Deletion scans all registered accounts of the matching provider.
- Backend: new `DELETE /client/workflow-runs/{runId}` route on the local runner. Desktop: delete button (×) appears on hover per history row with a two-step confirm (✓ / ✕).

### Current Ask

- Implemented and verified (Go build + TypeScript typecheck pass).

### Key Decisions

- `T-1` Scan ALL accounts of the matching provider when deleting provider session files — a session may have been copied into multiple account homes via cross-account resume (CA-098).
- `T-2` NDJSON rewrite is atomic: write to `.tmp` sibling then `os.Rename` so a crash mid-write does not corrupt the store.
- `T-3` UI uses optimistic removal (history row disappears immediately); if the API call fails, the store re-fetches history from the runner to restore the row.
- `T-4` Only the FlowPilot record and the local provider session file are deleted. Any Google Drive-synced copies are NOT removed (out of scope, see follow-up).
- `T-5` File deletion failures are best-effort (`_ = os.Remove(...)`) so a missing or already-deleted file never blocks the NDJSON/memory cleanup.

### Constraints

- Do not block cleanup on file-not-found errors — session files may already be missing (greyed-out items).
- A chat run is provider-exclusive (Claude XOR Codex), never both — the search is scoped by `ProviderKey` stored in the session record.
- NDJSON is append-only for writes; delete requires a full rewrite (filter-and-rename).

### Open Questions

- Drive-synced copy deletion — should delete propagate to `chat-sessions/runs/<machine>/<runId>/` in Google Drive? Currently out of scope; see `T-4`.

### Source Refs

- `CP-17-Workflow-Chat-And_Session.md`
- `09-Cross-Account-Chat-Resume-Implementation-Guide.md` §2 (D-5, D-6)
- `CA-097` (localFileSessionStore NDJSON persistence)
- `CA-098` (provider session portability spike)
- `CA-100` (this change audit)

---

## 1. Goal

Allow a user to delete a chat history entry from the sidebar. Deletion removes:

1. The `sessions.ndjson` record in FlowPilot's local data directory.
2. The in-memory `interactiveRun` entry in `InteractiveService`.
3. The actual provider session file on disk — the `.jsonl` file under the Claude projects directory or the Codex rollout file — found by scanning **all** registered accounts of the same provider (because cross-account copy may place the file in more than one home).

---

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md`
- tech design: `requirements/10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md`
- system spec: implied by chat session lifecycle spec (no dedicated SS for delete yet)
- specific upstream ids: D-5, D-6 from cross-account guide (storage contract)

---

## 3. Trigger

Users had no way to prune chat history. The history list grows unboundedly, and there is no mechanism to remove stale or failed chats from either the FlowPilot record or the underlying provider session files.

---

## 4. Exact Change

- `T-1` **`workflow_store.go`** — Added `DeleteProviderSession(ctx, runID) error` to `InteractiveStateStore` interface. Added implementation on `fakeWorkflowStore` (`delete(f.sessions, runID)`).
- `T-2` **`local_file_session_store.go`** — Implemented `DeleteProviderSession`: removes from in-memory map, rewrites `sessions.ndjson` atomically (temp-file + rename), skipping the deleted `run_id`.
- `T-3` **`interactive_resume.go`** — Added `deleteChatSession(runID) *apiErr`: resolves session from store or in-memory map; iterates all registered provider accounts matching `ProviderKey`; calls `LocateSessionFile` and `os.Remove` on each found path; removes from `s.runs`; calls `DeleteProviderSession` on the store.
- `T-4` **`interactive_handlers.go`** — Registered `DELETE /client/workflow-runs/{runId}` → `handleDeleteRun`. Returns `{"status":"deleted"}` on success.
- `T-5` **`contract.ts`** — Added `deleteRun(runId: string): Promise<void>` to `RunnerClient` interface.
- `T-6` **`HttpWsRunnerClient.ts`** — Implemented `deleteRun` using `DELETE /client/workflow-runs/{runId}`.
- `T-7` **`MockRunnerClient.ts`** — Added no-op stub `deleteRun`.
- `T-8` **`store.ts`** — Added `deleteHistoryRun(runId)` action: optimistic local removal → API call → re-fetch on error.
- `T-9` **`Navigator.tsx`** — Added `×` delete button (visible on row hover) with inline confirm (`✓` / `✕`) before calling `deleteHistoryRun`. Added `pendingDeleteId` and `deletingIds` state.
- `T-10` **`styles.css`** — Added CSS for `.project-history-delete-icon`, `.project-history-delete-confirm`, `.project-history-delete-cancel`.
- `T-11` **`store.test.ts`** — Added `deleteRun: async () => {}` to the inline `RunnerClient` mock to satisfy the updated interface.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/workflow_store.go`
  - `apps/local-runner/internal/runner/local_file_session_store.go`
  - `apps/local-runner/internal/runner/interactive_resume.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
  - `apps/desktop-flowpilot/src/state/store.test.ts`
- modules: local runner interactive service, localFileSessionStore, desktop Navigator sidebar
- routes: `DELETE /client/workflow-runs/{runId}` (new)
- tables: `sessions.ndjson` (NDJSON file rewritten on delete)

---

## 6. Acceptance Check

- [ ] Clicking `×` on a history row shows confirm (`✓ / ✕`) inline.
- [ ] Confirming removes the row from the sidebar immediately (optimistic).
- [ ] `DELETE /client/workflow-runs/{runId}` returns `200 {"status":"deleted"}`.
- [ ] The `sessions.ndjson` file no longer contains the deleted `run_id`.
- [ ] The provider session file (`.jsonl`) is removed from disk for all account homes that held a copy.
- [ ] If the provider session file is already missing, the delete still completes without error.
- [ ] If the API call fails, the sidebar row is restored (re-fetch from runner).
- [ ] A Codex chat is not searched in Claude directories and vice versa.
- [ ] Running a new chat after deleting an old one works normally.

---

## 7. Out of Scope

- Deleting the Google Drive-synced copy of the session (Drive manifest + provider file in `chat-sessions/runs/…`).
- Batch delete (delete all chats for a project).
- Undo / soft-delete / recycle bin.
- Supabase-backed session deletion (no `DeleteProviderSession` on `SupabaseWorkflowStore` yet — not needed for the non-Supabase path).

---

## 8. Completion Notes

- result: Implemented and verified — Go build clean, TypeScript typecheck clean.
- follow-ups:
  - Drive-copy deletion (propagate delete to Google Drive synced manifest + files).
  - Implement `DeleteProviderSession` on `SupabaseWorkflowStore` for parity when Supabase is configured.
  - Add integration test for the NDJSON atomic rewrite (temp-file + rename path).
- upstream docs updated: no upstream SS/SD/CP changes required — this is a pure additive delta to the session lifecycle. If Drive deletion is added later, `09-Cross-Account-Chat-Resume-Implementation-Guide.md` §1 (Scope) should be updated.
