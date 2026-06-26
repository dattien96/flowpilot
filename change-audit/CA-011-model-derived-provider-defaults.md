# CA-011 Model-Derived Provider Defaults

## Scope

Changed the AI configuration contract so `model` is the only user-facing choice and `provider` is derived automatically from the selected model. The default fallback is now `gpt-5.4` for model selection and `medium` for reasoning effort when the user does not pick either value.

This work spans project defaults, workflow defaults, reusable workflow steps, workflow run launch resolution, step execution resolution, and the corresponding docs and database backfills.

## Completed

- Updated the Admin Web settings and workflow editor surfaces so users edit model and reasoning only, while provider is shown as a read-only derived value.
- Updated workflow launch and execution resolution so provider is derived from the resolved model at every layer:
  - project default
  - workflow override
  - step override
  - workflow run baseline
- Updated the Supabase and demo gateways to persist the derived provider reference alongside model and reasoning defaults.
- Updated the local workflow runtime and the Supabase Edge Function workflow start path to resolve `gpt-5.4` and `medium` as the fallback values when nothing is selected.
- Updated the step-definition UI so `/workflow-steps` shows reasoning in cards and editing forms uses the same `gpt-5.4` / `medium` baseline.
- Added database backfill migrations:
  - `20260525140000_backfill_ai_model_and_reasoning_defaults.sql`
  - `20260525141000_backfill_step_definition_model_defaults.sql`
- Updated the AI provider specification, tech design, and coding plan documents to reflect the model-first contract and the new default behavior.
- Updated relevant tests to assert the new default values and provider derivation behavior.

## Verification

- Ran focused Vitest coverage in `apps/admin-web` for:
  - project settings
  - workflow create/detail
  - workflow step create/detail
  - Supabase gateway project creation
- Ran `npm run build` in `apps/admin-web`.
- Ran `git diff --check`.

## Residual Notes

- The migration backfills `step_definitions.model` from `gpt-5.5` to `gpt-5.4`; if any old environments intentionally depend on `gpt-5.5` step defaults, they will need a separate override after this migration.
- The Codex review loop timed out twice before returning findings, so verification currently relies on the successful app build, targeted tests, and diff checks.
- The working tree still contains other previously modified files from the broader AI orchestration workstream; this audit note covers the model-derived provider/defaults slice specifically.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-011
change_type: feature
summary: Model-Derived Provider Defaults
# --->8---
