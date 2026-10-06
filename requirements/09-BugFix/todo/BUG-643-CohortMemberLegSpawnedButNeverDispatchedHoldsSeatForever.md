# BUG-643 — A cohort-member leg can be spawned and registered while its first turn never dispatches: zero provider events + `turnInFlight=false` is skipped by the stall sweep's age attribution, so the open cohort seat is held forever with no re-drive and no wakeup

- **ID:** BUG-643
- **Severity:** High — the debate/cohort barrier cannot join and nothing
  ever re-drives or flags the dead member; the flow waits indefinitely
  until an operator stops the leg by hand.
- **Status:** FIXED (2026-10-06, CA-1232) — zero-event members on live
  statuses now age from `createdAt` in both the stall checker and the
  stall-timer scheduler, and `pendingTurnPrompt` joined the held-work
  intent shield.

## Evidence chain (all live)

1. run-306526 (vibe-tasks CP-04, Task-045), round 15: a debate mounted
   for the red-first/scaffold dispute spawned `owner_2` leg
   `run-460096`. The leg record existed and carried
   `flowCohortId=flow-auto-debate-round-0`, but produced **zero records**
   for ~17 minutes — turn never dispatched, no provider events, no
   `turnInFlight`.
2. `deadDispatchNonTerminalChildExists` counted the leg as live (any
   non-terminal status suppresses the re-drive), so the dead-dispatch
   sweep skipped the node — first wedge.
3. `checkAndBlockStalledMembers` computed stall age from
   `lastProviderEventAt`, falling back to `createdAt` **only while
   `turnInFlight` was true** — a never-dispatched leg hit the bare
   `else { continue }` and could never be flagged stalled — second wedge.
4. `maybeScheduleStallCheck` used the same attribution, so no sweep
   timer armed either — the leg produced no events that could re-trigger
   scheduling, so the deadlock had no wakeup path at all.
5. Operator `agent-loop/stop` on `run-460096` released the seat and the
   debate resolved within a minute — proving the only missing piece was
   detection of the never-dispatched member.

## Root cause

Leg spawn and turn dispatch are not atomic. When dispatch dies after
the run record exists (spawned record, silent adapter, lost reinvoke),
the leg sits at `running`/`starting` with `lastProviderEventAt` zero and
`turnInFlight` false — the one shape the stall sweep deliberately
skipped. The skip predates cohort membership being a barrier seat: a
silent non-member leg is harmless, but a silent *member* wedges the
whole cohort.

## Fix (CA-1232)

- `checkAndBlockStalledMembers` + `maybeScheduleStallCheck`: when
  `lastProviderEventAt` is zero, age the member from `createdAt` for
  live statuses (`running`/`starting`/`idle`) regardless of
  `turnInFlight`. Unparseable `createdAt` stays conservative (`continue`).
- `waiting_*` statuses are excluded — those are parks owned by the
  orphan/mirror heal sweeps (BUG-641, CA-1228); double-ownership would
  race the heal.
- `pendingTurnPrompt` joined the `intentArmed` shield alongside
  reprompt/resume — a queued turn is held work: serial dependency
  ordering (`dependenciesSatisfiedLocked`) can legitimately keep it
  armed past the timeout while a sibling runs.

## Regression coverage

`apps/local-runner/internal/runner/bug643_doa_leg_stall_test.go` —

- DOA member (0 events, not in-flight, aged out) parks hub
  `member_stalled` — red pre-fix.
- DOA member arms the stall timer — red pre-fix.
- Fresh DOA member inside the dispatch window does not stall.
- Member with armed `pendingTurnPrompt` does not stall.
- `waiting_*` member is not DOA-stalled (heal sweeps own it).

## DeclaredPaths

- `apps/local-runner/internal/runner/cohort_stall.go`
  (`checkAndBlockStalledMembers`, `maybeScheduleStallCheck`)
- `apps/local-runner/internal/runner/bug643_doa_leg_stall_test.go`
