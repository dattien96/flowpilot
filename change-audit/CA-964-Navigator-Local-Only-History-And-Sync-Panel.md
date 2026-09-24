# CA-964 — Navigator local-only history + on-demand Sync panel (CP-85) + attention-popover width fix (BUG-386)

- Feature keys: `project-nav`, `chat-history`, `desktop-ui-consistency`
- Documents: `requirements/07-Coding-Plan/done/CP-85-Navigator-Local-Only-History-And-Sync-Screen.md`, `requirements/09-BugFix/done/BUG-386-Attention-Inbox-Popover-Fixed-320px-Horizontal-Scroll.md`

## Summary

Operator request: the desktop chat-history area should be local-first and
calm — no remote section, no background remote fetches, no periodic "Loading"
flash — with Drive sync available only behind an explicit **Open Sync**
screen. Plus a separate bug: the needs-attention popover was hard-capped at
320px and scrolled horizontally.

Three deliverables:

1. **Navigator is local-only** (`Navigator.tsx`): the `Remote Chats` section,
   its store selectors, long-press remote selection mode, per-row sync icons,
   and every remote-list call are removed. Local History keeps groups,
   counts, newly-completed badges, refresh, multi-select delete, Show all.
   A quiet full-width **Open Sync** button (Globe icon + unsynced-local badge
   computed from local flags only) mounts the new `RemoteSyncPanel`.

2. **`RemoteSyncPanel.tsx` (new)**: overlay dialog on the `board-overlay`
   shell, opened only from Open Sync. Mounting it is the sole trigger for
   `loadRemoteChatSessions`. It owns both directions: download (remote list,
   per-chat restore+open, Restore all with x/y progress, unavailableReason
   rows disabled) and upload (unsynced local chats reconciled against the
   fresh remote list, per-chat sync with confirm dialog, Sync all driving the
   shared `syncBatchProgress`). Escape/backdrop close; project-scoped.

3. **Silent history poll**: `loadRunHistory(options?: { silent?: boolean })`.
   The 3s/10s interval now passes `{ silent: true }` so `historyLoading` is
   never toggled by background refresh; the flag still clears on settle so a
   superseded loud load cannot wedge it. Manual refresh keeps the spinner.

Plus **BUG-386**: `.attention-inbox-pop` widened from
`min(320px, 90vw)` to `min(560px, calc(100vw - 2 * var(--space-4)))` with
`overflow-x: hidden`; `.attention-inbox-li`/`.attention-inbox-item` got
`min-width: 0` so the grid track shrinks below min-content and title ellipsis
engages; `.attention-inbox-head` is now flex so "Approve all" right-aligns.

Responsive hardening along the way: `.project-history-section` /
`.project-rail-head-actions` / `.project-section-toggle` got `min-width: 0`
shrink rules so a narrow rail ellipsizes instead of overlapping.

## Remote-fetch call-site audit (P-2 gate)

`loadRemoteChatSessions` is now called from exactly three places:

- `RemoteSyncPanel` mount effect + its Refresh button — the intended trigger.
- `syncHistoryRun` post-sync refresh — only runs inside a user-initiated sync.
- `restoreRemoteChatSession` post-restore refresh — only runs inside a
  user-initiated restore.

`selectProject` resets `remoteChatSessions`/`remoteHistoryLoadError` to empty
but does not fetch. No bootstrap path, interval, or Navigator effect reaches
the remote list anymore.

## Tests

- New (additive-only, no existing test touched):
  - `state/store.loadRunHistory-silent.test.ts` — silent never raises
    `historyLoading`; non-silent still does; a silent load superseding a loud
    one clears the flag on settle.
  - `components/Navigator.localOnly.test.tsx` — mount renders local history,
    no "Remote Chats" text, zero `listRemoteChatSessions` calls; clicking
    Open Sync mounts the panel and only then fetches remote.
  - `components/RemoteSyncPanel.render.test.tsx` — fetch-on-mount + list,
    row restore → close, unsynced-local section + Sync all (batch), Escape.
- `tsc -p tsconfig.phase1-tests.json` — clean.
- New-test files: 8/8 pass. (jsdom render tests run under
  `node --test` + `scripts/phase1-runtime.js`.)

## Pre-existing suite failures observed (unrelated — reported, not touched)

Per additive-tests-only / oracle-rule, these were left as-is:

- `settingsHelpers.test.js` — `findDuplicateJiraIntegration` workspace-global
  scoping returns null (helper + test both unchanged at HEAD).
- `store.test.js` — 5 failures (`localStorage is not defined` in plain-node
  env; `historyOpenError` on non-provider failure; replay-stream abort
  timing; orchestration handoff text; stop interrupt list) — none exercise
  `loadRunHistory` or remote history.
- `store.history-replay-order.test.js` — recovered lifecycle ordering drift
  (`[12,7,8,…]` vs `[7,8,12,…]`).
- `timeline_terminal_parity.test.js` — `isTurnCompletedPlaceholder` crashes on
  `undefined.trim()`.
- `styles.tokens.test.js`, `importBoundary.test.js` — test-internal path
  bugs (`ENOENT` resolving `src/styles.css` / `packages/…/domain` from the
  `.phase1-tests` layout).
- `runnerRepositories.test.js` — health-payload mapping now carries new
  fields (`buildId`, `generation`, `lifecycleMode`, `phase`,
  `protocolVersion`, `runnerInstanceId`).

Note: `node --test <dirs>` in `test:phase1` stalls on this machine (a render
test that fires a real `fetch` to the live runner port leaves an undici
keep-alive socket open — the new tests avoid this by stubbing `loadProjects`
and using pure client stubs). The suite was verified file-by-file instead.

## Manual/operator pass still open

- Visual drag check: popover width, narrow-rail overlap, sync panel layout.
- `gitnexus detect_changes` — scope verified (Navigator internals removed,
  `loadRunHistory` signature extended, `RemoteSyncPanel` new); risk medium,
  driven by the Navigator file rewrite size, no upstream callers affected.
