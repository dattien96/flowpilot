# CA-1225 — runner: BUG-640 stopAgentLoop releases the target's own cohort seat

- **Area**: apps/local-runner/internal/runner (interactive_service.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-045), 2026-10-06 —
  `context-round-1` cohort member `run-429281` stopped via
  `agent-loop/stop` stayed an unfilled seat in the grandparent cohort;
  every subsequent `flow-control continue`/`done` was rejected with
  `rejected_cohort_incomplete` until an escalate-park path happened to
  release the seat. Same shape reproduced for stalled-member stops during
  the run's five zombie barriers.
- **Bug**: `stopAgentLoop` walked the target's *children* and appended a
  `cancelled` release placeholder for each child seated in a cohort — but
  when the target run was itself a cohort member (non-empty `parentRunID`
  + `flowCohortId`), nothing appended to *its* parent cohort. The seat
  leaked forever; barriers keyed on it could never join.
- **Fix**: before stopping, capture the target's own cohort membership
  (`parentRunID`/`flowCohortId`/`label`) and — unless a real result for
  that label is already buffered (`memberAlreadyBuffered`) — queue the
  target into the same `cancelledCohort` release path used for children.
  The existing tail then appends the placeholder and drains the barrier
  normally, preserving BUG-553 replacement semantics (a revived
  replacement leg's real result still overwrites the placeholder).
- **Provider parity**: provider-agnostic — cohort bookkeeping inside the
  orchestrator/service; no adapter, event stream, session, or gate-hook
  code touched. Tests use Codex fixtures; no provider branch exists in the
  edited path.
- **Tests (additive, reproduce-first)**:
  `bug640_stop_member_releases_cohort_seat_test.go` —
  - `TestStopMemberLegBuffersCancelledEntrySoGrandparentBarrierJoins`:
    stopping the seated member directly drains the grandparent cohort.
    Red before fix ("seat leaked").
  - `TestStopMemberLegKeepsSeatOpenForRespawn`: placeholder is a released
    seat, not a terminal write — a replacement leg's `completed` result
    replaces it (buffer length stays 2, no duplicate), and a later
    duplicate stop cannot clobber the real result.
- **Verification**: `go test -count=1 -run 'StopMemberLeg'
  ./internal/runner/` — green post-fix, red pre-fix. Cohort/stop batch
  (`Cohort|Stop|Barrier` patterns) green; the only batch failure
  (`TestReconstructResume_*`-adjacent, `TestBug334` opencode probe) is
  environmental/pre-existing, confirmed identical on the stashed
  pre-change tree.
- **Worktree**: n/a (runner-only change).
