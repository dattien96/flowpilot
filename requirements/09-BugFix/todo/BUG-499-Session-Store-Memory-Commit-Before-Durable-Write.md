# BUG-499 — session store commits memory before durable write; systemic `_ = persist` sites discard failures

## Status
partial — ordering bugs fixed; systemic discard-persist pattern classified, remaining sites tracked below

## Severity
High (ordering) / systemic (discard sites)

## Finding 1 — write ordering (fixed)

`localFileSessionStore.UpsertProviderSession` mutated the in-memory map
**before** marshalling and appending to `sessions.ndjson`. A marshal or
append failure left memory diverged from disk — the live process believed
the run persisted; restart silently dropped it.

`DeleteProviderSession` was worse: memory delete ran before the atomic
rewrite. A failed rewrite meant the record **resurrects on restart** while
the live process already forgot it — silent undelete.

**Fix:** both methods now do the durable operation first and commit to the
in-memory map only on success. Missing-file delete still drops the memory
entry (nothing on disk to resurrect). Lock order `s.mu →
fakeWorkflowStore.mu` is consistent across both paths — no deadlock.

Same ordering fixed in `UpsertQuestion`, `UpsertApproval`, `AppendEvent`
(flow-sidecar branch) — durable append precedes the memory commit.

## Finding 1b — dispatch ledger ordering (fixed)

`memoryDispatchStore` mutators (~20 sites) mutated memory **before**
`commitLine` (the `afterCommit` hook → `persistLine` fsync append). A
persist failure left memory ahead of disk permanently: a retry hit
`ErrStaleDispatch`/`ErrAlreadyExists` on state the log never recorded, and
the create-if-absent fast path reported *success* for a record that never
reached disk.

An earlier iteration latched `persistErr` in `commitLine` and made every
later mutation fail closed. That broke the designed **retryable** contract
(`TestBug449_FailedTerminalCommitDoesNotBurnSeq`: commit-before-mutate
paths — BUG-289/BUG-447/BUG-448 — must retry cleanly after a transient
fsync failure), so the latch was reverted.

**Fix (final):** every remaining mutator now follows the BUG-289/447/448
disk-before-RAM template — build the post-state on a clone (`cloneRec`,
`*st`/`*item`/`*r` repair copies), `appendAuditLocked`, `commitLine`, and
apply to memory only on success (`*r = cp`, map inserts, `clearIntentLocked`,
`setLiveIntentLocked`, `s.resolutions[…]` all move after the durable line
lands). On failure `uncommitAudit` restores `seq`/`audits` so no phantom
audit or burned seq survives — the operation is fully retryable.

Converted sites: `CreatePrepared`, `casAdvance` (CASAdvance /
CASRecoveryAdvance), `CASAdvanceSettle`, `ClaimRecovery`,
`SetCancelRequested`, `RequestRunStop`, `ReleaseRunStopFence`,
`CommitRecoveryUnknownOrRequireCancel`, `ClaimRecoveryAttach`,
`EnterRecoveryAttach`, `RecordRecoveryAttachedEffect`, `commitPreSend`,
`ResolveUncertain`, `RetryAsNew`, `RecordEffectDone`,
`CreateReleaseManifestItem`, `CommitReleaseManifestItem`,
`SuppressReleaseManifestItem`, `OpenRepair`, `BeginRepairResolution`.
(`commitTerminal`, `CommitReceiptAndClearIntent`, `CommitRepairResolution`
were already disk-first from BUG-289/447/448.)

## Finding 2 — systemic `_ = s.persistProviderSession(...)` (classified, tracked)

~22 call sites discard the persist error
(`interactive_resume.go` :1263/:1402/:1560/:1630/:1896-1947,
`interactive_service.go` :4293/:4675/:5331/:5433/:8525/:9245/:10029/:10062/:10101,
`interactive_service.go` :4507 is additionally **async** `go func`).

Direction analysis: most of these persist *reduced* authority states
(status transitions, pending notes, context) where a lost write leaves
disk claiming **less** progress than memory — restart re-derives a safe
conservative state (re-park, re-wait). The dangerous direction — disk left
claiming **more** (leg `active`, gate answered, binding healthy) — was the
BUG-489/490 class and is fixed.

**Remaining work (not yet implemented):** convert each discard site to a
`persistOrMarkDegraded`-style helper that logs + marks the run degraded
(using the `transitionLogDegraded` precedent), prioritized by direction:

- HIGH direction (disk over-claims): any site persisting a *closed/stopped/
  decided* state — mostly covered by BUG-489/490 fixes.
- LOW direction (disk under-claims): status/context notes — recoverable on
  restart, currently silent. Add degrade-logging.

## Tests

- `bug499_persist_ordering_test.go` — write-failure → memory clean;
  rewrite-failure → memory intact; missing-file delete → memory dropped;
  healthy round-trip unchanged; dispatch create failure → no record/
  envelope/audit in memory + retry succeeds; dispatch advance failure →
  revision/state unchanged + retry at same rev succeeds.
- `bug447_449_durability_test.go` — existing BUG-447/448/449 retryable
  contracts still pass unchanged (no test weakened).
