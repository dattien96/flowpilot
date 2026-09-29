# CA-1061 — R.2-2: typed `plan_complete` terminal when the vibe sprint plan drains

## Scope
- `apps/local-runner/internal/runner/provider_event.go` (`AgentLoopState.CompletionKind`)
- `apps/local-runner/internal/runner/vibe_cp.go` (`vibePlanDrainedLocked`,
  `settleVibePlanComplete`, `vibeCompletionPlanComplete`, wiring in
  `maybeStartNextVibeSprint` / `onVibeCpNodeDone` / `maybeParkVibeCpJoinResume`)
- `apps/local-runner/internal/runner/interactive_service.go`
  (`applyFlowControl` done-path kind stamp + completion note)
- `apps/local-runner/internal/tui/client/client.go`, `internal/tui/app/model.go`,
  `internal/tui/app/step_runtime.go` (contract field + statusline label)
- `apps/desktop-flowpilot/src/types/contract.ts`,
  `apps/desktop-flowpilot/src/components/OrchestrationBoard.tsx` (label)
- `apps/local-runner/internal/runner/vibe_plan_complete_test.go` (new, additive)

## Why
R.2-2 observation: when `todo/` drains — every task in `vibeTaskPlan` started
and finished — the run's terminal read was indistinguishable from a wedged
slicer. Three concrete wedges:

1. `decideNextVibeSprint` returns typed `Done`, but `maybeStartNextVibeSprint`
   fell through `!d.Start → return` — the loop stayed `running` forever.
2. A stale/late `task_slicer` completion on a fully-delivered plan hit the
   zero-task fail-closed park (`parkVibeRequirementFrom`) — the emptied
   `todo/` (the drained state) was read as "slicer produced nothing" and
   could flip even a `done` loop back to `blocked/requirement`.
3. `maybeParkVibeCpJoinResume` parked `cp_writer → task_slicer`
   resume-confirm on reopen of a delivered run (CP present, SS present,
   todo/ empty) → `flow_parked_awaiting_user` looking identical to a
   failure, Continue re-running the slicer and re-parking forever.

## What changed
New `AgentLoopState.CompletionKind` (additive, `omitempty`, durable via the
existing session-row LoopState serialization): `""` for generic completion,
`"plan_complete"` when `vibePlanDrainedLocked(rs)` — `len(vibeTaskPlan) > 0
&& vibeSprintIndex >= len(plan)`. An empty plan is never drained (the
slicer-failure shape keeps the retryable `blocked/requirement` path).

`settleVibePlanComplete` mirrors the `applyFlowControl` done tail:
conditional loop mutate (a `stopped`/`paused` loop keeps the operator
verdict — mutate BEFORE any run-status flip, same contract as
`parkVibeSprintBudget`), then `markFlowRunComplete` /
`settleParentRunOnFlowDone`, completion note, graph emit, synchronous
persist, `vibe_plan_complete` diag. Idempotent — already
`done+plan_complete` no-ops.

Wired at every drain-detection seam:
- `maybeStartNextVibeSprint`: `d.Done` settles instead of silent return.
- `applyFlowControl` done-path: stamps the kind + plan-complete note when
  drained (covers the audit `→ done` terminal edge).
- `onVibeCpNodeDone` zero-task branch: drained plan settles instead of the
  failure park (node still stamped DONE first, BUG-364 ordering intact).
- `maybeParkVibeCpJoinResume`: drained run skips the resume-confirm park.

UI: desktop `AgentLoopState.completionKind` + OrchestrationBoard status
line reads `plan complete`; TUI client contract mirrors the field and
`settleFlowIfDone` statusline reads `plan complete`. Both gate on
`status=="done"`.

## Non-changes
- Genuine zero-task slicer failure (plan never delivered) still parks
  `blocked/requirement` with the retry-from-node resume path (BUG-363/364/471
  contracts intact — pinned by `TestPlanComplete_ZeroTaskSlicerFreshRunStillBlocks`).
- `Status` stays `done` — CompletionKind is a qualifier, not a new loop
  status, so every done-gated consumer (`flowLoopDone`, settle chrome,
  stop-button logic) is unchanged.
- Sprint-remaining guard (`sprintRemaining > 0 && agentInitiated`) unchanged
  — an undrained plan still cannot be agent-settled done.

## Evidence
- `go test -count=1 -run TestPlanComplete_ ./internal/runner` — 7/7:
  done-settle stamps kind; undrained leaves kind empty; `maybeStartNext`
  settles drained (run `completed`); join-resume skipped on drain; stale
  slicer-on-drained settles; fresh zero-task still blocks; stopped loop wins.
- Adjacent: `-run 'TestVibe|TestBug36|TestTask32|TestCA79|TestCA80|TestTask34|
  TestTask35|FlowControl|TestBug471|TestBug468|TestClusterF|VibeSprint'` — pass.
- `./internal/tui/...` — all packages pass. Desktop `tsc --noEmit` clean.
- Full `./internal/runner` suite: 26 failures, all pre-existing — the six
  gate/reprompt/DoD failures (TestBug425/440/514, TestRun200816,
  TestGateHook_DodComplete×2) reproduced identically on clean HEAD
  3021d810; remainder = missing provider binaries (claude/codex/agy/
  opencode/gitnexus) + documented flakes.
