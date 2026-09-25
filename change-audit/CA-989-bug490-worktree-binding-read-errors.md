# CA-989 — BUG-490: chat worktree binding reads fail closed

## What changed

`apps/local-runner/internal/runner/run_worktree.go` +
`interactive_handlers.go`:

- `findChatWorktreeBindingLocked` returns `(*worktreeBinding, error)` — the
  `ListProviderSessionsByChat` failure propagates instead of looking like
  "no binding".
- `createRun` maps it to HTTP 502 `worktree_binding_unprovable`: a new leg
  never mounts a second worktree over an unproven binding.
- `markChatWorktreeState` returns `error`: leg-scan failure propagates;
  per-leg persist failures are logged and the first is returned.

## Why

Binding inheritance is how a chat's legs share one worktree. When the leg
scan errored, the lookup returned "no binding" and `createRun` provisioned
a second worktree for the same chat — split-brain isolation. The state
marker had the same asymmetry: a failed transition left durable rows stale
(e.g. `discarded` adopted back as live).

## Verification

- `bug490_worktree_binding_reads_test.go`: persisted live binding + reader
  fault → create fails 502, no new worktree; `markChatWorktreeState`
  surfaces scan/persist errors.
- Runner package suite green.
