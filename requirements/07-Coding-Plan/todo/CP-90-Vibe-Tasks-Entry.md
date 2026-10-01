# CP-90: Vibe Tasks Entry — Sprint Existing Tasks Under a Locked CP

- Document ID: `CP-90`
- Title: `Third Vibe entry: vibe-tasks — CP in, pre-broken Task-*.md plan out, identical sprint chain`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-30`
- Last Updated: `2026-09-30`
- Parent Documents: `SD-24` (Vibe sprint chain), `SS-18` (Vibe Working Mode), `CP-58` (task splitter), `BUG-468` (CP-scoped plan)
- Child Documents: `Task-456`, `Task-457`
- Related Documents: `CA-1061` (vibe plan_complete terminal), `SS-19` (harness family)
- Replaces: ``
- Tags: `vibe`, `flow`, `entry-point`, `sprint-chain`, `runner`

## AI Quick View

### Summary

- Vibe today has exactly two user-startable entries: `vibe-ingest` (idea → SS
  → CP → AI slices tasks → sprint) and `vibe-cp-ingest` (existing CP → AI
  slices tasks → sprint). `vibe-sprint` / `vibe-owner-debate` are system-only.
- The prepared-workspace case — SS/SD/CP and `requirements/08-Task/todo/
  Task-*.md` already broken down — has no entry. Pointing `vibe-cp-ingest` at
  that CP re-runs `task_slicer`, which writes NEW task files additively and
  duplicates the existing plan.
- `vibe-tasks` is a third user-startable flow: `cp_reader → cp_validator →
  cp_lock → task_plan_reader → done`. The reader is a delegate that reports
  the existing parented task manifest; it writes nothing. The Go seam after
  it is the SAME post-`task_slicer` machinery — `collectVibeSprintPlanForCP`
  scopes `Task-*.md` by `Parent Documents:` to the locked CP, seeds
  `vibeTaskPlan`, and `maybeStartNextVibeSprint` chains `vibe-sprint` per
  task. Sprint execution, handoff, budget cap, owner-debate, and the
  `plan_complete` terminal are byte-identical to the current pipeline.
- Fail-closed twice: admission rejects a non-CP source (422
  `invalid_cp_source`, shared fence) and a CP with zero parented tasks (422
  `no_cp_tasks`); if tasks disappear before the reader completes, the node
  parks `blocked/requirement` — never sprints a fallback plan.

### Current Ask

- Ship `vibe-tasks` as an explicit picker entry (Desktop Flow tab + TUI
  `/flow`) so a prepared workspace runs per-task sprints directly, without
  regenerating SS/CP/Tasks.

### Key Decisions

- `P-1` New FlowDefinition, not a conditional edge inside `vibe-cp-ingest` —
  topology stays truthful (a slicer node that is sometimes skipped would lie
  in the graph and in audit).
- `P-2` `task_plan_reader` is `run: delegate` like `task_slicer` — reuses the
  proven completion path (`tryAdvanceFlowFromNode → onVibeCpNodeDone`),
  checkpoint layer, and cohort-join semantics; avoids a second `hub.inline`
  node ambiguity (`hubInlineNodeID` first-match fallback).
- `P-3` The reader is advisory output only — the sprint plan is always
  re-derived from disk by `collectVibeTaskPlanForCP`. Schema-first contract:
  state never comes from node prose.
- `P-4` `DetectVibeEntry` keeps mapping a bare CP path to `vibe-cp-ingest`;
  `vibe-tasks` is explicit-picker only so behavior of existing quick-start
  paths is unchanged.

### Constraints

- SS-18 BR-2 stands: `cp_lock` user confirmation still gates sprint start —
  only task slicing is removed, not the lock.
- Sprint chain, budget (`vibeSprintBudget`), handoff files, debate resolver,
  checkpoint layers, and `plan_complete` terminal are reused untouched.
- Existing task files must carry `Parent Documents:` naming the locked CP;
  foreign/stale tasks never join the plan (BUG-468 scope).
- If all parented tasks are deleted mid-run, `restartVibeTaskSlicerFor
  MissingTasks` already recovers by re-running `task_slicer` — acceptable
  regeneration path, no new code.

### Open Questions

- none

### Source Refs

- `internal/runner/vibe_cp.go` (plan seed + sprint chain)
- `internal/workingmode/workingmode.go` (user-start gate)
- `internal/agentpack/flow-pack/flows/vibe-cp-ingest.yaml` (template)
- `bug468_vibe_task_plan_cp_scope_test.go` (scoped-plan contract)

## 1. Goal

Give Vibe a first-class entry for a fully prepared workspace: lock the CP,
discover its existing `Task-*.md` children, run the standard sprint chain.

## 2. Input Documents

- `SD-24` Vibe sprint orchestration
- `SS-18` Vibe working mode contract
- `FORMAT-REFERENCE-TASK.md` (Parent Documents metadata)

## 3. Implementation Strategy

- overall approach: add the flow as pack data; teach the existing
  post-slicer Go seam to treat `task_plan_reader` as an equivalent plan-
  seeding boundary; open the user-start gate and both client pickers.
- sequencing: pack YAML + prompt → runner seams → gate lists → TUI/desktop
  pickers → tests → live test.
- dependencies: none — all seams already exist.

## 4. Work Breakdown

- `P-1` Pack data: `flows/vibe-tasks.yaml` + `prompts/vibe-task-plan-read.md`
  + `manifest.yaml` registration.
- `P-2` Runner seams: `vibeTasksFlowID`/`vibeTaskPlanReaderNodeID` consts;
  `inferPackFlowRefFromNodes` (reader before cp_reader case); `onVibeCp
  NodeDone` reader arm with fail-closed park; `vibeLinearWriterNode` /
  `vibeInheritsSessionModel`; hub-terminal branch in `interactive_service`;
  checkpoint artifact scope + layer normalize; lock markers at both
  flow-start sites; shared CP-source admission fence (+`no_cp_tasks`).
- `P-3` Entry points: `workingmode` user list + `FlowPickerOptions`; TUI
  `isVibeCpSourcedFlow` arming; desktop `workingMode.ts` picker + family set.
- `P-4` Tests: `vibe_tasks_live_test.go` (in-process plan/park/gate/fence
  coverage + LIVE=1 real-binary path); update CA-1070 picker expectation;
  pack load/validation suite.

## 5. Touched Areas

- files: `internal/agentpack/flow-pack/{manifest.yaml,flows/vibe-tasks.yaml,
  prompts/vibe-task-plan-read.md}`, `internal/runner/{vibe_cp.go,
  vibe_checkpoint.go,interactive_service.go,interactive_handlers.go}`,
  `internal/workingmode/workingmode.go`, `internal/tui/app/{app.go,
  working_mode.go}`, `apps/desktop-flowpilot/src/state/workingMode.ts`
- modules: runner vibe seams, working-mode gate, pack manifest, TUI arming,
  desktop picker
- database: none
- external systems: none

## 6. Data or Migration Steps

- schema: none (durable run fields unchanged)
- data backfill: none — builtin mirror rows appear on next pack sync
- config updates: none

## 7. Validation Plan

- tests to add: `vibe_tasks_live_test.go` — reader-done seeds scoped plan,
  zero parented tasks parks `blocked/requirement`, admission rejects non-CP
  and empty-task CPs, gate allows vibe-user/dev-reject, `vibe-sprint` still
  system-only; LIVE=1 block drives the real binary end-to-end.
- manual checks: TUI `/flow vibe-tasks @<cp>`; desktop Flow tab picker shows
  `vibe-tasks` in vibe mode only.
- failure cases: CP without tasks → 422 `no_cp_tasks`; tasks deleted after
  admission → `blocked/requirement` park at reader, never a fallback sprint.

## 8. Rollout and Fallback

- rollout order: additive — new flow id, new picker option; no existing path
  changes.
- fallback path: `vibe-cp-ingest` unchanged for AI slicing; `vibe-tasks`
  rejects empty-task CPs so misuse routes to the correct entry.
- monitoring: `flow_advance`/`vibe` diag logs, `plan_complete` terminal.

## 9. Risks

- `R-1` User selects `vibe-tasks` on a CP whose tasks lack `Parent
  Documents:` metadata → scoped collector finds nothing → fail-closed
  park/422; message names the required metadata so the fix is obvious.
- `R-2` Reader as delegate adds one extra agent turn vs a pure Go step —
  accepted for topology truthfulness and a visible plan-manifest artifact.

## 10. Definition of Done

- `vibe-tasks` user-startable in vibe mode on Desktop picker + TUI `/flow`.
- Locked CP + pre-existing parented tasks → `vibe-sprint` chain identical to
  the cp-ingest path (handoff, budget, debate, `plan_complete`).
- Zero parented tasks → typed 422 at admission and `blocked/requirement`
  park at the reader node; no sprint ever starts from a fallback plan.
- All new unit tests + live test pass; no regression in `internal/runner`,
  `internal/workingmode`, `internal/agentpack`, desktop state tests.
