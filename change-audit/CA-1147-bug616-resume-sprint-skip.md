# CA-1147 — BUG-616: resume skipped a sprint — stale-sprint evidence + parked-topology blind spot

Live trigger: CP-03 watch on run-150388 (PrivateVault). Runner restarted at
18:24 local to deploy bc1c5412; seconds later the run emitted
`handoff-sprint-2.yaml`, stamped the `audit` step DONE, and spawned the
Task-033 sprint — while Task-032's `VaultContainer.cpp` was still all stub
sentinels, its coder/validate/reviewer legs had never run, and the task file
was still in `requirements/08-Task/todo/` (only Task-031 sits in `done/`).

## Root cause — two compounding holes at resume

1. **Foreign-sprint session projection.** `resumedFlowStepRows` matched
   child sessions to flow nodes by label/agent-name only; `vibe_task_index`
   is stamped on every vibe leg at spawn but was never consulted. Sprint-1's
   `coder`/`reviewer` legs stay `leg: active` forever (legs close at run
   end), so `run-162468` (coder, completed, index 1) satisfied sprint-2's
   `coder` step at resume. `reconstructPendingChildSessions` had the same
   hole: sprint-1 `flow-auto-validate-round-14` cohort members rebuilt and
   drained at resume, stamping `reviewer DONE` via `joinRecoveredCohort`
   (transition log line at 11:24:43Z).

2. **Debate-overlay blind spot.** During the mounted owner debate,
   `rs.activeFlowNodes` holds only the 4 debate nodes; the sprint topology
   lives in `vibeParkedNodes` (BUG-404). `vibeSprintEvidenceComplete` and
   `hasRunningSprintStep` scanned `activeFlowNodes` only → zero `agent.code`
   nodes found → the CA-1096 evidence check vacuously passed and the
   `blocked_missing_feature_key` audit draft auto-finalized →
   `maybeAutoAdvanceVibeSprintBoundary` reseeded the graph and started
   Task-033. (BUG-568 had already unioned parked nodes into step-row
   rebuilding; the *checks* were never updated.)

## Fix

- `sessionBelongsToVibeSprint(session, sprintIndex)`: a leg stamped for a
  different sprint never satisfies this sprint's nodes or cohort barriers.
  Index-0 (unstamped/legacy) sessions fail open — unknown provenance is not
  foreign evidence.
- `resumedFlowStepRows`: skip foreign-sprint sessions before both
  `matchFlowNodeForSession` and `legacyCohortNodeByRun` lookup.
- `reconstructPendingChildSessions`: exclude foreign-sprint sessions from
  `cohortMembers` so stale validate/debate cohorts can't rebuild and drain
  onto current-sprint nodes.
- `vibeSprintTopologyNodes(rs)`: union of `activeFlowNodes` +
  `vibeParkedNodes`; used by `vibeSprintEvidenceComplete` and
  `hasRunningSprintStep` so the checks see the real sprint steps during a
  debate mount or a restart-while-mounted.

## Tests (additive)

- `TestResumedFlowStepRowsForeignSprintLegsDoNotSatisfyNodes` — red before
  fix: sprint-1 coder/reviewer legs stamped sprint-2 nodes DONE.
- `TestVibeSprintEvidenceSeesParkedTopology` — red before fix: debate-only
  activeFlowNodes + parked coder PENDING returned evidence-complete.
- `TestResumedFlowStepRowsSameSprintLegsStillProject` — parity: same-sprint
  and index-0 legs still project.
- Focused + resume/cohort/reconstruct suites green; `go vet` clean;
  `detect_changes`: risk low, 6 symbols.

## Operator follow-up (state repair)

run-150388 sits at sprint_index=3 with the sprint-3 tdd leg cancelled.
Task-032 must be re-driven — either restore the sprint index to 2 (the
boundary artifacts `handoff-sprint-2.yaml` / step reseed are fabricated) or
re-run Task-032 in a fresh vibe pass. The fabricated handoff yaml should be
treated as void evidence.

## Files

- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`
- `apps/local-runner/internal/runner/bug616_stale_sprint_leg_projects_done_test.go` (new)
