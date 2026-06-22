# BUG-118: Opening Old Chat Fires Completion Notification And Bumps Timestamp

## Metadata

- Document ID: `BUG-118`
- Title: `Opening Old Chat Fires Completion Notification And Bumps Timestamp`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-112: History Reopen Truncates Multi-Turn Run](./BUG-112-History-Reopen-Truncates-Multi-Turn-Run-To-First-Turn.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `desktop, history, notification, run-toast, updatedAt, runner, resume`

## AI Quick View

### Summary

- Opening a previously-completed chat fired the "AI response complete" toast/native notification, and the chat's timestamp jumped to the current date (re-sorting it to the top of history) — as if a fresh reply had happened.
- Cause A (notification): `RunToast` fires when `status` transitions to `completed`. `openHistoryRun` replays the transcript, which drives `status` running→completed exactly like a live turn, so the toast fired on every open.
- Cause B (timestamp): `reconstructRun` set the in-memory `updatedAt` to `now` when loading a chat from disk, and the history list reports the in-memory `rs.updatedAt`, so merely opening a chat bumped its displayed time.
- Fix A: a `_historyReplaying` store flag set during the replay; `RunToast` skips toasts while it is true. Fix B: `reconstructRun` preserves the persisted `updatedAt` (falls back to now only when none is stored).

### Current Ask

- Opening an old chat must not notify "response complete" or change the chat's last-activity time.

### Key Decisions

- `V-1` `_historyReplaying` is set true in `openHistoryRun`'s state update and cleared in the replay's `.finally`, so the running→completed transitions emitted by the replay are recognized as replay (not a live turn) and suppressed.
- `V-2` `RunToast` early-returns while `_historyReplaying` — suppressing the completion toast (and the approval/question toasts, which are also unnecessary for a chat the user just opened; the cards still render in the timeline).
- `V-3` `reconstructRun` uses the persisted `st.UpdatedAt`; only a real turn (`emitLocked`) advances `updatedAt` thereafter.

### Constraints

- Live turns (`sendPrompt`/`consumeStream`) still fire the completion toast — `_historyReplaying` is false during them.
- No change to replay correctness (BUG-109/112) or the timeline reducer.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `_historyReplaying` flag, set/clear in `openHistoryRun`
- `apps/desktop-flowpilot/src/components/RunToast.tsx` — replay suppression guard
- `apps/local-runner/internal/runner/interactive_resume.go` — `reconstructRun` preserves `updatedAt`

## 1. Issue Summary

After opening an old chat, the app showed a completion notification and updated that chat's time to the current date, making it look like the chat had just replied.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-112](./BUG-112-History-Reopen-Truncates-Multi-Turn-Run-To-First-Turn.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner.
- reproduction steps:
  1. Open a completed chat from the history panel.
  2. Observe the "AI response complete" toast/notification fire.
  3. Observe the chat's time change to now and re-sort to the top of history.
- frequency: Every open of a completed chat.

## 4. Expected vs Actual

- expected: opening a chat is passive — no notification, no timestamp change.
- actual: the replay's status transition fired the toast, and `reconstructRun` bumped `updatedAt` to now.

## 5. Impact

- users affected: all desktop users browsing chat history.
- severity: Medium — misleading notifications and history ordering; erodes trust that an old chat is unchanged.

## 6. Root Cause

- confirmed cause A: `RunToast` (`apps/desktop-flowpilot/src/components/RunToast.tsx`) fires the completion toast on `status` → `completed` from an active prior status. `openHistoryRun` replays `turn_started`→`turn_completed`, producing that transition with no way to tell replay from a live turn.
- confirmed cause B: `reconstructRun` (`interactive_resume.go`) set `updatedAt: now`; the in-memory history endpoint (`interactive_handlers.go`) reports `rs.updatedAt`, so opening (which reconstructs the run into memory) surfaced `now`.
- evidence: runner log shows a `[notification]` line after `[chat-history-open]`; the history list time advanced to the open time.

## 7. Fix Strategy

- `F-1` Add `_historyReplaying: boolean` to the store; set true in `openHistoryRun`, clear in the replay `.finally`.
- `F-2` `RunToast` early-returns while `_historyReplaying` is true.
- `F-3` `reconstructRun` seeds `updatedAt` from `st.UpdatedAt` (falls back to now only if empty).

## 8. Validation

- `V-1` `tsc -p tsconfig.phase1-tests.json` passes; Go `build`/`vet` clean.
- `V-2` `store.test.js` 53/53 pass (no regression in openHistoryRun behavior).
- `V-3` Manual: opening a completed chat no longer fires the toast nor changes its history timestamp (to confirm after rebuild).

## 9. Regression Guard

- tests: existing openHistoryRun store tests cover the replay path; the flag defaults false so live turns are unaffected.
- audit checks: a `[notification]` line immediately after `[chat-history-open]` would indicate the suppression regressed.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
