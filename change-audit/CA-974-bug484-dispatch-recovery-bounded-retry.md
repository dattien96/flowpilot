# CA-974 — BUG-484: bounded in-process retry for boot dispatch recovery

Date: 2026-09-25
Branch: cp_live_test
Scope: `apps/local-runner/internal/runner`

## Problem

`ScanDispatchRecoveryOnBoot` ran exactly once at server start. A transient
dispatch-store outage (enumeration `ListRecoverable` failure, or a
per-record claim/get/commit failure mid-pass) was logged and abandoned —
non-terminal records stayed prepared/send_claimed/send_started and
settle-owed terminals stayed pending until the next process restart
(CP-51 INV-5 wedge).

## Fix — single-flight bounded retry coordinator

- `ScanDispatchRecoveryOnBoot` is now a coordinator: it runs
  `dispatchRecoveryPass` (scanner + settle drive) and retries on failure
  with exponential backoff (`dispatchRecoveryRetryBase` 500ms, doubling,
  jittered ±25%, capped at 30s) for at most `dispatchRecoveryMaxAttempts`
  (8) passes. `ctx.Done()` aborts the loop promptly; a permanently failing
  store gives up loudly instead of hanging the boot path.
- `recoveryScanRunning atomic.Bool` on `InteractiveService` is the
  single-flight guard — no duplicate scanner loops.
- `RecoveryScanner` remains the per-record ownership authority: the
  coordinator never sends; each retry re-runs claim + lease +
  Stop/CancelRequested recheck inside `reconcileOne` (unchanged), so a Stop
  arriving during the outage still preempts provider effects.
- Error propagation (previously swallowed): `ScanRun` returns an aggregate
  error when any `reconcileOne` fails; `ScanAllRecoverable` aggregates
  per-run failures; `drivePendingSettlesOnBoot` returns the enumeration
  error (callers ignoring the return still compile). Concurrent-claim skips
  stay benign (lease ownership respected, not an error).
- Backoff knobs are package vars so tests shrink the window without
  sleeping the suite.

## Tests

`bug484_recovery_retry_test.go` (red before fix — no retry existed):

- `TestBUG484_BootListFailureRetriesUntilRecovered` — 3 transient
  enumeration failures then heal → send_started still reaches
  `DispatchUncertain` without a restart.
- `TestBUG484_PerRecordFailureRetriesThatRecord` — a one-shot `Get`
  failure mid-reconcile marks the pass failed; retry reconciles.
- `TestBUG484_RetryIsBounded` — permanent outage exits after the attempt
  cap (no hang).
- `TestBUG484_CtxCancelStopsRetry` — cancellation stops the loop.

## Verification

- `go test -count=1 -run 'TestBUG484' ./internal/runner/` — pass
- `go test -count=1 -run 'Dispatch|Recovery|Settle|ScanAll|CP51|BUGG' ./internal/runner/` — pass
- Full suite `go test -count=1 ./...` — env-baseline reds only.
