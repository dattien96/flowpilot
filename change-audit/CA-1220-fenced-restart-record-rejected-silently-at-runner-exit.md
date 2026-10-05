# CA-1220 — Fenced restart record rejected silently at runner exit (second stack teardown)

- **Area**: supervisor lifecycle (scripts/supervisor.js) + drain-side
  fenced command writer (internal/cli/root.go)
- **Evidence**: live 23:11 — `/system/restart` accepted
  (`restart_9982385d…`), drain completed, fenced `supervisor.cmd` was
  written by the runner (no write-failed log), the runner exited ~200ms
  later, and the supervisor logged `Runner exited unexpectedly …
  shutting down stack` — full teardown of runner + desktop + web for the
  second time tonight. CA-1217 made the exit handler read the fenced
  record before declaring the death unexpected; the handler still fell
  through — the record was either absent, malformed, or failed
  `validateSupervisorCommand`, and **no log line exists anywhere in the
  stack for which of those happened** (the exit path logged nothing and
  the command poll didn't tick inside the ~200ms write→exit window).
- **Root cause candidates the code could not distinguish before this
  change**:
  1. `unverifiable_instance` — the adopt path
     (`--restart-existing`) learned `currentRunnerInstanceId` via a
     *single* fire-and-forget `/health` fetch with no retry; one failed
     fetch leaves it `null` forever and every fenced record fails
     validation.
  2. `missing_instance` — the writer side stamps
     `RunnerInstanceID: mgrIdentity(mgr)`, which returns `""` when the
     manager snapshot is unavailable; an empty fence makes the record
     dead-on-arrival with zero diagnostics on either side.
  3. `__invalid__` — a torn/malformed control file fails the restart
     action check and fell straight into the unexpected-death branch
     with no log.
- **Fix**:
  - Adopt path now retries `refreshRunnerInstanceId()` (60 × 500ms,
    bounded by `isExiting`) — same loop the fresh-spawn path already
    had; closes the never-learned-instance window.
  - Exit handler logs the validation verdict
    (`reason`, `writtenBy`, `known`) when a fenced restart record is
    present but rejected, and logs when a control record exists that is
    not a restart action at all — the next recurrence names its own
    reason instead of looking identical to "no command written".
  - `runSystemDrain` logs when the fenced record is written with an
    empty `runnerInstanceId` (manager snapshot unavailable) so the
    writer-side failure is visible in the drain log.
- **Tests**: `.phase1-tests` supervisor suite 11/11 green (planned-exit
  respawn, no-ghost-restart, fenced-expiry paths all unchanged — the
  additions are log-only + the adopt retry loop).
- **Follow-up**: this CA ships diagnostics + the most probable concrete
  fix (adopt retry). If a future drain still tears down the stack, the
  new warn line carries the exact validation reason — capture it into
  the next CA rather than reasoning blind.
