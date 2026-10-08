# BUG-647 — A completed child leg never settles its parent flow-step mirror: the step stays `RUNNING` after the leg is `completed`, blocking `tdd.done`-style edges until an operator force-settles

- **ID:** BUG-647
- **Severity:** High — every stale RUNNING step blocks its outbound edge
  AND wedges resume (BUG-656: the same stale stamp suppresses the
  resume-confirm gate). Cost observed: ~4h park + one full wasted TDD
  leg re-run.
- **Status:** OPEN (captured live, run-523131; sibling class BUG-464 for
  cancelled/stop shape, BUG-540 for in-session shape)

## Evidence chain (all live)

1. run-523131, sprint-4 TDD leg `run-570489` reached `completed` — its
   provider turn finished and wrote real output — while the parent
   `tdd` step mirror stayed `RUNNING`.
2. `tdd --done--> coder` never fired; `coder`/`validate`/review cohort
   stayed PENDING ~4h while `synthesis` re-parked on "missing verdict".
3. On quota-death resume the checkpoint picker offered `tdd` (the stale
   RUNNING step) — the runner re-ran the whole scaffold leg instead of
   resuming at `coder`.
4. Operator `flow-control {status:"continue"}` fired the synthesis
   back-edge and unblocked the sprint — proving only the settle was
   missing.

## Root cause (hypothesis)

`settleFlowChildStepTerminalLocked` (added CA-1234 for fail/cancel) does
not cover the **completed** terminal shape — a leg that finishes cleanly
leaves its step mirror RUNNING forever.

## Fix direction

- `F-1` Extend terminal settle to `completed` legs: on leg completion,
  stamp the owning flow step `DONE` (or the step's mapped success state)
  in the same ledger write.
- `F-2` Reconcile sweep: a step whose only leg is terminal-completed
  heals to DONE on the next sweep, not just on resume.

## Regression coverage

- `TestBug647_CompletedLegSettlesStep` — leg completes → step mirror
  `DONE` without operator input.
- `TestBug647_ResumeCheckpointSkipsSettledStep` — stale-free resume
  offers `coder`, not the already-done `tdd`.
