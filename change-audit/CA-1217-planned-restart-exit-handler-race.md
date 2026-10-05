# CA-1217 — Planned restart consumed by exit handler instead of poll (live 2026-10-05)

## Evidence

`POST /system/restart` accepted 21:59:06 (`draining_restart`,
`restart_ce5893b4`) — then the ENTIRE stack died: runner, supervisor,
desktop, admin-web all gone within seconds, no respawn.

Sequence:

1. `runSystemDrain` writes `.flowpilot/supervisor.cmd` (fenced
   `restart-runner` record) BEFORE `os.Exit` — step 2 of the drain.
2. `attachRunnerExitHandler` fired on runner exit with
   `plannedRunnerRestart=false`: the 500ms `commandWatchInterval` poll had
   not consumed the command yet.
3. Exit → `handleRunnerExitUnexpected` → `cleanupAndExit` → killed the
   supervisor itself plus every managed child. Nothing left to respawn.

The same request 32 min earlier (21:27) succeeded through the identical
path — timing luck: the poll happened to beat the exit that time.

## Fix

`attachRunnerExitHandler`'s unexpected branch now reads the control file
before declaring the death unplanned: the drain's write-before-exit
ordering guarantees a fenced restart record is already on disk when the
handler runs. A valid record (`restart`/`restart-runner`, fence-validated
against the cached `currentRunnerInstanceId`) is consumed and routed to
`handlePlannedRunnerRestart` — the same respawn-only-the-runner path the
poll would have taken. No record or an invalid one keeps the frozen
no-ghost policy (`handleRunnerExitUnexpected` → stack teardown).

## Tests

- `TestSupervisor_PlannedRestartConsumedByExitHandlerStillRespawns` —
  fenced restart written, runner exit emitted before any poll: command
  consumed, exactly one runner respawned, stack alive (RED pre-fix:
  teardown, zero spawned).
- `TestSupervisor_ExitWithoutFencedCommandStillTearsDown` — same path
  with no command on disk: teardown + no respawn (no-ghost preserved).
- Full `supervisorLifecycle` suite: 11/11 green.
