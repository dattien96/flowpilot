---
id: CA-936b
title: Flow resume/reliability prod fixes — hub self-escalation stamp, child reprompt retry drain, post-completion fake artifacts (BUG-454)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

Three deterministic baseline defects in `internal/runner` surfaced by
BUG-454's suite triage (all red on clean baseline `d191004f`):

- **Hub self-escalation left hub parked**: `resumeFlowWithFeedback` only
  restamped the hub `RUNNING` when `lastEscalatedInlineNodeID` matched the
  hub id or a pending prompt existed. When the *hub itself* escalates, that
  field is empty (it is only set for non-hub writer/audit/child-gate
  escalations), so the hub stayed visually `WAITING_USER_APPROVAL` even
  though generic reinvocation ran. `TestResumeFlowWithFeedbackAfterEscalate`.
- **Child verdict reprompt dropped on transient `turn_in_progress`**:
  `scheduleChildTurn`'s error handler re-armed `pendingHubReinvoke` on the
  *child* run, but `notifyTurnIdle` only drains that flag when
  `parentRunID == ""` — so a reprompt racing the previous turn's teardown
  was silently swallowed and the cohort never joined. The three E2E
  review-loop tests timed out on this. (BUG-454 residual of BUG-403.)
- **Post-completion fake artifacts**: `handleListArtifacts` served the
  hardcoded `fakeArtifacts` stub while `turn_completed` was already emitted
  but the real finalizer artifacts were still being written — a client
  polling after completion could read `"3 files changed"` placeholders.
  `TestFinalizerHookSurfacesArtifacts`.

## Change

- `interactive_service.go`: `resumeFlowWithFeedback` treats
  `lastEscalatedInlineNodeID == ""` as "the hub itself escalated" and
  restamps the hub `RUNNING` (`escalatedIsHub = esc == "" || esc == hubID`).
  Non-hub escalate no-flap behaviour is preserved.
- `flow_executor.go` (`scheduleChildTurn`): on transient `turn_in_progress`
  rejection the retry is rescheduled through `scheduleChildTurn` again
  instead of parking `pendingHubReinvoke` on a child run that can never
  drain it.
- `interactive_handlers.go` (`handleListArtifacts`): once a run has a
  finished turn (`lastTurnID` set), the fake-artifact stub is skipped and
  the handler falls through to real finalizer artifacts only.

## Tests

- Reproduce-first: `bug454_child_reprompt_retry_test.go`
  (`TestChildVerdictRepromptRetriesAfterTransientTurnInProgress`) failed on
  pre-fix code (`failCount=1`, undrainable `pendingHubReinvoke`) and passes
  after; existing `TestResumeFlowWithFeedbackAfterEscalate` and
  `TestFinalizerHookSurfacesArtifacts` flip red→green.
- E2E: `TestE2EReviewLoop{ApprovedPath…,MultiRoundChangesThenApproved,
  MultiRoundSlowSynthesisStillCompletes}` all green (`ok … 4.431s`).

## Result

- Hub restamp honours the durable-state contract without weakening
  non-hub escalation parking.
- Child reprompts are retryable through the same scheduling seam instead of
  a dead-end flag — bounded by the existing retry surface.
- Artifact endpoint no longer fabricates results after completion; the
  pre-completion fake-artifact stub path is unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-454
change_type: bugfix
summary: Hub self-escalation restamp on resume; child reprompt retries through scheduleChildTurn instead of undrainable pendingHubReinvoke; fake artifacts no longer served after a turn completes
# --->8---
