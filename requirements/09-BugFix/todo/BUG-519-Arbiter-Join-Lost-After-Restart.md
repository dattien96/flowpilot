# BUG-519 — Tournament arbiter join can never satisfy after runner restart

Status: FIXED + live-verified (run-82594 restart leg, 2026-09-26 r9)
Filed: 2026-09-26 (round-5 live test, run-49109 post-restart resume)
CA: CA-1027

## Symptom

Run-49109 parked on `tournament_arbiter_blocked` (BUG-518). After a
runner restart + `/resume` + `agent-loop/continue`, the re-dispatched
arbiter logged:

```
[tournament] arbiter join waiting on sibling "candidate-a" (run "run-49109")
```

forever — both candidates had completed and stamped DONE pre-restart.
The parked arbiter step went CANCELED and the run re-blocked; no
operator action could unstick it.

## Root cause

`tournamentJoinSatisfied` scanned `s.runs` — the in-memory run map — for
a terminal child per forward-done predecessor. After a restart,
`reconstructPendingChildSessions` deliberately restores only
live/pending children; a fully-finished cohort stays on disk only. The
memory scan therefore found no terminal `candidate-a`/`candidate-b`
child and waited on them permanently — a durability-contract violation
(state surviving restart is the strongest house invariant).

## Fix (CA-1027)

`tournamentJoinSatisfied` (tournament_dispatch.go) now does the memory
scan first, then falls back for still-pending predecessors to the
durable step-transition sidecar (`StepTransitionLogStore.LoadStepTransitions`,
last status per node wins). A predecessor whose final stamp is DONE /
FAILED / CANCELED / SKIPPED satisfies join:all exactly like a terminal
in-memory child — the run-1890 contract (failed members count) is
preserved. Store absent / log unreadable → nil map → memory-only
behavior (fail-closed on missing evidence). I/O runs outside `s.mu`.

## Tests

`internal/runner/bug519_arbiter_join_restart_test.go` —

- `TestBug519_JoinSatisfiedViaStepLogAfterRestart`: children absent from
  `s.runs`, sidecar stamps both candidates DONE → join satisfied.
- `TestBug519_JoinStillBlocksOnNonTerminalStepLog`: candidate-b's last
  stamp RUNNING → join still blocks (no fail-open).
- `TestBug519_JoinCountsFailedStepLogStamp`: FAILED stamp satisfies
  join (run-1890 semantics preserved).

Regression: `TestBug426_*`, BUG-446/453/459/478 tournament battery all
green unchanged.

## Live evidence

- Found: `run-49109` flow-diag `arbiter join waiting on sibling
  "candidate-a"` after r6 restart + resume.
- Post-fix live leg (r9, `run-82594`): tournament parked at the
  `merge_and_audit` decision card (winner candidate-a empty patch →
  human card armed) → `kill -9` → restart → `resume` rehydrated the run
  as `blocked` with the durable `decision_card` + `tournament_patches`
  intact while all cohort children were absent from memory →
  `continue` with `candidate-b` → patch conflict → fail-closed human
  card → `continue` with operator-resolved diff → merge applied
  (`mathx/` landed in main tree) → `merge_and_audit` DONE → run
  `completed`. No step waited on missing children; every durable
  terminal/non-terminal stamp was honored.
- Note: mid-turn kills (`status=running` persisted) still normalize to
  `cancelled` on resume — designed uncertain-delivery fail-closed, not
  this bug. The restart-leg must kill at a parked state.
