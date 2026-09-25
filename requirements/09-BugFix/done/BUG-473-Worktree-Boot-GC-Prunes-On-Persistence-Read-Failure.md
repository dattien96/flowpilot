# BUG-473: worktree boot GC treats persistence read failure as “unbound” and may prune live work

## Metadata

- Document ID: `BUG-473`
- Phase: `bugfix`
- Status: `done`
- Severity: `critical`
- Evidence: `code-confirmed; fixed CA-971; live fault-injection pending`
- Fixed By: `CA-971`
- Tests: `internal/runner/bug473_worktree_gc_authority_failure_test.go`
- Feature Keys: `run-worktree`
- Parent Documents: `CP-71`, `SS-23`, `SD-27`
- Related Documents: `BUG-378-Worktree-Committed-Work-Dropped-On-Merge`
- Affected Area: `internal/runner/run_worktree_merge.go`

## Summary

Boot GC must prune only worktrees proven orphaned. `sweepOrphanedWorktrees`
instead treats persistence lookup errors as absence. For a non-resident chat
binding, an error from `ListProviderSessionsByChat` is ignored; the fallback
`GetProviderSession(ownerID)` normally cannot find a chat-owned binding because
`ownerID` is a `chatId`, not a run ID. GC then force-removes the worktree.

This converts “authority unavailable” into “safe to delete,” violating both the
worktree contract and the runner's fail-closed durability rule.

## Evidence

- `run_worktree_merge.go:376-402` sets `bound` only when reads return `nil`
  error and a matching row.
- Neither `ListProviderSessionsByChat` nor `GetProviderSession` error aborts or
  quarantines the sweep.
- `run_worktree_merge.go:404-410` then runs `git worktree remove --force`,
  `os.RemoveAll`, removes sidecars and logs the owner as pruned.
- Existing boot-GC E2E covers healthy persisted bindings and true orphans, but
  not persistence read failure/corruption.

## Expected vs Actual

- Expected: GC deletes only after authoritative stores successfully prove no
  active/resumable/merge-pending binding; read error skips deletion and emits
  repair/diagnostic evidence.
- Actual: a read error can be indistinguishable from “no binding.”

## Impact

Unmerged code in a valid run worktree can be destroyed during runner boot. This
is the same severity class as CP-71's prior live data-loss defects, but at the
recovery/GC boundary.

## Required Fix Contract

1. Any binding-store read error must fail closed for that owner/repository.
2. Distinguish `not found` from `authority unavailable/corrupt`.
3. Never log `pruned` unless removal was both authorized and successful.
4. Consider a repair-required/diagnostic record for repeated failures.
5. Audit multi-project GC separately; the current sweep only scans the attached
   runner workspace.

## Required Tests

- RED: chat-session lookup error preserves the worktree and sidecars.
- RED: run-session lookup error preserves a flow-owned worktree.
- RED: corrupt persisted binding never becomes GC-eligible by zero-value.
- E2E restart with injected store failure, followed by successful retry.
- Live kill/restart with unmerged files and temporary store unavailability.

## Implementation Plan

### P-1 — Fault model and RED tests

- Add `bug473_worktree_gc_authority_failure_test.go` with a workflow-store stub
  that independently fails `ListProviderSessionsByChat` and
  `GetProviderSession`.
- Seed a real worktree containing committed and uncommitted changes plus base
  and patch sidecars.
- Run `sweepOrphanedWorktrees` and assert current code removes it; preserve this
  as the red assertion before changing production.

### P-2 — Three-state binding lookup

- Extract a helper returning `bound | proven_orphan | unknown`, plus error.
- A successful empty lookup from every applicable authority may return
  `proven_orphan`; any read/decode error returns `unknown`.
- Chat owners must be checked by chat lookup; flow owners by run lookup. Do not
  reinterpret a chat lookup failure via a normally-not-found run lookup.
- `unknown` skips every destructive action and emits bounded diagnostic data.

### P-3 — GC execution integrity

- Check errors from `git worktree remove`, `os.RemoveAll`, sidecar removal and
  final `git worktree prune`.
- Log `pruned` only after required cleanup succeeds. Use a distinct
  `gc_deferred`/`gc_failed` diagnostic otherwise.
- Keep candidate worktrees and every active/merge-pending/resumable/lost binding
  excluded exactly as today.
- Review multi-project enumeration: derive repository roots from persisted
  bindings rather than scanning only `runner.workspace`, or explicitly split
  that follow-up if outside the minimal fix.

### P-4 — Recovery and live drill

- Retry deferred owners on a bounded schedule or next authoritative attach;
  never busy-loop at boot.
- Live drill: leave an unmerged worktree, make the store unreadable, restart,
  prove survival, restore store, then prove only a true orphan is pruned.

## Definition of Done

- [ ] Store read/decode failure cannot authorize deletion.
- [ ] Chat-owned and flow-owned bindings have separate positive authority tests.
- [ ] True orphan GC still removes worktree, branch metadata and sidecars.
- [ ] Cleanup failure is not logged or surfaced as successful prune.
- [ ] Active, merge-pending, resumable and lost bindings survive boot GC.
- [ ] Live restart fault drill preserves unmerged content byte-for-byte.
- [ ] Multi-project GC scope is fixed or recorded as a separately numbered BUG.
- [ ] Full CP-71/worktree suite and race-focused runner tests pass unchanged.
