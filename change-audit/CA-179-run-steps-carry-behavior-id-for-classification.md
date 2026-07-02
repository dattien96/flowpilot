# CA-179: Live Workflow Run Steps Carry behavior_id for Classification (BUG-NOTE-CP42 #7)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: the live per-run step bridge (`SupabaseWorkflowStore.LoadRunSteps`) dropped a step's CP-42 `behavior_id`, leaving generic-flow steps unclassifiable by the Flow Mode context-handoff helpers.

## The bug

The migration stores a step's canonical CP-42 behavior on `workflow_steps.behavior_id`, but `LoadRunSteps`'s embedded select only pulled `requires_approval` from the joined `workflow_steps` definition row — `behavior_id` never reached `RuntimeWorkflowStep` at all. `isCodingStepType`/`isPlanStepType` (`flow_context_handoff.go`) classified purely from `RuntimeWorkflowStep.StepType`, running it through `agentpack.NormalizeBehaviorID`'s alias table.

This was worse than "inconsistent" — it was actually broken for the exact case CP-42 introduced: a UI-authored generic flow's step has `step_type` set to one of the reusable dispatch-category values seeded by the `add_flow_engine_attrs_to_workflows` migration (e.g. `"flow-agent-delegate"`), and `NormalizeBehaviorID` has no alias entry for that string at all — it only recognizes the legacy CP-41 literal values (`"coding"`, `"plan"`, `"implementation"`, etc.). So a generic flow's coding/plan steps were completely invisible to `injectFlowContextIfCoding`/`findPlanStepID`/`maybeClearPlanContextForPlanStep`, disconnecting UI-authored flows from the Flow Mode context-handoff path entirely — not just misclassified, unclassifiable.

## Fix

- Added `RuntimeWorkflowStep.BehaviorID string` (`workflow_state_machine.go`).
- `LoadRunSteps`'s embedded select (`supabase_workflow_store.go`) now requests `workflow_steps(requires_approval,behavior_id)` and decodes `behavior_id` into the new field.
- `isCodingStepType`/`isPlanStepType` (`flow_context_handoff.go`) now take `(behaviorID, stepType string)` and classify via a new `classifyStepBehavior` helper: prefer `behaviorID` when non-empty, fall back to `stepType` otherwise — preserving the exact legacy behavior for steps predating CP-42 (their `behavior_id` is empty, so they classify by the recognized `"coding"`/`"plan"` step_type literal exactly as before).
- All three call sites in `flow_context_handoff.go` updated to pass both `BehaviorID` and `StepType` from the relevant `RuntimeWorkflowStep`.

## Verification

- New test `TestGenericFlowStepTypeClassifiesByBehaviorID` (`flow_pack_migration_test.go`): asserts `step_type="flow-agent-delegate"` alone does NOT classify as Coding (proving the "completely unclassifiable" claim), but classifies correctly once `behavior_id="agent.delegate"` is supplied; same for a Plan step via `context.produce`.
- Extended `TestSupabaseStoreLoadRunStepsShaping` (`phase5_test.go`) to assert the embed select string includes `behavior_id` and that a row with `"behavior_id":"context.produce"` decodes into `RuntimeWorkflowStep.BehaviorID`.
- Updated existing tests (`TestArbitraryNodeStepTypeIsInertWithoutRunnerChange`, `TestLegacyPlanCodingStepTypesStillResolveThroughAliasTable`) for the new two-argument signature — their assertions and behavior are otherwise unchanged, proving the legacy step_type-only path still works byte-for-byte when no `behavior_id` is present.
- Full suite: 1008 passed, 15 pre-existing/environmental failures (same test names as the established baseline; a summarizer subtest-counting artifact reported "16" vs "15" across two otherwise-identical runs — confirmed not a real regression).
- `go build ./...` and `go vet ./...` clean (only the pre-existing, unrelated `gitnexus.go` vet warning remains).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: LoadRunSteps now carries behavior_id through to RuntimeWorkflowStep, and isCodingStepType/isPlanStepType classify by it first, so a CP-42 generic flow's steps (whose step_type is an unrecognized dispatch category) are no longer invisible to the Flow Mode context-handoff path
# --->8---
