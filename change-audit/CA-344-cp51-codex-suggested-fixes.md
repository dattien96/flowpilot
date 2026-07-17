# CA-344: CP-51 — implement Codex-review-suggested fixes (Stop fail-closed + recovery enumeration/redispatch)

## Scope

Implement the concrete code fixes from the 2026-07-17 Codex review (CA-343): Task-249's Stop path was not fail-closed, and Task-250's recovery scanner had two deeper gaps than "no boot caller" — wrong enumeration source and no real redispatch action. Also fixes a genuine root-cause bug the review's `Close()` diagnosis pointed at.

## Changes

### `multiProjectDispatchStore.Close()` (P0.5)
- `apps/local-runner/internal/runner/dispatch_store_open.go` — added `Close()` iterating `byProject`, closing every child `localDispatchStore` (releases its flock). Previously the hub had no `Close` at all while its children did, so a caller closing "the store" never released file locks.
- Fixed `TestCrashMatrix_MultiProjectShardAndExportImport` and `TestDispatchStore_ContractSuite/multi-project-local` (both add `defer hub.Close()`), which were failing on Windows with `TempDir RemoveAll cleanup: unlinkat … dispatch.lock: used by another process`.

### Task-249 — Stop fail-closed
- `apps/local-runner/internal/runner/dispatch_live.go` — `requestRunStopV2` now returns `error` and fails closed on `GetRunStopState`/`RequestRunStop` failures (the two calls establishing the durable INV-3 fence); `ListRecoverable`/`SetCancelRequested` failures remain logged-only (courtesy sweep on already-claimed records, not the CAS-level fence).
- `apps/local-runner/internal/runner/interactive_service.go` — `stopAgentLoop` now tracks `dispatchStopFenceErr` and returns `apiErr{code:"dispatch_stop_fence_failed"}` (HTTP 500) if the durable fence could not be written, instead of reporting unqualified success.
- New tests: `TestLiveStop_FailClosed_ReturnsErrorWhenDurableFenceCannotBeWritten`, `TestStopAgentLoop_FailClosed_ReportsErrorWhenDurableStopFenceFails` (end-to-end, via a `RequestRunStop`-failing store wrapper).

### Task-250 — recovery enumeration + redispatch
- `apps/local-runner/internal/runner/dispatch_recovery.go`:
  - `ScanAllRecoverable` now calls `Store.ListRecoverable(ctx, "")` (already existed, returns every non-terminal record across every run) instead of `ListAttention` (which only surfaces `DispatchUncertain`) — prepared/send_claimed/send_started records are no longer invisible to boot recovery.
  - New `RecoveryScanner.EnsureLiveAndRedispatch` hook (nil-safe), called from `reconcileOne` for prepared/send_claimed records with no pending cancel.
- `apps/local-runner/internal/runner/dispatch_live.go` — new `InteractiveService.ScanDispatchRecoveryOnBoot(ctx)` (wires `EnsureLiveAndRedispatch` to `ensureLiveAndRedispatch`) and `ensureLiveAndRedispatch` (reconstructs the run into RAM via the existing `loadPersistedRun` if not already live, then calls the existing `flushDurableTurnIntents` — reusing `startTurn`'s own durable-idempotency-key relaunch logic rather than inventing new provider-launch plumbing).
- `apps/local-runner/internal/cli/root.go` — wired `go interactive.ScanDispatchRecoveryOnBoot(ctx)` at server boot, right after `SetDispatchStore`, mirroring the existing `ScanPersistedChatsForSummaries` best-effort pattern.
- New `apps/local-runner/internal/runner/dispatch_recovery_test.go` (4 tests): enumeration across all non-terminal states, redispatch trigger, cancel-requested skip, and an end-to-end reconstruct+redispatch proof for a run with no live RAM state (simulated process restart).

## Deliberately NOT done in this change (documented in Task-250 §8)

- Provider reconciliation (T-4): no adapter exposes query-by-operation-id for any enabled provider (Task-257 evidenced), so `send_started`/`provider_accepted` correctly still falls through to `CommitRecoveryUnknownOrRequireCancel` — this is the correct behavior, not a gap, but the attach/reconcile machinery for a future evidence-backed provider remains uncalled.
- `effectiveStopRequested` / parent-fence authority in recovery — still only `PreSendStopSelf`.

## Verification

- `go build ./...` clean; `go vet ./internal/runner` clean.
- Targeted new tests all pass (8 Stop-fence tests, 4 recovery tests, 2 store-Close tests).
- Full `go test ./internal/runner/...` : 19 failures remain, all pre-existing/environment-dependent (Windows path assumptions, live Codex/Grok CLI dependent tests, skill-merge precedence, git-guard-shim) or flaky under parallel load (confirmed pass in isolation) — zero regressions from this change; net improvement of 3 previously-failing tests now green (`TestCrashMatrix_MultiProjectShardAndExportImport`, `TestDispatchStore_ContractSuite`, `TestStopAgentLoopCancelsParentTurn` from CA-342).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Fail-closed live Stop (Task-249); fix recovery boot enumeration + real redispatch for prepared/send_claimed (Task-250); add multiProjectDispatchStore.Close()
# --->8---
