# CA-1201 — overlay mount no longer overwrites the parked flow's loop budget (live-039 residual)

## Evidence

Live run `run-183756`: `startResolvedFlowFromNode` seeds the shared loop's
`Cap`/`RoundCap`/`ExtendBy`/`NegotiationCap` (and `rs.stallTimeout`) from the
mounted flow's policy on every mount. When the owner-debate overlay mounted
over a parked `vibe-sprint` (cap=20), the debate's policy overwrote the
sprint's budget — the sprint's own round counter reported "round cap reached
5/5" against the debate leg's policy. `restoreVibeFlowAfterDebate` restores
topology/flowRef but never re-seeds the loop budget, so the corruption
outlived the debate.
Ledger: `loop-cap-overwrite|shared-loop|run-183756`.

## Root cause

The seed site is unconditional — it cannot tell a primary mount (the run's
own flow) from an overlay mount (a flow parked underneath). The debate's own
round budget lives in the mount/retry counters (`vibeOwnerFailRetries`,
`maxVibeDebateMountsPerSprint`, the wedge ladder), so the overlay never
needed to own the shared loop's cap at all.

## Fix

`flow_executor.go` `startResolvedFlowFromNode`: detect `overlayMount` =
`len(rs.vibeParkedNodes) > 0` under the existing locked read. When true, skip
seeding `stallTimeout` and the whole `Cap/RoundCap/ExtendBy/NegotiationCap`
mutateLoop write — the parked flow's budget persists for the overlay's
lifetime and remains correct after restore. BUG-631 extend-grant semantics
are untouched for primary mounts.

## Tests

`live039_loop_cap_overlay_test.go`:
- overlay mount (parked nodes + debate-shaped flow with cap=5/extendBy=1/
  negotiationCap=2) leaves the sprint loop at Cap/RoundCap=20, ExtendBy=3,
  NegotiationCap=7.
- primary mount still seeds its own policy (cap=3) — BUG-631 contract kept.

Regression: Bug631/594/595/624, CA1088, debate/mount/cap/flow-start
families — all green.
