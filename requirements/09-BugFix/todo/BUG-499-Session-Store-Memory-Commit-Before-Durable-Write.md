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

## Finding 1b — dispatch ledger ordering (partially fixed)

`memoryDispatchStore` mutations (~28 sites) all mutate memory then call
`commitLine` (the `afterCommit` hook → `persistLine` fsync append). A
persist failure leaves memory ahead of disk permanently — and the
create-if-absent fast path then reported *success* for a record that never
reached disk.

**Fix:** `commitLine` now latches the first persist error into
`persistErr` — every later mutation fails fast with "dispatch store
degraded" instead of growing a ledger the restart will lose.
`CreatePrepared`'s idempotent fast path checks the latch before reporting
success, so a never-persisted in-memory record can no longer masquerade as
durable.

**Tracked residual:** ~14 idempotent/dedupe early-success paths in the
other mutators (`RecordEffectDone`, `CommitReceiptAndClearIntent`,
`ResolveUncertain`, `RetryAsNew`, repair APIs, …) return success without
reaching `commitLine`; on a latched store they still report done for
non-durable in-memory state. Each has a different return signature —
converting them is a mechanical sweep tracked here, deliberately not
bundled into this change to keep the diff reviewable.

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
  healthy round-trip unchanged.
