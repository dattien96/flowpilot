# CA-1029 — BUG-521: flow hub withholds `completed` while mounted loop is open

Date: 2026-09-26 — BUG-521 found live on run-60145 (round-5 tournament drive)

## Change

`apps/local-runner/internal/runner/interactive_service.go`:

- New helper `flowHubCompletionWithheldLocked(rs)`: for a flow-engine ROOT
  run, returns true while the mounted loop status is anything other than
  ""/"done"/"stopped" (mirrors the BUG-507 emitLocked open-loop withhold
  condition exactly).
- Live gate-pass settle (~line 9241): after a post-turn gate pass on a run
  with armed `pendingFlowGateSettle`, a flow hub with an open loop now stays
  `running` instead of publishing `completed`. The settle is still consumed,
  the deferred `turn_completed` event still materializes/broadcasts, and
  `signalChild` still fires — only the dishonest run-level terminal changes.
- Restart-resume gate pass (`resumePendingFlowGate`, ~line 5634): same
  withhold, so a reconstructed hub with a durable settle and an open loop
  cannot publish `completed` either.

## Why

`emitLocked` treats a code-free non-flow root with an open loop as still
running (BUG-507), but the deferred-gate publish seam stamped `completed`
unconditionally — and a flowEngineDriven root ALWAYS arms
`pendingFlowGateSettle` for its own turns. Live on run-60145: the
continue-dispatched hub turn settled with loop `running` post-unblock and the
run read `completed` while `candidate-b` sat WAITING_USER_APPROVAL and
`tournament_arbiter`/`merge_and_audit` were PENDING. The gate only verifies
the turn's diff; graph completeness is the loop's job — the loop publishes
the real terminal via `markFlowRunComplete`/`settleParentRunOnFlowDone`, and
an open-loop hub now honestly shows `running` until then.

`loop == "blocked"` before the gate is already guarded by the run-1675 skip
(~line 8966); the new check additionally covers a loop that flips to
blocked/paused/escalation mid-gate.

## Tests

- `TestBug521_GatePassWithholdsCompletedWhileLoopOpen` — live seam, loop
  `running` + mid-turn `blocked`; verified RED (assertion) pre-fix.
- `TestBug521_ResumedGatePassWithholdsCompletedWhileLoopOpen` — restart seam
  via `resumePendingFlowGate` with a durable armed settle + open loop.
- `TestBug521_SealedLoopGatePassStillCompletes` — regression direction:
  sealed (`done`) loop still completes.
- `TestBug521_LiveFlowRunStaysRunningWhileLoopOpen` — live-entry check:
  real `startTurn(FlowRef)` first turn → `startResolvedFlow` → delegate
  children (`drafting`→`final_check`) → joined-result hub synthesis turn →
  post-turn gate → run stays `running` with the loop `blocked` by the
  no-tool-call escalation instead of publishing `completed`.

## Risk

Low-moderate: mid-flow hub runs now report `running` (not `completed`)
between turns — the shape BUG-507 already established for open loops and the
shape `normalizeResumedFlowRun`/`redriveQuietFlowLoop` already handle on
restart. Event/turn stream semantics unchanged (deferred `turn_completed`
still broadcasts; the event is turn-level, the status is run-level).
