# CA-1156 — BUG-624: settle ladder raced the debate mount window → duplicate owner legs

Live trigger: run-174243 (Task-033 sprint). At 23:08:19 the gate-resolver
mounted `vibe-owner-debate`: `stashVibeFlowForDebate` parked the sprint,
`vibeDebateMountInFlight=true` was set, `debate_trigger` stamped RUNNING,
and the first owner spawn request went out. In that same second — before
any owner child registered — a settle-path goroutine
(`go s.maybeSettleVibeOwnerDebate`) evaluated the board:

- `vibeOwnerDebateGraph(activeFlowNodes)` already true (graph swapped),
- `ownerChildren == 0` (spawn requests still dispatching),
- `LoadRunSteps` had not yet surfaced the `debate_trigger` RUNNING stamp.

That shape satisfied the CA-1088 `ownersStarved` predicate, so the settle
ladder fired `vibe_owner_fail_retry` → a **second** `startResolvedFlow` on
the already-mounted debate graph → a second owner_1 + owner_2 pair
spawned into the same cohort (`flow-auto-debate_trigger-round-0`): 4
owner legs where 2 belong. `cohort_expected_inferred` later reconciled
expected=4 from live siblings, so the join settled anyway — cost was two
wasted provider turns plus cohort arithmetic noise, not a wedge.

Root cause: `maybeSettleVibeOwnerDebate` never consulted
`rs.vibeDebateMountInFlight`. CA-1088's `stTrig != Running` guard covers
the post-stamp dispatch window; the pre-stamp window (graph swapped,
flag set, no stamps yet) had no guard at all.

## Fix

`maybeSettleVibeOwnerDebate` returns early when
`rs.vibeDebateMountInFlight` — the mount goroutine owns the window, and
its post-check (`maybeReleaseVibeDebateClaimIfMountDied`) is the only
legitimate verdict for a mount that dies inside it. The flag is set under
`s.mu` before the mount goroutine starts and cleared after
`startResolvedFlow` returns + claim post-check, so the check reads a
consistent epoch.

The settle-retry's own `startResolvedFlow` (vibe_debate.go) does not set
the flag — it runs sequentially inside the same settle goroutine, so it
cannot race itself; every path that *could* race is flag-covered.

## Tests

`bug624_debate_settle_races_mount_test.go` (additive):
- `TestBUG624_SettleDoesNotRetryWhileDebateMountInFlight` — starved-shape
  board + flag set → no retry, `vibeOwnerFailRetries` stays 0.
- `TestBUG624_SettleRetriesAfterMountWindowCloses` — same board, flag
  clear → CA-1088 ownersStarved retry still fires.
