# CA-1237 — run-752742 chat-leg lifecycle defect fix batch: 5 bugs (BUG-666..670) — legless chat surface, closed-leg reinvoke budget, stale-leg relay, phantom relay loops, expired-gate resurrection

## Summary

Live run `run-752742` (PrivateVault vibe-sprint, chat `cht_3078718bf299`)
exercised the chat-leg lifecycle hard: provider-switch mint failures, leg
rotations under quota, reattach legs, and 4h+ expired gate cards. Five
defects were captured as BUG-666..670, each fixed reproduce-first (red test
→ smallest fix → green), no old tests touched. All five BUG docs flipped to
`Status: FIXED — CA-1237`.

| BUG | Defect | Fix |
|-----|--------|-----|
| 666 | Switch closed the source leg, successor mint failed → chat left with ALL legs closed, every turn failing `leg_closed` forever with no surfaced state | `healChatLegsLocked` now emits a one-time typed `leg_recovery_required` transcript record naming the newest closed leg as the explicit reattach point (`closedRunId`, `legSeq`, `closedReason`, `reattachHint`, `fromProvider`, `fromModel`). Heal surfaces honestly — it never auto-mints (routing stays a user decision). `EventTypeChatLegRecoveryRequired` added to `chat_ssot.go`. |
| 667 | `leg_closed` from `scheduleChildTurn`'s reinvoke burned `hubReinvokeStartFailCount` — 3 retries during a leg rotation exhausted the budget and stranded the armed pending in `hub_stalled` forever | `leg_closed` joins the transient-busy class (`hub_parked`, `flow_awaiting_user`): the relay target can only exist after a later tick, so the pending stays armed without consuming the budget. |
| 668 | Closed-leg relay (both sites: `leg_closed` relay + quota-rotation relay) picked ANY `LegStateActive` sibling under Go map order — landed on completed/parked stale legs at random (live: 4 stale-active legs, coin-flip into dead legs) | New `selectRelayLegLocked`: among `LegStateActive` siblings excluding the source, skip terminal-status legs and pick the NEWEST by `legSeq` — deterministic, and completed/cancelled legs that kept `legState=active` after partial heals can no longer swallow turns. |
| 669 | Reattach legs inherit `flowArm=started` + `flowEngineDriven` (CP-89 launch-dedup latch) but never mount `activeFlowNodes` — every `parentRunID=="" && flowEngineDriven` check treated them as flow hubs: a zero-value loop materialized on the leg, the stall watchdog parked it, and admission rejected every relayed turn with `flow_awaiting_user` | `legIsFlowRelay` discriminator (`chatID != "" && parentRunID == "" && flowEngineDriven && len(activeFlowNodes) == 0`). `flowLoopOwnerForLocked` resolves the chat's newest leg that actually mounted topology. Root-turn admission consults `flowAdmissionLoopStatusLocked` (launcher leg's loop for relay legs, own loop otherwise). `checkAndBlockStalledHub` redirects relay-leg stall checks to the launcher leg's loop and refuses to park a loop on a leg with no launcher resident. |
| 670 | `expireQuestion`/`expireApproval` cleared the pending pointer + flipped `waiting_question→running` in memory only — never persisted the run row; a restart resurrected the durable wait on a dead card. Separately, `healOrphanedWait` bailed when `pendingQuestionID != ""` without verifying the record was live — dangling gate ids wedged the sweep forever | Both expiries now persist the owning `ProviderSessionState` after clearing (parity with `clearPendingQuestion`/`clearPendingApproval`). `healOrphanedWait` calls `clearDanglingGateIDsLocked` BEFORE the live-card check — dangling ids are cleared and persisted, then the wait heals through the normal path. |

## Verification

- New red→green tests (all additive, zero old-test edits):
  - `bug666_zero_active_leg_heal_test.go` — all-closed chat emits exactly one
    `leg_recovery_required` record naming the newest closed leg; heal
    re-entry is idempotent.
  - `bug667_leg_closed_reinvoke_transient_test.go` — `leg_closed` reinvoke
    failures keep the pending armed without burning the fail budget
    (verified red: budget hit 4 pre-fix).
  - `bug668_relay_leg_selection_test.go` — relay deterministically picks the
    newest non-terminal active leg; terminal-but-stale-active legs are
    skipped.
  - `bug669_relay_leg_phantom_loop_test.go` — relay leg is not treated as a
    loop owner; stall watch redirects to the launcher leg; admission
    consults the real loop.
  - `bug670_expired_question_pending_pointer_test.go` — question expiry
    persists the cleared run row; approval expiry gets the same persistence
    (parity); dangling gate ids heal through the wedge sweep.
- Full `go test ./internal/runner/` (600s): remaining failures are the
  pre-existing baseline set (quota-route env: `quota_route_blocked`/
  `quota_route_required`/`switch_seed_failed`/`handoff_run_busy` classes —
  `TestChatFollowUp*`, `TestChatMode*`, `TestSwitchChainThreeProvidersMultiLeg`,
  `TestSwitchSeedPromptReachesTargetAdapterAsFirstTurn`, `TestSpawnChildRun*`,
  `TestBug1184/1185`, durable-persist fakes, `TestFlowFollowUp*` — ALL
  confirmed red on a clean stash of this change, i.e. none are regressions
  introduced here).
- Provider parity: all five fixes live in provider-agnostic shared paths
  (leg selection, heal, expiry persistence, loop-ownership resolution,
  reinvoke classification). BUG-668's test exercises multi-provider leg
  sets; no provider branch was touched.

## Prior CA claims cross-checked

- CA-1236 (BUG-653/654/657) reinvoke/park drain semantics preserved —
  `leg_closed` is additive to the same transient class, budget behavior for
  hard errors unchanged.
- CA-1090/CP-89 reattach latch (`flowArm=started` inherit) preserved —
  BUG-669 adds a discriminator, it does not remove the latch or change
  launch dedup.
- SD26-S-2/E-9 heal windows in `healChatLegsLocked` untouched — BUG-666
  adds a new non-overlapping case (all legs closed) gated on
  `newLeg == nil && oldLeg == nil`.
