# Task-064: Desktop ChatInput Send→Stop Button Transform

## Metadata

- Document ID: `Task-064`
- Title: `Desktop ChatInput Send→Stop Button Transform`
- Phase: `task`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `DatNguyen`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: `CP-17-Workflow-Chat-And_Session.md`, `SS-11-Workflow-With_Session.md`
- Child Documents: none
- Related Documents: `CA-090-desktop-chatinput-send-to-stop-button.md`, `Task-044`, `Task-046`, `Task-048`
- Replaces: none
- Tags: `desktop, chat-input, ux, stop, cancel`

## AI Quick View

### Summary

- When AI is in progress (`status` is `running`, `waiting_approval`, or `waiting_question`), the Send button in `ChatInput` transforms into a red Stop button
- The Stop button renders a filled square icon (SVG) and calls `store.stop()` on click
- `store.stop()` → `client.interrupt(runId)` → backend cancels the current turn — no new logic needed
- CSS class `.send-btn-stop` added for red styling using existing `--err` color variable

### Current Ask

- Done. Send button now transforms to red Stop button when AI is blocked.

### Key Decisions

- `T-1` Reuse the existing `stop()` store action — no new backend or store changes needed
- `T-2` Show the stop button for all three blocked states (`running`, `waiting_approval`, `waiting_question`) consistent with `RunStatus.tsx` behavior
- `T-3` Keep the existing "Stop" button in `RunStatus.tsx` header — removal is out of scope

### Constraints

- UI-only change; no backend, store, or contract modifications
- Must not break keyboard submit (Enter) or attachment flow

### Open Questions

- none

### Source Refs

- `CP-17-Workflow-Chat-And_Session.md` — unified action box / chat input design
- `SS-11-Workflow-With_Session.md` — session-aware interaction model

## 1. Goal

Transform the Send button in `ChatInput` into a red Stop/Cancel button while the AI is in progress, giving users a direct cancel affordance in the composer rather than only in the header status bar.

## 2. Parent Links

- coding plan: `CP-17-Workflow-Chat-And_Session.md`
- tech design: `SD-12-Refactor-Workflow-With_Session.md`
- system spec: `SS-11-Workflow-With_Session.md`
- specific upstream ids: none

## 3. Trigger

Scout investigation confirmed all cancel logic exists end-to-end (`store.stop()` → `client.interrupt(runId)` → Go backend context cancel → `turn_failed` event). The only gap was the Send button being disabled during AI work instead of transforming into a stop affordance, as seen in the Claude app.

## 4. Exact Change

- `T-1` Added `StopIcon` inline SVG component (filled square, 14×14) to `ChatInput.tsx`
- `T-2` Added `const stop = useStore((s) => s.stop)` selector to `ChatInput`
- `T-3` Replaced `<button … Send>` at line 665 with conditional: when `blocked===true` render `<button class="send-btn-stop">` calling `stop()`; otherwise render original Send button
- `T-4` Added `.send-btn-stop` CSS rule using `var(--err)` (#f85149) background/border with white icon and `#d73a3a` hover state

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules: desktop chat composer
- routes: n/a
- tables: n/a

## 6. Acceptance Check

- When AI is running: Send button becomes a red square Stop button
- Clicking Stop calls `store.stop()` → `client.interrupt(runId)` → turn cancelled
- When AI is idle/completed/failed: Send button returns to normal blue state
- TypeScript: `npx tsc --noEmit` passes with no errors

## 7. Out of Scope

- Removing the existing "Stop" button from `RunStatus.tsx` header bar
- Visual animation/transition between Send and Stop states
- Changes to backend cancel logic (already works for both Claude and Codex)
- Stop button in workflow (non-chat) mode

## 8. Completion Notes

- result: Implemented and verified. TypeScript clean (`npx tsc --noEmit` → no errors).
- follow-ups: Consider hiding the `RunStatus.tsx` header "Stop" button when the composer stop button is visible (Task-065 candidate)
- upstream docs updated: none required — this is a UI delta with no business rule or interface contract changes
