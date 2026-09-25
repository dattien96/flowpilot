# CA-980: BUG-479 — mux upsert inserts unseen lanes into Desktop history

## Why

`patchHistoryLane` only patched rows already present in
`projectHistoryById` — a run created after the last 30s history poll
(e.g. by another client or project) never appeared on
Navigator/SessionsBoard until polling, violating CP-84's <1s
lane-visibility target.

## What changed

- `patchHistoryLane` (store.ts) now upserts: when the lane's runId is
  absent it appends a minimal `RunHistoryItem` synthesized by the new
  `muxLaneHistoryRow` helper instead of returning early.
- Minimal row carries only projection fields (runId, projectId, chatId,
  providerKey — `?? "codex"` matching existing board fallback, status,
  updatedAt-as-startedAt, lastSummary→lastMessage). Server-polled
  metadata is untouched for existing rows; the next poll replaces the
  slice wholesale by runId, so no duplicate registry is needed.
- Cross-project insertion updates only that project's slot — `runHistory`
  mirrors only when `selectedProjectId` matches; focus/draft untouched.

## Tests

- `src/state/muxUpsertBug479.test.ts` (4 tests): snapshot insert, upsert
  insert-then-patch (exactly one row), cross-project no-focus-steal,
  existing-row metadata preservation. All RED on clean tree, GREEN after.

## Verification

- Full desktop phase1 suite: 572 tests — 11 failures identical to clean-
  tree baseline (localStorage/env + replay-timing flakes); zero new.
