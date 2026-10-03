# CA-1141 — BUG-594: non-debate flow starts clobber the mounted owner-debate overlay

- **Bug:** BUG-594 (live run-139670). A post-turn gate on a sprint coder leg
  diverted into `stashVibeFlowForDebate` at 00:21:57 while the ingest chain
  (`vibe-tasks`) was still live underneath — the park claimed the topology
  correctly. At 00:22:03 the already-spawned `task_plan_reader` leg completed,
  `onVibeCpNodeDone → maybeStartNextVibeSprint` fired `vibeSprintStartBlocked`,
  which only checked `vibeAwaitingLock` — the sprint mount swapped
  `activeFlowNodes` to the sprint graph and stomped the debate overlay that
  was mid-resolve. The owner-cohort join at 00:26:59 then resolved its hub on
  the sprint graph and stamped `synthesis` (a sprint node) instead of
  `debate_synthesis`; every later gate divert saw `vibeParkedNodes > 0`,
  suppressed, and the claim could never heal — `maybeResolveZombieVibeDebate`
  requires the debate graph to still be mounted. The durable record persisted
  exactly this shape (`vibe_parked_flow_ref=vibe-tasks`, `active=sprint`).
- **Fix:** the debate claim now fences every non-debate flow mount.
  `startResolvedFlowFromNode` calls `deferVibeFlowStartForDebate` first —
  while `vibeParkedNodes` is held, a non-debate, non-sprint `flowRef` queues
  onto `vibeDeferredFlowStarts` (durable via `ProviderSessionState` +
  `ndjsonSessionRecord`, restored in `reconstructRunInternal`, cleared for
  invalid pending-flow records) and drains in `restoreVibeFlowAfterDebate`
  after the parked topology is restored. Sprint starts are *refused* rather
  than deferred: `vibeSprintStartBlocked` and the `maybeStartNextVibeSprint`
  take both check `len(vibeParkedNodes) > 0`, so sprint producers keep their
  own rollback/re-fire semantics — a consumed take can never roll forward
  and re-fire into a second index after restore.
- **Stale-claim healing:** a parked snapshot recording a *different* flow
  than the live graph (`vibeParkedFlowRef` vs `chatFlowRef`) is detached
  corruption, never a restore source — `vibeDebateClaimForeignLocked`
  detects it, and `stashVibeFlowForDebate` + `maybeResolveZombieVibeDebate`
  drop it so diverts re-park the live topology and mount a real debate.
- **Mount-death release:** `stashVibeFlowForDebate` returns success before
  the debate goroutine resolves — a mount that dies in flight used to leave
  the claim suppressing diverts forever. The mount goroutine now post-checks
  via `maybeReleaseVibeDebateClaimIfMountDied`: a same-flow unmounted claim
  replays the idempotent restore (gated children still owe reprompts), a
  foreign claim is dropped. `vibeDebateMountInFlight` marks the resolve
  window so a mid-resolve claim is never released early.
- **Contract preserved:** a genuinely mounted debate keeps its claim (test:
  live overlay + parked claim is never released); the deferred queue is
  deduped and bounded (`maxVibeDeferredFlowStarts = 8`) — overflow drops with
  a diagnostic, never silently queues; non-vibe flows see no change (the
  defer helper early-returns when no claim is held).
- **Tests:** `bug594_debate_claim_fence_test.go` — red before fix:
  chain sprint start blocked under claim; foreign flow start defers;
  deferred starts drain on restore; foreign stale claim drops and a real
  debate re-parks; mount-death releases; live claim is not released.
- **Refs:** BUG-594. Live evidence: run-139670 (clobber + zombie claim),
  `PrivateVault/.flowpilot/chats/sessions.ndjson` persisted corrupted shape.
