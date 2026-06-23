## Metadata

- Document ID: `BUG-098`
- Title: `Desktop Chat Composer Clears Prompt Immediately After Send`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-068: Desktop Chat Prompt Skills History Not Persisted In Timeline](./BUG-068-Desktop-Chat-Prompt-Skills-History-Not-Persisted-In-Timeline.md), [BUG-083: Desktop Chat Resume Replays Composed Prompt Not User Input](./BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md)
- Replaces: none
- Tags: desktop, chat-input, composer, regression

## AI Quick View

### Summary

- After sending a prompt, the desktop composer could keep the previous text visible until the next render cycle, which made it look like the message had not been cleared.
- The issue was isolated to the local input state, not to the runner or transcript data.
- The fix clears the composer before the send call continues.

### Current Ask

- Capture the send/clear regression as a standalone bugfix record.

### Key Decisions

- `V-1` Clearing the composer should happen immediately at the UI boundary after the user sends.
- `V-2` The send path should not depend on asynchronous completion before clearing the visible input.

### Constraints

- Do not change transcript content or run history.
- Do not delay send execution just to clear the input.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/components/Timeline.tsx`

## 1. Issue Summary

The chat composer kept showing the sent prompt after submit in some flows. That made the input box look stale and caused a mismatch between what the user just sent and what the UI displayed.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop chat composer
- reproduction steps:
  1. Type a prompt in the composer.
  2. Send the prompt.
  3. Observe the input box before the next turn starts.
- frequency: intermittent before the fix, especially when the send path re-rendered later

## 4. Expected vs Actual

- expected: the input clears as soon as the prompt is submitted
- actual: the previous prompt could remain visible in the composer

## 5. Impact

- users affected: desktop chat users
- workflows affected: every prompt send flow
- severity: medium, because the stale prompt makes the send state ambiguous

## 6. Root Cause

- hypothesis: the composer was cleared too late in the send sequence
- confirmed cause: the send flow did not clear the local composer before handing control to the asynchronous send path
- evidence: the fix now calls the clear path immediately before `sendPrompt`

## 7. Fix Strategy

- `F-1` Clear the composer immediately before sending the turn.
- `F-2` Keep the send path itself unchanged so the prompt still reaches the runner normally.

## 8. Validation

- `V-1` Desktop typecheck passes.
- `V-2` Manual smoke check confirms the composer empties as soon as the prompt is sent.

## 9. Regression Guard

- tests: desktop typecheck plus manual send smoke
- alerts: none
- audit checks: the prompt content still reaches the send path unchanged

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: this is a UI state fix only
