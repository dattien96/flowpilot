# CA-985 — BUG-486: dispatch store shard read errors surface to recovery + attention consumers

## What changed

`apps/local-runner/internal/runner/dispatch_store_open.go`:

- `forRun`: remembers the first shard-open failure during the disk scan;
  returns it when no healthy shard contains the run ("not found" is only
  provable when every shard read cleanly). A `os.ReadDir` failure on the
  chats root also propagates.
- `ListRecoverable(ctx, "")`: now discovers on-disk project shards (was:
  only already-open stores — a shard never written in this process was
  invisible to recovery) and returns partial results plus the first shard
  error instead of `continue`-swallowing.
- `ListRecoverable(ctx, runID)`: `ErrNotFound` stays clean (`nil, nil`);
  other `forRun` errors propagate.
- `ListAttention`: same partial+first-error contract; `os.ReadDir` failure
  surfaces too.

## Why

BUG-484 added a retry coordinator for dispatch recovery boot passes, and
BUG-475 taught the SSE path to map attention-authority errors to a resync
frame — but the file-backed store could never produce such an error:
`forProject`/`forRun`/`ListRecoverable`/`ListAttention` each discarded shard
failures. An unreadable `dispatch.ndjson` (permissions, EIO, wrong fs type)
silently looked like "no recoverable records / no attention", leaving the
retry coordinator with nothing to retry and the SSE snapshot claiming empty.

## How found

BUG-484 live-drill prep: reading the aggregation loops to design a fault
injection showed the error branches were dead code on the production store.

## Compatibility

- Provider-agnostic; durable-store fan-out only.
- All existing callers already branch on `err != nil` (recovery scanner →
  pass failure → coordinator retry; `dispatchAttentionByRun` → resync;
  operator endpoint → 502; settle drive → log+propagate). Happy-path results
  are unchanged; `ErrNotFound` semantics preserved.
- `localDispatchStore` malformed-line handling (torn-drop counting) is
  untouched.

## Tests

`internal/runner/bug486_dispatch_shard_errors_test.go` (6 tests,
reproduce-first red): shard unreadable → `ListRecoverable("")` errors while
keeping readable-shard records; `ListAttention` errors; `ListRecoverable(id)`
errors; clean not-found preserved; `ScanAllRecoverable` pass fails (retry
trigger); `forRun` distinguishes unprovable absence from `ErrNotFound`.
