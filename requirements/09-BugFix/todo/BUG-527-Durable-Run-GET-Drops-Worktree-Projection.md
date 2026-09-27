# BUG-527 — Durable single-run GET drops worktree metadata

## Severity

Important — restart read projection is incomplete.

## Evidence

- `apps/local-runner/internal/runner/interactive_handlers.go:1442-1453`
- `apps/local-runner/internal/runner/interactive_handlers.go:1770-1801`
- `apps/local-runner/internal/runner/workflow_store.go:211-221`

`durableRunSnapshot` now correctly normalizes status for a run absent from RAM, but it only projects run ID, provider session, provider, status and working directory. `ProviderSessionState` durably contains the worktree owner, path, branch, base, slug, state, enabled flag and resolution, while the live snapshot includes a `worktree` object.

## Impact

After restart, terminal or non-rehydrated worktree runs lose their worktree badge/state and merge-resolution metadata on `GET /client/workflow-runs/{id}`, even though the same data is present on disk. The endpoint differs depending on whether the run happens to be resident in memory.

## Missing regression

Seed a persisted-only run with a complete worktree binding and call the real single-run HTTP GET. Assert parity with the in-memory snapshot's `worktree` projection, including pending merge resolution state.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
