# BUG-484: transient dispatch-store failure during boot recovery has no in-process retry

## Metadata

- Document ID: `BUG-484`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `code-confirmed; outage-recovery tests green (CA-974)`
- Feature Keys: `agent-flow-engine`
- Parent Documents: `CP-51`, `SD-24`, `SD-25`, `SS-17`
- Related Documents: `BUG-475`, `B-51-5`
- Affected Areas: `dispatch_live.go`, `dispatch_recovery.go`

## Summary

Production dispatch recovery runs once at server boot. If enumeration or an
individual reconciliation fails because the dispatch store is temporarily
unavailable, the error is logged and the scan ends. There is no periodic or
backoff retry after the store recovers. Non-terminal work can remain stranded
until another process restart.

## Evidence

- `ScanDispatchRecoveryOnBoot` calls `ScanAllRecoverable` once and logs failure.
- `ScanAllRecoverable`/`ScanRun` log per-run/per-record errors and continue.
- No timer, durable retry intent, or store-recovery callback schedules another
  scan.
- `drivePendingSettlesOnBoot` similarly returns after `ListRecoverable` error.
- CP-51 INV-5 explicitly requires transient storage outage recovery without a
  permanent wedge.

## Expected vs Actual

- Expected: transient outage retries with bounded backoff and leases until each
  record reaches terminal, safely-retryable dispatch, or explicit attention.
- Actual: recovery opportunity is tied to process boot; a failed pass can leave
  records untouched indefinitely in the running process.

## Impact

Prepared/send-claimed turns may never redispatch, sent states may never become
`uncertain/cancel_required`, and terminal settle phases may remain pending.
This is loss of liveness, not permission to blindly resend.

## Required Fix Contract

1. Add bounded backoff/retry driven by durable record state and recovery leases.
2. Recheck Stop state before every provider action on every retry.
3. Avoid tight loops and duplicate scanners; one owner lease per record.
4. Surface repeated store failure as operator-visible repair/health state where
   possible.

## Required Tests

- RED boot list failure then store recovery drains without process restart.
- Per-record failure retries only that record and respects lease ownership.
- Settle-list failure later resumes settle driver.
- Stop arriving during retry prevents provider send.
- Full outage→recover live test against the configured durable store.

## Implementation Plan

### P-1 — RED outage/recovery harness

- Add a DispatchStore wrapper that fails `ListRecoverable`, `Get`, claim and
  settle reads for a controlled number of calls, then becomes healthy.
- Start production recovery wiring once; assert current code leaves records
  untouched after health returns unless process restarts.
- Cover prepared, send-started/cancel-requested and terminal settle-pending.

### P-2 — One bounded recovery coordinator

- Add a service-owned recovery loop with cancellation tied to runner lifecycle.
- Trigger at boot and on retryable store errors; use exponential backoff with
  jitter and a cap. Coalesce triggers so only one enumeration loop runs.
- `RecoveryScanner` leases remain the per-record ownership authority; coordinator
  never sends directly.

### P-3 — Correct per-record retry

- Classify stale lease/concurrent claim as benign skip, transient store errors as
  retryable, and semantic/corrupt state as repair-required.
- Before every provider call, reload under lease and recheck own-run Stop,
  parent fence and `CancelRequested` exactly as current scanner requires.
- Settle driver shares the coordinator trigger but keeps independent durable
  settle phases.

### P-4 — Observability and shutdown

- Expose bounded health counters: last successful scan, pending recoverable
  count if known, consecutive failures and next retry time.
- Shutdown cancels timers/goroutines cleanly. Never log prompt/envelope contents.

### P-5 — Live outage drill

- Make the configured store unavailable at boot, restore it without restarting
  runner, and observe prepared redispatch plus cancel-required repair opening.
- Resolve repair and verify no duplicate provider send.

## Definition of Done

- [ ] One boot call eventually recovers after transient store restoration.
- [ ] Backoff is bounded, jittered and has no duplicate scanner loops.
- [ ] Stop/fence recheck precedes every retried provider action.
- [ ] Prepared work redispatches safely; sent-unknown becomes explicit attention.
- [ ] Settle-pending records resume to finalized after store recovery.
- [ ] Corrupt/non-retryable state fails closed to repair, not infinite retry.
- [ ] Shutdown leaves no recovery goroutine/timer leak.
- [ ] Crash matrix, B-51-5 and `go test -race ./internal/runner` pass.
- [ ] Live outage→recovery evidence proves no restart and no duplicate send.
