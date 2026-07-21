# CA-368: Context Produce step model visibility

## Summary

Fixed both Flow Step Timeline and Settings > Step Definitions rendering so explicit non-agent behaviors, including `context.produce`, do not display a model as step-owned metadata. The Settings list now displays `grok-context`, not `grok-context / grok-4.5`; delegated and legacy nodes keep their existing model display.

## Verification

- `npx tsx --test apps/desktop-flowpilot/src/components/FlowStepTimeline.model-visibility.test.ts`: 4 passed.
- `npx tsx --test apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.step-list-model-visibility.test.ts`: 4 passed.
- `npm --prefix apps/desktop-flowpilot run build`: passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-290
change_type: bugfix
summary: Hide model labels from non-agent Context Produce Flow Timeline and Step Definitions items.
# --->8---
