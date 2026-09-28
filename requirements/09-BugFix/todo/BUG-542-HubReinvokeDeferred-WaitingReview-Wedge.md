# BUG-542 — Deferred hub reinvoke stranded; watchdog never fires `hub_stalled`

**Status:** FIXED — drain-side recovery + watchdog re-arm (CA-634); live re-verify pending
**Found:** 2026-09-28 live campaign (runner `/private/tmp/fp-runner`, :4322 workspace `/tmp/fp-live5`)
**Severity:** high — a flow root parks forever at `waiting_review` with no escalation surface

## Reproduction (live)

1. Launch a tournament flow on a chat run: `POST /client/workflow-runs` then turn
   with `workflowId` resolving to the tournament flow (run-18354, provider devin).
2. Child `run-18660` (candidate-a / coder) completes at 05:41:38; its completion
   signals the parent while the hub provider turn (`righteous-mollusk`) is still
   in flight.
3. 05:41:39 `hub_reinvoke_deferred` (`loop_status=waiting_review`,
   `cohort_note_len=0`, `auto_orchestrate=false` on the child spawn).
4. Hub turn settles fully at 05:42:37 (`turn-18356`: gate_eval →
   completion_event → graph_signal → dependents_release → finalizer).
5. **Nothing re-drives the hub.** Loop stays `waiting_review`, run stays
   `running`, provider session dead. 45+ minutes silent — no `hub_stalled`, no
   reinvoke, no card. `POST /agent-loop/continue` returns the graph unchanged
   (no-op for `waiting_review`).

## Analysis

- `maybeAutoReinvokeHubWithNote` defer branch (interactive_service.go ~3660):
  `pendingHubReinvoke` is armed **only if** `parent.autoOrchestrate &&
  parent.turnInFlight && !parent.reinvokeInFlight &&
  len(parent.pendingAgentContext) > 0`. The cohort note rode the provider's own
  `spawn_agent` tool channel (cohort_note_len=0), so `pendingAgentContext` was
  empty → flag **not armed**. `rearmed=false` then schedules
  `maybeScheduleHubStallCheck` (2 min) as the fallback.
- The watchdog `checkAndBlockStalledHub` (hub_stall.go) never surfaced a card
  across 45 min. Masking suspects, in order observed:
  - `pendingApprovalID` held busy while `appr-18519` was pending (correct).
  - After the approval resolved, the re-arm chain should have fired ~2 min
    later — it did not. Either a busy residue persisted (unverifiable from
    outside) or the in-memory `hubStallTimers` chain was dropped.
  - A subsequent plain chat turn (turn-19922, `ping`) was accepted, completed
    and settled — clearing `reinvokeInFlight`/`turnInFlight` — and the loop is
    **still** `waiting_review` with no escalation.
- `pendingHubReinvoke`/`hubLastProgressAt` are **RAM-only** (absent from
  sessionStateOf), so neither survives restart nor appears in durable rows —
  the wedge is invisible to operators.

## Impact

Any deferred-during-turn hub reinvoke whose join note arrives via the provider
tool channel (not `pendingAgentContext`) can strand permanently: no drain at
settle (flag never armed), and the watchdog fallback either never armed or is
masked by stale busy residue. The flow looks `running` forever.

## Fix direction (for implementer)

- Make `pendingHubReinvoke` durable (sessionStateOf + rehydrate), or re-derive
  it from loop `waiting_review` + un-consumed cohort join.
- `onSettleFinalized`/`notifyTurnIdle` should drain a deferred reinvoke even
  when `pendingAgentContext` is empty — the defer condition and the drain
  condition must use the same predicate.
- Audit `checkAndBlockStalledHub` masking: verify `reinvokeInFlight`,
  `pendingApprovalID`, `gateCancelLive` residue cannot re-arm the watchdog
  indefinitely without ever blocking.
- Durable evidence to assert in tests: loop `waiting_review` + dead provider
  session + last event `hub_reinvoke_deferred` → `hub_stalled` block within
  timeout, or a real hub turn dispatch.

## Fix (2026-09-28, CA-634)

Drain-side recovery rather than arm-site (the `pendingAgentContext` gate is a
deliberate anti-spurious-duplicate contract — `TestAutoReinvokeHubNoPendingContextNoDefer`):

- `waitingReviewDrainDueLocked` (interactive_resume.go): an idle
  flow-driven hub whose loop is `waiting_review` owes one bounded reinvoke —
  the join obligation exists even when no note/context survived.
  `waitingReviewReinvokeDrainCap=2` bounds it; a prose-only hub exhausts the
  budget and the stall watchdog surfaces `hub_stalled` instead of burning
  turns forever. Budget resets when the loop leaves `waiting_review`.
- `notifyTurnIdle` dispatches the owed reinvoke when no other durable intent
  consumed the idle transition.
- `checkAndBlockStalledHub` (hub_stall.go) now re-arms the watchdog before
  returning on a live approval/question card — previously the chain died on
  the first live-card tick and never evaluated again after resolution.

Tests: `bug542_hub_reinvoke_deferred_wedge_test.go` (7 tests — idle
waiting_review owes reinvoke, bounded drain, reset on advance, notifyTurnIdle
dispatches, watchdog re-arms under approval + question, surfaces hub_stalled
after resolution). All green; legacy arm-site contracts unchanged.
