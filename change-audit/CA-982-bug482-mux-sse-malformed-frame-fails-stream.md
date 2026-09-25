# CA-982: BUG-482 — malformed mux SSE frame fails the stream for resync

## Why

`streamRunUpdates` caught JSON.parse errors and silently skipped the
frame while keeping the connection alive. Mux frames are authoritative
level-state changes: losing one upsert/remove leaves stale lane/decision
state until an unrelated mutation or the 30s poll — with no reconnect
snapshot requested and no diagnostic.

## What changed

- `HttpWsRunnerClient.streamRunUpdates` parser rewritten:
  - Events split per SSE spec on a blank line — `\n\n` or `\r\n\r\n`
    (was: `\n\n` only).
  - Multiple `data:` lines per event join with `\n` (was: first line
    only, which also made multi-line data undeliverable).
  - Comment/heartbeat and non-data field lines still ignored.
  - Malformed JSON now throws `RunnerApiError(0, "stream_protocol_error",
    "... (N bytes)")` — byte-length metadata only, never payload text.
    Frames after the malformed one on the same connection are never
    yielded.
- `consumeRunUpdatesLoop` needs no change: its existing catch applies
  bounded backoff and reconnects for a healing full snapshot; committed
  state stays visible meanwhile.
- Per-run `openStream` parser intentionally left byte-compatible — its
  events are delta-replayable with seq cursors, a different contract.

## Existing-test change (documented, not weakened)

`streamRunUpdates.test.ts` had a test literally named "malformed JSON
frame is skipped, stream continues" — it pinned the defect this BUG
removes. Updated to assert the new contract (typed throw); every other
assertion in the file is unchanged.

## Tests

- `streamRunUpdatesBug482.test.ts` (5): typed throw + no post-error
  yields, redacted diagnostics, CRLF parity, multi-line data join,
  malformed mid-snapshot chunk kills connection before commit.
- `muxUpsertBug479.test.ts` +1: stream drop → reconnect → complete
  snapshot heals the lane set while pre-drop state stays visible.

## Verification

- Client/state suites: 16/16 green.
- Full desktop suite 651 tests: 13 failures all reproduced on clean tree
  (11 documented env/flake + 2 phase1 baselines); zero new.
