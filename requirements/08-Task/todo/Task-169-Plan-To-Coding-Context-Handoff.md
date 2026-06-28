# Task-169: Plan To Coding Context Handoff

## Metadata

- Document ID: `Task-169`
- Title: `Plan To Coding Context Handoff`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-28`
- Last Updated: `2026-06-28`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/todo/CP-41-RAG-Harness-Flow-Mode.md), [Task-168: Flow Mode Context Package Contract](Task-168-Flow-Mode-Context-Package-Contract.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [Task-157: Improve Context Hardness](../done/Task-157-Improve-Context-Hardness.md), [Task-161: Per-Feature Chat-Summary Timeline](../done/Task-161-Per-Feature-Chat-Summary-Timeline.md), [CA-132: Prompt Context Continuity And Provider Handoff](../../../change-audit/CA-132-prompt-context-continuity-and-provider-handoff.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, prompt-assembly, handoff, no-vector`

## AI Quick View

### Summary

- Wire the `Task-168` context package into Flow Mode Plan-to-Coding handoff.
- Coding receives one stable context block and does not broaden retrieval on its own.
- Prompt logging must make the composed handoff inspectable.
- Existing chat prompt injection behavior must not regress.

### Current Ask

- Implement the prompt-assembly handoff from Plan output to Coding input.

### Key Decisions

- `T-1` Plan owns broad context retrieval; Coding consumes the rendered package.
- `T-2` Coding prompt includes the same package on retries unless Plan is rerun.
- `T-3` Handoff is step-scoped and source-referenced.
- `T-4` Prompt logging remains the primary manual inspection path.

### Constraints

- Depends on `Task-168`.
- Do not change provider adapter contracts unless required to pass prompt text through existing turn input.
- Do not add vector retrieval or model-side tool browsing for Coding context expansion.
- Preserve `isHandoffPrompt` and system-prompt protections in `feature_history.go`.

### Open Questions

- None. Use the defaults in this task.

### Source Refs

- `CP-41 P-3`, `DOD-2`, `DOD-5`
- `Task-168`
- `CA-132`

## 1. Goal

Make the Flow Mode Plan step hand a rendered context package to the Coding step through a stable prompt section that downstream code and tests can inspect.

## 2. Parent Links

- coding plan: `CP-41`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-41 P-3`, `CP-41 DOD-2`, `Task-168 DOD-6`

## 3. Trigger

Once the Plan step can build a package, Flow Mode needs a reliable way to pass it to Coding without asking Coding to rediscover history or read unbounded context.

## 4. Exact Change

- `T-1` Add Flow Mode handoff storage/state.
  - Store the `FlowContextPackage` or rendered package against the workflow run and Plan step result.
  - The package should be available to the next Coding step without rebuilding.
  - If the user reruns Plan, replace the package with the new Plan output.

- `T-2` Add Coding prompt composition.
  - Prepend the rendered package under `## Flow Context Package`.
  - Add a short instruction block:
    - use the package as the source of truth for prior context;
    - do not broaden retrieval unless explicitly instructed;
    - preserve source refs when explaining changes.
  - Keep the user's Coding instruction visible after the package.

- `T-3` Add retry reuse semantics.
  - Coding retries must reuse the same Plan package by default.
  - A retry may add Testing feedback from `Task-170`, but must not mutate the original package.
  - A Plan rerun is the only normal way to replace the package.

- `T-4` Add prompt logging coverage.
  - Ensure `logComposedPrompt` captures the Flow Context Package in the final Coding prompt.
  - Add test or fixture assertion against the logged prompt text.

- `T-5` Preserve existing prompt behavior.
  - Normal chat history injection still uses existing `injectFeatureHistoryPrompt`.
  - Handoff/system prompts must not accidentally re-resolve on the package text.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/workflow_orchestrator.go`
  - `apps/local-runner/internal/runner/feature_history.go`
  - `apps/local-runner/internal/runner/prompt_log.go`
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/workflow_orchestrator_test.go`
  - optional new file: `apps/local-runner/internal/runner/flow_context_handoff.go`
- modules:
  - workflow orchestration
  - prompt assembly
  - runner state
  - prompt logging
- routes:
  - existing workflow run/turn routes only
- tables:
  - none expected

## 6. Acceptance Check

- A Flow Mode Coding step receives the rendered package from Plan.
- The package appears once, before the Coding instruction, with source refs intact.
- A Coding retry reuses the same package and appends only retry feedback when `Task-170` is present.
- A Plan rerun replaces the package.
- Normal chat prompt injection and cross-provider handoff protections still pass existing tests.

### 6.1 Test Items

- `TestFlowCodingPromptIncludesPlanContextPackage`
- `TestFlowCodingPromptIncludesPackageOnce`
- `TestFlowCodingRetryReusesPlanPackage`
- `TestPlanRerunReplacesFlowContextPackage`
- `TestFlowContextPackageAppearsInPromptLog`
- `TestNormalChatFeatureHistoryInjectionUnchanged`
- `TestSystemPromptDoesNotResolveFromContextPackageText`

### 6.2 Definition of Done

- [ ] `DOD-1` Flow run state can store and retrieve the Plan context package.
- [ ] `DOD-2` Coding prompt includes a stable `## Flow Context Package` section.
- [ ] `DOD-3` Coding retries reuse the package unless Plan reruns.
- [ ] `DOD-4` Prompt logging captures the package for manual validation.
- [ ] `DOD-5` Existing chat/handoff prompt tests still pass.
- [ ] `DOD-6` No vector DB, embedding, or similarity-search path is introduced.
- [ ] `DOD-7` Targeted runner tests pass.

## 7. Out of Scope

- Building the package contract. That is `Task-168`.
- Running tests and retrying Coding. That is `Task-170`.
- Audit generation. That is `Task-171`.
- Desktop UI for inspecting packages unless already available through prompt logs.

## 8. Completion Notes

- result: `pending`
- follow-ups: `Task-170` appends validation feedback to this handoff path.
- upstream docs updated: update `CP-41` only if handoff ownership changes.
