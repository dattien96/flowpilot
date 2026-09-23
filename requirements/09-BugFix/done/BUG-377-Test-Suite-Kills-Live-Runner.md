# BUG-377: TUI test POSTs /system/shutdown to the real :4317 — kills live runner

## Metadata

- Document ID: `BUG-377`
- Title: `TestCmdShutdownAndQuit_SkipsKillWhenRunnerReused executes a real /system/shutdown against the default runner port`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `runner-lifecycle`
- Parent Documents: CP-81 (lifecycle lease contract)
- Child Documents: `none`
- Related Documents: CP-81 Task-418 (fenced two-phase shutdown), CA-911/CA-913 (requester attribution)
- Replaces: `none`
- Tags: `tui, lifecycle, shutdown, test-hygiene, live-kill`

## AI Quick View

### Summary

- `internal/tui/app/bugfix_regression_test.go` `TestCmdShutdownAndQuit_SkipsKillWhenRunnerReused` built the model with `New(cfg, "http://127.0.0.1:4317")` and **executed** `cmdShutdownAndQuit()` — a real POST `/system/shutdown` to whatever process is bound to the default port.
- Observed live 2026-09-23: the package test run issued `/system/shutdown` (`xclient="tui"`, `proc=app.test` pid=35019) and killed the dev runner mid-validation (`phase=ready clients=0` → idle path → accepted → drain → exit).
- This is a plausible mechanism for the CP-81 operator report "đang dùng app thì tự nhiên bị out ra": any test sweep (CI, local `go test ./...`, watch loop) can terminate a live runner; with no registered desktop lease the runner treats itself as idle and accepts.

### Constraints

- safe-fix-contract: the test's assertion (`QuitMsg` returned) is preserved byte-for-byte; only the target URL moved to an `httptest` stub — hygiene fix, no weakening.
- The runner-side contract (unauthenticated shutdown accepted when `clients=0 work=0`) is **intentional** for unmanaged/legacy `ShutdownStack` compat — not changed here. With a registered desktop lease the same request would have been 409-fenced.

## 4. Expected vs Actual

- expected: unit tests never perform network mutations against real ports.
- actual: the test POSTed `/system/shutdown` to the live default runner.

## 5. Root Cause

Stale test predating the CP-81 lifecycle contract — sibling tests (`quit_kills_reused_runner_test.go`) already use `httptest.NewServer`; this one was missed.

## 6. Fix

Point the test at an `httptest` stub returning `202 {"status":"accepted"}`. Assertion unchanged.

## 7. Verification

- `go test -run TestCmdShutdownAndQuit ./internal/tui/app/` — green (4 tests incl. httptest-based siblings).
- Audit of all `4317` references in `internal/tui/app/*_test.go`: this was the only test that executes a network-issuing Cmd against the live URL (the rest construct models without running network commands).

## 8. Follow-up (not in scope)

- Consider a defense-in-depth env guard (`FLOWPILOT_TEST=1` → `ShutdownStack` no-ops on default ports) or a guard rejecting unauthenticated shutdown while SSE/mux subscribers are attached. File separately if wanted.
