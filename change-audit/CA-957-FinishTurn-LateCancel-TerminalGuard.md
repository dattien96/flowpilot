# CA-957 — Live-found: late context.Canceled overwrote completed runs

## Summary

Live SWE-2 validation (KR-005 follow-up) surfaced a real status bug: a chat
run whose turn completed successfully was stamped `cancelled` ~200ms later —
durable `sessions.ndjson` showed `running → completed → cancelled` with
`last_message: "DONE"` and no `turn-failed`/interrupt log.

Root cause: `InteractiveService.finishTurn`'s generic `context.Canceled`
branch ("interrupted by user") set `rs.status = RunStatusCancelled`
unconditionally. A late teardown cancellation arriving after the turn had
already settled terminal overwrote the result. Sibling paths already guard
(`rs.status == RunStatusRunning` checks); this branch did not.

Fix: terminal statuses (`completed`/`failed`/`cancelled`) are sticky inside
the cancel branch — a late cancel returns early instead of re-stamping.
`pendingFlowGateSettle` is deliberately NOT in the guard: a real user
interrupt while a gate is pending must still cancel.

## Repro

`apps/local-runner/internal/runner/finish_turn_terminal_guard_test.go` —
`TestFinishTurnLateCancelPreservesCompleted`: `finishTurn(ctx.Canceled)` on a
completed run asserted `cancelled` before the fix (red), now `completed`.

## Files

- `apps/local-runner/internal/runner/interactive_service.go` — terminal guard
  in the `context.Canceled` branch of `finishTurn`
- `apps/local-runner/internal/runner/finish_turn_terminal_guard_test.go` — NEW

## Verified

- Focused: `TestFinishTurnPreservesFailedForStalledSkipCancel`,
  `TestFinishTurnCancelsNormallyWithoutStalledSkipCause`,
  `TestFinishTurnStallRetrySuppressesCohortFailed`,
  `TestFinishTurnLateCancelPreservesCompleted` — all pass.
- Full `go test ./internal/runner` suite pass recorded in thread.

## Provider parity

Provider-agnostic: `finishTurn` is turn-finalization logic shared by all
providers; the guard keys on run status, not provider type. No adapter,
event-stream, or session-shape change.
