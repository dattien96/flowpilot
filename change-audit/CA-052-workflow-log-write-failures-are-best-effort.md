# CA-052: Workflow Log Write Failures Are Best Effort

## Summary

- `insertLog()` in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` now treats Supabase log insert failures as non-fatal.
- A `TypeError: fetch failed` from `workflow_run_logs` no longer aborts background workflow execution.
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` now covers the failure mode.

## Changed Files

- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`

## Verification

- `npx vitest run src/features/workflow-engine/workflow-start-runtime.test.ts`
- `npm run build`

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-052
change_type: feature
summary: Workflow Log Write Failures Are Best Effort
# --->8---
