# CA-920: Desktop Lease Register Retry — Fix Self-Shutdown While App Is Open

## Summary

User report: "mở desktop thao tác 1 lúc thì tự nó tắt" — the supervised runner
log showed `idle_grace → draining_shutdown → exit` with `clients=0` while the
Desktop app was open and in use, after which the supervisor tore down the
whole stack.

**Root cause.** `desktopLifecycle.start()` ran `register()` exactly once at
`app.whenReady()`. But `scripts/supervisor.js` spawns the Desktop app
immediately after spawning the runner — and the runner is launched via
`go run`, i.e. it compiles first and only starts listening seconds later.
When `register()` lost that race it caught the error, set `lease = null`, and
**never retried** — the Desktop stayed permanently unmanaged. The runner then
correctly followed SD-28 D-3 (`supervised` mode: 90s boot grace + 30s
zero-client grace), saw `clients=0`, drained, and exited; the supervisor
treated the exit as unexpected and shut the stack down — closing the app
mid-use.

## Fix (`src/lifecycle/desktopLifecycle.ts`)

- `start()` now goes through `ensureRegistered()`: on failure a
  `registerRetryTimer` re-arms every `REGISTER_RETRY_MS` (2s, injectable via
  `ports.registerRetryMs`) until a lease is held or the app quits.
- The same retry is armed on the two other register-failure paths that
  previously stranded the client unmanaged: the stale-lease re-register inside
  `heartbeatOnce()` and the new-instance re-register inside `pollReconnect()`.
- `stop()`/`runnerLost()` clear the retry timer; the retry callback is guarded
  by `quitting`/`lease` so it can never fire post-quit or mint a second lease.

Not changed (deliberately): a single heartbeat transport error still triggers
the unplanned-loss path (`runnerLost` → notify + quit) — that is the designed
SD-28 contract and is asserted by `TestDesktop_UnplannedRunnerLossClosesWith
Notice`. Tolerating missed beats would change specified behavior; if a real
"flaky heartbeat kills app" report appears, revisit as a separate change.

## Tests (additive)

- `TestDesktop_RegisterRetriesUntilRunnerUp` — register ECONNREFUSED ×2 at
  boot → retry rejoins, lease held.
- `TestDesktop_StaleLeaseReregisterFailureRetries` — `lease_unknown` →
  re-register fails once → retry still rejoins, heartbeats resume.
- `TestDesktop_RetryStopsAfterQuit` — quitting with retry pending freezes
  register calls.
- All 11 pre-existing lifecycle tests pass unmodified (14/14 total).

## Notes

- `scripts/supervisor.js` line ~496 comment claims supervised runners "never
  idle-exit" — that is wrong per SD-28 D-3 (supervised DOES idle-exit: 90s
  boot + 30s zero-client grace). Left as-is (comment-only), but it caused a
  brief misdiagnosis; worth correcting next time that file is touched.
- TUI shares the register-once pattern (`cmdRegisterLease`) but registers
  only after `ConnectedMsg` (runner already reachable), so the race window is
  negligible there. Not changed.
- Provider-agnostic: lifecycle lease plumbing only.
