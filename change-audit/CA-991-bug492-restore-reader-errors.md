# CA-991 — BUG-492: Drive restore aborts on unreadable collision probes and local-ahead reads

## What changed

`apps/local-runner/internal/runner/chat_session_sync.go`:

- `resolveRestoredRunID` → `(string, error)`: `GetProviderSession` and
  `ListAllProviderSessions` errors abort the restore (502-class `apiErr`)
  instead of returning `sourceRunID` verbatim — which would have upserted
  over an unrelated local record.
- `firstFreeRestoredRunID` accepts a `probe func(string) (bool, error)`;
  a probe failure aborts instead of treating the candidate as free.
- Local-ahead merge extracted to `applyLocalAheadSessionFields`; a
  `GetProviderSession` fault aborts the restore with
  `workflow_state_unavailable` instead of applying stale remote metadata
  over a locally-ahead record.

## Why

Restore collision checks exist to protect local state. A failed read
previously collapsed to "no collision"/"no local row" — the two exact
cases where restore *must not* write.

## Verification

- `bug492_restore_reader_errors_test.go`: index read failure, collision
  probe failure, and local-ahead read failure each abort; existing
  suffix-allocation and field-preservation tests unchanged.
- Runner package suite green.
