# CA-1177 — Child gate-settle must defer while parent loop is blocked

## Evidence

Live run-183756 (PrivateVault Task-038 vibe sprint): cohort children
(`run-201967` spec_align, `run-201977` reviewer) finished their provider turns
with verdicts in the transcript but stayed `status=running` for 26+ minutes —
zombies. Manual `gate-decision`/`resume` API calls were the only way their
verdicts joined; the sprint-boundary then saw missing verdicts and deferred
forever (`deferred_member_in_flight` at 13:04, all legs CANCELED by 13:56).

## Root cause

`resumePendingFlowGate` (interactive_service.go) drops `pendingFlowGateSettle`
via `clearStaleFlowGateIntentsLocked` whenever the run's effective loop —
**the parent's loop for children** — is `stopped`/`done`/`blocked` (P1-04 /
run-63960).

For a CHILD, the settle flag is the *owed terminal disposition* of a finished
provider turn, not a spawn intent. `markPendingFlowGateSettleLocked` pins
`status=running` while armed, so dropping the settle:

1. leaves `status=running` forever (no stamp path remains),
2. loses the verdict — `appendCohortResult` never runs for the member,
3. leaves no driver — the dispatch record either finalized the settle as
   "bookkeeping" (evaluateSettleGate allows when the flag is gone) or the
   record never reached terminal+SettleOwed, so neither
   `drivePendingSettlesOnBoot` nor the BUG-540 in-session sweep sees it.

The P1-04 hazard was designed for ROOT runs: a root's reprompt `startTurn`
hits `flow_awaiting_user` → wipe to Failed with no card. A child's reprompt
goes through `handleSpawnedChildTurnFailure`, which treats
`flow_awaiting_user` as a RETRYABLE refusal and parks a durable resume intent.
Every child eval outcome is therefore safe: pass→cohort join, park→visible
card, reprompt→durable fenced park.

The wedge sweep (`sweepWedgedFlowWork`, BUG-571 class 1) already re-fires
`resumePendingFlowGate` every 30s for armed settles — so deferring is
self-healing: the armed settle converges automatically once the parent
unblocks.

## Fix

`resumePendingFlowGate`: for `rs.parentRunID != ""` when the effective loop is
`blocked` — keep `pendingFlowGateSettle` and its payload armed, still drop
reprompt spawn intents (`clearIntentFieldsLocked(rs, "reprompt")`, gen
high-water preserved — the deferred eval re-derives them), persist the
reprompt drop, and return without evaluating.

Unchanged (by design):

- Root run + blocked own-loop → drop (P1-04 original hazard).
- Child + parent `stopped`/`done` → drop (terminal parent owns children via
  `reconcileChildRunsOnFlowDone`; re-arming would reschedule every restart).
- Single-flight `gateClaimID` / live `gateCancelLive` checks — untouched.

## Files

- `apps/local-runner/internal/runner/interactive_service.go` — child defer
  branch inside the blocked-loop guard.
- `apps/local-runner/internal/runner/bug1177_child_settle_defer_test.go` —
  NEW: defer-while-blocked, converge-on-unblock, terminal-parent still drops.
- `apps/local-runner/internal/runner/run63960_blocked_restart_no_gate_reprompt_test.go`
  — pinned the defect: `TestRun63960ChildGateResumeSkippedWhenParentBlocked`
  now asserts the settle defers (armed) and reprompt drops; no-turn/gen
  coverage preserved.
- `apps/local-runner/internal/runner/bug327_scope_drift_ledger_exemption_test.go`
  — same pinned assertion corrected (`loop_already_blocked_early_return`
  still asserts the early return + park survival; the flag now must stay
  armed).

## Tests

- `go test -count=1 -run 'TestBug1177|TestRun63960|TestResumePendingFlowGate' ./internal/runner/` — pass.
- `go test -count=1 -run 'Settle|PendingFlowGate|Wedge|Stale|Cohort|Resume|V10|Blocked|Verdict' ./internal/runner/` — only pre-existing environmental
  fails (missing `codex`/`agy` provider binaries, TempDir cleanup flake).

## Interaction with BUG-1176

BUG-1176 made `missingVerdictLabelsWithLiveMember` detect zombies
(`memberVerdictStillLive` requires an executable continuation). BUG-1177
removes the most common zombie *factory*: the dropped settle under a blocked
parent. Together: settles survive to join (1177), and any zombie that still
appears escalates instead of deferring forever (1176) → sprint boundary can
auto-advance or redrive.
