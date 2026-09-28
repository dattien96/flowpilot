# CA-639 — Cohort Expected-Count Counts Logical Seats, Not Physical Runs (BUG-546)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-546

---

## Problem

Live `run-34296` (`:4322`, post-BUG-544 binary): the quota-vetoed
`candidate-a` leg parked correctly, the route answer spawned successor
`run-34359` holding the same seat, and both logical candidates reached DONE
— yet `cohort_join_complete` never fired and the tournament parked on
`hub_stalled`.

Root cause: expected-count healing counted **physical runs**. After the
runner restart (BUG-545 rebuild), `reconstructPendingChildSessions` saw
three session rows tagged `flow-auto-parallel_rollout-attempt-0`
(vetoed `run-34317` + successor `run-34359` + `run-34322`) and called
`preRegisterCohort(parent, cid, len(members))` → `expected=3`. The cohort
buffer dedups by **label** (`appendCohortResult` Task-241), so it can only
ever hold two entries for two seats — `cohortComplete` stayed false
forever. `ensureCohortExpectedLocked` (the live-sibling healer added for
run-98153) had the same run-counting shape.

## Fix

Both inference sites now count **distinct logical seats** — `label` when
set (same key the buffer dedups on), run id otherwise:

- `interactive_service.go` `ensureCohortExpectedLocked` — sibling scan
  dedups `r.label`, falling back to `r.id`.
- `interactive_resume.go` `reconstructPendingChildSessions` — seat set
  resolves the same label ordering the buffer append uses
  (`firstNonEmptyResumeValue(child.label, session.Label, session.AgentName)`),
  falling back to `session.RunID`.

Spawn-time registration (`spawnChildRun` `CohortSeatInherited`) and the
mid-turn-veto fresh-seat path (`respawnChildOnRoute`) are unchanged — the
fix only corrects *inference/rebuild* of a lost count.

## Tests

- `internal/runner/bug546_cohort_seat_dedup_test.go` (red first):
  - two runs sharing `candidate-a` + one `candidate-b` → inferred
    expected = 2; buffer reaches 2 → `cohortComplete` true;
  - three unlabeled members → per-run counting preserved (3).
- `TestBug544_*`, `TestBug519_*`, `TestBug543_*` still green.

## Live evidence

Pre-fix `:4322`: run-34296 buffered both labels
(`cohort_member_completed` for run-34359 at 12:16:08Z and run-34322 at
12:42:48Z) but `expected` was rebuilt as 3 on the mid-run restart →
`cohort_join_complete` absent → `hub_stalled` at 12:44:49Z.
Post-fix verification: resume the same durable run on a rebuilt binary —
the rebuild infers 2 seats, `joinRecoveredCohort` drains the already-
buffered pair and advances to `tournament_arbiter`.
