# BUG-482: Desktop mux parser silently drops malformed authoritative SSE frames without resync

## Metadata

- Document ID: `BUG-482`
- Phase: `bugfix`
- Status: `done`
- Severity: `medium-high`
- Evidence: `code-confirmed; transport fault-injection pending`
- Feature Keys: `event-plane`
- Parent Documents: `CP-84`, `Task-429`
- Related Documents: `BUG-475`, `BUG-479`
- Affected Area: `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`

## Summary

`streamRunUpdates` catches JSON parse errors and silently skips the frame while
keeping the SSE connection alive. Because frames are authoritative level-state
changes, losing one upsert/remove can leave stale lane or decision state until
an unrelated later mutation or 30-second poll. No reconnect snapshot is
requested and no diagnostic identifies the loss.

## Evidence

```ts
try {
  yield JSON.parse(json) as RunRealtimeFrame;
} catch {
  // skip malformed frame
}
```

CP-84's reconnect contract relies on a fresh full snapshot to heal missed
frames. Silent continuation bypasses that recovery mechanism.

## Expected vs Actual

- Expected: malformed authoritative frame terminates/restarts the stream (or
  emits an explicit resync) so a full snapshot reconciles state.
- Actual: the individual frame is dropped and the stream is considered healthy.

## Impact

Approval/gate/repair items may linger or fail to appear, terminal removes may be
missed, and reconnect backoff is not activated. Polling eventually helps some
run statuses but does not replace every decision payload contract.

## Required Tests

- RED malformed JSON between valid frames triggers reconnect and snapshot.
- Fragmented and multi-frame chunks still parse normally.
- CRLF and LF SSE separators are accepted.
- Malformed snapshot chunk never partially commits.
- Logging contains frame metadata only, never decision payload text.

## Implementation Plan

### P-1 — Parser RED matrix

- Add `HttpWsRunnerClient` stream tests using a controlled `ReadableStream`.
- Cases: fragmented JSON, multiple frames/chunk, LF, CRLF, comment heartbeat,
  multi-line `data:`, malformed JSON and malformed snapshot middle chunk.
- Assert current malformed case silently continues without reconnect.

### P-2 — Fail stream on authoritative parse error

- Extract a small SSE frame decoder shared only if doing so preserves the
  existing per-run stream contract.
- Normalize legal SSE line endings and concatenate multiple data lines per spec.
- On malformed non-comment event, throw a typed `stream_protocol_error`; do not
  yield later frames from the same connection.
- Never partially commit a snapshot: the attention queue keeps staging until a
  valid `complete` frame from one snapshot ID.

### P-3 — Reconnect and diagnostics

- `consumeRunUpdatesLoop` catches the typed error, retains committed state,
  applies normal bounded backoff and reconnects for a full snapshot.
- Emit one bounded diagnostic containing event kind/byte length, not raw data.
- Repeated malformed streams obey max backoff and do not spin.

### P-4 — End-to-end transport proof

- Serve one malformed frame between valid updates from an HTTP test server,
  then a healthy reconnect snapshot. Verify stale state is corrected only by
  the complete snapshot and no partial burst leaks.

## Definition of Done

- [ ] Parser supports fragmented/multi-frame LF and CRLF streams.
- [ ] Malformed authoritative frame terminates that stream.
- [ ] Reconnect occurs with bounded backoff and full snapshot reconciliation.
- [ ] Last committed lane/decision state remains visible during reconnect.
- [ ] Partial malformed snapshot never commits.
- [ ] Logs redact payload text and avoid reconnect loops.
- [ ] Existing per-run SSE behavior remains byte-compatible or gains equivalent tests.
- [ ] CP-84 mux-loop and HTTP handler suites pass.
