# CA-402 - run-43831: hub stall gap after child complete

## Symptom

Live Review Loop `run-43831`: coder (`gpt-5.4-mini`) ran ~3m29s and completed.
Immediately after coder DONE, hub showed:

> hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)

Then `reviewer_correctness` spawned into an already-blocked loop (empty/failed)
and `reviewer_security` aborted mid-spawn (`flow_advance_aborted_mid_spawn`).

## Cause

BUG-289 F-0 residual after CA-361/run-333:

1. `hasActiveFlowChild` correctly re-arms F-0 while a child is still active.
2. `hubLastProgressAt` is only stamped on hub-side activity, not when a child
   terminal event is accepted.
3. In the gap between child DONE (no longer active) and the next spawn becoming
   active, age was already >2m → false `hub_stalled` mid auto-advance.
4. `parkFlowForAwaitingUser` then froze the flow while advance continued into
   the blocked loop.

Provider-agnostic: `hub_stall.go` has no `providerKey` branch. One representative
regression test covers Codex/Claude/Grok.

## Fix

- Add `touchParentHubProgressFromChildLocked` (root `flowEngineDriven` parents only).
- Stamp on:
  - `settleFlowChildTurnCompletedLocked` (before advance/reinvoke goroutines)
  - `notifyHubOfFlowChildFailureLocked`
  - cohort `EventTurnFailed` buffer path
  - `handleChildStartTurnFailure` (pre-adapter cohort/non-cohort start fail)
- Additive tests only: `run43831_hub_stall_auto_advance_test.go`
  - completion gap
  - non-cohort failure
  - cohort pre-flight failure (Codex review follow-up)

## Tests

```bash
go test ./internal/runner -run 'TestRun43831|TestRun333HubStallDoesNotCancelRunningChild|TestBug289_F0_HubStallBlocksWhenIdleTooLong|TestHubStallDoesNotFireWhenBlocked' -count=1
```

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-289
change_type: bugfix
summary: Stamp hub progress on child terminal handoff so F-0 does not false-stall mid auto-advance (run-43831)
# --->8---
