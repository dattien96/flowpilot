# CA-1047 — Instant chat navigation: pending row, click-time open, stale-response guards

## Summary

Starting a new chat left the sidebar empty until the next `loadRunHistory`
poll (3s while running, 10s idle), and clicking a history row changed nothing
on screen until `POST /resume` *and* the `chatTimeline` tail-page fetch both
resolved — users read that as lag and clicked again, which made things worse.
Separately, a `startRun` mint that resolved after the user had already opened
another chat would refocus the just-minted run and clobber the open chat's
timeline (the focus writes were unguarded).

## Changes

- `apps/desktop-flowpilot/src/state/store.ts`
  - New state: `pendingChatStart` (pre-runId first-send window),
    `historyOpeningRunId` (row clicked but resume not yet landed),
    `_historyOpenSeq` (stale-response counter bumped by every open/reset/send),
    `_locallyStartedRuns` (rows minted locally, pending server echo).
  - `sendPrompt`: bumps `_streamRunSeq` at send time (not when startRun lands)
    and seeds `pendingChatStart`; post-`startRun` focus writes are gated on
    `sendSeq` so a late mint cannot steal focus back, while
    `noteLocallyStartedRun` still upserts the new run's history row
    (the minted turn proceeds server-side detached — `consumeStream`
    self-stales). The send also clears `historyOpeningRunId` since it
    supersedes any in-flight open.
  - `openHistoryRun`: same-row click is a no-op; sets `historyOpeningRunId`
    synchronously so the Navigator highlights/spins the row at click time;
    fires the `chatTimeline` tail fetch *before* the focus `set` and merges
    the page in afterwards (id-deduped prepend), so the switch paints on
    resume — cached snapshots render instantly, uncached opens no longer wait
    for the transcript read. Every await point re-checks `_historyOpenSeq`.
  - `loadRunHistory`/`loadProjectHistory` fold `_locallyStartedRuns` into the
    authoritative list and prune entries the server now echoes — a poll
    fetched mid-POST can no longer blank the fresh chat's row.
  - `deleteHistoryRun` prunes the local row alongside the optimistic removal
    so a deleted chat can't be resurrected by the merge.
  - `resetRun` clears `historyOpeningRunId`/`pendingChatStart` and bumps both
    seq counters.
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - Renders `pendingChatStart` as a non-interactive "Starting…" skeleton row
    (`PENDING_CHAT_ROW_ID`, excluded from sync count / selection-mode
    delete-all) at the top of the active project's history.
  - Rows being opened render active styling + spinner + `aria-current`
    immediately via `historyOpeningRunId`.

## Tests (RED→GREEN — `store.chatNavigationLatency.test.ts`)

- new send → `pendingChatStart` set; minted row upserted on handle-land.
- stale `listRunHistory` (empty) cannot drop the locally-started row.
- click → `historyOpeningRunId` set before resume resolves; focus on land.
- focus moves after resume even while `chatTimeline` is still pending.
- racing opens: the latest click wins; an older resume response is discarded.
- switch during slow `startRun`: minted turn still POSTs server-side but
  never refocuses (run stays on the opened chat, optimistic rows cleared).
- `resetRun` invalidates a pending open's late response.

## Verification

- `tsc -p tsconfig.phase1-tests.json` clean.
- Focused suite (new file + scaffold + timelineWindow + silent-poll +
  chatSwitch): 29/29 pass.
- Full phase1 suite: 690 tests / 677 pass / 13 fail — identical to the
  clean-baseline failure set (Node 26 `localStorage`, leftover-state and
  timer flakes verified against a stash in the previous session).

# ---8<--- flowpilot:change-ledger
feature_key: desktop-scaffold-chat
source_doc_id: CA-1047
change_type: bugfix
summary: instant chat navigation — pending skeleton row on first send, local minted-run upsert surviving stale polls, click-time open marker, focus on resume instead of transcript fetch, and seq guards so late resume/startRun responses can't steal focus
# --->8---
