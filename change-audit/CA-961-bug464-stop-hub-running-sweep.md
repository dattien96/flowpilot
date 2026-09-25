# CA-961 — BUG-464: Stop sweeps non-member RUNNING/WAITING flow steps to CANCELED

- type: bugfix
- bug: BUG-464 (found live run-47170 — B-51-6 crash-recovery drill)
- feature: agent-flow-engine
- follows: CA-960

## Change

`internal/runner/interactive_service.go` `stopAgentLoop`:

- After the cohort-member cancel block, flow-engine-driven runs now sweep
  `LoadRunSteps` and stamp `CANCELED` on every non-terminal, non-PENDING
  row (`RUNNING`, `WAITING_USER_APPROVAL`). The member-label stamps only
  ever covered cohort children; the inline hub node — stamped `RUNNING` by
  a cohort join while `loopIsAdvancing` was still true an instant before
  the loop sealed — was stranded forever on a cancelled run (live:
  `ss_validator` RUNNING on cancelled run-47170, durable in
  `run-47170-step-transitions.ndjson`).
- `PENDING` stays `PENDING` (never started — restart evidence-walk
  convention); terminal rows untouched.

## Tests (additive only)

- `TestStopAgentLoopSweepsNonMemberRunningSteps` — RED-by-assertion
  (hub `synthesis` RUNNING survived Stop before the fix; member DONE /
  never-started PENDING rows verified untouched).

## Provider parity

Provider-agnostic — shared stop path, step-runtime store, and transition
log; no adapter/session/event code touched.
