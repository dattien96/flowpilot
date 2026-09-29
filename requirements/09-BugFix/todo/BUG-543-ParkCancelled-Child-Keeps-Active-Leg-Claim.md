# BUG-543 — park/stop-cancelled and completed flow children keep `leg_state=active` (residue, assessed)

## Status
ASSESSED — not a fixable narrow bug; pinned the contract with additive tests
(`bug543_stopped_flow_child_leg_test.go`). Residual claim-release happens only
through the explicit terminal seams listed below.

## Live captures
- `run-18660` — `completed` + `leg_state=active` (fp-live5). Symptom of the
  BUG-542 wedge: the parent parked `waiting_review` forever, so
  `reconcileChildRunsOnFlowDone` never ran.
- `run-23156`, `run-28790` — `cancelled` + `leg_state=active`
  (park-cancelled in-flight turns on escalate cycles).

## Assessment (why no close-on-cancel/stop fix)
The leg claim is the child's re-drivable provider pin. Three legal paths can
re-drive a `cancelled`/`waiting`/`completed` child on the SAME leg:

1. `reinvokeMatchingFlowChild` — label-matched re-drive (review-loop rounds,
   tournament retry edges) sets `status=running` and schedules a turn; a
   closed leg would make `startTurn` fail `leg_closed`.
2. `resumeAgentLoop` revives a `stopped` loop unconditionally —
   `resumePendingLoopWork` flushes children carrying durable intents, and
   cap-stops (`advanceRound` cap → `stopped`) are designed to be extended
   (`extend-cap`) and resumed.
3. Orphan-cure revives `waiting_user_approval` children with armed intents.

Closing legs at park/stop therefore breaks resume-after-stop and review-loop
reuse — the reverted broad close proved it (~56 suite failures).

Terminal ownership already transfers through the four explicit seams that DO
know the member is done for good:
- `reconcileChildRunsOnFlowDone` — `LegClosedReasonFlowDone` (flow `done`).
- `member_action` skip — `LegClosedReasonMemberSkipped` (BUG-539).
- dispatch failure — `LegClosedReasonDispatchFailed`.
- candidate sweep — `LegClosedReasonWorktreeSwept` (BUG-535).

A cancelled/idle child outside those seams keeps `leg_state=active` by
contract; its worktree claim is likewise held until a terminal seam runs.

## Tests (additive, pin current contract)
- `TestBug543_StopKeepsChildLegClaim` — Stop terminalizes children to
  `cancelled` but must NOT strip leg claims (resume needs them).
- `TestBug543_FlowDoneReconcileClosesLegs` — the flow-done reconcile is the
  generic close seam: `completed` + `flow_done`.

## Residual — RESOLVED by sweep (CA-1060)
Orphaned cancelled children whose parent is sealed `stopped`/`done` and never
resumed are now reclaimed by `sweepStaleLegClaims` (boot via `AttachRunner` +
in-session `StartLegClaimSweep`, 10 min cadence). The conservative predicate
closes a claim only when the child row is terminal, older than
`legClaimReclaimMinAge` (24h), carries no armed intents or pending cards, has
no merge in flight, is not remote-origin, and the parent loop is provably
sealed (stopped/done), terminal-loopless, or deleted. Each reclaim persists
`leg_closed_reason=claim_reclaimed` + a durable `leg_claim_reclaimed` flow
sidecar event. Reader errors fail closed to keep.

## Related
BUG-235/538 (done-reconcile), BUG-539 (member skip close), BUG-535
(sweep close), BUG-542 (the wedge that produced run-18660's residue).
