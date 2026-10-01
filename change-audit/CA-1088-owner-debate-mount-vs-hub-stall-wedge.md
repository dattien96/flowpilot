# CA-1088: owner-debate mount raced hub_stalled — owner cohort spawn refused, dead debate churned escalate/re-park loop

Date: 2026-10-01
Refs: user-reported live production hang — vibe-tasks run-3362 spinning
hub reinvokes (~190% runner CPU) on `vibe-owner-debate` with
`debate_trigger` repeatedly re-parked `WAITING_USER_APPROVAL`,
`owner_1`/`owner_2` permanently PENDING.

## Symptom (live run-3362, 18:03–18:18)

- 18:03:04 — sprint `coder` child (run-12553) spawned, RUNNING.
- 18:06:03.5 — the coder's post-turn gate routed to the owner-debate
  remediation (`flow_start_begin vibe-owner-debate`).
- 18:06:04.4 — the hub stall watchdog's armed tick landed **mid-mount**:
  the coder's turn had just ended (child read as ghost) and the debate's
  owner children did not exist yet → `hasActiveFlowChild` false →
  `hub_stalled` blocked the shared loop.
- 18:06:04.6–.7 — `debate_trigger`'s done-edge dispatched `owner_1` /
  `owner_2` → both refused: `child_spawn_refused_blocked_loop` →
  `flow_start_no_entry` → dead debate on a blocked loop.
- 18:06:45 → 18:07:44 — stale `activeHubNodeID="synthesis"` reinvokes kept
  re-running the sprint synthesizer against the debate topology; the model
  escalated "joined result note missing" (there were no owner outputs to
  join) → each escalate re-parked on `debate_trigger` → repeat.
- Operator `POST /gate-decision` (custom feedback) at 18:16 discharged the
  debate; the sprint restored and continued to sprint 5/5.

## Root cause — two stacked defects

1. **The stall watchdog blocks a loop that is hosting a mounted debate.**
   `checkAndBlockStalledHub` has no notion of "debate mounting/active":
   `vibeParkedNodes` (set synchronously by `stashVibeFlowForDebate` before
   `startResolvedFlow` is even spawned) and the debate topology itself were
   ignored. The mid-mount silence window (gated child's turn just ended,
   owner children not yet created) reads as "no progress" → hub_stalled →
   the owner cohort spawn is refused (BUG-432 guard) → the remediation
   flow can never make progress.

2. **`stashVibeFlowForDebate` leaves a stale hub pointer.**
   `rs.activeHubNodeID` still pointed at the sprint's `synthesis` hub after
   the topology swap — post-mount reinvokes resolved onto a node that is
   not in the debate graph, stamping RUNNING transitions and driving the
   escalate loop above.

   Secondary gap: `maybeSettleVibeOwnerDebate` only recognized "both owner
   steps FAILED" — the spawn-refused shape leaves them PENDING with zero
   owner children, so the settle never retried the starved mount.

## Fix (`internal/runner` only)

- `hub_stall.go` `checkAndBlockStalledHub`: while a debate is mounted
  (`vibeParkedNodes` non-empty — set synchronously at mount — or the
  active graph is the debate graph), re-arm the watchdog instead of
  blocking. The debate's own settle/cap ladder bounds a genuinely wedged
  debate; the shared loop must stay spawnable for the cohort.
- `vibe_debate.go` `maybeSettleVibeOwnerDebate`: treat the starved-mount
  shape — zero owner children + both owner steps neither RUNNING nor DONE
  — as the same wedge class as both-owners-failed and retry through the
  existing retry/cap ladder. `debate_trigger` RUNNING still skips (a mount
  actively dispatching must never be double-restarted), and unseeded
  mounts (`stTrig == ""`) are not "starved".
- `vibe_cp.go` `stashVibeFlowForDebate`: clear `rs.activeHubNodeID` when
  parking the flow — post-mount reinvokes re-resolve via
  `hubInlineNodeID` on the debate graph (`debate_trigger`); after restore
  the same fallback re-resolves `synthesis` on the sprint graph.

## Regression tests

`ca1088_debate_mount_stall_test.go` (all red before the fix — the first
reproduced the exact `hub_stalled` gate reason from the live log):

- `TestCA1088_StallWatchdogNeverBlocksMountedDebate` — starved-mount shape
  must not be converted to hub_stalled.
- `TestCA1088_StallWatchdogSkipsInFlightDebateMount` — mid-dispatch mount
  (`debate_trigger` RUNNING) must not be blocked or double-restarted.
- `TestCA1088_StarvedDebateRetriesNotStalls` — owners-PENDING + zero
  children + loop blocked hub_stalled → settle retries the debate and
  clears the block.
- `TestCA1088_StashClearsStaleHubPointer` — `activeHubNodeID` cleared on
  stash.
- `TestCA1088_SettleSkipsInFlightDebateMount` — settle must not restart a
  mount still dispatching.

## Verification

- `go test -count=1 -run 'TestCA1088' ./internal/runner` — red → green.
- Focused sweep (`TestCA796|CA1073|CA1087|HubStall|Stall|OwnerDebate|
  VibeOwner|StashVibe|Debate`) — green; CA-796 owner-fail retry/cap and
  harness-still-stalls contracts unchanged.
- Live: operator gate-decision unblocked run-3362; it then completed
  sprint 4/5 → boundary → started sprint 5/5 (`preflight_contract_plan`
  → freeze → context → tdd). Runner binary must be rebuilt for the fix
  to protect future mounts.
