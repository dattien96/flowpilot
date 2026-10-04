# CA-1176 — missing-verdict defer no longer waits on zombie members

## What changed

`apps/local-runner/internal/runner/review_done_verdict.go`:

- `missingVerdictLabelsWithLiveMember` now counts a member child as live
  only when a path still exists that can yield its verdict — via the new
  `memberVerdictStillLive` predicate (in-flight turn, queued turn prompt,
  pending approval/question gate, armed gate-reprompt or resume intent,
  live post-turn gate, `Starting` status): the same activity set
  `hasActiveFlowChild` and the cohort stall sweep already treat as real
  work.
- A non-terminal child record is now authoritative over the step stamp:
  `childSeen` suppresses the step-RUNNING channel per-label, so a stale
  RUNNING step cannot resurrect a zombie's liveness. Terminal children
  do not populate `childSeen`, so a re-dispatch window (RUNNING step,
  child row not yet inserted) still defers.
- New `flowStepRunningWithinSpawnGrace`: the no-child step-RUNNING
  channel (hub ad-hoc spawns, BUG-561) counts live only inside
  `defaultStallTimeout` of `StartedAt`; an unparseable stamp keeps the
  BUG-565 safe default. A stale RUNNING step with no child is a dead
  dispatch — it escalates instead of hanging.

## Why (live evidence, run-183756 / Task-038)

- The sprint's cohort children emitted verdicts and were gate-confirmed,
  but their run records stayed `Running` forever — `turnInFlight=false`,
  no pending turn, no gates, no intents (the missing-durable-transition
  class: turn ended, terminal stamp never landed).
- `missingVerdictLabelsWithLiveMember` counted bare non-terminal status
  as live, so every synthesis `done` returned
  `deferred_member_in_flight`. The defer waits for "the member's
  settle/join to re-invoke the hub" — a zombie has no settle left to
  fire — so the loop sat `running` with no active turns from 13:04 until
  everything was stamped CANCELED at 13:56.
- The sprint-boundary auto-advance (`maybeAutoAdvanceVibeSprintBoundary`,
  CA-1093) therefore never ran and the next task never spawned — the
  `sprint_transition_no_auto_next_task` regression the operator reported.

With the fix, a zombie member's missing verdict falls through to the
designed escalate park (`applyHubDoneVerdictTransition`); Continue
re-drives the deficient member (CA-1098 D6), the fresh leg produces its
verdict, done advances through `audit`, and the boundary auto-starts the
next sprint. Genuinely in-flight members keep the BUG-565 defer — the
test matrix pins all eight live shapes.

## Regression coverage

- `runner/bug1176_zombie_member_defer_test.go` — zombie member +
  stale RUNNING step → escalate (the live shape); eight live-member
  shapes → still defer; spawn-grace fresh/stale step window; terminal
  child not suppressing the step channel.
- `bug565_verdict_gate_inflight_test.go` — in-flight member defer,
  never-spawned escalate, Continue-spawns-member — all untouched, all
  green.
- `go test -run 'Verdict|ca1098|Hub|Sprint|Boundary|Cohort'` — 26.7s
  green across the verdict/hub/sprint/cohort families.
