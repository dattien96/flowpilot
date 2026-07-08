# BUG-253: Cloned Built-In Workflow Edit/Save Hits Workflow Step Order Unique Constraint

## Metadata

- Document ID: `BUG-253`
- Title: `Cloned Built-In Workflow Edit/Save Hits Workflow Step Order Unique Constraint`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [Task-175: Built-In Flow Mirror Sync And Resolver](../../08-Task/done/Task-175-Builtin-Flow-Mirror-Sync-And-Resolver.md), [Task-179: Settings Flow Pack Authoring UI](../../08-Task/done/Task-179-Settings-Flow-Pack-Authoring-UI.md), [Task-189: Custom Flow Graph Authoring](../../08-Task/done/Task-189-Custom-Flow-Graph-Authoring.md)
- Child Documents: `none`
- Related Documents: [CA-160: Flow Definitions Migrated To Workflows Table](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md), [CA-168: Insert-Before-Delete for Workflow Steps Save/Mirror](../../../change-audit/CA-168-atomic-insert-then-delete-workflow-steps.md), [CA-182: Preserve Policy Edges On Workflow Save](../../../change-audit/CA-182-preserve-policy-edges-on-workflow-save.md)
- Replaces: `none`
- Tags: `workflow-authoring, desktop, supabase, clone, workflow-steps, order-index, regression`

## AI Quick View

### Summary

- Found in the live desktop Settings flow while editing a user-owned clone of a built-in workflow: clone succeeds, but the first subsequent save fails with `duplicate key value violates unique constraint "workflow_steps_workflow_id_order_index_key"`.
- Root cause: `SupabaseAdminRepository.saveWorkflow` had already been hardened to insert replacement `workflow_steps` before deleting old ones (to avoid data loss, CA-168), but unlike the Go mirror/store path it did not offset the replacement rows' `order_index` during that insert. A cloned workflow already has existing `workflow_steps` rows in the same `workflow_id`, so inserting the replacements at `0..N-1` collides with the still-live old rows occupying that exact unique key range.
- Fixed by applying the same two-phase order strategy the Go path already uses conceptually: insert replacement rows outside the live `0..N-1` band, delete superseded rows, then renormalize the new rows back to `0..N-1`.

### Current Ask

- Preserve the insert-before-delete safety from CA-168 without tripping `workflow_steps(workflow_id, order_index)` uniqueness when saving an edited clone of a built-in workflow.

### Key Decisions

- `V-1` Keep the insert-before-delete ordering from CA-168; do not regress to delete-first just to avoid the unique constraint.
- `V-2` Add a TS-side `order_index` offset during the replacement insert, matching the same underlying invariant the Go store already documented: new rows must not reuse the old rows' unique key slots until the old rows are deleted.
- `V-3` Renormalize the newly inserted rows back to `0..N-1` after the delete so subsequent reads remain stable and human-friendly.

### Constraints

- Scoped to the desktop app's real workflow authoring save path: `WorkflowsSettings.tsx` → `getAdminUseCases()` → `SupabaseAdminRepository.saveWorkflow`.
- Must not touch the separate design contract from BUG-236 / Task-189: node identity stays on `step_definitions`, not `workflow_steps`.
- Must preserve the existing CA-168 guarantee that a failed replacement insert leaves the old step rows untouched.

### Open Questions

- None.

### Source Refs

- User repro: clone a built-in workflow in Settings, edit it, save it; desktop shows `duplicate key value violates unique constraint "workflow_steps_workflow_id_order_index_key"`.
- [supabaseAdminRepository.ts](/Users/tiendat/Desktop/flowpilot/flowpilot/packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts)
- [workflowFlowEngineAttrs.test.ts](/Users/tiendat/Desktop/flowpilot/flowpilot/tests/phase1/workflowFlowEngineAttrs.test.ts)
- [CA-168-atomic-insert-then-delete-workflow-steps.md](/Users/tiendat/Desktop/flowpilot/flowpilot/change-audit/CA-168-atomic-insert-then-delete-workflow-steps.md)

## 1. Issue Summary

In the desktop app's Settings > Workflows screen, a built-in workflow can be cloned into an editable user-owned copy. That clone path itself works, but editing the cloned workflow and pressing Save immediately failed with:

- `duplicate key value violates unique constraint "workflow_steps_workflow_id_order_index_key"`

This was reproducible against the live Supabase-backed desktop authoring flow and blocked further verification of custom-flow behavior because the cloned workflow could not be persisted after modification.

## 2. Parent Links

- impacted coding plan: none directly; this is a desktop workflow-authoring persistence bug in the implementation path shipped for built-in mirror / custom-flow authoring.
- impacted task lineage:
  - [Task-175](../../08-Task/done/Task-175-Builtin-Flow-Mirror-Sync-And-Resolver.md) — cloneable built-in workflow copies
  - [Task-179](../../08-Task/done/Task-179-Settings-Flow-Pack-Authoring-UI.md) — desktop Settings workflow authoring surface
  - [Task-189](../../08-Task/done/Task-189-Custom-Flow-Graph-Authoring.md) — custom flow graph editing on top of that save path

## 3. Environment and Reproduction

- environment: desktop-flowpilot app, real Supabase-backed Settings > Workflows screen
- reproduction steps:
  1. Open a built-in workflow in Settings.
  2. Click Clone and create a user-owned copy.
  3. Edit the cloned workflow (rename, add/remove/reorder steps, or otherwise trigger a save with `steps` present).
  4. Click Save.
  5. Observe the save failure with the unique-constraint error above.
- frequency: deterministic for cloned workflows with existing `workflow_steps` rows, because the replacement insert reuses `order_index` values that are still occupied until the later delete runs.

## 4. Expected vs Actual

- expected: a cloned workflow saves successfully after edits; the replacement step list remains atomic and order is preserved.
- actual: the save fails at the replacement insert with a unique-constraint violation on `(workflow_id, order_index)`.

## 5. Impact

- users affected: anyone editing a cloned built-in workflow in the desktop authoring UI
- workflows affected: Settings workflow clone/edit/save path, especially custom-flow experimentation built on cloned built-ins
- severity: medium — no data loss after CA-168's insert-before-delete fix, but the workflow becomes effectively uneditable because each save attempt errors out

## 6. Root Cause

- hypothesis: the desktop TS save path kept CA-168's safer insert-before-delete ordering but did not also adopt the unique-key collision avoidance already documented on the Go side.
- confirmed cause: `SupabaseAdminRepository.saveWorkflow` inserted replacement `workflow_steps` rows using the exact final `order_index` values (`0..N-1`) before deleting the old rows. For a cloned workflow, the old rows already occupy `(workflow_id=<clone-id>, order_index=0..N-1)`. Because `workflow_steps` enforces `unique(workflow_id, order_index)`, the replacement insert collides immediately and fails before the delete can run.
- evidence:
  - the live desktop error reports the exact Postgres unique constraint for `(workflow_id, order_index)`
  - the affected save path is [supabaseAdminRepository.ts](/Users/tiendat/Desktop/flowpilot/flowpilot/packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts), which already used insert-before-delete from CA-168
  - [CA-168-atomic-insert-then-delete-workflow-steps.md](/Users/tiendat/Desktop/flowpilot/flowpilot/change-audit/CA-168-atomic-insert-then-delete-workflow-steps.md) explicitly documented the same uniqueness hazard on the Go side, but its TS note assumed the offset trick was unnecessary; this bug proves that assumption was false for real cloned workflows

## 7. Fix Strategy

- `F-1` Add a TS-side `workflowStepInsertOrderOffset = 1_000_000` in [supabaseAdminRepository.ts](/Users/tiendat/Desktop/flowpilot/flowpilot/packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts).
- `F-2` During `saveWorkflow`, insert replacement `workflow_steps` rows at `workflowStepInsertOrderOffset + (step.orderIndex ?? index)` instead of their final `0..N-1` values.
- `F-3` After the replacement rows are inserted successfully, delete the superseded rows exactly as before.
- `F-4` Renormalize each newly inserted row back to `order_index = 0..N-1` with per-row updates after the delete completes.
- `F-5` Update the phase1 repository tests to assert the new ordering contract and the offset/renormalize behavior, while preserving the existing regression guard that a failed insert must not delete the old rows.

## 8. Validation

- `V-1` `rtk npx tsx --test tests/phase1/workflowFlowEngineAttrs.test.ts` — `7/7` pass, including the new replacement-offset/renormalize assertions.
- `V-2` `rtk npx tsx --test tests/phase1/supabaseAdminRepository.test.ts` — `5/5` pass.
- `V-3` `rtk npx tsc -p tsconfig.phase1-tests.json --pretty false` — no TypeScript errors from the edited files.
- `V-4` `rtk npm --prefix apps/desktop-flowpilot run test:phase1` still reports pre-existing unrelated phase1 test failures in other files (`desktopSupabaseAuthRepository.test.ts`, `navigatorCatalog.test.ts`, `settingsHelpers.test.ts`); none are attributable to this fix, and the targeted repository tests above pass cleanly.
- `V-5` Not executed in this environment: a second live desktop click-through after the fix. The expected manual verification is: clone built-in workflow → edit → save succeeds → reload still shows correct step order.

## 9. Regression Guard

- tests:
  - `saveWorkflow inserts new steps before deleting superseded ones`
  - `saveWorkflow offsets replacement step order_index before renormalizing`
  - `saveWorkflow does not delete existing steps when the insert fails`
  - existing `BUG-236` contract test updated to assert against the replacement insert payload rather than the post-renormalize state
- alerts: none
- audit checks: `gitnexus_detect_changes()` was not run because GitNexus MCP tools were unavailable in this thread; validation proceeded by direct diff review and targeted tests

## 10. Follow-Up Document Updates

- upstream docs that must change: none required. This bug corrects an implementation detail in the desktop persistence path; it does not change built-in cloneability, custom-flow authoring intent, or the workflow/node data-model contract.
- notes left unchanged on purpose: [CA-168](../../../change-audit/CA-168-atomic-insert-then-delete-workflow-steps.md) remains historically correct about the original data-loss fix and the Go-side uniqueness hazard. Its TS-side note is now superseded by this bug's finding, but the bug is documented here rather than rewriting that older audit entry in place.
