# CA-097: Fix Desktop Run History Lost On Restart (No Supabase)

## Scope

- `apps/local-runner/internal/runner/workflow_store.go` — extend `ProviderSessionState`
- `apps/local-runner/internal/runner/local_file_session_store.go` — new file
- `apps/local-runner/internal/runner/local_file_session_store_test.go` — 12 new tests
- `apps/local-runner/internal/runner/interactive_service.go` — re-persist on turn start/end
- `apps/local-runner/internal/runner/interactive_handlers.go` — populate display fields from restored sessions
- `apps/local-runner/internal/cli/root.go` — wire `localFileSessionStore` at startup
- `requirements/09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md`

Fixes BUG-080: run history disappears on app restart when Supabase is not configured.

## Status

Implemented and committed.

## Implemented Changes

### F-1 — Extend `ProviderSessionState`

Added five fields to the struct in `workflow_store.go`:
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
- `ListProviderSessionsByProject`: reads from in-memory map (populated from disk on startup via `loadFromDisk`).
- `loadFromDisk`: last-wins per `run_id`, skips malformed lines, prunes entries older than 90 days.
- File writes protected by a dedicated mutex. `os.MkdirAll` on construction.

### F-3 — Re-persist on key transitions

Two `persistProviderSession` calls added in `interactive_service.go`:
1. In `startTurn` — snapshot `sessionStateOf(rs)` under lock after `rs.lastPrompt` / `rs.updatedAt` are set, persist outside lock (captures prompt title for history).
2. In `runTurn` after `finishTurn` — snapshot under lock after status/message/time are settled, persist outside lock (captures final state).

Both pass the full `ProviderSessionState` including the new display fields.

### F-4 — `projectRunHistory` display fields

Expanded the `ProviderSessionState → runHistoryItem` mapping in `interactive_handlers.go` to include `StartedAt`, `UpdatedAt`, `LastPrompt`, `LastMessage`, `RunKind` so restored items sort and display correctly.

### F-5 — Wire at startup

In `root.go`, `storeDir` is `filepath.Join(instance.Health().Cwd, ".flowpilot", "chats")` — co-located with `artifacts`, `settings`, etc. Falls back to default in-memory store on any filesystem error.

## Storage Design Notes

- **File location**: `<workspace>/.flowpilot/chats/sessions.ndjson`
- **Content**: run metadata only (`runId`, `projectId`, `status`, `lastPrompt`, `lastMessage`, `providerSessionId`, timestamps). Full conversation content lives in the provider's own storage (Codex thread files, Claude session files).
- **`providerSessionId`** is persisted and is the foundation for a future full resume feature. Currently, clicking a history item from a previous session returns `run_not_found` because `s.runs` is empty after restart.
- **NDJSON**: one line per upsert, last-wins per `run_id` on load. Chosen over a full-rewrite JSON array to avoid data loss on crash mid-write.
- **90-day pruning**: applied on startup to bound file growth.

## Verification

- `go build ./...` → clean
- `go test ./internal/runner/...` → all tests pass
- 12 unit tests cover: upsert+list, restart round-trip, multiple runs, last-wins, project filter, 90-day pruning, missing file, malformed lines, WorkflowStore delegation, InteractiveStateStore delegation, nested subdir creation, interface compliance.

## Residual Notes

- Task-070 / commit 48db8f0 is the Supabase half of this fix (complete). This is the local-disk half.
- `fakeWorkflowStore` is deliberately left unchanged — it remains the test double.
- History only persists for runs created after the fix is deployed; runs from prior sessions were never written to disk.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-080
change_type: fix
summary: Fix Desktop Run History Lost On Restart (No Supabase)
# --->8---
