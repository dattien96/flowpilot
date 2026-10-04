# CA-1168 — run-2062497 D5: Devin remote-session abort on terminal transition

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- `stopAgentLoop` collects the Devin run IDs it terminalizes (cancelled
  child legs + the parent when it is itself a Devin leg) and calls
  `abortDevinRemoteSession` for each after durable stop handling.
- New `abortDevinRemoteSession(runID)`: reads the leg's
  `realProviderSessionID`/`providerSessionID` + account under the service
  lock, returns for non-Devin providers or synthetic ids, then on a
  bounded (30s) goroutine sends ACP `session/cancel` through any live
  `devin acp` process in `Runner.devinProcesses` — lazily starting one from
  the leg's account launch env when none exists. Failures log; the Stop
  response never blocks.

`apps/local-runner/internal/runner/interactive_resume.go`:

- `reconstructRunInternal` calls the same helper when a leg persisted
  `running` (the runner process died mid-turn) reconstructs into a terminal
  status — the remote backend-owned session may still be alive even though
  no local ctx or process remains to carry the ctx-Done cancel.

Live repro: `run-2062497-tournament` stamped cancelled at 02:56 while its
Devin ACP session kept writing into the live workspace until ~20:19 —
local ctx cancellation cannot reach a session the backend owns after the
local process is gone.

## Invariant

A leg our store calls terminal must not keep a live backend session writing
— cancellation is best-effort across every terminalization seam (graceful
stop, restart reconstruction), pinned to the leg's recorded real session id
and account.

## Tests

`bugf_cluster_settle_resume_test.go` —
`TestRun2062497_CancelledDevinLegAbortsRemoteSession` (a recording
`devinDispatcher` asserts the `session/cancel` frame carries the leg's
session id) and `TestRun2062497_AbortSkipsNonDevinAndSyntheticSessions`.
