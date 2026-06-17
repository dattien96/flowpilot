# Task-058: Run Toast Notifications — AI Response Done & Approval Required

## Metadata

- Document ID: `Task-058`
- Title: `Run Toast Notifications — AI Response Done & Approval Required`
- Phase: `task`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `DatNguyen`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [SS-08-Approve-Gate.md](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-10-Full-Flow.md](../../05-System-Specs/SS-10-Full-Flow.md), [SD-09-Approval-Gates.md](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Child Documents: `none`
- Related Documents: [Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md](../done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md), [BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md](../../09-BugFix/done/BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md)
- Replaces: `none`
- Tags: `notification, toast, ui, approval, run-lifecycle`

## AI Quick View

### Summary

- Adds a `RunToast` component that watches Zustand `status` transitions and renders in-app toast notifications at the bottom-right corner of the screen.
- Shows "AI response complete" toast (green, 4 s) when `status` transitions to `completed`.
- Shows "Approval required" toast (yellow, 8 s) when `status` transitions to `waiting_approval`.
- Shows "AI has a question" toast (purple, 8 s) when `status` transitions to `waiting_question`.
- Also fires the Web `Notification` API for native OS-level notifications when the document is hidden.

### Current Ask

- Done. In-app toast notifications appear at bottom-right on AI response complete, approval required, and question required events.

### Key Decisions

- `T-1` Zustand selector + `useRef` prev-status pattern detects exact transitions without adding new store state.
- `T-2` No external toast library — self-contained CSS using existing `--ok`, `--warn`, `--ask` design tokens.
- `T-3` Native `window.Notification` fires only when `document.hidden` to avoid redundancy when the app is focused.
- `T-4` `waiting_question` included alongside `waiting_approval` since both require immediate user action.

### Constraints

- No new npm dependencies.
- Toasts must auto-dismiss (4 s for done, 8 s for approval/question) and be manually dismissible.
- Additive UI only — does not modify existing approval card or question card flows.

### Open Questions

- None.

### Source Refs

- SS-08 AC-1 (approval gate halts execution until user acts)
- SS-10 Section 14 (step completion outcomes)
- SD-09 Section 3 (notification on WAITING_USER_APPROVAL — this task is the desktop-client equivalent)

## 1. Goal

Surface a visible, actionable notification when the AI run reaches a terminal or user-input-required state, so the user does not miss a completed response or a pending approval request when they are not actively watching the timeline.

## 2. Parent Links

- coding plan: no dedicated CP (additive UI anchored to SS-08 / SS-10 run lifecycle)
- tech design: SD-09-Approval-Gates.md (Section 3 — notification on WAITING_USER_APPROVAL)
- system spec: SS-08-Approve-Gate.md, SS-10-Full-Flow.md
- specific upstream ids: SS-08 AC-1, SS-10 Section 14

## 3. Trigger

User requested in-app notifications for two events: AI response complete and approval needed. No in-app notification mechanism existed in the desktop app (only the `RunStatus` dot in the header and timeline messages). SD-09 Section 3 already specified a Telegram notification for `WAITING_USER_APPROVAL`; this task implements the desktop-client equivalent.

## 4. Exact Change

- `T-1` Created `apps/desktop-flowpilot/src/components/RunToast.tsx` — subscribes to Zustand `status`, tracks previous status via `useRef`, pushes toast entries on `completed` / `waiting_approval` / `waiting_question` transitions, auto-dismisses, and fires `window.Notification` when `document.hidden`.
- `T-2` Appended `.run-toast-stack`, `.run-toast`, and variant CSS classes to `apps/desktop-flowpilot/src/styles.css` using existing design tokens.
- `T-3` Imported `RunToast` in `apps/desktop-flowpilot/src/App.tsx` and mounted it inside the authenticated render tree (fixed-positioned via CSS, renders only when authenticated).

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/RunToast.tsx` (new)
  - `apps/desktop-flowpilot/src/styles.css` (appended toast CSS)
  - `apps/desktop-flowpilot/src/App.tsx` (import + `<RunToast />` mount)
- modules: desktop-flowpilot UI layer
- routes: n/a (Electron desktop app)
- tables: none

## 6. Acceptance Check

- After sending a prompt and waiting for the AI to finish, a green "AI response complete" toast appears at the bottom-right and auto-dismisses after 4 seconds.
- When the AI requests approval (`permission_required` event), a yellow "Approval required — AI is waiting for you" toast appears.
- When the AI asks a question (`user_question_required`), a purple "AI has a question for you" toast appears.
- Each toast has an ✕ button to dismiss manually.
- Toasts do not appear for idle or intermediate streaming states.
- `tsc --noEmit` passes with zero errors.

## 7. Out of Scope

- Telegram / external push notifications (SD-09 Section 3, separate backend concern).
- Persistent notification history or notification center.
- Notifications for `turn_failed` (failure already shown as red system message in the timeline).
- Sound effects or badge counts.
- User setting to toggle toast or native notifications.

## 8. Completion Notes

- result: Implemented. `tsc --noEmit` reports zero TypeScript errors.
- follow-ups: Consider a user preference to enable/disable toast or native OS notifications.
- upstream docs updated: No upstream intent changed; SD-09 Section 3 is the governing spec and remains accurate.
