# CA-151: RAG Harness Behavior Dispatch Rewire

## Scope

Implement the backend slice of `Task-178`: route the CP-41 context-package build/render steps through the CP-42 behavior registry (`Task-176`) instead of calling `BuildFlowContextPackage`/`ComposeFlowCodingPrompt` directly from `injectFlowContextIfCoding`.

## Completed

- Added `produceFlowContextPackage` and `renderFlowContextPrompt` in `flow_context_handoff.go`, which dispatch `context.produce`/`context.render` through `DefaultBehaviorRegistry()` and only fall back to the direct function call if dispatch itself errors (a registry defect, not a legitimate no-context case).
- Added `DefaultBehaviorRegistry()` — a process-wide, lazily-built `BehaviorRegistry` singleton — so call sites share one registry instance instead of constructing one per call.
- `injectFlowContextIfCoding`'s three call sites (cached-package render, fresh-package build, freshly-built render) now go through behavior dispatch. Caching, event emission (`EventFlowContextPackage`), and the `no_plan_step` warning logic are unchanged — they still live in `injectFlowContextIfCoding`, which is orchestration, not the behavior itself.
- `isPlanStepType`/`isCodingStepType` are unchanged: they already route through `agentpack.NormalizeBehaviorID` (CA-147) and remain the single, explicitly-labeled legacy `StepType`-string compatibility shim, since `RuntimeWorkflowStep` has no `behaviorId` field yet (that would be a larger data-model migration, tracked separately, not attempted here).

## Verification

- `go test ./internal/runner -run 'TestFlowCoding|TestFlowContext|TestPlanRerun|TestCodingStep|TestPlanPackage|TestMaybeClearPlanContext|TestNormalChat|TestSystemPromptDoesNotResolve|TestBehavior|TestDefaultRegistry'` — all existing flow-context-handoff behavioral assertions (caching, idempotency, warning generation, event keying) pass unchanged, since the wrapped handlers replicate the same logic as the direct calls they replace.
- `go test ./internal/runner/... ./internal/agentpack/...` — full suite; identical pre-existing failure set to CA-149/CA-150 (unrelated Codex/Windows-path/provider-account environment tests).

## Follow-ups

- Making `RuntimeWorkflowStep` carry a `behaviorId` field (so the classification itself, not just dispatch, is data-driven) is a larger schema change and remains open for `Task-180`/future work.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-178
change_type: refactor
summary: route RAG harness context production and rendering through the CP-42 behavior registry instead of direct function calls
# --->8---
