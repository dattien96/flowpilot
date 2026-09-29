# BUG-553 — stale `waiting_user_approval` children counted as open cohort members; `flow-control done` soft-defers forever

## Status
RESOLVED — CA-1068 (2026-09-29). Regression tests in
`apps/local-runner/internal/runner/bug552553_park_decision_and_cohort_test.go`
verified RED→GREEN. Filed from live run-15525 verification
(post CA-1062/1063/1064).
Related to BUG-543 (park-cancelled child keeps active leg claim) — same
residue family, different surface: 543 is the leg-claim leak, this is
the cohort-membership/counting leak.

## Live-found during
Post-fix live verification on `:4421` (runner v3, `/tmp/fp-vibe-item2`
store root), 2026-09-29 ~21:29–21:44.

## Symptom
After the escalate park cancelled in-flight turns, gated children were
left stamped `waiting_user_approval` (and `pending_resume_*` residue).
When the flow later advanced and the operator tried `flow-control done`
(or the engine evaluated cohort completion), the stale parked children
were still counted as open cohort members — the terminal evaluation
soft-deferred indefinitely because "a child is still open", even though
those children were dead residue from turns the park had already
cancelled.

Expected: a child whose owning turn was cancelled by an escalate park
(or whose leg is closed/completed) must not count toward open-cohort
membership. Step/run stamps should reflect the actual child lifecycle —
cancelled/dead children are terminal for cohort-counting purposes.

## Root cause (observed, not yet root-caused in code)
`waiting_user_approval` (and `pending_resume_*`) state on children is
not cleared when the escalate park cancels the cohort or when the flow
resumes past that point. Whatever counts "open children" for cohort
settle / flow-control-done reads the status stamp directly and treats
`waiting_user_approval` as live, so residue produced by the park-freeze
keeps the cohort permanently open.

## Reproduction
1. Vibe sprint with a delegated child that parks `waiting_user_approval`
   on a node gate.
2. Escalate park cancels in-flight work on the parent.
3. Advance the flow past the point (continue / debate resolution) — the
   child's stamp is never refreshed.
4. `flow-control done` (or cohort join evaluation) reports the cohort
   still open; the stale child blocks terminal evaluation.

## Fix direction (not implemented)
- On escalate park / cohort cancel, stamp the cancelled children
  terminal (cancelled) rather than leaving `waiting_user_approval`.
- Alternatively, make open-cohort counting ignore children whose owning
  turn/leg is already closed — count live work, not stale stamps.
- Regression test shape: parked child + escalate cancel → cohort count
  excludes the dead child; `flow-control done` settles instead of
  soft-deferring.
