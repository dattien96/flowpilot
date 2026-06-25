# BUG-146 — Chat Scrolls to Top After Gate Reprompt Fires

## Metadata

- Document ID: `BUG-146`
- Title: `Chat auto-scrolls to top after gate-triggered reprompt turn starts`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [BUG-145](./BUG-145-Gate-Warn-Inactive-Chat-Shows-Running-Spinner.md), [BUG-140: Gate-Reprompt-Lacks-Actionable-Remediation](../done/BUG-140-Gate-Reprompt-Lacks-Actionable-Remediation.md), [CA-131](../../../change-audit/CA-131-fix-gate-warn-spinner-and-reprompt-scroll.md)
- Replaces: none
- Tags: `context-regression-engine, ui, chat-ui, scroll, flow-gate, reprompt, regression`

## AI Quick View

### Summary

- When the flow gate fires a `reprompt` violation, the runner sends a `turn_started` event with the reprompt message text as `e.prompt`. `applyTimelineEvent` pushes this as a new `prompt` item into the timeline.
- The Timeline component's `useEffect([timeline])` fires `endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" })` on every `timeline` reference change. Simultaneously, `useEffect([totalPromptCount])` updates `visiblePromptCount` via `setVisiblePromptCount(...)`, which triggers a follow-up render that can shift which items are in `visibleTimeline` (pagination window change).
- When the new prompt causes `totalPromptCount` to cross the `TIMELINE_PAGE_SIZE (6)` threshold for the first time, the "↑ Load earlier prompts" button is inserted at the TOP of the scroll container on the next render. This shifts existing content DOWN. Because the scroll animation from the effect was already in-flight (or was scheduled against the pre-shift DOM), the chat viewport ends up at an earlier position — visually appearing to scroll to the top.

### Current Ask

- Ensure the chat viewport stays at the bottom (latest message) after a gate-triggered reprompt `turn_started` — the same behavior as a user-initiated turn.

### Key Decisions

- `V-1` After the gate reprompt `turn_started` fires, the chat viewport must be scrolled to the latest message without any visible jump to the top.

### Constraints

- The fix must not break the existing per-event scroll-to-bottom behavior during normal AI turns.
- Must not introduce layout jank for chats with fewer than `TIMELINE_PAGE_SIZE` prompts (the common case).

### Open Questions

- `Q-1` Should the scroll effect be moved to `[visibleTimeline]` instead of `[timeline]` to avoid reacting to a `timeline` change before the pagination window has updated?
- `Q-2` Would deferring the scroll call (e.g., `requestAnimationFrame`) after the DOM has committed the pagination change be sufficient?
- `Q-3` Is CSS `overflow-anchor: auto` already in effect on `.timeline`? If not, enabling it may prevent the layout shift without JS changes.

### Source Refs

- `apps/desktop-flowpilot/src/components/Timeline.tsx` — `useEffect([timeline])` scroll (line ~531), `useEffect([totalPromptCount])` pagination (line ~514), `sliceTimelineFromPrompt` (line ~203), `TIMELINE_PAGE_SIZE = 6` (line ~197), `endRef` (line ~592).
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `turn_started` branch pushes `e.prompt` to timeline (line ~148–161).

## 1. Issue Summary

After the flow gate fires a reprompt and the runner auto-sends a new AI turn (`turn_started`), the chat timeline scrolls UP — showing the gate reprompt message or earlier content — instead of staying at the bottom where the AI's new response will appear. The user must manually scroll down to see the latest message. This only occurs when the gate reprompt prompt item causes the pagination window to shift (i.e., when `totalPromptCount` crosses `TIMELINE_PAGE_SIZE`).

## 2. Parent Links

- impacted coding plan: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- impacted tech design: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- impacted system spec: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)

## 3. Environment and Reproduction

- environment: Desktop app (any provider); any project where gate fires a `reprompt` violation.
- reproduction steps:
  1. Start a chat and run enough turns so that `totalPromptCount` reaches `TIMELINE_PAGE_SIZE (6)` (6 user+gate prompts).
  2. On the next AI turn, trigger a gate reprompt (e.g., change code without adding a change-audit note).
  3. The gate fires `turn_started` with the reprompt prompt text (adding prompt #7).
  4. Observe: the chat timeline jumps/scrolls UP, showing earlier prompts or the "↑ Load earlier prompts" button.
  5. The user must manually scroll DOWN to see the latest AI response at the bottom.
- frequency: Reproducible whenever the reprompt prompt causes `totalPromptCount` to cross the `TIMELINE_PAGE_SIZE` boundary; intermittent (less visible) at lower prompt counts.

## 4. Expected vs Actual

- expected: After a gate reprompt `turn_started`, the chat viewport remains at the bottom and the new AI response arrives in-place without any upward jump.
- actual: The chat scrolls UP, placing earlier prompts or the "Load earlier prompts" button in view. The AI's latest response is off-screen until the user manually scrolls down.

## 5. Impact

- users affected: Any user whose chat receives a gate reprompt and has accumulated `TIMELINE_PAGE_SIZE` or more prompts.
- workflows affected: Gate reprompt flow — the user loses sight of the AI's corrective response and may not notice it completed.
- severity: Medium — disorienting UX; does not block functionality.

## 6. Root Cause

- hypothesis: The scroll effect fires on `[timeline]` change. The pagination window update (`visiblePromptCount`) is a deferred state change (`setVisiblePromptCount`). When the gate reprompt prompt is added, the "Load earlier prompts" button is inserted at the TOP of the DOM (on the render following the `visiblePromptCount` update). This insertion shifts the visible content DOWN without the scroll container adjusting its pixel offset, making the viewport appear to have scrolled up. The pending `scrollIntoView` call may also target the DOM at an inconsistent state.
- confirmed cause: Not yet confirmed in running app; diagnosis based on code inspection of `Timeline.tsx` pagination and scroll logic.
- evidence: `sliceTimelineFromPrompt` with `visiblePromptCount < totalPromptCount` returns the window `[skip N, ..., end]`. Adding a new prompt item shifts this window. The "Load earlier" button is a DOM node inserted before all timeline items. CSS scroll anchoring may not be in effect on `.timeline` container.

## 7. Fix Strategy

- `F-1` Change the scroll `useEffect` dependency from `[timeline]` to `[visibleTimeline]` (the sliced array after pagination). This ensures the scroll fires only after the pagination window — and the "Load earlier" button — have already been applied to the DOM, so `endRef` reflects the correct final position.
- `F-2` (Alternative) Wrap the `scrollIntoView` call in `requestAnimationFrame(() => ...)` so it fires after the browser has committed all pending layout shifts, including new nodes inserted at the top.
- `F-3` (Complementary) Add `overflow-anchor: auto` to the `.timeline` CSS container if not already present. Browser scroll anchoring automatically adjusts the scroll offset when content is inserted above the anchor node, preventing the visual shift without JS changes.

## 8. Validation

- `V-1` Gate reprompt `turn_started` (at any prompt count) does not cause the viewport to show earlier content.
- `V-2` Normal AI turns still auto-scroll to the bottom.
- `V-3` The "Load earlier prompts" button still appears correctly when needed.
- `V-4` Manually scrolling up and then triggering a new turn (gate or user) does NOT force-snap the user back to the bottom mid-read (if F-1 or F-2 is chosen, evaluate whether this trade-off is acceptable).

## 9. Regression Guard

- tests: Visual/manual test: open a chat with 6+ prompts, trigger gate reprompt, confirm viewport stays at bottom.
- alerts: None.
- audit checks: Any change to the `[timeline]` scroll effect dependency must be reviewed for impact on other scroll scenarios (history replay, agent focus switch).

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: The pagination logic (`sliceTimelineFromPrompt`, `visiblePromptCount`) is correct and does not need to change. Only the scroll trigger timing needs adjustment.
