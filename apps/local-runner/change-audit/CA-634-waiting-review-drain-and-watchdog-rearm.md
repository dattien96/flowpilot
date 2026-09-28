# CA-634 — Waiting-Review Drain + Watchdog Re-arm (BUG-542)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-542

---

## Problem

Live `run-18354` (tournament parent): a cohort join arrived while the hub
turn was in flight. `maybeAutoReinvokeHubWithNote` logged
`hub_reinvoke_deferred` but only armed `pendingHubReinvoke` when
`pendingAgentContext` was non-empty — the join rode the provider tool
channel (`cohort_note_len=0`), so nothing armed. The hub answered in prose,
never emitted the deterministic flow signal, and the loop stayed
`waiting_review` with candidate-b never spawned.

Second defect: `checkAndBlockStalledHub` returned early when a live
approval/question card existed WITHOUT re-arming the watchdog. When the
card resolved, no future watchdog invocation existed — the flow wedged
silently for 45+ minutes (`hub_stalled` never surfaced).

## Fix

Drain-side recovery, not arm-site (the `pendingAgentContext` gate is a
deliberate contract — see `TestAutoReinvokeHubNoPendingContextNoDefer` /
`SingleFlightConcurrent` — spurious duplicate calls must not queue work):

### `internal/runner/interactive_resume.go`

- `waitingReviewDrainDueLocked` — an idle flow-driven hub whose loop is
  `waiting_review` owes one bounded reinvoke; the join obligation exists
  even when no note/context survived.
- `waitingReviewReinvokeDrainCap` bounds the drain: a hub that keeps
  answering in prose exhausts the budget and the stall watchdog surfaces
  `hub_stalled` instead of burning turns forever.
- The budget resets when the loop leaves `waiting_review`.
- `notifyTurnIdle` dispatches the owed reinvoke when no other durable
  intent consumed the idle transition.

### `internal/runner/hub_stall.go`

- `checkAndBlockStalledHub` re-arms the watchdog before returning on a
  live approval/question card, so the check survives card resolution.

## Tests

`bug542_hub_reinvoke_deferred_wedge_test.go` — idle waiting_review owes a
reinvoke, drain is bounded, bound resets on loop advance, `notifyTurnIdle`
dispatches, watchdog re-arms under live approval and question cards, and
still surfaces `hub_stalled` after the card resolves. Legacy arm-site
contracts unchanged and green.
