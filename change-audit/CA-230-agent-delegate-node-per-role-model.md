# CA-230: Agent-Delegate Node Per-Role Model

## Summary

User confirmed the intended model-resolution design for Flow Mode, restated uniformly for model and YOLO mode: Chat mode reads both from the controller; a normal flow reads YOLO from the Flow and Model via Step > Flow > Project; within a running flow, an **agent** node (spawns its own child run) can run its own provider/model, while an **inline** node (executes within the parent's own turn) always uses the flow's model. Implemented the agent-node override: `flow_executor.go`'s three `spawnChildRun` call sites now resolve each `agent.delegate` node's own model via a new `resolveFlowNodeModel` helper, which maps the node's agent role (e.g. `agents/reviewer.md` → `reviewer`) to its purpose-named `step_definitions` catalog row (`flow-agent-delegate-reviewer` — the same "Flow: Reviewer" entry the manual workflow builder already exposes, from `BUG-161`'s migration). `spawnChildRun` now treats this per-node model as the top-priority tier for both model and provider derivation, using the same "model is authoritative" pattern `BUG-171` established. A node whose role has no matching catalog row (or no model configured) transparently falls back to the pre-existing inherit-from-parent behavior — this only changed the coder/reviewer built-in roles that have purpose-named rows today. Closed out `BUG-228` (moved from a documented known-limitation to fixed) and updated `SS-05`/`SD-06` to describe the per-node override alongside the existing run-level resolution chain.

## What Changed

- `apps/local-runner/internal/runner/agent_orchestrator.go`:
  - Added `SpawnAgentInput.Model` (internal-only, `json:"-"`, same convention as `AgentDefOverride`/`UIInitiated`).
- `apps/local-runner/internal/runner/flow_executor.go`:
  - Added `resolveFlowNodeModel(ctx, node)`: derives the node's agent role via the existing `flowNodeAgentName`, looks up `step_definitions` row `flow-agent-delegate-<role>` via `s.catalog.ListSteps`, returns its `Model` (or `""` if no matching row or no model set).
  - Wired `Model: s.resolveFlowNodeModel(...)` into all three `spawnChildRun` call sites: `startResolvedFlow`'s entry-node spawn, `startInlineEntryChain`'s delegate-target spawn, `tryAdvanceFlowFromNode`'s auto-advance spawn.
- `apps/local-runner/internal/runner/interactive_service.go`:
  - `spawnChildRun` now derives `providerKey` from `in.Model` (via `providerKeyFromModel`) as the top-priority tier, above explicit/agent-definition/parent-inherited provider.
  - `childModel` now uses `in.Model` as the top-priority tier, above the (always model-free) agent definition and above same-provider inheritance.
- `apps/local-runner/internal/runner/flow_executor_test.go`:
  - Added `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`: drives the full flow (coder completes → auto-advance spawns the reviewer cohort) with a catalog carrying a `flow-agent-delegate-reviewer` row set to `claude-sonnet`, asserting both reviewer children resolve to `claude`/`claude-sonnet` while the coder child (no matching row) still inherits the run's own `codex`/`gpt-5.4-mini`.
- `requirements/09-BugFix/done/BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md`: moved from `todo/` to `done/`, rewritten to describe the actual implementation (previously described two undecided candidate directions).
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §3: documented the per-node override alongside the run-level resolution chain.
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6.2.1 (new), §6.3: documented `resolveFlowNodeModel`'s mechanics and pseudocode; removed the now-resolved "known limitation" note.

## Verification

- `go build ./...` in `apps/local-runner` — passes.
- `go test ./internal/runner/...` — 1057 passed, 15 failed (identical pre-existing, environment-specific failure set already established as this session's baseline), 14 skipped.
- `go test ./internal/runner -run 'TestCreateRun|TestE2EReviewLoop|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestAutoReinvokeHub|TestWorkflowStepsRuntime|TestCoderCompletion|TestStartResolvedFlow'` — 47 passed.
- `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel` specifically re-run 5x (`-count=1` each) to rule out flakiness in the async cohort-spawn wait — passed every time. `-race` unavailable in this environment (cgo disabled).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: feature
summary: Let an agent-delegate flow node resolve its own model/provider from its role's step_definitions row instead of always inheriting the flow's single resolved model
# --->8---
