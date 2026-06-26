# CA-036: Update Supported Models Constraint

Update the step definitions supported model check constraint in Supabase database schema to include the new Gemini models.

## Scope

- Database check constraint `step_definitions_supported_model_check` on the `step_definitions` table.

## Completed

- Created SQL migration `supabase/migrations/20260528154900_update_supported_models_constraint.sql`.
- Dropped the existing constraint `step_definitions_supported_model_check`.
- Re-added the check constraint with the updated model list:
  - `auto-gemini-3`
  - `auto-gemini-2.5`
  - `gemini-3.1-pro-preview`
  - `gemini-3-flash-preview`
  - `gemini-3.1-flash-lite-preview`
  - `gemini-2.5-pro`
  - `gemini-2.5-flash`
  - `gemini-2.5-flash-lite`
  - `gemini-flash`
  - `gemini-pro`
  - `claude-haiku`
  - `claude-sonnet`
  - `claude-opus`
  - `gpt-5.4-mini`
  - `gpt-5.4`
  - `gpt-5.5`

## Verification

- Verified that frontend tests pass successfully via `vitest run`.
- PostgreSQL syntax is correct and covers all new Gemini models.

## Residual Notes

- The user must apply the migration (`supabase db push` or equivalent in their local/staging environment) to reflect the changes in the database.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-036
change_type: feature
summary: Update Supported Models Constraint
# --->8---
