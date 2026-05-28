# BUG-004: Completed Sub-Agent Step Must Not Show Follow-Up Chat

## Problem
The workflow run detail page currently shows the follow-up chat input for a completed step even when that step was configured with a `subagent`.

This is misleading UX.

For sub-agent steps, the isolated session is intentionally terminated after the step completes successfully. The UI should not imply that the user can continue chatting with that sub-agent session.

## Current Behavior
- A step with `subagent != null` runs in an isolated session.
- After the step succeeds, runtime cleanup closes that sub-agent session.
- The step detail UI still shows the follow-up chat box because completed steps are treated as generally continuable.

## Expected Behavior
- If a step uses a `subagent`, and the step is already completed, do not show the follow-up chat input.
- The user should still be able to:
  - view the step output
  - view session history
  - inspect logs
  - inspect session metadata
- The step should be treated as finished unless the workflow explicitly re-enters it through a new runtime action.

## UX Decision
This is not a runner bug and not a session-termination bug.

The session termination behavior is correct for sub-agent steps.

The actual UX issue is:
- the UI offers a follow-up chat affordance for a session type that is intentionally short-lived and already closed.

## Root Cause
The follow-up chat section is gated by step completion state, but it does not distinguish between:
- main workflow sessions that remain reusable
- isolated sub-agent sessions that are closed after success

## Fix Direction
- In the workflow run detail page, hide the follow-up chat input when the selected step definition has a `subagent`.
- Keep the follow-up chat available for non-subagent steps that reuse the main session.

## Acceptance Criteria
- A completed step with `subagent != null` does not show the follow-up chat input.
- A completed step without a subagent can still show the follow-up chat input when main-session continuation is supported.
- The page still shows outputs, logs, and session details for completed sub-agent steps.
- No change is made to the current runtime rule that closes successful sub-agent sessions.
