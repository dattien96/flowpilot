# BUG-492 — Drive restore session reads swallow errors → runID collision clobber + remote-downgrades-local regression

- Status: `todo`
- Severity: **medium-high** — two `GetProviderSession` reads in
  `chat_session_sync.go` treat a store error as "record not found", which
  can overwrite an existing durable record or resurrect a BUG-091 class
  metadata downgrade.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

**D1 — collision check blind (~chat_session_sync.go:1374)**

Restore derives a fresh local runID per manifest leg, collision-checking
each candidate:

```go
_, found, _ := reader.GetProviderSession(ctx, candidate)
```

Store error → `found=false` → the candidate is accepted as "free" while a
record with that runID may exist on disk → the subsequent
`UpsertProviderSession` **clobbers** the existing record.

**D2 — `localAhead` metadata merge blind (~chat_session_sync.go:1806)**

When the local rollout is ahead of the remote snapshot, the restore re-reads
the local record to keep local conversation metadata (BUG-091):

```go
if local, found, _ := reader.GetProviderSession(ctx, localRunID); found {
    session.LastPrompt = local.LastPrompt ... // preserve local fields
}
```

Store error → `found=false` → local preservation skipped → the older
remote metadata **downgrades the local record** — the exact regression
BUG-091 fixed, re-opened whenever the store hiccups during restore.

## Blast radius

- D1 requires generated-id collision + store fault simultaneously — narrow
  window, but the outcome is silent destruction of a durable record.
- D2 requires store fault during a restore-with-local-ahead — wider;
  symptom is subtle metadata regressions (wrong LastPrompt/Status/UpdatedAt)
  that look like sync bugs.

## Fix contract

1. Collision check: `GetProviderSession` error → abort the candidate /
   propagate — restore must not mint an id while existence is unprovable.
2. `localAhead` merge: read error → abort the restore write (fail-closed:
   cannot verify local state → do not write remote-derived state over it).
   The restore is retryable; a clobbered record is not.
3. Keep genuine `!found` (clean read, no record) semantics unchanged.

## Required tests (RED first)

- `TestBUG492_RestoreCollisionCheckStoreErrorAborts`: reader error →
  derived-id allocation aborts/retries instead of clobbering.
- `TestBUG492_RestoreLocalAheadReadErrorAbortsWrite`: localAhead=true +
  reader error → no `UpsertProviderSession` issued.
- Positive controls: real not-found → candidate accepted; healthy local
  read → local fields preserved.

## Definition of Done

- No restore path writes when the local authority is unreadable.
- Tests green, CA entry, commit.
