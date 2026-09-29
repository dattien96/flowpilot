# CA-1066 — BUG-551 quiet wedge recovery (three seams + adjacent cap hole)

## Symptom (live run-15525)

Sequence observed on the live vibe run:

1. An escalate-park froze and cancelled an in-flight hub turn.
2. `continue` unblocked the loop; the next hub reinvoke hit the round cap
   and was dropped silently.
3. Extend-cap + `continue` dispatched nothing — the loop record stayed
   `running` with zero in-flight work, zero armed intents, and zero
   watchdogs. The wedge was invisible until a manual hub turn was injected.

## Root cause — three stacked recovery holes

- **Seam 1 — resume early return** (`resumeFlowWithFeedback`): when the
  durable loop status was not `blocked`, the function returned the graph
  snapshot without doing anything. `running` was read as "nothing to
  recover", so a running-but-dead loop had no recovery surface at all.
- **Seam 2 — orphaned pending context** (`redriveQuietFlowLoop`): the
  quiet predicate treated non-empty `pendingAgentContext` as proof that a
  turn was scheduled to drain it. But the note's owning turn had been
  cancelled — the context was orphaned input, not evidence of work. The
  predicate therefore returned "busy" forever and never redrove the hub.
- **Seam 3 — watchdog lost on restart** (`reconstructRunInternal`): the
  hub-stall watchdog is an in-memory `time.Timer`; a rehydrated `running`
  flow root never re-armed it, so after restart a dead loop stayed dead
  with no `hub_stalled` card to give `continue` a real surface.
- **Adjacent hole — cap-blocked reinvoke**: `maybeAutoReinvokeHubWithNote`
  returned silently when `Round >= effectiveCap` — the exact drop that
  produced the live wedge — arming nothing. Same class on
  `maybeAutoReinvokeHubWithPrompt`, which re-arms the RAM intent but
  armed no watchdog either (nothing drains the armed intent while no
  turn is in flight).

## Fix

- `resumeFlowWithFeedback` (`interactive_service.go`): when the loop was
  not blocked, operator feedback is appended to `pendingAgentContext` and
  `redriveQuietFlowLoop` is invoked — the same quiet-redrive seam
  `resumePendingLoopWork` owns — instead of returning inert. A busy loop
  is untouched because the quiet predicate self-gates.
- `redriveQuietFlowLoop`: `pendingAgentContext` removed from the busy
  predicate. Every real "a turn is armed" signal (turnInFlight,
  reinvokeInFlight, pendingRestart*, pendingHubReinvoke, pendingResume*,
  pendingGateReprompt, pendingFlowGateSettle, pending cards, vibe gate
  latches, active children) is still checked — only the input buffer is
  no longer mistaken for a scheduled turn.
- `reconstructRunInternal` (`interactive_resume.go`): after the run is
  rebuilt, non-terminal roots (`parentRunID==""` and not
  completed/failed/cancelled) re-arm `maybeScheduleHubStallCheck`. The
  watchdog self-gates on `flowEngineDriven` + loop status and re-arms
  itself while real work is in flight, so a healthy run pays exactly one
  timer; a running-but-dead root regains recovery coverage.
- `maybeAutoReinvokeHubWithNote` + `maybeAutoReinvokeHubWithPrompt`:
  the cap-blocked return path now arms the hub-stall watchdog so the
  dead loop re-surfaces as an actionable `hub_stalled` card
  (continue/extend-cap then has a real surface).

## Invariants preserved

- Bounded: no new unbounded retry — the watchdog surfaces a card; the
  redrive dispatches through the existing `maybeAutoReinvokeHub` path,
  which still enforces the round cap and single-flight guard.
- No duplicate dispatch: the quiet predicate still requires every armed
  intent to be empty before redriving; `reinvokeInFlight` remains the
  single-flight latch.
- Cancelled/failed/completed roots never arm the watchdog (fail-closed
  cancel path unchanged — `normalizeResumedFlowRun` still cancels
  incomplete flow roots with no pending work).
- `pendingAgentContext` semantics unchanged: it is still drained
  atomically by `startTurn`; the fix only stops reading it as a
  scheduling proof.

## Tests — `internal/runner/bug551_quiet_wedge_recovery_test.go`

Four tests, all RED→GREEN verified (stash-prod → fail → restore → pass):

- `TestBug551_ContinueOnQuietRunningLoopRedrives` — `continue` on a
  quiet `running` loop dispatches a hub turn (seam 1).
- `TestBug551_OrphanedPendingContextDoesNotSuppressRedrive` — a
  cancelled-turn orphan note no longer suppresses quiet redrive
  (seam 2).
- `TestBug551_RehydratedRunningFlowRearmsWatchdog` — a reconstructed
  `running` root (kept alive by a parked child, the live shape) re-arms
  the watchdog; a `cancelled` root stays disarmed (seam 3).
- `TestBug551_CapBlockedReinvokeArmsWatchdog` — a reinvoke dropped at
  the round cap arms the watchdog instead of going silent
  (adjacent hole).

Adjacent suites (`TestBug542_*`, `TestBug289_*`, `TestBug318_*`,
`TestBug410_*`, `TestCA799_*`, resume/redrive/stall/reinvoke patterns):
green.

## Provider parity

Provider-agnostic — all changes are in hub scheduling/recovery logic
before provider selection; no adapter, event-stream, or session code
touched.
