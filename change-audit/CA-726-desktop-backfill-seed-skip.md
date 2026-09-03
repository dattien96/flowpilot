# CA-726 — Desktop backfill mirrors TUI skipLeg: seed replies never render as orphan bubbles

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-350
change_type: bugfix
summary: buildPriorChatTimeline gains the TUI renderChatTimelineBackfill skipLeg rule (seed turn marks its leg skipped until a real turn lands) so seed envelope replies never render as orphan assistant bubbles; I1 proven headless by cross-rendering both real code paths over live cht_1e5b706a8201 records (5 identical items); test mirror copy synced, stale expectation updated with operator approval, live-shape regression test added
# --->8---

## Problem

- I1 headless cross-render on live `cht_1e5b706a8201` (16 records, 1 switch):
  real TUI backfill → 5 items (seed reply seq 8 skipped); real Desktop
  `buildPriorChatTimeline` → 6 items with orphan bubble "Tôi là Grok 4.5…".
- Desktop backfill never got the BUG-347-class `skipLeg` rule.

## Changes

- `apps/desktop-flowpilot/src/state/store.ts` (`buildPriorChatTimeline`):
  empty/missing/envelope-prefix `turn_started` marks `skipLeg = legRunId`;
  any real `turn_started` clears it; `message_completed` on the skipped leg
  is dropped. Verbatim TUI semantics.
- `src/state/store.chatOpenTimeline.test.ts`: mirror copy synced; stale
  "skips handoff seed prompt" expectation updated to TUI parity
  (operator-approved 2026-09-04 — the message on a seed leg pre-real-turn
  IS the seed reply); new live-shape test (empty-prompt seed + reply skip,
  exact 5-item sequence incl. divider text).
- Throwaway Go cross-render test used for proof was deleted after use.

## Verification

- Post-fix headless cross-render: Desktop 5 items identical to TUI
  (prompt/assistant/divider/prompt/assistant, same texts).
- 23/23 green across `chatOpenTimeline`, `chatPostureSwitch`,
  `chatSwitch`, `chat-detached-parity`; `tsc` no new errors.
- Desktop live reopen of `cht_1e5b706a8201` (no orphan bubble): pending
  operator eyeball-confirm (30s).
