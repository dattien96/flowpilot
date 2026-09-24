# CP-85: Navigator local-only chat history + on-demand Sync screen

- Document ID: `CP-85`
- Title: `Navigator shows local chat history only; remote Drive sync moves behind an explicit "Open Sync" screen`
- Phase: `coding_plan`
- Status: `done`
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `SD-02`, `SS-01`
- Child Documents: `BUG-386`
- Related Documents: `CP-84` (attention inbox), chat-session sync work in `chat_session_sync.go`
- Tags: `desktop`, `navigator`, `chat-history`, `sync`, `ux`

## AI Quick View

### Summary

- The Navigator sidebar currently renders two history sections per project —
  local `History` and `Remote Chats` — and eagerly fetches the remote
  (Drive-synced) session list on every project select, refresh, sync, and
  restore. Users who never sync pay the fetch cost + a cluttered, fragile
  layout that overlaps when the rail is narrow.
- The Navigator drops the Remote Chats section, all remote fetch calls, and
  the per-row / batch sync-to-Drive controls. It shows local history only,
  plus one `Open Sync` button.
- `Open Sync` mounts a new overlay screen (`RemoteSyncPanel`) that owns every
  remote interaction: fetch remote sessions, restore (download) one/all, and
  upload unsynced local chats — nothing remote runs until the user asks.
- The 3s/10s `loadRunHistory` poll becomes silent so "Loading" no longer
  flashes in the History header on every tick.

### Current Ask

- Clean local-only chat history UI in the Navigator; zero remote-fetch logic
  runs by default; a single button opens a dedicated sync screen that runs the
  fetch + sync-down logic on demand.

### Key Decisions

- `P-1` Remote surface = overlay panel (same shell as `SessionsBoard`), scoped
  to the selected project, mounted from the Navigator's `Open Sync` button.
- `P-2` `loadRemoteChatSessions` stays in the store (panel calls it on mount);
  all *automatic* call sites in `Navigator` are removed. Post-sync / post-
  restore refreshes inside `syncHistoryRun` / `restoreRemoteChatSession` keep
  refreshing because they only run inside the sync flow.
- `P-3` Both sync directions live in the panel: "Download" (restore remote
  chats) and "Upload" (sync unsynced local chats via `syncRuns` /
  `syncAllInProject`) so no existing capability is lost.
- `P-4` Background history poll calls `loadRunHistory({ silent: true })` —
  updates data without toggling `historyLoading`; the flag remains for
  user-initiated loads only.
- `P-5` Remote per-item long-press selection mode is dropped; per-item +
  Restore-all cover the use cases with a simpler UI.

### Constraints

- No Go runner / API changes — `listRemoteChatSessions`, `syncChatRun`,
  `restoreChatRun` endpoints are untouched; only *when* the desktop calls them
  changes.
- Additive tests only; no existing test edited (safe-fix / oracle rules).
- `isSyncableRun` reconciliation against `remoteChatSessions` still works —
  the panel always has a freshly fetched list before computing it.

### Open Questions

- none blocking — panel intentionally scoped to the selected project (same
  scope the Remote Chats section had).

## 1. Goal

Make the Navigator's chat area local-only and calm: no remote list, no remote
fetch, no Drive sync affordances, no periodic "Loading" flash. Remote sync
becomes an explicit, user-invoked screen.

## 3. Implementation Strategy

- Extract remote UI + logic out of `Navigator.tsx` into
  `components/RemoteSyncPanel.tsx` (overlay dialog, `board-overlay`/`board-panel`
  shell, Escape/backdrop close).
- Navigator keeps: project groups, local History section (list, counts,
  newly-completed badge, refresh, long-press multi-select for **delete**),
  `Show all`, and a new full-width `Open Sync` button at the section bottom.
- Silent poll: `loadRunHistory(options?: { silent?: boolean })` — when silent,
  the call never sets `historyLoading: true`; it still applies results and
  clears the flag on settle (a superseding non-silent load can leave it true).
- Header layout fix: `.project-rail-head-actions` gets real flex styling and
  `min-width: 0`; the "Loading" dashed-box label is removed from the section
  head (initial loads show in the list body; manual refresh shows the button
  spinner).

## 4. Work Breakdown

- `P-1` `store.ts`: `loadRunHistory({ silent })` signature + flag handling.
- `P-2` `Navigator.tsx`: strip remote state/effects/section/sync controls;
  silent interval poll; `Open Sync` button mounts `RemoteSyncPanel`.
- `P-3` `RemoteSyncPanel.tsx`: fetch-on-open remote list, restore item/all
  (with x/y progress), unsynced-local upload section (item sync + sync-all +
  batch progress), error/empty states, sync confirm dialog parity.
- `P-4` `styles.css`: `.project-rail-head-actions`, `.project-history-open-sync`,
  `sync-panel` rows; drop dead `.project-history-section--remote` rule.
- `P-5` Tests: `RemoteSyncPanel.render.test.tsx` (fetch on open, restore,
  sync-all), `Navigator.render.test.tsx` (no remote section, no remote fetch,
  Open Sync button), `store.loadRunHistory-silent.test.ts`.

## 5. Touched Areas

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/RemoteSyncPanel.tsx` (new)
- `apps/desktop-flowpilot/src/state/store.ts` (`loadRunHistory` only)
- `apps/desktop-flowpilot/src/styles.css`
- tests under `apps/desktop-flowpilot/src/**`

## 7. Validation Plan

- tests to add: panel render/restore/sync flows; navigator "no remote fetch on
  mount"; silent poll does not set `historyLoading`.
- manual checks: Navigator shows only local History; remote count/fetch absent
  until `Open Sync`; panel restores a chat; narrow-width header no longer
  overlaps; no periodic "Loading" flash.
- failure cases: remote fetch error shows inline error in panel only;
  `cwd_remap_required` restore retry path unchanged.

## 8. Rollout and Fallback

- Single desktop release; fallback = revert CP-85 commits (remote section was
  pure UI removal; endpoints untouched).

## 9. Risks

- `R-1` Users expecting inline remote list must now click `Open Sync` — accepted
  per request (clean local-first UI).
- `R-2` `syncAllInProject`/`isSyncableRun` depend on `remoteChatSessions` being
  fresh — mitigated: panel fetches on open before upload actions compute.

## 10. Definition of Done

- [x] Navigator renders no Remote Chats UI and issues no
      `listRemoteChatSessions` call outside the sync panel —
      `Navigator.localOnly.test.tsx` asserts zero remote fetches on mount and
      no "Remote Chats" text; the only call sites left are `RemoteSyncPanel`
      (open/refresh) and the post-sync/post-restore refreshes that only run
      inside the sync flow.
- [x] `Open Sync` opens the panel; fetch + restore + upload work there —
      `RemoteSyncPanel.render.test.tsx` covers fetch-on-mount, row restore →
      close, unsynced-local list + Sync all, Escape close.
- [x] History header has no periodic "Loading" flash (poll is silent) —
      `store.loadRunHistory-silent.test.ts` (3 tests incl. the superseded-load
      settle case).
- [x] Section head layout does not overlap at narrow rail widths —
      `.project-rail-head-actions` flex + `min-width: 0` shrink rules; visual
      drag pass is operator-verified.
- [x] New tests green; existing tests untouched and green — 8 new tests pass;
      no existing test file edited. Pre-existing failures observed in the
      suite are unrelated (documented in `CA-964`).
- [x] CA ledger entry written under `project-nav` / `chat-history` feature key
      — `change-audit/CA-964-Navigator-Local-Only-History-And-Sync-Panel.md`.
