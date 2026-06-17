# BUG-080: Desktop Run History Lost On App Restart (No Supabase)

## Metadata

- Document ID: `BUG-080`
- Title: `Desktop Run History Lost On App Restart (No Supabase)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-070: History Supabase Reader Production Fix](../../08-Task/done/Task-070-History-Supabase-Reader-Production-Fix.md), [BUG-060: Desktop Run History Empties After Switching Runs](./BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `desktop, history, local-runner, persistence, restart`

## AI Quick View

### Summary

- When the desktop app restarts (runner and/or app-server process re-created), the History panel shows nothing — all previous runs are lost.
- The actual conversation data (Claude session files, Codex workspace files) is still present on disk in each provider's own storage. What is lost is the runner's **history index**: the `ProviderSessionState` records (runId, projectId, lastPrompt, status, timestamps) that `projectRunHistory` reads to populate the sidebar.
- Task-070 (commit 48db8f0) fixed this for the **Supabase path**: `SupabaseWorkflowStore.ListProviderSessionsByProject` now reads `workflow_provider_sessions` from the database after restart. That fix is only active when Supabase is configured.
- For the **local (non-Supabase) path** — the default for desktop users — `newInteractiveService` falls back to `fakeWorkflowStore`, which is a pure in-memory store. Both `s.runs` (InteractiveService's run map) and `fakeWorkflowStore.sessions` are empty on a new process; history is gone.
- A second problem compounds the issue: `ProviderSessionState` is missing the display fields that `projectRunHistory` populates for in-memory runs (`LastPrompt`, `LastMessage`, `StartedAt`, `UpdatedAt`, `RunKind`). Even if sessions were persisted to disk, restored history items would appear with blank prompt and zero timestamps, sorted incorrectly.

### What Was Built

- `localFileSessionStore`: new type in `local_file_session_store.go` that embeds `*fakeWorkflowStore` and adds NDJSON write-through to `<workspace>/.flowpilot/chats/sessions.ndjson`.
- `ProviderSessionState` extended with `LastPrompt`, `LastMessage`, `StartedAt`, `UpdatedAt`, `RunKind`.
- `persistProviderSession` called at two additional points: `startTurn` (captures prompt) and `runTurn` post-`finishTurn` (captures final status/message).
- `projectRunHistory` extended to populate all display fields from restored sessions.
- 12 unit tests in `local_file_session_store_test.go` covering round-trip, restart simulation, last-wins, pruning, malformed lines, and interface compliance.

### What Is NOT Built (Scope Boundary)

- **`sessions.ndjson` stores metadata only** — `runId`, `projectId`, `status`, `lastPrompt`, `lastMessage`, `providerSessionId`, timestamps. It does NOT store full conversation content.
- **Full conversation replay after restart is not implemented.** Clicking a history item from a previous session will fail with `run_not_found` because `s.runs` is empty after restart. The `providerSessionId` field is persisted and is the foundation for a future resume feature.

### Key Decisions

- `V-1` A new `localFileSessionStore` (not a modification of `fakeWorkflowStore`) that writes `ProviderSessionState` records to `<workspace>/.flowpilot/chats/sessions.ndjson` — co-located with `.flowpilot/artifacts/`, `.flowpilot/settings/`, etc. (A global OS cache dir was considered but rejected in favour of keeping history with the workspace.)
- `V-2` `fakeWorkflowStore` stays as-is — it is used by tests and does not need disk persistence.
- `V-3` `ProviderSessionState` gains: `LastPrompt string`, `LastMessage string`, `StartedAt string`, `UpdatedAt string`, `RunKind string`. These map 1-to-1 to the fields on `interactiveRun` that `projectRunHistory` currently reads only from `s.runs`.
- `V-4` Re-persist at two points: (a) `startTurn` — after `rs.lastPrompt` / `rs.updatedAt` are set under lock, persist outside lock; (b) `runTurn` — after `finishTurn` returns, capture snapshot under lock, persist outside. Not on every event — too chatty.
- `V-5` `root.go` constructs `localFileSessionStore` with `storeDir = filepath.Join(instance.Health().Cwd, ".flowpilot", "chats")` and passes it to `NewInteractiveServiceWithStore`. Falls back to `fakeWorkflowStore` on any filesystem error.
- `V-6` `localFileSessionStore` wraps `fakeWorkflowStore` for the `WorkflowStore` interface methods (step transitions, run status, logs) so `newInteractiveService` always has a complete `WorkflowStore`. The file store adds only the persistence layer on top.
- `V-7` NDJSON last-wins with 90-day pruning on startup. Malformed lines are silently skipped.

### Constraints

- Do not modify `fakeWorkflowStore` — it must stay pure-in-memory for tests.
- Do not change the `SessionHistoryReader` or `InteractiveStateStore` interface signatures.
- Do not change any desktop or admin-web code — backend Go only.
- `localFileSessionStore` must be safe for concurrent writes (mutex on file operations).
- NDJSON append is preferred over a full-rewrite JSON array to avoid data loss on crash mid-write.

### Source Refs

- `apps/local-runner/internal/runner/local_file_session_store.go` — new file
- `apps/local-runner/internal/runner/local_file_session_store_test.go` — 12 tests
- `apps/local-runner/internal/runner/workflow_store.go` — `ProviderSessionState` extended
- `apps/local-runner/internal/runner/interactive_service.go` — `startTurn`, `runTurn` re-persist calls
- `apps/local-runner/internal/runner/interactive_handlers.go` — `projectRunHistory`, `createRun` display fields
- `apps/local-runner/internal/cli/root.go` — wire at startup

## 1. Issue Summary

After restarting the desktop app (or the local runner process), the History sidebar panel shows no runs. Runs from the previous session are not visible even though the underlying provider conversation data (Claude session files, Codex workspace files) is still present on disk.

## 2. Parent Links

- direct upstream: [Task-070](../../08-Task/done/Task-070-History-Supabase-Reader-Production-Fix.md) — Task-070 closed the Supabase gap; this bug closes the local-disk gap.
- related: [BUG-060](./BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md) — established the `SessionHistoryReader` pattern this bug must extend.

## 3. Environment and Reproduction

- environment: desktop-flowpilot, local runner, **Supabase not configured** (the typical desktop-only setup)
- reproduction steps:
  1. Open the desktop app. Select a project. Start one or more runs (chat or workflow).
  2. Confirm the runs appear in the History sidebar.
  3. Quit and reopen the desktop app (or restart the runner process).
  4. Observe: the History sidebar is empty. All previously visible runs are gone.
- frequency: 100% reproducible when Supabase is not configured

## 4. Expected vs Actual

- expected: after restart, previously started runs reappear in the History sidebar (at minimum their status, prompt title, and time)
- actual: History sidebar is empty; all history is lost

## 5. Impact

- users affected: all desktop users not using Supabase (i.e. most desktop-only installs)
- severity: high — users lose all run history every session; the sidebar is effectively a within-session-only view

## 6. Root Cause

Two independent problems combine:

### R-1 — No disk persistence for `ProviderSessionState` in local mode

`newInteractiveService` uses `fakeWorkflowStore` when no store is provided:

```go
// interactive_service.go:190-191
if workflowStore == nil {
    workflowStore = newFakeWorkflowStore()
}
```

`fakeWorkflowStore` implements `InteractiveStateStore` and `SessionHistoryReader` entirely in memory:

```go
// workflow_store.go:96, 108
sessions map[string]ProviderSessionState{}
```

On restart, this map is empty. `projectRunHistory` finds nothing via `SessionHistoryReader` and falls back to `s.runs` which is also empty.

Task-070 added `SupabaseWorkflowStore.ListProviderSessionsByProject` which reads from the database — but only fires when the store is a `SupabaseWorkflowStore`. With `fakeWorkflowStore`, the type assertion at `interactive_handlers.go:612` still succeeds (fake store implements the interface), but returns nothing because the in-memory map was cleared.

### R-2 — `ProviderSessionState` missing display fields; session not re-persisted after creation

`persistProviderSession` was called exactly once per run — at creation (`createRun`) — with only these fields set:

```go
ProviderSessionState{
    RunID, ProjectID, WorkflowID,
    ProviderSessionID, ProviderKey, ProviderAccountID,
    WorkingDirectory, Status: rs.status (= idle),
}
```

The display fields that matter for the History panel — `LastPrompt`, `LastMessage`, `StartedAt`, `UpdatedAt`, `RunKind` — were never written to `ProviderSessionState`. They only existed on `interactiveRun` in `s.runs`.

## 7. Fix

### F-1 — Extend `ProviderSessionState` with display fields

Added to `ProviderSessionState` in `workflow_store.go`:
- `LastPrompt string`
- `LastMessage string`
- `StartedAt string`
- `UpdatedAt string`
- `RunKind string`

### F-2 — `localFileSessionStore`

New file: `apps/local-runner/internal/runner/local_file_session_store.go`

Implements `WorkflowStore` + `InteractiveStateStore` + `SessionHistoryReader`:
- Embeds `*fakeWorkflowStore` — delegates all `WorkflowStore` methods unchanged.
- `UpsertProviderSession`: updates in-memory map AND appends a NDJSON line to `{dataDir}/sessions.ndjson`.
- `ListProviderSessionsByProject`: reads from the in-memory map (populated on startup from file).
- On startup `loadFromDisk()`: reads all NDJSON lines, last-wins per `run_id`, prunes entries older than 90 days, skips malformed lines.
- File writes protected by a dedicated mutex (separate from `fakeWorkflowStore`'s mutex).
- `os.MkdirAll` ensures the directory is created if it does not exist.

### F-3 — Re-persist session on key state transitions

Two new `persistProviderSession` calls in `interactive_service.go`:

1. **`startTurn`**: after `rs.lastPrompt` and `rs.updatedAt` are set under `s.mu`, snapshot via `sessionStateOf(rs)` and persist outside the lock (captures prompt title immediately).
2. **`runTurn`** after `finishTurn`: lock, snapshot `sessionStateOf(rs)`, unlock, persist (captures final `status`, `lastMessage`, `updatedAt`).

### F-4 — `projectRunHistory` display fields

Expanded the `ProviderSessionState → runHistoryItem` mapping in `interactive_handlers.go` to include `StartedAt`, `UpdatedAt`, `LastPrompt`, `LastMessage`, `RunKind`.

### F-5 — Wire at startup

In `apps/local-runner/internal/cli/root.go`:

```go
var sessionStore runner.WorkflowStore
storeDir := filepath.Join(instance.Health().Cwd, ".flowpilot", "chats")
if fs, err := runner.NewLocalFileSessionStore(storeDir); err == nil {
    sessionStore = fs
}
interactive := runner.NewInteractiveServiceWithStore(
    runner.ProviderRegistryFor(instance),
    runner.CatalogStoreFor(instance),
    sessionStore,
)
```

Falls back to `fakeWorkflowStore` (nil → default) on any filesystem error.

## 8. Validation

- `go build ./...` → clean
- `go test ./internal/runner/...` → all tests pass
- 12 unit tests in `local_file_session_store_test.go`:
  - `TestLocalFileSessionStoreUpsertAndList`
  - `TestLocalFileSessionStoreRestart`
  - `TestLocalFileSessionStoreMultipleRunsRestart`
  - `TestLocalFileSessionStoreLastWins`
  - `TestLocalFileSessionStoreProjectFilter`
  - `TestLocalFileSessionStorePruning`
  - `TestLocalFileSessionStoreMissingFile`
  - `TestLocalFileSessionStoreMalformedLines`
  - `TestLocalFileSessionStoreWorkflowStoreDelegation`
  - `TestLocalFileSessionStoreInteractiveStateDelegation`
  - `TestLocalFileSessionStoreNewSubdir`
  - `TestLocalFileSessionStoreSessionHistoryReaderInterface`

## 9. Regression Guard

- `fakeWorkflowStore` is not modified — all existing tests continue to use it unchanged.
- The `localFileSessionStore` wraps `fakeWorkflowStore` for `WorkflowStore` methods, so orchestration behaviour is identical.
- NDJSON last-wins merge is idempotent — re-upsert of an existing record (same `run_id`) produces the same result.
