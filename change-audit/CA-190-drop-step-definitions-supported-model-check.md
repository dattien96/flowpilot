# CA-190: Drop step_definitions Supported-Model Check Constraint

## Summary

Fixed `BUG-154`: saving a workflow step definition with a model registered dynamically through `ai_supported_models` (Task-013) but not present in the hard-coded `step_definitions_supported_model_check` allow-list failed with a Postgres check-constraint violation, even though the model was valid per the app's own registry-backed dropdowns and runtime validation.

## What Changed

- Added `supabase/migrations/20260702110000_drop_step_definitions_supported_model_check.sql`, which drops `step_definitions_supported_model_check` from `public.step_definitions` and does not replace it with another closed list.
- This implements the "Migration C" step that `Task-013` had already scoped and deferred: stop relying on a hard-coded SQL allow-list for step models now that `ai_supported_models` is the dynamic source of truth used by every model dropdown and by `workflow-start-runtime.ts` validation.
- No application code changed; `workflow-steps` create/edit forms already source their model options from `useSupportedModels()` (the registry), so no new invalid values become reachable as a result of this migration.

## Verification

- Reviewed all `step_definitions.model` write paths (`saveStepDefinition` in `supabase-workflow-engine-gateway.ts`) and confirmed nothing depends on the constraint rejecting a value.
- Confirmed no test or source file references the constraint name `step_definitions_supported_model_check`.
- Not verified: applying the migration against a live/local Supabase database and re-running the manual repro, because no `supabase` CLI / local stack was available in this environment. Documented as an open validation gap in `BUG-154` section 7 (`V-3`).

## Notes

- This is the third time this constraint has needed a change (`20260528154900`, `20260628152000`, now removal) purely to keep a static list in sync with an already-dynamic registry table; removing the constraint entirely (rather than appending yet another string) avoids a fourth recurrence.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-154
change_type: bugfix
summary: Drop stale step_definitions_supported_model_check constraint so registry-added AI models can be saved on step definitions
# --->8---
