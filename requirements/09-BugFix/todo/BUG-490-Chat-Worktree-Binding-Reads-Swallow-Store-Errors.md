# BUG-490 — chat worktree binding lookup + lifecycle-mark swallow store errors → split-brain worktrees / stale-state adoption

- Status: `todo`
- Severity: **high** — two sites in `run_worktree.go` treat a session-store
  error as "no persisted binding/no persisted legs", which can create a
  second worktree for a chat that already has one, or leave persisted legs
  carrying stale lifecycle state.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

**D1 — `findChatWorktreeBindingLocked` (~run_worktree.go:113)**

```go
if rows, err := reader.ListProviderSessionsByChat(ctx, chatID); err == nil {
    for i := len(rows)-1; i>=0; i-- { ...return latest non-lost binding }
}
return nil
```

On store error the lookup returns `nil` = "no binding". The caller
(`interactive_handlers.go:1161`, run-start worktree inheritance) then
provisions a **new** worktree for the chat — while an `active` binding
still exists on disk. Two worktrees for one chat → divergent edits the
merge-back flow cannot reconcile.

**D2 — `markChatWorktreeState` (~run_worktree.go:260)**

```go
if rows, err := reader.ListProviderSessionsByChat(ctx, chatID); err == nil {
    for _, row := range rows { row.WorktreeState = state; _ = persist(row) }
}
```

Both the list error and each row's persist error are dropped. A store
fault leaves persisted legs holding a stale lifecycle value (e.g.
`active` when the real state moved to `discarded`/`merge_pending`) →
D1's lookup later adopts the stale state as authoritative → binds to a
worktree that should not be reused (or skips one that should).

## Fix contract

1. `findChatWorktreeBindingLocked` returns `(*worktreeBinding, error)`.
   On list error → propagate. The run-start caller fails closed: refuse to
   provision a worktree while the binding authority is unreadable (typed
   retryable error, BUG-475 precedent). Never "assume none".
2. `markChatWorktreeState` returns an error (or at minimum logs + degrades
   loudly): list error → caller sees it; per-row persist failure → log +
   continue marking the rest (partial success is better than aborting the
   batch) but the failure must be reported, not dropped.
3. Callers of `markChatWorktreeState` decide per-site: lifecycle transitions
   that gate merge/resolve must propagate; informational marks may log.
   Check each call site before choosing.

## Required tests (RED first)

- `TestBUG490_BindingLookupStoreErrorPropagates`: reader returning error →
  lookup returns error (pre-fix: nil binding) → run-start path fails with
  typed error instead of provisioning a second worktree.
- `TestBUG490_BindingLookupHealthyFindsBinding`: positive control.
- `TestBUG490_MarkStateStoreErrorReported`: list error surfaces to caller.
- `TestBUG490_MarkStateRowPersistFailureLogged`: one row upsert fails →
  other rows still marked, failure reported.

## Definition of Done

- No worktree-binding decision can be made from a provably-partial store
  view.
- Lifecycle-state writes that fail are reported/logged, never invisible.
- Tests green, CA entry, commit.
