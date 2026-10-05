# CA-1203 — Wedge sweep re-drives dead-dispatched PENDING nodes; verdict gate defers on pending upstream work (live run-183756)

## Evidence
run-183756 ledger fingerprint `hub-early-done-advance|synthesis`: after the
owner-debate overlay consumed tdd's completion, coder stayed PENDING
forever — the activation edge had fired (or been consumed by the divert)
but no child leg and no dispatch ever materialized. When the synthesis hub
was reinvoked it self-graded done; the verdict gate correctly found
spec_align/reviewer machine verdicts missing, escalated, and parked the
loop WAITING_USER_APPROVAL — a dead end, because the verdicts can only be
produced by work that was never dispatched: reviewer needs validate needs
coder.

## Root cause
Two seams each handled half the shape:

- `maybeRedriveDeadDispatchedSteps` (CA-1202) only re-drove RUNNING-stamped
  steps. A node whose completion edge was consumed before its RUNNING stamp
  landed (debate divert / overlay restore) sits PENDING with all-terminal
  predecessors — the same dead end, minus the stamp.
- `advanceHubDoneThroughEdge` / the `viaReviewOutcome` missing-verdict gate
  escalated to a park as soon as no live member and no auto-redrive target
  remained. Escalation is only a real dead end when nothing upstream can
  still produce the verdict — undispatched-but-dispatchable pending work
  can, and parking first guarantees it never does.

## Fix
- `pendingDispatchableFlowNodes(parentRunID, edges, nodes)` — PENDING node +
  every *direct* forward-done predecessor terminal (DONE/SKIPPED) + no
  non-terminal child leg + self-drivable kind (inline-dispatchable or
  provider-backed delegate). `artifact.audit_draft` is excluded: the BUG-572
  deferred-audit class owns it (it must additionally wait out unrelated
  RUNNING sprint steps), and it can never produce a missing verdict.
- `maybeRedriveDeadDispatchedSteps` gains a PENDING class: on a *silent* run
  (lastProviderEventAt aged past `flowStepDeadDispatchBound`, or zero traffic
  with all run-level drivers quiet — the dispatch window stays owned by the
  predecessor's settle goroutine), with no open cohort, dispatch goes through
  the same seams a fresh advance uses — inline handlers via
  `tryAdvanceFlowThroughInline`, hub reinvoke for `hub.inline`, and
  `redriveDeadDelegateLeg` for delegate nodes. `redriveDeadDelegateLeg` now
  stamps RUNNING after a successful reinvoke/spawn (idempotent for the
  RUNNING class; refreshes StartedAt so the wedge-age clock restarts).
- Verdict gate (`advanceHubDoneThroughEdge` and the `viaReviewOutcome`
  branch in `applyFlowControl`): defer as `deferred_member_in_flight` when
  `pendingDispatchableFlowNodes` is non-empty — same unstamp-and-keep-loop-
  running shape as the live-member defer; the chain drives itself to the
  verdict instead of parking on one nobody can submit.

## Guards preserved
- Entry-shape nodes (no direct done-edge preds) are never sweep-dispatched.
- Recent provider activity keeps the dispatch window to the live path.
- Open cohort, non-terminal child, or persisted Completed leg → skip.
- `artifact.audit_draft` defer semantics unchanged (BUG-572 tests green).
- Missing verdict with nothing dispatchable still escalates — the old
  contract holds (regression test).

## Tests
`ca1203_pending_dispatch_sweep_test.go` — pending redispatch on silent run,
live-pred guard, dispatch-window guard, entry-node guard, hub-done defer on
dispatchable upstream, escalate preserved when nothing dispatchable.
