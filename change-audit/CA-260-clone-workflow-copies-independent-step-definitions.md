# CA-260: Clone Workflow Copies Independent Step Definitions

## Scope

Fixed BUG-262, found while diagnosing BUG-261: cloning a workflow (e.g. Settings' "Clone" on the built-in Review Loop) reused the source's `step_type` values verbatim for the clone's steps, so the clone and its source shared the exact same `step_definitions` rows. Saving the clone's edges then silently overwrote the source's (including a built-in's) `dependsOn` with the clone's own edge topology — this is exactly what corrupted the live `review-loop` built-in's `synthesis` node.

## Changes

- `supabaseAdminRepository.ts`: `cloneWorkflow` now deep-copies each source step into a new `step_definitions` row with a fresh, workflow-scoped `step_type` (`<clonedWorkflowId>__<sourceStepType>`), and points the clone's `workflow_steps` at the new rows instead of the source's.
- `WorkflowsSettings.tsx`: `persistEdgeDerivedDependsOn` now checks `listWorkflowsUsingSteps` before writing a computed `dependsOn`, skipping (and reporting to the user) any step still shared with another workflow instead of silently overwriting it. `saveWorkflow`/`saveNewWorkflow` surface skipped steps in the save confirmation message.

## Verification

- `npm --prefix apps/desktop-flowpilot run typecheck` — passed.
- `npx tsx --test tests/phase1/workflowFlowEngineAttrs.test.ts` — 8 passed, including the new regression test asserting a clone's steps get independent step_definitions rows.
- `npx tsx --test tests/phase1/*.test.ts` — 48 passed, 2 failed (pre-existing, unrelated path-resolution issues in `desktopRunnerMode.test.ts`/`importBoundary.test.ts`, confirmed via stack trace not touching any file this change modified).
- `persistEdgeDerivedDependsOn`'s new guard (F-2) has no dedicated unit test — `WorkflowsSettings.tsx` has no existing test harness in this repo; verified by code review and typecheck only.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-262
change_type: bugfix
summary: clone workflow now deep-copies step definitions with fresh step_types, and saving edge-derived dependsOn skips steps still shared with another workflow
# --->8---
