# CA-1184 — done-edge auto-redrive for terminal verdict-deficient members

## Defect

A review-cohort member whose leg dies terminal (`turn_failed`, provider
error, cancelled) never records its machine verdict. At the done-edge,
`missingVerdictLabelsWithLiveMember` correctly excludes terminal children —
so the gate fell straight through to escalate → human park. The operator
had to click Continue (which routes to `resumeVerdictDeficientMembers`,
CA-1098) for a defect the engine can remediate itself. Live evidence:
cohort join parked awaiting a verdict from a member that no longer exists.

## Fix

`autoRedriveVerdictDeficientMembers(parentRunID)` runs at BOTH done-edge
verdict-gate sites (`synthesisDoneVerdictError` in applyFlowControl,
`hubDoneVerdictError` in advanceHubDoneThroughEdge) when the live-member
defer does not apply:

- **Scoped trigger** — requires a MISSING verdict backed by a TERMINAL
  member record (failed/cancelled/completed). Never-spawned members keep
  the CP-61 escalate contract; bare-zombie (non-terminal, channel-less)
  children keep BUG-1176's escalate; a recorded-but-not-approved verdict
  stays human-adjudicated.
- **Bounded** — `verdictAutoRedrives` on the run caps at
  `maxVerdictAutoRedrives = 2` per episode; the counter resets when a
  fresh member verdict lands in `snapshotReviewCohortVerdictsLocked`
  (progress ⇒ next episode gets a fresh bound).
- Reuses `resumeVerdictDeficientMembers` — resurrects the terminal member
  via `reinvokeMatchingFlowChild`, or spawns a replacement leg via
  `spawnFlowDelegateLeg` when the child record is absent. Success ⇒ the
  done-edge DEFERS identically to the member-in-flight branch (unstamps
  the decision, keeps the loop running; the member's settle rejoins the
  cohort and re-invokes the hub).

## Regression evidence

- `bug1184_terminal_member_redrive_test.go`: terminal-failed reviewer +
  missing verdict → member re-driven with the CA-1098 verdict prompt, hub
  done deferred; budget spent → escalate; landing verdict resets budget.
- CP-61 missing-verdict escalate tests, BUG-1176 zombie tests, CA-1098
  Continue-redrive tests, BUG-565 defer tests: all green unchanged.

## Files

- `apps/local-runner/internal/runner/review_done_verdict.go` —
  `autoRedriveVerdictDeficientMembers`, `maxVerdictAutoRedrives`, budget
  reset in `snapshotReviewCohortVerdictsLocked`
- `apps/local-runner/internal/runner/interactive_service.go` — both
  done-edge sites wire the auto-redrive; `verdictAutoRedrives` field
- `apps/local-runner/internal/runner/bug1184_terminal_member_redrive_test.go`
