# BUG-144 — Chat Intent Type Locked After First Turn

## Metadata

- Document ID: `BUG-144`
- Title: `Chat intent type selector locked after first turn — cannot change type mid-chat`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [Task-114: Chat Mode Selector Normal/Task/Bug](../../08-Task/done/Task-114-Chat-Mode-Selector-Task-Bug-Normal.md), [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- Child Documents: none
- Related Documents: `BUG-143`, `CA-129`, `CA-130`
- Replaces: none
- Tags: `chat-ui, ux, chat-intent, mode-selector`

## AI Quick View

### Summary

- After the first turn, `ChatStartIntentPanel` replaced the 3-tab selector with a static "Declared Intent" locked chip — the user could no longer change the chat type (Normal / Task / Bug).
- The locked chip was also hidden entirely when mode was Normal, so there was no visual indicator at all after the first turn in Normal mode.
- The mode-aware stroke border on the chat area (`BUG-143`) derived its active mode from `lastTurnInput?.changeType` post-turn, making it out of sync if the user had a reason to switch mode.

### Current Ask

- Always show the 3-tab intent selector (Normal / Task / Bug); never lock it after first turn.
- Active tab shows a colored border: Task = green, Bug = red, Normal = default neutral.
- Tabs and the doc ID input are disabled while `status === "running"`; re-enabled when idle/done.
- Chat area stroke border follows `chatStartMode` directly (always in sync with the panel).

### Key Decisions

- `V-1` `chatStartMode` is the single source of truth for both the panel display and the `workspace-main` border class — no longer split between `chatStartMode` (pre-turn) and `lastTurnInput?.changeType` (post-turn).
- `V-2` Running guard: `isRunning = status === "running"` disables tabs and doc-ID input; does not hide them.
- `V-3` Colored tab borders use `is-<mode>` class + `.active` compound selector — no runtime style injection.

### Constraints

- Runner still only stamps `changeType` on the first turn (`isFirstChatTurn && chatStartMode !== "normal"`); allowing mid-chat type changes does not retroactively restamp previous turns. This is a known limitation, not addressed here.
- Must not break Normal mode: no border class applied when `chatStartMode === "normal"`.

### Open Questions

- none

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` — `ChatStartIntentPanel`, `ChatWorkspace`
- `apps/desktop-flowpilot/src/styles.css` — `.chat-start-mode-tab`
- `apps/desktop-flowpilot/src/state/store.ts:626` — `setChatStartMode`
- `Task-114` (introduced the selector and locked behavior)
- `BUG-143` (stroke border, same mode state)

## 1. Issue Summary

`ChatStartIntentPanel` had two render paths: pre-turn (3-tab selector) and post-turn (locked chip). Once the first turn was sent, the selector was replaced by a static chip with the text "This chat is locked to the selected flow-gate intent." For Normal mode, the panel returned `null` entirely post-turn. The user could not change the intent type mid-chat.

Additionally, the chat area stroke border (`BUG-143`) derived `activeChatSubMode` from `lastTurnInput?.changeType` after the first turn rather than `chatStartMode`, making the two panels inconsistent if `chatStartMode` changed.

## 2. Parent Links

- impacted coding plan: [CP-35](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- impacted tech design: none directly
- impacted system spec: none (UX behavior change only)

## 3. Environment and Reproduction

- environment: desktop app, authenticated, normal-chat mode
- reproduction steps:
  1. Open Chat workspace, right sidebar shows Chat Intent panel with Normal / Task / Bug tabs.
  2. Select **Task** tab.
  3. Send any message.
  4. Observe right sidebar — the 3-tab selector disappears; replaced by "Declared Intent" chip. No way to change to Bug or Normal.
  5. In Normal mode: after first turn the panel disappears entirely.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: 3-tab selector always visible; active tab has colored border; tabs disabled only while AI is running
- actual: tabs replaced by locked chip after first turn; chip hidden in Normal mode; no way to change type

## 5. Impact

- users affected: all desktop users working across multiple intent types in a single chat session
- workflows affected: chat intent selection UX; chat area stroke border indicator
- severity: medium — blocks valid UX flow of switching intent mid-session

## 6. Root Cause

- hypothesis: Task-114 implemented the locked view as a deliberate design (mode locked after first turn injection) but this does not match the desired UX.
- confirmed cause: `ChatStartIntentPanel` had an explicit `if (hasTurns)` branch that rendered a non-interactive locked view. The `activeChatSubMode` in `ChatWorkspace` also switched to `lastTurnInput?.changeType` post-turn, diverging from `chatStartMode`.
- evidence: `ChatWorkspace.tsx:178` — `if (hasTurns) { ... return locked chip }`. `ChatWorkspace.tsx:352` — `const activeChatSubMode = hasTurns ? (lastTurnInput?.changeType ?? "normal") : chatStartMode`.

## 7. Fix Strategy

- `F-1` Remove the `hasTurns` locked branch from `ChatStartIntentPanel` entirely.
- `F-2` Drop `timeline` and `lastTurnInput` selectors from `ChatStartIntentPanel`; add `runStatus` (`s.status`).
- `F-3` Add `disabled={isRunning}` to all 3 tab buttons and the doc-ID input (`isRunning = status === "running"`).
- `F-4` Add `is-${item.mode}` class to each tab button for CSS targeting.
- `F-5` Add CSS: `.chat-start-mode-tab.active.is-task` (green border/bg), `.active.is-bugfix` (red border/bg), `:disabled` (opacity + cursor).
- `F-6` In `ChatWorkspace`, replace `activeChatSubMode = hasTurns ? ... : chatStartMode` with `activeChatSubMode = chatStartMode`; remove unused `timeline` and `lastTurnInput` selectors.

## 8. Validation

- `V-1` Pre-turn: 3 tabs visible, Normal selected by default — no border change on chat area.
- `V-2` Select Task → green border on tab + green stroke on chat area.
- `V-3` Select Bug → red border on tab + red stroke on chat area.
- `V-4` Send first message → tabs remain visible and interactive after turn completes.
- `V-5` While AI is running → tabs are visually dimmed and unclickable.
- `V-6` After run completes → tabs re-enable, user can switch type.
- `V-7` Switch to Normal mid-chat → chat area stroke border disappears (no class added).
- `V-8` TypeScript build passes — no unused variable warnings for removed `timeline`/`lastTurnInput`.

## 9. Regression Guard

- tests: no dedicated unit test; existing `store.test.ts` covers `chatStartMode` propagation.
- alerts: none
- audit checks: `CA-130`

## 10. Follow-Up Document Updates

- upstream docs that must change: none — UX behavior reversal of Task-114's locked-mode design; the lock was implicit design, not a written AC.
- notes left unchanged on purpose: runner's `changeType` stamping logic (`isFirstChatTurn` guard) is unchanged — stamping only on first turn is a separate concern.
