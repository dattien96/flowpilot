# CA-1011 — BUG-510: steps endpoint falls back to the durable session row

## Context

Deep review B-6: after a restart, `GET .../workflow-runs/{id}/steps`
404'd for runs whose durable step rows still existed — the handler only
consulted the in-memory `s.runs` cache. Same bug class BUG-508 fixed for
the snapshot endpoint.

## Changes

- `handleRunSteps` (`interactive_handlers.go`): on an in-memory miss it
  falls back to `durableRunSnapshot(runID)` — the read-only projection
  BUG-508 added — then serves the persisted step list. Unknown ids still
  404; store errors surface 5xx.
- `bug510_steps_runtime_rehydrate_test.go`: durable session + step rows
  with empty `s.runs` → 200 with persisted status; unknown id → 404.

## Verification

`go test -count=1 -run TestBug510 ./internal/runner/` — green.
