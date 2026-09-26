# CA-1005 — BUG-508: run snapshot falls back to the durable session row

## Context

Live runs 16950/3688/22241 post-restart: `GET .../workflow-runs/{id}`
returned `run_not_found` for completed runs whose durable
`provider_sessions` rows still existed — `runSnapshot` only consulted the
in-memory `s.runs` cache, so any run evicted/never rehydrated into RAM
after restart 404'd even though its durable record was intact.

## Changes

- `runSnapshot` (`interactive_handlers.go`): on an in-memory miss it now
  falls back to `durableRunSnapshot(runID)`, which reads the
  `SessionHistoryReader.GetProviderSession` row and projects a read-only
  snapshot (run id, provider session id, provider key, status, cwd).
  Store errors surface as a typed `store_unavailable` 5xx instead of a
  misleading 404; genuinely unknown ids still return `run_not_found`.
  Same store-is-SSOT pattern `projectRunHistory` already uses (BUG-060).
- Regression test `bug508_snapshot_rehydrate_test.go`: seeds a durable
  session row with an empty `s.runs`, asserts the snapshot resolves
  (terminal status intact) and that a store failure surfaces the typed
  error, not a 404.

## Verification

`go test -count=1 -run TestBug508 ./internal/runner/` — green.
