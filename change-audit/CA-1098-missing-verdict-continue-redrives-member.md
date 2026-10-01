# CA-1098: missing-verdict escalate Continue re-prompted the hub forever instead of re-driving the reviewer member

Date: 2026-10-01
Refs: live scratch run-31884 (vibe-tasks on fp-debate-live). The sprint
reviewer child (run-33967) settled verdict-less on a provider that never
calls `submit_review_outcome`; the settle-time CP-67 reprompt budget
(`verdictRepromptCount`, cap 2) exhausted, the member joined the `review`
cohort verdict-less, and `hubDoneVerdictError` rejected the synthesis hub's
`approved → done`. Every operator Continue then routed back to the synthesis
hub, which re-submitted `approved` and was rejected identically — observed
live as `blocked / awaiting_user` at round 0/3 repeating forever. The hub can
never manufacture the missing member verdict, so the generic reinvoke is
structurally wedged.

## Root cause

`resumeFlowWithFeedback` had no verdict-aware escalate routing. A park whose
GateReason is `done blocked — missing machine verdict from review
reviewer(s): …` (or `verdict not approved`) fell through to the generic
hub reinvoke, which re-prompts the hub with operator feedback — the one
actor that cannot fix the deficiency.

## Fix

- `internal/runner/review_done_verdict.go`
  - `isReviewVerdictGateReason`: recognizes gate reasons containing
    `done blocked` plus `machine verdict` or `verdict not approved`.
  - `resumeVerdictDeficientMembers`: resolves the active hub's inbound
    `review` cohort, merges pending + last-known verdicts, and re-drives
    every member whose verdict is not `approved` via
    `reinvokeMatchingFlowChild` with an explicit "call
    submit_review_outcome FIRST" prompt. Each retried member's
    `verdictRepromptCount` resets to 0 — a user-driven Continue is a fresh
    attempt, not a continuation of the exhausted budget — and
    `reinvokeInFlight` is armed so a failed dispatch retries/drains instead
    of dropping silently (BUG-403 pattern). Returns false when no deficient
    member maps to a live child, preserving the generic resume.
- `internal/runner/interactive_service.go`
  - New branch in `resumeFlowWithFeedback`, placed after the CA-1074
    validation-retry branch and before generic escalated-node handling:
    `prevBlockReason == "escalate" && isReviewVerdictGateReason(prevGateReason)`
    → `resumeVerdictDeficientMembers`.

The member's re-settle rejoins the cohort through the normal path
(`recordReviewCohortMemberVerdict` → `snapshotReviewCohortVerdicts`), so the
hub is re-invoked by its own edges and re-decides on fresh verdict data —
no hub prompt is synthesized by the resume path.

## Verification

- Red→green: `ca1098_missing_verdict_resume_test.go` (new file; no existing
  test modified).
  - `TestCA1098MissingVerdictContinueRedrivesReviewer` (claude/codex/grok
    matrix): Continue on the missing-verdict park re-dispatches the reviewer
    child with the verdict instruction; the CP-67 reprompt then fires again
    — only reachable with a reset budget (member entered at count=2/cap).
  - `TestCA1098HubTurnNeverPrecedesMemberRedrive`: the only legitimate
    parent-hub turn is the cohort re-join reinvoke after the member's
    re-driven turn; a hub prompt arriving first is the wedge.
  - `TestCA1098UnrelatedEscalateKeepsGenericResume`: a non-verdict escalate
    keeps the generic hub-reinvoke path (no member hijack).
- Red proof: pre-fix run showed `reviewer child status = "completed"` and
  the captured prompt byte-identical to the live wedge's hub re-prompt.
- Adjacent suites (verdict/resume/cohort/escalate patterns) green; `go vet`
  clean. Test fixture note: `finishTurn` synthesizes `TurnCompleted` when an
  adapter returns nil without terminal events, so settle-time counters are
  asserted behaviorally (reprompt firing proves the reset) rather than by
  racing the counter.
