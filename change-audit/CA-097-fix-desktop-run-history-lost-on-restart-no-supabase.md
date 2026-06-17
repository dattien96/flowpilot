# CA-097: Fix Desktop Run History Lost On Restart (No Supabase)

## Scope

- `apps/local-runner/internal/runner/workflow_store.go` — extend `ProviderSessionState`
- `apps/local-runner/internal/runner/local_file_session_store.go` — new file
- `apps/local-runner/internal/runner/local_file_session_store_test.go` — new file
- `apps/local-runner/internal/runner/interactive_service.go` — re-persist on turn start/end
- `apps/local-runner/internal/runner/interactive_handlers.go` — populate display fields from restored sessions
- `apps/local-runner/internal/cli/root.go` — wire `localFileSessionStore` at startup
- `requirements/09-BugFix/todo/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md`

Fixes BUG-080: run history disappears on app restart when Supabase is not configured.

## Status

Not yet implemented. This audit document was created to track the planned change.

## Planned Changes

### F-1 — Extend `ProviderSessionState`

Add five fields to the struct in `workflow_store.go`:
```go
LastPrompt  string
LastMessage string
StartedAt   string
UpdatedAt   string
RunKind     string
```
No interface changes. Callers that only use `RunID`/`ProjectID`/`Status` are unaffected.

### F-2 — `localFileSessionStore`

New type implementing `WorkflowStore` + `InteractiveStateStore` + `SessionHistoryReader`.

- Embeds `*fakeWorkflowStore` — delegates all `WorkflowStore` methods (step transitions, run status, logs) unchanged.
- `UpsertProviderSession`: updates in-memory map AND appends a NDJSON line to `{dataDir}/sessions.ndjson`.
- `ListProviderSessionsByProject`: reads NDJSON file (last-wins per `run_id`), filters by `project_id`, merges with in-memory map.
- File writes protected by a mutex. Directory created on first write.
- Startup pruning: entries with `UpdatedAt` older than 90 days are dropped on load to bound file growth.

### F-3 — Re-persist on key transitions

Two new `persistProviderSession` calls in `interactive_service.go`:
1. In `finalizeInputLocked` — after `rs.lastPrompt` / `rs.updatedAt` are set (captures prompt title for history).
2. In `finishTurn` — after `rs.status` / `rs.lastMessage` / `rs.updatedAt` are settled (captures final state).

Both calls pass the full current `ProviderSessionState` including the new display fields.

### F-4 — `projectRunHistory` display fields

Expand the `ProviderSessionState → runHistoryItem` mapping in `interactive_handlers.go:619-625` to include `StartedAt`, `UpdatedAt`, `LastPrompt`, `LastMessage`, `RunKind` so restored items sort and display correctly.

### F-5 — Wire at startup

In `root.go`, construct `localFileSessionStore` (data dir from `--session-store` flag or `os.UserCacheDir()/flowpilot/`) and pass it to `newInteractiveService` when Supabase is not configured.

## Verification Plan

- `go build ./...` → no compile errors
- `go test ./internal/runner/...` → all existing tests pass
- New `TestLocalFileSessionStoreRoundTrip` — round-trip upsert + restart simulation
- Manual smoke test: start runs, restart app, confirm history persists

## Residual Notes

- Task-056 / commit 48db8f0 is the Supabase half of this fix (now complete). This is the local-disk half.
- `fakeWorkflowStore` is deliberately left unchanged — it remains the test double.
- NDJSON format chosen over full JSON rewrite to avoid data loss on crash mid-write.
