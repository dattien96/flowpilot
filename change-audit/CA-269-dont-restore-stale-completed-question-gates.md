# CA-269: Don't Restore Stale Completed Question Gates

## Scope

Fixed a desktop history-reopen regression where a completed run could resurrect an old question card from the cached run snapshot, even though the run had already settled. The symptom surfaced immediately after the runtime Google Drive picker question landed, but the bug is generic to any question gate cached before `turn_completed`.

## Changes

- `store.ts`: added snapshot sanitizing so cached/restored `pendingApprovals` and `pendingQuestions` are only preserved for `waiting_approval` / `waiting_question` snapshots. Completed/failed/cancelled/running snapshots no longer restore stale pending gates.
- `store.ts`: `backToMainRun()` now restores via `restoreRunSnapshot(...)` instead of spreading the raw cached snapshot, so the same sanitizing logic applies there too.
- `store.test.ts`: added a regression test proving `openHistoryRun("run-1")` does not re-show a stale `pendingQuestions` entry from a completed cached snapshot and stamps the replayed question card as answered.

## Verification

- `rtk npm run typecheck` in `apps/desktop-flowpilot`
- `rtk npx vitest run src/state/store.test.ts` could not complete in this package's current test environment because the renderer test path hits `ReferenceError: require is not defined in ES module scope` through `node_modules/.vite-electron-renderer/assert/strict.mjs`

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: Task-204
change_type: bugfix
summary: stop completed run snapshots from restoring stale pending question gates when reopening history
# --->8---
