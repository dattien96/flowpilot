# Task-178: RAG Harness Pack And Chat Baseline

## Metadata

- Document ID: `Task-178`
- Title: `RAG Harness Pack And Chat Baseline`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-180`
- Related Documents: `Task-175`, `Task-176`, `CP-41-RAG-Harness-Flow-Mode`
- Replaces: `N/A`
- Tags: `rag-harness, context-package, chat-mode, flow-mode, generic-flow`

## AI Quick View

### Summary

- Convert CP-41 RAG Harness behavior from step-name hardcodes into behavior/config-driven execution.
- Keep Chat Mode context baseline auto-on as the current product behavior.
- Flow Mode RAG Harness remains a selectable built-in flow template.

### Current Ask

- Make context package creation and handoff generic enough that runner does not check `plan`, `coding`, or equivalent semantic strings.

### Key Decisions

- `T-1` Context package generation is deterministic Go behavior, not an AI prompt contract.
- `T-2` Chat Mode baseline RAG is automatic and not a picker option.
- `T-3` Flow Mode RAG Harness is a built-in read-only flow that users can clone.

### Constraints

- Depends on `Task-175` and `Task-176`.
- Do not remove old CP-41 helper functions until parity is validated.
- Environment validation failures must not trigger coding retry.

### Open Questions

- Future user-defined context types should be extension points, not required for this task.

### Source Refs

- `apps/local-runner/internal/runner/flow_context_package.go`
- `apps/local-runner/internal/runner/flow_context_handoff.go`
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/prompts/flow-context-handoff.md`

## 1. Goal

Refactor CP-41 context package and handoff behavior so it is selected by flow node behavior/config, while Chat Mode continues to receive the deterministic baseline context package automatically.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `CP-41`, `Task-175`, `Task-176`

## 3. Trigger

CP-41 currently has hardcoded helpers like `isPlanStepType` and `isCodingStepType`. That contradicts the generic flow model because new step names require runner edits.

## 4. Exact Change

- `T-1` Register deterministic context behavior:
  - `context.deterministic_feature_package`
  - input: user ask, feature key resolver config, source selectors
  - output: `FlowContextPackage`
- `T-2` Register prompt handoff behavior:
  - `context.prompt_handoff`
  - input: package ID/ref, prompt template ref, target node
  - output: prompt fragment with sentinel.
- `T-3` Update `rag-harness.yaml` so Plan node uses context package behavior and Coding node consumes prompt handoff behavior.
- `T-4` Remove step-name checks from the active execution path:
  - no behavior should require `stepType == plan`.
  - no behavior should require `stepType == coding`.
- `T-5` Keep compatibility shim for existing definitions that still use old step types.
- `T-6` Chat Mode:
  - always creates baseline context package where current behavior requires it.
  - does not require user to select RAG Harness.
  - stores package reference in session state.
- `T-7` Flow Mode:
  - only creates context package when the selected flow node declares that behavior.
  - uses mirrored `rag-harness` built-in definition if user selects it.
- `T-8` Add tests for:
  - Chat Mode baseline package creation.
  - Flow Mode RAG Harness package creation.
  - custom flow without context behavior does not create package.
  - old `plan/coding` step types still work through compatibility shim.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_context_package.go`
  - `apps/local-runner/internal/runner/flow_context_handoff.go`
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/flow-context-handoff.md`
- modules:
  - context harness
  - behavior registry
  - flow executor
- routes:
  - none unless Chat Mode request/response needs context metadata
- tables:
  - definition mirror only
  - no run logs

## 6. Acceptance Check

- `isPlanStepType` and `isCodingStepType` are not required by the main execution path.
- Chat Mode still gets RAG/context baseline automatically.
- Flow Mode can run RAG Harness from built-in flow definition.
- Custom flow without context behavior does not trigger RAG package logic.
- Existing CP-41 validation retry semantics are preserved.

## 7. Out of Scope

- Arbitrary user-defined context plugin execution.
- UI flow editor.
- Audit commit confirmation UI changes.

## 8. Completion Notes

- result: partially implemented
- notes: `injectFlowContextIfCoding`'s build/render steps now dispatch through the `context.produce`/`context.render` behaviors (`DefaultBehaviorRegistry`) instead of calling `BuildFlowContextPackage`/`ComposeFlowCodingPrompt` directly; all existing caching, event-emission, and warning behavior is unchanged and covered by existing tests. `isPlanStepType`/`isCodingStepType` remain as the single, explicitly-documented legacy `StepType`-string shim (they already normalize through `agentpack.NormalizeBehaviorID`) because `RuntimeWorkflowStep` has no `behaviorId` field yet — making the step classification itself (not just dispatch) data-driven is a larger data-model change deferred to `Task-180`/future work. Chat Mode `subMode` request fields and the Flow Mode built-in `rag-harness` selection flow (T-6/T-7) are UI/contract work not attempted here — see Task-177 status for the same UI dependency.
- follow-ups: `Task-180`
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes and [CA-151](../../../change-audit/CA-151-rag-harness-behavior-dispatch-rewire.md)
