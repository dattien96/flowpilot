---
id: BUG-448
title: Receipt durability failure is masked by in-memory accepted state
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-410, CA-921b, CA-922b]
---

## AI Quick View
- **What**: A failed receipt append can clear the intent in RAM without durably recording the receipt.
- **Why**: Receipt commit mutates memory before persistLine; the new retry sees provider_accepted and returns.
- **Key constraint**: Never turn an uncommitted provider receipt into a terminal/accepted claim or silently discard provider evidence.

## 1. Metadata
- Document ID: `BUG-448`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [CA-922b](../../../change-audit/CA-922-dispatch-durability-fixes.md)

## 2. Symptom and Impact
`CommitReceiptAndClearIntent` changes the dispatch record to `DispatchProviderAccepted`, increments revision, calls `clearIntentLocked` and appends audit **before** `s.commitLine` (`internal/runner/dispatch_store_memory.go:514-558`). For a local store, `commitLine` calls `persistLine` (`dispatch_store_local.go:61,117`). If disk append fails, `turnBridge.Accepted` schedules `retryReceiptCommit` (`dispatch_live.go:463-472`). Its fresh `Get` reads the RAM record as `DispatchProviderAccepted` and immediately returns **without retrying the disk append** (`:599-625`). Restart then sees the previous non-accepted record and intent, although the live runner behaved as though accepted. This is a pre-existing commit-order flaw combined with the CA-922b retry path. `turnBridge.Accepted` currently notes that no enabled Codex/Grok/Claude adapter calls it (`dispatch_live.go:435-438`), so the receipt branch is a **latent future-provider defect**, not a proven live provider regression. Severity: **medium** for receipt correctness; the terminal retry error branch below is active and needs separate fault-injection evidence.

Separately, `retryTerminalCommit` claims to handle transient store errors but returns immediately on non-stale commit error or Get failure (`dispatch_live.go:563-566,590-595`); no evidence-backed terminal proof is persisted/scheduled for future retry. That branch needs independent verification before claiming BUG-410 fully closed.

## 3. Reproduction / Evidence
Inject a single `afterCommit` error on receipt `record` append, then call `retryReceiptCommit`; compare `Get` in-process with replayed durable store. Existing BUG-410 tests cover stale revision on terminal commits, not receipt persist failure. **Static failure-path finding; fault-injection test and live reproduction pending.**

## 4. Acceptance and Verification
Durable write before RAM/intent mutation (or recoverable outbox), plus bounded retry of transient errors and typed uncertain fallback. Add assertion-first tests for I/O failure, stale CAS, stop-fence, restart/replay and different provider bridge callers. Preserve existing tests.

## 5. Resolution (2026-09-23, CA-932b)

- `CommitReceiptAndClearIntent` builds + durably commits the
  `provider_accepted` record (fsync via `afterCommit`) before mutating RAM or
  clearing the intent — retry sees the durable revision, restart recovers the
  receipted state.
- Test: `bug447_449_durability_test.go` — failure injection + in-process retry
  + restart coverage; red before fix.
