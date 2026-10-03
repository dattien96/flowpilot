# CA-1154 — BUG-623: single-member cohort join to a delegate target dead-ends

Live trigger: run-150388, sprint 2 (Task-032). After the BUG-622 resume fix
re-armed the sprint and `context → tdd` spawned leg run-171163, the tdd leg
completed cleanly — and the run parked again:

```
21:18:27 tdd DONE → synthesis RUNNING → hub reinvoke scheduled
21:18:43 hub done blocked: reviewer machine verdict missing or not approved
21:18:43 synthesis WAITING_USER_APPROVAL
```

Root cause: `tryAdvanceFlowFromNode` stamps `flow-auto-<node>-round-<N>`
cohort ids on every spawned target, so the tdd leg settled through the
cohort-join path. `flowSharedInlineJoinTarget` only resolves a SHARED INLINE
node (coder→validate, reviewer→synthesis); tdd→coder is a delegate target,
so the join fell through to a hub reinvoke. But the hub cannot express the
member's edge — its only instrument (`flow_control continue`) resolves the
HUB node's own edges (`synthesis → coder`) and that back-edge only
reinvokes an *existing* child ("matched no existing child and did not spawn
one"). No coder leg could ever be produced; the hub tried `done`, was
correctly blocked by the missing reviewer verdict, escalated, parked.

Same bug class as BUG-616/620/622: a mechanism designed for multi-member
joins silently misapplied to the single-member case that
`tryAdvanceFlowFromNode` creates on every single-target advance.

## Fix

`settleFlowChildTurnCompletedLocked` cohort-join arm now detects a
single-member cohort whose forward `done` targets are all
delegate-spawnable (`singleMemberDelegateJoinTarget` in flow_executor.go)
and fires the member's own done-edge through `tryAdvanceFlowFromNode` —
the same advance an un-cohorted leg gets via `advanceOrNotifyHub`. On a
no-edge result it falls back to the hub-note reinvoke unchanged. The hub
RUNNING stamp is skipped for this shape (no hub turn is scheduled — a
ghost RUNNING would trip the stall watchdog). Multi-member cohorts,
shared-inline targets, writer joins, and `→ done`/`→ hub` terminations are
byte-identical to before.

## Files

- `internal/runner/interactive_service.go` — join-path arm + stamp guard.
- `internal/runner/flow_executor.go` — `singleMemberDelegateJoinTarget`.
- `internal/runner/bug623_single_member_delegate_join_test.go` — red-verified
  repro: single-member tdd join now spawns the coder leg; pre-fix it only
  reinvoked the hub.

## Verification

- `TestBUG623_SingleMemberCohortJoinAdvancesDelegateTarget` red before fix
  (6s timeout, no coder leg), green after (1.1s).
- Regression family green: CA-789/1087/1091/1092/1096/1098/1099/1151,
  BUG-318/364/426/454/458/478/491, run-98153 dual-reviewer join,
  BUG-580 verdict survival, review-reprompt family — 5.4s.
- Baseline-broken on this machine (fail identically with and without the
  change): BUG-377/425/440/514 reprompt-gate env family, BUG-334 process
  family, Firebase MCP, gate-blind env; contract-freeze git-fixture tests
  flake on `git` subprocess stall under load and pass isolated.
