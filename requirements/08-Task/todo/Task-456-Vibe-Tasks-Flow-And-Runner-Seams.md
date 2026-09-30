# Task-456: Vibe-Tasks Flow Definition & Runner Seams

- Document ID: `Task-456`
- Title: `vibe-tasks.yaml flow + task_plan_reader plan-seed seam reusing the post-slicer sprint chain`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-30`
- Last Updated: `2026-09-30`
- Parent Documents: `CP-90` (work item `P-1`, `P-2`)
- Child Documents: ``
- Related Documents: `vibe-cp-ingest.yaml`, `BUG-468`, `CA-1061`
- Replaces: ``
- Tags: `vibe`, `flow`, `runner`, `agentpack`

## AI Quick View

### Summary

- New builtin flow `vibe-tasks`: `cp_reader → cp_validator → cp_lock →
  task_plan_reader → done` — same CP path as `vibe-cp-ingest`, but the
  terminal node only READS existing `Task-*.md` parented to the locked CP.
- `task_plan_reader` completion runs the identical post-`task_slicer` seam:
  `collectVibeSprintPlanForCP` → seed `vibeTaskPlan`/reset cursor →
  `maybeStartNextVibeSprint`. Empty scope parks `blocked/requirement`.

### Current Ask

- Land the pack YAML + prompt + manifest row and every Go seam the node id
  touches (done-handler, checkpoint layer, hub-terminal belt, writer-node
  lists, lock markers, admission fence, flow-ref inference).

### Key Decisions

- `T-1` `task_plan_reader` is `run: delegate` (mirrors `task_slicer`), not a
  second `hub.inline` — avoids `hubInlineNodeID` first-match ambiguity.
- `T-2` Node output is advisory; `collectVibeTaskPlanForCP` on disk is the
  authoritative plan source (schema-first; no prose parsing).
- `T-3` Admission fence reuses `validateVibeCpIngestSource` for both flows;
  `vibe-tasks` additionally requires ≥1 parented task → 422 `no_cp_tasks`.

### Constraints

- `cp_lock` stays a `user.confirm` — SS-18 BR-2 lock before any sprint.
- Reader writes nothing; on `escalate` → `ask_user`, on `done` → flow `done`
  (sprints chain off the Go seam, not more graph nodes).

### Source Refs

- `apps/local-runner/internal/runner/vibe_cp.go`
- `apps/local-runner/internal/runner/vibe_checkpoint.go`
- `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-cp-ingest.yaml`

## 1. Goal

A `vibe-tasks` run lands with `vibeCpDocID` pinned, lock gating identical to
cp-ingest, then seeds the sprint plan purely from existing parented tasks.

## 2. Parent Links

- CP-90 P-1/P-2; BUG-468 scoped collector; CA-1061 `plan_complete` terminal.

## 3. Trigger

- User starts `vibe-tasks` on a CP whose `Task-*.md` children already exist.

## 4. Exact Change

- `internal/agentpack/flow-pack/flows/vibe-tasks.yaml` (new)
- `internal/agentpack/flow-pack/prompts/vibe-task-plan-read.md` (new)
- `internal/agentpack/flow-pack/manifest.yaml` (register both)
- `internal/runner/vibe_cp.go`: consts `vibeTasksFlowID`,
  `vibeTaskPlanReaderNodeID`; `inferPackFlowRefFromNodes` reader case BEFORE
  `cp_reader`; `onVibeCpNodeDone` reader arm with per-node reason text;
  `vibeLinearWriterNode` + `vibeInheritsSessionModel` += reader.
- `internal/runner/vibe_checkpoint.go`: artifact scope + layer normalize.
- `internal/runner/interactive_service.go`: hub-terminal belt (:7726),
  `runFirstTurnFences` + startTurn admission conditions, both lock-marker
  sites (flow-start turn + re-pin forward).
- `internal/runner/interactive_handlers.go`: createRun lock markers.
- `internal/runner/vibe_cp.go` `restartVibeTaskSlicerForMissingTasks`:
  `cpNode == vibeTaskPlanReaderNodeID` counts as passed-slicer evidence.

## 5. Touched Areas

- `internal/agentpack/flow-pack`, `internal/runner` (vibe seams only)

## 6. Acceptance Check

- [ ] `onVibeCpNodeDone(runID, "task_plan_reader")` seeds `vibeTaskPlan`
  scoped to `vibeCpDocID` and leaves `vibeSprintIndex == 0`.
- [ ] Zero parented tasks → `blocked`/`requirement` park, GateReason names
  the reader + CP; no sprint node starts.
- [ ] `validateVibeCpIngestSource` rejects vibe-tasks without a CP source
  (422 `invalid_cp_source`) and a CP with zero parented tasks (422
  `no_cp_tasks`).
- [ ] `inferPackFlowRefFromNodes` on vibe-tasks topology returns
  `flowpilot-core-flow-pack/vibe-tasks`.

## 7. Out of Scope

- Gate lists, TUI/desktop pickers, live test — Task-457.

## 8. Completion Notes

