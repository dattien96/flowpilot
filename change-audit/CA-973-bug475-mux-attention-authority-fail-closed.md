# CA-973 — BUG-475: mux dispatch-attention read failure is not an empty decision set

Date: 2026-09-25
Branch: cp_live_test
Scope: `apps/local-runner/internal/runner`

## Problem

`dispatchAttentionByRun` swallowed `DispatchStore.ListAttention` errors and
returned an empty map. Both mux paths consumed it as authoritative:

- `subscribeRunUpdates` emitted a complete snapshot with zero dispatch
  decisions — the client reconcile erased live `uncertain`,
  `repair_required`, `cancel_required`, `settle_pending` inbox items.
- `drainRunUpdates` projected a terminal run whose only lane relevance was
  dispatch attention as lane-irrelevant and emitted a `remove` tombstone —
  stranding the repair surface until the next poll.

## Fix — fail closed through the existing resync contract

- `dispatchAttentionByRun` now returns `(map, error)`; a nil store remains a
  legitimate "no dispatch authority configured" (empty, nil).
- `subscribeRunUpdates`: on attention-read failure the subscriber is
  registered as `closed`+`retryable` and no lanes are returned; the handler
  (new `runUpdateSubRetryableClosed` check in `handleAllEventsStream`) emits
  a retryable `resync` frame instead of any snapshot — never an empty
  `complete` that would reconcile the client to zero lanes.
- `drainRunUpdates`: on failure it commits nothing (dirty set, fingerprints,
  and `removed` markers untouched — closing the subscriber discards them
  wholesale, which is equally non-committing), marks the subscriber
  closed+retryable, removes it from the registry, and returns a single
  retryable `resync` frame. No `remove` is ever emitted from an unreadable
  authority.
- Recovery uses the existing T-5 client path: reconnect → fresh subscribe →
  either a complete authoritative snapshot (store healed) or another
  retryable resync. Bounded by client reconnect backoff; no new protocol.
- Logs carry only subscriber id — never decision reason/payload text.

## Tests

`bug475_mux_attention_authority_test.go` (red before fix — 3/4 failed:
empty snapshot emitted, remove tombstone emitted, `snapshot{complete}` on
the wire):

- `TestBUG475_SnapshotStoreFailureIsRetryableResync` — subscribe registers
  closed+retryable and returns no lanes.
- `TestBUG475_DrainStoreFailureEmitsResyncNotRemove` — drain never
  tombstones the terminal repair lane; single retryable resync.
- `TestBUG475_RecoveryResnapshotRestoresRepairDecision` — healed store
  re-projects `dispatch-repair:{runId}` with stable ID/revision, exactly
  once.
- `TestBUG475_HTTPStreamResyncThenHealthySnapshot` — wire-level: first
  frame is retryable `resync`, never `snapshot{complete}`; reconnect after
  heal delivers the repair decision.

## Verification

- `go test -count=1 -run 'TestBUG475' ./internal/runner/` — pass
- `go test -count=1 -run 'Mux|RunUpdates|Decision|Dispatch|CP84' ./internal/runner/` — pass
- Full suite `go test -count=1 ./...` — env-baseline reds only (see BUG doc).
