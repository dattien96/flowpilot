# CA-201: Remove workflow_steps Overrides

## Summary

Fixed `BUG-164`: removed `provider_override`/`model_override`/`reasoning_effort_override` from `workflow_steps` entirely, after live investigation (following `BUG-160`/`BUG-163`) confirmed the current live system doesn't depend on them and the documented architecture (SS-05/SD-06) doesn't require them. A step's model/provider is now always its step type's own `step_definitions.model`.

## What Changed

- `supabase/migrations/20260702140000_drop_workflow_steps_overrides.sql`: drops the three columns from `workflow_steps` (leaves `workflows`' own equivalent columns untouched).
- `apps/local-runner/internal/runner/workflow_state_machine.go`: `RuntimeWorkflowStep.Provider`/`.Model` doc updated — always derived from `step_definitions.model`.
- `apps/local-runner/internal/runner/supabase_workflow_store.go`: `LoadRunSteps` no longer selects/reads `workflow_steps.provider_override`/`model_override`; Model/Provider unconditionally come from the joined `step_definitions.model`.
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`: `insertSteps` no longer needs the `BUG-163` explicit-null workaround (column gone).
- `apps/local-runner/internal/runner/phase5_test.go`: updated/consolidated tests for the unconditional catalog-derived model/provider.
- `packages/flowpilot-client-core/src/domain/adminModels.ts`: `WorkflowStep` drops the three fields.
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`: `mapWorkflowStep`, `saveWorkflow`'s step insert, and `cloneWorkflow`'s step-copy insert stop reading/writing them.
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: removed the per-step Model override/Reasoning `<select>` fields; `addWorkflowStep` no longer initializes them; workflow-dirty snapshot no longer tracks them for steps.

## Verification

- `go build ./...` + targeted `go test` in `apps/local-runner` — pass.
- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Confirmed `apps/admin-web`'s own (separate) workflow-engine code is unaffected by the shared-package changes; left untouched per user's "legacy, unused" statement — its raw queries against the dropped columns will fail, accepted as collateral.
- Not verified live (no Supabase/backend in this environment) — flagged in `BUG-164` (`V-5`).

## Notes

- SS-05/SD-06 both still document a per-step override storage tier on `workflow_steps` that no longer exists — flagged as a documentation-only follow-up in `BUG-164`, not addressed here.
- A related gap (workflow/project-level model defaults are not actually resolved anywhere in the live desktop/Go run-start path) was discovered during this investigation and explicitly deferred by the user as separate future work.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-164
change_type: refactor
summary: Remove workflow_steps provider/model/reasoning override columns entirely; a step's model/provider is now always its step type's own step_definitions.model
# --->8---
