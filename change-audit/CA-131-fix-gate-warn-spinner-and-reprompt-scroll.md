# CA-131 — Fix Gate-Warn Navigator Spinner and Reprompt Scroll

Two chat-UI regressions triggered by flow-gate events.

## Files Changed

- `apps/desktop-flowpilot/src/state/store.ts`
  - `applyEvent`: extended `_gateBlockedRunIds` condition from `status === "block"` to `status === "block" || status === "warn"`. A `warn`-status gate violation settles the store to "completed" but the backend runner keeps the run "running"; without this guard the Navigator fell through to `item.status = "running"` and showed an infinite spinner for the inactive run. The `gateBlock` modal is still only shown for `status === "block"`.

- `apps/desktop-flowpilot/src/components/Timeline.tsx`
  - `Timeline` scroll `useEffect`: wrapped `scrollIntoView` in `requestAnimationFrame` so the scroll fires after any layout shift caused by the "Load earlier prompts" button being inserted at the top of the scroll container (triggered when a gate reprompt `turn_started` pushes `totalPromptCount` over `TIMELINE_PAGE_SIZE`). The `cancelAnimationFrame` cleanup prevents stale calls on rapid re-renders.

## Why

BUG-145: `_gateBlockedRunIds` (added in BUG-137 for hard blocks) was not extended to cover `warn` violations. Same navigator spinner symptom, different gate status.

BUG-146: The scroll effect fired against the pre-shift DOM; deferred via `rAF` so the full layout (including new pagination button at top) is committed before the scroll target is measured.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: BUG-145
change_type: bugfix
summary: extend _gateBlockedRunIds to warn status; defer reprompt scroll via rAF
# --->8---
