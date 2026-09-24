# BUG-471: vibe requirement park resume falls into sealed-hub no-op — unblock leaves a running loop with no work

- status: done
- found: live run-102429 + run-103685 (fp-beds/full, A-60-4 R-TK delete-demotion drill — zero CP-01-parented tasks after task removal)
- fixed_by: CA-968
- tests: internal/runner/bug471_requirement_park_resume_test.go

## Symptom (live)

After the task_slicer completed with zero Task files parented to the run's
CP, `onVibeCpNodeDone` correctly failed closed: stamped the step DONE and
parked `blocked/requirement`. The operator then sent `agent-loop/continue`;
the loop unblocked to `running` — and nothing ever dispatched again. The
next flow node flapped `RUNNING`/`WAITING_USER_APPROVAL` with no delegate
spawn, no turn, no card; the run only resurfaced via the `hub_stalled`
watchdog. Same shape on run-102429 (validator step flapped, never
dispatched) and run-103685 (unblock → sealed-hub no-op → hub_stalled).

## Root cause

Every vibe requirement park resolves through the generic
`resumeFlowWithFeedback` tail: unblock the loop, then
`maybeAutoReinvokeHubWithNote`. On a post-lock vibe run that reinvoke is a
deliberate no-op — `vibeHubSealed` (vibe_lock.go) skips hub reinvocation
once cp_lock/ss_lock sealed the validator hub. That seal is correct for
review-loop semantics, but it means an unblock-with-generic-resume on a
sealed run re-opens the loop with zero work behind it: no advance, no
dispatch, no park — a running zombie until the stall watchdog.

All six requirement-park sites sit on the `tryAdvanceFlowFromNode` path
(or its `maybeResumeVibeCoderAfterTdd` resume twin), so a dedicated resume
can re-invoke the same advance: the parked condition is re-evaluated and
either proceeds (operator fixed the bed) or re-parks with the same reason.

Park sites covered:

- `vibe_cp.go` — task_slicer / sprint_slicer produced zero CP-scoped tasks.
- `flow_executor.go` — frozen contract store unreadable; tdd artifact
  missing before coder; coder has no frozen contract after tdd; coder
  spawn failed after tdd.
- `vibe_sprint.go` (`maybeResumeVibeCoderAfterTdd`) — tdd artifact missing
  on the resume path; sprint graph missing (`resume:coder` marker re-runs
  the resume function, not a node advance).

## Fix

- New durable run field `vibeRequirementFromNode` records the completed
  node whose advance parked (or `resume:coder` for the resume-path parks).
  Persisted through `ProviderSessionState.VibeRequirementFromNode` and the
  local session store row (`vibe_requirement_from_node`), serialized in
  both directions like `VibeCpDocID`.
- `parkVibeRequirement(parentRunID, gateReason)` is kept as the 2-arg
  wrapper; the six advance-path sites now call
  `parkVibeRequirementFrom(parentRunID, gateReason, fromNode)`.
- `resumeVibeRequirement(parentRunID, feedback)` consumes the saved node:
  `resume:coder` → `maybeResumeVibeCoderAfterTdd`; otherwise
  `tryAdvanceFlowFromNode(parentRunID, fromNode, msg)` — which re-runs
  `onVibeCpNodeDone` (the parked condition check lives there) before
  advancing. A no-dispatch result is diag-logged
  (`vibe_requirement_resume_no_advance`) so the watchdog path stays
  observable.
- `resumeFlowWithFeedback` branches on `prevBlockReason == "requirement"`
  after the sprint-boundary branch; parks with no recorded node fall
  through to the generic path unchanged.
- `applyVibeGateResolver`'s requirement branch clears any stale
  `vibeRequirementFromNode` — a turn-level gate park records no advance
  context and must never replay an earlier advance-path park's node.

## Verification

- `TestBUG471_RequirementParkContinueReevaluatesAndReparks` — zero CP-01
  tasks at Continue → advance re-runs → re-park `blocked/requirement`
  (pre-fix: running zombie). Red before the fix.
- `TestBUG471_RequirementParkContinueAdvancesWhenTasksAppear` — operator
  adds a CP-01 task while parked → Continue rebuilds the CP-scoped plan
  (Task-9 + Task-21) and consumes the saved node.
- `TestBUG471_RequirementFromNodeSurvivesDurableRoundTrip` —
  `sessionStateOf` → `reconstructRunInternal` preserves the field.
- `TestBUG471_RequirementParkCoderResumeMarkerReparks` — the `resume:coder`
  park re-runs `maybeResumeVibeCoderAfterTdd` and re-parks while tdd
  evidence is absent.
- `TestBUG471_GateRequirementParkClearsStaleFromNode` — the gate-classified
  park clears residue and still parks `blocked/requirement`.
- Vibe/sprint/resume/gate regression surface
  (`-run 'Vibe|Sprint|Resume|Requirement|BUG365|BUG468|BUG469|BUG470|
  BUG363|BUG364|FlowGate|GateResolver|Park'`) green — BUG-365 park-graph
  emission, BUG-363/364 fail-closed stamps, and the BUG-468/469/470 fixes
  unchanged.

## Live evidence

Pre-fix runs run-102429 / run-103685 (fp-beds/full): unblock after the
zero-task park produced `status: running` with `cp_validator` stamped
RUNNING and no delegate/turn — the sealed-hub no-op, later settling into
`hub_stalled`. Post-fix live verification tracked in
requirements/07-Coding-Plan/todo/CP-Full-Live-Test.md (A-60-4 R-TK leg).

## Related surface (not changed)

The gate-classified requirement park (`applyVibeGateResolver`, a
turn-level violation such as requirement-signature drift) also blocks with
`BlockReason == "requirement"` and no recorded node — it intentionally
keeps the generic resume path. Whether that path can drive a sealed-hub
run is unproven; flagged here rather than silently scoped in.
