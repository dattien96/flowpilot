# CA-1113 — BUG-565: verdict gate defers on in-flight member; Continue spawns a never-spawned member

## Why

Live run-69320 hit the verdict gate from two directions on the `synthesis`
hub:

1. 14:28 — the hub submitted `done` 23s after the reviewer leg spawned.
   No machine verdict existed yet, so the gate escalated → the park
   cancelled the reviewer mid-turn → the verdict could never arrive.
   (BUG-560's audit-side defer didn't cover the hub-done verdict gate.)
2. The parked Continue then looped: `resumeVerdictDeficientMembers` only
   re-drives children that EXIST — a member that was never dispatched
   (its spawn outcome had been consumed as `stale_hub_done` by the BUG-562
   wipe) left no child to re-drive, so Continue generically resumed the
   hub, which re-submitted `done`, which re-parked — forever, 16s apart.

## What changed

`apps/local-runner/internal/runner/review_done_verdict.go`:

- New `missingVerdictLabelsWithLiveMember(parentRunID, hubID)`: expected
  inbound-cohort labels whose machine verdict is missing AND whose member
  leg is still live — a non-terminal child run with that label, or the
  flow step row still RUNNING (hub ad-hoc spawns carry no `flow_cohort_id`,
  so the barrier alone misses them — BUG-561's same class).
- `resumeVerdictDeficientMembers`: deficient labels with no existing child
  are collected and SPAWNED via `spawnFlowDelegateLeg` instead of being
  silently skipped; the redrive diag now reports `spawned` labels.

`apps/local-runner/internal/runner/interactive_service.go`:

- `applyFlowControl` (synthesis done path) and `advanceHubDoneThroughEdge`
  (hub done path): when the verdict error is provisional — a missing
  verdict whose member is still in flight — both return
  `deferred_member_in_flight`, unstamp `lastFlowControlTurnID`, and keep
  the loop running instead of escalating. The member's settle/join
  re-invokes the hub, which re-submits done on fresh state. A verdict
  missing with NO live member still escalates fail-closed.

`apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:

- `spawnFlowDelegateLeg` extracted from `advanceToNextInlineOrDelegate`'s
  `agent.delegate` case — the same spawn shape (agent resolution, contract
  + templated inputs, posture stamp, RUNNING step) reused by verdict
  remediation.

## Invariant

A missing verdict with a live member is provisional — defer, never
escalate. A missing verdict with no member and a spawnable node spawns the
member — never loops the hub. A missing verdict with neither still fails
closed.

## Tests

`bug565_verdict_gate_inflight_test.go`:

- hub done + live reviewer child → defer, member keeps running, stamp
  cleared (re-entry possible)
- synthesis done + step RUNNING (adhoc member, no cohort child) → defer
- member never spawned and no live leg → escalate remains
- Continue on the never-spawned park spawns the reviewer leg
