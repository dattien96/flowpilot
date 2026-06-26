# CA-051 Single-Step Step-Definition YOLO Policy

## Scope

Finalized the YOLO source-of-truth rule after Task-030/Task-031 churn. YOLO may exist only on `workflows.yolo_mode` and `step_definitions.yolo_mode`; workflow-definition runs use workflows, direct single-step runs use step definitions, and run-detail UI is display-only.

## Completed

- Documented that `workflows.yolo_mode` is the SSOT for every workflow-definition run, including workflows with exactly one child step.
- Documented that `step_definitions.yolo_mode` is the SSOT only for direct `single-step` launches.
- Documented that `workflow_steps.yolo_mode` must remain out of the active model.
- Documented that `workflow_runs.yolo_mode` is historical and must not drive resume, continue, or follow-up behavior.
- Documented that run detail UI should display one current effective YOLO value and must not expose a YOLO mutation control.

## Verification

- Documentation-only sync in this pass; no tests were run after the doc wording update.
- Previous implementation verification before this doc sync:
  - `npm test -- src/features/workflow-engine/workflow-start-runtime.test.ts src/data/repository/supabase/workflow-engine-mappers.test.ts src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts src/routes/_authenticated/workflow-steps/create.test.tsx 'src/routes/_authenticated/workflow-steps/$stepType.test.tsx'` passed with `63` tests.
  - `npm run build` passed for `apps/admin-web`.

## Residual Notes

- GitNexus MCP tools were not available in this thread, so the repository's required symbol impact analysis could not be executed before edits.
- The working tree already contained uncommitted Task-030 changes before this slice; this audit records the final YOLO rule sync.
- Existing code may still need a follow-up implementation pass if any UI currently allows YOLO mutation from the run detail page or other prohibited run-level surfaces.
- The build still emits existing Vite warnings about browser-externalized Node modules and large chunks.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: TASK-030
change_type: feature
summary: Single-Step Step-Definition YOLO Policy
# --->8---
