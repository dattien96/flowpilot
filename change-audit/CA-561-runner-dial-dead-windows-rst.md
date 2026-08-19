# CA-561: runnerDialDeadErr recognizes Windows TCP reset so resume retry + total-failure restore work on Windows

## Fix

Two resume-related tests (`TestCmdFocusAgent_ResumeRetriedOnceOnDialError`,
`TestCmdFocusAgent_TotalFailureNoStuckChrome`) were failing on Windows (they
passed on Linux/macOS). Root cause: the `runnerDialDeadErr` classifier only
matched Unix/Go error text (`connection refused`, `connectex`, `dial tcp`,
`connection reset`, `no such host`, `network is unreachable`). Windows reports
an actively reset connection as WSAECONNRESET (10054) surfaced through
`wsarecv`/`wsasend` — "An existing connection was forcibly closed by the remote
host" — which matched none of those substrings.

Consequences on Windows, not just for tests:
- `resumeChildRunForFocus` never retried a transient dial/reset failure, so the
  CA-517 supervisor-restart/port-handoff recovery silently did nothing.
- `cmdFocusAgent` classified a reset as a server-side resume failure and fell
  back to the live-only stream, which serves nothing for a dead runner → empty
  stuck child chrome instead of restoring the main transcript with an error.

Fix: add `forcibly closed by the remote host` to `runnerDialDeadErr`. It is the
Windows spelling of "connection reset by peer" (already in the set) — an active
reset, never a slow response. Windows connect/i-o timeouts (WSAETIMEDOUT
"did not properly respond…", `i/o timeout`) remain classified as slow per
CA-514, so the slow-vs-dead split is preserved.

## Where

`apps/local-runner/internal/tui/app/step_runtime.go` — `runnerDialDeadErr`
gains the Windows RST pattern; `runnerUnreachableErr` inherits it (poll-pause
accounting also counts a reset runner as unreachable).

## Tests

New (additive): `tui_no_freeze_catalog_test.go`
`TestRunnerDialDeadErr_ClassifiesWindowsRST` — wsarecv/wsasend RST strings trip
the classifier, Windows connect-timeout text and `i/o timeout` do not, and the
RST also counts toward the poll pause. The two previously failing resume tests
now pass on Windows; the full `internal/tui/app` suite is green.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-517
change_type: bugfix
summary: runnerDialDeadErr now matches Windows WSAECONNRESET ('An existing connection was forcibly closed by the remote host'), restoring dial-retry and total-failure main-transcript restore on Windows while keeping i/o timeouts classified as slow
# --->8---