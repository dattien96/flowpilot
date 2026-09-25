# CA-998 — BUG-499: stores commit memory only after durable write

## What changed

`apps/local-runner/internal/runner/local_file_session_store.go`:

- `UpsertProviderSession`, `UpsertQuestion`, `UpsertApproval`,
  `AppendEvent` (flow-sidecar branch): durable append now precedes the
  in-memory commit — a failed write leaves memory unmutated.
- `DeleteProviderSession`: tmp+rename rewrite lands before the memory
  delete — a failed rewrite can no longer resurrect the record on restart
  while the live process forgot it. Missing-file delete still drops the
  memory entry.
- New `deleteFromMemory` helper; lock order `s.mu → fakeWorkflowStore.mu`
  consistent across paths — no inversion.

`apps/local-runner/internal/runner/dispatch_store_memory.go`:

- `commitLine` latches the first persist failure into `persistErr`; every
  later mutation fails fast ("dispatch store degraded") instead of growing
  a ledger the restart will silently lose.
- `CreatePrepared`'s create-if-absent fast path checks the latch before
  reporting success — a never-persisted record can no longer replay as
  durable.

## Tracked-not-fixed

- ~22 `_ = s.persistProviderSession(...)` discard sites (mostly
  reduced-authority/fail-safe direction) — `persistOrMarkDegraded` sweep
  proposed in the BUG-499 doc.
- ~14 idempotent early-return paths in other dispatch mutators still
  report success on a latched store — mechanical follow-up.

## Tests

`bug499_persist_ordering_test.go`: session-store write-fail → memory
clean; rewrite-fail → memory intact; missing-file delete → memory dropped;
healthy round-trip; dispatch latch → second mutation fails without
touching persist.
