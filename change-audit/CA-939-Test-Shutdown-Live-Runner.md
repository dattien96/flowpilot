# CA-939 — BUG-377: TUI shutdown test hit the real :4317 port

## Summary

`TestCmdShutdownAndQuit_SkipsKillWhenRunnerReused` executed
`cmdShutdownAndQuit()` against a hardcoded `http://127.0.0.1:4317` — a real
POST `/system/shutdown` that killed the live dev runner during CP-84 live
validation (idle runner accepted the unauthenticated drain). Plausible
mechanism for the CP-81 "suddenly out" report: any `go test` sweep can
terminate a runner with no registered leases.

Fix: point the test at an `httptest` stub (sibling pattern already used by
`quit_kills_reused_runner_test.go`). Assertion unchanged — hygiene, not
weakening. Runner-side idle-shutdown contract left intact (unmanaged/legacy
compat, fenced once a lease is registered).

## Verified

- `go test -run TestCmdShutdownAndQuit ./internal/tui/app/` — 4 tests green.
- Audited all `4317` refs in `internal/tui/app/*_test.go`: this was the only
  test executing a network-issuing Cmd against the live URL.

## Files

- `internal/tui/app/bugfix_regression_test.go`,
  `requirements/09-BugFix/todo/BUG-377-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: runner-lifecycle
source_doc_id: BUG-377
change_type: bugfix
