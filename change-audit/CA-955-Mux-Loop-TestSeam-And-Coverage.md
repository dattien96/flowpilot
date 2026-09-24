# CA-955 — KR-005 CP-84: SSE parser + reconnect/backoff + deep-link coverage

## Summary

The mux transport had zero tests below the state layer. Added:

- `streamRunUpdates.test.ts` — SSE parser: frames fragmented across chunks,
  multiple frames per chunk, malformed JSON skipped, heartbeat comments
  ignored, non-OK → RunnerApiError, pre-aborted signal yields nothing.
- `muxLoop.test.ts` — consumeRunUpdatesLoop: reconnect after clean end with
  ≥500ms backoff, complete-snapshot resets backoff to min, resync triggers
  reconnect, exponential growth (500→1000→2000), upsert frames patch history
  lanes end-to-end.
- `attentionDeepLink.test.ts` — notification-click path: cross-project
  `openRunAtAttention` selects project before opening, same-project opens
  directly, item evicted from queue falls back to polled runHistory.

To make the module-private infinite loop testable, added additive export
`runUpdatesLoopTestHooks` in store.ts (consume + stop); production wiring via
`startRunUpdatesStream` is unchanged.

## Files

- `apps/desktop-flowpilot/src/state/store.ts` — `runUpdatesLoopTestHooks`
  additive seam only
- `apps/desktop-flowpilot/src/client/streamRunUpdates.test.ts` — NEW
- `apps/desktop-flowpilot/src/state/muxLoop.test.ts` — NEW
- `apps/desktop-flowpilot/src/state/attentionDeepLink.test.ts` — NEW

## Verified

- New files: 14/14 pass via compiled node --test.
- Regression: runUpdates + attention_queue + store.spectator + boardModel —
  29/29 pass.

## Provider parity

Provider-agnostic: transport/backoff/notification plumbing; lane projections
carry `providerKey` but the loop logic never branches on it (covered by the
provider-parity runner test upstream).
