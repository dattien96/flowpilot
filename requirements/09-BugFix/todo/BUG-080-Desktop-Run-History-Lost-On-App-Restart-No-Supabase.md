# BUG-080: Desktop Run History Lost On App Restart (No Supabase)

## Metadata

- Document ID: `BUG-080`
- Title: `Desktop Run History Lost On App Restart (No Supabase)`
- Phase: `bugfix`
- Status: `open`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-056: History Supabase Reader Production Fix](../../08-Task/done/Task-056-History-Supabase-Reader-Production-Fix.md), [BUG-060: Desktop Run History Empties After Switching Runs](./BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `desktop, history, local-runner, persistence, restart`

## AI Quick View

### Summary

- When the desktop app restarts (runner and/or app-server process re-created), the History panel shows nothing — all previous runs are lost.
- The actual conversation data (Claude session files, Codex workspace files) is still present on disk in each provider's own storage. What is lost is the runner's **history index**: the `ProviderSessionState` records (runId, projectId, lastPrompt, status, timestamps) that `projectRunHistory` reads to populate the sidebar.
- Task-056 (commit 48db8f0) fixed this for the **Supabase path**: `SupabaseWorkflowStore.ListProviderSessionsByProject` now reads `workflow_provider_sessions` from the database after restart. That fix is only active when Supabase is configured.
- For the **local (non-Supabase) path** — the default for desktop users — `newInteractiveService` falls back to `fakeWorkflowStore`, which is a pure in-memory store. Both `s.runs` (InteractiveService's run map) and `fakeWorkflowStore.sessions` are empty on a new process; history is gone.
- A second problem compounds the issue: `ProviderSessionState` is missing the display fields that `projectRunHistory` populates for in-memory runs (`LastPrompt`, `LastMessage`, `StartedAt`, `UpdatedAt`, `RunKind`). Even if sessions were persisted to disk, restored history items would appear with blank prompt and zero timestamps, sorted incorrectly.

### Current Ask

- Implement a local file-backed session store (`localFileSessionStore`) that satisfies `InteractiveStateStore` + `SessionHistoryReader` and is used when Supabase is not configured.
- Add the five missing display fields to `ProviderSessionState` and re-persist the session on key state transitions so restored items are complete.

### Key Decisions

- `V-1` A new `localFileSessionStore` (not a modification of `fakeWorkflowStore`) that writes `ProviderSessionState` records to a NDJSON file in the OS user cache dir: `os.UserCacheDir()/flowpilot/sessions.ndjson`.
- `V-2` `fakeWorkflowStore` stays as-is — it is used by tests and does not need disk persistence.
- `V-3` `ProviderSessionState` gains: `LastPrompt string`, `LastMessage string`, `StartedAt string`, `UpdatedAt string`, `RunKind string`. These map 1-to-1 to the fields on `interactiveRun` that `projectRunHistory` currently reads only from `s.runs`.
- `V-4` Re-persist the session at two points: (a) when a turn starts (`lastPrompt` / `updatedAt` set), (b) when a turn ends via `finishTurn` (`status`, `lastMessage`, `updatedAt` settled). Not on every event — too chatty.
- `V-5` `NewInteractiveServiceWith` constructs the `localFileSessionStore` when Supabase is not configured and passes it to `newInteractiveService` as the `workflowStore` parameter.
- `V-6` `localFileSessionStore` wraps `fakeWorkflowStore` for the `WorkflowStore` interface methods (step transitions, run status, logs) so `newInteractiveService` always has a complete `WorkflowStore`. The file store adds only the persistence layer on top.

### Constraints

- Do not modify `fakeWorkflowStore` — it must stay pure-in-memory for tests.
- Do not change the `SessionHistoryReader` or `InteractiveStateStore` interface signatures.
- Do not change any desktop or admin-web code — backend Go only.
- `localFileSessionStore` must be safe for concurrent writes (mutex on file operations).
- NDJSON append is preferred over a full-rewrite JSON array to avoid data loss on crash mid-write.

### Open Questions

- Should `os.UserCacheDir()/flowpilot/sessions.ndjson` be the path, or should the desktop app pass a `--session-store` flag to the runner? The flag approach allows the desktop to colocate the file with its own app data dir. **Recommendation: flag with fallback to `os.UserCacheDir()/flowpilot/`** so the desktop can override but vanilla CLI users get a sensible default.
- Should old completed/failed sessions be pruned from the NDJSON file to bound file growth? **Recommendation: prune entries older than 90 days on startup**, keeping the file small.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `newInteractiveService`, `persistProviderSession`, `emitLocked`, `finishTurn`, `finalizeInputLocked`
- `apps/local-runner/internal/runner/interactive_handlers.go` — `projectRunHistory`, `createRun`
- `apps/local-runner/internal/runner/workflow_store.go` — `WorkflowStore`, `InteractiveStateStore`, `SessionHistoryReader`, `ProviderSessionState`, `fakeWorkflowStore`
- `apps/local-runner/internal/runner/supabase_workflow_store.go` — reference implementation of `ListProviderSessionsByProject`
- `apps/local-runner/internal/cli/root.go:97` — `NewInteractiveServiceWith` call site

## 1. Issue Summary

After restarting the desktop app (or the local runner process), the History sidebar panel shows no runs. Runs from the previous session are not visible even though the underlying provider conversation data (Claude session files, Codex workspace files) is still present on disk.

## 2. Parent Links

- direct upstream: [Task-056](../../08-Task/done/Task-056-History-Supabase-Reader-Production-Fix.md) — Task-056 closed the Supabase gap; this bug closes the local-disk gap.
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

Task-056 added `SupabaseWorkflowStore.ListProviderSessionsByProject` which reads from the database — but only fires when the store is a `SupabaseWorkflowStore`. With `fakeWorkflowStore`, the type assertion at `interactive_handlers.go:612` still succeeds (fake store implements the interface), but returns nothing because the in-memory map was cleared.

### R-2 — `ProviderSessionState` missing display fields; session not re-persisted after creation

`persistProviderSession` is called exactly once per run — at creation (`createRun`, `interactive_handlers.go:513`) — with only these fields set:

```go
ProviderSessionState{
    RunID, ProjectID, WorkflowID,
    ProviderSessionID, ProviderKey, ProviderAccountID,
    WorkingDirectory, Status: rs.status (= idle),
}
```

The display fields that matter for the History panel — `LastPrompt`, `LastMessage`, `StartedAt`, `UpdatedAt`, `RunKind` — are never written to `ProviderSessionState`. They only exist on `interactiveRun` in `s.runs`. After restart, `projectRunHistory` reconstructs from `ProviderSessionState` (lines 619-625) but those fields are blank:

```go
// interactive_handlers.go:619-625
out = append(out, runHistoryItem{
    RunID:       sess.RunID,
    ProjectID:   sess.ProjectID,
    WorkflowID:  sess.WorkflowID,
    ProviderKey: sess.ProviderKey,
    Status:      sess.Status,
    // StartedAt, UpdatedAt, LastPrompt, LastMessage, RunKind — all empty
})
```

## 7. Fix Strategy

### F-1 — Extend `ProviderSessionState` with display fields

Add to `ProviderSessionState` in `workflow_store.go`:
- `LastPrompt string`
- `LastMessage string`
- `StartedAt string`
- `UpdatedAt string`
- `RunKind string`

These map to the same-named fields on `interactiveRun`. No interface changes needed.

### F-2 — Create `localFileSessionStore`

New file: `apps/local-runner/internal/runner/local_file_session_store.go`

Implements:
- `WorkflowStore` — delegate all methods to an embedded `*fakeWorkflowStore`
- `InteractiveStateStore` — `UpsertProviderSession` writes/updates the record in memory AND appends a write-ahead entry to `{dataDir}/sessions.ndjson`; other methods delegate to fake
- `SessionHistoryReader` — `ListProviderSessionsByProject` reads from the NDJSON file (merged with in-memory map for freshness)

NDJSON format: one JSON object per line, keyed by `run_id`. On read, load all lines, last-wins per `run_id` (allowing upsert-by-append), filter by `project_id`.

Concurrent write safety: single mutex on file operations.

### F-3 — Re-persist session on key state transitions

Add calls to `persistProviderSession` (with full updated state) at two places in `interactive_service.go`:

1. **`finalizeInputLocked`** (line ~796): after `rs.lastPrompt` and `rs.updatedAt` are set — persist to capture the prompt title.
2. **`finishTurn`** (line ~709): after `rs.status`, `rs.lastMessage`, and `rs.updatedAt` are settled — persist to capture final state.

These are the only two points where the display fields that matter to history change in a durable way. Not called on every event to avoid per-event file I/O.

### F-4 — Wire `localFileSessionStore` into startup

In `apps/local-runner/internal/cli/root.go`, before calling `NewInteractiveServiceWith`:

```go
// Add --session-store flag (or fallback to os.UserCacheDir()/flowpilot/)
// construct localFileSessionStore
// pass via a new NewInteractiveServiceWithStore(registry, catalog, store) or
// expose a setter on the returned InteractiveService before registering routes
```

When Supabase IS configured, use `SupabaseWorkflowStore` as before (unchanged).

### F-5 — Update `projectRunHistory` to populate display fields from restored sessions

At lines 619-625 in `interactive_handlers.go`, expand the `runHistoryItem` construction from `ProviderSessionState` to include the new fields:

```go
out = append(out, runHistoryItem{
    RunID:       sess.RunID,
    ...
    StartedAt:   sess.StartedAt,
    UpdatedAt:   sess.UpdatedAt,
    LastPrompt:  sess.LastPrompt,
    LastMessage: sess.LastMessage,
    RunKind:     sess.RunKind,
})
```

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` → no compile errors after `ProviderSessionState` field additions
- `V-2` `go test ./internal/runner/...` → all existing tests pass (fakeWorkflowStore unchanged)
- `V-3` New unit test `TestLocalFileSessionStoreRoundTrip`: upsert two sessions from two projects, call `ListProviderSessionsByProject` for each, assert correct items returned. Create a new store instance pointing at the same file (simulate restart), call `ListProviderSessionsByProject` again, assert same results.
- `V-4` Manual smoke test: start two runs, quit app, reopen — both runs appear in history with correct title and status.

## 9. Regression Guard

- `fakeWorkflowStore` is not modified — all existing tests continue to use it unchanged.
- The `localFileSessionStore` wraps `fakeWorkflowStore` for `WorkflowStore` methods, so orchestration behaviour is identical.
- NDJSON last-wins merge is idempotent — re-upsert of an existing record (same `run_id`) produces the same result.
