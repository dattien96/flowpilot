# BUG-290: Context Produce Step Card Shows Inherited Model

## Metadata

- Document ID: `BUG-290`
- Title: `Context Produce Step Card Shows Inherited Model`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex, Claude Sonnet`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: [Task-183](../../08-Task/done/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [BUG-227](./BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md)
- Child Documents: `none`
- Related Documents: [CP-51](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CA-225](../../../change-audit/CA-225-user-owned-model-resolution.md), [CA-368](../../../change-audit/CA-368-context-produce-step-model-visibility.md)
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, flow-step-timeline, workflow-settings, context-produce, display-bug`

## AI Quick View

### Summary

- A `context.produce` item displayed the run's inherited model (for example `grok-4.5`) despite being a non-agent inline behavior with no model configuration of its own.
- The flow item remains visible; only its misleading model label is hidden.
- The Settings > Step Definitions list had the same defect: it rendered `stepType / model` even though the Model field is hidden for that behavior.

### Current Ask

- Render model metadata only for `agent.delegate` and legacy nodes without a `behaviorId`, in both the Flow Timeline and Step Definitions list.

### Key Decisions

- `F-1`: Gate the existing run-model fallback with a pure `flowStepShowsModel` helper.
- `F-2`: Treat `context.produce` and other explicit non-agent behaviors as model-less; preserve legacy no-`behaviorId` rendering.
- `F-3`: Make the Settings form and list share one dependency-light model-visibility/subtitle helper.

### Constraints

- Desktop display-only: do not change backend model resolution, step execution, or the visible Context Produce item.
- Add regression coverage in a new test file only.

### Open Questions

- None.

### Source Refs

- `packages/flowpilot-client-core/src/domain/adminModels.ts` (`context.produce` has `requiresAgent: false`)
- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- [BUG-227](./BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md)

## 1. Issue Summary

The Flow Step Timeline and Settings > Step Definitions list showed a model on Context Produce, implying the inline non-agent behavior had an independently configured model.

## 2. Parent Links

- Task-183 owns Flow Mode model/provider display behavior.
- BUG-227 is the related desktop model-display correction.

## 3. Environment and Reproduction

1. Run a flow containing a `context.produce` step.
2. Open its expanded Flow Step Timeline item or Settings > Step Definitions.
3. Observe the model metadata line or the list subtitle (`grok-context / grok-4.5`).

## 4. Expected vs Actual

- Expected: Context Produce remains visible but does not show a model label; its Settings list subtitle is `grok-context`, without a trailing model.
- Actual: the timeline showed the run fallback model and the Settings list showed `grok-context / grok-4.5`.

## 5. Impact

- Misleading desktop-only model provenance for non-agent flow behaviors.
- Execution and persisted model resolution are unaffected.

## 6. Root Cause

- Confirmed: `FlowStepTimeline` unconditionally derived `step.model || runModel`, and `WorkflowsSettings` unconditionally rendered `step.stepType / step.model`, including explicit non-agent behaviors.

## 7. Fix Strategy

- `F-1`: add `flowStepShowsModel(behaviorId)` and only apply the model fallback when the node is `agent.delegate` or legacy behavior-less.
- `F-2`: add a focused new test file covering Context Produce, other inline behaviors, delegated agents, and legacy nodes.
- `F-3`: use `stepDefinitionListSubtitle` for the Settings list and the same `stepDefinitionRequiresModel` rule used by the form; add a new focused test asserting the exact subtitle for Context Produce, delegated, and legacy steps.

## 8. Validation

- `V-1`: `npx tsx --test apps/desktop-flowpilot/src/components/FlowStepTimeline.model-visibility.test.ts` passed 4/4.
- `V-2`: `npm --prefix apps/desktop-flowpilot run build` passed (`tsc --noEmit` and Vite build).
- `V-3`: `npx tsx --test apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.step-list-model-visibility.test.ts` passed 4/4.

## 9. Regression Guard

- New `FlowStepTimeline.model-visibility.test.ts` and `WorkflowsSettings.step-list-model-visibility.test.ts` assert that Context Produce and other explicit inline behaviors hide model metadata while delegated and legacy nodes keep it.

## 10. Follow-Up Document Updates

- No upstream change: this corrects desktop display semantics only and does not change model-resolution or execution behavior.
