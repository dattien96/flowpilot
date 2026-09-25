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

- First iteration latched `persistErr` in `commitLine` — reverted because
  it broke the designed retryable contract for commit-before-mutate paths
  (BUG-289/447/448; `TestBug449` asserts a clean retry after transient
  fsync failure).
- Final approach: converted the remaining ~20 mutate-first mutators to the
  disk-before-RAM template — build post-state on a clone, `commitLine`,
  apply to memory only on success; `uncommitAudit` rolls back the seq/
  audit allocation on failure. Converted: `CreatePrepared`, `casAdvance`,
  `CASAdvanceSettle`, `ClaimRecovery`, `SetCancelRequested`,
  `RequestRunStop`, `ReleaseRunStopFence`,
  `CommitRecoveryUnknownOrRequireCancel`, `ClaimRecoveryAttach`,
  `EnterRecoveryAttach`, `RecordRecoveryAttachedEffect`, `commitPreSend`,
  `ResolveUncertain`, `RetryAsNew`, `RecordEffectDone`,
  `CreateReleaseManifestItem`, `CommitReleaseManifestItem`,
  `SuppressReleaseManifestItem`, `OpenRepair`, `BeginRepairResolution`.

`apps/local-runner/internal/runner/interactive_service.go`:

- `persistProviderSession` logs the durable-write error once before
  returning it — the ~22 callers that discard the result now leave a
  diagnostic trace instead of a fully silent drop.

## Tracked-not-fixed

- ~22 `_ = s.persistProviderSession(...)` discard sites (mostly
  reduced-authority/fail-safe direction: disk under-claims, restart
  re-derives a conservative state) — now logged centrally; a per-site
  `persistOrMarkDegraded` sweep is still proposed in the BUG-499 doc.

## Tests

`bug499_persist_ordering_test.go`: session-store write-fail → memory
clean; rewrite-fail → memory intact; missing-file delete → memory dropped;
healthy round-trip; dispatch create failure → no memory residue + retry
succeeds; dispatch advance failure → revision/state unchanged + retry at
the same revision succeeds. `bug447_449_durability_test.go` unchanged and
still green (retryable contract preserved).
