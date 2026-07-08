# CA-255: Offset Workflow Step Insert Order To Avoid Unique Collision

## Scope

Fixed a live desktop Settings > Workflows regression: saving an edited clone of a built-in workflow failed with `duplicate key value violates unique constraint "workflow_steps_workflow_id_order_index_key"`.

## Changes

- `supabaseAdminRepository.ts`: `saveWorkflow` now inserts replacement `workflow_steps` rows at `order_index = (step.orderIndex ?? index) + workflowStepInsertOrderOffset` (offset `1_000_000`) instead of their final `0..N-1` values, so the insert-before-delete step (CA-168) never collides with the old rows still occupying that key range on a cloned workflow. After the superseded rows are deleted, each new row is renormalized back to `0..N-1` via a per-row update.
- `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`: mirrored the same offset-then-renormalize change for the phase1 test runner.
- `tests/phase1/workflowFlowEngineAttrs.test.ts` / `.phase1-tests/tests/phase1/workflowFlowEngineAttrs.test.js`: `FakeTable` now tracks `insertPayloads`/`updatePayloads` separately from `lastPayload` and gains an `update()` method; added `saveWorkflow offsets replacement step order_index before renormalizing`; the insert/delete-order test now asserts `["insert", "delete", "update"]`; the BUG-236 node-identity test reads `insertPayloads[0]` and asserts the pre-renormalize offset value (`1_000_000`) instead of `0`.

## Verification

- `rtk npx tsc --noEmit -p tsconfig.phase1-tests.json --pretty false` — no errors.
- `rtk npx tsx --test tests/phase1/workflowFlowEngineAttrs.test.ts` — 7/7 passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-253
change_type: bugfix
summary: offset replacement workflow_steps insert order_index outside the live 0..N-1 range before renormalizing, so saving an edited clone of a built-in workflow no longer hits the (workflow_id, order_index) unique constraint
# --->8---
