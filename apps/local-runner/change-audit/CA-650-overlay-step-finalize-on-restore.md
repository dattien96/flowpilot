# CA-650 — Concluded debate overlay rows finalized on restore (live-039)

## Evidence
Live run-183756 ledger: `debate_trigger` parked `WAITING_USER_APPROVAL`
mid-debate; the debate concluded (`debate_synthesis` DONE) and
`restoreVibeFlowAfterDebate` reseeded the parked sprint topology — but
`mergeReseedSteps` intentionally preserves every prior row (BUG-562/BUG-586
execution-order merge), so the overlay's WAITING row survived untouched. Any
step-ledger reader (monitor waiting list, step-transition consumers) sees a
gate that is still parked forever; the row can never resolve because the
overlay topology is unmounted.

## Root cause
`restoreVibeFlowAfterDebate` swaps `activeFlowNodes` to the parked topology
and reseeds, but nothing owns a terminal transition for overlay rows left
non-terminal. There is no "overlay concluded" sweep — a WAITING/PENDING
overlay row is residue by definition at that point.

## Fix
`restoreVibeFlowAfterDebate` captures the overlay's node ids before the
topology swap and, after `reseedFlowStepRuntime`, calls new
`finalizeConcludedOverlaySteps`: for each overlay node id that does not
collide with the restored topology, is still non-terminal, and has no live
work (`vibeNodeHasLiveWork` — a leg still mid-turn owns its own settle),
stamp `SKIPPED` via `setFlowStepStatus` so the transition is persisted and
appended to the step-transition log. Terminal rows (DONE/FAILED/CANCELED/
SKIPPED) and ids reused by the restored topology are untouched.

## Tests
- `live039_overlay_step_finalize_test.go`: WAITING `debate_trigger` ->
  SKIPPED on restore; DONE overlay rows survive; RUNNING row with a live
  owner leg is not stamped.
- Debate/restore/reseed regression suite green (Debate|Restore|reseed|
  StepRuntime|BUG5xx/6xx|CA1088|Live039|vibe).
