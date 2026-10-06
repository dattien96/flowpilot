# CA-1232 — runner: BUG-643 DOA cohort members age from createdAt so the stall sweep can flag them

- **Area**: apps/local-runner/internal/runner (cohort_stall.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-045), 2026-10-06 —
  debate `owner_2` leg `run-460096` was spawned and registered into the
  open debate cohort but its first turn never dispatched: zero records,
  `lastProviderEventAt` zero, `turnInFlight` false. It held the cohort
  seat ~17min; the debate could not join until an operator stopped the
  leg. Same DOA shape appeared for earlier silenced legs in the run.
- **Bug**: two reinforcing wedges on the same record shape.
  1. `checkAndBlockStalledMembers` computed member age as
     `now - lastProviderEventAt`, falling back to `createdAt` **only
     while `turnInFlight`** — a spawned-but-never-dispatched leg hit the
     bare `else { continue }` and could never stall, no matter how old.
  2. `deadDispatchNonTerminalChildExists` counts any non-terminal leg as
     live, so the dead-dispatch re-drive sweep never re-drove the node
     either — the seat was held with no wakeup and no re-drive.
- **Fix**: zero-event members now age from `createdAt` on live statuses
  (`running`/`starting`/`idle`) in both `checkAndBlockStalledMembers`
  and `maybeScheduleStallCheck` — the latter is required or the sweep
  timer never arms for a leg that produces no events. `waiting_*`
  statuses stay excluded: those are parks owned by the orphan/mirror
  heal sweeps (BUG-641/CA-1228). `pendingTurnPrompt` joined the
  `intentArmed` shield — a queued turn is held work (serial dependency
  ordering can keep it armed past the timeout), not silence.
  Unparseable `createdAt` fails conservative (`continue`), unchanged.
- **Provider parity**: provider-agnostic — stall bookkeeping inside the
  orchestrator/service; no adapter, event stream, session, or gate-hook
  code touched. Tests use Codex fixtures; no provider branch exists in
  the edited path.
- **Tests (additive, reproduce-first)**:
  `bug643_doa_leg_stall_test.go` —
  - `TestBug643_DOAMemberLegStallsOpenCohort`: zero-event non-in-flight
    member aged past timeout parks the hub `member_stalled`. Red before
    fix.
  - `TestBug643_DOAMemberLegSchedulesStallTimer`: the sweep timer arms
    for a DOA leg — otherwise the wedge only resolves on a provider
    event that never comes. Red before fix.
  - `TestBug643_FreshDOAMemberLegDoesNotStall`: inside the dispatch
    window is not yet provably dead.
  - `TestBug643_QueuedTurnMemberIsNotStalled`: armed `pendingTurnPrompt`
    is held work.
  - `TestBug643_WaitingMemberIsNotDOAStalled`: `waiting_*` statuses stay
    owned by the heal sweeps — no double-ownership.
- **Verification**: `go test -count=1 -run 'TestBug643_'
  ./internal/runner/` — 5 green post-fix, 2 red pre-fix. Stall/cohort/
  debate/resume regression batch green except 2 environmental
  (`codex`/`agy` binaries absent from PATH) and 2 pre-existing
  `TestReconstructResume*` failures identical on the stashed pre-change
  tree.
- **Worktree**: n/a (runner-only change).
