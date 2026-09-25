# BUG-477: CP-66 audit knowledge update is fire-and-forget with no durable retry

## Metadata

- Document ID: `BUG-477`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `code-confirmed; crash-window reproduction pending`
- Feature Keys: `living-knowledge-base`
- Parent Documents: `CP-66`, `SS-09`, `SD-17`, `SD-22`
- Affected Area: `internal/runner/knowledge_bootstrap.go`

## Summary

After an audit node completes, `updateKnowledgeForAudit` launches
`go UpdateAsync(...)` and records no durable intent. If the runner exits after
audit completion but before the goroutine writes the update, the knowledge base
remains stale indefinitely. On next boot bootstrap is skipped because the
knowledge directory already exists, and there is no pending-update record to
replay.

## Evidence

- `onAuditNodeCompleted` returns immediately after `go UpdateAsync`.
- No session field, queue, ledger or dirty marker records the changed paths.
- `ensureKnowledgeBaseAsync` returns immediately when `knowledge.Missing` is
  false, so restart does not heal a stale existing base.
- Tests prove non-blocking behavior and synchronous worker correctness, but do
  not kill/restart between audit completion and write.

## Expected vs Actual

- Expected: audit-approved code eventually updates the living knowledge base,
  including across runner kill/restart.
- Actual: the update is best-effort in one process and can be lost permanently.

## Impact

Planner/scout context can describe pre-audit architecture after code has
successfully landed. This is derived-state corruption rather than source-code
data loss, but it can misdirect later plans and reviews.

## Required Fix Contract

1. Persist a bounded workspace dirty-set/update intent before acknowledging the
   audit hook, or make boot deterministically detect stale knowledge.
2. Replay under a per-workspace lease; merge/coalesce paths safely.
3. Knowledge failure remains non-fatal to the completed flow.
4. Corrupt intent/index fails closed to full rebuild, not silent freshness.

## Required Tests

- RED crash after durable dirty marker and before update write; restart replays.
- Multiple audit completions coalesce without dropping paths.
- Failed update remains retryable after restart.
- Existing non-blocking audit completion behavior remains intact.
- Live SIGKILL immediately after audit DONE, then verify eventual refresh.

## Implementation Plan

### P-1 — Durable dirty intent

- Add a versioned per-workspace knowledge-update ledger under `.flowpilot/`
  using atomic append/write-rename semantics.
- Before the audit completion hook returns, append changed concrete code paths
  plus a monotonic intent ID. This write must be small and bounded; GitNexus
  work remains asynchronous.
- Coalesce duplicate paths while preserving intents not yet committed.

### P-2 — Leased worker and commit

- Replace naked `go UpdateAsync` ownership with a per-workspace worker that
  claims pending intents, runs incremental distill and marks them committed
  only after knowledge files/index are atomically written.
- On failure leave intents pending and retry with bounded backoff.
- Keep flow/audit status independent: knowledge failure logs/alerts but never
  changes the already-approved code result.

### P-3 — Boot reconciliation

- On workspace bind, inspect the ledger even when the knowledge directory
  exists. Replay pending intents before declaring knowledge current.
- Corrupt/version-unknown ledger or index triggers a safe full rebuild and
  retains evidence; never silently discard pending paths.
- Clean committed ledger entries with crash-safe compaction.

### P-4 — Tests and live kill matrix

- Add subprocess barriers: after dirty-intent fsync, before distill, after
  knowledge write and before intent commit.
- Kill at each barrier, restart and assert eventual correct index/sections.
- Verify simultaneous audit completions serialize and coalesce.

## Definition of Done

- [ ] Audit completion durably records knowledge work before returning.
- [ ] Kill at every worker boundary loses no changed path.
- [ ] Restart replays pending work even when knowledge files already exist.
- [ ] Failed update remains retryable without blocking the completed flow.
- [ ] Corrupt/version-incompatible ledger fails closed to rebuild/repair.
- [ ] Concurrent audit updates do not overwrite or omit each other.
- [ ] Token bounds and planner/coder profile behavior remain unchanged.
- [ ] Live SIGKILL drill proves eventual refresh and records artifact hashes.
