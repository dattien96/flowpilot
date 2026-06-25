# BUG-143 — Desktop Chat Area Lacks Mode-Aware Stroke Border

## Metadata

- Document ID: `BUG-143`
- Title: `Desktop chat area lacks mode-aware stroke border`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [Task-114: Chat Mode Selector Normal/Task/Bug](../../08-Task/done/Task-114-Chat-Mode-Selector-Task-Bug-Normal.md), [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- Child Documents: none
- Related Documents: `CA-128` (r-task rule), `CA-126` (gate-block ui)
- Replaces: none
- Tags: `chat-ui, ux, visual-indicator, mode-feedback`

## AI Quick View

### Summary

- The chat workspace main area (`<main class="workspace-main">`) has no visual indicator of the active mode (Normal / Task / Bug / Flow).
- After Task-114 introduced the Chat Intent sub-mode selector (Normal / Task / Bug), the mode state is held in the store but not reflected on the primary chat region itself — only the right-sidebar chip shows the locked mode after first turn.
- Users have no persistent glanceable cue that they are in Task (green) or Bug (red) mode while composing turns, nor that they are in Flow mode (gradient).
- Missing visual feedback can cause accidental wrong-mode submissions.

### Current Ask

- Add a colored stroke border around the `workspace-main` element that switches based on the active mode: Normal = none, Task = green, Bug = red, Flow = gradient.

### Key Decisions

- `V-1` Border derives from the same mode logic used by `ChatStartIntentPanel`: pre-turn uses `chatStartMode`; post-first-turn uses `lastTurnInput?.changeType ?? "normal"`.
- `V-2` Flow mode (`chatMode === "workflow_step_auto"`) always shows gradient regardless of `chatStartMode`.
- `V-3` Implemented via three additive CSS classes (`chat-area-task`, `chat-area-bug`, `chat-area-flow`) applied dynamically to the `<main>` element — zero layout impact for Normal mode (no border added).
- `V-4` Gradient border for Flow uses `border-image: linear-gradient(...)` — valid because `.workspace-main` has no `border-radius`.

### Constraints

- Must not alter layout in Normal mode (no border = no extra space).
- Must not break existing `workspace-main` flex sizing.
- `border-image` and solid-color `border` consume 2 px from the element's padding-box — acceptable for the chat area's size.

### Open Questions

- none

### Source Refs

- `Task-114` (introduced `chatStartMode`, `ChatStartMode`, `ChatModeIntentIcon`, `ChatStartIntentPanel`)
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` — `ChatWorkspace`, `ChatStartIntentPanel`
- `apps/desktop-flowpilot/src/styles.css` — `.workspace-main`
- `apps/desktop-flowpilot/src/state/store.ts` — `ChatMode`, `ChatStartMode`, `chatStartMode`, `lastTurnInput`
- `apps/desktop-flowpilot/src/types/contract.ts:321` — `TurnInput.changeType?: "task" | "bugfix"`

## 1. Issue Summary

After Task-114 added the Chat Intent sub-mode picker (Normal / Task / Bug) to the right sidebar, the selected mode has no visual representation on the chat area itself. The right-sidebar chip only appears after the first turn is sent (as a "Declared Intent" locked state), and is absent while the user is composing the first turn. There is also no visual distinction when the workspace is in Flow (Workflow) mode.

The result is that users composing a prompt have no persistent glanceable signal of which mode is active on the primary chat region.

## 2. Parent Links

- impacted coding plan: [CP-35](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- impacted tech design: none directly (pure UI presentation layer)
- impacted system spec: none (no business rule change)

## 3. Environment and Reproduction

- environment: desktop app, authenticated, runner reachable
- reproduction steps:
  1. Open desktop app, navigate to Chat workspace.
  2. In the right sidebar, select **Task** mode in Chat Intent panel.
  3. Observe the chat area (`<main>`) — no visual change; border is absent.
  4. Switch to **Workflow** mode (top tab in WorkflowControlPanel) — same absence.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: chat area shows a 2px green border in Task mode, red border in Bug mode, gradient border in Flow mode, and no border in Normal mode
- actual: chat area always shows no border regardless of active mode

## 5. Impact

- users affected: all desktop users using Task / Bug / Flow modes
- workflows affected: chat mode selection UX; no functional run behavior affected
- severity: low — UX feedback gap, no data loss or run failure

## 6. Root Cause

- hypothesis: Task-114 wired the mode state and the right-sidebar chip but did not add a corresponding visual to the primary chat region.
- confirmed cause: `ChatWorkspace` did not read `chatMode` / `chatStartMode` / `lastTurnInput` and applied no CSS class to `<main class="workspace-main">`.
- evidence: `ChatWorkspace.tsx` — the `<main>` element has only static class `"main workspace-main"`; no CSS class or style reflects mode state. `.workspace-main` in `styles.css` has no border rule.

## 7. Fix Strategy

- `F-1` In `ChatWorkspace`, read `chatMode`, `chatStartMode`, `timeline`, and `lastTurnInput` from the store.
- `F-2` Derive `activeChatSubMode`: when turns exist use `lastTurnInput?.changeType ?? "normal"`, otherwise use `chatStartMode`.
- `F-3` Compute `chatAreaClass`: `"chat-area-flow"` when `chatMode === "workflow_step_auto"`, `"chat-area-task"` when `activeChatSubMode === "task"`, `"chat-area-bug"` when `activeChatSubMode === "bugfix"`, empty string otherwise.
- `F-4` Apply `chatAreaClass` to the `<main>` element: `className={\`main workspace-main\${chatAreaClass ? \` \${chatAreaClass}\` : ""}\`}`.
- `F-5` Add CSS rules in `styles.css` immediately after `.workspace-main`: `.chat-area-task` (green `border`), `.chat-area-bug` (red `border`), `.chat-area-flow` (`border-image` gradient).

## 8. Validation

- `V-1` Switch to Normal mode → no border visible on chat area.
- `V-2` Switch to Task mode before first turn → green `rgba(64,196,99,0.7)` border appears on chat area.
- `V-3` Switch to Bug mode before first turn → red `rgba(240,72,72,0.7)` border appears on chat area.
- `V-4` Switch to Workflow (Flow) mode → gradient border appears (`#4c8dff → #a855f7 → #ec4899 → #f97316`).
- `V-5` Send first turn in Task mode → border remains green (derived from `lastTurnInput.changeType`).
- `V-6` Confirm Normal mode after turns leaves no border (no rogue class).
- `V-7` TypeScript build passes with no new type errors.

## 9. Regression Guard

- tests: no dedicated unit test added — pure CSS class toggle on existing store selectors; `store.test.ts` already covers `changeType` propagation (`Task-114`).
- alerts: none
- audit checks: `CA-129`

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a pure UI presentation addition, no business rule or AC changed
- notes left unchanged on purpose: `Task-114` correctly defines the mode concept; this bug doc records only the missing visual layer
