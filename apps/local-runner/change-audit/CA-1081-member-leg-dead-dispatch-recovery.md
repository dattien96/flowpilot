# CA-1081 — member-leg dead dispatch detection + blocked surface (BUG-665)

## Scope
Wedge-sweep liveness (`deadDispatchNonTerminalChildExists`,
`childLegGhostLocked`), fresh-spawn suppression in
`redriveDeadDelegateLeg`, and `runSnapshot` status surfacing in
`interactive_handlers.go`. `vibeNodeHasLiveWork` claim semantics
explicitly preserved (BUG-1197).

## Defect
Live run-644825, leg `run-675483`: a coder leg completed its provider
turn and stayed `running` for ~2h20m with no further dispatch. Every
designed recovery lever was a silent no-op (`agent-loop/continue`,
`agent-loop/resume`, `feedback`, re-register); only a manual
`POST /turns` revived it. Two compounding holes:

1. **Detection hole.** `deadDispatchNonTerminalChildExists` counted any
   non-terminal leg as live work. A leg whose turn finished and which
   holds no queued intent — a *dispatch ghost* — suppressed the sweep's
   re-drive forever.
2. **Surface mask.** The run detail reported `status: running` while
   the loop was durably `blocked` awaiting a human gate — the fence
   that rejected `POST /turns` (`flow_awaiting_user`) was invisible in
   the snapshot.

## Semantic split (why the first attempt regressed)
`vibeNodeHasLiveWork` answers *"does a leg claim this node?"* for
orchestration/advance — a ghost leg still legitimately claims its node
(predecessor protection, BUG-1197). Making ghosts non-live there broke
`TestBug1197_AdvanceLeavesUnfinishedPredecessor`. Dispatch liveness is a
different question, now answered only inside the wedge sweep.

## Fix
- `hub_stall.go`: `childLegGhostLocked` — a Running child is a dispatch
  ghost when it has no in-flight turn (without a live post-turn gate),
  no queued intent (pendingTurnPrompt / pendingApprovalID /
  gate-reprompt / resume prompt / question gate), and no live gate
  cancel window. `waiting_*` parks never qualify — they own a
  user-visible resume surface. Armed intents count as work owed.
- `wedge_sweep.go`: `deadDispatchNonTerminalChildExists` skips ghost
  legs — the dead step re-drives via `reinvokeMatchingFlowChild`
  (in-place, preserves lineage) instead of being suppressed.
- `wedge_sweep.go`: `claimedNonTerminalChildExists` + spawn suppression
  in `redriveDeadDelegateLeg`. The dispatch-liveness scan sees `s.runs`;
  the reinvoke scan sees only the agent graph. A same-label
  non-terminal leg the graph cannot reach (e.g. a park the orphan-heal
  just un-stamped — the CA-1202 protected shape) still owns the node:
  suppress the fresh spawn rather than duplicate the claim.
- `wedge_sweep.go` (review round): a leg with armed
  `pendingFlowGateSettle` counts live in
  `deadDispatchNonTerminalChildExists` — it is parked-by-design work
  owed to the gate-settle machinery (deferred under a blocked parent
  per BUG-1177, re-driven by `resumePendingFlowGate` otherwise). Without
  the exemption the ghost check mis-classified it and the re-drive
  armed a bogus reprompt against the deferred-settle invariant. The
  exemption lives in the sweep, not in `childLegGhostLocked`, because
  BUG-354 deliberately counts settle-less-of-gate legs as non-shielding
  for the hub watchdog.
- `interactive_handlers.go`: `runSnapshot` surfaces `blocked` when the
  run is flow-engine-driven, the underlying loop is `blocked`, and the
  raw status is still live-looking (running/idle/starting). Explicit
  terminal and waiting statuses are preserved.
- `interactive_handlers.go` (review round): `agent-loop/continue` on a
  ghost member leg now redrives it through `reinvokeMatchingFlowChild`
  via `ghostMemberLegParent` — the live lever that silently no-oped.
  Settle-armed legs are exempt (BUG-1177), matching the sweep.
  `member_stalled`/`memberAction` classification remains cohort-scoped
  — the sweep + this lever cover recovery; expanding the member-action
  surface to non-cohort legs is out of scope.

## Provider neutrality
The fix is in shared orchestration/sweep code — no provider adapter is
involved. Ghost-ness is defined purely by run dispatch state.

## Tests
- `bug665_dead_leg_dispatch_test.go` (new): ghost leg not counted live;
  running/waiting/in-flight legs still counted; settle-armed legs count
  live; terminal legs not counted; ghost defined by dispatch state not
  event age; blocked loop surfaces `blocked`; running loop not masked;
  terminal status not masked; handler-level: continue on a ghost leg
  redrives (activationSeq bump), continue on a settle-armed leg does
  not.
- Regression caught and fixed during implementation: the ghost check
  initially lived in `vibeNodeHasLiveWork` → `TestBug1197` failed →
  moved to sweep-scoped detection; CA-1202's
  `WaitingUserLegIsNotDeadDispatch` then exposed the graph-vs-`s.runs`
  scan divergence → `claimedNonTerminalChildExists` suppression added.
- Green: all BUG-665 tests, CA-1202 suite, BUG-1197 suite, and the
  BUG-520/539/641/643/653/660 stall/wedge surface.
