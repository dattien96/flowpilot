# CA-1238 — run-752742 relay-leg & dispatch defect fix batch: BUG-671..675 — invisible topology on relay legs, undrained reinvoke intent, zombie turn_in_progress, phantom-loop spawn veto, dead-mount cap burn

## Summary

Continuation of the run-752742 defect batch (CA-1237 covered BUG-666..670).
Five more live-observed defects fixed reproduce-first (red test → smallest
root-cause fix → green), zero old-test edits. All fixes live on
provider-agnostic shared paths — Claude/Codex/Grok/Devin parity holds by
construction.

| BUG | Defect | Fix |
|-----|--------|-----|
| 671 | Relay/re-attach legs inherit `flowEngineDriven` but mount no `activeFlowNodes` — every topology read (`flowNodeForRunLocked`, `flowNodePostureFor`, `flowHasVerdictOnlyNode`, `flowRequiresSynthesisMachineVerdict`, `flowRequiresHubMachineVerdict`, verdict maps) resolved on the relay's empty topology, so `TurnRequest.OfferReviewOutcomeTool` stayed false and verdict/cohort gates were invisible (live: Devin legs `run-816045`/`run-816751`/`run-817740`/`run-826566+826571` and Grok `run-815559` all lacked `submit_review_outcome` — extends BUG-628 beyond Grok) | `flowTopologySourceLocked` resolves the launcher (topology-owning) leg for relay legs — same discriminator as BUG-669's `legIsFlowRelay` — and every topology/verdict-map read routes through it. Verdict RECORDS also resolve to the owner leg so writes and reads agree. |
| 672 | `pendingHubReinvoke` armed + `hubReinvokeStartFailCount > 3` was un-drainable: the watchdog refuses >3, and `resumeFlowWithFeedback`'s non-blocked Continue path never consumed the armed intent → permanent `hub_stalled` (live: reinvoke armed for hours post-unblock) | Continue now consumes the armed pending intent in both branches — preserves its prompt, resets the fail counter, dispatches `maybeAutoReinvokeHub{,WithPrompt}` — while the automatic watchdog budget stays bounded. |
| 673 | A dispatch dying between `prepared` and provider acceptance left `turnInFlight` pinned forever — every new turn/relay rejected `turn_in_progress` until operator `interrupt` (live: `turn-816726`, ~5h) | `sweepWedgedFlowWork` collects `turnInFlight && currentTurnID` runs with no live gate; `maybeReleaseZombieTurn` releases only with durable proof (record terminal, pre-acceptance stale ≥10m, or absent on a V2-active run past bound). `provider_accepted`/`uncertain` and fresh records keep the flag — concurrent-send protection unchanged. Release persists + `notifyTurnIdle` drains the queue. |
| 674 | `spawn_agent` admission read `loopStateFor(parentRunID)` on the addressed (relay) leg — its phantom/stopped loop answered `loop is stopped — no further spawns` while the launcher's loop was running (live: relay `run-3` refused the spec_align/reviewer cohort spawn) | Spawn guards resolve `flowOwnerLoopStateForLocked(parentRun, parentRunID)` (BUG-669 resolver) for blocked/sealed checks, diagnostics, and error text. Root/launcher runs unaffected. |
| 675 | `stashVibeFlowForDebate` ledgered `vibeDebateMounts` + `vibeDebateMountsByEntity` before the overlay materialized; `maybeReleaseVibeDebateClaimIfMountDied` released dead mounts without refunding — 3 dead mounts hit `maxVibeDebateMountsPerSprint`, `escalateVibeDebateMountCap` parked the sprint and `parkFlowForAwaitingUser` froze/killed the live cohort | The mount-died release refunds total + the entity charged first at stash (`vibeParkedGatedRunIDs[0]` via `vibeDebateEntityKeyLocked`, `"hub"` fallback), floored at 0. A claim whose overlay actually mounted keeps its budget. |

## Red→Green Evidence

- `bug671_relay_leg_tool_offer_test.go` — red: `OfferReviewOutcomeTool=false`
  on a delegate child under a relay parent and on the relay hub itself
  (fixture isolates quota/session env). Green post-fix.
- `bug672_reinvoke_intent_drain_test.go` — red: `timed out waiting for
  reinvoke turn dispatched`; green post-fix.
- `bug673_zombie_turn_test.go` — 4 cases; verified red by stash
  (`turnInFlight still pinned` ×2), green restored.
- `bug674_spawn_relay_owner_loop_test.go` — verified red by stash
  (`loop is stopped` refusal), green restored.
- `bug675_mount_cap_test.go` — red: `dead mount kept the budget: mounts=1
  entity=1`; green post-fix. Second case pins the boundary: a claim whose
  debate overlay actually mounted keeps its budget.

## Regression / Baseline

- Focused sweep of touched areas (`TestBug*|TestCA1203|TestCohort|
  TestVibeDebate|TestStall|TestSweep|TestMemberAction|TestPark|TestSpawn|
  TestReinvoke|TestRelay|TestLeg`, 265s): failures are the known
  environmental class — `quota_route_blocked`/`pinned account is unusable`,
  missing provider binaries (`claude` not in PATH), TempDir/gitnexus-repo
  env issues.
- Differential proof on the sensitive set (`TestBug521|TestBug561|
  TestReinvokeLifecycle|TestBug504`) — run on a clean stash: identical
  failures and identical timings (quota/env), zero new regressions from
  this batch.
- Provider parity: all changes are on shared topology-resolution /
  orchestration paths; `OfferReviewOutcomeTool` remains a single shared
  boolean consumed identically by every adapter (claude/codex/devin/grok/
  opencode). No provider-specific tool semantics added.

## GitNexus

- `node .gitnexus/run.cjs detect-changes --scope all --repo .` — **complete**
  (no partial/truncated): 7 files, 4 changed symbols
  (`flowNodeForRunLocked`, `maybeReleaseVibeDebateClaimIfMountDied`,
  `vibeOwnerDebateGraph`, `wedgedWaitStaleFor`), **0 affected processes**,
  risk **low**. Smaller blast radius than CA-1237 because the new seams are
  additive helpers + a new sweep class; existing call flow unchanged.

## Files

- `internal/runner/gate_hook.go`, `node_isolation.go`,
  `interactive_service.go`, `review_done_verdict.go` — BUG-671 topology
  owner resolution; BUG-672 drain; BUG-674 spawn guard.
- `internal/runner/wedge_sweep.go` — BUG-673 zombie-turn sweep.
- `internal/runner/vibe_debate.go` — BUG-675 mount refund.
- `internal/runner/chat_switch.go` — (batch context / BUG-668 follow-on
  adjustments if any; see diff).
- New tests + BUG docs BUG-671..675 (all `FIXED — CA-1238`).

## Known Environmental Failures (unchanged baseline)

`quota_route_blocked`, missing provider binaries, seed/session setup,
TempDir cleanup, gitnexus repo-label — all reproduced identically on the
clean-tree stash.
