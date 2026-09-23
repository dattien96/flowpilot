# CA-934 — Run-view snapshot cache: bounded LRU + scroll anchor (CP-84 Task-433)

## Summary

`_runSnapshots` grew unbounded and cached timelines could alias into live
arrays. The existing seam was extended (no parallel `_runViews` cache):

- Bounded LRU with pin set — the focused run, `mainRunId`, and active-agent
  ancestry are never evicted.
- Clone-on-capture/restore for arrays/Sets (timeline, artifacts, pending
  lists, token usage, replay/paging metadata) — no aliasing between the live
  run and its snapshot.
- Stable scroll anchor (anchored item id + offset) instead of raw scrollTop —
  restored on reopen.
- Dirty marking on newer mux revisions: cache kept, revalidate-on-open always
  fetches fresh state; generation guards drop stale async restores across
  rapid A→B→C switching.
- Cache-miss → existing fetch/replay path; `selectProject` prunes the leaving
  project's snapshots; `deleteHistoryRun` drops related snapshots.
- Task-404 attention ingest behavior preserved.

## Verified

- 10 new tests green (`runViewCache.test.ts`).
- Clean-HEAD baseline worktree: identical 5 store.test + 3 replay-ordering
  failures → zero regressions.

## Files

- `state/store.ts`, `components/Timeline.tsx`,
  `state/runViewCache.test.ts` (new)

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CP-84
change_type: feature
summary: bounded LRU run-view snapshots with focus pinning, clone isolation, stable scroll anchors, dirty-revision revalidation, and generation guards
# --->8---
