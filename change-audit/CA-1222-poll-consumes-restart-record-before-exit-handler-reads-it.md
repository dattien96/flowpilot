# CA-1222 — command poll consumes the fenced restart record before the exit handler can read it

- **Area**: supervisor lifecycle (scripts/supervisor.js)
- **Evidence**: live restart at 23:33:53 (restartId `restart_91c1996a…`).
  Log sequence:

  ```
  [lifecycle] drain complete action=restart — exiting
  [Supervisor] Runner exited unexpectedly (code=0 pid=25070). No silent respawn — shutting down stack…
  [Supervisor] Planned runner restart (restartId=restart_91c1996a…). Waiting for the old runner to exit, then respawning the runner only.
  [Supervisor] Grace period expired. Force-killing lingering processes...
  ```

  The runner died, the stack tore down, and a respawned runner was
  force-killed mid-teardown — runner + desktop + web all lost. This is the
  third time a planned `/system/restart` collapsed the whole stack
  (CA-1217 fixed the write-side ordering; this is the remaining
  consume-side race).

- **Root cause**: `pollSupervisorCommand` consumes a valid fenced record
  by **deleting the file** (`clearStaleSupervisorCommand`) before
  deferring `handlePlannedRunnerRestart` by 200ms ("consume before acting
  so a retried poll cannot replay it"). When the runner's 'exit' event
  lands inside that window, CA-1217's exit-handler `readSupervisorCommand`
  reads an absent file, falls through to `handleRunnerExitUnexpected` →
  `cleanupAndExit` (`isExiting=true`) — while the poll's deferred timer
  still fires `handlePlannedRunnerRestart`, respawning a runner into the
  middle of the teardown, which the 3s force-kill then destroys.
- **Fix**: a module-level `pendingRestartCmd` handoff slot shared by both
  consumers.
  - The poll still consumes+deletes the file (replay protection
    unchanged) but now also stores the record in `pendingRestartCmd`.
    Its 200ms timer became a hung-drain *watchdog only*: it fires
    `handlePlannedRunnerRestart` solely when the exit handler hasn't
    claimed the record (`pendingRestartCmd !== cmd`), the planned flag is
    unset, and teardown hasn't begun (`isExiting`) — respawning
    mid-teardown was precisely the lost-respawn half of the live failure.
  - The runner exit handler now consults `pendingRestartCmd` **before**
    the filesystem — whichever side consumed the record, the exit path
    always sees it and takes the planned-restart branch. It then clears
    the slot, so a consumed record can never double-fire.
  - `startRunnerProcess` clears the slot on each spawn — a record
    consumed for generation N must not stain generation N+1's exit.
- **Test**: `TestSupervisor_PollConsumedRestartExitStillRespawns` —
  poll consumes first (file gone), `exit` fires inside the watchdog
  window, runner-only respawn happens, stack stays up. Suite 12/12 green.
- **Scope note**: `shutdown` records intentionally keep the old shape —
  a consumed shutdown that loses the exit race still resolves to
  `cleanupAndExit` through the unexpected path, which is the same target
  state (log wording aside). Only the restart path carried a
  respawn-then-kill contradiction.
